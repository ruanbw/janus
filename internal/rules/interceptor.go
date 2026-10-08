package rules

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
)

// RuleInterceptor 允许在遍历规则快照之前短路求值逻辑，直接给出最终决议。
// 典型应用场景：
// 1. 外部状态驱动的全局放行（如某条短链在外部被临时挂起维护窗口）；
// 2. 外部状态驱动的全局阻断（如风控侧的紧急熔断开关）。
type RuleInterceptor interface {
	Name() string
	// Intercept 返回 (*Decision, true) 表示短路生效，系统将跳过后续规则匹配直接采用该决议；
	// 返回 (nil, false) 则表示放行，继续进行后续规则求值。
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

// interceptorSnapshot 取注册表的快照副本（读锁只用于拷贝，不跨插件调用：
// 插件在自己的 Intercept 里反过来注册/重置插件时不会自锁）。
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

// checkOne 执行单个拦截器。
// panic 被当作「这个拦截器没拦住」：继续问后面的拦截器，全都没拦住就交回基座求值。
// 不能让 panic 冒到访客热路径 —— 冒出去只会被全局中间件兜成 500，
// 而「插件失灵」的正确定义是「它这一票不算」，不是「整次访问失败」。
func checkOne(i RuleInterceptor, ctx context.Context, r *http.Request, linkID int64, fact *Fact) (d *Decision, ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			d, ok = nil, false
			slog.Error("规则拦截器 panic,按未拦截处理(fail-open)",
				"interceptor", i.Name(), "panic", rec, "stack", string(debug.Stack()))
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
	return checkPluginInterceptors(ctx, r, linkID, fact)
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-04T13:24:34+08:00","module_hash":"7112c51c1cf30f1220c6301b50c50baf0919e619a89bc3b8d79e3fac2ffe0115","functions":[{"id":"func/RegisterInterceptor","name":"RegisterInterceptor","line":28,"end_line":35,"hash":"972b9f1c267da849dead05fc1319d2de5c79962fa810ccd2b2ce5679c6f95ae5"},{"id":"func/ResetInterceptors","name":"ResetInterceptors","line":38,"end_line":42,"hash":"6055c4d511392d8431fffc2ff9f04c2f0ed4389d2a09efd19301b1635f188b6b"},{"id":"func/interceptorSnapshot","name":"interceptorSnapshot","line":46,"end_line":55,"hash":"a1c5eb6aa10b35e8d172c87c633cbd3a212432a26f38c0fec1f9905dbb82e8f5"},{"id":"func/checkOne","name":"checkOne","line":61,"end_line":70,"hash":"ba5e381eaba91bf50bcbfcd7e2d746fefb09a796232b6b071831fea864ac5be8"},{"id":"func/CheckInterceptors","name":"CheckInterceptors","line":73,"end_line":80,"hash":"17f1ab7bcf7a989cb8da4122b9bf0238f1a2afc9089e65c687ac1920271e92e2"}]}
// mutate4go-manifest-end
