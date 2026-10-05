package plugin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"janus/pkg/plugin"
)

// mockEnricher 模拟访客画像富化器
type mockEnricher struct {
	name string
	asn  string
}

func (m *mockEnricher) Name() string { return m.name }
func (m *mockEnricher) Enrich(ctx context.Context, r *http.Request, fact *plugin.Fact) {
	if fact != nil {
		fact.ASN = m.asn
	}
}

// orderRecorderEnricher 记录执行顺序的富化器
type orderRecorderEnricher struct {
	name   string
	order  *[]string
	lock   *sync.Mutex
}

func (o *orderRecorderEnricher) Name() string { return o.name }
func (o *orderRecorderEnricher) Enrich(ctx context.Context, r *http.Request, fact *plugin.Fact) {
	o.lock.Lock()
	defer o.lock.Unlock()
	*o.order = append(*o.order, o.name)
}

// panicEnricher 模拟 panic 的画像富化器
type panicEnricher struct {
	name string
}

func (p *panicEnricher) Name() string { return p.name }
func (p *panicEnricher) Enrich(ctx context.Context, r *http.Request, fact *plugin.Fact) {
	panic("enricher panic boom")
}

// mockInterceptor 模拟前置规则拦截器
type mockInterceptor struct {
	name     string
	decision *plugin.Decision
	match    bool
}

func (m *mockInterceptor) Name() string { return m.name }
func (m *mockInterceptor) Intercept(ctx context.Context, r *http.Request, linkID int64, fact *plugin.Fact) (*plugin.Decision, bool) {
	return m.decision, m.match
}

// panicInterceptor 模拟 panic 的规则拦截器
type panicInterceptor struct {
	name string
}

func (p *panicInterceptor) Name() string { return p.name }
func (p *panicInterceptor) Intercept(ctx context.Context, r *http.Request, linkID int64, fact *plugin.Fact) (*plugin.Decision, bool) {
	panic("interceptor panic boom")
}

// mockSDKInjector 模拟 SDK 注入器
type mockSDKInjector struct {
	name   string
	suffix string
}

func (m *mockSDKInjector) Name() string { return m.name }
func (m *mockSDKInjector) WrapSDK(domain, code, rawSDK string) string {
	return rawSDK + "\n/* " + m.suffix + " */"
}

// panicSDKInjector 模拟 panic 的 SDK 注入器
type panicSDKInjector struct {
	name string
}

func (p *panicSDKInjector) Name() string { return p.name }
func (p *panicSDKInjector) WrapSDK(domain, code, rawSDK string) string {
	panic("sdk injector panic boom")
}

// =========================================================================
// 扩展点 1: FactEnricher 测试规格
// =========================================================================

func TestFactEnricherContract(t *testing.T) {
	t.Run("Given 初始状态 When 未注册任何 Enricher Then Apply 不修改 Fact 且 HasASNProvider 为 false", func(t *testing.T) {
		plugin.ResetEnrichers()
		defer plugin.ResetEnrichers()

		if plugin.HasASNProvider() {
			t.Fatal("expected HasASNProvider to be false initially")
		}

		fact := &plugin.Fact{Country: "US"}
		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		plugin.ApplyEnrichers(context.Background(), req, fact)

		if fact.ASN != "" {
			t.Fatalf("expected ASN to be empty, got %q", fact.ASN)
		}
	})

	t.Run("Given 多个 Enricher When 按序注册 Then Apply 必须保持注册顺序依次执行", func(t *testing.T) {
		plugin.ResetEnrichers()
		defer plugin.ResetEnrichers()

		var execOrder []string
		var mu sync.Mutex

		e1 := &orderRecorderEnricher{name: "enricher-1", order: &execOrder, lock: &mu}
		e2 := &orderRecorderEnricher{name: "enricher-2", order: &execOrder, lock: &mu}
		e3 := &orderRecorderEnricher{name: "enricher-3", order: &execOrder, lock: &mu}

		plugin.RegisterEnricher(e1)
		plugin.RegisterEnricher(e2)
		plugin.RegisterEnricher(e3)

		fact := &plugin.Fact{Country: "US"}
		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		plugin.ApplyEnrichers(context.Background(), req, fact)

		if len(execOrder) != 3 || execOrder[0] != "enricher-1" || execOrder[1] != "enricher-2" || execOrder[2] != "enricher-3" {
			t.Fatalf("expected execution order [enricher-1, enricher-2, enricher-3], got %v", execOrder)
		}
	})

	t.Run("Given 前置 Enricher 发生 panic When 执行 Apply Then 保证 Fail-Open 隔离且后续 Enricher 继续生效", func(t *testing.T) {
		plugin.ResetEnrichers()
		defer plugin.ResetEnrichers()

		plugin.RegisterEnricher(&panicEnricher{name: "faulty"})
		plugin.RegisterEnricher(&mockEnricher{name: "asn", asn: "AS15169"})

		fact := &plugin.Fact{Country: "US"}
		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)

		// 关键断言：不发生未捕获 panic
		plugin.ApplyEnrichers(context.Background(), req, fact)

		if fact.ASN != "AS15169" {
			t.Fatalf("expected ASN = AS15169 despite previous panic, got %q", fact.ASN)
		}
	})

	t.Run("Given 高并发注册与求值 When 并发调用 Register 与 Apply Then 无竞态冲突", func(t *testing.T) {
		plugin.ResetEnrichers()
		defer plugin.ResetEnrichers()

		const goroutines = 20
		const iterations = 50
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					if id%2 == 0 {
						plugin.RegisterEnricher(&mockEnricher{name: "asn", asn: "AS12345"})
					} else {
						f := &plugin.Fact{Country: "DE"}
						r := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
						plugin.ApplyEnrichers(context.Background(), r, f)
					}
				}
			}(i)
		}
		wg.Wait()
	})

	t.Run("Given 已注册 Enricher When 调用 ResetEnrichers Then 状态完全隔离重置", func(t *testing.T) {
		plugin.ResetEnrichers()
		plugin.RegisterEnricher(&mockEnricher{name: "asn", asn: "AS15169"})
		if !plugin.HasASNProvider() {
			t.Fatal("expected ASN provider to exist")
		}

		plugin.ResetEnrichers()
		if plugin.HasASNProvider() {
			t.Fatal("expected ASN provider to be reset")
		}
	})
}

// =========================================================================
// 扩展点 2: RuleInterceptor 测试规格
// =========================================================================

func TestRuleInterceptorContract(t *testing.T) {
	t.Run("Given 初始状态 When 未注册 Interceptor Then CheckInterceptors 返回 false", func(t *testing.T) {
		plugin.ResetInterceptors()
		defer plugin.ResetInterceptors()

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		fact := &plugin.Fact{Country: "US"}

		d, ok := plugin.CheckInterceptors(context.Background(), req, 100, fact)
		if ok || d != nil {
			t.Fatalf("expected false and nil decision, got ok=%v, d=%v", ok, d)
		}
	})

	t.Run("Given 拦截器短路命中 When 执行 CheckInterceptors Then 返回短路决议并中断后续求值", func(t *testing.T) {
		plugin.ResetInterceptors()
		defer plugin.ResetInterceptors()

		plugin.RegisterInterceptor(&mockInterceptor{
			name:     "pass_interceptor",
			decision: &plugin.Decision{Action: "pass"},
			match:    true,
		})
		plugin.RegisterInterceptor(&mockInterceptor{
			name:     "never_reached",
			decision: &plugin.Decision{Action: "block"},
			match:    true,
		})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		fact := &plugin.Fact{Country: "US"}

		d, ok := plugin.CheckInterceptors(context.Background(), req, 100, fact)
		if !ok || d == nil || d.Action != "pass" {
			t.Fatalf("expected short-circuit pass decision, got ok=%v, d=%+v", ok, d)
		}
	})

	t.Run("Given 拦截器发生 panic When 执行 CheckInterceptors Then Fail-Open 忽略并继续由后续拦截器处理", func(t *testing.T) {
		plugin.ResetInterceptors()
		defer plugin.ResetInterceptors()

		plugin.RegisterInterceptor(&panicInterceptor{name: "panic_first"})
		plugin.RegisterInterceptor(&mockInterceptor{
			name:     "second_interceptor",
			decision: &plugin.Decision{Action: "allow"},
			match:    true,
		})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		fact := &plugin.Fact{Country: "US"}

		d, ok := plugin.CheckInterceptors(context.Background(), req, 100, fact)
		if !ok || d == nil || d.Action != "allow" {
			t.Fatalf("expected second interceptor to handle after panic, got ok=%v, d=%+v", ok, d)
		}
	})

	t.Run("Given 重置拦截器 When 调用 ResetInterceptors Then 清空所有已注册拦截器", func(t *testing.T) {
		plugin.ResetInterceptors()
		plugin.RegisterInterceptor(&mockInterceptor{
			name:     "temp",
			decision: &plugin.Decision{Action: "temp"},
			match:    true,
		})
		plugin.ResetInterceptors()

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		d, ok := plugin.CheckInterceptors(context.Background(), req, 100, nil)
		if ok || d != nil {
			t.Fatalf("expected empty interceptors after reset, got ok=%v, d=%+v", ok, d)
		}
	})
}

// =========================================================================
// 扩展点 3: ActionHandler 测试规格
// =========================================================================

func TestActionHandlerContract(t *testing.T) {
	t.Run("Given 注册自定义 ActionHandler When 执行 ExecuteActionHandler Then 成功交付并返回 true", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		var called bool
		plugin.RegisterActionHandler("proxy", func(c *gin.Context, dctx plugin.DeliveryContext) bool {
			called = true
			c.String(http.StatusOK, "proxied:"+dctx.Target)
			return true
		})

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		dctx := plugin.DeliveryContext{
			Target: "https://origin.example.com",
			Decision: plugin.Decision{
				Action: "proxy",
			},
		}

		handled := plugin.ExecuteActionHandler(c, "proxy", dctx)
		if !handled {
			t.Fatal("expected ExecuteActionHandler to return true")
		}
		if !called {
			t.Fatal("expected custom handler to be called")
		}
		if w.Body.String() != "proxied:https://origin.example.com" {
			t.Fatalf("unexpected response body: %q", w.Body.String())
		}
	})

	t.Run("Given 未注册 Action When 执行 ExecuteActionHandler Then 返回 false 退化为基座处理", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		dctx := plugin.DeliveryContext{Target: "https://example.com"}
		handled := plugin.ExecuteActionHandler(c, "unregistered_action", dctx)
		if handled {
			t.Fatal("expected unhandled action to return false")
		}
	})

	t.Run("Given Handler 执行 panic 且尚未写入响应 When 触发异常 Then Fail-Open 退化返回 false", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		plugin.RegisterActionHandler("boom_action", func(c *gin.Context, dctx plugin.DeliveryContext) bool {
			panic("handler panic before write")
		})

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		dctx := plugin.DeliveryContext{Target: "https://example.com"}
		handled := plugin.ExecuteActionHandler(c, "boom_action", dctx)
		if handled {
			t.Fatal("expected handled to be false when handler panicked before writing")
		}
	})

	t.Run("Given Handler 执行 panic 但已写入部分响应 When 触发异常 Then 返回 true 标记已终结", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		plugin.RegisterActionHandler("boom_after_write", func(c *gin.Context, dctx plugin.DeliveryContext) bool {
			c.Status(http.StatusOK)
			_, _ = c.Writer.WriteString("partial response")
			panic("handler panic after write")
		})

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		dctx := plugin.DeliveryContext{Target: "https://example.com"}
		handled := plugin.ExecuteActionHandler(c, "boom_after_write", dctx)
		if !handled {
			t.Fatal("expected handled to be true when response was already written before panic")
		}
	})

	t.Run("Given 非法参数注册 When 传入空 action 或 nil handler Then 拒绝注册", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		plugin.RegisterActionHandler("", func(*gin.Context, plugin.DeliveryContext) bool {
			return true
		})
		plugin.RegisterActionHandler("nil_handler", nil)

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		dctx := plugin.DeliveryContext{Target: "https://example.com"}

		if handled := plugin.ExecuteActionHandler(c, "", dctx); handled {
			t.Fatal("expected empty action to not be registered")
		}
		if handled := plugin.ExecuteActionHandler(c, "nil_handler", dctx); handled {
			t.Fatal("expected nil handler to not be registered")
		}
	})
}

// =========================================================================
// 扩展点 4: SDKInjector 测试规格
// =========================================================================

func TestSDKInjectorContract(t *testing.T) {
	t.Run("Given 未设置 Injector When 调用 WrapGeneratedSDK Then 原样返回原生 SDK", func(t *testing.T) {
		plugin.ResetSDKInjector()
		defer plugin.ResetSDKInjector()

		raw := "console.log('original sdk');"
		wrapped := plugin.WrapGeneratedSDK("example.com", "link1", raw)
		if wrapped != raw {
			t.Fatalf("expected raw SDK when injector is nil, got %q", wrapped)
		}
	})

	t.Run("Given 设置了活跃 Injector When 调用 WrapGeneratedSDK Then 返回包装后的 SDK 内容", func(t *testing.T) {
		plugin.ResetSDKInjector()
		defer plugin.ResetSDKInjector()

		plugin.SetSDKInjector(&mockSDKInjector{name: "probe", suffix: "anti-headless-probe"})
		raw := "console.log('original sdk');"
		wrapped := plugin.WrapGeneratedSDK("example.com", "link1", raw)
		expected := raw + "\n/* anti-headless-probe */"
		if wrapped != expected {
			t.Fatalf("expected wrapped SDK %q, got %q", expected, wrapped)
		}
	})

	t.Run("Given Injector 执行中 panic When 调用 WrapGeneratedSDK Then Fail-Open 原样返回原生 SDK", func(t *testing.T) {
		plugin.ResetSDKInjector()
		defer plugin.ResetSDKInjector()

		plugin.SetSDKInjector(&panicSDKInjector{name: "faulty_injector"})
		raw := "console.log('original sdk');"
		wrapped := plugin.WrapGeneratedSDK("example.com", "link1", raw)
		if wrapped != raw {
			t.Fatalf("expected raw SDK on injector panic, got %q", wrapped)
		}
	})

	t.Run("Given 重置 Injector When 调用 ResetSDKInjector Then 恢复为未设置状态", func(t *testing.T) {
		plugin.ResetSDKInjector()
		plugin.SetSDKInjector(&mockSDKInjector{name: "probe", suffix: "temp"})
		plugin.ResetSDKInjector()

		raw := "console.log('original sdk');"
		wrapped := plugin.WrapGeneratedSDK("example.com", "link1", raw)
		if wrapped != raw {
			t.Fatalf("expected raw SDK after reset, got %q", wrapped)
		}
	})
}

// =========================================================================
// 扩展点 5: PostVisitHook 测试规格
// =========================================================================

func TestPostVisitHookContract(t *testing.T) {
	t.Run("Given 注册后置钩子 When 触发 TriggerPostVisitHooks Then 异步执行并接收到正确访问上下文", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		defer plugin.ResetPostVisitHooks()

		var hookCalled int32
		var receivedLinkID int64
		var mu sync.Mutex

		plugin.RegisterPostVisitHook(func(ctx context.Context, r *http.Request, visit plugin.VisitRecord) {
			mu.Lock()
			receivedLinkID = visit.LinkID
			mu.Unlock()
			atomic.AddInt32(&hookCalled, 1)
		})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		record := plugin.VisitRecord{
			LinkID: 42,
			IP:     "1.2.3.4",
			Action: "redirect",
		}

		plugin.TriggerPostVisitHooks(context.Background(), req, record)

		// 异步执行等待断言
		eventually(t, 2*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return atomic.LoadInt32(&hookCalled) == 1 && receivedLinkID == 42
		})
	})

	t.Run("Given 某个钩子执行中发生 panic When 异步触发 Then 不影响进程存活且后续钩子照常执行", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		defer plugin.ResetPostVisitHooks()

		var secondRan int32
		plugin.RegisterPostVisitHook(func(ctx context.Context, r *http.Request, visit plugin.VisitRecord) {
			panic("post hook boom")
		})
		plugin.RegisterPostVisitHook(func(ctx context.Context, r *http.Request, visit plugin.VisitRecord) {
			atomic.AddInt32(&secondRan, 1)
		})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		record := plugin.VisitRecord{LinkID: 100}

		plugin.TriggerPostVisitHooks(context.Background(), req, record)

		eventually(t, 2*time.Second, func() bool {
			return atomic.LoadInt32(&secondRan) == 1
		})
	})

	t.Run("Given 注册 nil 钩子 When 注册 Then 忽略且不影响 Trigger", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		defer plugin.ResetPostVisitHooks()

		plugin.RegisterPostVisitHook(nil)
		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		record := plugin.VisitRecord{LinkID: 1}

		// 不产生 panic
		plugin.TriggerPostVisitHooks(context.Background(), req, record)
	})

	t.Run("Given 重置钩子 When 调用 ResetPostVisitHooks Then 清空所有后置钩子", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		var ran int32
		plugin.RegisterPostVisitHook(func(ctx context.Context, r *http.Request, visit plugin.VisitRecord) {
			atomic.AddInt32(&ran, 1)
		})
		plugin.ResetPostVisitHooks()

		req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
		plugin.TriggerPostVisitHooks(context.Background(), req, plugin.VisitRecord{LinkID: 1})

		time.Sleep(100 * time.Millisecond)
		if atomic.LoadInt32(&ran) != 0 {
			t.Fatal("expected hooks to be reset, but hook was called")
		}
	})
}

// eventually 辅助断言函数
func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
