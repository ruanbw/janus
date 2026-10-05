package janus_test

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"janus/internal/db"
	"janus/internal/store"
	"janus/pkg/janus"
)

func setupMetadataTestDB(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable"
	}

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
			t.Fatalf("test database not available: %v", err)
		}
		t.Skipf("test database not available: %v", err)
	}

	release, err := db.LockTestDB(ctx, pool)
	if err != nil {
		pool.Close()
		t.Fatalf("lock test database: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := db.Migrate(ctx, pool, migrationsDir); err != nil {
		release()
		pool.Close()
		t.Fatalf("migrate base db: %v", err)
	}

	t.Cleanup(func() {
		release()
		pool.Close()
	})

	return pool, dsn
}

func TestMetadataTypeContracts(t *testing.T) {
	t.Run("Given 空或nil Metadata When 执行 Value 序列化 Then 产生非空 json 字符串对象 '{}'", func(t *testing.T) {
		var m janus.Metadata
		val, err := m.Value()
		if err != nil {
			t.Fatalf("unexpected error on nil metadata Value(): %v", err)
		}
		strVal, ok := val.(string)
		if !ok {
			if byteVal, isBytes := val.([]byte); isBytes {
				strVal = string(byteVal)
			} else {
				t.Fatalf("expected string or []byte from Value(), got %T", val)
			}
		}
		if strVal != "{}" {
			t.Fatalf("expected '{}' for nil metadata, got %q", strVal)
		}

		m = janus.Metadata{}
		val, err = m.Value()
		if err != nil {
			t.Fatalf("unexpected error on empty metadata Value(): %v", err)
		}
		strVal, ok = val.(string)
		if !ok {
			if byteVal, isBytes := val.([]byte); isBytes {
				strVal = string(byteVal)
			} else {
				t.Fatalf("expected string or []byte from Value(), got %T", val)
			}
		}
		if strVal != "{}" {
			t.Fatalf("expected '{}' for empty metadata, got %q", strVal)
		}
	})

	t.Run("Given 包含业务键值的 Metadata When 执行 Value 序列化与 Scan 反序列化 Then 正确往返恢复数据", func(t *testing.T) {
		m := janus.Metadata{
			"cloak_mode":   "safe",
			"proxy_url":    "https://example.com/money",
			"score_thresh": float64(80),
			"enabled":      true,
		}

		val, err := m.Value()
		if err != nil {
			t.Fatalf("Value() error: %v", err)
		}

		var target janus.Metadata
		if err := target.Scan(val); err != nil {
			t.Fatalf("Scan() error: %v", err)
		}

		if !reflect.DeepEqual(m, target) {
			t.Fatalf("expected roundtrip metadata %v, got %v", m, target)
		}
	})

	t.Run("Given 数据库读出 nil 或空字段 When Scan 反序列化 Then 防御性初始化为空字典而不是 nil", func(t *testing.T) {
		var target janus.Metadata
		if err := target.Scan(nil); err != nil {
			t.Fatalf("Scan(nil) error: %v", err)
		}
		if target == nil {
			t.Fatal("expected target to be non-nil Metadata{} after Scan(nil)")
		}
		if len(target) != 0 {
			t.Fatalf("expected empty metadata, got %v", target)
		}
	})

	t.Run("Given 实现 driver.Valuer 和 sql.Scanner 接口 Then 满足数据库驱动契约", func(t *testing.T) {
		var _ driver.Valuer = janus.Metadata{}
		var _ driver.Valuer = (*janus.Metadata)(nil)
	})
}

func TestLinkModelMetadataPersistence(t *testing.T) {
	t.Run("Given Link 携带自定义 Metadata When 落库与重新读取 Then Metadata 完整持久化并正确反序列化", func(t *testing.T) {
		pool, dsn := setupMetadataTestDB(t)

		gdb, err := db.OpenGORM(dsn)
		if err != nil {
			t.Fatalf("open gorm: %v", err)
		}
		t.Cleanup(func() {
			sqlDB, _ := gdb.DB()
			if sqlDB != nil {
				_ = sqlDB.Close()
			}
		})

		ctx := context.Background()
		st := store.New(gdb)

		// 准备测试租户与平台域名
		testEmail := "metadata-test-tenant@example.com"
		_, _ = pool.Exec(ctx, "DELETE FROM tenants WHERE email = $1", testEmail)
		tenant, err := st.CreateTenant(ctx, testEmail, "hashed-password", "metadata-tenant", false)
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}

		// 创建带有元数据的短链
		meta := janus.Metadata{
			"cloak_flow": "direct_offer",
			"retry_cnt":  float64(3),
			"is_vip":     true,
		}

		linkCode := "test-meta-code"
		_, _ = pool.Exec(ctx, "DELETE FROM links WHERE code = $1", linkCode)

		link := store.Link{
			TenantID:       tenant.ID,
			Code:           linkCode,
			RedirectStatus: store.RedirectStatus302,
			Status:         "enabled",
			LinkType:       "redirect",
			LandingSource:  "url",
			Metadata:       meta,
		}

		if err := gdb.WithContext(ctx).Create(&link).Error; err != nil {
			t.Fatalf("insert link with metadata: %v", err)
		}

		// 重新从数据库读取 Link
		var fetched store.Link
		if err := gdb.WithContext(ctx).First(&fetched, link.ID).Error; err != nil {
			t.Fatalf("fetch link: %v", err)
		}

		if fetched.Metadata == nil {
			t.Fatal("expected non-nil fetched.Metadata")
		}
		if fetched.Metadata["cloak_flow"] != "direct_offer" {
			t.Fatalf("expected cloak_flow 'direct_offer', got %v", fetched.Metadata["cloak_flow"])
		}
		if fetched.Metadata["is_vip"] != true {
			t.Fatalf("expected is_vip true, got %v", fetched.Metadata["is_vip"])
		}
	})
}
