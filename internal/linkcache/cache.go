// Package linkcache 缓存跳转热路径上的短链快照。
//
// 一次跳转原本要付 GetDomainByFQDN + LookupLinkForVisit(JOIN + 一次 link_targets)
// + PickTarget(又一次 link_targets) 的代价,而短链内容在两次管理端改动之间是静止的。
// 这里把它收敛成"不可变 Record 快照 + 精确失效":
//
//   - 快照不可变:Record 一旦交给缓存就不再修改,读侧拿到的是同一个指针,
//     多个并发跳转共享它(跳转链路只读,选目标用 store.TargetForRR)。
//     拷贝一份"内容相同的新对象"会让每个 key 的 TTL 起点各不相同,热路径也多一次分配。
//   - 不做负缓存:ErrNotFound 与加载失败都不写缓存。短码刚创建就必须立刻可用;
//     一次瞬时 DB 抖动也不该被固化成 TTL 长的 404。
//   - 惊群保护:同一 (domainID, code) 的并发 Get 只触发一次加载,等待者复用同一个结果。
//   - 缓存键必须含 domainID:同一租户的多个域名下允许同名短码,只按 code 做键
//     会把 A 域名下的目标错送到 B 域名。
//
// 失效靠显式调用:短链/域名/租户变更后必须调 InvalidateCode / InvalidateDomain /
// InvalidateTenant;TTL 只是漏调失效时的兜底。
package linkcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNotFound 表示该 (domainID, code) 当前不存在(或被 loader 判定为不可见)。
// loader 直接返回它即可;本包原样透出,且**绝不**缓存它。
var ErrNotFound = errors.New("linkcache: not found")

// defaultTTL 兜底存活时间。跳转链路的从库延迟与漏调失效都靠它收口;
// 取 60s:管理端改目标后最迟一分钟生效,而每个活跃短链一分钟只回源一次。
const defaultTTL = time.Minute

// Record 一条短链在跳转热路径上需要的全部字段。
//
// 交给缓存之后**不得再被修改**,包括 Targets 这个切片:并发的跳转共享同一个 *Record,
// 就地改写任何一个字段都等于在没有锁的情况下写共享内存。
type Record struct {
	LinkID         int64
	TenantID       int64
	DomainID       int64
	Status         string
	Deleted        bool
	LinkType       string
	LandingSource  string
	LandingURL     string
	RedirectStatus int
	RulesEnabled   bool
	Targets        []string
}

// Loader 回源加载一条短链快照(未命中/不可见返回 ErrNotFound)。
type Loader func(ctx context.Context, domainID int64, code string) (*Record, error)

// Stats 累计统计。失效不会清零:清零会让"缓存大量失效"在监控上看成"缓存刚开始工作",
// 事故复盘时读到的掉底曲线是假的。
type Stats struct {
	Hits uint64 // 直接从缓存里拿到 Record 的 Get 次数
	// Misses 没拿到缓存的 Get 次数。每个 Get 恰好记一次 hit 或 miss,
	// 所以在同一次回源上等待的那些也算 miss(Hits+Misses 恒等于 Get 总次数)。
	Misses uint64
	// Loads 成功回源的次数(loader 返回了非 nil Record)。等待者不重复计数。
	Loads uint64
	// LoadErrors loader 返回真实错误的次数。ErrNotFound 不算错误(那是"现在没有",
	// 不是加载失败),否则一次探活/扫段的 404 会把错误率打成红。
	LoadErrors uint64
}

// key 缓存键。(domainID, code) 才唯一确定一条短链。
type key struct {
	domainID int64
	code     string
}

// entry 一个缓存项。rec 只在慢路径的写回里出现(nil 项不入表)。
type entry struct {
	rec      *Record
	loadedAt time.Time
}

// Cache 短链快照缓存。并发安全。
type Cache struct {
	mu      sync.RWMutex
	entries map[key]*entry
	// byDomain / byTenant 是失效用的倒排索引:按域名/租户失效时不必扫描全表。
	// 它们只索引已加载的条目,并在条目被移除时一并摘除(内层集合空了连外层一起删),
	// 否则长期运行下索引本身会按"历史出现过的域名/租户数"无界增长。
	byDomain map[int64]map[key]struct{}
	byTenant map[int64]map[key]struct{}

	loading *keyLocks
	loader  Loader
	ttl     time.Duration
	clock   func() time.Time

	// epoch 每次 Invalidate* 自增。慢路径在回源前记下它、写回前再比对:
	// 不一致说明这次加载横跨了一次失效,读到的可能是失效之前提交的旧行。
	// 那种结果只返回给本次调用,不写缓存 —— 否则刚保存的目标要在缓存里躺满一个 TTL。
	epoch atomic.Uint64

	hits       atomic.Uint64
	misses     atomic.Uint64
	loads      atomic.Uint64
	loadErrors atomic.Uint64
}

// New 构造缓存。loader 为空时 Get 会 panic(调用方必须提供回源函数)。
func New(loader Loader) *Cache {
	return &Cache{
		entries:  make(map[key]*entry),
		byDomain: make(map[int64]map[key]struct{}),
		byTenant: make(map[int64]map[key]struct{}),
		loading:  &keyLocks{locks: make(map[key]*keyLock)},
		loader:   loader,
		ttl:      defaultTTL,
	}
}

// SetTTL 设置兜底存活时间;<=0 视为默认值。
// 只在构造期/测试里调用,不得与 Get 并发(与 rules.Cache 的 Option 同约定)。
func (c *Cache) SetTTL(d time.Duration) {
	if d <= 0 {
		d = defaultTTL
	}
	c.ttl = d
}

// SetClock 替换时钟(测试用假时钟)。所有"是否过期"的判断都走它,
// 于是 TTL 语义不必由墙钟决定,测试也不需要 sleep。调用约定同 SetTTL。
func (c *Cache) SetClock(now func() time.Time) {
	c.clock = now
}

// Get 取一条短链快照:命中缓存直接返回同一个 *Record,否则回源一次并写入缓存。
func (c *Cache) Get(ctx context.Context, domainID int64, code string) (*Record, error) {
	k := key{domainID: domainID, code: code}
	if rec := c.cached(k); rec != nil {
		c.hits.Add(1)
		return rec, nil
	}
	// 每个 Get 恰好记一次 miss(包括随后在同一次加载上等待的那些):
	// 它们确实没从缓存里拿到东西,算成 hit 会让冷启动风暴在命中率上看不出来。
	c.misses.Add(1)
	return c.load(ctx, k)
}

// load 慢路径:按 key 加锁 → 复查缓存 → 回源 → 写回。
// 复查是惊群保护的关键:并发等待者拿到锁时,前一个已经把结果写进缓存了。
func (c *Cache) load(ctx context.Context, k key) (*Record, error) {
	unlock := c.loading.lock(k)
	defer unlock()

	if rec := c.cached(k); rec != nil {
		return rec, nil
	}
	startEpoch := c.epoch.Load()
	rec, err := c.loader(ctx, k.domainID, k.code)
	if err != nil {
		// ErrNotFound 是"现在没有",不是"永远没有":短码刚建好就得立刻可用,
		// 负缓存会让"后台建好短链、刷新还是 404"变成一个要等 TTL 才恢复的谜。
		if !errors.Is(err, ErrNotFound) {
			c.loadErrors.Add(1)
		}
		return nil, err
	}
	if rec == nil {
		// loader 违反约定返回 (nil, nil):原样透出,不入缓存(缓存只是搬运工,不做语义改写)。
		return nil, nil
	}
	c.loads.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch.Load() != startEpoch {
		// 加载期间发生过失效:手里这份可能是失效前的旧行,只服务本次调用。
		return rec, nil
	}
	if prev := c.entries[k]; prev != nil {
		// 同一 key 再次加载(过期/失效后):先摘掉旧索引,免得留下指向旧租户的陈旧项。
		c.unindexLocked(k, prev.rec)
	}
	c.entries[k] = &entry{rec: rec, loadedAt: c.now()}
	c.indexLocked(k, rec)
	return rec, nil
}

// cached 读缓存:有且未过期才算命中。过期项留在表里等下次写回覆盖 ——
// 它的存在不影响正确性(读取一律先判过期),只影响 Len 的读数。
func (c *Cache) cached(k key) *Record {
	c.mu.RLock()
	var rec *Record
	var loadedAt time.Time
	if e := c.entries[k]; e != nil {
		// 指针与时间必须在锁内取出来:e 的字段会被慢路径改写(整体替换这条 entry),
		// 锁外读 e.rec 就是与写回并发的一对读写。
		rec, loadedAt = e.rec, e.loadedAt
	}
	c.mu.RUnlock()
	// 锁外只碰 rec 自己:Record 交给缓存后不可变,拿到旧指针的读者可以安全用完。
	if rec == nil || c.stale(loadedAt) {
		return nil
	}
	return rec
}

// stale 是否超过存活时间。
func (c *Cache) stale(loadedAt time.Time) bool {
	return c.ttl > 0 && !c.now().Before(loadedAt.Add(c.ttl))
}

// now 当前时刻(假时钟优先)。
func (c *Cache) now() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

// InvalidateCode 丢弃单条短链的快照(短链 CRUD 后调用)。
func (c *Cache) InvalidateCode(domainID int64, code string) {
	c.epoch.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(key{domainID: domainID, code: code})
}

// InvalidateDomain 丢弃某域名下**全部**短链的快照(域名级配置变更后调用)。
// 只清这个域名:同租户其他域名下的同名短码必须继续吃缓存,否则一次域级改动
// 会把整个租户的跳转全打到库里。
func (c *Cache) InvalidateDomain(domainID int64) {
	c.epoch.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	// 先把整块索引摘掉再逐条清条目:正在被 range 的就是这块集合,边遍历边删它
	// 虽然合法,但把"这一域名没有条目了"与"逐条摘除"混在一起读起来更容易出错。
	doomed := c.byDomain[domainID]
	delete(c.byDomain, domainID)
	for k := range doomed {
		e := c.entries[k]
		if e == nil {
			continue
		}
		delete(c.entries, k)
		if e.rec != nil {
			dropIndexKey(c.byTenant, e.rec.TenantID, k)
		}
	}
}

// InvalidateTenant 丢弃某租户**跨域名**的全部快照(整租户停用、规则/配额批量变更后调用)。
func (c *Cache) InvalidateTenant(tenantID int64) {
	c.epoch.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	doomed := c.byTenant[tenantID]
	delete(c.byTenant, tenantID)
	for k := range doomed {
		if e := c.entries[k]; e != nil {
			delete(c.entries, k)
			// 这里要两边都摘:域的集合正被别处使用,不能整块删掉。
			c.unindexLocked(k, e.rec)
		}
	}
}

// Len 当前缓存的条目数(含已过期未覆盖的项,不含从未加载成功的 key)。
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stats 累计统计快照。
func (c *Cache) Stats() Stats {
	return Stats{
		Hits:       c.hits.Load(),
		Misses:     c.misses.Load(),
		Loads:      c.loads.Load(),
		LoadErrors: c.loadErrors.Load(),
	}
}

// removeLocked 移除单条条目并摘掉它的索引(调用方持写锁)。
func (c *Cache) removeLocked(k key) {
	e := c.entries[k]
	if e == nil {
		return
	}
	delete(c.entries, k)
	c.unindexLocked(k, e.rec)
}

// indexLocked 登记一条条目的失效索引(调用方持写锁)。
func (c *Cache) indexLocked(k key, rec *Record) {
	if rec == nil {
		return
	}
	addIndexKey(c.byDomain, k.domainID, k)
	addIndexKey(c.byTenant, rec.TenantID, k)
}

// unindexLocked 摘掉一条条目的失效索引(调用方持写锁)。
func (c *Cache) unindexLocked(k key, rec *Record) {
	if rec == nil {
		return
	}
	dropIndexKey(c.byDomain, k.domainID, k)
	dropIndexKey(c.byTenant, rec.TenantID, k)
}

// addIndexKey 把 key 登记进 idx[id]。
func addIndexKey(idx map[int64]map[key]struct{}, id int64, k key) {
	set := idx[id]
	if set == nil {
		set = make(map[key]struct{})
		idx[id] = set
	}
	set[k] = struct{}{}
}

// dropIndexKey 把 key 从 idx[id] 摘掉;集合空了连 id 一起删,
// 否则索引会按历史出现过的域名/租户数一直涨下去。
func dropIndexKey(idx map[int64]map[key]struct{}, id int64, k key) {
	set := idx[id]
	if set == nil {
		return
	}
	delete(set, k)
	if len(set) == 0 {
		delete(idx, id)
	}
}
