package event

import (
	"context"
	"testing"
	"time"
)

// TestPublishBoundedDispatch 钉住「有界派发」契约(包内测试:可下调并发上限)。
func TestPublishBoundedDispatch(t *testing.T) {
	Reset()
	defer Reset()
	prevLimit := maxConcurrentDispatches.Load()
	maxConcurrentDispatches.Store(2)
	defer maxConcurrentDispatches.Store(prevLimit)

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	Subscribe("bounded.topic", func(ctx context.Context, e Event) {
		started <- struct{}{}
		<-release
	})

	// 前两次派发占满槽位(handler 阻塞)。
	for i := 0; i < 2; i++ {
		Publish(context.Background(), simpleEventType{topic: "bounded.topic"})
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("handler 未如期启动")
		}
	}

	// 第三次派发:槽位已满,必须被丢弃而非再起 goroutine 或阻塞调用方。
	done := make(chan struct{})
	go func() {
		Publish(context.Background(), simpleEventType{topic: "bounded.topic"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("派发在槽位已满时阻塞了发布方(必须非阻塞丢弃)")
	}

	if got := DroppedDispatches(); got != 1 {
		t.Fatalf("DroppedDispatches() = %d, want 1", got)
	}

	close(release)
	// 等在途清零,避免污染后续用例。
	deadline := time.Now().Add(2 * time.Second)
	for inFlightDispatches.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

// TestPublishHandlerContextDecoupled 钉住 ctx 语义:发布方 ctx 取消不打断异步订阅者,
// 且订阅者 ctx 带固定超时。
func TestPublishHandlerContextDecoupled(t *testing.T) {
	Reset()
	defer Reset()

	type result struct {
		err      error
		hasLimit bool
	}
	got := make(chan result, 1)
	Subscribe("ctx.topic", func(ctx context.Context, e Event) {
		time.Sleep(100 * time.Millisecond)
		_, hasDeadline := ctx.Deadline()
		got <- result{err: ctx.Err(), hasLimit: hasDeadline}
	})

	parent, cancel := context.WithCancel(context.Background())
	Publish(parent, simpleEventType{topic: "ctx.topic"})
	cancel()

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("订阅者 ctx 不应随发布方 ctx 取消,got %v", r.err)
		}
		if !r.hasLimit {
			t.Fatal("订阅者 ctx 应带固定超时(Dedline)")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("订阅者未在超时内执行")
	}
}

type simpleEventType struct{ topic string }

func (s simpleEventType) Topic() string         { return s.topic }
func (s simpleEventType) Payload() any          { return nil }
func (s simpleEventType) OccurredAt() time.Time { return time.Now() }
