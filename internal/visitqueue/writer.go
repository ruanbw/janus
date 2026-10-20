// Package visitqueue 把访问明细的写库从跳转热路径挪到后台批处理。
//
// 短链跳转原本每条明细都要同步 INSERT 一次:一次 DB 往返直接加在访问者的
// 首字节延迟上,而且突发流量会把连接池打满。这里改成「请求只入队,后台攒够
// 一批写一次」,并且给队列一个硬上界——队列满时退回同步直写,宁可慢一条,
// 也不让内存无界增长或让请求无限阻塞在排队上。
package visitqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// 默认值:部署方只填 Sink 时也必须是一个能用的队列。
//
// QueueSize 给突发留的余量(1024 条明细约几十 KB,可以放心给大);
// BatchSize 与 FlushInterval 是一对权衡:批量越大越省 DB 往返,但低流量时
// 一条明细最多等 FlushInterval 才能落库,所以定时器必须有。
const (
	defaultQueueSize     = 1024
	defaultBatchSize     = 100
	defaultFlushInterval = 200 * time.Millisecond
)

// ErrNoSink 表示 New 时没有提供 Sink。
//
// 这是启动期配置错误。如果拖到第一条访问明细才在热路径上暴露,只能靠日志发现;
// 让 Enqueue 明确返回这个错误,调用方(或冒烟检查)能在启动时就知道队列是死的。
var ErrNoSink = errors.New("visitqueue: Options.Sink 未配置")

// Options 是 Writer 的配置;零值字段由 New 补成文档化的默认值。
type Options struct {
	// Sink 是真正的批量写库入口。
	//
	// 同一个批次不会被并发调用,但不同批次可能并发:后台刷出与「队列满/已关闭」的
	// 同步直写是两条独立路径。Sink 必须自身并发安全。
	// batch 的底层数组归 Writer 复用,实现里若需要留住数据必须自行拷贝。
	Sink func(ctx context.Context, batch []store.VisitRecord) error

	// OnBatchError 在 Sink 返回错误时被调用,n 是该批的条数。
	// 批次失败即丢弃、不重试,这个回调是唯一的可观测信号。
	// 它可能被后台 goroutine 与同步直写的调用方并发触发。
	OnBatchError func(err error, n int)

	// QueueSize 是待写队列容量(条数),<=0 取 1024。
	QueueSize int
	// BatchSize 是一次 Sink 调用最多写几条,<=0 取 100;缓冲攒到它立即刷出。
	BatchSize int
	// FlushInterval 是最长滞留时间,<=0 取 200ms;到期无条件刷出当前缓冲。
	FlushInterval time.Duration
}

// Stats 是 Writer 的计数快照(原子读,任意时刻可安全调用)。
//
// Enqueued 数的是被接受的记录(走队列与走同步直写都算),Written 数的是真正
// 交给 Sink 且成功的条数;Failed 数的是失败的批次数而不是条数——失败即整批丢弃,
// 所以 Enqueued-Written 与 Failed 之间没有直接换算关系。
// 计数是异步累加的:有在途批次时快照可能短暂对不上,静止下来后
// Enqueued == Written + 失败丢弃的条数。
//
// Pending 是唯一的瞬时值(不是累计):此刻还排在队列里、没被后台收走的条数。
// 它回答的是「消费是不是跟不上生产」——Enqueued-Written 只能说明有多少条没写完,
// 分不清是下游慢(卡在 Sink 里)还是队列在堆,两者的处置完全不同。
type Stats struct {
	Enqueued     int64
	Written      int64
	SyncFallback int64
	Failed       int64
	// Pending 队列当前积压条数;不含正在 Sink 里写的那一批。
	Pending int
}

// Writer 是有界队列 + 单个后台批处理 goroutine。
//
// 队列满、或 Close 之后入队的记录走同步直写:这两种情况下后台 goroutine 要么
// 停止消费(可能正卡在慢 Sink 上),要么已经退出,排队等于把记录烂在内存里。
type Writer struct {
	sink         func(context.Context, []store.VisitRecord) error
	onBatchError func(err error, n int)
	batchSize    int
	// flushInterval 只在后台 goroutine 里被读取,声明在构造时定值。
	flushInterval time.Duration

	queue    chan store.VisitRecord
	flushCh  chan chan struct{}
	stopCh   chan struct{}
	done     chan struct{}
	closeOne sync.Once

	// mu 只保护 closed:它把「Enqueue 决定是否入队」与「Close 开始排空」排序,
	// 保证 Close 之后 Enqueue 不会把记录投进一个没人再读的队列。
	mu     sync.RWMutex
	closed bool

	// batch 是后台批处理的固定缓冲,只有后台 goroutine 碰它(Flush 也通过
	// flushCh 交给后台执行),因此不需要锁。
	batch []store.VisitRecord

	enqueued     atomic.Int64
	written      atomic.Int64
	syncFallback atomic.Int64
	failed       atomic.Int64
}

// New 构造并启动 Writer。参数缺省按 Options 的文档补默认值。
//
// Sink 为空时不在这里报错而是让 Enqueue 返回 ErrNoSink:测试与调用方都按
// 「New 只返回 *Writer」的形态使用,错误必须出现在真正写数据的那一刻。
func New(opts Options) *Writer {
	w := &Writer{
		sink:          opts.Sink,
		onBatchError:  opts.OnBatchError,
		batchSize:     opts.BatchSize,
		flushInterval: opts.FlushInterval,
		flushCh:       make(chan chan struct{}),
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
	}
	if w.batchSize <= 0 {
		w.batchSize = defaultBatchSize
	}
	if w.flushInterval <= 0 {
		w.flushInterval = defaultFlushInterval
	}
	size := opts.QueueSize
	if size <= 0 {
		size = defaultQueueSize
	}
	w.queue = make(chan store.VisitRecord, size)
	w.batch = make([]store.VisitRecord, 0, w.batchSize)

	if w.sink == nil {
		// 配置错误:不启动后台 goroutine(它只会阻塞到进程退出)。
		// 把 done 关掉,保证「done 关闭 ⟺ 不会有 goroutine 再写」这个不变式成立。
		close(w.done)
		return w
	}

	go w.run()
	return w
}

// Enqueue 接受一条明细。返回 nil 表示「记录已接管」——要么进了队列,
// 要么已经同步写进 Sink;调用方不需要(也不应该)重试。
//
// ctx 只透传给同步直写那一路。记录的是「访问者已经看到跳转」这个既成事实,
// 请求 ctx 被取消(客户端断开、超时)不代表该抹掉它,所以 ctx 取消不影响入队。
func (w *Writer) Enqueue(ctx context.Context, rec store.VisitRecord) error {
	if w.sink == nil {
		return ErrNoSink
	}

	// 持读锁做非阻塞入队:Close 拿写锁时会等这一次投递落定,随后它排空的队列
	// 一定包含这条记录;而关闭之后的 Enqueue 一定看到 closed 走直写。
	w.mu.RLock()
	if !w.closed {
		select {
		case w.queue <- rec:
			w.mu.RUnlock()
			w.enqueued.Add(1)
			return nil
		default:
		}
	}
	w.mu.RUnlock()

	return w.writeSync(ctx, rec)
}

// Flush 是屏障:返回时调用方此前入队的记录都已交给 Sink(nil 表示成功)。
//
// 缓冲为空时不调用 Sink:否则每次停机、每次排障都白打一次 DB 往返。
// 后台正在写一批时,Flush 会等那一批结束——「屏障」的代价就是可能要等到
// Sink 返回,这正是调用方要的语义(ctx 可用来兜底)。
func (w *Writer) Flush(ctx context.Context) error {
	if w.sink == nil {
		return ErrNoSink
	}

	w.mu.RLock()
	closed := w.closed
	w.mu.RUnlock()
	if closed {
		// 关闭后后台已排空退出,后来者一律同步直写,不会留下没人刷的缓冲。
		return nil
	}

	ack := make(chan struct{})
	select {
	case w.flushCh <- ack:
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 停收、排空队列、刷出最后一批,然后等后台 goroutine 退出。
//
// 幂等且可重入:停机路径常常被多个 defer 触发,第二次再刷一批或 panic
// 都会让停机变成事故。返回 nil 只表示「排空已完成」(或已在别的调用里完成)。
func (w *Writer) Close(ctx context.Context) error {
	if w.sink == nil {
		return ErrNoSink
	}

	w.closeOne.Do(func() {
		// 先置 closed 再发停止信号:保证在排空开始之前,后续 Enqueue 全部走直写,
		// 不会有记录在排空结束后才被投进队列。
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		close(w.stopCh)
	})

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats 返回计数快照。
func (w *Writer) Stats() Stats {
	return Stats{
		Enqueued:     w.enqueued.Load(),
		Written:      w.written.Load(),
		SyncFallback: w.syncFallback.Load(),
		Failed:       w.failed.Load(),
		Pending:      len(w.queue),
	}
}

// writeSync 绕过队列直接写一条。
//
// 直接调 Sink 而不是排队:队列满意味着后台正卡在慢 Sink 上,把记录排到那批
// 后面等于让跳转等到 DB 恢复;关闭之后则根本没人再来刷队列。
// 这里自己分配单条切片,不能借后台的缓冲(两侧会并发)。
func (w *Writer) writeSync(ctx context.Context, rec store.VisitRecord) error {
	w.enqueued.Add(1)
	w.syncFallback.Add(1)

	err := w.sink(ctx, []store.VisitRecord{rec})
	if err != nil {
		w.failed.Add(1)
		w.reportError(err, 1)
		return err
	}
	w.written.Add(1)
	return nil
}

// run 是唯一的后台 goroutine:收记录、攒批、定时刷出、收到停止后排空退出。
func (w *Writer) run() {
	defer close(w.done)

	timer := time.NewTimer(w.flushInterval)
	defer timer.Stop()

	for {
		select {
		case rec := <-w.queue:
			w.batch = append(w.batch, rec)
			if len(w.batch) >= w.batchSize {
				w.flush()
				resetTimer(timer, w.flushInterval)
			}
		case <-timer.C:
			w.flush()
			timer.Reset(w.flushInterval)
		case ack := <-w.flushCh:
			// Flush 是屏障:队列里还没被后台收走的记录也算「调用方此前入队的」,
			// 必须一并交给 Sink。队列容量(默认 1024)天然给这一次刷出封了顶。
			w.pullAvailable()
			w.flush()
			resetTimer(timer, w.flushInterval)
			close(ack)
		case <-w.stopCh:
			w.drain()
			return
		}
	}
}

// pullAvailable 把队列里已经排着的记录全部收进缓冲(不阻塞,队空即返回)。
// 只允许后台 goroutine 调用。
func (w *Writer) pullAvailable() {
	for {
		select {
		case rec := <-w.queue:
			w.batch = append(w.batch, rec)
		default:
			return
		}
	}
}

// drain 在退出前把队列与缓冲清空,然后刷出最后一批。
//
// 按 BatchSize 分段写而不是一口气全收:停机时积压可能有上千条,
// 一次写超大语句更容易超时、失败丢得更多。分段不改变顺序(FIFO 排空)。
func (w *Writer) drain() {
	for {
		select {
		case rec := <-w.queue:
			w.batch = append(w.batch, rec)
			if len(w.batch) >= w.batchSize {
				w.flush()
			}
		default:
			w.flush()
			return
		}
	}
}

// flush 把当前缓冲交给 Sink 并清空。
//
// 只允许后台 goroutine 调用(它是 batch 的唯一所有者)。失败不重试:
// 明细是分析数据不是账单,一次 DB 抖动就重放同一批会把写放大成风暴、把内存拖爆,
// 失败即丢弃,只留计数与回调。
func (w *Writer) flush() {
	n := len(w.batch)
	if n == 0 {
		return
	}

	// 后台批次没有「某个请求的 ctx」可用:一批记录来自多个请求,任何一个的
	// 取消或超时都不该连坐整批。
	err := w.sink(context.Background(), w.batch)
	w.batch = w.batch[:0]
	if err != nil {
		w.failed.Add(1)
		w.reportError(err, n)
		return
	}
	w.written.Add(int64(n))
}

// reportError 调用户回调;回调自身出问题不能把后台 goroutine 带走。
func (w *Writer) reportError(err error, n int) {
	if w.onBatchError == nil {
		return
	}
	w.onBatchError(err, n)
}

// resetTimer 重启定时器。刷出之后重新计时(而不是沿用上一次的到期时刻),
// 语义是「这段时间内没有新的刷出,就把缓冲写出去」。
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}
