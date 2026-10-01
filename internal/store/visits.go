package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Visit struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	LinkID       int64  `json:"linkId" gorm:"column:link_id"`
	DomainID     int64  `json:"-" gorm:"column:domain_id"`
	Domain       string `json:"domain" gorm:"->"` // 只读字段,查询时 join 填充
	IP           string `json:"ip"`
	UserAgent    string `json:"userAgent" gorm:"column:user_agent"`
	Referer      string `json:"referer"`
	Action       string `json:"action"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason"`
	TargetURL    string `json:"targetUrl" gorm:"column:target_url"`
	Country      string `json:"country"`
	IsDatacenter bool   `json:"isDatacenter" gorm:"column:is_datacenter"`
	ASN          string `json:"asn"`
	Lang         string `json:"lang"`
	// RuleID / RuleAction 是本次访问的规则裁决结果(spec D8):无规则参与时为空。
	// RuleID 可空是因为规则被删后 ON DELETE SET NULL,历史明细保留但不再指向任何规则。
	RuleID     *int64    `json:"ruleId" gorm:"column:rule_id"`
	RuleAction string    `json:"ruleAction" gorm:"column:rule_action"`
	CreatedAt  time.Time `json:"createdAt" gorm:"column:created_at"`
}

// 访问动作:一次 visits 行代表"触发了什么动作"。
// link.visits 计数只含成功的 redirect / landing_view(见 CountVisitsByLink),
// click 是落地页按钮回传,混进访问量会把到达率算高一倍。
const (
	VisitActionRedirect    = "redirect"     // 跳转型短链的一次访问
	VisitActionLandingView = "landing_view" // 落地页型短链的一次访问(落地页视图)
	VisitActionClick       = "click"        // 落地页按钮经 SDK 回传的一次点击
)

// 动作结果:成功即按预期重定向,失败是可归属到该短链的失败(见 VisitReason*)。
const (
	VisitOutcomeSuccess = "success"
	VisitOutcomeFailed  = "failed"
)

// 失败原因:仅 outcome='failed' 时非空。
// 不记录无法归属到具体短链的失败(短码未命中、域名未激活、租户被封禁)。
const (
	VisitReasonLinkDisabled   = "link_disabled"   // 短码命中但短链已停用
	VisitReasonLinkDeleted    = "link_deleted"    // 短码命中但短链已逻辑删除
	VisitReasonNoTarget       = "no_target"       // PickTarget 失败(目标 URL 列表为空)
	VisitReasonLandingMissing = "landing_missing" // landing+upload 来源但托管文件缺失
	// 规则裁决导致的失败(spec D4):这两种失败一定带 rule_id / rule_action。
	// 与上面四种"短链自身不可用"分开——后者排在规则之前,明细里没有规则字段。
	VisitReasonRuleBlocked   = "rule_blocked"   // 规则裁决 notfound → 404
	VisitReasonRuleThrottled = "rule_throttled" // 规则裁决 throttle → 429
)

// visitCountActions 计入访问量的动作集合(关键不变式的一半:click 排除在外;
// 另一半是 outcome 必须为 success,见 CountVisitsByLink)。
var visitCountActions = []string{VisitActionRedirect, VisitActionLandingView}

// ErrInvalidAction 访问列表的 action 过滤值非法(不在 VisitAction* 枚举内)。
// 与 ErrNotFound 分开定义:前者是调用方传参错误(可映射为 400),后者是数据缺失。
var ErrInvalidAction = errors.New("invalid visit action")

// ErrInvalidOutcome 访问列表的 outcome 过滤值非法(不在 VisitOutcome* 枚举内)。
// 与 ErrInvalidAction 同理:它是参数错误(400),不是数据缺失。
var ErrInvalidOutcome = errors.New("invalid visit outcome")

// ValidVisitAction 判断 action 过滤值是否合法(空串 = 不过滤,视为合法)。
// 上层用它提前回 400,ListVisitsByLink 内部仍会再校验一次作为兜底。
func ValidVisitAction(action string) bool {
	switch action {
	case "":
		return true
	case VisitActionRedirect, VisitActionLandingView, VisitActionClick:
		return true
	}
	return false
}

// ValidVisitOutcome 判断 outcome 过滤值是否合法(空串 = 不过滤,视为合法)。
func ValidVisitOutcome(outcome string) bool {
	switch outcome {
	case "":
		return true
	case VisitOutcomeSuccess, VisitOutcomeFailed:
		return true
	}
	return false
}

// validVisitAction / validVisitOutcome 过滤值白名单校验(空串表示不过滤)。
func validVisitAction(action string) error {
	if !ValidVisitAction(action) {
		return ErrInvalidAction
	}
	return nil
}

func validVisitOutcome(outcome string) error {
	if !ValidVisitOutcome(outcome) {
		return ErrInvalidOutcome
	}
	return nil
}

// VisitRecord 一次访问/点击的入库字段(Action/Outcome/Reason/TargetURL/Lang/Country 由调用方填写;
// Country 来自离线 ip2region 库,查不到时为空。IsDatacenter/ASN 仍无数据源,不在此列)。
type VisitRecord struct {
	LinkID    int64
	DomainID  int64
	IP        string
	UserAgent string
	Referer   string
	Action    string
	Outcome   string
	Reason    string
	TargetURL string
	Lang      string
	// Country:访客 IP 解析出的 ISO 3166-1 alpha-2 国家码(查不到时为空)。
	// asn / is_datacenter 不在此列:当前没有数据源(ADR 0009),
	// 传空值与数据库默认值无法区分"没查"和"查了没有",不如不传。
	Country string
	// RuleID / RuleAction:本次命中的规则与它的裁决(无规则参与时留空)。
	// 命中不写任何计数表(spec D9),只多写这两列——明细是这次裁决唯一留痕的地方。
	RuleID     *int64
	RuleAction string
}

// InsertVisit 记录一次访问/点击动作(含访问者 IP)。
// 统计写入失败由调用方决定是否阻断跳转,本函数只返回 error。
func (s *Store) InsertVisit(ctx context.Context, rec VisitRecord) error {
	v := Visit{
		LinkID: rec.LinkID, DomainID: rec.DomainID, IP: rec.IP,
		UserAgent: rec.UserAgent, Referer: rec.Referer,
		Action: rec.Action, Outcome: rec.Outcome, Reason: rec.Reason,
		TargetURL: rec.TargetURL, Lang: rec.Lang,
		Country: rec.Country,
		RuleID:  rec.RuleID, RuleAction: rec.RuleAction,
	}
	return s.db.WithContext(ctx).Create(&v).Error
}

// CountVisitsByLink 访问计数 = 成功的访问行(action IN ('redirect','landing_view') 且 outcome='success')。
// 两类行都不得灌水:click 是落地页按钮回传;outcome='failed' 是短链不可用导致的失败,
// 租户看到"访问量"时应理解为真正被成功重定向出去的次数。
func (s *Store) CountVisitsByLink(ctx context.Context, linkID int64) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).
		Where("link_id = ? AND action IN ? AND outcome = ?", linkID, visitCountActions, VisitOutcomeSuccess).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListVisitsByLink 分页访问列表(按时间倒序)。
// action 非空时按该动作过滤,取值不在枚举内返回 ErrInvalidAction(由上层映射 400)。
func (s *Store) ListVisitsByLink(ctx context.Context, linkID int64, action string, page, pageSize int) ([]*Visit, int, error) {
	return s.ListVisitsByLinkFiltered(ctx, linkID, VisitFilter{Action: action}, page, pageSize)
}

// VisitFilter 访问明细列表的过滤条件(空串 = 该维度不过滤)。
//
// action 与 outcome 是两个正交维度,不能互相替代:一次 landing_view 访问既有
// action 也有 outcome,而"点击行"与"失败行"要靠 action='click' / outcome='failed'
// 分别排除。只按 action 过滤会把失败行一起带出来(同一次访问可能既是
// landing_view 又是 failed),只按 outcome 过滤则分不出点击。
type VisitFilter struct {
	Action  string
	Outcome string
}

// ListVisitsByLinkFiltered 按 action + outcome 过滤的分页访问列表(按时间倒序)。
// 两个过滤值都做白名单校验,非法值返回 ErrInvalidAction / ErrInvalidOutcome。
//
// count 与明细两段查询必须用同一组过滤条件,否则 total 与 items 会不一致。
func (s *Store) ListVisitsByLinkFiltered(ctx context.Context, linkID int64, f VisitFilter, page, pageSize int) ([]*Visit, int, error) {
	if err := validVisitAction(f.Action); err != nil {
		return nil, 0, err
	}
	if err := validVisitOutcome(f.Outcome); err != nil {
		return nil, 0, err
	}
	q := s.db.WithContext(ctx).Model(&Visit{}).Where("link_id = ?", linkID)
	itemQ := s.db.WithContext(ctx).Table("visits v").Where("v.link_id = ?", linkID)
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
		itemQ = itemQ.Where("v.action = ?", f.Action)
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
		itemQ = itemQ.Where("v.outcome = ?", f.Outcome)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Visit
	err := itemQ.
		Select("v.id, v.link_id, d.fqdn AS domain, v.ip, v.user_agent, v.referer, " +
			"v.action, v.outcome, v.reason, v.target_url, v.country, v.is_datacenter, v.asn, v.lang, " +
			"v.rule_id, v.rule_action, v.created_at").
		Joins("JOIN domains d ON d.id = v.domain_id").
		Order("v.id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).Scan(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

// ---------- 总览聚合(07 / ADR-0010) ----------
//
// 为什么不用"前端拉明细自己数":
//
//  1. 口径必然漂移。link.visits 由服务端按 action IN ('redirect','landing_view')
//     AND outcome='success' 过滤,而前端那份等价实现一旦漏掉一个条件(历史上就漏过),
//     同屏 KPI 与分布图就对不上,而且没有任何测试会发现。
//  2. 抽样当全量。原先总览页取"访问量最高的 10 条短链 × 最近 50 条明细"当样本,
//     点击量大的落地页最近 50 行可能全是 click,分布图等于只统计了点击者。
//  3. 覆盖不全。总量累加 listLinks(1,100),超过 100 条短链的租户看到的是偏小的数。
//
// 所以这里一次 SQL 出各维度分布,前端只负责把 user_agent 字符串翻译成
// 设备/系统/浏览器标签(复用已有的 ua-parser-js,不在 Go 侧另引一套 UA 解析器)。

// OverviewTopLimit topLinks 最多返回多少条短链。
const OverviewTopLimit = 5

// overviewFacetLimit 单个维度的分桶上限(按访问量降序取前 N 桶)。
//
// 只对 user_agent 设上限:UA 的不同取值理论上无界(带随机 token 的爬虫每次都不一样),
// 而国家与来源是有限集合。截断时 FacetCoverage.Truncated 为 true,前端据此把
// 占比的分母改成"已返回部分"并在界面上说明,而不是拿一个偏小的分母算出偏大的占比。
const overviewFacetLimit = 500

// OverviewTotals 总览页的累计计数,全部来自 visits 明细表(口径与 CountVisitsByLink 一致)。
//
// Clicks 与 Visits 同源同期:分子分母都只覆盖"visits 表当前持有的行",
// 即同样的保留期窗口(默认 90 天)。它不再取 links.clicks 那个永久计数器 ——
// 那个计数器永不衰减而分母会被保留期削掉,运行满一个保留期后 CTR 会单调虚高到 100% 以上。
type OverviewTotals struct {
	Links          int64 `json:"links"`
	ActiveLinks    int64 `json:"activeLinks"`
	LandingLinks   int64 `json:"landingLinks"`
	Visits         int64 `json:"visits"`
	RedirectVisits int64 `json:"redirectVisits"`
	LandingVisits  int64 `json:"landingVisits"`
	Clicks         int64 `json:"clicks"`
}

// OverviewTopLink 热门短链排行的一行(全租户,不受短链列表分页限制)。
type OverviewTopLink struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	LinkType string `json:"linkType"`
	Visits   int64  `json:"visits"`
}

// FacetCount 维度分布的一桶:原始取值 + 访问次数。
type FacetCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// SourceCount 来源分布的一桶(Name 取值与 trafficBreakdown.ts 的来源标签同名)。
type SourceCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// FacetCoverage 某个维度的覆盖情况:分母该用多少、返回的桶覆盖了多少。
type FacetCoverage struct {
	Total     int64 `json:"total"`
	Returned  int64 `json:"returned"`
	Truncated bool  `json:"truncated"`
}

// OverviewFacets 各维度分布。
type OverviewFacets struct {
	// UserAgents 是**原始 User-Agent 字符串**的计数,不是设备标签。
	// 设备/系统/浏览器的归类规则只存在于前端的 ua-parser-js 一处,
	// 这里再实现一遍 Go 版 UA 解析器只会制造第二套会漂移的口径。
	UserAgents []FacetCount `json:"userAgents"`
	// Sources 已按广告平台归类完成(见 overviewSourceExpr)。
	Sources []SourceCount `json:"sources"`
	// Countries 只含可定位的访问;解析不出国家的访问既不进任何国家,
	// 也不塞进"其他"——那批访问的来源确实未知。
	Countries []FacetCount `json:"countries"`
	// UserAgentCoverage / CountryCoverage 说明各自分母的取法。
	UserAgentCoverage FacetCoverage `json:"userAgentCoverage"`
	CountryCoverage   FacetCoverage `json:"countryCoverage"`
}

// OverviewStats 总览页一次取齐的聚合结果。
type OverviewStats struct {
	Totals   OverviewTotals    `json:"totals"`
	TopLinks []OverviewTopLink `json:"topLinks"`
	Facets   OverviewFacets    `json:"facets"`
}

// overviewScopeFrom 总览统计的公共作用域:只圈定"哪些行属于本租户在册的短链"。
//
// 只统计**未逻辑删除**的短链(与短链列表页 /api/links 口径一致)。逻辑删除保留了该
// 短链的历史明细(单链页仍可查),但不进总览 —— 否则租户删掉一条短链后,总览的
// "总访问数"不降而排行里它消失了,两个数字互相矛盾。
//
// 动作/结果过滤不写在这里而由各查询自己加:clicks 要数 click 行,其余维度要数
// redirect/landing_view 行,两者共用同一段 FROM/WHERE 才是 CTR 分子分母
// "同源同期"的结构前提。
const overviewScopeFrom = `
	  FROM visits v
	  JOIN links l ON l.id = v.link_id
	 WHERE l.tenant_id = ? AND l.deleted_at IS NULL`

// overviewVisitActions 计入"访问"的动作集合。与 visitCountActions 同一组取值,
// 但写成 SQL 字面量而不是拼接 Go 变量:这段文本会被拼进 Raw(),展开成常量才不可能
// 被将来误改成可注入的形式。
const overviewVisitActions = `('redirect','landing_view')`

// overviewVisitScope 计入"访问"的那部分行(分布图与热门短链排行都用它)。
//
// click 行与 failed 行(多半是扫描器)一律排除:落地页型短链的同一次访问会落
// landing_view + click 两行,不排除的话同一访客会在设备/系统/浏览器/来源/国家
// 五张图里各被计两次。
const overviewVisitScope = overviewScopeFrom + `
	   AND v.action IN ` + overviewVisitActions + ` AND v.outcome = 'success'`

// overviewSourceExpr 来源归类表达式。
//
// 判定顺序与前端 trafficBreakdown.ts 的 sourceOf 保持一致(TikTok 先于 Meta,
// 因为 TikTok 的落地 URL 里常带 facebook/meta 字样)。两边是同一套规则的
// 两个实现:前端那处服务于单链明细页的抽样聚合,这里这处服务于总览的全量聚合。
// 改一处必须同步改另一处 —— 归类标签是对外可见的文案,不一致会让同屏两处对不上。
const overviewSourceExpr = `
	CASE
	  WHEN v.referer = '' OR v.referer = '-' THEN '直接访问'
	  WHEN v.referer ILIKE '%tiktok%' THEN 'TikTok Ads'
	  WHEN v.referer ILIKE '%facebook%' OR v.referer ILIKE '%instagram%' OR v.referer ILIKE '%meta%' THEN 'Meta Ads'
	  WHEN v.referer ILIKE '%google%' THEN 'Google Ads'
	  ELSE '其他来源'
	END`

// OverviewStatsForTenant 一次取齐总览页所需的全部聚合。
//
// 各维度都是"全量 GROUP BY",不再是把最近 N 行当全量的抽样:代价是几条
// 覆盖索引上的聚合扫描(见迁移 0019 的部分索引),换来的是分布图与 KPI
// 必然对得上,以及超过 100 条短链的租户也能看到真实总量。
func (s *Store) OverviewStatsForTenant(ctx context.Context, tenantID int64) (*OverviewStats, error) {
	totals, err := s.overviewTotals(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	top, err := s.overviewTopLinks(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	facets, err := s.overviewFacets(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &OverviewStats{Totals: *totals, TopLinks: top, Facets: *facets}, nil
}

// overviewTotals 短链存量与累计访问计数。
//
// 存量(links / activeLinks / landingLinks)来自 links 表,与 /api/links 同口径;
// 累计访问与点击来自 visits 表,与 CountVisitsByLink 同口径。两组数放在同一个响应里,
// 就是为了让前端不必再"把短链列表的 visits 字段加起来"——那正是漏算超过 100 条短链的地方。
func (s *Store) overviewTotals(ctx context.Context, tenantID int64) (*OverviewTotals, error) {
	var t OverviewTotals
	// 分母与分子同源同期:两者都只数 visits 表当前持有的行(同一个保留期窗口),
	// 唯一的差别是 FILTER 里的动作集合。这也是 CTR 落在 0~100% 的结构前提 ——
	// 分子不再取 links.clicks 那个永不衰减的永久计数器,否则 90 天清理会持续
	// 削掉分母而分子不动,运行满一个保留期后 CTR 单调虚高到 100% 以上。
	if err := s.db.WithContext(ctx).Raw(`
		SELECT count(*) FILTER (
		         WHERE v.action IN ('redirect','landing_view') AND v.outcome = 'success'
		       ) AS visits,
		       count(*) FILTER (
		         WHERE v.action = 'redirect' AND v.outcome = 'success'
		       ) AS redirect_visits,
		       count(*) FILTER (
		         WHERE v.action = 'landing_view' AND v.outcome = 'success'
		       ) AS landing_visits,
		       count(*) FILTER (
		         WHERE v.action = 'click' AND v.outcome = 'success'
		       ) AS clicks
		`+overviewScopeFrom+`
		 WHERE v.outcome = 'success'`, tenantID).Scan(&t).Error; err != nil {
		return nil, err
	}
	// 短链存量:与 /api/links 同口径(不含逻辑删除),供「共 N 条短链」一类文案使用。
	var linkStats struct {
		Links        int64
		ActiveLinks  int64
		LandingLinks int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT count(*) AS links,
		       count(*) FILTER (WHERE status = 'enabled') AS active_links,
		       count(*) FILTER (WHERE link_type = 'landing') AS landing_links
		  FROM links WHERE tenant_id = ? AND deleted_at IS NULL`, tenantID).Scan(&linkStats).Error; err != nil {
		return nil, err
	}
	t.Links, t.ActiveLinks, t.LandingLinks = linkStats.Links, linkStats.ActiveLinks, linkStats.LandingLinks
	return &t, nil
}

// overviewTopLinks 全租户按访问量降序的前 N 条短链(不受短链列表分页限制)。
func (s *Store) overviewTopLinks(ctx context.Context, tenantID int64) ([]OverviewTopLink, error) {
	var out []OverviewTopLink
	err := s.db.WithContext(ctx).Raw(`
		SELECT v.link_id AS id, l.code AS code, l.link_type AS link_type, count(*) AS visits
		`+overviewVisitScope+`
		GROUP BY v.link_id, l.code, l.link_type
		ORDER BY visits DESC, v.link_id ASC
		LIMIT ?`, tenantID, OverviewTopLimit).Scan(&out).Error
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []OverviewTopLink{}
	}
	return out, nil
}

// overviewFacets 三个维度的分布:UA 原始串、来源、国家。
func (s *Store) overviewFacets(ctx context.Context, tenantID int64) (*OverviewFacets, error) {
	f := &OverviewFacets{
		UserAgents: []FacetCount{}, Sources: []SourceCount{}, Countries: []FacetCount{},
	}
	// UA:按原始串分组,标签留给前端ua-parser-js 翻译。
	if err := s.db.WithContext(ctx).Raw(`
		SELECT v.user_agent AS value, count(*) AS count
		`+overviewVisitScope+`
		GROUP BY v.user_agent
		ORDER BY count DESC, v.user_agent ASC
		LIMIT ?`, tenantID, overviewFacetLimit).Scan(&f.UserAgents).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT `+overviewSourceExpr+` AS name, count(*) AS count
		`+overviewVisitScope+`
		GROUP BY 1
		ORDER BY count DESC, name ASC`, tenantID).Scan(&f.Sources).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT v.country AS value, count(*) AS count
		`+overviewVisitScope+`
		 AND v.country <> ''
		GROUP BY v.country
		ORDER BY count DESC, v.country ASC`, tenantID).Scan(&f.Countries).Error; err != nil {
		return nil, err
	}
	f.UserAgentCoverage = coverageOf(f.UserAgents, len(f.UserAgents) >= overviewFacetLimit)
	f.CountryCoverage = coverageOf(f.Countries, false)
	return f, nil
}

// coverageOf 由已返回的桶算出分母与是否截断。
//
// Total 就等于 Returned:总览是全量聚合,除 user_agent 的基数上限外没有抽样,
// 所以"已返回的桶"本身就是完整分母。之所以把两个字段都留在响应里,是为了让前端
// 在 truncated 为真时明确改用 Returned 作分母并如实告知用户,而不是默认分母恒等于全量。
func coverageOf(items []FacetCount, truncated bool) FacetCoverage {
	c := FacetCoverage{Truncated: truncated}
	for _, it := range items {
		c.Returned += it.Count
	}
	c.Total = c.Returned
	return c
}

// CleanupVisitsBefore 清理保留期前的访问记录。
func (s *Store) CleanupVisitsBefore(ctx context.Context, before time.Time) (int64, error) {
	return s.CleanupVisitsBeforeBatched(ctx, before, DefaultVisitCleanupBatch)
}

// DefaultVisitCleanupBatch 未显式配置 CLOAK_VISIT_CLEANUP_BATCH 时的单批行数。
const DefaultVisitCleanupBatch = 10000

// CleanupVisitsBeforeBatched 分批删除保留期前的访问记录,返回累计删除行数。
//
// 为什么必须分批:一条 `DELETE FROM visits WHERE created_at < ?` 会把全部过期行
// 锁住直到事务提交。高流量租户保留 90 天,单次可能删几十万行 —— 长事务、
// WAL 暴涨、表膨胀,而且与热路径的 INSERT 争抢同一批行锁。
// 分批后每批独立提交,批间让出,单批的锁持有时间与影响面都是有界的。
//
// 每批只删当前仍满足条件的前 N 行(而不是推进游标),这样并发写入的新过期行
// 也会被后续批次扫到,不会因游标语义而在两次清理之间漏掉。
func (s *Store) CleanupVisitsBeforeBatched(ctx context.Context, before time.Time, batch int) (int64, error) {
	if batch <= 0 {
		batch = DefaultVisitCleanupBatch
	}
	var total int64
	for {
		// 单批一个事务。Postgres 的 DELETE 没有 LIMIT 语法,用子查询按主键取前 N 行再删。
		var n int64
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Exec(`
				DELETE FROM visits
				WHERE id IN (
					SELECT id FROM visits WHERE created_at < ? ORDER BY id LIMIT ?
				)`, before, batch)
			if res.Error != nil {
				return res.Error
			}
			n = res.RowsAffected
			return nil
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(batch) {
			return total, nil
		}
		// 批间让出,给热路径的 INSERT 留出窗口
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
