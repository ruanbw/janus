package geo

import (
	"hash/maphash"
	"sync"
)

// DefaultCacheEntries 全局缓存**两代合计**的条目上限。
//
// 数量级按"独立访客 IP"估:中型租户一天几万到十几万个不同出口 IP,
// 10 万条足够让重复访问(同一 NAT 出口、同一爬虫)基本全部命中,
// 内存占用是十万个 map 条目量级(几 MB),换来的是把绝大多数查询从
// 二分查找 + 切片分配降到一次哈希查找。
//
// 注意是**两代合计**:换代瞬间 cur 与 old 各留一份(见 cached.perShard),
// 若按单代分配,实际峰值会是这里声明值的两倍 —— 一个"条目上限"字段
// 实际给出两倍容量,会让容量规划与故障排查都建立在错误的数上。
const DefaultCacheEntries = 100_000

// cacheShards 分片数,必须是 2 的幂(靠位与选片)。
// 分片是为了让读写锁的竞争足够低:跳转是全并发链路,
// 一把全局锁会把所有请求排在同一行上。
const cacheShards = 16

// generations 换代时同时保留的代数。cur 写满后整体丢弃换新,old 留一代
// 兜住老条目 —— 所以峰值内存是单代的 generations 倍。
const generations = 2

var cacheSeed = maphash.MakeSeed()

// cached 给底层 Lookup 套一层进程内缓存。
type cached struct {
	inner    Lookup
	perShard int
	shards   [cacheShards]shard
}

// shard 一片缓存。两代 map:cur 写满后整体丢弃换新,old 留一代兜住老条目。
type shard struct {
	mu  sync.RWMutex
	cur map[string]Info
	old map[string]Info
}

// Cached 返回带缓存的 Lookup。entries 是全局**两代合计**的条目上限(会均分到各分片),
// <= 0 时用 DefaultCacheEntries。
//
// 负结果(查不到)同样进缓存,而且是这里最要紧的一条:一个查不到的 IP 如果每次
// 都重新去查,散落在未知网段与私网地址上的流量会把查询成本打满——
// 缓存的收益恰恰主要来自这些"反复查不到"的地址。
func Cached(inner Lookup, entries int) Lookup {
	if entries <= 0 {
		entries = DefaultCacheEntries
	}
	// 每片的上限按"两代合计"折半:一代写满时 cur 与 old 各持有 max 条,
	// 峰值才是 2 × max = entries / cacheShards,而不是它的两倍。
	c := &cached{inner: inner, perShard: entries / (cacheShards * generations)}
	if c.perShard < 1 {
		c.perShard = 1
	}
	return c
}

func (c *cached) Lookup(ip string) Info {
	if ip == "" {
		return Info{}
	}
	sh := &c.shards[maphash.String(cacheSeed, ip)&(cacheShards-1)]
	if v, ok, gen := sh.get(ip); ok {
		// 命中"上一代"时顺手搬进当前代。
		//
		// 原来两代只读不搬,于是每一次换代都会把上一代整体作废:一个
		// 只被间歇访问的 IP(同一爬虫每隔几分钟扫一次)必然在换代那一刻
		// 丢掉缓存,下一次又要去查底层 —— 高基数流量下命中率会**周期性**
		// 塌陷再爬升,而不是稳定在高位。搬一次的成本是一次 map 写,
		// 换来的是"还在被访问的条目一定能活过换代",这正是按访问热度保留,
		// 且不需要维护 LRU 链表或额外的计数器。
		if gen < 0 {
			sh.promote(ip, v, c.perShard)
		}
		return v
	}
	v := c.inner.Lookup(ip)
	sh.put(ip, v, c.perShard)
	return v
}

// get 返回 (值, 是否命中, 命中的是哪一代)。
// 代号为 0 表示当前代(cur),-1 表示上一代(old)。
func (s *shard) get(ip string) (Info, bool, int) {
	s.mu.RLock()
	v, ok := s.cur[ip]
	gen := 0
	if !ok {
		v, ok = s.old[ip]
		gen = -1
	}
	s.mu.RUnlock()
	return v, ok, gen
}

// promote 把上一代里被再次命中的条目搬进当前代(见 Lookup 的说明)。
func (s *shard) promote(ip string, v Info, max int) {
	s.mu.Lock()
	// cur 已满就不再搬:这一代正在换代边缘,搬进来也会立刻被丢弃,
	// 白白占一次写锁,不如留给下一代的自然写入。
	if s.cur != nil && len(s.cur) < max {
		s.cur[ip] = v
	}
	s.mu.Unlock()
}

func (s *shard) put(ip string, v Info, max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		s.cur = make(map[string]Info, 256)
	} else if len(s.cur) >= max {
		// 整代换新,不做逐条 LRU:IP → 国家的映射没有访问顺序上的时间局部性,
		// 逐条维护链表带来的命中率提升微乎其微,却要在每次写入时抢锁 + 改链表。
		// 整代丢弃的代价是刚淘汰的那一代要重新查一次,而重新查的结果会进新一代。
		s.old = s.cur
		s.cur = make(map[string]Info, 256)
	}
	s.cur[ip] = v
}
