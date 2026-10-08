package store

import (
	"context"
	"database/sql/driver"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Link struct {
	ID             int64          `json:"id" gorm:"primaryKey"`
	TenantID       int64          `json:"-" gorm:"column:tenant_id"`
	Code           string         `json:"code"`
	TargetURLs     []string       `json:"targetUrls" gorm:"-"`
	RedirectStatus RedirectStatus `json:"redirectStatus" gorm:"column:redirect_status"`
	LinkType       string         `json:"linkType" gorm:"column:link_type"`
	LandingSource  string         `json:"landingSource" gorm:"column:landing_source"`
	LandingURL     string         `json:"landingUrl" gorm:"column:landing_url"`
	Status         string         `json:"status"`
	RulesEnabled   bool           `json:"rulesEnabled" gorm:"column:rules_enabled"`
	Clicks         int64          `json:"clicks" gorm:"column:clicks"`
	Metadata       Metadata       `json:"metadata" gorm:"column:metadata;type:jsonb"`
	// DeletedAt 对外可见:回收站列表要靠它区分「已删除」,且前端在 PATCH 无关字段时
	// 也需要知道这条记录处于已删除态(不可编辑)。omitempty 让未删除的记录不带该字段。
	DeletedAt *time.Time `json:"deletedAt,omitempty" gorm:"column:deleted_at"`
	Domains   []string   `json:"domains" gorm:"-"`
	Visits    int64      `json:"visits" gorm:"-"`
	// ClickVisits 与 Visits 同源同期的点击计数(visits 表 action='click' 且成功)。
	// 与 Clicks(links.clicks 永久计数器)分开:后者不受保留期清理影响,拿它当 CTR
	// 分子会在清理后虚高(见 overviewTotals 的注释)。列表页 CTR 用本字段。
	ClickVisits int64 `json:"clickVisits" gorm:"-"`
	// RuleCount / RuleNames 是"适用规则"的投影(全局规则 + 显式关联的规则),查询后填充。
	// 关联只存在规则一侧(spec D1),这里是按短链反查同一份数据,不存在第二份规则列表。
	RuleCount int64    `json:"ruleCount" gorm:"-"`
	RuleNames []string `json:"ruleNames" gorm:"-"`
	// Rules 是**显式关联**到本短链的规则(不含全局继承的),供短链列表在行内直接
	// 渲染「规则名 + 启用开关」——只认关联,是因为 enabled 是规则级开关,全局规则的
	// 开关在规则页,顺手在某一行的单链上下文里改它会改掉所有短链。
	Rules           []LinkRuleBrief `json:"rules" gorm:"-"`
	LandingUploaded bool            `json:"landingUploaded" gorm:"-"`
	CreatedAt       time.Time       `json:"createdAt" gorm:"column:created_at"`
}

// LinkRuleBrief 短链列表行内用的规则投影:只有渲染「名字 + 开关」需要的字段。
type LinkRuleBrief struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
}

// 短链类型(16):redirect 访问即跳转目标;landing 访问先到落地页、按钮点击后到目标。
const (
	LinkTypeRedirect = "redirect"
	LinkTypeLanding  = "landing"
)

// 落地页来源(16):url 填写地址;upload 上传 zip 由平台托管在"短码/"路径下。
const (
	LandingSourceURL    = "url"
	LandingSourceUpload = "upload"
)

// LinkDomain 短链-域名关联。
//
// 「同一域名下短码不重复」这条不变式由 migrations/0017 的**部分唯一索引**
// link_domains_live_code_uniq 保证:`UNIQUE (domain_id, code) WHERE NOT link_deleted`。
// link_deleted 是 links.deleted_at 的 denormalize 副本,由数据库触发器维护
// (见 0017 迁移头注释),因此软删短链不再占用短码,而还原时会重新占用。
//
// 字段带 `->` = 只读:写入永远交给数据库触发器派生,GORM 不参与赋值,
// 也就不会出现「应用层忘了同步」这种让索引与 links 脱节的情况。
type LinkDomain struct {
	ID       int64 `gorm:"primaryKey"`
	LinkID   int64 `gorm:"column:link_id"`
	DomainID int64 `gorm:"column:domain_id"`
	Code     string
	// LinkDeleted 带 `->` = 只读:写入永远交给数据库触发器派生,GORM 不参与赋值,
	// 也就不会出现「应用层忘了同步」这种让索引与 links 脱节的情况。
	LinkDeleted bool `gorm:"column:link_deleted;->"`
}

// LinkTarget 短链目标 URL(position 从 0 开始,按 position 升序;(link_id, position) 唯一)。
type LinkTarget struct {
	ID       int64 `gorm:"primaryKey"`
	LinkID   int64 `gorm:"column:link_id"`
	URL      string
	Position int
}

// maxLinkRuleNames 短链列表里直接展示的规则名上限(超出由界面走 +K)。
// 计数 RuleCount 始终是全量,只有名字截断——"N 条"与 "+K"都需要真实总数。
const maxLinkRuleNames = 3

// fillLinkMeta 补充 domains(fqdn 列表)、targetUrls、visits 计数与适用规则。
func (s *Store) fillLinkMeta(ctx context.Context, l *Link) error {
	domains, err := s.linkDomainFQDNs(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Domains = domains
	urls, err := s.linkTargetURLs(ctx, l.ID)
	if err != nil {
		return err
	}
	l.TargetURLs = urls
	n, err := s.CountVisitsByLink(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Visits = n
	c, err := s.CountClicksByLink(ctx, l.ID)
	if err != nil {
		return err
	}
	l.ClickVisits = c
	if l.Metadata == nil {
		l.Metadata = Metadata{}
	}
	return s.fillLinkRuleMeta(ctx, []*Link{l})
}

// fillLinksMeta 批量补充一组短链的 domains/targetUrls/visits(列表页避免 N+1)。
func (s *Store) fillLinksMeta(ctx context.Context, links []*Link) error {
	if len(links) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ID)
	}

	type domainRow struct {
		LinkID int64  `gorm:"column:link_id"`
		FQDN   string `gorm:"column:fqdn"`
	}
	var domainRows []domainRow
	if err := s.db.WithContext(ctx).Table("link_domains ld").
		Select("ld.link_id, d.fqdn").
		Joins("JOIN domains d ON d.id = ld.domain_id").
		Where("ld.link_id IN ?", ids).
		Order("ld.link_id, d.id").
		Scan(&domainRows).Error; err != nil {
		return err
	}
	domainMap := make(map[int64][]string, len(links))
	for _, row := range domainRows {
		domainMap[row.LinkID] = append(domainMap[row.LinkID], row.FQDN)
	}

	type targetRow struct {
		LinkID int64  `gorm:"column:link_id"`
		URL    string `gorm:"column:url"`
	}
	var targetRows []targetRow
	if err := s.db.WithContext(ctx).Table("link_targets").
		Select("link_id, url").
		Where("link_id IN ?", ids).
		Order("link_id, position").
		Scan(&targetRows).Error; err != nil {
		return err
	}
	targetMap := make(map[int64][]string, len(links))
	for _, row := range targetRows {
		targetMap[row.LinkID] = append(targetMap[row.LinkID], row.URL)
	}

	type visitRow struct {
		LinkID int64 `gorm:"column:link_id"`
		Visits int64 `gorm:"column:visits"`
		Clicks int64 `gorm:"column:clicks"`
	}
	var visitRows []visitRow
	// 访问量与点击量同源同期(同一张表、同一个 outcome 作用域),与 CountVisitsByLink /
	// CountClicksByLink 逐条查询的口径一致。动作集合写成 SQL 字面量而不是拼接
	// Go 变量,理由同 visits.go 的 overviewVisitActions:展开成常量才不可能被将来
	// 误改成可注入的形式。
	if err := s.db.WithContext(ctx).Table("visits").
		Select("link_id, "+
			"count(*) FILTER (WHERE action IN ('redirect','landing_view')) AS visits, "+
			"count(*) FILTER (WHERE action = 'click') AS clicks").
		Where("link_id IN ? AND outcome = ?", ids, VisitOutcomeSuccess).
		Group("link_id").
		Scan(&visitRows).Error; err != nil {
		return err
	}
	visitMap := make(map[int64]int64, len(visitRows))
	clickMap := make(map[int64]int64, len(visitRows))
	for _, row := range visitRows {
		visitMap[row.LinkID] = row.Visits
		clickMap[row.LinkID] = row.Clicks
	}

	for _, l := range links {
		l.Domains = domainMap[l.ID]
		l.TargetURLs = targetMap[l.ID]
		l.Visits = visitMap[l.ID]
		l.ClickVisits = clickMap[l.ID]
	}
	if err := s.fillLinkRuleMeta(ctx, links); err != nil {
		return err
	}
	return nil
}

// fillLinkRuleMeta 批量补充一组短链的适用规则(避免 N+1,整页一条 SQL)。
//
// "适用"= 该租户全部 scope=global 的规则 ∪ 与本短链显式关联的 scope=links 规则,
// 与求值侧 Snapshot.applies 同一套口径(spec D1)。按 priority 升序取前 maxLinkRuleNames 个
// 名字:界面按徽标展示,排序与求值顺序一致,租户看到的先后就是真实的求值先后。
// 停用规则同样列出("适用"与"启用"是两件事)。
func (s *Store) fillLinkRuleMeta(ctx context.Context, links []*Link) error {
	if len(links) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ID)
	}
	type row struct {
		LinkID  int64  `gorm:"column:link_id"`
		RuleID  int64  `gorm:"column:rule_id"`
		Name    string `gorm:"column:name"`
		Enabled bool   `gorm:"column:enabled"`
		Scope   string `gorm:"column:scope"`
	}
	var rows []row
	// 关联表与规则表各 join 一次:rl 侧的 link_id 已按短链过滤,rl.link_id IS NOT NULL
	// 即"显式关联";全局规则不走关联表(LEFT JOIN 后该列为空)。
	// enabled / scope 顺带取出来填 Rules:结果集里 scope=links 的行必然 rl 命中,
	// 所以不用再查一遍关联表,行内开关的数据就来自这一条 SQL。
	if err := s.db.WithContext(ctx).Table("links l").
		Select("l.id AS link_id, r.id AS rule_id, r.name, r.enabled, r.scope").
		Joins("JOIN rules r ON r.tenant_id = l.tenant_id").
		Joins("LEFT JOIN rule_links rl ON rl.rule_id = r.id AND rl.link_id = l.id").
		Where("l.id IN ? AND (r.scope = ? OR rl.link_id IS NOT NULL)", ids, RuleScopeGlobal).
		Order("r.priority, r.id").Scan(&rows).Error; err != nil {
		return err
	}
	nameMap := make(map[int64][]string, len(links))
	countMap := make(map[int64]int64, len(links))
	briefMap := make(map[int64][]LinkRuleBrief, len(links))
	for _, row := range rows {
		countMap[row.LinkID]++
		if n := len(nameMap[row.LinkID]); n < maxLinkRuleNames {
			nameMap[row.LinkID] = append(nameMap[row.LinkID], row.Name)
		}
		if row.Scope != RuleScopeGlobal {
			briefMap[row.LinkID] = append(briefMap[row.LinkID], LinkRuleBrief{
				ID: row.RuleID, Name: row.Name, Enabled: row.Enabled, Scope: row.Scope,
			})
		}
	}
	for _, l := range links {
		l.RuleCount = countMap[l.ID]
		l.RuleNames = nameMap[l.ID]
		// 空也要给 [] 而不是 nil:界面对这个字段做 map/filter,null 会在每处都要判一次
		if briefs := briefMap[l.ID]; briefs != nil {
			l.Rules = briefs
		} else {
			l.Rules = []LinkRuleBrief{}
		}
	}
	return nil
}

func (s *Store) linkDomainFQDNs(ctx context.Context, linkID int64) ([]string, error) {
	var out []string
	err := s.db.WithContext(ctx).Table("link_domains ld").
		Select("d.fqdn").Joins("JOIN domains d ON d.id = ld.domain_id").
		Where("ld.link_id = ?", linkID).Order("d.id").Pluck("fqdn", &out).Error
	return out, err
}

// linkTargetURLs 按 position 升序返回短链的全部目标 URL。
func (s *Store) linkTargetURLs(ctx context.Context, linkID int64) ([]string, error) {
	var out []string
	err := s.db.WithContext(ctx).Table("link_targets").
		Where("link_id = ?", linkID).Order("position").Pluck("url", &out).Error
	return out, err
}

// effectiveRedirectStatus 返回落库用的跳转方式:落地页型一律 302。
//
// 为什么在 store 侧再归一一次(而不是只在 handler 的 normalizeLandingFields 里):
// 规则裁决命中 `action=redirect` 时走 redirectToTarget,它读的是 link.RedirectStatus。
// 若「301 跳转型 → 落地页型」的切换把 301 留在库里(前端切类型时不发 redirectStatus),
// 那条规则改写路径就会对落地页型短链发出 301。redirect.go 属另一个代理,
// 所以把不变式钉在**写入侧**:任何让 link_type=landing 落库的路径都写 302。
func effectiveRedirectStatus(redirectStatus RedirectStatus, linkType string) RedirectStatus {
	if linkType == LinkTypeLanding {
		return RedirectStatus302
	}
	return redirectStatus
}

// CreateLinkInTx 在**给定事务**内插入短链、目标 URL 与域名关联,返回短链 id。
//
// 与 CreateLink 的区别只在事务边界:本函数不自己开事务,必须由调用方传入 tx。
// 存在的理由是配额:handleCreateLink 需要在 WithQuotaInTx 持有的租户行锁内插入,
// 用 s.db 写会跑在事务外,锁就白加了(见 store/quota.go 的注释)。
//
// 任一 (domain_id, code) 与**存活**短链冲突时返回唯一约束错误,由调用方决定重试。
// 注意:唯一约束冲突会中止整个事务,所以撞码重试必须包在本函数**外面**。
func (s *Store) CreateLinkInTx(ctx context.Context, tx *gorm.DB, tenantID int64, code string, targetURLs []string, redirectStatus RedirectStatus, linkType, landingSource, landingURL string, domainIDs []int64) (int64, error) {
	link := Link{TenantID: tenantID, Code: code,
		RedirectStatus: effectiveRedirectStatus(redirectStatus, linkType),
		LinkType:       linkType, LandingSource: landingSource, LandingURL: landingURL,
		Status: "enabled", RulesEnabled: true, Metadata: Metadata{}}
	if err := tx.WithContext(ctx).Create(&link).Error; err != nil {
		return 0, err
	}
	for i, u := range targetURLs {
		lt := LinkTarget{LinkID: link.ID, URL: u, Position: i}
		if err := tx.WithContext(ctx).Create(&lt).Error; err != nil {
			return 0, err
		}
	}
	for _, dID := range domainIDs {
		ld := LinkDomain{LinkID: link.ID, DomainID: dID, Code: code}
		if err := tx.WithContext(ctx).Create(&ld).Error; err != nil {
			return 0, err
		}
	}
	return link.ID, nil
}

// CreateLink 创建短链并关联域名(自开事务)。
// 任一 (domain_id, code) 与存活短链冲突时返回唯一约束错误(整个创建回滚)。
func (s *Store) CreateLink(ctx context.Context, tenantID int64, code string, targetURLs []string, redirectStatus RedirectStatus, linkType, landingSource, landingURL string, domainIDs []int64) (*Link, error) {
	var linkID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, err := s.CreateLinkInTx(ctx, tx, tenantID, code, targetURLs, redirectStatus,
			linkType, landingSource, landingURL, domainIDs)
		if err != nil {
			return err
		}
		linkID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetLinkByID(ctx, tenantID, linkID)
}

// GetLinkByID 按 id 查询短链(租户隔离;不含逻辑删除)。
func (s *Store) GetLinkByID(ctx context.Context, tenantID, id int64) (*Link, error) {
	var l Link
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).First(&l).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.fillLinkMeta(ctx, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ListLinksByTenant 分页列出租户短链(不含逻辑删除),按创建时间倒序。
func (s *Store) ListLinksByTenant(ctx context.Context, tenantID int64, page, pageSize int) ([]*Link, int, error) {
	return s.listLinks(ctx, tenantID, page, pageSize, false)
}

// ListDeletedLinksByTenant 分页列出租户**已逻辑删除**的短链(回收站视图),
// 按删除时间倒序(最近删的排前面,与用户"刚刚清掉的那批"的预期一致)。
//
// 与 ListLinksByTenant 是互斥的两个视图而不是"加个开关把两边混在一起":
// 回收站是另一个 UI 语境(只有还原/彻底删除两个动作、没有状态开关与规则列),
// 混在一页里会让分页 total 与页面上真正可操作的对象对不上。
func (s *Store) ListDeletedLinksByTenant(ctx context.Context, tenantID int64, page, pageSize int) ([]*Link, int, error) {
	return s.listLinks(ctx, tenantID, page, pageSize, true)
}

// listLinks 列表查询的共同实现;deletedOnly=false 取存活短链,true 取回收站。
func (s *Store) listLinks(ctx context.Context, tenantID int64, page, pageSize int, deletedOnly bool) ([]*Link, int, error) {
	scope := func(db *gorm.DB) *gorm.DB {
		if deletedOnly {
			return db.Where("tenant_id = ? AND deleted_at IS NOT NULL", tenantID)
		}
		return db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	}
	var total int64
	if err := scope(s.db.WithContext(ctx).Model(&Link{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Link
	if err := scope(s.db.WithContext(ctx)).Order("id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	if err := s.fillLinksMeta(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

// LinkUpdate 短链局部更新字段(指针非空才更新)。
type LinkUpdate struct {
	TargetURLs     *[]string
	RedirectStatus *RedirectStatus
	LinkType       *string
	LandingSource  *string
	LandingURL     *string
	Status         *string
	RulesEnabled   *bool
	Metadata       *Metadata
	// DomainIDs 非空时整体替换关联域名(空数组 = 清空关联,由调用方保证不合法场景已拦截)。
	DomainIDs *[]int64
}

// GetLinkByIDAnyStatus 按 id 查询短链(租户隔离;**含**逻辑删除)。
// 回收站里的"还原 / 彻底删除"都作用在已删除记录上,不能再用 GetLinkByID。
func (s *Store) GetLinkByIDAnyStatus(ctx context.Context, tenantID, id int64) (*Link, error) {
	var l Link
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).First(&l).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.fillLinkMeta(ctx, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateLink 更新短链(租户隔离)。domainIDs 替换时若与存活短链冲突返回唯一约束错误。
//
// 读当前值在**事务内**做:原先在事务外先读一遍,两个并发 PATCH 会各自拿到同一份
// 旧快照,后提交的那个用自己的旧值覆盖掉前一个的改动(link_type/status/redirect_status
// 这几个字段尤其明显)。放进同一个事务后,后到的 UPDATE 会阻塞在前一个的写锁上,
// 读到的一定是最新值。
func (s *Store) UpdateLink(ctx context.Context, tenantID, id int64, upd LinkUpdate) (*Link, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cur Link
		// FOR UPDATE:不只是"把读挪进事务",还要让这条读**阻塞**在并发写上。
		// Postgres 默认 READ COMMITTED 下,不加锁的普通 SELECT 不会等前一个
		// 事务提交,后到的 PATCH 会读到旧快照并用自己的旧值覆盖掉前一个的改动;
		// 加锁后它先等前一笔提交完,再读到**已含前一笔结果**的值,才谈得上不丢更新。
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		redirectStatus, status := cur.RedirectStatus, cur.Status
		linkType, landingSource, landingURL := cur.LinkType, cur.LandingSource, cur.LandingURL
		rulesEnabled := cur.RulesEnabled
		if upd.RedirectStatus != nil {
			redirectStatus = *upd.RedirectStatus
		}
		if upd.Status != nil {
			status = *upd.Status
		}
		if upd.RulesEnabled != nil {
			rulesEnabled = *upd.RulesEnabled
		}
		if upd.LinkType != nil {
			linkType = *upd.LinkType
		}
		if upd.LandingSource != nil {
			landingSource = *upd.LandingSource
		}
		if upd.LandingURL != nil {
			landingURL = *upd.LandingURL
		}
		// 落地页型一律 302(写入侧钉死,见 effectiveRedirectStatus 的注释)
		redirectStatus = effectiveRedirectStatus(redirectStatus, linkType)
		updates := map[string]any{
			"redirect_status": redirectStatus, "status": status,
			"link_type": linkType, "landing_source": landingSource, "landing_url": landingURL,
			"rules_enabled": rulesEnabled,
		}
		if upd.Metadata != nil {
			updates["metadata"] = *upd.Metadata
		}
		if err := tx.Model(&Link{}).Where("id = ? AND tenant_id = ?", id, tenantID).
			Updates(updates).Error; err != nil {
			return err
		}
		// TargetURLs 非空时整体替换目标列表(先删后插,保持 position 顺序)
		if upd.TargetURLs != nil {
			if err := tx.Where("link_id = ?", id).Delete(&LinkTarget{}).Error; err != nil {
				return err
			}
			for i, u := range *upd.TargetURLs {
				lt := LinkTarget{LinkID: id, URL: u, Position: i}
				if err := tx.Create(&lt).Error; err != nil {
					return err
				}
			}
		}
		if upd.DomainIDs != nil {
			if err := tx.Where("link_id = ?", id).Delete(&LinkDomain{}).Error; err != nil {
				return err
			}
			for _, dID := range *upd.DomainIDs {
				ld := LinkDomain{LinkID: id, DomainID: dID, Code: cur.Code}
				if err := tx.Create(&ld).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetLinkByID(ctx, tenantID, id)
}

// SoftDeleteLink 逻辑删除(deleted_at 置位,记录保留)。
//
// 副作用(由 migrations/0017 的触发器承担,应用层不写):link_domains.link_deleted
// 随之置真,该短码在存活的关联行上不再占用 —— 软删后同域名可重建同短码。
func (s *Store) SoftDeleteLink(ctx context.Context, tenantID, id int64) error {
	res := s.db.WithContext(ctx).Model(&Link{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", gorm.Expr("now()"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreLink 还原逻辑删除的短链(deleted_at 置 NULL,记录与关联原样恢复)。
//
// 短码是否还能拿回来由数据库回答:触发器把 link_domains.link_deleted 翻回 false,
// 若该 (domain_id, code) 已被另一条存活短链占用,部分唯一索引直接报 23505,
// 调用方据此回 409。应用层不预先"查一下有没有被占"——那是 TOCTOU,
// 判定与写入必须在同一个不变式里。
//
// 语义:只还原**已逻辑删除**的短链;传入存活或不存在的 id 一律 ErrNotFound(幂等)。
func (s *Store) RestoreLink(ctx context.Context, tenantID, id int64) error {
	res := s.db.WithContext(ctx).Model(&Link{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NOT NULL", id, tenantID).
		Update("deleted_at", nil)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreLinks 批量还原(语义同 RestoreLink)。
// 逐条执行而非一条 UPDATE:还原可能因短码被占用而对某条失败,而批量端点需要
// 区分"哪些真的还原了"(返回给前端刷新列表),不能让一条失败拖垮整批。
// 返回实际还原条数。
func (s *Store) RestoreLinks(ctx context.Context, tenantID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// 先取本租户真实命中且处于已删除态的 id(与 DetachDomain 同范式)
	var restoreable []int64
	if err := s.db.WithContext(ctx).Model(&Link{}).
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NOT NULL", tenantID, ids).
		Pluck("id", &restoreable).Error; err != nil {
		return 0, err
	}
	var n int64
	for _, id := range restoreable {
		// 短码被占用会返回唯一约束错误,跳过继续(逐条独立事务语义:
		// 单条失败不影响其它条目)
		if err := s.RestoreLink(ctx, tenantID, id); err != nil {
			if IsUniqueViolation(err) {
				continue
			}
			return n, err
		}
		n++
	}
	return n, nil
}

// PurgeLink 物理删除短链(连同关联与访问记录)。
// PurgeLink 物理删除短链(关联、目标、访问明细随库内 CASCADE 消失)。
//
// 这里**刻意不**失效租户的规则快照:快照里残留的 linkID 永远不会被命中
// (id 来自 BIGSERIAL 且不复用,求值又排在短链可用性之后),加了纯属写放大。
// ⚠️ 这个判断依赖「短链 id 不可复用」这个前提。若将来改成按租户分段分配、
// 或引入 id 回收复用,本函数与 PurgeLinks 必须同时调用 ruleCache.Invalidate(tenantID)。
func (s *Store) PurgeLink(ctx context.Context, tenantID, id int64) error {
	res := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&Link{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteLinks 批量逻辑删除(deleted_at 置位,记录与关联保留)。
// 仅作用于本租户且尚未逻辑删除的 id,跨租户/已删除/不存在的 id 静默跳过(幂等),
// 返回实际置位行数。ids 为空直接返回 0,不生成非法的 IN () 条件。
func (s *Store) SoftDeleteLinks(ctx context.Context, tenantID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := s.db.WithContext(ctx).Model(&Link{}).
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, ids).
		Update("deleted_at", gorm.Expr("now()"))
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// PurgeLinks 批量物理删除短链(连同 visits/link_targets/link_domains,由库内
// ON DELETE CASCADE 承担;已逻辑删除的行也会被清除)。
// 仅作用于本租户的 id,跨租户/不存在的 id 静默跳过(幂等);
// 返回"实际被删除的短链 ID"——落地页文件按短链 ID 存放在租户间共享的目录中,
// 调用方必须只清理这批 ID 的文件,否则跨租户传入的 id 会误删他人落地页。
// ids 为空直接返回 nil,不生成非法的 IN () 条件。
func (s *Store) PurgeLinks(ctx context.Context, tenantID int64, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var purged []int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先取本租户真实命中的 id(与 DetachDomain 同范式),再据此删除
		if err := tx.Model(&Link{}).
			Where("tenant_id = ? AND id IN ?", tenantID, ids).
			Pluck("id", &purged).Error; err != nil {
			return err
		}
		if len(purged) == 0 {
			return nil
		}
		return tx.Where("id IN ?", purged).Delete(&Link{}).Error
	})
	if err != nil {
		return nil, err
	}
	return purged, nil
}

// CountActiveLinks 按"尚未物理删除"计数(含逻辑删除行),用于配额校验。
func (s *Store) CountActiveLinks(ctx context.Context, tenantID int64) (int, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Link{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// lookupRow 跳转/落地路由的命中行(短链 + 域名 + deleted_at)。
type lookupRow struct {
	LinkID           int64
	LinkTenantID     int64
	Code             string
	RedirectStatus   RedirectStatus
	LinkType         string
	LandingSource    string
	LandingURL       string
	LinkStatus       string
	LinkRulesEnabled bool
	LinkDeletedAt    *time.Time
	LinkCreatedAt    time.Time
	DomainID         int64
	DomainTenantID   int64
	FQDN             string
	Description      string
	Origin           string
	DomainStatus     string
	CertStatus       string
	ActivatedAt      *time.Time
	DomainCreatedAt  time.Time
}

// link 把命中行装配为 Link(含轮询目标列表)。
func (r lookupRow) link(targets []string) *Link {
	return &Link{
		ID: r.LinkID, TenantID: r.LinkTenantID, Code: r.Code,
		RedirectStatus: r.RedirectStatus, LinkType: r.LinkType,
		LandingSource: r.LandingSource, LandingURL: r.LandingURL,
		Status: r.LinkStatus, RulesEnabled: r.LinkRulesEnabled, DeletedAt: r.LinkDeletedAt,
		CreatedAt: r.LinkCreatedAt, TargetURLs: targets,
	}
}

// domain 把命中行装配为 Domain。
func (r lookupRow) domain() *Domain {
	return &Domain{
		ID: r.DomainID, TenantID: r.DomainTenantID, FQDN: r.FQDN, Description: r.Description,
		Origin: r.Origin, Status: r.DomainStatus, CertStatus: r.CertStatus,
		ActivatedAt: r.ActivatedAt, CreatedAt: r.DomainCreatedAt,
	}
}

// lookupLink 按 域名 + 短码 命中一行(ResolveLink 与 LookupLinkForVisit 共用,避免两份 SQL 漂移)。
// strict=true 追加 "短链未删除且启用" 条件;strict=false 只要求短码命中(调用方自行判定不可用原因)。
// 两种口径都保留 d.status='active' 与租户 active:这两种失败无法归属到具体短链,不计明细。
//
// 排序不变式:同一 (domain, code) 允许"一行未删除 + 任意多行已软删"并存(部分唯一索引),
// 宽松口径必须优先命中未删除那行,其次取最新的软删行;否则复用短码后新链接会随机 404,
// 访问明细也会记到旧链接上。
func (s *Store) lookupLink(ctx context.Context, domainID int64, code string, strict bool) (lookupRow, error) {
	linkCond := ""
	if strict {
		linkCond = "AND l.deleted_at IS NULL AND l.status = 'enabled'"
	}
	var row lookupRow
	// linkCond 只由上面两个常量分支拼接,不拼接任何外部输入。
	sql := `SELECT l.id AS link_id, l.tenant_id AS link_tenant_id, l.code,
	        l.redirect_status, l.link_type, l.landing_source, l.landing_url,
	        l.status AS link_status, l.rules_enabled AS link_rules_enabled, l.deleted_at AS link_deleted_at, l.created_at AS link_created_at,
	        d.id AS domain_id, d.tenant_id AS domain_tenant_id, d.fqdn, d.description,
	        d.origin, d.status AS domain_status, d.cert_status, d.activated_at,
	        d.created_at AS domain_created_at
	     FROM link_domains ld
	     JOIN links l ON l.id = ld.link_id
	     JOIN domains d ON d.id = ld.domain_id
	     JOIN tenants t ON t.id = d.tenant_id AND t.id = l.tenant_id
	     WHERE ld.domain_id = ? AND ld.code = ? ` + linkCond + `
	       AND d.status = 'active' AND t.status = 'active'
	     ORDER BY ld.link_deleted, l.id DESC
	     LIMIT 1`
	res := s.db.WithContext(ctx).Raw(sql, domainID, code).Scan(&row)
	if res.Error != nil {
		return lookupRow{}, res.Error
	}
	if res.RowsAffected == 0 {
		return lookupRow{}, ErrNotFound
	}
	return row, nil
}

// ResolveLink 跳转/落地路由:在 active 域名下按短码命中未删除、启用的短链。
// 不推进轮询计数(选目标用 PickTarget);未命中返回 ErrNotFound。
func (s *Store) ResolveLink(ctx context.Context, domainID int64, code string) (*Link, *Domain, error) {
	row, err := s.lookupLink(ctx, domainID, code, true)
	if err != nil {
		return nil, nil, err
	}
	targets, err := s.linkTargetURLs(ctx, row.LinkID)
	if err != nil {
		return nil, nil, err
	}
	return row.link(targets), row.domain(), nil
}

// LookupLinkForVisit 供"记访问明细"使用的宽松命中:短码命中即返回,
// 短链是否可用交给调用方按 reason 判定(便于把 link_disabled / link_deleted 这类
// 可归属到该短链的失败也落一行明细)。
// 返回值:未命中返回 ErrNotFound(此时不得记任何行);
// 命中但不可用时 reason 为 VisitReasonLinkDeleted / VisitReasonLinkDisabled;
// 可用时 reason 为空串。不推进轮询计数(选目标用 PickTarget)。
func (s *Store) LookupLinkForVisit(ctx context.Context, domainID int64, code string) (*Link, *Domain, string, error) {
	row, err := s.lookupLink(ctx, domainID, code, false)
	if err != nil {
		return nil, nil, "", err
	}
	// 逻辑删除优先于停用:删除后短链已不可见,归属也随记录保留而继续
	reason := ""
	switch {
	case row.LinkDeletedAt != nil:
		reason = VisitReasonLinkDeleted
	case row.LinkStatus != "enabled":
		reason = VisitReasonLinkDisabled
	}
	targets, err := s.linkTargetURLs(ctx, row.LinkID)
	if err != nil {
		return nil, nil, "", err
	}
	return row.link(targets), row.domain(), reason, nil
}

// PickTarget 轮询选一个目标 URL(rr_index 自增);无目标返回 ErrNotFound。
//
// 单目标直接返回、不碰 DB。绝大多数短链只有一个出口,此时轮询是恒等映射
// (targets[0]),而那条 `UPDATE links SET rr_index = rr_index + 1` 的代价却是实的:
//
//	· 每次跳转都在**同一条 links 行**上抢行锁,一条热门短链的全部并发跳转在这个
//	  行锁上排队,而它们本来互不相干(结果只取决于 rr_index,锁保护不了任何不变量);
//	· 行版本无限膨胀,每次跳转一个新版本,autovacuum 压力与表膨胀都来自这行。
//
// 省掉它对单目标短链是纯赚,且不改变任何可观测行为(rr_index 对单目标没有意义)。
//
// 多目标仍保留原来的**单语句** `UPDATE ... RETURNING`:轮询指针必须原子自增,
// 读-改-写(read rr_index → 算 → 写回)会让并发下同一个目标被重复选中。
// 导出签名保持不变(redirect.go / landing.go 都按它选目标)。
func (s *Store) PickTarget(ctx context.Context, linkID int64) (string, error) {
	targets, err := s.linkTargetURLs(ctx, linkID)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", ErrNotFound
	}
	if len(targets) == 1 {
		return targets[0], nil
	}
	var cur int64
	if err := s.db.WithContext(ctx).Raw(
		`UPDATE links SET rr_index = rr_index + 1 WHERE id = ? RETURNING rr_index`, linkID).
		Scan(&cur).Error; err != nil {
		return "", err
	}
	n := len(targets)
	return targets[int(((cur-1)%int64(n)+int64(n))%int64(n))], nil
}

// IncrementClicks 点击计数原子 +1(落地页按钮回传;失败由调用方决定是否阻断)。
func (s *Store) IncrementClicks(ctx context.Context, linkID int64) error {
	return s.db.WithContext(ctx).Exec(
		`UPDATE links SET clicks = clicks + 1 WHERE id = ?`, linkID).Error
}

// RedirectStatus 跳转方式(契约枚举:"301" | "302";JSON 序列化为字符串,
// 库中 redirect_status 为 INT,通过 Valuer/Scanner 自动转换)。
type RedirectStatus string

const (
	RedirectStatus301 RedirectStatus = "301"
	RedirectStatus302 RedirectStatus = "302"
)

func (r RedirectStatus) Value() (driver.Value, error) {
	return strconv.Atoi(string(r))
}

func (r *RedirectStatus) Scan(v any) error {
	switch n := v.(type) {
	case int64:
		*r = RedirectStatus(strconv.FormatInt(n, 10))
	case int32:
		*r = RedirectStatus(strconv.FormatInt(int64(n), 10))
	case int:
		*r = RedirectStatus(strconv.Itoa(n))
	case []byte:
		*r = RedirectStatus(string(n))
	default:
		*r = RedirectStatus("")
	}
	return nil
}
