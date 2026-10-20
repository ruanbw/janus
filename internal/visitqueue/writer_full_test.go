package visitqueue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// syncFallbackTimeout 是队列满时 Enqueue 必须返回的时限。
// 取 500ms 是因为它挂在跳转热路径上:超时这段时间访问者就卡在短链上。
const syncFallbackTimeout = 500 * time.Millisecond

// enqueueWithin 在时限内执行一次 Enqueue,超时即判定为失败。
//
// 为什么要包一层:Enqueue 是个可能被写路径拖住的阻塞调用,直接写在用例里的话,
// 实现一旦阻塞就是整个用例挂到 go test 的 10 分钟超时,报错信息只剩一堆 goroutine 栈;
// 包一层能把"阻塞"变成一条带上下文的失败。
func enqueueWithin(t *testing.T, w *Writer, ctx context.Context, rec store.VisitRecord, limit time.Duration, when string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- w.Enqueue(ctx, rec) }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("%s: Enqueue 在 %v 内没有返回(卡在写入路径上)", when, limit)
		return nil
	}
}

// TestEnqueueFallsBackToSyncWriteWhenQueueFull 锁住「队列满时 Enqueue 不得无限阻塞」。
//
// 这是把同步写挪到后台 goroutine 后新引入的风险:一旦 DB 慢,Sink 卡住(这里用
// channel 把它钉死),后台 goroutine 停止消费,队列随即打满;此时 Enqueue 若还
// 阻塞等待排队,整条跳转链路就跟着 DB 一起挂——比原来的同步 INSERT 更糟。
//
// 时序(全靠 channel,不用 sleep 碰运气):
//
//	Enqueue#1 → 入队 → 定时刷出撞上被卡住的 Sink(= DB 慢)
//	Enqueue#2 → 填满容量为 1 的队列
//	Enqueue#3 → 队列已满,必须走同步直写并在 500ms 内返回
func TestEnqueueFallsBackToSyncWriteWhenQueueFull(t *testing.T) {
	// 同步直写的那条:假 Sink 见到它立刻放行,否则无法区分"Enqueue 卡在队列上"
	// 和"Enqueue 卡在同一个慢 Sink 上"。
	const syncSentinel int64 = 99

	entered := make(chan struct{}, 8) // Sink 进入了(后台批次卡在这)
	release := make(chan struct{})    // 放行被卡的批次

	var mu sync.Mutex
	var seen []int64
	sink := func(_ context.Context, batch []store.VisitRecord) error {
		mu.Lock()
		for _, rec := range batch {
			seen = append(seen, rec.LinkID)
		}
		mu.Unlock()
		if len(batch) == 1 && batch[0].LinkID == syncSentinel {
			return nil
		}
		entered <- struct{}{}
		<-release
		return nil
	}

	// 失败路径也要放行被卡的 Sink,否则用例失败时会一直挂着等 go test 超时
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	w := New(Options{
		Sink:          sink,
		QueueSize:     1,
		BatchSize:     100,
		FlushInterval: 20 * time.Millisecond, // 让第一条尽快被定时刷出,好把后台 goroutine 钉在 Sink 里
	})

	ctx := context.Background()
	if err := enqueueWithin(t, w, ctx, store.VisitRecord{LinkID: 1}, waitTimeout, "第 1 条"); err != nil {
		t.Fatalf("第 1 条入队失败: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(waitTimeout):
		t.Fatal("定时刷出没有发生,后台 goroutine 没有进入 Sink,无法构造队列满的场景")
	}

	// 后台 goroutine 卡在 Sink 里,队列(容量 1)现在是空的 → 第 2 条能进队列
	if err := enqueueWithin(t, w, ctx, store.VisitRecord{LinkID: 2}, waitTimeout, "第 2 条"); err != nil {
		t.Fatalf("第 2 条入队失败: %v", err)
	}

	// 队列已满:第 3 条只能同步直写,且不能把跳转拖在这里
	err := enqueueWithin(t, w, ctx, store.VisitRecord{LinkID: syncSentinel}, syncFallbackTimeout, "队列已满时的第 3 条")
	if err != nil {
		t.Fatalf("同步直写成功时 Enqueue 应返回 nil, got %v", err)
	}

	if got := w.Stats().SyncFallback; got != 1 {
		t.Fatalf("Stats.SyncFallback = %d, want 1(队列满那一次)", got)
	}
	// 直写必须真的落到 Sink:否则"队列满时同步直写"只是换了个地方丢数据
	mu.Lock()
	sawSentinel := false
	for _, id := range seen {
		if id == syncSentinel {
			sawSentinel = true
		}
	}
	mu.Unlock()
	if !sawSentinel {
		t.Fatalf("队列满的那条记录没有进 Sink(实际写出 %v)", seen)
	}

	unblock()
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
}

// TestCloseFlushesPendingRecordsAndIsIdempotent 断言优雅停机不丢数据。
//
// 停机时缓冲里通常还有没攒够批的明细(上面有 FlushInterval 兜底,但进程不会等它);
// Close 必须"停收 + 排空 + flush",否则每次发布都会丢掉最后几百毫秒的访问明细。
// 幂等是因为关闭往往被多个 defer 触发(HTTP server shutdown、测试收尾),
// 第二次再刷一批或 panic 都会让停机变成事故。
func TestCloseFlushesPendingRecordsAndIsIdempotent(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     10,
		FlushInterval: time.Hour, // 只有 Close 能把残留刷出去
	})

	for _, rec := range visitRecs(1, 3) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}
	if n := sink.callCount(); n != 0 {
		t.Fatalf("Close 之前就写了 %d 批,用例前提被破坏", n)
	}

	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close 返回错误: %v", err)
	}
	sink.waitCall(t, "Close 排空残留")
	assertLinkIDs(t, sink.callAt(0), []int64{1, 2, 3}, "Close 排空")

	// 第二次 Close:返回 nil、不 panic、也不再多打一次 Sink
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("第二次 Close 返回 %v,幂等应该返回 nil", err)
	}
	if n := sink.callCount(); n != 1 {
		t.Fatalf("第二次 Close 之后 Sink 共被调用 %d 次, want 1 次", n)
	}
}

// TestEnqueueAfterCloseWritesSynchronously 断言停机窗口里仍在跳转的请求不丢明细。
//
// Close 与"HTTP server 停止监听"之间总有在途请求;这些请求已经看过跳转,
// 明细必须还能落库(同步直写),而且不能因为 Writer 已关闭就 panic 把请求带崩。
func TestEnqueueAfterCloseWritesSynchronously(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     16,
		BatchSize:     10,
		FlushInterval: time.Hour,
	})
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}

	var enqueueErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Close 之后 Enqueue panic: %v(停机期间的跳转会把请求打崩)", r)
			}
		}()
		enqueueErr = w.Enqueue(context.Background(), store.VisitRecord{LinkID: 42})
	}()

	// 契约只承诺"同步直写 + 不 panic",没有规定返回值;这里不锁死返回值,
	// 但记录必须真的落到 Sink 上——那才是"不丢明细"。
	if enqueueErr != nil {
		t.Logf("Close 之后 Enqueue 返回 %v(契约未规定返回值,只要求记录不丢)", enqueueErr)
	}
	if !sink.sawLinkID(42) {
		t.Fatalf("Close 之后的记录没有进 Sink:停机期间的最后几条明细丢了")
	}
	if got := w.Stats().SyncFallback; got != 1 {
		t.Fatalf("Stats.SyncFallback = %d, want 1(关闭后走的是同步直写)", got)
	}
}

// TestEnqueueKeepsRecordWhenContextCanceled 断言 ctx 取消不影响记录落库。
//
// 契约第 8 条允许二选一,本用例选择并固定「照常入队/直写」这一支:
// 访问明细记录的是"访问者已经看到跳转"这个既成事实,请求 ctx 在写库前被取消
// (客户端断开、超时)不该把它抹掉——那正是最需要排查的长尾请求。
//
// 因此:Enqueue 返回 nil(不报错、不丢),Close 之后 Sink 必能收到这条记录。
func TestEnqueueKeepsRecordWhenContextCanceled(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     8,
		BatchSize:     10,
		FlushInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 入队前就已取消

	if err := w.Enqueue(ctx, store.VisitRecord{LinkID: 7}); err != nil {
		t.Fatalf("选择的语义是「ctx 取消也照常入队」,不该返回错误, got %v", err)
	}
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}

	sink.waitCall(t, "Close 排空取消上下文里入队的记录")
	assertLinkIDs(t, sink.callAt(0), []int64{7}, "ctx 已取消时入队的记录")
}

// TestStatsCountExactlyAcrossRounds 断言 Stats 的计数在多轮入队/刷出后精确。
//
// 这些数字是唯一能判断"队列在正常工作、没有偷偷吞数据"的运维信号
// (Enqueued-Written-Failed 对不上就说明有记录烂在内存里),所以不能有漂移或重复计数。
func TestStatsCountExactlyAcrossRounds(t *testing.T) {
	sink := newRecordingSink()
	w := New(Options{
		Sink:          sink.sink,
		QueueSize:     32,
		BatchSize:     3,
		FlushInterval: time.Hour,
	})
	defer func() { _ = w.Close(context.Background()) }()

	t.Run("Given 入队 7 条且 BatchSize=3_When 攒够两批后再 Flush_Then 计数精确", func(t *testing.T) {
		for _, rec := range visitRecs(1, 7) {
			if err := w.Enqueue(context.Background(), rec); err != nil {
				t.Fatalf("入队失败: %v", err)
			}
		}
		sink.waitCall(t, "第一批(3 条)")
		sink.waitCall(t, "第二批(3 条)")

		// Flush 是屏障:它返回后前 7 条都已写出,第 7 条不能还卡在队里
		if err := w.Flush(context.Background()); err != nil {
			t.Fatalf("Flush 失败: %v", err)
		}
		sink.waitCall(t, "Flush 刷出的第 7 条")
		assertLinkIDs(t, sink.callAt(2), []int64{7}, "Flush 刷出的尾巴")

		st := w.Stats()
		if st.Enqueued != 7 || st.Written != 7 || st.SyncFallback != 0 || st.Failed != 0 {
			t.Fatalf("Stats = %+v, want {Enqueued:7 Written:7 SyncFallback:0 Failed:0}", st)
		}
	})

	t.Run("Given 缓冲已空_When 再 Flush 并再攒一批_Then 不重复计数", func(t *testing.T) {
		if err := w.Flush(context.Background()); err != nil {
			t.Fatalf("空缓冲 Flush 失败: %v", err)
		}
		if st := w.Stats(); st.Written != 7 || st.Enqueued != 7 {
			t.Fatalf("空缓冲 Flush 改了计数: %+v", st)
		}

		for _, rec := range visitRecs(8, 3) {
			if err := w.Enqueue(context.Background(), rec); err != nil {
				t.Fatalf("入队失败: %v", err)
			}
		}
		sink.waitCall(t, "第三批(3 条)")

		st := w.Stats()
		if st.Enqueued != 10 || st.Written != 10 || st.SyncFallback != 0 || st.Failed != 0 {
			t.Fatalf("Stats = %+v, want {Enqueued:10 Written:10 SyncFallback:0 Failed:0}", st)
		}
	})
}

// TestNewUsesDocumentedDefaults 断言 <=0 的参数落到文档化的默认值。
//
// 默认值不是可有可无的细节:部署方只填 Sink 时,队列容量决定了突发时会不会退化成
// 同步直写,BatchSize 决定了"一次 INSERT 写几条"。这里用 BatchSize 的默认 100 做锚点:
// 第 100 条入队必须自动写出(而不是攒到 101 条、或一条一条写)。
func TestNewUsesDocumentedDefaults(t *testing.T) {
	const defaultBatchSize = 100
	sink := newRecordingSink()
	w := New(Options{Sink: sink.sink}) // 其余字段全零
	defer func() { _ = w.Close(context.Background()) }()

	for _, rec := range visitRecs(1, defaultBatchSize) {
		if err := w.Enqueue(context.Background(), rec); err != nil {
			t.Fatalf("入队失败(零值参数应在 New 里补成默认 1024 的队列容量): %v", err)
		}
	}

	sink.waitCall(t, "默认 BatchSize=100 攒够一批")
	if n := len(sink.callAt(0)); n != defaultBatchSize {
		t.Fatalf("默认配置下首批 %d 条, want %d", n, defaultBatchSize)
	}
	if got := w.Stats().Written; got != defaultBatchSize {
		t.Fatalf("Stats.Written = %d, want %d", got, defaultBatchSize)
	}
}
