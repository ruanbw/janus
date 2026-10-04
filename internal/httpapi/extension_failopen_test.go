package httpapi_test

// 扩展点 Fail-open 契约(ADR-0012 / 私有文档「热路径铁律」第 3 条):
//
//	插件 panic 绝不能让访客看到 5xx —— 基座必须静默退化为原生行为,
//	并且把这次 panic 记下来(不静默吞掉,否则线上无从排查)。
//
// 这里全部从 HTTP 边界黑盒验证,因为"访客拿到什么"才是契约本身:
// 富化器/拦截器/交付处理器/SDK 注入器 panic 后仍应拿到基座原生响应,
// 后置钩子 panic 不应杀死进程,也不应挡住后续钩子。

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"janus/internal/httpapi"
	"janus/internal/rules"
	"janus/internal/store"
	"janus/internal/testutil"
)

// ---- 故意 panic 的插件 ----

type panicEnricher struct{}

func (panicEnricher) Name() string { return "panic_enricher" }
func (panicEnricher) Enrich(context.Context, *http.Request, *rules.Fact) {
	panic("enricher boom")
}

type panicInterceptor struct{}

func (panicInterceptor) Name() string { return "panic_interceptor" }
func (panicInterceptor) Intercept(context.Context, *http.Request, int64, *rules.Fact) (*rules.Decision, bool) {
	panic("interceptor boom")
}

type panicInjector struct{}

func (panicInjector) Name() string { return "panic_injector" }
func (panicInjector) WrapSDK(string, string, string) string {
	panic("injector boom")
}

// ---- 日志捕获:panic 必须被记录,不能静默吞掉 ----

type lockedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// captureLogs 把 slog 默认 logger 换成写入缓冲的 handler,返回读取方法,cleanup 还原。
func captureLogs(t *testing.T) func() string {
	t.Helper()
	buf := &lockedBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf.String
}

// ---- 各扩展点的 fail-open 行为 ----

// TestEnricherPanicFallsBackToNativeRedirect 富化器 panic:访客仍拿到基座原生 302。
// 断言点放在 HTTP 状态码与 Location 上——"退化"必须是访客可观测的行为。
func TestEnricherPanicFallsBackToNativeRedirect(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fpen", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	// 富化器只在规则快照非空时才跑(ruleDecision 早退),所以必须先有规则,
	// 否则这个用例会在插件根本没被调用的情况下"通过"。
	createRule(t, c, map[string]any{
		"name": "记录命中", "action": store.RuleActionPass, "conditions": ruleOnPath(link.Code)})

	logs := captureLogs(t)
	rules.RegisterEnricher(panicEnricher{})
	t.Cleanup(rules.ResetEnrichers)

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Fatalf("Location = %q, want 基座原生目标", loc)
	}
	if !strings.Contains(logs(), "enricher boom") {
		t.Fatal("富化器 panic 未被记录,等于静默吞掉")
	}
}

// TestInterceptorPanicFallsBackToRuleEngine 拦截器 panic:退回基座规则求值,
// 规则该拦的照拦(不是"panic 一次就整条短链放行")。
func TestInterceptorPanicFallsBackToRuleEngine(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fpi", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	createRule(t, c, map[string]any{
		"name": "拦下", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})

	logs := captureLogs(t)
	rules.RegisterInterceptor(panicInterceptor{})
	t.Cleanup(rules.ResetInterceptors)

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	action, outcome, reason, _, ruleAction := ruleVisit(t, env, link.ID)
	if outcome != store.VisitOutcomeFailed || reason != store.VisitReasonRuleBlocked ||
		ruleAction != store.RuleActionNotfound {
		t.Fatalf("明细 = %q/%q/%q, want failed/rule_blocked/notfound", action, outcome, reason)
	}
	if !strings.Contains(logs(), "interceptor boom") {
		t.Fatal("拦截器 panic 未被记录")
	}
}

// TestActionHandlerPanicFallsBackToBaseAction 交付处理器 panic(还没写响应):
// 基座必须接手自己那份动作(这里渲染 404),而不是把 panic 抛给全局中间件变 5xx。
func TestActionHandlerPanicFallsBackToBaseAction(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fpa", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	createRule(t, c, map[string]any{
		"name": "拦下", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})

	logs := captureLogs(t)
	httpapi.RegisterActionHandler(store.RuleActionNotfound,
		func(*gin.Context, httpapi.DeliveryContext) bool { panic("handler boom") })
	t.Cleanup(httpapi.ResetActionHandlers)

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	_, outcome, reason, _, _ := ruleVisit(t, env, link.ID)
	if outcome != store.VisitOutcomeFailed || reason != store.VisitReasonRuleBlocked {
		t.Fatalf("明细 = %q/%q, want 基座记账 failed/rule_blocked", outcome, reason)
	}
	if !strings.Contains(logs(), "handler boom") {
		t.Fatal("交付处理器 panic 未被记录")
	}
}

// TestActionHandlerPanicAfterWriteKeepsPartialResponse 交付处理器已经写了响应
// 才 panic:此时不能再让基座写第二次(会把半截响应拼成两段,访客拿到 Frankenstein 页面)。
// 访客拿到的就是插件已经写出的那次响应。
func TestActionHandlerPanicAfterWriteKeepsPartialResponse(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fpw", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	createRule(t, c, map[string]any{
		"name": "拦下", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})

	logs := captureLogs(t)
	httpapi.RegisterActionHandler(store.RuleActionNotfound,
		func(c *gin.Context, _ httpapi.DeliveryContext) bool {
			c.String(http.StatusOK, "half-written")
			panic("handler boom after write")
		})
	t.Cleanup(httpapi.ResetActionHandlers)

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "half-written" {
		t.Fatalf("body = %q, want 只保留插件已写出的半截响应", got)
	}
	if !strings.Contains(logs(), "handler boom after write") {
		t.Fatal("写响应后的 panic 未被记录")
	}
}

// TestSDKInjectorPanicReturnsBaseSDK SDK 注入器 panic:原样返回基座 SDK。
// 基座的点击上报能力不能因为注入器炸了就一起丢。
func TestSDKInjectorPanicReturnsBaseSDK(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	createLandingLink(t, c, lid, "fpsdk", "url", "https://page.example.com/lp")

	logs := captureLogs(t)
	httpapi.SetSDKInjector(panicInjector{})
	t.Cleanup(httpapi.ResetSDKInjector)

	resp := redirectGet(t, env, "localhost", "/fpsdk/sdk.js")
	assertStatus(t, resp, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "window.Janus") {
		t.Fatalf("body 不含基座 SDK: %q", string(body))
	}
	if !strings.Contains(logs(), "injector boom") {
		t.Fatal("SDK 注入器 panic 未被记录")
	}
}

// TestPostVisitHookPanicKeepsServing 后置钩子 panic:进程存活,且后续钩子照常执行。
// 钩子跑在请求链之外的 goroutine 里,基座不 recover 就等于整个进程被插件带走;
// 同一循环里后面的钩子也会被一起丢掉。
func TestPostVisitHookPanicKeepsServing(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fph", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	var ran int32
	httpapi.RegisterPostVisitHook(func(context.Context, *http.Request, store.VisitRecord) {
		panic("hook boom")
	})
	httpapi.RegisterPostVisitHook(func(context.Context, *http.Request, store.VisitRecord) {
		atomic.AddInt32(&ran, 1)
	})
	t.Cleanup(httpapi.ResetPostVisitHooks)

	// 能走到断言本身就是"进程没被带走"的证据:panic 未 recover 时测试进程直接死。
	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	testutil.Poll(t, 3*time.Second, "panic 之后的钩子仍执行", func() bool {
		return atomic.LoadInt32(&ran) == 1
	})
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 1 {
		t.Fatalf("link.visits = %v, want 1(钩子 panic 不影响记账)", got)
	}
}

// TestRegisterActionHandlerRejectsUnusableRegistration 注册入口必须挡住
// 空 action 与 nil handler：前者会让"没注册任何处理器"与"注册在空名字上"
// 变得无法区分，后者一旦被取出调用就是一次当场 panic。
func TestRegisterActionHandlerRejectsUnusableRegistration(t *testing.T) {
	httpapi.ResetActionHandlers()
	defer httpapi.ResetActionHandlers()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	dctx := httpapi.DeliveryContext{Target: "https://t.example.com", Decision: rules.Decision{Action: ""}}

	httpapi.RegisterActionHandler("", func(*gin.Context, httpapi.DeliveryContext) bool {
		t.Error("空 action 上的处理器不该被注册,更不该被调用")
		return true
	})
	if handled := httpapi.ExecuteActionHandler(c, "", dctx); handled {
		t.Error("空 action 未被注册,ExecuteActionHandler 应返回 false")
	}

	httpapi.RegisterActionHandler("nil_handler", nil)
	if handled := httpapi.ExecuteActionHandler(c, "nil_handler", dctx); handled {
		t.Error("nil handler 未被注册,ExecuteActionHandler 应返回 false")
	}
}

// TestPostVisitHookHealthyRunLogsNothing 正常钩子不得产出 panic 日志。
// 这是"降级要留痕"的另一半：留痕只能发生在真的出事时。
// recover 判断写反不会让 panic 漏出去（recover() 已经被调用），
// 后果是每次正常访问都记一条假错误，真正的 panic 被噪声淹没。
func TestPostVisitHookHealthyRunLogsNothing(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "fpq", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	var ran int32
	httpapi.RegisterPostVisitHook(func(context.Context, *http.Request, store.VisitRecord) {
		atomic.AddInt32(&ran, 1)
	})
	t.Cleanup(httpapi.ResetPostVisitHooks)

	logs := captureLogs(t)
	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	testutil.Poll(t, 3*time.Second, "正常钩子执行", func() bool {
		return atomic.LoadInt32(&ran) == 1
	})
	if s := logs(); strings.Contains(s, "后置访问钩子 panic") {
		t.Fatalf("正常钩子不得输出 panic 日志, got %q", s)
	}
}
