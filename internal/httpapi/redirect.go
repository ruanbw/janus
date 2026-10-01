package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 记 Visit → 302/301(Location=轮询目标);
// 16 — 落地页型:记 Visit → 固定 302 → landingUrl 或 /{code}/。
// 未命中(域名不解析/短码不存在)→ 404 且不记;命中但不可用(停用/逻辑删除/无目标/落地页文件缺失)
// → 404 并记一行 outcome=failed 的明细(可归属到该短链的失败不丢)。
//
// 规则裁决按 spec D4 插在"短链可用性检查之后、原目标选择之前":
//
//	短链不可用 → 404 + 失败明细          ← 规则不参与
//	规则求值(priority 升序,首条命中即定) → redirect 改写目标 / notfound 404 / throttle 429 / pass 继续
//	未命中任何规则 → 原跳转流程
//
// 三条热路径上的硬约束:
//  1. 零 DB 查询:规则集合来自按租户缓存的内存快照(Cache.Get 只读一次 map)。
//     多数租户没有规则,此时连访客画像都不构造。
//  2. 零写放大:命中不 UPDATE 任何表,只多写本次访问那一行明细。
//     「24h 命中」是列表接口从 visits 读时聚合出来的(spec D9)。
//  3. fail-open:快照加载失败/求值 panic 一律按"未命中"继续(spec 风险章节),
//     风控规则不该把线上短链打成 500。

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"janus/internal/httpapi/templates"
	"janus/internal/rules"
	"janus/internal/store"
)

// ---------- 访客可见错误页的安全响应头与租户错误页快照 ----------
//
// 信任模型(ADR-0005 已接受):租户可以在自己的域名下运行任意 HTML/JS,
// 与开放重定向是同一条边界 —— 系统信任租户自己的内容,不试图审查它。
// 因此错误页响应带 CSP 是为了**收紧**而不是"防租户" :它保证租户的 HTML
// 即使写着 <script> 也只能在访客自己的页面上跑,拿不到本站的会话 cookie、
// 也无法把内容伪装成别的 MIME 类型。

// visitorErrorCSP 访客可见错误页的 CSP。
//
// `sandbox` 让页面进入一个**不透明来源**(opaque origin):租户脚本能跑
// (allow-scripts,否则自定义页里的表单与统计脚本全废),但读不到 document.cookie
// / localStorage,发起的请求也不再带本站凭据。刻意**不给** allow-same-origin ——
// 给了它等于把这个 iframe/页面的来源等同于本站,租户脚本就能直接读会话。
// allow-forms 保留表单提交能力(很多 404 页是"搜索框"形态)。
//
// 为什么必须有这一头:租户错误页是**原样输出**的租户 HTML。在加 CSP 之前,
// 一个租户可以往自己的 404 页里写 <script>,而同一个响应会出现在**任意**访客
// 的浏览器上 —— 包括别的租户用同一个反向代理访问时。缺 X-Content-Type-Options
// 时,浏览器还会对未声明类型的响应做嗅探,text/plain 里的标记可能被当 HTML 执行。
const visitorErrorCSP = "sandbox allow-scripts allow-forms; default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data:; base-uri 'none'; form-action 'none'"

// throttleRetryAfterSeconds 429 响应携带的 Retry-After 秒数。
//
// 规则裁决的 throttle 动作本身不带窗口配置(见 store.RuleActionThrottle),
// 但缺这个头的后果是客户端无从退避:重试节奏只能靠猜,而重试越密集越会
// 继续撞上同一条限流规则。60 秒是一个明确、可预期、又不至于让访客干等太久
// 的默认值。
const throttleRetryAfterSeconds = 60

// errorPagesSnapshot 某租户某一时刻的自定义错误页内容。
//
// 与规则快照同理,构造后不可变:读侧不需要再拿锁,变更走整体原子替换。
type errorPagesSnapshot struct {
	notFound        string
	tooManyRequests string
	builtAt         time.Time
}

// DefaultErrorPagesTTL 错误页快照的兜底存活时间。
//
// 与 rules.DefaultSnapshotTTL 取同一量级并**共用同一套失效路径**:
// 租户改错误页后必须显式 Invalidate,这个 TTL 只防"漏调 Invalidate 导致改配置
// 永远不生效"这类静默失效。代价是每个活跃租户每分钟一次很小的整租户查询。
const DefaultErrorPagesTTL = time.Minute

// errorPagesCache 租户 → 自定义错误页快照 的按租户内存缓存。
//
// 与规则快照同构(惰性加载 + 整体原子替换 + TTL 兜底 + 显式 Invalidate),
// 因为两者的失效理由完全一样:配置类数据,读多写极少,且改完必须立刻生效。
//
// 为什么必须缓存:未命中是最容易触发的路径 —— 爬虫、扫描器、输错短码都会走这里。
// 没有缓存时,每一次未命中都要 SELECT 两个可能各 512KB 的 TEXT 字段,
// 等于把"攻击者随便打几个不存在的短码"变成放大器。而它恰恰是 README 1.2
// 硬约束第 1 条("零 DB 查询")与 .scratch/custom-error-pages/spec.md D3
// 都承诺过要按租户缓存的东西。
type errorPagesCache struct {
	mu    sync.RWMutex
	items map[int64]*errorPagesSnapshot
	// load 读取某租户的自定义错误页(签名与 (*store.Store).GetTenantErrorPages 一致)。
	load func(ctx context.Context, tenantID int64) (string, string, error)
	// loadMu 串行化慢路径:缓存失效瞬间的并发请求只触发一次加载,避免惊群。
	loadMu sync.Mutex
	ttl    time.Duration
	log    *slog.Logger
}

func newErrorPagesCache(load func(ctx context.Context, tenantID int64) (string, string, error)) *errorPagesCache {
	return &errorPagesCache{
		items: make(map[int64]*errorPagesSnapshot),
		load:  load,
		ttl:   DefaultErrorPagesTTL,
		log:   slog.Default(),
	}
}

// Get 取某租户的错误页快照。加载失败时返回空快照(fail-open:退回系统内置页),
// 且**不写缓存** —— 否则一次数据库抖动会被缓存成"这个租户永远没有自定义页"。
func (c *errorPagesCache) Get(ctx context.Context, tenantID int64) *errorPagesSnapshot {
	if c == nil || c.load == nil {
		return &errorPagesSnapshot{builtAt: time.Now()}
	}
	if snap := c.cached(tenantID); snap != nil {
		return snap
	}
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	if snap := c.cached(tenantID); snap != nil {
		return snap
	}
	p404, p429, err := c.load(ctx, tenantID)
	if err != nil {
		c.log.Error("加载租户错误页失败,退回内置页(fail-open)", "tenant", tenantID, "err", err)
		return &errorPagesSnapshot{builtAt: time.Now()}
	}
	snap := &errorPagesSnapshot{notFound: p404, tooManyRequests: p429, builtAt: time.Now()}
	c.mu.Lock()
	c.items[tenantID] = snap
	c.mu.Unlock()
	return snap
}

// cached 读缓存:有且未过期才算命中。
func (c *errorPagesCache) cached(tenantID int64) *errorPagesSnapshot {
	c.mu.RLock()
	snap := c.items[tenantID]
	c.mu.RUnlock()
	// 锁外只读 snap 自己的字段:它是不可变的,拿到旧指针的读者可以安全地用完它。
	if snap == nil || (c.ttl > 0 && time.Since(snap.builtAt) >= c.ttl) {
		return nil
	}
	return snap
}

// Invalidate 丢弃某租户的快照(租户改错误页后调用),下次访问重新加载。
func (c *errorPagesCache) Invalidate(tenantID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.items, tenantID)
	c.mu.Unlock()
}

// errorPageCaches 按 *store.Store 索引的缓存表。
//
// 挂在包级而不是 API 结构体字段上,是因为 API 结构体定义在 server.go ——
// 那份文件此刻正被另一个代理改动,以读文件的方式加字段会制造合并冲突。
// 按 store 实例索引等价于按"这份数据来自哪个库"索引:生产只有一个进程一个 store,
// 而黑盒测试每次 Setup 都会建一个新的 store,天然按测试隔离(否则一个用例写进
// 缓存的自定义页会漏给下一个用例的同号租户,变成查不到原因的偶发失败)。
var errorPageCaches sync.Map // *store.Store -> *errorPagesCache

// errorPagesCacheFor 取(并按需创建)该 store 的错误页缓存。
func (a *API) errorPagesCacheFor() *errorPagesCache {
	if v, ok := errorPageCaches.Load(a.store); ok {
		return v.(*errorPagesCache)
	}
	st := a.store
	c := newErrorPagesCache(st.GetTenantErrorPages)
	actual, _ := errorPageCaches.LoadOrStore(st, c)
	return actual.(*errorPagesCache)
}

// invalidateErrorPages 丢弃某租户的自定义错误页快照。
//
// 与规则快照失效**分开**而不是复用 invalidateRules:两者是不同的数据
// (规则 vs 租户错误页),触发时机也不同(改规则不影响错误页,改错误页不影响规则)。
// 原先 me.go 在改错误页后调 invalidateRules,那个名字既不描述它做的事、
// 也让"改错误页会失效规则快照"这个假因果看起来像是刻意设计。
func (a *API) invalidateErrorPages(tenantID int64) {
	a.errorPagesCacheFor().Invalidate(tenantID)
}

func hostOnly(h string) string {
	host, _, err := net.SplitHostPort(h)
	if err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(h)
}

// resolveDomainByHost 按 Host 定位 active 域名;不存在/未激活/停用返回 nil。
func (a *API) resolveDomainByHost(c *gin.Context) *store.Domain {
	host := hostOnly(c.Request.Host)
	if host == "" {
		return nil
	}
	d, err := a.store.GetDomainByFQDN(c.Request.Context(), host)
	if err != nil || d.Status != "active" {
		return nil
	}
	return d
}

// resolveDomainForError 按 Host 定位域名与租户,**不看域名状态**,供错误页渲染使用。
//
// 与 resolveDomainByHost 的区别正是 issue 7:那个函数只认 active 域名,
// 因为只有 active 域名才该承载短链访问;但**渲染错误页**需要另一套语义 ——
// 租户停用自己域名之后,访问者仍应看到该租户自己的 404 页,而不是系统内置页。
// 租户配置自定义错误页,就是为了让"我的域名出错了"长成自己的品牌;
// 域名一停用就退回系统页,等于在最需要品牌一致性的时刻把它撤掉。
//
// 走的是唯一索引上的等值查询(fqdn 唯一),不是扫描,而且只在错误路径上发生。
func (a *API) resolveDomainForError(c *gin.Context) (*store.Domain, int64) {
	host := hostOnly(c.Request.Host)
	if host == "" {
		return nil, 0
	}
	d, err := a.store.GetDomainByFQDNAnyStatus(c.Request.Context(), host)
	if err != nil {
		return nil, 0
	}
	return d, d.TenantID
}

// errorPageTenantID 只取租户 id 的便捷封装(不需要域名本身时用)。
func (a *API) errorPageTenantID(c *gin.Context) int64 {
	_, tenantID := a.resolveDomainForError(c)
	return tenantID
}

func (a *API) handleRedirect(c *gin.Context) {
	code := c.Param("code")
	if code == "" || strings.Contains(code, "/") {
		a.renderVisitorError(c, http.StatusNotFound, a.errorPageTenantID(c), nil)
		return
	}
	d := a.resolveDomainByHost(c)
	if d == nil {
		// 域名不存在,或存在但不是 active(停用 / 待激活)。后者仍要拿到租户 id,
		// 好让访客看到该租户自己的 404 页而不是系统内置页(issue 7)。
		_, tenantID := a.resolveDomainForError(c)
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	// 宽松命中:短链不可用时也返回它,由本函数把"为什么不可用"记进明细
	link, _, reason, err := a.store.LookupLinkForVisit(c.Request.Context(), d.ID, code)
	if err != nil {
		// 未命中(短码不存在):无法归属到任何短链,不记明细
		a.renderVisitorError(c, http.StatusNotFound, d.TenantID, nil)
		return
	}
	// 动作按短链类型确定:跳转型记 redirect,落地页型记 landing_view
	action := store.VisitActionRedirect
	if link.LinkType == store.LinkTypeLanding {
		action = store.VisitActionLandingView
	}
	if reason != "" {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: reason,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 其余两种"短链自身不可用"同样排在规则之前(spec D4 / ADR 0008):
	// 落地页托管文件缺失、跳转型没有可用目标——此时讨论"这次访问该不该被规则改写"没有意义,
	// 访问直接失败,明细里也不带规则字段。
	var landingDest string
	if link.LinkType == store.LinkTypeLanding {
		landingDest = link.LandingURL
		if link.LandingSource == store.LandingSourceUpload {
			// upload 来源的目标是"短码/"路径下的托管文件;文件缺失时 404 比跳到空页面好,
			// 并把失败落一行明细,避免租户以为落地页还在正常收流量
			if !a.landingUploaded(link.ID) {
				a.recordVisit(c, store.VisitRecord{
					LinkID: link.ID, DomainID: d.ID,
					Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonLandingMissing,
				})
				a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
				return
			}
			landingDest = "/" + code + "/"
		}
	} else if len(link.TargetURLs) == 0 {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 规则裁决:pass 与未命中都落到「继续原跳转流程」,区别只在明细里记不记这次命中
	dec := a.ruleDecision(c, link)
	if dec.Action != "" && a.applyRuleDecision(c, link, d, action, dec) {
		return
	}
	ruleID, ruleAction := visitRuleFields(dec)
	// 落地页型(16):记一次落地页视图 → 固定 302 到落地页
	if link.LinkType == store.LinkTypeLanding {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: landingDest,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		c.Redirect(http.StatusFound, landingDest)
		return
	}
	// 跳转型:先选目标(无目标即失败,落一行明细)
	targetURL, err := a.store.PickTarget(c.Request.Context(), link.ID)
	if err != nil {
		// 防御性分支:上面已按 len(TargetURLs)==0 判过一次,这里兜住并发改动
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 记录访问(短链、域名、IP、UA、来源、动作、结果、目标、时间);统计失败不阻断跳转
	a.recordVisit(c, store.VisitRecord{
		LinkID: link.ID, DomainID: d.ID,
		Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: targetURL,
		RuleID: ruleID, RuleAction: ruleAction,
	})
	a.redirectToTarget(c, link, targetURL)
}

// redirectToTarget 按短链配置的 301/302 重定向到目标(默认临时 302)。
func (a *API) redirectToTarget(c *gin.Context, link *store.Link, targetURL string) {
	status := http.StatusFound // 302
	if link.RedirectStatus == store.RedirectStatus301 {
		status = http.StatusMovedPermanently
	}
	c.Redirect(status, targetURL)
}

// ruleDecision 对本次访问求值该租户的规则快照(spec D4)。
//
// 无规则(空快照)时连访客画像都不构造:多数租户没有规则,这条链路不该为它付代价。
// 快照加载失败与求值 panic 都在 rules 包内 fail-open(记日志后按未命中返回),
// 这里不需要也不能"重试一次"——重试只是把同一份故障再打一遍。
func (a *API) ruleDecision(c *gin.Context, link *store.Link) rules.Decision {
	if !link.RulesEnabled {
		return rules.Decision{}
	}
	snap := a.ruleCache.Get(c.Request.Context(), link.TenantID)
	if snap == nil || len(snap.Rules) == 0 {
		return rules.Decision{}
	}
	// 用与访问明细同一个来源 IP 构造画像,保证"明细里记的 IP"与"规则看到的 IP"一致,
	// 不会因为两处解析口径不同而出现"规则按 A 拦截、明细却记着 B"。
	//
	// 地理值也在这之前查完并挂到请求的副本上:求值期必须零 IO(ADR 0009),
	// 查库的动作只能发生在这里。取不到就填空值,不填默认国家。
	req := c.Request
	g := a.visitGeo(c)
	if g.Country != "" || g.ASN != "" {
		req = req.WithContext(rules.WithGeo(req.Context(), g.Country, g.ASN))
	}
	vCtx := rules.AcquireVisitorContext(req, g.Country, g.ASN).WithIP(a.clientIPForVisitor(req))
	defer rules.ReleaseVisitorContext(vCtx)

	dec, matched := snap.Evaluate(vCtx, link.ID)
	if !matched {
		return rules.Decision{}
	}
	return dec
}

// visitRuleFields 把裁决折成访问明细的两个字段:命中(含 pass)时非空,未命中时为空。
// pass 也要记:契约里 ruleId/ruleAction 记的是"本次访问的规则裁决结果",
// 放行同样是一次裁决,租户要能看出"这次访问被哪条规则看过"。
func visitRuleFields(dec rules.Decision) (*int64, string) {
	if dec.Action == "" {
		return nil, ""
	}
	id := dec.RuleID
	return &id, dec.Action
}

// applyRuleDecision 按裁决改变本次访问的结果;返回 true 表示已终结本次请求。
// pass(或改写目标为空时的兜底)返回 false,由调用方走原跳转流程——但这次命中仍会记进明细。
//
// 关键不变式:命中不写任何计数表(只落一行明细)。跳转是这条链路上 QPS 最高的部分,
// 每命中一次写一次库就是写放大,而"24h 命中"由规则列表从明细读时聚合(spec D9)。
func (a *API) applyRuleDecision(c *gin.Context, link *store.Link, d *store.Domain, action string, dec rules.Decision) bool {
	ruleID, ruleAction := visitRuleFields(dec)
	switch dec.Action {
	case store.RuleActionNotfound:
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonRuleBlocked,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, &dec)
		return true
	case store.RuleActionThrottle:
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonRuleThrottled,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.renderVisitorError(c, http.StatusTooManyRequests, link.TenantID, &dec)
		return true
	case store.RuleActionRedirect:
		// 规则改写的目标不参与轮询(spec 风险章节):轮询是"在目标池里选一条",
		// 规则改写是"换一条路",语义不同,拿它去 PickTarget 会把改写目标轮询没了。
		if dec.Destination == "" {
			// 理论上写入时已校验过;真出现空目标时按"未命中"放行并记日志,
			// 而不是把访问者重定向到一个空地址。
			slog.Error("规则裁决为改写目标但 destination 为空,按未命中处理",
				"rule", dec.RuleID, "link", link.ID)
			return false
		}
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeSuccess, TargetURL: dec.Destination,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.redirectToTarget(c, link, dec.Destination)
		return true
	}
	return false
}

// renderVisitorError 针对访客端重定向/未命中/拦截场景渲染 HTML 错误页面 (404 / 429)。
// 决议优先级:
// 1. 规则专属自定义页面 (dec.PageMode == "custom" && dec.CustomHTML != "")
// 2. 租户全局自定义页面 (tenantID > 0 时从按租户的内存快照读,不再实时查库)
// 3. 系统内置默认自适应 HTML 页面
//
// 每条路径都带齐三个头(CSP / X-Content-Type-Options / Cache-Control),原因见各自常量。
func (a *API) renderVisitorError(c *gin.Context, status int, tenantID int64, dec *rules.Decision) {
	setVisitorErrorHeaders(c, status)
	if dec != nil && dec.PageMode == "custom" && dec.CustomHTML != "" {
		c.String(status, dec.CustomHTML)
		return
	}

	if tenantID > 0 {
		pages := a.errorPagesCacheFor().Get(c.Request.Context(), tenantID)
		if status == http.StatusTooManyRequests && pages.tooManyRequests != "" {
			c.String(status, pages.tooManyRequests)
			return
		}
		if status == http.StatusNotFound && pages.notFound != "" {
			c.String(status, pages.notFound)
			return
		}
	}

	if status == http.StatusTooManyRequests {
		c.String(status, templates.Default429HTML())
	} else {
		c.String(status, templates.Default404HTML())
	}
}

// setVisitorErrorHeaders 给访客可见的错误页响应补齐安全与缓存语义的头。
//
// 头在写 body **之前**设置:gin 一旦开始写 body 就锁定了状态码与部分头,
// 之后再补就来不及了(Content-Length 也会对不上)。
func setVisitorErrorHeaders(c *gin.Context, status int) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	// 租户错误页是原样输出的租户 HTML。缺 nosniff 时浏览器会对未声明类型的
	// 响应做 MIME 嗅探,一段 text/plain 里的标记也可能被当 HTML 执行。
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", visitorErrorCSP)
	if status == http.StatusTooManyRequests {
		// 429 不只是"慢一点再来":它是这条访问被规则裁决拦下的证据。
		// 缓存下来会让同一个访客在解除限流后仍看到 429,而 Retry-After 缺失
		// 又让客户端无从退避,只能继续撞同一条规则。
		c.Header("Retry-After", strconv.Itoa(throttleRetryAfterSeconds))
		c.Header("Cache-Control", "no-store")
		return
	}
	// 404 同理:租户改完自定义页后,启发式缓存会让访客继续看到旧版,
	// 而"错误页长什么样"恰恰是租户最常改的东西(404 最容易触发,被缓存的
	// 概率也最高)。no-store 让每次都回源,成本由上面的内存快照兜住。
	c.Header("Cache-Control", "no-store")
}
