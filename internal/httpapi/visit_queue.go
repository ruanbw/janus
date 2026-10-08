package httpapi

// 访问明细异步写入队列。
//
// 为什么要把 InsertVisit 挪出请求路径:
//
//  1. 延迟叠加。原先每次跳转在写完 Location 之前同步 INSERT 一行明细,
//     这一次往返(以及连接池排队)完整地叠加到访客的跳转延迟上,而访客根本不关心统计。
//  2. 连接池放大。突发流量下每个跳转请求都要占一条连接去写明细,
//     明细写入与"查短链"抢同一个连接池,池满后查短链也开始排队 —— 统计把跳转拖慢了。
//  3. 客户端断开即丢数据。INSERT 用的是请求 ctx,访客一关页面(或爬虫拿到 302 就断开)
//     ctx 被 net/http 取消,INSERT 随之失败,明细静默丢失。
//
// 现在:请求路径只做一次非阻塞入队(纳秒级);少量 worker 把队列里的明细攒成批,
// 用一条多行 INSERT 落库,ctx 与请求解耦(context.WithoutCancel + 独立超时)。
//
// 背压策略是**丢弃而不是阻塞**:队列满说明数据库已经跟不上,此时阻塞只会把
// 数据库的慢传导成跳转的慢(恰恰是本队列要消除的东西)。丢弃会计数并限频打日志,
// 运维能从日志/计数里看到"统计在丢",而短链本身始终可用。
//
// 后置访问钩子(PostVisitHook)在明细**落库尝试之后**触发,保持钩子"访问日志持久化
// 完成后的异步回调"的契约:钩子里回查 visits 能查到这一行(落库失败时与改造前一致,
// 钩子照常触发,不因统计故障丢掉下游通知)。被丢弃的明细不触发钩子 —— 队列满就是过载,
// 再为它起 goroutine 跑钩子只会加剧过载;丢弃计数是这类损失唯一的留痕。
//
// 关闭:Close 停止接收新明细,等 worker 把队列里剩余的明细全部落库(受 ctx 约束)后返回。
// 必须在 http.Server.Shutdown 之后调用 —— 那时已没有进行中的请求会再入队。

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"janus/internal/store"
)

// 队列默认参数(VisitQueueConfig 对应字段 <=0 时生效)。
const (
	DefaultVisitQueueSize    = 10000
	DefaultVisitQueueWorkers = 2
	DefaultVisitQueueBatch   = 200
	// DefaultVisitInsertTimeout 单批 INSERT 的超时。与请求解耦后必须有自己的上限,
	// 否则数据库卡死时 worker 永远挂着,Close 也永远等不到。
	DefaultVisitInsertTimeout = 5 * time.Second
)

// 入队失败的两种原因。
var (
	// ErrVisitQueueFull 队列已满,本条明细被丢弃(已计数)。
	ErrVisitQueueFull = errors.New("visit queue full")
	// ErrVisitQueueClosed 队列已关闭(进程正在退出)。调用方可退回同步写入。
	ErrVisitQueueClosed = errors.New("visit queue closed")
)

// VisitQueueConfig 访问明细队列参数;零值字段取 Default* 常量。
type VisitQueueConfig struct {
	Size          int           // 队列容量(条)
	Workers       int           // 落库 worker 数
	BatchSize     int           // 单批最大行数
	InsertTimeout time.Duration // 单批 INSERT 超时
}

func (c VisitQueueConfig) withDefaults() VisitQueueConfig {
	if c.Size <= 0 {
		c.Size = DefaultVisitQueueSize
	}
	if c.Workers <= 0 {
		c.Workers = DefaultVisitQueueWorkers
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultVisitQueueBatch
	}
	if c.InsertTimeout <= 0 {
		c.InsertTimeout = DefaultVisitInsertTimeout
	}
	return c
}

// visitJob 队列中的一条待落库明细,连同触发后置钩子所需的请求上下文。
// ctx 已经过 context.WithoutCancel:保留请求里的值,但不随请求结束而取消。
type visitJob struct {
	ctx context.Context
	r   *http.Request
	rec store.VisitRecord
}

// VisitQueue 有界的访问明细异步写入队列(见文件头注释)。并发安全。
type VisitQueue struct {
	cfg VisitQueueConfig
	ch  chan visitJob
	// insertBatch 批量落库;insertOne 批量失败后逐行重试(隔离单行坏数据,不让它拖垮整批)。
	insertBatch func(ctx context.Context, recs []store.VisitRecord) error
	insertOne   func(ctx context.Context, rec store.VisitRecord) error
	// afterInsert 落库尝试之后对每条明细调用(生产为触发后置访问钩子)。
	afterInsert func(job visitJob)
	log         *slog.Logger

	// mu 保护 closed 与 ch 的关闭:Enqueue 持读锁做非阻塞发送,Close 持写锁关 channel,
	// 从而不会出现"向已关闭的 channel 发送"的 panic。
	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	wg        sync.WaitGroup

	dropped  atomic.Uint64
	inserted atomic.Uint64
	failed   atomic.Uint64
}

// NewVisitQueue 构建并启动生产用的访问明细队列(落库走 st,落库后触发后置访问钩子)。
func NewVisitQueue(st *store.Store, cfg VisitQueueConfig) *VisitQueue {
	return newVisitQueue(cfg, st.InsertVisits, st.InsertVisit, func(job visitJob) {
		TriggerPostVisitHooks(job.ctx, job.r, job.rec)
	})
}

// newVisitQueue 可注入落库函数的构造(单测不依赖 Postgres)。
func newVisitQueue(
	cfg VisitQueueConfig,
	insertBatch func(ctx context.Context, recs []store.VisitRecord) error,
	insertOne func(ctx context.Context, rec store.VisitRecord) error,
	afterInsert func(job visitJob),
) *VisitQueue {
	cfg = cfg.withDefaults()
	q := &VisitQueue{
		cfg:         cfg,
		ch:          make(chan visitJob, cfg.Size),
		insertBatch: insertBatch,
		insertOne:   insertOne,
		afterInsert: afterInsert,
		log:         slog.Default(),
	}
	q.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go q.worker()
	}
	return q
}

// Enqueue 非阻塞入队一条明细。队列满返回 ErrVisitQueueFull(已计入丢弃数),
// 已关闭返回 ErrVisitQueueClosed。ctx 会被 context.WithoutCancel 处理后随明细保存。
func (q *VisitQueue) Enqueue(ctx context.Context, r *http.Request, rec store.VisitRecord) error {
	job := visitJob{ctx: context.WithoutCancel(ctx), r: r, rec: rec}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrVisitQueueClosed
	}
	select {
	case q.ch <- job:
		return nil
	default:
		n := q.dropped.Add(1)
		// 限频:首次丢弃与此后每 1000 次打一条,避免过载时日志本身成为新的瓶颈。
		if n == 1 || n%1000 == 0 {
			q.log.Warn("访问明细队列已满,丢弃明细(跳转不受影响)",
				"dropped_total", n, "capacity", q.cfg.Size, "link", rec.LinkID)
		}
		return ErrVisitQueueFull
	}
}

// Dropped 因队列满而丢弃的明细累计条数。
func (q *VisitQueue) Dropped() uint64 { return q.dropped.Load() }

// Inserted 成功落库的明细累计条数。
func (q *VisitQueue) Inserted() uint64 { return q.inserted.Load() }

// Failed 落库失败(批量与逐行重试都失败)的明细累计条数。
func (q *VisitQueue) Failed() uint64 { return q.failed.Load() }

// Close 停止接收新明细并等待队列排空。可重复调用(之后的调用只等待)。
// ctx 到期时立即返回 ctx.Err(),worker 仍会在后台把剩余明细写完或超时放弃。
func (q *VisitQueue) Close(ctx context.Context) error {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.ch)
		q.mu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		q.log.Error("访问明细队列关闭超时,剩余明细可能丢失", "pending", len(q.ch))
		return ctx.Err()
	}
}

// worker 阻塞取第一条,再非阻塞地把已在队列里的明细捞满一批后落库。
//
// 不设"攒够 N 条或等 T 毫秒"的定时器:低峰期每条明细立刻落库(不引入统计延迟),
// 高峰期队列里自然堆着多条,一次就能捞满一批 —— 批量大小随负载自适应。
func (q *VisitQueue) worker() {
	defer q.wg.Done()
	batch := make([]visitJob, 0, q.cfg.BatchSize)
	for job := range q.ch {
		batch = append(batch[:0], job)
	fill:
		for len(batch) < q.cfg.BatchSize {
			select {
			case next, ok := <-q.ch:
				if !ok {
					break fill
				}
				batch = append(batch, next)
			default:
				break fill
			}
		}
		q.flush(batch)
	}
}

// flush 落库一批明细,随后对每条触发 afterInsert。
// 整批失败时逐行重试:一条坏数据(例如超长字段)不该连累同批的其他访客。
func (q *VisitQueue) flush(batch []visitJob) {
	recs := make([]store.VisitRecord, len(batch))
	for i, j := range batch {
		recs[i] = j.rec
	}
	ctx, cancel := context.WithTimeout(context.Background(), q.cfg.InsertTimeout)
	err := q.safeInsertBatch(ctx, recs)
	cancel()
	if err == nil {
		q.inserted.Add(uint64(len(recs)))
	} else {
		q.log.Error("访问明细批量写入失败,改为逐行重试", "rows", len(recs), "err", err)
		for _, rec := range recs {
			ctx, cancel := context.WithTimeout(context.Background(), q.cfg.InsertTimeout)
			if err := q.safeInsertOne(ctx, rec); err != nil {
				q.failed.Add(1)
				q.log.Error("访问明细写入失败,已丢弃", "link", rec.LinkID, "err", err)
			} else {
				q.inserted.Add(1)
			}
			cancel()
		}
	}
	if q.afterInsert != nil {
		for _, j := range batch {
			q.safeAfter(j)
		}
	}
}

// safeInsertBatch / safeInsertOne / safeAfter 兜住 panic:worker 跑在请求链之外,
// 一次 panic 会带走整个进程(连带所有正在跳转的访客)。
func (q *VisitQueue) safeInsertBatch(ctx context.Context, recs []store.VisitRecord) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("insert batch panic")
			q.log.Error("访问明细批量写入 panic", "panic", r)
		}
	}()
	return q.insertBatch(ctx, recs)
}

func (q *VisitQueue) safeInsertOne(ctx context.Context, rec store.VisitRecord) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("insert panic")
			q.log.Error("访问明细写入 panic", "panic", r)
		}
	}()
	if q.insertOne == nil {
		return q.insertBatch(ctx, []store.VisitRecord{rec})
	}
	return q.insertOne(ctx, rec)
}

func (q *VisitQueue) safeAfter(j visitJob) {
	defer func() {
		if r := recover(); r != nil {
			q.log.Error("访问明细落库后回调 panic,已忽略", "panic", r)
		}
	}()
	q.afterInsert(j)
}
