package event_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"janus/pkg/event"
)

type simpleEvent struct {
	topic string
	data  any
	at    time.Time
}

func (s simpleEvent) Topic() string         { return s.topic }
func (s simpleEvent) Payload() any          { return s.data }
func (s simpleEvent) OccurredAt() time.Time { return s.at }

func TestEventBusContracts(t *testing.T) {
	t.Run("Given 订阅特定 topic When 发布该 topic 事件 Then 订阅者成功接收并处理", func(t *testing.T) {
		event.Reset()
		defer event.Reset()

		var received atomic.Pointer[string]
		done := make(chan struct{})

		event.Subscribe("link.created", func(ctx context.Context, e event.Event) {
			if str, ok := e.Payload().(string); ok {
				received.Store(&str)
			}
			close(done)
		})

		evt := simpleEvent{
			topic: "link.created",
			data:  "test-link-123",
			at:    time.Now(),
		}
		event.Publish(context.Background(), evt)

		select {
		case <-done:
			val := received.Load()
			if val == nil || *val != "test-link-123" {
				t.Fatalf("expected payload 'test-link-123', got %v", val)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event delivery")
		}
	})

	t.Run("Given 多个订阅者且首个订阅者发生 panic When 发布事件 Then Fail-Open 隔离且后续订阅者继续接收", func(t *testing.T) {
		event.Reset()
		defer event.Reset()

		var secondRan atomic.Bool
		done := make(chan struct{})

		// 异常订阅者
		event.Subscribe("link.deleted", func(ctx context.Context, e event.Event) {
			panic("boom handler panic")
		})

		// 正常订阅者
		event.Subscribe("link.deleted", func(ctx context.Context, e event.Event) {
			secondRan.Store(true)
			close(done)
		})

		event.Publish(context.Background(), simpleEvent{
			topic: "link.deleted",
			data:  int64(999),
			at:    time.Now(),
		})

		select {
		case <-done:
			if !secondRan.Load() {
				t.Fatal("expected second subscriber to execute despite first subscriber panic")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for second subscriber")
		}
	})

	t.Run("Given 重置事件总线 When 发布事件 Then 不再触发任何历史订阅者", func(t *testing.T) {
		event.Reset()

		var executed atomic.Bool
		event.Subscribe("domain.verified", func(ctx context.Context, e event.Event) {
			executed.Store(true)
		})

		event.Reset()

		event.Publish(context.Background(), simpleEvent{
			topic: "domain.verified",
			data:  "example.com",
			at:    time.Now(),
		})

		time.Sleep(50 * time.Millisecond)
		if executed.Load() {
			t.Fatal("expected no handlers to run after Reset()")
		}
	})

	t.Run("Given 并发订阅与发布 When 多个协程操作 Then 保持并发安全", func(t *testing.T) {
		event.Reset()
		defer event.Reset()

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				event.Subscribe("concurrent.test", func(ctx context.Context, e event.Event) {})
			}()
		}

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				event.Publish(context.Background(), simpleEvent{
					topic: "concurrent.test",
					data:  "concurrent-data",
					at:    time.Now(),
				})
			}()
		}

		wg.Wait()
	})
}
