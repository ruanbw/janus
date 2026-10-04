package httpapi

import (
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

// ActionHandler 执行具体的 HTTP 内容交付（如重定向、渲染错误页或服务端代吐）。
// 返回 true 表示本次响应已被终结并交付；返回 false 表示由基座按默认逻辑处理。
type ActionHandler func(c *gin.Context, dctx DeliveryContext) bool

var (
	actionHandlersLock sync.RWMutex
	actionHandlers     = make(map[string]ActionHandler)
)

// RegisterActionHandler 允许外部注册新型交付动作处理器（例如 "proxy"）。
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
func ExecuteActionHandler(c *gin.Context, action string, dctx DeliveryContext) bool {
	actionHandlersLock.RLock()
	handler, ok := actionHandlers[action]
	actionHandlersLock.RUnlock()
	if !ok || handler == nil {
		return false
	}
	return handler(c, dctx)
}
