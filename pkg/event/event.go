package event

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
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

// 有界派发:每个已发布事件占用一个派发槽,槽满即丢弃并计数(Fail-Open),绝不阻塞
// 发布方热路径,也不无限起 goroutine。同一次派发内,同一 topic 的多个 handler 串行。
const defaultMaxConcurrentDispatches = 256

// handlerTimeout 单个事件派发的固定执行上限。
//
// Publish 的 ctx 语义:Publish 调用方(通常是请求热路径)传入的 ctx 可能随请求结束
// 被取消,而事件处理发生在异步阶段 —— 直接透传会让订阅者在处理中途被 context
// canceled 打断。因此内部用 context.WithoutCancel 断开取消信号,并叠加该固定超时
// 防止订阅者永久挂住。需要请求级数据的订阅者应把数据放进事件 Payload。
const handlerTimeout = 30 * time.Second

var (
	maxConcurrentDispatches atomic.Int64 // 派发并发上限(原子,便于测试下调)
	inFlightDispatches      atomic.Int64 // 当前在途派发数
	droppedDispatches       atomic.Uint64
)

func init() {
	maxConcurrentDispatches.Store(defaultMaxConcurrentDispatches)
}

// Subscribe 注册事件监听器
func Subscribe(topic string, handler Handler) {
	if topic == "" || handler == nil {
		return
	}
	busLock.Lock()
	defer busLock.Unlock()
	listeners[topic] = append(listeners[topic], handler)
}

// Reset 清空所有订阅者与派发计数（测试隔离用）
func Reset() {
	busLock.Lock()
	defer busLock.Unlock()
	listeners = make(map[string][]Handler)
	droppedDispatches.Store(0)
}

// Publish 异步派发事件至对应主题的所有订阅者，严格遵守 Fail-Open 隔离铁律。
//
// 有界语义:派发并发存在上限(见 MaxConcurrentDispatches);已达上限时本次派发被
// 丢弃并计入 DroppedDispatches,同时打一条告警 —— 丢弃优于阻塞发布方(跳转热路径)
// 或无限起 goroutine。ctx 语义见 handlerTimeout 注释。
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

	// 抢占派发槽:非阻塞,抢不到即丢弃。
	limit := maxConcurrentDispatches.Load()
	if inFlightDispatches.Add(1) > limit {
		inFlightDispatches.Add(-1)
		n := droppedDispatches.Add(1)
		slog.Warn("事件派发并发已达上限,丢弃本次派发(Fail-Open)",
			"topic", e.Topic(), "limit", limit, "dropped_total", n)
		return
	}

	// 异步派发，保证不拖慢主调用方热路径/主业务线程
	go func() {
		defer inFlightDispatches.Add(-1)
		// 断开请求级取消信号 + 固定超时(见 handlerTimeout 注释)。
		handlerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), handlerTimeout)
		defer cancel()
		for _, h := range copied {
			runHandlerSafely(handlerCtx, e, h)
		}
	}()
}

// MaxConcurrentDispatches 返回事件派发的并发上限。
func MaxConcurrentDispatches() int { return int(maxConcurrentDispatches.Load()) }

// DroppedDispatches 返回因达到并发上限而被丢弃的派发累计次数(观测用)。
func DroppedDispatches() uint64 { return droppedDispatches.Load() }

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
