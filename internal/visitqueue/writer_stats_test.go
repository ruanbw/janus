package visitqueue

import (
	"context"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// TestStatsPendingCountsQueueBacklog 固化「队列积压」这个瞬时读数。
//
// 为什么值得单独钉住:Enqueued - Written 只能说明「有多少条没写完」,分不清是
// 正卡在 Sink 里、还是全堆在队列上。前者说明下游 DB 慢,后者说明消费速度跟不上
// 生产速度(再涨就要落到同步直写)。两者的处置完全不同,所以 Pending 必须是
// 队列本身的长度,不能用累计差值近似。
func TestStatsPendingCountsQueueBacklog(t *testing.T) {
	ctx := context.Background()
	release := make(chan struct{})
	inFlight := make(chan struct{}, 1)

	w := New(Options{
		QueueSize:     8,
		BatchSize:     1,
		FlushInterval: time.Hour, // 只靠攒批 flush,定时器不参与本次断言
		Sink: func(context.Context, []store.VisitRecord) error {
			select {
			case inFlight <- struct{}{}:
			default:
			}
			<-release
			return nil
		},
	})

	for _, rec := range visitRecs(1, 3) {
		if err := w.Enqueue(ctx, rec); err != nil {
			t.Fatalf("Enqueue 返回错误 %v", err)
		}
	}

	// 等第一条真的进了 Sink:此时后台 goroutine 卡在里面,后两条只能留在队列上。
	select {
	case <-inFlight:
	case <-time.After(waitTimeout):
		t.Fatal("Sink 一直没被调用:后台 goroutine 没有开始消费")
	}

	got := w.Stats()
	if got.Enqueued != 3 {
		t.Fatalf("Enqueued 应为 3, 得到 %d", got.Enqueued)
	}
	if got.Pending != 2 {
		t.Fatalf("一条正在写、两条在排队, Pending 应为 2, 得到 %d", got.Pending)
	}

	close(release)
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close 返回错误 %v", err)
	}

	got = w.Stats()
	if got.Pending != 0 {
		t.Fatalf("排空之后队列应为空, Pending 得到 %d", got.Pending)
	}
	if got.Written != 3 {
		t.Fatalf("三条都应写入, Written 得到 %d", got.Written)
	}
}
