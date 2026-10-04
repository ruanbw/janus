package httpapi

import (
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/gin-gonic/gin"

	"janus/internal/rules"
	"janus/internal/store"
)

// DeliveryContext 传递给交付处理器的上下文信息。
type DeliveryContext struct {
	Link     *store.Link
	Domain   *store.Domain
	Decision rules.Decision
	Target   string
}

// ActionHandler 执行具体的 HTTP 内容交付（如重定向、渲染错误页或自定义响应）。
// 返回 true 表示本次响应已被终结并交付；返回 false 表示由基座按默认逻辑处理。
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
//
// panic 隔离（fail-open）：处理器是外部注入的代码，panic 不允许冒到访客热路径。
// 退化方式取决于「响应写出去了没有」：
//   - 还没写：返回 false 交回基座原生分支，由基座自己交付这次响应；
//   - 已经写了：只能当成已终结。再让基座写一次会把半截响应拼成两段
//     （"插件的前半段" + "基座的后半段"），访客拿到一个两边都不是的页面。
func ExecuteActionHandler(c *gin.Context, action string, dctx DeliveryContext) (handled bool) {
	actionHandlersLock.RLock()
	handler, ok := actionHandlers[action]
	actionHandlersLock.RUnlock()
	// 不需要再判 handler == nil：RegisterActionHandler 是这张表的唯一写入口，
	// 而它拒绝空 action 与 nil handler，Reset 只把表清空。
	if !ok {
		return false
	}
	defer func() {
		if rec := recover(); rec != nil {
			handled = c.Writer.Written()
			slog.Error("交付动作处理器 panic,退化为基座默认交付(fail-open)",
				"action", action, "handled", handled, "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	return handler(c, dctx)
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-04T13:30:45+08:00","module_hash":"123fc272486c87e72fcf5512dd2d0b5723f6c6fc35b5de2683e5d98604a50c23","functions":[{"id":"func/RegisterActionHandler","name":"RegisterActionHandler","line":32,"end_line":39,"hash":"f239103474e42afc540a3bbe13c753c49cd5641bd779fc18da29771dae41006a"},{"id":"func/ResetActionHandlers","name":"ResetActionHandlers","line":42,"end_line":46,"hash":"f732af033f5bc12b20e27ccd7eae5816f54ae765f74a138eafb5dcb7a4e861dc"},{"id":"func/ExecuteActionHandler","name":"ExecuteActionHandler","line":55,"end_line":72,"hash":"ac39909ddb159856962065717874f4d586393545ce98afb08de4c66c6700be7a"}]}
// mutate4go-manifest-end
