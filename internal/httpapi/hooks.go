package httpapi

import (
	"context"
	"net/http"
	"sync"

	"janus/internal/store"
)

// PostVisitHook 访问日志持久化完成后的异步回调。
// 必须严格遵守：异步非阻塞，严禁拖慢访客跳转热路径。
type PostVisitHook func(ctx context.Context, r *http.Request, visit store.VisitRecord)

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
func TriggerPostVisitHooks(ctx context.Context, r *http.Request, visit store.VisitRecord) {
	postHooksLock.RLock()
	if len(postHooks) == 0 {
		postHooksLock.RUnlock()
		return
	}
	list := make([]PostVisitHook, len(postHooks))
	copy(list, postHooks)
	postHooksLock.RUnlock()

	// 必须异步执行，绝对不拖慢访客跳转热路径
	go func() {
		for _, h := range list {
			h(ctx, r, visit)
		}
	}()
}
