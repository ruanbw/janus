package geo

import (
	"hash/maphash"
	"sync"
)

// DefaultCacheEntries 全局缓存条目上限。
//
// 数量级按"独立访客 IP"估:中型租户一天几万到十几万个不同出口 IP,
// 10 万条足够让重复访问(同一 NAT 出口、同一爬虫)基本全部命中,
// 内存占用是十万个 map 条目量级(几 MB),换来的是把绝大多数查询从
// 二分查找 + 切片分配降到一次哈希查找。
const DefaultCacheEntries = 100_000

// cacheShards 分片数,必须是 2 的幂(靠位与选片)。
// 分片是为了让读写锁的竞争足够低:跳转是全并发链路,
// 一把全局锁会把所有请求排在同一行上。
const cacheShards = 16

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

// Cached 返回带缓存的 Lookup。entries 是全局条目上限(会均分到各分片),
// <= 0 时用 DefaultCacheEntries。
//
// 负结果(查不到)同样进缓存,而且是这里最要紧的一条:一个查不到的 IP 如果每次
// 都重新去查,散落在未知网段与私网地址上的流量会把查询成本打满——
// 缓存的收益恰恰主要来自这些"反复查不到"的地址。
func Cached(inner Lookup, entries int) Lookup {
	if entries <= 0 {
		entries = DefaultCacheEntries
	}
	c := &cached{inner: inner, perShard: entries / cacheShards}
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
	if v, ok := sh.get(ip); ok {
		return v
	}
	v := c.inner.Lookup(ip)
	sh.put(ip, v, c.perShard)
	return v
}

func (s *shard) get(ip string) (Info, bool) {
	s.mu.RLock()
	v, ok := s.cur[ip]
	if !ok {
		v, ok = s.old[ip]
	}
	s.mu.RUnlock()
	return v, ok
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
