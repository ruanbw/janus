package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable"
	}
	pool, err := Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
			t.Fatalf("test database not available: %v", err)
		}
		t.Skipf("test database not available: %v", err)
	}
	defer pool.Close()

	release, err := LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	defer release()

	migrationsDir := filepath.Join("..", "..", "migrations")

	// 第一次执行迁移
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("first migrate failed: %v", err)
	}

	// 第二次执行迁移，应幂等成功，不报任何错误
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("second migrate (idempotent) failed: %v", err)
	}

	// 不只是「没报错」:版本表里应当确实记录了 0001、0020 与 0021。
	// 只断言这几版存在,不锁死整表内容 —— 历史库可能残留更早的版本行(0 号、旧 2–19 号等),
	// 那是历史事实,不是本次迁移的失败。
	for _, v := range []int64{1, 20, 21} {
		var ok bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM goose_db_version WHERE version_id = $1 AND is_applied)`, v).Scan(&ok); err != nil {
			t.Fatalf("query goose_db_version: %v", err)
		}
		if !ok {
			t.Fatalf("期望 goose_db_version 已记录版本 %d", v)
		}
	}

	assertMigrationEffects(t, ctx, pool)
}

// assertMigrationEffects 断言 0020/0021 迁移的真实效果
func assertMigrationEffects(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	// visits(domain_id) 索引必须存在且有效(detachInTx 按 domain_id 清 visits)。
	var idxValid bool
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT i.indisvalid FROM pg_index i
		                 WHERE i.indexrelid = to_regclass('public.idx_visits_domain')), false)`).Scan(&idxValid); err != nil {
		t.Fatalf("query idx_visits_domain: %v", err)
	}
	if !idxValid {
		t.Fatal("期望 idx_visits_domain 索引存在且有效(0020 迁移未生效)")
	}
	// links.metadata 列必须存在(ADR-0014)。
	var col bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.columns
		              WHERE table_schema = 'public' AND table_name = 'links' AND column_name = 'metadata')`).Scan(&col); err != nil {
		t.Fatalf("query links.metadata: %v", err)
	}
	if !col {
		t.Fatal("期望 links.metadata 列存在(0021 迁移未生效)")
	}
}

// TestMigrateUpgradeFromPreSquashVersions 回归:5d8ec86 合并旧 0001–0019 后复用了 2/3 号,
// 合并前迁移过的库 goose_db_version 已有 2/3(乃至 19)号,goose 会静默跳过新文件。
// 模拟这种旧库(版本 1–19 已记录,但索引与 metadata 列不存在),迁移后两者都应存在;
// 同时覆盖「新库曾以 2/3 号应用过新文件」的情况 —— 改号后以 20/21 号幂等重跑。
func TestMigrateUpgradeFromPreSquashVersions(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable"
	}
	pool, err := Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
			t.Fatalf("test database not available: %v", err)
		}
		t.Skipf("test database not available: %v", err)
	}
	defer pool.Close()

	release, err := LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	defer release()

	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("initial migrate failed: %v", err)
	}

	// 回退到「合并前旧库」状态:撤销 0020/0021 的效果与版本记录,补齐旧 2–19 号版本行
	stmts := []string{
		`DROP INDEX IF EXISTS idx_visits_domain`,
		`ALTER TABLE links DROP COLUMN IF EXISTS metadata`,
		`DELETE FROM goose_db_version WHERE version_id IN (20, 21)`,
		`CREATE TEMP TABLE IF NOT EXISTS presquash_inserted (version_id bigint)`,
		`WITH ins AS (
			INSERT INTO goose_db_version (version_id, is_applied)
			SELECT v, true FROM generate_series(2, 19) AS v
			WHERE NOT EXISTS (SELECT 1 FROM goose_db_version g WHERE g.version_id = v)
			RETURNING version_id)
		 INSERT INTO presquash_inserted SELECT version_id FROM ins`,
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			t.Fatalf("prepare pre-squash state (%s): %v", s, err)
		}
	}
	defer func() {
		// 清理伪造的旧版本行,避免污染共享测试库
		_, _ = conn.Exec(context.WithoutCancel(ctx),
			`DELETE FROM goose_db_version WHERE version_id IN (SELECT version_id FROM presquash_inserted)`)
		_, _ = conn.Exec(context.WithoutCancel(ctx), `DROP TABLE IF EXISTS presquash_inserted`)
	}()

	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrate on pre-squash db failed: %v", err)
	}
	assertMigrationEffects(t, ctx, pool)

	// 再跑一次仍应幂等
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("second migrate on pre-squash db failed: %v", err)
	}
}
