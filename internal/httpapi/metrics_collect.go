package httpapi

import (
	"sync"

	"github.com/ruanbw/janus/internal/linkcache"
	"github.com/ruanbw/janus/internal/metrics"
	"github.com/ruanbw/janus/internal/visitqueue"
)

// statsCollector 把跳转热路径的内部计数搬进 Prometheus 注册表。
//
// 为什么不在热路径上直接 Inc:linkcache 与 visitqueue 已经各自维护了同一份原子
// 计数,再在每次跳转/入队时多打一遍指标,等于给最热的路径再加两次锁,而且两份
// 计数会漂移。这里改成「抓取时同步」:抓取频率是秒级、与 QPS 无关,代价可忽略。
//
// 同步方式必须做差分:上游 Stats() 是单调累计值(且文档保证永不清零),而
// metrics.Counter 只能累加。每次抓取只把「上次快照 → 本次快照」的增量加进去,
// 否则抓取次数会被算成业务量(抓得越勤,跳转数涨得越快)。
//
// 仪表盘类的值(缓存条目数、队列积压、异步开关)直接覆盖,它们本来就是瞬时量。
//
// 一个注册表只对应一个 HTTP 实例:同名计数器会被 Registry 复用成同一个对象,
// 若两个实例共用一个注册表并各自 collect,同一段增量就会被加两遍。
type statsCollector struct {
	reg    *metrics.Registry
	cache  *linkcache.Cache
	writer *visitqueue.Writer

	cacheHits       *metrics.Counter
	cacheMisses     *metrics.Counter
	cacheLoadErrors *metrics.Counter
	cacheEntries    *metrics.Gauge

	writerEnabled       *metrics.Gauge
	writerPending       *metrics.Gauge
	writerEnqueued      *metrics.Counter
	writerWritten       *metrics.Counter
	writerSyncFallback  *metrics.Counter
	writerBatchFailures *metrics.Counter

	// mu 保护 last* 与「读快照 + 写增量」的整体性:两个并发抓取各自读到同一份
	// 上游快照时,不加锁会把同一段增量加两遍。
	mu         sync.Mutex
	lastCache  linkcache.Stats
	lastWriter visitqueue.Stats
}

// newStatsCollector 注册全部指标并返回收集器。
//
// 指标在构造期就注册(而不是首次抓取时):系列必须从进程启动起就存在,
// 否则「异步没开」与「抓取端配错了指标名」在面板上都表现为 no data,
// 而这两种情况的处置完全不同。
func newStatsCollector(reg *metrics.Registry, cache *linkcache.Cache, writer *visitqueue.Writer) *statsCollector {
	s := &statsCollector{reg: reg, cache: cache, writer: writer}
	if reg == nil {
		// 指标未装配(嵌入方直接构造 API):collect 退化为 no-op,
		// 调用点不必为此写 if。
		return s
	}

	s.cacheHits = reg.Counter("janus_linkcache_hits_total", "短链快照缓存直接命中的次数。")
	s.cacheMisses = reg.Counter("janus_linkcache_misses_total", "短链快照缓存未命中(含随后回源)的次数。")
	s.cacheLoadErrors = reg.Counter("janus_linkcache_load_errors_total", "短链快照回源失败(未找到不计)的次数。")
	s.cacheEntries = reg.Gauge("janus_linkcache_entries", "当前缓存的短链快照条数。")

	s.writerEnabled = reg.Gauge("janus_visit_writer_enabled", "访问明细是否走异步批量写入(1=异步,0=同步直写)。")
	s.writerPending = reg.Gauge("janus_visit_writer_pending", "访问明细队列当前积压条数(不含正在写入的一批)。")
	s.writerEnqueued = reg.Counter("janus_visit_writer_enqueued_total", "被接受的访问明细条数(入队与同步直写都算)。")
	s.writerWritten = reg.Counter("janus_visit_writer_written_total", "成功落库的访问明细条数。")
	s.writerSyncFallback = reg.Counter("janus_visit_writer_sync_fallback_total", "队列满或已关闭时退回同步直写的条数。")
	s.writerBatchFailures = reg.Counter("janus_visit_writer_batch_failures_total", "失败的批次数(失败即整批丢弃)。")

	return s
}

// collect 把上游的最新快照同步进注册表。必须在 WriteText 之前调用。
func (s *statsCollector) collect() {
	if s == nil || s.reg == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cache != nil {
		cur := s.cache.Stats()
		s.cacheHits.Add(delta(cur.Hits, s.lastCache.Hits))
		s.cacheMisses.Add(delta(cur.Misses, s.lastCache.Misses))
		s.cacheLoadErrors.Add(delta(cur.LoadErrors, s.lastCache.LoadErrors))
		s.lastCache = cur
		s.cacheEntries.Set(float64(s.cache.Len()))
	}

	if s.writer == nil {
		// 同步逐行写入模式:系列保留并显式置 0,见 newStatsCollector 的注释。
		s.writerEnabled.Set(0)
		s.writerPending.Set(0)
		return
	}

	cur := s.writer.Stats()
	s.writerEnabled.Set(1)
	s.writerPending.Set(float64(cur.Pending))
	s.writerEnqueued.Add(deltaUint64ToFloat(cur.Enqueued, s.lastWriter.Enqueued))
	s.writerWritten.Add(deltaUint64ToFloat(cur.Written, s.lastWriter.Written))
	s.writerSyncFallback.Add(deltaUint64ToFloat(cur.SyncFallback, s.lastWriter.SyncFallback))
	s.writerBatchFailures.Add(deltaUint64ToFloat(cur.Failed, s.lastWriter.Failed))
	s.lastWriter = cur
}

// delta 计算两次累计快照之间的增量。
//
// 上游不可能倒退(计数只在 Add 里增长);真的倒退了说明上游被重置过。
// 此时返回 0 而不是负增量:负增量会让 Prometheus 侧看到一个非单调的 counter,
// 抓取端会按 reset 处理并重新起算,反而比少记一次更难排查。
func delta(cur, last uint64) float64 {
	if cur <= last {
		return 0
	}
	return float64(cur - last)
}

func deltaUint64ToFloat(cur, last int64) float64 {
	if cur <= last {
		return 0
	}
	return float64(cur - last)
}
