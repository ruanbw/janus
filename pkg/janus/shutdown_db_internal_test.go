package janus

import (
	"context"
	"os"
	"testing"
	"time"

	"janus/internal/config"
	"janus/internal/db"
)

// TestShutdownClosesOwnedDB 钉住「Shutdown 关闭自建连接池」契约:
// SDK 场景下 App 经 DatabaseURL 自建的底层 sql.DB 必须在 Shutdown 时关闭,
// 否则每次生命周期都泄漏一个连接池。
func TestShutdownClosesOwnedDB(t *testing.T) {
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test_pkg?sslmode=disable"
	}
	ctx := context.Background()
	probe, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	probe.Close()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Addr = "127.0.0.1:0"
	cfg.DatabaseURL = dsn
	cfg.MigrationsDir = "" // 跳过迁移,避免依赖相对路径

	app, err := New(WithConfig(cfg))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = app.Run(runCtx) }()

	// 等待 bootstrap 完成(server 已赋值即代表 gdb/store 已建立)。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		app.mu.Lock()
		srv := app.server
		app.mu.Unlock()
		if srv != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	app.mu.Lock()
	gdb := app.gdb
	owned := app.dbOwned
	app.mu.Unlock()
	if gdb == nil || !owned {
		t.Fatal("期望 bootstrap 自建 gdb 且标记为自有")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gorm DB(): %v", err)
	}
	if err := sqlDB.PingContext(ctx); err == nil {
		t.Fatal("Shutdown 后连接池应已关闭(Ping 应失败)")
	}
}
