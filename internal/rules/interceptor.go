package rules

import (
	"context"
	"net/http"
	"sync"
)

// RuleInterceptor 允许在遍历规则快照之前短路求值逻辑，直接给出最终决议。
// 典型应用场景：
// 1. Safe Mode（审核期模式）：新广告活动前 N 小时强制纯白页面放行；
// 2. Panic Switch（紧急熔断）：风控报警时强制全局拦截。
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

// CheckInterceptors 依次执行前置拦截器，首个命中的拦截器将短路后续流程。
func CheckInterceptors(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	interceptorsLock.RLock()
	if len(interceptors) == 0 {
		interceptorsLock.RUnlock()
		return nil, false
	}
	list := make([]RuleInterceptor, len(interceptors))
	copy(list, interceptors)
	interceptorsLock.RUnlock()

	for _, i := range list {
		if d, ok := i.Intercept(ctx, r, linkID, fact); ok && d != nil {
			return d, true
		}
	}
	return nil, false
}
