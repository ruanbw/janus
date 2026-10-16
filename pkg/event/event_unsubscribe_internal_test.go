package event

import (
	"context"
	"testing"
	"time"
)

// TestUnsubscribeStopsDelivery 钉住按 ID 解绑的契约。
func TestUnsubscribeStopsDelivery(t *testing.T) {
	Reset()
	defer Reset()

	var count int
	delivered := make(chan struct{}, 4)
	id := SubscribeID("unsub.topic", func(ctx context.Context, e Event) {
		count++
		select {
		case delivered <- struct{}{}:
		default:
		}
	})
	if id == 0 {
		t.Fatal("SubscribeID 应返回非零 ID")
	}

	Publish(context.Background(), simpleEventType{topic: "unsub.topic"})
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("解绑前应收到事件")
	}

	if !Unsubscribe("unsub.topic", id) {
		t.Fatal("Unsubscribe 应命中")
	}
	if Unsubscribe("unsub.topic", id) {
		t.Fatal("重复 Unsubscribe 不应再次命中")
	}

	Publish(context.Background(), simpleEventType{topic: "unsub.topic"})
	time.Sleep(150 * time.Millisecond)
	if count != 1 {
		t.Fatalf("解绑后仍收到事件: count=%d, want 1", count)
	}
}
