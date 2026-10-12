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
	"strconv"
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

// notfoundInterceptor 外部状态驱动的全局熔断:任何访问都判 notfound。
// 它不带规则 id(RuleID==0),所以裁决落进明细时 rule_id 必须是 NULL。
type notfoundInterceptor struct{}

func (notfoundInterceptor) Name() string { return "global_kill_switch" }
func (notfoundInterceptor) Intercept(context.Context, *http.Request, int64, *rules.Fact) (*rules.Decision, bool) {
	return &rules.Decision{Action: store.RuleActionNotfound}, true
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

// TestInterceptorStillAppliesWhenLinkRulesDisabled 一键熔断与短链的规则开关无关。
//
// ruleDecision 原来在 !link.RulesEnabled 时直接早退,于是"外部状态驱动的全局阻断"
// 对未启用规则的短链(以及零规则租户)完全失效 —— 熔断开关拨了,流量照走。
//
// 同一条用例钉住拦截裁决的记账:拦截器没有规则 id(RuleID==0),
// visits.rule_id 必须为 NULL。写 0 会撞 rules(id) 的外键(id 从 1 起),
// recordVisit 吞掉错误后整行访问明细静默丢失,租户在访问列表里看不到这次拦截。
func TestInterceptorStillAppliesWhenLinkRulesDisabled(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "icptoff", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"rulesEnabled": false})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	rules.RegisterInterceptor(notfoundInterceptor{})
	t.Cleanup(rules.ResetInterceptors)

	// 拦截器生效:访客拿到 404,而不是直接跳走
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)

	// 裁决落进明细:rule_id 为 NULL(拦截器没有规则 id),rule_action 保留
	action, outcome, reason, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonRuleBlocked {
		t.Fatalf("明细 = %q/%q/%q, want redirect/failed/rule_blocked", action, outcome, reason)
	}
	if ruleID != nil || ruleAction != store.RuleActionNotfound {
		t.Fatalf("拦截裁决 = %v/%q, want NULL/notfound", ruleID, ruleAction)
	}

	// 访问列表接口能看到这一行:整行没丢,而不是"记了个 NULL 但行不见了"
	page := listVisits(t, c, link.ID, "")
	if len(page.Items) != 1 {
		t.Fatalf("访问明细 = %d 行, want 1(拦截裁决不能丢行)", len(page.Items))
	}
	if page.Items[0].RuleID != nil || page.Items[0].RuleAction != store.RuleActionNotfound {
		t.Fatalf("列表里的裁决 = %v/%q, want NULL/notfound", page.Items[0].RuleID, page.Items[0].RuleAction)
	}
}

// fixedIDInterceptor 返回一个指定 RuleID / Action 的拦截器裁决,
// 用于验证「非法规则 id 不得把整行明细带走」。
type fixedIDInterceptor struct {
	id     int64
	action string
}

func (fixedIDInterceptor) Name() string { return "fixed_id" }
func (i fixedIDInterceptor) Intercept(context.Context, *http.Request, int64, *rules.Fact) (*rules.Decision, bool) {
	return &rules.Decision{RuleID: i.id, Action: i.action}, true
}

// TestInterceptorInvalidRuleIDStillRecordsVisit 拦截器裁决带非法/不存在的 rule_id 时,
// 明细行不得因为外键或 CHECK 违约而整行丢失。
//
// 三种失效形态各跑一次:
//   - 负数 id:rules(id) 从 1 起,写进去就是 FK 违约;
//   - 不存在的正数 id(插件指向一条已删除的规则):同样 FK 违约;
//   - 自定义 action(如 proxy):visits.rule_action 的 CHECK 只允许四个内置值,
//     直接落库就是 CHECK 违约。
//
// 三者旧行为都是 recordVisit 吞错 -> 访问列表里看不到这次拦截,
// 而「拦截器到底拦没拦」恰恰只能从明细里回答。
func TestInterceptorInvalidRuleIDStillRecordsVisit(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "badrid", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	cases := []struct {
		name   string
		id     int64
		action string
	}{
		{"负数 id", -1, store.RuleActionNotfound},
		{"不存在的正数 id", 999999, store.RuleActionNotfound},
		{"自定义 action", 0, "proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules.RegisterInterceptor(fixedIDInterceptor{id: tc.id, action: tc.action})
			t.Cleanup(rules.ResetInterceptors)
			// 自定义 action 没有注册处理器 -> 按原跳转继续(302);notfound -> 404。
			// 两者都不该 500,也不该丢明细。
			resp := redirectGet(t, env, "localhost", "/"+link.Code)
			if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 302/404", resp.StatusCode)
			}
			_ = resp.Body.Close()
		})
	}

	page := listVisits(t, c, link.ID, "")
	if len(page.Items) != len(cases) {
		t.Fatalf("访问明细 = %d 行, want %d(非法 rule_id / 自定义 action 不能把整行带走)",
			len(page.Items), len(cases))
	}
	for _, it := range page.Items {
		if it.RuleID != nil {
			t.Fatalf("列表里的 rule_id = %v, want NULL(非法/不存在的 id 不得落库)", *it.RuleID)
		}
	}
}

// TestSimulateRunsInterceptorsLikeProduction 模拟器必须与线上一致地执行前置拦截器(ADR-0018 §4)。
//
// 线上 ruleDecision 先跑 CheckInterceptors,命中就短路;模拟器若不跑,
// 租户看到的推演与线上行为就是两套。这里注册一个恒命中的 notfound 拦截器,
// 断言 simulate 响应里给出「[前置拦截器拦截]」这一步且 verdict.blocked=true。
func TestSimulateRunsInterceptorsLikeProduction(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "simicpt", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	rules.RegisterInterceptor(notfoundInterceptor{})
	t.Cleanup(rules.ResetInterceptors)

	got := simulate(t, c, map[string]any{
		"url": "https://localhost/" + link.Code, "ip": "203.0.113.7",
	})
	if len(got.Steps) != 1 || got.Steps[0].RuleName != "[前置拦截器拦截]" {
		t.Fatalf("steps = %+v, want 一步 [前置拦截器拦截]", got.Steps)
	}
	if !got.Verdict.Blocked || got.Verdict.Action != store.RuleActionNotfound {
		t.Fatalf("verdict = %+v, want blocked notfound", got.Verdict)
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
