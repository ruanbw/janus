package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"janus/internal/rules"
	"janus/internal/store"
)

type mockInjector struct {
	suffix string
}

func (m *mockInjector) Name() string { return "mock_injector" }
func (m *mockInjector) WrapSDK(domain, code, raw string) string {
	return raw + "\n/* " + m.suffix + " */"
}

func TestExtensionPointsInHTTPAPI(t *testing.T) {
	// 1. ActionHandler 测试
	t.Run("ActionHandler 扩展点拦截与执行", func(t *testing.T) {
		ResetActionHandlers()
		defer ResetActionHandlers()

		var called bool
		RegisterActionHandler("proxy", func(c *gin.Context, dctx DeliveryContext) bool {
			called = true
			c.String(http.StatusOK, "proxied content: "+dctx.Target)
			return true
		})

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		dctx := DeliveryContext{
			Target: "https://money-page.example.com",
			Decision: rules.Decision{
				Action: "proxy",
			},
		}

		executed := ExecuteActionHandler(c, "proxy", dctx)
		if !executed {
			t.Fatal("expected ExecuteActionHandler to return true")
		}
		if !called {
			t.Fatal("expected custom ActionHandler to be called")
		}
		if w.Body.String() != "proxied content: https://money-page.example.com" {
			t.Fatalf("unexpected body: %q", w.Body.String())
		}

		// 测试未注册动作退化为 false
		notHandled := ExecuteActionHandler(c, "unknown_action", dctx)
		if notHandled {
			t.Fatal("expected ExecuteActionHandler for unknown action to return false")
		}
	})

	// 2. SDKInjector 测试
	t.Run("SDKInjector 包装落地页 SDK", func(t *testing.T) {
		ResetSDKInjector()
		defer ResetSDKInjector()

		raw := "console.log('original');"
		wrapped := WrapGeneratedSDK("example.com", "code1", raw)
		if wrapped != raw {
			t.Fatalf("expected raw SDK when injector is nil, got %q", wrapped)
		}

		SetSDKInjector(&mockInjector{suffix: "injected probe"})
		wrapped = WrapGeneratedSDK("example.com", "code1", raw)
		expected := raw + "\n/* injected probe */"
		if wrapped != expected {
			t.Fatalf("expected wrapped SDK, got %q", wrapped)
		}
	})

	// 3. PostVisitHook 测试
	t.Run("PostVisitHook 异步非阻塞执行", func(t *testing.T) {
		ResetPostVisitHooks()
		defer ResetPostVisitHooks()

		var hookCalled int32
		RegisterPostVisitHook(func(ctx context.Context, r *http.Request, visit store.VisitRecord) {
			atomic.AddInt32(&hookCalled, 1)
		})

		req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
		TriggerPostVisitHooks(context.Background(), req, store.VisitRecord{
			LinkID: 101,
		})

		// 异步等待触发
		time.Sleep(50 * time.Millisecond)
		if atomic.LoadInt32(&hookCalled) != 1 {
			t.Fatalf("expected hook to be called once, got %d", atomic.LoadInt32(&hookCalled))
		}
	})
}
