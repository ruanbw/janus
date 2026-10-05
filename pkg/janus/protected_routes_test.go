package janus_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"janus/internal/config"
	"janus/internal/db"
	"janus/internal/jwt"
	"janus/internal/store"
	"janus/pkg/janus"
)

// setupTestDB 初始化真实 Postgres 连接与基座迁移，返回连接池和清理回调
func setupTestDB(t *testing.T) (*pgxpool.Pool, string) {
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

func TestProtectedRoutesAndExtraMigrations(t *testing.T) {
	t.Run("Given 外部通过 janus.WithProtectedRoutes 注册业务路由 When 未提供鉴权信息请求 Then 返回 401 Unauthorized", func(t *testing.T) {
		_, dsn := setupTestDB(t)

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

		app, err := janus.New(
			janus.WithDB(gdb),
			janus.WithConfig(config.Config{
				JWTSecret: "test-secret-key-1234567890123456",
				JWTTTL:    time.Hour,
			}),
			janus.WithProtectedRoutes(func(rg *gin.RouterGroup) {
				rg.GET("/custom/protected-endpoint", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"status": "ok"})
				})
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/custom/protected-endpoint", nil)
		app.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for unauthenticated request, got %d (body: %s)", w.Code, w.Body.String())
		}
	})

	t.Run("Given 外部通过 janus.WithProtectedRoutes 注册业务路由 When 携带租户登录态/有效 JWT 请求 Then 成功进入 Handler 且可提取 TenantID UserID Role", func(t *testing.T) {
		pool, dsn := setupTestDB(t)

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

		// 准备测试租户
		testEmail := "test-protected-tenant@example.com"
		_, _ = pool.Exec(ctx, "DELETE FROM tenants WHERE email = $1", testEmail)
		tenant, err := st.CreateTenant(ctx, testEmail, "hashed-password", "protected-test-tenant", false)
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		// 激活租户
		_, err = pool.Exec(ctx, "UPDATE tenants SET status = 'active' WHERE id = $1", tenant.ID)
		if err != nil {
			t.Fatalf("activate tenant: %v", err)
		}

		jwtSecret := "test-secret-key-protected-tenant-auth"
		jwtMgr := jwt.NewManager(jwtSecret, time.Hour)
		token, err := jwtMgr.Sign(tenant.ID, tenant.TokenVersion)
		if err != nil {
			t.Fatalf("sign jwt: %v", err)
		}

		type responseData struct {
			TenantID int64  `json:"tenantId"`
			UserID   int64  `json:"userId"`
			Role     string `json:"role"`
		}

		app, err := janus.New(
			janus.WithDB(gdb),
			janus.WithConfig(config.Config{
				JWTSecret: jwtSecret,
				JWTTTL:    time.Hour,
			}),
			janus.WithProtectedRoutes(func(rg *gin.RouterGroup) {
				rg.GET("/custom/identity", func(c *gin.Context) {
					auth, ok := janus.GetAuthContext(c)
					if !ok {
						c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth context"})
						return
					}
					c.JSON(http.StatusOK, responseData{
						TenantID: auth.TenantID,
						UserID:   auth.UserID,
						Role:     auth.Role,
					})
				})
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/custom/identity", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		app.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK with valid bearer token, got %d (body: %s)", w.Code, w.Body.String())
		}

		var resp responseData
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		if resp.TenantID != tenant.ID {
			t.Fatalf("expected tenantID %d, got %d", tenant.ID, resp.TenantID)
		}
		if resp.UserID != tenant.ID {
			t.Fatalf("expected userID %d, got %d", tenant.ID, resp.UserID)
		}
		if resp.Role != "tenant" {
			t.Fatalf("expected role 'tenant', got %q", resp.Role)
		}
	})

	t.Run("Given 外部注册自定义迁移 janus.WithExtraMigrations When 启动/迁移时 Then 额外表的迁移脚本被成功执行并在数据库中建表", func(t *testing.T) {
		pool, dsn := setupTestDB(t)

		// 确保清理历史专有表与迁移记录
		_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS test_extra_custom_table")
		_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS goose_db_version_extra")

		// 构造虚拟扩展迁移文件系统
		extraFS := fstest.MapFS{
			"0001_create_custom_table.sql": &fstest.MapFile{
				Data: []byte(`-- +goose Up
CREATE TABLE test_extra_custom_table (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE test_extra_custom_table;
`),
			},
		}

		baseCfg, err := config.Load()
		if err != nil {
			t.Fatalf("load base config: %v", err)
		}
		baseCfg.DatabaseURL = dsn
		baseCfg.MigrationsDir = filepath.Join("..", "..", "migrations")

		app, err := janus.New(
			janus.WithConfig(baseCfg),
			janus.WithExtraMigrations(extraFS),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runErrCh := make(chan error, 1)
		go func() {
			runErrCh <- app.Run(ctx)
		}()

		// 稍微等待 Run 执行迁移
		for i := 0; i < 20; i++ {
			time.Sleep(50 * time.Millisecond)
			var exists bool
			_ = pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_extra_custom_table')`).Scan(&exists)
			if exists {
				break
			}
		}
		cancel()

		select {
		case err := <-runErrCh:
			if err != nil && err != context.Canceled {
				t.Fatalf("app run returned unexpected error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("app did not exit in time")
		}

		// 断言额外迁移表 test_extra_custom_table 是否成功建表
		var tableExists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'test_extra_custom_table'
		)`
		if err := pool.QueryRow(context.Background(), query).Scan(&tableExists); err != nil {
			t.Fatalf("check table exists: %v", err)
		}

		if !tableExists {
			t.Fatal("expected test_extra_custom_table to be created by extra migrations, but it does not exist")
		}
	})
}
