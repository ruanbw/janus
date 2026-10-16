package domain

// 后台任务队列:有界并发 + 按 key 去重 + 队列有上限。
//
// 原来的 recheck / patch(active) 每次请求都直接 `go` 一个 15s 超时的 goroutine
// (内含 DNS 查询 + HTTPS 探活),而 /api/domains* 全线没有限流:并发 N 次请求就是
// N 个 goroutine × N 次出网请求,把平台自己的 DNS/ACME 一起打满,还绕过了任何
// 上限。这里把并发钉死在固定几个 worker 上,同一个域名同时只跑一个任务
// (single-flight:重复点击"重新校验"只做一次),队列满则直接丢弃并记日志
// —— 队列是有界的,内存不会随请求数线性增长。

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
)

const (
	// defaultTaskWorkers 后台校验任务并发数。DNS 查询与 HTTPS 探活都打网络,
	// 放大并发只会互相拖慢,不会更快。
	defaultTaskWorkers = 4
	// defaultTaskQueue 队列长度上限。
	defaultTaskQueue = 256
)

type task struct {
	key string
	fn  func(context.Context)
}

// TaskQueue 有界后台任务队列。
//
// 生命周期:队列持有一个上下文(NewTaskQueue 里创建),任务拿到的就是它;
// Close 取消该上下文并等正在跑的任务结束。没有这一步,每个持有者
// (一个 DNSChecker = 一个 API 实例)都会留下 defaultTaskWorkers 个永不退出的
// goroutine,而且任务只能拿到 context.Background() —— 停机时正在跑的 DNS 查询与
// HTTPS 探活收不到任何中止信号,只能干等超时。
type TaskQueue struct {
	tasks   chan task
	running sync.Map // key → struct{}:正在执行或已入队的 key
	wg      sync.WaitGroup

	// ctx 队列的存活上下文:Close 时取消,任务是它的下游。
	ctx    context.Context
	cancel context.CancelFunc

	closed    atomic.Bool
	closeOnce sync.Once
}

// NewTaskQueue 启动 workers 个后台 goroutine。
func NewTaskQueue(workers, queueLen int) *TaskQueue {
	if workers <= 0 {
		workers = defaultTaskWorkers
	}
	if queueLen <= 0 {
		queueLen = defaultTaskQueue
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &TaskQueue{tasks: make(chan task, queueLen), ctx: ctx, cancel: cancel}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

// worker 反复取任务执行,直到队列被 Close。
//
// 刻意不用 `for t := range q.tasks`(靠 close(channel) 收口):那要求 Submit 在
// 发送前确认 channel 未关闭,而"检查"与"发送"之间必然有竞争窗口 —— 发到已关闭的
// channel 会 panic。改成"停机 = 取消上下文",Submit 就永远不会 panic。
func (q *TaskQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case t := <-q.tasks:
			q.run(t)
		}
	}
}

// run 执行单个任务并吞掉它的 panic:任务里的任何 bug 都不该把进程带走
// (goroutine 内未捕获的 panic 会终止整个进程,连带 HTTP 服务)。
func (q *TaskQueue) run(t task) {
	defer func() {
		q.running.Delete(t.key)
		if rec := recover(); rec != nil {
			log.Printf("domain task %s panic: %v", t.key, rec)
		}
	}()
	// 传入队列的存活上下文:任务自己派生带超时的子 ctx(每个任务需要独立的超时),
	// 但必须能感知停机,不能再用 context.Background() 把停机信号挡在门外。
	t.fn(q.ctx)
}

// Submit 提交一个后台任务,返回是否真的入队。
// 同一 key 已有任务在执行/排队时直接跳过;队列已关闭或已满时丢弃(记日志)。
func (q *TaskQueue) Submit(key string, fn func(context.Context)) bool {
	if q == nil || fn == nil || q.closed.Load() {
		return false
	}
	if _, dup := q.running.LoadOrStore(key, struct{}{}); dup {
		return false
	}
	select {
	case q.tasks <- task{key: key, fn: fn}:
		return true
	default:
		q.running.Delete(key)
		log.Printf("domain task %s dropped: 后台任务队列已满(%d)", key, cap(q.tasks))
		return false
	}
}

// Close 停止接收新任务、取消任务上下文,并等正在执行的任务结束。
//
// 幂等:重复调用安全(持有者的停机路径可能与其它退出路径重入)。已入队但还没被
// worker 取到的任务不再保证执行 —— 停机语义是"尽快收手",而不是"把队列排空"。
func (q *TaskQueue) Close() {
	if q == nil {
		return
	}
	q.closeOnce.Do(func() {
		q.closed.Store(true)
		q.cancel()
	})
	q.wg.Wait()
}
