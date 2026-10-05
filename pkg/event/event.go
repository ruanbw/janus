package event

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Event 领域事件公开接口
type Event interface {
	Topic() string
	Payload() any
	OccurredAt() time.Time
}

// Handler 事件订阅者函数签名
type Handler func(ctx context.Context, e Event)

var (
	busLock   sync.RWMutex
	listeners = make(map[string][]Handler)
)

// Subscribe 注册事件监听器
func Subscribe(topic string, handler Handler) {
	if topic == "" || handler == nil {
		return
	}
	busLock.Lock()
	defer busLock.Unlock()
	listeners[topic] = append(listeners[topic], handler)
}

// Reset 清空所有订阅者（测试隔离用）
func Reset() {
	busLock.Lock()
	defer busLock.Unlock()
	listeners = make(map[string][]Handler)
}

// Publish 异步派发事件至对应主题的所有订阅者，严格遵守 Fail-Open 隔离铁律
func Publish(ctx context.Context, e Event) {
	if e == nil || e.Topic() == "" {
		return
	}

	busLock.RLock()
	handlers := listeners[e.Topic()]
	if len(handlers) == 0 {
		busLock.RUnlock()
		return
	}
	// 拷贝一份 handler 切片，避免长持读锁或遍历时修改
	copied := make([]Handler, len(handlers))
	copy(copied, handlers)
	busLock.RUnlock()

	// 异步派发，保证不拖慢主调用方热路径/主业务线程
	go func() {
		for _, h := range copied {
			runHandlerSafely(ctx, e, h)
		}
	}()
}

// runHandlerSafely 单独捕获每个订阅者的 panic，实现 Fail-Open
func runHandlerSafely(ctx context.Context, e Event, h Handler) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("事件订阅者执行异常(Fail-Open)",
				"topic", e.Topic(),
				"panic", rec,
				"stack", string(debug.Stack()),
			)
		}
	}()
	h(ctx, e)
}
