package visitqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// waitTimeout 是等 goroutine 的宽限。它是「断言失败」的兜底,不是同步原语:
// 所有正向同步都靠假 Sink 在 channel 上发信号,sleep 只会让用例既慢又不稳。
const waitTimeout = 2 * time.Second

// visitRecs 造 n 条能区分顺序的记录(LinkID 从 start 连续递增)。
// 明细的其它字段与队列无关,断言只需要 FIFO 的 LinkID 序列。
func visitRecs(start int64, n int) []store.VisitRecord {
	recs := make([]store.VisitRecord, n)
	for i := range recs {
		recs[i] = store.VisitRecord{LinkID: start + int64(i)}
	}
	return recs
}

// recordingSink 是假 Sink:记住每次调用拿到的批次,并在 channel 上发信号。
//
// 批次必须拷贝:批量写入的前提就是 Writer 复用同一个 batch 底层数组,
// 直接留住切片会让断言看到随后被覆盖的内容(用例会变成"偶发失败")。
type recordingSink struct {
	mu     sync.Mutex
	calls  [][]store.VisitRecord
	errs   []error // 按调用序号返回的错误;越界视为成功
	called chan struct{}
}

func newRecordingSink() *recordingSink {
	// 缓冲留足所有用例的调用次数;真写满说明用例写错了,
	// 而不是让它悄悄丢掉信号导致断言挂在超时上。
	return &recordingSink{called: make(chan struct{}, 64)}
}

func (s *recordingSink) sink(_ context.Context, batch []store.VisitRecord) error {
	cp := make([]store.VisitRecord, len(batch))
	copy(cp, batch)

	s.mu.Lock()
	idx := len(s.calls)
	s.calls = append(s.calls, cp)
	var err error
	if idx < len(s.errs) {
		err = s.errs[idx]
	}
	s.mu.Unlock()

	s.called <- struct{}{}
	return err
}

func (s *recordingSink) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *recordingSink) callAt(i int) []store.VisitRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.calls) {
		return nil
	}
	return s.calls[i]
}

// sawLinkID 判断某条记录是否被任何一次 Sink 调用收到——「不丢明细」的最终判据。
func (s *recordingSink) sawLinkID(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, batch := range s.calls {
		for _, rec := range batch {
			if rec.LinkID == id {
				return true
			}
		}
	}
	return false
}

// waitCall 等下一次 Sink 调用;超时即判定这一批没有被写出去。
func (s *recordingSink) waitCall(t *testing.T, when string) {
	t.Helper()
	select {
	case <-s.called:
	case <-time.After(waitTimeout):
		t.Fatalf("%s: %v 内没有等到 Sink 调用,记录没有被写出去", when, waitTimeout)
	}
}

// linkIDs 提取一批记录的 LinkID,用来断言 FIFO 顺序。
func linkIDs(batch []store.VisitRecord) []int64 {
	ids := make([]int64, len(batch))
	for i, rec := range batch {
		ids[i] = rec.LinkID
	}
	return ids
}

// assertLinkIDs 比对一次 Sink 调用里的 LinkID 序列(顺序敏感)。
func assertLinkIDs(t *testing.T, batch []store.VisitRecord, want []int64, when string) {
	t.Helper()
	got := linkIDs(batch)
	if len(got) != len(want) {
		t.Fatalf("%s: Sink 收到 %d 条 %v, want %d 条 %v", when, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: 第 %d 条 LinkID = %d, want %d(实际顺序 %v,应为入队顺序)", when, i, got[i], want[i], got)
		}
	}
}

// TestWriterDoesNotWriteBelowBatchSize 断言「攒批」真的在起作用。
//
// 当前 recordVisit 每次跳转都同步 INSERT 一次,这正是要消掉的热路径写延迟。
// 若不到 BatchSize 也照样写,Sink 就还是"一条一次 DB 往返",
// 只是把同步 IO 挪进另一个 goroutine——延迟没降,复杂度白涨。
func TestWriterDoesNotWriteBelowBatchSize(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     10,
		FlushInterval: time.Hour, // 关掉定时刷出:这条用例里只有 Flush 能触发写
	})
	defer func() { _ = w.Close(context.Background()) }()

	for _, rec := range visitRecs(1, 3) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}

	if n := sink.callCount(); n != 0 {
		t.Fatalf("3 条(未到 BatchSize=10)被写成了 %d 批,攒批失效", n)
	}
	if got := w.Stats().Written; got != 0 {
		t.Fatalf("Stats.Written = %d, 还没有任何一条写出去", got)
	}
	if got := w.Stats().Enqueued; got != 3 {
		t.Fatalf("Stats.Enqueued = %d, want 3", got)
	}
}

// TestFlushWritesBufferedRecordsOnceAndInOrder 断言 Flush 是"屏障 + 恰好一次 + FIFO"。
//
// Flush 返回后调用方就认为缓冲已经落库(优雅停机、排障都靠这个语义),
// 所以这里不等 channel,而是直接检查 Sink 已经被调用过、条数一致、顺序等于入队顺序。
// 空缓冲上再 Flush 不能再打一次 Sink:否则每次停机都多一次无意义的 DB 往返。
func TestFlushWritesBufferedRecordsOnceAndInOrder(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     10,
		FlushInterval: time.Hour,
	})
	defer func() { _ = w.Close(context.Background()) }()

	for _, rec := range visitRecs(1, 3) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}

	if n := sink.callCount(); n != 1 {
		t.Fatalf("Flush 后 Sink 被调用 %d 次, want 1 次(3 条要合成一批)", n)
	}
	assertLinkIDs(t, sink.callAt(0), []int64{1, 2, 3}, "Flush 刷出的批次")
	if got := w.Stats().Written; got != 3 {
		t.Fatalf("Stats.Written = %d, want 3", got)
	}

	// 缓冲已空:再 Flush 是空操作
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("空缓冲 Flush 失败: %v", err)
	}
	if n := sink.callCount(); n != 1 {
		t.Fatalf("空缓冲上再 Flush 又调了 %d 次 Sink, want 0 次新调用", n-1)
	}
}

// TestWriterFlushesWhenBatchSizeReached 断言不需要任何人显式 Flush:
// 攒够 BatchSize 这一步本身就要把批次推出去。
//
// 少了这条,明细会一直躺在内存里直到定时器到期,进程崩溃就丢。
func TestWriterFlushesWhenBatchSizeReached(t *testing.T) {
	const batchSize = 5
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     batchSize,
		FlushInterval: time.Hour, // 排除定时刷出:只能由"攒够一批"触发
	})
	defer func() { _ = w.Close(context.Background()) }()

	for _, rec := range visitRecs(1, batchSize) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}

	sink.waitCall(t, "入队达到 BatchSize")
	if n := len(sink.callAt(0)); n != batchSize {
		t.Fatalf("Sink 收到 %d 条, want %d(攒够一批才写)", n, batchSize)
	}
	assertLinkIDs(t, sink.callAt(0), []int64{1, 2, 3, 4, 5}, "攒够一批")
	if n := sink.callCount(); n != 1 {
		t.Fatalf("Sink 被调用 %d 次, want 1 次(一批不能被拆成两次写)", n)
	}
}

// TestWriterFlushesAfterInterval 断言低频场景也不会丢:
// 只有 1 条、远不到 BatchSize,定时器到期也得把它写出去。
//
// 短链的访问是突发的,没有这条,一条孤零零的明细要等到下一次攒够批才落库。
func TestWriterFlushesAfterInterval(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     8,
		BatchSize:     100,
		FlushInterval: 10 * time.Millisecond,
	})
	defer func() { _ = w.Close(context.Background()) }()

	if err := w.Enqueue(context.Background(), store.VisitRecord{LinkID: 7}); err != nil {
		t.Fatalf("入队失败: %v", err)
	}

	sink.waitCall(t, "FlushInterval 到期")
	assertLinkIDs(t, sink.callAt(0), []int64{7}, "定时刷出的批次")
	if n := sink.callCount(); n != 1 {
		t.Fatalf("Sink 被调用 %d 次, want 1 次", n)
	}
}

// TestWriterReportsBatchErrorWithoutRetry 断言失败语义:计数 + 回调 + 不重试。
//
// 访问明细是分析数据,不是账单:一次 DB 抖动就让 Writer 无限重放同一批,
// 会把写放大成风暴、把进程内存拖爆。所以失败即丢弃,只留可观测信号。
// "不重试"这里用正向证据断言:下一批写出的内容是新记录,而不是失败的那两条又来了。
func TestWriterReportsBatchErrorWithoutRetry(t *testing.T) {
	errBoom := errors.New("db 抖了一下:连接被重置")
	sink := newRecordingSink()
	sink.errs = []error{errBoom} // 只有第一批失败

	gotErr := make(chan error, 4)
	gotN := make(chan int, 4)
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     2,
		FlushInterval: time.Hour,
		OnBatchError: func(err error, n int) {
			gotErr <- err
			gotN <- n
		},
	})
	defer func() { _ = w.Close(context.Background()) }()

	for _, rec := range visitRecs(1, 2) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}

	sink.waitCall(t, "第一批攒够并写出")
	select {
	case err := <-gotErr:
		// 回调必须把原始错误透出来,否则运维只能看到"失败了一批"却查不到原因
		if !errors.Is(err, errBoom) {
			t.Fatalf("OnBatchError 收到 %v, want %v", err, errBoom)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Sink 失败后 OnBatchError 没有被调用,失败会变成静默丢数据")
	}
	if n := <-gotN; n != 2 {
		t.Fatalf("OnBatchError 的 n = %d, want 2(该批条数)", n)
	}
	if got := w.Stats().Failed; got != 1 {
		t.Fatalf("Stats.Failed = %d, want 1", got)
	}

	// 再攒一批新的:写出的必须正好是新记录(没有重放上一批)
	for _, rec := range visitRecs(3, 2) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}
	sink.waitCall(t, "第二批攒够并写出")
	assertLinkIDs(t, sink.callAt(1), []int64{3, 4}, "失败后的下一批(不应重放 1、2)")
	if n := sink.callCount(); n != 2 {
		t.Fatalf("Sink 被调用 %d 次, want 2 次(失败批不能被自动重试)", n)
	}
	if got := w.Stats().Failed; got != 1 {
		t.Fatalf("Stats.Failed = %d, want 1(失败计数只增不减)", got)
	}
}
