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
type TaskQueue struct {
	tasks   chan task
	running sync.Map // key → struct{}:正在执行或已入队的 key
	wg      sync.WaitGroup
}

// NewTaskQueue 启动 workers 个后台 goroutine。
func NewTaskQueue(workers, queueLen int) *TaskQueue {
	if workers <= 0 {
		workers = defaultTaskWorkers
	}
	if queueLen <= 0 {
		queueLen = defaultTaskQueue
	}
	q := &TaskQueue{tasks: make(chan task, queueLen)}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

func (q *TaskQueue) worker() {
	defer q.wg.Done()
	for t := range q.tasks {
		q.run(t)
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
	// ctx 由调用方在闭包里自己建(每个任务需要独立的超时),这里只保证传入非 nil。
	t.fn(context.Background())
}

// Submit 提交一个后台任务,返回是否真的入队。
// 同一 key 已有任务在执行/排队时直接跳过;队列满时丢弃(记日志)。
func (q *TaskQueue) Submit(key string, fn func(context.Context)) bool {
	if q == nil || fn == nil {
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

// Close 停止 worker(不再接受新任务)。
func (q *TaskQueue) Close() {
	if q == nil {
		return
	}
	close(q.tasks)
	q.wg.Wait()
}
