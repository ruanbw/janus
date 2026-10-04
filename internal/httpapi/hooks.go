package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
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
			runHook(h, ctx, r, visit)
		}
	}()
}

// runHook 执行单个后置钩子。
// 这里必须 recover：钩子跑在请求链之外的 goroutine 里，一次 panic 会直接带走整个进程
// （连带同一循环里后面的钩子一起丢掉），而它做的事本来就不该影响任何访客的响应。
func runHook(h PostVisitHook, ctx context.Context, r *http.Request, visit store.VisitRecord) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("后置访问钩子 panic,已忽略(fail-open)",
				"panic", rec, "stack", string(debug.Stack()))
		}
	}()
	h(ctx, r, visit)
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-04T13:33:43+08:00","module_hash":"164a329a98d321fd2f11e276bdb7d07dcbc31f99d7f4d171acd2c90399104c04","functions":[{"id":"func/RegisterPostVisitHook","name":"RegisterPostVisitHook","line":23,"end_line":30,"hash":"623a904d0ffa374c5fa4e72333510b488d1d78dd17cb95845ac8014126eaaf69"},{"id":"func/ResetPostVisitHooks","name":"ResetPostVisitHooks","line":33,"end_line":37,"hash":"a060d8bb92b379b589f8fde92fad232d707d35cf0b329178bec1aaaa34e6a068"},{"id":"func/TriggerPostVisitHooks","name":"TriggerPostVisitHooks","line":40,"end_line":56,"hash":"6093b4ff53c4e12b5eb1ef0531b0aacd27144dc3242654aee6a31ee4d5a4a79e"},{"id":"func/runHook","name":"runHook","line":61,"end_line":69,"hash":"80a626dcf5e50ba544b03e76be3a740a70214e40b5128d8d08c4dbb8cd84ee6c"}]}
// mutate4go-manifest-end
