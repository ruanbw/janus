package janus

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"janus/internal/config"
	"janus/internal/db"
)

// TestRunCancelsWorkerOnServerError 钉住「服务异常退出必须收口 worker」契约。
// 包内测试(可访问 app.server / app.workerDone):让 HTTP 服务在 runCtx 未取消时
// 异常退出,断言 Run 返回前已 cancel runCtx 并等 worker 退出(workerDone 关闭)。
func TestRunCancelsWorkerOnServerError(t *testing.T) {
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test_pkg?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	if err := db.Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	pool.Close()

	gdb, err := db.OpenGORM(dsn)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Addr = "127.0.0.1:0"
	// 保证 worker 各循环间隔合法(非正会让 worker 立即拒绝启动,测不出泄漏)。
	cfg.DNSRetryInterval = time.Hour
	cfg.DNSMaxAge = time.Hour
	cfg.VisitRetention = time.Hour
	cfg.VisitCleanupEvery = time.Hour

	app, err := New(WithDB(gdb), WithConfig(cfg))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(runCtx) }()

	// 等服务真正起来(srv 已赋值)。
	var srv *http.Server
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		app.mu.Lock()
		srv = app.server
		app.mu.Unlock()
		if srv != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if srv == nil {
		t.Fatal("服务未在预期时间内启动")
	}

	// 制造「服务异常退出」:Close 让 Serve 返回 ErrServerClosed(非 runCtx 取消)。
	if err := srv.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run 未在服务退出后返回")
	}

	// Run 返回前必须已 cancel + 等 worker 收口 → workerDone 已关闭。
	app.mu.Lock()
	workerDone := app.workerDone
	app.mu.Unlock()
	select {
	case <-workerDone:
	default:
		t.Fatal("服务异常退出后 worker 未收口(workerDone 未关闭),说明 runCtx 未被 cancel")
	}
}
