// 租户级规则快照:整租户的规则一次性读进内存、预编译、按租户缓存,
// 跳转热路径上只做纯内存求值(spec D7)。
//
// 三条关键不变式:
//  1. 快照不可变:构造完成后只读,读侧不需要再拿锁(RLock 只用来取指针);
//     规则变更走"整体原子替换"——旧快照在被替换前仍可安全完成本次求值。
//  2. 加载失败 fail-open:按"这个租户没有规则"放行,且不把失败结果写进缓存
//     (否则一次数据库抖动会被缓存成"这个租户永远没有规则")。
//  3. 失效靠显式调用:规则/关联变更后必须 Invalidate(该租户);
//     TTL 只是漏调 Invalidate 时的兜底,让配置改动最迟一个 TTL 生效。
package rules

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"cloak/internal/store"
)

// DefaultSnapshotTTL 快照兜底存活时间。规则/关联变更后应显式 Invalidate,
// 这个 TTL 只防"漏调 Invalidate 导致规则永远不生效"这类静默失效:
// 代价只是每个活跃租户每分钟一次很小的整租户查询。
const DefaultSnapshotTTL = time.Minute

// Snapshot 一个租户在某一时刻的规则集合,按求值顺序(priority 升序, id 升序)排好。
// 构造后不可变,多 goroutine 可并发读。
type Snapshot struct {
	Rules   []Compiled
	BuiltAt time.Time
	log     *slog.Logger
}

// Loader 加载某租户的全部启用规则。
// 签名与 (*store.Store).RulesForTenant 一致,可直接以方法值注入:
// rules.NewCache(st.RulesForTenant)。
type Loader func(ctx context.Context, tenantID int64) ([]store.Rule, error)

// Cache 租户 → 快照 的缓存。整份快照原子替换,读侧无锁遍历。
type Cache struct {
	mu     sync.RWMutex
	items  map[int64]*Snapshot
	loader Loader
	// loadMu 串行化加载:缓存失效瞬间的并发请求只触发一次加载,避免惊群
	loadMu sync.Mutex
	ttl    time.Duration
	log    *slog.Logger
}

// Option Cache 构造选项。
type Option func(*Cache)

// WithLogger 指定日志器(默认 slog.Default())。
func WithLogger(l *slog.Logger) Option {
	return func(c *Cache) {
		if l != nil {
			c.log = l
		}
	}
}

// WithTTL 指定快照兜底存活时间(<=0 表示只靠 Invalidate 失效)。
func WithTTL(d time.Duration) Option {
	return func(c *Cache) { c.ttl = d }
}

// NewCache 构造规则快照缓存。loader 为空时 Get 恒返回空快照(等价于"没有规则")。
func NewCache(loader Loader, opts ...Option) *Cache {
	c := &Cache{
		items:  make(map[int64]*Snapshot),
		loader: loader,
		ttl:    DefaultSnapshotTTL,
		log:    slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewSnapshot 预编译一份规则快照。
// 排序在本包里做而不是依赖数据库 ORDER BY:求值顺序就是裁决顺序(spec D4 首命中即裁决),
// 不能因为某个 Loader 忘了排序而改变语义。
// 停用规则不进快照(它们不参与求值);某条规则的条件全部无法编译时整条丢弃,
// 坏条件不拖垮整份快照,也不会让规则退化成"恒命中的兜底"。
func NewSnapshot(rules []store.Rule, log *slog.Logger) *Snapshot {
	if log == nil {
		log = slog.Default()
	}
	sorted := make([]store.Rule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].ID < sorted[j].ID
	})
	snap := &Snapshot{BuiltAt: time.Now(), log: log}
	for _, r := range sorted {
		if !r.Enabled {
			continue
		}
		if c, ok := compileRule(r, log); ok {
			snap.Rules = append(snap.Rules, c)
		}
	}
	return snap
}

// Get 取某租户的快照:命中缓存直接返回,否则加载一份并整体原子替换(惰性加载)。
// 加载出错时返回空快照(fail-open,放行),不写缓存。
func (c *Cache) Get(ctx context.Context, tenantID int64) *Snapshot {
	if c == nil || c.loader == nil {
		return &Snapshot{BuiltAt: time.Now(), log: slog.Default()}
	}
	c.mu.RLock()
	snap := c.items[tenantID]
	c.mu.RUnlock()
	if snap != nil && !c.stale(snap) {
		return snap
	}
	// 慢路径:同一时刻只放一次加载进来(惊群保护),拿到锁后复查一次缓存
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	c.mu.RLock()
	snap = c.items[tenantID]
	c.mu.RUnlock()
	if snap != nil && !c.stale(snap) {
		return snap
	}
	rules, err := c.loader(ctx, tenantID)
	if err != nil {
		c.log.Error("加载租户规则失败,按无规则放行(fail-open)", "tenant", tenantID, "err", err)
		return &Snapshot{BuiltAt: time.Now(), log: c.log}
	}
	snap = NewSnapshot(rules, c.log)
	c.mu.Lock()
	c.items[tenantID] = snap
	c.mu.Unlock()
	return snap
}

// stale 快照是否超过兜底存活时间。
func (c *Cache) stale(snap *Snapshot) bool {
	return c.ttl > 0 && time.Since(snap.BuiltAt) >= c.ttl
}

// Invalidate 丢弃某租户的快照(规则 CRUD、关联变更后调用),下次访问重新加载。
func (c *Cache) Invalidate(tenantID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.items, tenantID)
	c.mu.Unlock()
}

// InvalidateAll 清空全部租户的快照(平台级操作时使用)。
func (c *Cache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items = make(map[int64]*Snapshot)
	c.mu.Unlock()
}

// Len 当前缓存的租户数(测试与观测用)。
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Evaluate 按求值顺序返回首个命中的裁决(spec D4:首命中即裁决,不做多规则叠加,
// 避免"拦截规则被后面的放行规则救回来"这类静默失效)。
//
// fail-open:入口 recover 兜底,panic 时记日志并按"未命中"返回——
// 风控规则永远不能把线上短链打成 500。空快照(nil 或零规则)直接返回未命中。
func (s *Snapshot) Evaluate(ctx VisitorContext, linkID int64) (dec Decision, matched bool) {
	defer func() {
		if rec := recover(); rec != nil {
			log := slog.Default()
			if s != nil && s.log != nil {
				log = s.log
			}
			log.Error("规则求值异常,按未命中处理(fail-open)", "panic", rec,
				"stack", string(debug.Stack()))
			dec, matched = Decision{}, false
		}
	}()
	if ctx == nil || s == nil || len(s.Rules) == 0 {
		return Decision{}, false
	}
	for i := range s.Rules {
		c := &s.Rules[i]
		if !c.applies(linkID) {
			continue
		}
		if !c.matchAll(ctx) {
			continue
		}
		return Decision{
			RuleID:      c.Rule.ID,
			Name:        c.Rule.Name,
			Action:      c.Rule.Action,
			Destination: c.Rule.Destination,
			Priority:    c.Rule.Priority,
			PageMode:    c.Rule.PageMode,
			CustomHTML:  c.Rule.CustomHTML,
		}, true
	}
	return Decision{}, false
}
