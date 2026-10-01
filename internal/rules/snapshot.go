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
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"janus/internal/store"
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
	mu sync.RWMutex
	// items 是"每个租户的缓存状态":快照 + 版本号(gen)。
	// 快照可能为 nil(加载中、或加载失败)——那种情况下这条记录只承载版本号。
	items map[int64]*cacheEntry
	// loads 按租户分片的惊群保护:同一租户的并发请求只触发一次加载,
	// 不同租户互不阻塞(见 tenantLocks 的注释)。
	loads  *tenantLocks
	loader Loader
	ttl    time.Duration
	log    *slog.Logger
}

// cacheEntry 一个租户的缓存状态。
//
// gen 是这个租户的"失效代数":Invalidate 每调用一次就自增,Get 在**加载开始时**
// 记下当时的 gen,写回前再比对一次。不一致就丢弃本次加载结果 ——
// 见 Cache.Get 里"丢失效竞态"那段注释。
type cacheEntry struct {
	snap *Snapshot
	gen  uint64
}

// tenantLocks 按租户分片的加载锁:每租户一把,互不阻塞,且会在没人用时回收。
//
// 为什么不用一把全局锁:重建一份快照是一次 DB 往返 + 最多 200 条规则的预编译
// (含 regexp.Compile)。一把全局锁意味着**一个租户的慢查询会阻塞所有其他租户**的
// 跳转求值 —— 自托管多租户下这是直接的跨租户串行瓶颈。
//
// 为什么不用 golang.org/x/sync/singleflight:它在 go.mod 里只是 indirect,
// 提为直接依赖要动 go.mod/go.sum(本波明确要求避免);而且 singleflight 的
// 结果合并语义正好和"失效版本号"打架 —— 它会把 B 的在途结果复用给 A,
// 而 A 拿到的正是我们要防的那份过期数据。自己写一把按租户的锁反而更直白。
//
// 回收:refs 归零(既没有持有者也没有等待者)时把这条从表里摘掉,
// 免得长期运行按租户数无界增长。
type tenantLocks struct {
	mu    sync.Mutex
	locks map[int64]*tenantLock
}

type tenantLock struct {
	mu   sync.Mutex
	refs int
}

// lock 锁住某个租户,返回解锁函数(必须调用)。
func (t *tenantLocks) lock(tenantID int64) func() {
	t.mu.Lock()
	if t.locks == nil {
		t.locks = make(map[int64]*tenantLock)
	}
	l := t.locks[tenantID]
	if l == nil {
		l = &tenantLock{}
		t.locks[tenantID] = l
	}
	// refs 在表锁内自增:等锁的 goroutine 也持有一份引用,
	// 于是持有者解锁时表里这条一定还在,不会出现"有人等着一把已被摘掉的锁"。
	l.refs++
	t.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		t.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(t.locks, tenantID)
		}
		t.mu.Unlock()
	}
}

// size 当前持有的租户锁数(测试与观测用)。
func (t *tenantLocks) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.locks)
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
		items:  make(map[int64]*cacheEntry),
		loads:  &tenantLocks{locks: make(map[int64]*tenantLock)},
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
		return ruleBefore(&sorted[i], &sorted[j])
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

// ruleBefore 求值顺序:优先级升序,同优先级按 id 升序。
// 快照构造与仿真链路共用它——顺序错一次,首命中即裁决的诊断结论就是假的。
func ruleBefore(a, b *store.Rule) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	// 未保存的草稿没有 id,排在同优先级的存量规则之后:它还不存在,
	// 没理由抢在存量规则前面改变裁决顺序。
	if (a.ID == 0) != (b.ID == 0) {
		return b.ID == 0
	}
	return a.ID < b.ID
}

// Get 取某租户的快照:命中缓存直接返回,否则加载一份并整体原子替换(惰性加载)。
// 加载出错时返回空快照(fail-open,放行),不写缓存。
func (c *Cache) Get(ctx context.Context, tenantID int64) *Snapshot {
	if c == nil || c.loader == nil {
		return &Snapshot{BuiltAt: time.Now(), log: slog.Default()}
	}
	if snap := c.cached(tenantID); snap != nil {
		return snap
	}
	// 慢路径整体兜住 panic:README 承诺"快照加载失败一律按未命中继续",
	// 而这一段原来只覆盖 loader **返回 error**;loader **panic**(比如某个
	// 第三方 VisitorContext 实现有问题、或者将来 loader 里加了会炸的代码)
	// 会一路冒到全局 recover 变成 500。与 Evaluate 的 recover 对称。
	snap := &Snapshot{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				c.log.Error("加载租户规则 panic,按无规则放行(fail-open)",
					"tenant", tenantID, "panic", rec, "stack", string(debug.Stack()))
				snap = &Snapshot{BuiltAt: time.Now(), log: c.log}
			}
		}()
		snap = c.load(ctx, tenantID)
	}()
	return snap
}

// cached 读缓存:有且未过期才算命中。
func (c *Cache) cached(tenantID int64) *Snapshot {
	c.mu.RLock()
	e := c.items[tenantID]
	var snap *Snapshot
	if e != nil {
		// 必须在锁内把指针取出来。锁外再读 e.snap 就成了与 Invalidate
		// (e.snap = nil)的并发读写 —— 这正是 go test -race 会抓的那一类。
		snap = e.snap
	}
	c.mu.RUnlock()
	// 锁外只碰 snap 自己:快照构造完就不再改(整体原子替换),
	// 拿到旧指针的读者可以安全地用完它。
	if snap == nil || c.stale(snap) {
		return nil
	}
	return snap
}

// load 慢路径:按租户加锁 → 复查缓存 → 读库 → 预编译 → 写回。
//
// 这里有两道独立的锁,各管一件事:
//   - loads 的租户锁:惊群保护(同租户只加载一次)且跨租户并行;
//   - items 的 gen 版本号:防"加载途中发生的失效被这次加载覆盖掉"。
//
// 两者不能互相替代 —— 租户锁不参与 Invalidate,光有它拦不住下面的丢失效竞态。
func (c *Cache) load(ctx context.Context, tenantID int64) *Snapshot {
	unlock := c.loads.lock(tenantID)
	defer unlock()

	if snap := c.cached(tenantID); snap != nil {
		return snap
	}

	// 在**碰数据库之前**就把这条租户的记录建好并记下当前代数。
	// 这一步是丢失效竞态能被看见的前提:记录先存在,后面到达的 Invalidate
	// 才找得到它、才谈得上自增代数;否则 Invalidate 落在一个还不存在的 key 上,
	// 自增了也没人读。
	c.mu.Lock()
	e := c.items[tenantID]
	if e == nil {
		e = &cacheEntry{}
		c.items[tenantID] = e
	}
	startGen := e.gen
	c.mu.Unlock()

	rules, err := c.loader(ctx, tenantID)
	if err != nil {
		c.log.Error("加载租户规则失败,按无规则放行(fail-open)", "tenant", tenantID, "err", err)
		return &Snapshot{BuiltAt: time.Now(), log: c.log}
	}
	snap := NewSnapshot(rules, c.log)

	// 写回前比对代数:加载期间发生过 Invalidate,说明我们手上这份是旧的
	// (它读的是失效之前提交的库状态)。写回去等于让刚保存的规则在整个 TTL 内
	// 完全不生效,而且没有任何报错 —— ADR-0008 把这条列为不可接受。
	// 这里丢弃本次结果:本次调用按未命中继续(fail-open),下一次 Get 会重新加载。
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.gen != startGen {
		c.log.Warn("租户规则在加载期间变更,丢弃本次快照结果(按未命中继续)",
			"tenant", tenantID)
		return &Snapshot{BuiltAt: time.Now(), log: c.log}
	}
	e.snap = snap
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
	if e := c.items[tenantID]; e != nil {
		// 记录留着(代数留着),只丢快照:正在加载的 goroutine 靠这个代数
		// 发现自己手里的结果是旧的。key 本身不删,否则代数会一起丢,
		// 正在加载的 goroutine 就看不出发生过失效了。
		e.snap = nil
		e.gen++
	}
	c.mu.Unlock()
}

// InvalidateAll 清空全部租户的快照(平台级操作时使用)。
func (c *Cache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	// 每个租户的代数都要自增:正在加载的 goroutine 也要看出"全局失效过"。
	for _, e := range c.items {
		e.snap = nil
		e.gen++
	}
	c.mu.Unlock()
}

// Len 当前缓存的租户数(测试与观测用)。
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	n := 0
	for _, e := range c.items {
		if e.snap != nil {
			n++
		}
	}
	return n
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
		return decisionOf(c), true
	}
	return Decision{}, false
}

// decisionOf 由命中的规则展开裁决。
// Evaluate 与 Simulate 共用这一个构造函数:两份结论只要有一个字段对不上,
// "模拟器说的"和"线上做的"就等于两套行为,那是最难查的一类错。
func decisionOf(c *Compiled) Decision {
	return Decision{
		RuleID:      c.Rule.ID,
		Name:        c.Rule.Name,
		Action:      c.Rule.Action,
		Destination: c.Rule.Destination,
		Priority:    c.Rule.Priority,
		PageMode:    c.Rule.PageMode,
		CustomHTML:  c.Rule.CustomHTML,
	}
}

// 规则仿真(诊断链路):同一份快照、同一套条件求值,额外把"每条规则为什么
// 命中/不命中"记下来,供 POST /api/rules/simulate 回答
// 「这个访客会命中哪条规则、凭什么」。
//
// 与 Evaluate 的关系只有一条:结论必须一致。Simulate 不是第二套判定,
// 它是 Evaluate 的同源旁路——命中的判定来自同一个 (*Compiled).matchAll,
// 条件明细来自同一个 (*compiledCond).match。顺序也共用同一个比较器,
// 否则"首条命中即裁决"的诊断结果就是假的。
const (
	// StepStatusHit 首条命中,裁决就是这条规则。
	StepStatusHit = "hit"
	// StepStatusSkip 不产生裁决:未命中、不适用于该短链,或已被首条命中盖过。
	StepStatusSkip = "skip"
	// StepStatusDisabled 规则已停用(只可能来自草稿:存量停用规则不进快照)。
	StepStatusDisabled = "disabled"
)

// ConditionTrace 单条条件的求值痕迹:这次比的是"实际值"对"期望值"。
type ConditionTrace struct {
	Field     string   `json:"field"`
	Operator  string   `json:"operator"`
	Expected  []string `json:"expected"`
	Actual    string   `json:"actual"`
	Available bool     `json:"available"`
	Matched   bool     `json:"matched"`
	Seen      int      `json:"seen,omitempty"`
	// Description 一句人能读的解释。判定在前端复刻一遍就会漂,文案由后端出。
	Description string `json:"description"`
}

// StepTrace 决策链上的一条规则(按求值顺序排列)。
type StepTrace struct {
	RuleID     int64            `json:"ruleId"`
	RuleName   string           `json:"ruleName"`
	Priority   int              `json:"priority"`
	Scope      string           `json:"scope"`
	Action     string           `json:"action"`
	Status     string           `json:"status"`
	Reason     string           `json:"reason"`
	Logic      string           `json:"logic,omitempty"`
	Draft      bool             `json:"draft,omitempty"`
	Conditions []ConditionTrace `json:"conditions,omitempty"`
}

// SimulationResult 一次仿真的结果。
type SimulationResult struct {
	Verdict Decision    // 命中规则展开的裁决(未命中时是零值)
	Matched bool        // 是否命中
	Steps   []StepTrace // 决策链
	// Error 求值期异常被兜住时的说明。诊断接口如实上报,线上 Evaluate 仍然 fail-open。
	Error string `json:"error,omitempty"`
}

// Simulate 按 Evaluate 的顺序与语义回放一遍求值,并记录过程。
//
// linkID 是这次访问命中的短链(0 = 没有具体短链,只按全局规则推演);
// onlyRuleID 非空时只看这一条存量规则(诊断"单条规则自己生效吗");
// draft 是未保存的草稿规则,按 id 顶替同 id 的存量规则,不写回快照。
// 草稿没启用时也会占一步并标成 disabled:租户需要看到"停用了就不会拦"。
func (s *Snapshot) Simulate(ctx VisitorContext, linkID int64, onlyRuleID *int64, draft *Compiled) (res SimulationResult) {
	defer func() {
		if rec := recover(); rec != nil {
			res.Error = fmt.Sprintf("求值异常,已按未命中处理: %v", rec)
			if s != nil && s.log != nil {
				s.log.Error("规则仿真异常,按未命中处理", "panic", rec, "stack", string(debug.Stack()))
			}
		}
	}()
	if ctx == nil || s == nil {
		return SimulationResult{}
	}
	for _, item := range s.simChain(onlyRuleID, draft) {
		if res.Matched {
			// Evaluate 在这里就 return 了:后面的规则线上根本不看,更不该被求值。
			res.Steps = append(res.Steps, StepTrace{
				RuleID:   item.rule.Rule.ID,
				RuleName: item.rule.Rule.Name,
				Priority: item.rule.Rule.Priority,
				Scope:    item.rule.Rule.Scope,
				Action:   item.rule.Rule.Action,
				Status:   StepStatusSkip,
				Reason:   "前一条规则已命中(首命中即裁决),线上不会再求值这条。",
				Draft:    item.draft,
			})
			continue
		}
		step := traceRule(item.rule, ctx, linkID, item.draft)
		res.Steps = append(res.Steps, step)
		if step.Status == StepStatusHit {
			res.Verdict = decisionOf(item.rule)
			res.Matched = true
		}
	}
	return res
}

// simChainItem 决策链上的一项:存量规则,或顶替/插入链路的草稿。
type simChainItem struct {
	rule  *Compiled
	draft bool
}

// simChain 组装决策链。取值只读快照:草稿不进 s.Rules,仿真不写回任何共享状态。
func (s *Snapshot) simChain(onlyRuleID *int64, draft *Compiled) []simChainItem {
	items := make([]simChainItem, 0, len(s.Rules)+1)
	for i := range s.Rules {
		c := &s.Rules[i]
		if onlyRuleID != nil && c.Rule.ID != *onlyRuleID {
			continue
		}
		if draft != nil && draft.Rule.ID != 0 && draft.Rule.ID == c.Rule.ID {
			items = append(items, simChainItem{rule: draft, draft: true})
			continue
		}
		items = append(items, simChainItem{rule: c})
	}
	// 草稿不只顶替存量:只回放一条规则(onlyRuleID)时,草稿也必须参与,
	// 否则"这条草稿生效吗"永远得到空链路。
	if draft != nil && !chainHasRuleID(items, draft.Rule.ID) {
		items = append(items, simChainItem{rule: draft, draft: true})
	}
	// 草稿可能带来新的优先级/新 id,按快照同一套顺序重排一次。
	sort.SliceStable(items, func(i, j int) bool {
		return ruleBefore(&items[i].rule.Rule, &items[j].rule.Rule)
	})
	return items
}

func chainHasRuleID(items []simChainItem, id int64) bool {
	if id == 0 {
		return false
	}
	for _, it := range items {
		if it.rule.Rule.ID == id {
			return true
		}
	}
	return false
}

// traceRule 回放一条规则:先按权威路径判命中(Evaluate 用的同一个 matchAll),
// 再用同一个 compiledCond.match 逐条留痕,两者不可能各说各话。
func traceRule(c *Compiled, ctx VisitorContext, linkID int64, draft bool) StepTrace {
	step := StepTrace{
		RuleID:   c.Rule.ID,
		RuleName: c.Rule.Name,
		Priority: c.Rule.Priority,
		Scope:    c.Rule.Scope,
		Action:   c.Rule.Action,
		Draft:    draft,
	}
	if !c.Rule.Enabled {
		step.Status = StepStatusDisabled
		step.Reason = "规则已停用:线上求值时停用规则不进快照,不参与裁决。"
		return step
	}
	if !c.applies(linkID) {
		step.Status = StepStatusSkip
		if !c.appliesAll && len(c.linkIDs) == 0 {
			step.Reason = "「指定短链」但一条短链都没关联:零关联的规则恒不命中,不兜底成全局。"
		} else {
			step.Reason = fmt.Sprintf("该短链不在规则的关联列表里(规则只关联了 %d 条短链),线上不参与求值。", len(c.linkIDs))
		}
		return step
	}
	step.Logic = c.Rule.Logic
	matched := c.matchAll(ctx)
	if c.Rule.RuleType == store.RuleTypeExpression {
		if matched {
			step.Status = StepStatusHit
			step.Reason = "表达式求值结果为 true: " + c.Rule.Expression
			return step
		}
		step.Status = StepStatusSkip
		step.Reason = "表达式求值结果为 false: " + c.Rule.Expression
		return step
	}
	step.Conditions = traceConds(c.root, ctx)
	if matched {
		step.Status = StepStatusHit
		step.Reason = "首条命中即裁决(First-Match-Wins):后面的规则不再求值。"
		return step
	}
	step.Status = StepStatusSkip
	step.Reason = logicMissReason(c, step.Conditions)
	return step
}

// traceConds 逐条条件留痕。只读快照与画像,不改判定。
func traceConds(root compiledNode, ctx VisitorContext) []ConditionTrace {
	out := make([]ConditionTrace, 0, root.leafCount())
	collectTraces(root, ctx, &out)
	return out
}

// collectTraces 走遍整棵树逐条留痕,不按短路停下来:
// 决策链的价值就在于"另一条为什么没生效",只留走到的那一支等于让用户自己猜。
func collectTraces(n compiledNode, ctx VisitorContext, out *[]ConditionTrace) {
	for i := range n.conds {
		cc := &n.conds[i]
		tr := ConditionTrace{Field: cc.field, Operator: cc.op, Expected: cc.raw}
		tr.Actual, tr.Available = ctx.Field(cc.field)
		if cc.op == OpDuplicated {
			tr.Seen = seenCount(ctx, cc.field, tr.Actual)
		}
		tr.Matched = cc.match(ctx)
		tr.Description = describeCond(&tr)
		*out = append(*out, tr)
	}
	for i := range n.children {
		collectTraces(n.children[i], ctx, out)
	}
}

// logicMissReason 解释"为什么没命中":成立了几条、缺数据的有几条。
func logicMissReason(c *Compiled, conds []ConditionTrace) string {
	hits, missing := 0, 0
	for _, t := range conds {
		if t.Matched {
			hits++
		}
		if !t.Available {
			missing++
		}
	}
	need := "全部条件都要成立"
	switch {
	case c.nested:
		// 树形态下一句"全部/任一"说清不了,只报叶子统计
		need = "条件树求值未通过"
	case !c.logicAll:
		need = "任一条件成立即可"
	}
	msg := fmt.Sprintf("%s,实际成立 %d/%d 条", need, hits, len(conds))
	if c.nested {
		msg += "叶子条件"
	}
	msg += "。"
	if missing > 0 {
		msg += fmt.Sprintf("其中 %d 条字段取不到数据,恒不成立。", missing)
	}
	return msg
}

// describeCond 把一次条件比较翻成人话。
func describeCond(t *ConditionTrace) string {
	name := fieldLabel(t.Field)
	if !t.Available {
		// 取不到数据是本引擎最关键的不变式:空值既不是"匹配",也不是"不匹配的反面"。
		return fmt.Sprintf("%s 本次没有数据源,条件恒不成立。", name)
	}
	if t.Operator == OpDuplicated {
		verdict := "未达到"
		if t.Matched {
			verdict = "已达到"
		}
		return fmt.Sprintf("%s「%s」出现 %d 次,%s阈值 %s 次。", name, t.Actual, t.Seen, verdict, t.Expected[0])
	}
	phrase := comparePhrase(t.Operator, t.Expected)
	if t.Matched {
		return fmt.Sprintf("%s 实际为「%s」,满足条件(%s)。", name, t.Actual, phrase)
	}
	return fmt.Sprintf("%s 实际为「%s」,不满足条件(%s)。", name, t.Actual, phrase)
}

func comparePhrase(op string, expected []string) string {
	values := strings.Join(expected, " / ")
	switch op {
	case OpIn:
		return "在 " + values + " 中"
	case OpNotIn:
		return "不在 " + values + " 中"
	case OpEq:
		return "等于 " + values
	case OpNeq:
		return "不等于 " + values
	case OpContains:
		return "包含 " + values
	case OpNotContains:
		return "不包含 " + values
	case OpStartsWith:
		return "以 " + values + " 开头"
	case OpEndsWith:
		return "以 " + values + " 结尾"
	case OpInCIDR:
		return "在网段 " + values + " 内"
	case OpGT:
		return "大于 " + values
	case OpLT:
		return "小于 " + values
	case OpRegex:
		return "匹配正则 " + values
	case OpDuplicated:
		return "出现次数达到 " + values
	}
	return op + " " + values
}

// fieldLabel 字段的中文名(诊断文案用;判定仍以字段常量为准)。
func fieldLabel(field string) string {
	switch field {
	case FieldIP:
		return "来源 IP"
	case FieldIPAttr:
		return "IP 归属"
	case FieldCountry:
		return "国家/地区"
	case FieldASN:
		return "ASN"
	case FieldLang:
		return "语言"
	case FieldRef:
		return "来源站点"
	case FieldUTM:
		return "UTM 来源"
	case FieldUA:
		return "UserAgent"
	case FieldDevType:
		return "设备类型"
	case FieldOS:
		return "操作系统"
	case FieldBrowser:
		return "浏览器"
	case FieldPath:
		return "请求路径"
	case FieldDomain:
		return "域名"
	}
	return field
}

// seenCount 读"重复出现"计数,与 (*compiledCond).match 的 duplicated 分支同一份键空间。
func seenCount(ctx VisitorContext, field, value string) int {
	if value == "" {
		return 0
	}
	key := SeenKey(field, value)
	switch f := ctx.(type) {
	case *Fact:
		if f.Seen != nil {
			return f.Seen[key]
		}
	case Fact:
		if f.Seen != nil {
			return f.Seen[key]
		}
	}
	return 0
}

// CompileDraft 预编译一条未保存的草稿规则(规则编辑器的"改完先试一遍")。
// 与存量规则走同一套编译逻辑:草稿编译不出来,就不给结论。
func CompileDraft(r store.Rule, log *slog.Logger) (*Compiled, bool) {
	if log == nil {
		log = slog.Default()
	}
	c, ok := compileRule(r, log)
	if !ok {
		return nil, false
	}
	return &c, true
}
