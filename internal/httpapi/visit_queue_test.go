package httpapi

// 访问明细异步队列的单测:不依赖 Postgres,落库函数用内存替身注入。

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"janus/internal/store"
)

// fakeVisitSink 记录每次批量/逐行落库调用;block 非 nil 时落库阻塞到它被关闭。
type fakeVisitSink struct {
	mu        sync.Mutex
	batches   [][]store.VisitRecord
	singles   []store.VisitRecord
	block     chan struct{}
	failBatch atomic.Bool
}

func (f *fakeVisitSink) insertBatch(ctx context.Context, recs []store.VisitRecord) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.failBatch.Load() {
		return errors.New("batch failed")
	}
	cp := append([]store.VisitRecord(nil), recs...)
	f.mu.Lock()
	f.batches = append(f.batches, cp)
	f.mu.Unlock()
	return nil
}

func (f *fakeVisitSink) insertOne(_ context.Context, rec store.VisitRecord) error {
	if rec.LinkID < 0 {
		return errors.New("bad row")
	}
	f.mu.Lock()
	f.singles = append(f.singles, rec)
	f.mu.Unlock()
	return nil
}

func (f *fakeVisitSink) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.singles)
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func closeQueue(t *testing.T, q *VisitQueue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestVisitQueueBatches 积压的明细被攒成批落库,且单批不超过 BatchSize。
func TestVisitQueueBatches(t *testing.T) {
	sink := &fakeVisitSink{block: make(chan struct{})}
	q := newVisitQueue(VisitQueueConfig{Size: 100, Workers: 1, BatchSize: 10},
		sink.insertBatch, sink.insertOne, nil)
	// worker 取到第 1 条后卡在落库上,其余 25 条在队列里积压
	for i := 0; i < 26; i++ {
		if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: int64(i)}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	close(sink.block)
	closeQueue(t, q)

	if got := sink.total(); got != 26 {
		t.Fatalf("落库总数 = %d, want 26", got)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.batches) >= 26 {
		t.Fatalf("批次数 = %d,积压的明细没有被攒批", len(sink.batches))
	}
	for i, b := range sink.batches {
		if len(b) > 10 {
			t.Fatalf("第 %d 批 %d 行,超过 BatchSize=10", i, len(b))
		}
	}
	if q.Inserted() != 26 || q.Dropped() != 0 || q.Failed() != 0 {
		t.Fatalf("计数 inserted/dropped/failed = %d/%d/%d", q.Inserted(), q.Dropped(), q.Failed())
	}
}

// TestVisitQueueDropsWhenFull 队列满时立即返回 ErrVisitQueueFull 并计数,绝不阻塞调用方。
func TestVisitQueueDropsWhenFull(t *testing.T) {
	sink := &fakeVisitSink{block: make(chan struct{})}
	q := newVisitQueue(VisitQueueConfig{Size: 2, Workers: 1, BatchSize: 1},
		sink.insertBatch, sink.insertOne, nil)

	// 第 1 条被 worker 取走并卡住;等它离开 channel 再填满容量,保证计数确定
	if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: 1}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(q.ch) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("worker 未取走第一条明细")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 2; i <= 3; i++ {
		if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: int64(i)}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	start := time.Now()
	var fullErrs int
	for i := 0; i < 5; i++ {
		if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: 100}); errors.Is(err, ErrVisitQueueFull) {
			fullErrs++
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("队列满时 Enqueue 阻塞了 %v", elapsed)
	}
	if fullErrs != 5 || q.Dropped() != 5 {
		t.Fatalf("满队列返回 Full %d 次 / Dropped=%d, want 5/5", fullErrs, q.Dropped())
	}

	close(sink.block)
	closeQueue(t, q)
	if got := sink.total(); got != 3 {
		t.Fatalf("落库总数 = %d, want 3(被丢弃的不落库)", got)
	}
}

// TestVisitQueueDrainsOnClose Close 等队列里的全部明细落库(并触发回调)后才返回;
// 关闭后入队返回 ErrVisitQueueClosed,重复 Close 不 panic。
func TestVisitQueueDrainsOnClose(t *testing.T) {
	sink := &fakeVisitSink{}
	var after atomic.Int64
	q := newVisitQueue(VisitQueueConfig{Size: 1000, Workers: 3, BatchSize: 7},
		func(ctx context.Context, recs []store.VisitRecord) error {
			time.Sleep(time.Millisecond) // 模拟落库耗时,让 Close 时队列里确有积压
			return sink.insertBatch(ctx, recs)
		}, sink.insertOne, func(visitJob) { after.Add(1) })

	for i := 0; i < 500; i++ {
		if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: int64(i)}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	closeQueue(t, q)

	if got := sink.total(); got != 500 {
		t.Fatalf("Close 返回时落库 %d 条, want 500", got)
	}
	if got := after.Load(); got != 500 {
		t.Fatalf("落库后回调 %d 次, want 500", got)
	}
	if err := q.Enqueue(context.Background(), nil, store.VisitRecord{}); !errors.Is(err, ErrVisitQueueClosed) {
		t.Fatalf("关闭后 Enqueue = %v, want ErrVisitQueueClosed", err)
	}
	closeQueue(t, q) // 幂等
}

// TestVisitQueueRetriesRowsOnBatchFailure 整批失败后逐行重试,坏行只丢它自己。
func TestVisitQueueRetriesRowsOnBatchFailure(t *testing.T) {
	sink := &fakeVisitSink{}
	sink.failBatch.Store(true)
	sink.block = make(chan struct{})
	q := newVisitQueue(VisitQueueConfig{Size: 10, Workers: 1, BatchSize: 10},
		sink.insertBatch, sink.insertOne, nil)
	for _, id := range []int64{1, -1, 2} {
		if err := q.Enqueue(context.Background(), nil, store.VisitRecord{LinkID: id}); err != nil {
			t.Fatal(err)
		}
	}
	close(sink.block)
	closeQueue(t, q)
	if q.Inserted() != 2 || q.Failed() != 1 {
		t.Fatalf("inserted/failed = %d/%d, want 2/1", q.Inserted(), q.Failed())
	}
}

// TestVisitQueueDetachesRequestCancel 请求 ctx 被取消(访客断开)不影响落库;
// 落库用的是队列自己的超时 ctx。
func TestVisitQueueDetachesRequestCancel(t *testing.T) {
	var gotErr atomic.Value
	done := make(chan struct{})
	q := newVisitQueue(VisitQueueConfig{Size: 1, Workers: 1, BatchSize: 1},
		func(ctx context.Context, _ []store.VisitRecord) error {
			gotErr.Store(ctx.Err() == nil)
			close(done)
			return nil
		}, nil, func(j visitJob) {
			if j.ctx.Err() != nil {
				t.Errorf("回调拿到的 ctx 已被取消: %v", j.ctx.Err())
			}
		})
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // 访客已断开
	if err := q.Enqueue(reqCtx, nil, store.VisitRecord{LinkID: 1}); err != nil {
		t.Fatal(err)
	}
	<-done
	closeQueue(t, q)
	if ok, _ := gotErr.Load().(bool); !ok {
		t.Fatal("落库 ctx 不应继承请求的取消")
	}
}
