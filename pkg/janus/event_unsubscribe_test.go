package janus_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"janus/pkg/event"
	"janus/pkg/janus"
)

// TestEventListenerUnsubscribedOnShutdown 钉住「App 销毁即解绑全局事件订阅」契约:
// 否则实例销毁后订阅仍挂在全局事件总线上,多实例互相串扰、重复触发。
func TestEventListenerUnsubscribedOnShutdown(t *testing.T) {
	event.Reset()
	t.Cleanup(event.Reset)

	var count int32
	delivered := make(chan struct{}, 4)

	app, err := janus.New(
		janus.WithConfig(janus.Config{Addr: "127.0.0.1:0"}),
		janus.WithEventListener("unsub.event", func(ctx context.Context, e janus.Event) {
			atomic.AddInt32(&count, 1)
			select {
			case delivered <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("构造 App 失败: %v", err)
	}

	ctx := context.Background()
	janus.PublishEvent(ctx, janus.NewSimpleEvent("unsub.event", "first"))

	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("Shutdown 前应收到事件")
	}

	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Shutdown 后订阅应已解绑,再发布不应再触发。
	janus.PublishEvent(ctx, janus.NewSimpleEvent("unsub.event", "second"))
	time.Sleep(200 * time.Millisecond)

	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("Shutdown 后订阅未解绑: 期望只触发 1 次,实际 %d 次", got)
	}
}
