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
	listeners = make(map[string][]subscription)
	nextSubID atomic.Uint64
)

// subscription 一个已注册监听器:持有订阅 ID 以便解绑(Go 的函数值不可比较,
// 无法按 handler 反查)。
type subscription struct {
	id      uint64
	handler Handler
}

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

// Subscribe 注册事件监听器。保留原签名;需要后续解绑时用 SubscribeID。
func Subscribe(topic string, handler Handler) {
	_ = SubscribeID(topic, handler)
}

// SubscribeID 注册事件监听器并返回订阅 ID(非零),可与 Unsubscribe 配对解绑。
// topic 为空或 handler 为 nil 时返回 0(未注册)。
func SubscribeID(topic string, handler Handler) uint64 {
	if topic == "" || handler == nil {
		return 0
	}
	id := nextSubID.Add(1)
	busLock.Lock()
	defer busLock.Unlock()
	listeners[topic] = append(listeners[topic], subscription{id: id, handler: handler})
	return id
}

// Unsubscribe 按 (topic, id) 解绑先前 SubscribeID 注册的监听器,返回是否命中。
// 用于避免全局单例订阅在实例销毁后继续被触发(多实例互相串扰/重复注册)。
func Unsubscribe(topic string, id uint64) bool {
	if topic == "" || id == 0 {
		return false
	}
	busLock.Lock()
	defer busLock.Unlock()
	list := listeners[topic]
	for i, s := range list {
		if s.id == id {
			listeners[topic] = append(list[:i:i], list[i+1:]...)
			if len(listeners[topic]) == 0 {
				delete(listeners, topic)
			}
			return true
		}
	}
	return false
}

// Reset 清空所有订阅者与派发计数（测试隔离用）
func Reset() {
	busLock.Lock()
	defer busLock.Unlock()
	listeners = make(map[string][]subscription)
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
	subs := listeners[e.Topic()]
	if len(subs) == 0 {
		busLock.RUnlock()
		return
	}
	// 拷贝一份订阅切片，避免长持读锁或遍历时修改
	copied := make([]subscription, len(subs))
	copy(copied, subs)
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
		for _, s := range copied {
			runHandlerSafely(handlerCtx, e, s.handler)
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
