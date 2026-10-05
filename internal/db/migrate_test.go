package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const migrationsDir = "../../migrations"

// testDatabaseURL 返回测试库连接串(与 testutil 同一约定)。
func testDatabaseURL() string {
	if dsn := os.Getenv("JANUS_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable"
}

// withDatabase 把连接串指向另一个库(同主机同凭据),用于建一次性数据库。
func withDatabase(dsn, dbName string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func skipUnlessRequired(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

// TestInitScriptIsTheOnlySchemaArtifact 固化「只留初始化脚本」:
// migrations/ 下只有 init.sql,且它是可整段执行的纯 SQL,不依赖 goose 迁移框架。
func TestInitScriptIsTheOnlySchemaArtifact(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}

	t.Run("Given migrations 目录 When 枚举 SQL 文件 Then 只有唯一的初始化脚本 init.sql", func(t *testing.T) {
		if len(sqlFiles) != 1 || sqlFiles[0] != "init.sql" {
			t.Fatalf("期望 migrations/ 下只有 init.sql 一份初始化脚本,实际 %v", sqlFiles)
		}
	})

	t.Run("Given 初始化脚本 When 检查内容 Then 不依赖 goose 扩展(不含 +goose 注解,可整段执行)", func(t *testing.T) {
		if len(sqlFiles) == 0 {
			t.Fatal("migrations/ 下没有任何 SQL 初始化脚本")
		}
		content, err := os.ReadFile(filepath.Join(migrationsDir, sqlFiles[0]))
		if err != nil {
			t.Fatalf("read %s: %v", sqlFiles[0], err)
		}
		if strings.Contains(string(content), "+goose") {
			t.Fatalf("%s 含 goose 注解:初始化脚本应是不依赖迁移框架的纯 SQL", sqlFiles[0])
		}
	})
}

// TestMigrateAppliesInitScript 用一次性全新数据库验证初始化脚本:
// 一份脚本必须能独立把空库建成完整 schema,且重复执行幂等、不引入任何版本表。
func TestMigrateAppliesInitScript(t *testing.T) {
	ctx := context.Background()
	dsn := testDatabaseURL()

	adminDSN, err := withDatabase(dsn, "postgres")
	if err != nil {
		t.Fatalf("build admin dsn: %v", err)
	}
	admin, err := Connect(ctx, adminDSN)
	if err != nil {
		skipUnlessRequired(t, "test database not available: %v", err)
	}
	defer admin.Close()

	scratchDB := fmt.Sprintf("janus_init_test_%d", os.Getpid())
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+scratchDB+" WITH (FORCE)"); err != nil {
		skipUnlessRequired(t, "无法重建一次性测试库 %s(需要 CREATEDB 权限): %v", scratchDB, err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+scratchDB); err != nil {
		skipUnlessRequired(t, "无法创建一次性测试库 %s(需要 CREATEDB 权限): %v", scratchDB, err)
	}

	scratchDSN, err := withDatabase(dsn, scratchDB)
	if err != nil {
		t.Fatalf("build scratch dsn: %v", err)
	}
	pool, err := Connect(ctx, scratchDSN)
	if err != nil {
		t.Fatalf("connect scratch db: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+scratchDB+" WITH (FORCE)")
	})

	t.Run("Given 全新空库 When 执行迁移 Then 全量 schema(表/索引/后补列/种子)一次落地", func(t *testing.T) {
		if err := Migrate(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("migrate fresh db: %v", err)
		}

		for _, table := range []string{
			"tiers", "tenants", "domains", "links", "link_targets", "link_domains",
			"rules", "rule_links", "visits", "sessions", "email_tokens",
		} {
			var exists bool
			if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
				t.Fatalf("query table %s: %v", table, err)
			}
			if !exists {
				t.Errorf("初始化脚本未创建表 %s", table)
			}
		}

		var idxDomain, metadata bool
		// visits(domain_id) 索引:域名删除按 domain_id 清 visits,此前是全表扫描 + 长锁。
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.idx_visits_domain') IS NOT NULL`).Scan(&idxDomain); err != nil {
			t.Fatalf("query idx_visits_domain: %v", err)
		}
		if !idxDomain {
			t.Error("初始化脚本缺少 idx_visits_domain 索引")
		}
		// links.metadata:外部工程挂载业务自定义元数据(ADR-0014)。
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM information_schema.columns
			               WHERE table_name = 'links' AND column_name = 'metadata')`).Scan(&metadata); err != nil {
			t.Fatalf("query links.metadata: %v", err)
		}
		if !metadata {
			t.Error("初始化脚本缺少 links.metadata 列")
		}

		var tiers int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tiers WHERE name = 'free'`).Scan(&tiers); err != nil {
			t.Fatalf("query tiers seed: %v", err)
		}
		if tiers != 1 {
			t.Errorf("初始化脚本未插入 free 档种子,实际 %d 行", tiers)
		}

		// 没有增量迁移,就没有任何版本表。
		var versionTables bool
		if err := pool.QueryRow(ctx, `
			SELECT to_regclass('public.goose_db_version') IS NOT NULL
			    OR to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&versionTables); err != nil {
			t.Fatalf("query version tables: %v", err)
		}
		if versionTables {
			t.Error("核心 schema 不应引入迁移版本表:只留一份可重复执行的初始化脚本")
		}
	})

	t.Run("Given 已初始化的库 When 重复执行迁移 Then 幂等成功且不破坏已有数据", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tenants (email, tier_id, slug)
			VALUES ('init-idempotent@example.com', (SELECT id FROM tiers WHERE name = 'free'), 'init-idempotent')`); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
		if err := Migrate(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("second migrate (idempotent) failed: %v", err)
		}

		var tenants int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE slug = 'init-idempotent'`).Scan(&tenants); err != nil {
			t.Fatalf("query tenant: %v", err)
		}
		if tenants != 1 {
			t.Fatalf("重复执行初始化脚本破坏了已有数据:tenants=%d", tenants)
		}

		var freeTiers int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tiers WHERE name = 'free'`).Scan(&freeTiers); err != nil {
			t.Fatalf("query tiers: %v", err)
		}
		if freeTiers != 1 {
			t.Fatalf("重复执行初始化脚本重复插入了种子:free 档 %d 行", freeTiers)
		}
	})
}
