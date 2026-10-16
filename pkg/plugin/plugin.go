package plugin

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// PanicCounts 各扩展点触发 Fail-Open(panic 被吞)的累计次数,供运维观测。
// 任何一项持续增长都意味着某个外部插件在静默失效,应尽快定位。
type PanicCounts struct {
	Enricher      uint64
	Interceptor   uint64
	ActionHandler uint64
	SDKInjector   uint64
	PostVisitHook uint64
}

var (
	panicsEnricher      atomic.Uint64
	panicsInterceptor   atomic.Uint64
	panicsActionHandler atomic.Uint64
	panicsSDKInjector   atomic.Uint64
	panicsPostHook      atomic.Uint64
)

// PanicStatistics 快照各扩展点的 Fail-Open 累计次数(观测用)。
func PanicStatistics() PanicCounts {
	return PanicCounts{
		Enricher:      panicsEnricher.Load(),
		Interceptor:   panicsInterceptor.Load(),
		ActionHandler: panicsActionHandler.Load(),
		SDKInjector:   panicsSDKInjector.Load(),
		PostVisitHook: panicsPostHook.Load(),
	}
}

// Fact 一次访问的访客画像公开 DTO。
// 字段语义与基座规则求值画像对齐，支持外部 Enricher 进行扩展富化。
type Fact struct {
	IP      string
	IPAttr  string
	Country string
	ASN     string
	Lang    string
	Ref     string
	UTM     string
	UA      string
	DevType string
	OS      string
	Browser string
	Path    string
	Domain  string
}

// Decision 一次求值的裁决结果公开 DTO。
type Decision struct {
	RuleID      int64
	Name        string
	Action      string // pass / redirect / notfound / throttle / 自定义 action
	Destination string
	Priority    int
	PageMode    string
	CustomHTML  string
}

// DeliveryContext 传递给交付处理器的上下文信息。
type DeliveryContext struct {
	Target   string
	Decision Decision
}

// VisitRecord 一次访问/点击的明细记录公开 DTO。
type VisitRecord struct {
	LinkID     int64
	DomainID   int64
	IP         string
	UserAgent  string
	Referer    string
	Action     string
	Outcome    string
	Reason     string
	TargetURL  string
	Lang       string
	Country    string
	RuleID     *int64
	RuleAction string
}

// =========================================================================
// 扩展点 1: FactEnricher
// =========================================================================

// FactEnricher 用于在规则求值前，对基础访客画像进行扩展富化。
// 必须严格遵守：纯内存或本地离线库计算，耗时必须在微秒级，严禁引入数据库查询。
type FactEnricher interface {
	Name() string
	Enrich(ctx context.Context, r *http.Request, fact *Fact)
}

var (
	enrichersLock sync.RWMutex
	enrichers     []FactEnricher
)

// RegisterEnricher 注册外部画像富化器。
func RegisterEnricher(e FactEnricher) {
	if e == nil {
		return
	}
	enrichersLock.Lock()
	defer enrichersLock.Unlock()
	enrichers = append(enrichers, e)
}

// ResetEnrichers 清空已注册的富化器（测试隔离用）。
func ResetEnrichers() {
	enrichersLock.Lock()
	defer enrichersLock.Unlock()
	enrichers = nil
}

func enricherSnapshot() []FactEnricher {
	enrichersLock.RLock()
	defer enrichersLock.RUnlock()
	if len(enrichers) == 0 {
		return nil
	}
	list := make([]FactEnricher, len(enrichers))
	copy(list, enrichers)
	return list
}

func enrichOne(e FactEnricher, ctx context.Context, r *http.Request, fact *Fact) {
	defer func() {
		if rec := recover(); rec != nil {
			n := panicsEnricher.Add(1)
			slog.Error("画像富化器 panic,按未富化处理(fail-open)",
				"enricher", e.Name(), "panic", rec, "panics_total", n, "stack", string(debug.Stack()))
		}
	}()
	e.Enrich(ctx, r, fact)
}

// ApplyEnrichers 依次执行已注册的富化器。
func ApplyEnrichers(ctx context.Context, r *http.Request, fact *Fact) {
	for _, e := range enricherSnapshot() {
		enrichOne(e, ctx, r, fact)
	}
}

// HasASNProvider 判断是否已注册了能够提供 ASN 数据的画像富化器。
func HasASNProvider() bool {
	enrichersLock.RLock()
	defer enrichersLock.RUnlock()
	for _, e := range enrichers {
		if e.Name() == "asn" || e.Name() == "asn_enricher" {
			return true
		}
	}
	return false
}

// =========================================================================
// 扩展点 2: RuleInterceptor
// =========================================================================

// RuleInterceptor 允许在遍历规则快照之前短路求值逻辑，直接给出最终决议。
//
// Fail-Open 契约(实现者必读):Intercept panic 时基座按"未拦截"处理并继续下一个
// 拦截器/常规规则求值 —— 即放行。对**安全型**拦截器(应阻断可疑流量者)而言,
// panic 等于静默失效,不可依赖 panic 表达阻断;必须用返回 (*Decision, true) 显式
// 给出 block/redirect 决议。观测型/增强型拦截器可安全享受 Fail-Open。
// 每次 Fail-Open 都会递增 PanicStatistics().Interceptor 并打结构化日志。
type RuleInterceptor interface {
	Name() string
	Intercept(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool)
}

var (
	interceptorsLock sync.RWMutex
	interceptors     []RuleInterceptor
)

// RegisterInterceptor 注册前置规则拦截器。
func RegisterInterceptor(i RuleInterceptor) {
	if i == nil {
		return
	}
	interceptorsLock.Lock()
	defer interceptorsLock.Unlock()
	interceptors = append(interceptors, i)
}

// ResetInterceptors 清空已注册的拦截器（测试隔离用）。
func ResetInterceptors() {
	interceptorsLock.Lock()
	defer interceptorsLock.Unlock()
	interceptors = nil
}

func interceptorSnapshot() []RuleInterceptor {
	interceptorsLock.RLock()
	defer interceptorsLock.RUnlock()
	if len(interceptors) == 0 {
		return nil
	}
	list := make([]RuleInterceptor, len(interceptors))
	copy(list, interceptors)
	return list
}

func checkOne(i RuleInterceptor, ctx context.Context, r *http.Request, linkID int64, fact *Fact) (d *Decision, ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			d, ok = nil, false
			n := panicsInterceptor.Add(1)
			slog.Error("规则拦截器 panic,按未拦截处理(fail-open)",
				"interceptor", i.Name(), "panic", rec, "panics_total", n, "stack", string(debug.Stack()))
		}
	}()
	return i.Intercept(ctx, r, linkID, fact)
}

// CheckInterceptors 依次执行前置拦截器，首个命中的拦截器将短路后续流程。
func CheckInterceptors(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	for _, i := range interceptorSnapshot() {
		if d, ok := checkOne(i, ctx, r, linkID, fact); ok && d != nil {
			return d, true
		}
	}
	return nil, false
}

// =========================================================================
// 扩展点 3: ActionHandler
// =========================================================================

// ActionHandler 执行具体的 HTTP 内容交付。
// 返回 true 表示本次响应已被终结并交付；返回 false 表示由基座按默认逻辑处理。
//
// Fail-Open 契约:handler panic 时基座按"未处理"处理(若响应尚未写入则返回 false,
// 交回基座默认交付;若已写入部分响应则返回 true 终结)。每次 Fail-Open 递增
// PanicStatistics().ActionHandler 并打结构化日志。
type ActionHandler func(c *gin.Context, dctx DeliveryContext) bool

var (
	actionHandlersLock sync.RWMutex
	actionHandlers     = make(map[string]ActionHandler)
)

// RegisterActionHandler 允许外部注册新型交付动作处理器。
func RegisterActionHandler(action string, handler ActionHandler) {
	if action == "" || handler == nil {
		return
	}
	actionHandlersLock.Lock()
	defer actionHandlersLock.Unlock()
	actionHandlers[action] = handler
}

// ResetActionHandlers 清空自定义交付动作处理器（测试隔离用）。
func ResetActionHandlers() {
	actionHandlersLock.Lock()
	defer actionHandlersLock.Unlock()
	actionHandlers = make(map[string]ActionHandler)
}

// ExecuteActionHandler 尝试调用外部注册的交付动作处理器。
func ExecuteActionHandler(c *gin.Context, action string, dctx DeliveryContext) (handled bool) {
	actionHandlersLock.RLock()
	handler, ok := actionHandlers[action]
	actionHandlersLock.RUnlock()
	if !ok {
		return false
	}
	defer func() {
		if rec := recover(); rec != nil {
			handled = c.Writer.Written()
			n := panicsActionHandler.Add(1)
			slog.Error("交付动作处理器 panic,退化为基座默认交付(fail-open)",
				"action", action, "handled", handled, "panic", rec, "panics_total", n, "stack", string(debug.Stack()))
		}
	}()
	return handler(c, dctx)
}

// =========================================================================
// 扩展点 4: SDKInjector
// =========================================================================

// SDKInjector 允许对基座原生生成的 sdk.js 进行包装与增强。
type SDKInjector interface {
	Name() string
	WrapSDK(domain string, code string, rawSDK string) string
}

var (
	sdkInjectorLock   sync.RWMutex
	activeSDKInjector SDKInjector
)

// SetSDKInjector 设置活跃的 SDK 注入器。
func SetSDKInjector(injector SDKInjector) {
	sdkInjectorLock.Lock()
	defer sdkInjectorLock.Unlock()
	activeSDKInjector = injector
}

// ResetSDKInjector 重置 SDK 注入器（测试隔离用）。
func ResetSDKInjector() {
	sdkInjectorLock.Lock()
	defer sdkInjectorLock.Unlock()
	activeSDKInjector = nil
}

// WrapGeneratedSDK 执行包装逻辑，未配置注入器（或注入器 panic）时原样返回基座 SDK。
func WrapGeneratedSDK(domain, code string, rawSDK string) (out string) {
	sdkInjectorLock.RLock()
	injector := activeSDKInjector
	sdkInjectorLock.RUnlock()
	if injector == nil {
		return rawSDK
	}
	defer func() {
		if rec := recover(); rec != nil {
			out = rawSDK
			n := panicsSDKInjector.Add(1)
			slog.Error("SDK 注入器 panic,回退基座 SDK(fail-open)",
				"injector", injector.Name(), "panic", rec, "panics_total", n, "stack", string(debug.Stack()))
		}
	}()
	return injector.WrapSDK(domain, code, rawSDK)
}

// =========================================================================
// 扩展点 5: PostVisitHook
// =========================================================================

// PostVisitHook 访问日志持久化完成后的异步回调。
//
// 异步阶段契约(钩子实现者必读):
//   - 调用发生在请求已交付、原始请求生命周期结束之后的 goroutine 中;
//   - ctx 不是请求 ctx:基座用 context.WithoutCancel 断开取消信号并叠加固定超时,
//     因此请求结束/客户端断连不会取消钩子。钩子应尊重 ctx 的 Deadline;
//   - r 是原始 *http.Request 指针。此时连接可能已关闭、Body 可能已读尽,
//     钩子不得再次读取 Body、不得依赖连接级状态;需要的字段请在持久化前拷进 visit;
//   - visit 是只读快照,钩子不得修改。
type PostVisitHook func(ctx context.Context, r *http.Request, visit VisitRecord)

// asyncHookTimeout 异步后置钩子的执行上限:既避免请求级 ctx 取消误伤,
// 也避免个别慢钩子永久占用 goroutine。
const asyncHookTimeout = 30 * time.Second

var (
	postHooksLock sync.RWMutex
	postHooks     []PostVisitHook
)

// RegisterPostVisitHook 注册后置访问钩子。
func RegisterPostVisitHook(hook PostVisitHook) {
	if hook == nil {
		return
	}
	postHooksLock.Lock()
	defer postHooksLock.Unlock()
	postHooks = append(postHooks, hook)
}

// ResetPostVisitHooks 清空已注册的后置钩子（测试隔离用）。
func ResetPostVisitHooks() {
	postHooksLock.Lock()
	defer postHooksLock.Unlock()
	postHooks = nil
}

// TriggerPostVisitHooks 异步触发全部注册的后置钩子。
//
// 基座传入的 ctx 是请求级 ctx:请求交付完成后会被取消。异步阶段必须与它解耦,
// 否则钩子(常见于落库/统计)会在写入完成前被 context canceled 打断,表现为
// "偶发丢数据"。这里用 context.WithoutCancel 断开取消信号,并叠加固定超时护栏。
func TriggerPostVisitHooks(ctx context.Context, r *http.Request, visit VisitRecord) {
	postHooksLock.RLock()
	if len(postHooks) == 0 {
		postHooksLock.RUnlock()
		return
	}
	list := make([]PostVisitHook, len(postHooks))
	copy(list, postHooks)
	postHooksLock.RUnlock()

	go func() {
		hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncHookTimeout)
		defer cancel()
		for _, h := range list {
			runHook(h, hookCtx, r, visit)
		}
	}()
}

func runHook(h PostVisitHook, ctx context.Context, r *http.Request, visit VisitRecord) {
	defer func() {
		if rec := recover(); rec != nil {
			n := panicsPostHook.Add(1)
			slog.Error("后置访问钩子 panic,已忽略(fail-open)",
				"panic", rec, "panics_total", n, "stack", string(debug.Stack()))
		}
	}()
	h(ctx, r, visit)
}

// =========================================================================
// 基座接线辅助:基座在访客热路径上先问一句"有没有注册",没有就不做 DTO 转换,
// 保证未启用插件时零额外分配。
// =========================================================================

// HasEnrichers 是否注册了任何画像富化器。
func HasEnrichers() bool {
	enrichersLock.RLock()
	defer enrichersLock.RUnlock()
	return len(enrichers) > 0
}

// HasInterceptors 是否注册了任何前置拦截器。
func HasInterceptors() bool {
	interceptorsLock.RLock()
	defer interceptorsLock.RUnlock()
	return len(interceptors) > 0
}

// HasPostVisitHooks 是否注册了任何后置访问钩子。
func HasPostVisitHooks() bool {
	postHooksLock.RLock()
	defer postHooksLock.RUnlock()
	return len(postHooks) > 0
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-06T03:55:46+08:00","module_hash":"8b8817c14170b90a317bef915c97014e3264f8f74b14666d5daf247fd4f9858f","functions":[{"id":"func/RegisterEnricher","name":"RegisterEnricher","line":82,"end_line":89,"hash":"b268daf63ed16c0301f88813e1c9017718ceeff71ba9ee95b846163cb8fa823f"},{"id":"func/ResetEnrichers","name":"ResetEnrichers","line":92,"end_line":96,"hash":"05c618acc30c3743a35a34daf75b3ca706ffeba04630d0b31ed4d24fa7a84938"},{"id":"func/enricherSnapshot","name":"enricherSnapshot","line":98,"end_line":107,"hash":"6e96134b9896167c0c675eebf03b8f62485c8ed6019ca5f6c032d4c071a71797"},{"id":"func/enrichOne","name":"enrichOne","line":109,"end_line":117,"hash":"f5f8f2170f2cd16a453782f838abc86b875cd0bd4a8f556e312a37c9df801846"},{"id":"func/ApplyEnrichers","name":"ApplyEnrichers","line":120,"end_line":124,"hash":"33da766c39463d80d87c726c2e940a66e1b505aca525598f1f11e1a9199cf472"},{"id":"func/HasASNProvider","name":"HasASNProvider","line":127,"end_line":136,"hash":"b895f047e9d30ae679a093d8ba45ba79af54be929deb104457e22f2c14fe10dc"},{"id":"func/RegisterInterceptor","name":"RegisterInterceptor","line":154,"end_line":161,"hash":"972b9f1c267da849dead05fc1319d2de5c79962fa810ccd2b2ce5679c6f95ae5"},{"id":"func/ResetInterceptors","name":"ResetInterceptors","line":164,"end_line":168,"hash":"6055c4d511392d8431fffc2ff9f04c2f0ed4389d2a09efd19301b1635f188b6b"},{"id":"func/interceptorSnapshot","name":"interceptorSnapshot","line":170,"end_line":179,"hash":"a1c5eb6aa10b35e8d172c87c633cbd3a212432a26f38c0fec1f9905dbb82e8f5"},{"id":"func/checkOne","name":"checkOne","line":181,"end_line":190,"hash":"ba5e381eaba91bf50bcbfcd7e2d746fefb09a796232b6b071831fea864ac5be8"},{"id":"func/CheckInterceptors","name":"CheckInterceptors","line":193,"end_line":200,"hash":"17f1ab7bcf7a989cb8da4122b9bf0238f1a2afc9089e65c687ac1920271e92e2"},{"id":"func/RegisterActionHandler","name":"RegisterActionHandler","line":216,"end_line":223,"hash":"f239103474e42afc540a3bbe13c753c49cd5641bd779fc18da29771dae41006a"},{"id":"func/ResetActionHandlers","name":"ResetActionHandlers","line":226,"end_line":230,"hash":"f732af033f5bc12b20e27ccd7eae5816f54ae765f74a138eafb5dcb7a4e861dc"},{"id":"func/ExecuteActionHandler","name":"ExecuteActionHandler","line":233,"end_line":248,"hash":"31a003b90e5b01c26a3dee7e83241f06f9191aea865b7d170419895c2e255b85"},{"id":"func/SetSDKInjector","name":"SetSDKInjector","line":266,"end_line":270,"hash":"657711e9f614b3835ed5b6be94aebf9e419581a0c804e0e5cef36c67a3031537"},{"id":"func/ResetSDKInjector","name":"ResetSDKInjector","line":273,"end_line":277,"hash":"d7df9207b23f4f73dbd0f39b5b076e281664b1a29cb3b7d0c5675bf8dc03f63f"},{"id":"func/WrapGeneratedSDK","name":"WrapGeneratedSDK","line":280,"end_line":295,"hash":"148c1ae8e99c188f5c6058012672bc4b8a1af256f5d3a9ff3d3283c786fd837a"},{"id":"func/RegisterPostVisitHook","name":"RegisterPostVisitHook","line":310,"end_line":317,"hash":"623a904d0ffa374c5fa4e72333510b488d1d78dd17cb95845ac8014126eaaf69"},{"id":"func/ResetPostVisitHooks","name":"ResetPostVisitHooks","line":320,"end_line":324,"hash":"a060d8bb92b379b589f8fde92fad232d707d35cf0b329178bec1aaaa34e6a068"},{"id":"func/TriggerPostVisitHooks","name":"TriggerPostVisitHooks","line":327,"end_line":342,"hash":"a438f9eb2cd6bb1066d913e044bf7f0d192669e9fd1dc083fd88f5c7dcc01f65"},{"id":"func/runHook","name":"runHook","line":344,"end_line":352,"hash":"721412eb32372137611017f4fa83d82ba6a8b3d77e45a42d09d58980233960f5"}]}
// mutate4go-manifest-end
