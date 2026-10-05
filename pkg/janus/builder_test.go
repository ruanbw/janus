package janus_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"janus/pkg/janus"
)

func TestAppBuilderContracts(t *testing.T) {
	t.Run("Given 默认选项 When 构造 App Then 成功构建默认实例", func(t *testing.T) {
		app, err := janus.New()
		if err != nil {
			t.Fatalf("expected New() without options to succeed, got %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil App instance")
		}
		if app.Handler() == nil {
			t.Fatal("expected non-nil http.Handler from App")
		}
	})

	t.Run("Given 无效配置 When 构造 App Then 返回配置校验错误", func(t *testing.T) {
		// 覆盖非法端口配置，期望验证失败
		invalidCfg := janus.Config{
			Addr: "invalid-port-addr-format:999999",
		}
		app, err := janus.New(janus.WithConfig(invalidCfg))
		if err == nil {
			t.Fatal("expected error when building App with invalid config, got nil")
		}
		if app != nil {
			t.Fatal("expected nil App instance when New fails")
		}
	})

	t.Run("Given 注入自定义路由 janus.WithRoutes When 通过 Handler 请求 Then 自定义端点正常工作", func(t *testing.T) {
		app, err := janus.New(
			janus.WithRoutes(func(engine *gin.Engine) {
				engine.GET("/custom/ping", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"message": "pong"})
				})
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app with routes: %v", err)
		}

		handler := app.Handler()
		if handler == nil {
			t.Fatal("handler should not be nil")
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/ping", nil)
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if body := w.Body.String(); body != `{"message":"pong"}` {
			t.Fatalf("unexpected response body: %s", body)
		}
	})

	t.Run("Given 基础系统健康端点 When 请求 /healthz Then 返回默认健康状态", func(t *testing.T) {
		app, err := janus.New()
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		app.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected /healthz to return 200, got %d", w.Code)
		}
		body, _ := io.ReadAll(w.Body)
		if len(body) == 0 {
			t.Fatal("expected non-empty healthz response")
		}
	})

	t.Run("Given 运行中的 App 实例 When 触发 Run 并由 Context 取消 Then 优雅停止 Run", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{
			Addr: "127.0.0.1:0", // 动态本地测试端口
		}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		runErrCh := make(chan error, 1)

		go func() {
			runErrCh <- app.Run(ctx)
		}()

		// 运行片刻后主动取消上下文
		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-runErrCh:
			if err != nil && err != context.Canceled {
				t.Fatalf("expected nil or context.Canceled from Run, got %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("app.Run did not stop within 3 seconds after cancel")
		}
	})

	t.Run("Given 运行中的 App 实例 When 显式调用 Shutdown Then 优雅释放资源", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{
			Addr: "127.0.0.1:0",
		}))
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		runCtx, runCancel := context.WithCancel(context.Background())
		defer runCancel()

		go func() {
			_ = app.Run(runCtx)
		}()

		time.Sleep(50 * time.Millisecond)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()

		if err := app.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("expected Shutdown to succeed, got %v", err)
		}
	})

	t.Run("Given 声明式注入事件监听器 janus.WithEventListener When 发布事件 Then 监听器被触发", func(t *testing.T) {
		done := make(chan struct{})
		_, err := janus.New(
			janus.WithEventListener("test.event", func(ctx context.Context, e janus.Event) {
				close(done)
			}),
		)
		if err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		janus.PublishEvent(context.Background(), janus.NewSimpleEvent("test.event", "payload"))

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event listener")
		}
	})
}
