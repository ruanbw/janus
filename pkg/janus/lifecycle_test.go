package janus_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"janus/pkg/janus"
)

// runWithTimeout 执行 app.Run 并在超时后判失败,避免启动错误被吞掉时测试挂死
func runWithTimeout(t *testing.T, app *janus.App, d time.Duration) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(context.Background()) }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(d):
		t.Fatalf("app.Run 未在 %v 内返回(启动错误可能被吞掉,应用以无 DB 模式继续运行)", d)
		return nil
	}
}

func TestAppRunLifecycle(t *testing.T) {
	t.Run("Given 不支持的 DatabaseURL scheme When Run Then 返回错误而不是以无 DB 模式静默启动", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{
			Addr:        "127.0.0.1:0",
			DatabaseURL: "mysql://user:pass@127.0.0.1:3306/janus",
		}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		err = runWithTimeout(t, app, 3*time.Second)
		if err == nil || !strings.Contains(err.Error(), "scheme") {
			t.Fatalf("expected unsupported scheme error, got %v", err)
		}
		// 启动失败后 running 必须复位:再次 Run 应得到同样的错误而不是 already running
		err = runWithTimeout(t, app, 3*time.Second)
		if err == nil || strings.Contains(err.Error(), "already running") {
			t.Fatalf("expected running reset after failed Run, got %v", err)
		}
	})

	t.Run("Given postgresql:// scheme 的 DatabaseURL When Run Then 尝试连接并返回连接错误", func(t *testing.T) {
		// 端口非法,pgx 解析阶段即失败,无需真实数据库;
		// 旧实现只认 postgres:// 前缀,postgresql:// 会被跳过,应用在没有 DB 的情况下启动。
		app, err := janus.New(janus.WithConfig(janus.Config{
			Addr:        "127.0.0.1:0",
			DatabaseURL: "postgresql://janus:janus@127.0.0.1:notaport/janus",
		}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		err = runWithTimeout(t, app, 5*time.Second)
		if err == nil || !strings.Contains(err.Error(), "connect db") {
			t.Fatalf("expected connect db error for postgresql:// url, got %v", err)
		}
	})

	t.Run("Given WithDB 注入且配置了 MigrationsDir When Run Then 执行基座核心迁移并返回其错误", func(t *testing.T) {
		// 指向不可达端口的 GORM 连接(关闭自动 ping,构造阶段不连库):
		// 若 Run 跳过核心迁移(旧实现传 dir=""),这里会正常启动而不是报错。
		gdb, err := gorm.Open(postgres.Open("postgres://janus:janus@127.0.0.1:1/janus?sslmode=disable&connect_timeout=2"), &gorm.Config{
			DisableAutomaticPing: true,
			Logger:               logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			t.Fatalf("open gorm: %v", err)
		}
		t.Cleanup(func() {
			if sqlDB, err := gdb.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})

		app, err := janus.New(
			janus.WithDB(gdb),
			janus.WithConfig(janus.Config{
				Addr:          "127.0.0.1:0",
				MigrationsDir: "../../migrations",
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		err = runWithTimeout(t, app, 10*time.Second)
		if err == nil || !strings.Contains(err.Error(), "migrate") {
			t.Fatalf("expected migrate error from WithDB branch, got %v", err)
		}
	})

	t.Run("Given 端口已被占用 When Run Then 返回监听错误且 running 复位", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer func() { _ = ln.Close() }()

		app, err := janus.New(janus.WithConfig(janus.Config{Addr: ln.Addr().String()}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		for i := 0; i < 2; i++ {
			err := runWithTimeout(t, app, 3*time.Second)
			if err == nil || strings.Contains(err.Error(), "already running") {
				t.Fatalf("run #%d: expected listen error, got %v", i+1, err)
			}
		}
	})

	t.Run("Given 显式 Shutdown When Run 返回 Then 返回 nil 且可再次 Run", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{Addr: "127.0.0.1:0"}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		for i := 0; i < 2; i++ {
			errCh := make(chan error, 1)
			go func() { errCh <- app.Run(context.Background()) }()
			time.Sleep(50 * time.Millisecond)

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := app.Shutdown(shutdownCtx); err != nil {
				cancel()
				t.Fatalf("run #%d: shutdown: %v", i+1, err)
			}
			cancel()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("run #%d: expected nil after Shutdown, got %v", i+1, err)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("run #%d: Run did not return after Shutdown", i+1)
			}
		}
	})

	t.Run("Given WithRoutes 与 WithProtectedRoutes When New Then 路由回调各只执行一次", func(t *testing.T) {
		var routeCalls, protCalls atomic.Int32
		app, err := janus.New(
			janus.WithConfig(janus.Config{Addr: "127.0.0.1:0"}),
			janus.WithRoutes(func(engine *gin.Engine) {
				routeCalls.Add(1)
				engine.GET("/custom", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
			}),
			janus.WithProtectedRoutes(func(rg *gin.RouterGroup) {
				protCalls.Add(1)
				rg.GET("/secret", func(c *gin.Context) { c.String(http.StatusOK, "secret") })
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}
		if routeCalls.Load() != 1 || protCalls.Load() != 1 {
			t.Fatalf("expected each route callback once, got routes=%d protected=%d", routeCalls.Load(), protCalls.Load())
		}

		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/custom", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("custom route: expected 200, got %d", w.Code)
		}
		// 无 DB 模式下受保护路由一律 401
		w = httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/secret", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("protected route without db: expected 401, got %d", w.Code)
		}
		w = httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("unknown route: expected 404, got %d", w.Code)
		}
	})
}
