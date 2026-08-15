package store

import (
	"context"
	"database/sql/driver"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type Link struct {
	ID              int64          `json:"id" gorm:"primaryKey"`
	TenantID        int64          `json:"-" gorm:"column:tenant_id"`
	Code            string         `json:"code"`
	TargetURLs      []string       `json:"targetUrls" gorm:"-"`
	RedirectStatus  RedirectStatus `json:"redirectStatus" gorm:"column:redirect_status"`
	LinkType        string         `json:"linkType" gorm:"column:link_type"`
	LandingSource   string         `json:"landingSource" gorm:"column:landing_source"`
	LandingURL      string         `json:"landingUrl" gorm:"column:landing_url"`
	Status          string         `json:"status"`
	Clicks          int64          `json:"clicks" gorm:"column:clicks"`
	DeletedAt       *time.Time     `json:"-" gorm:"column:deleted_at"`
	Domains         []string       `json:"domains" gorm:"-"`
	Visits          int64          `json:"visits" gorm:"-"`
	LandingUploaded bool           `json:"landingUploaded" gorm:"-"`
	CreatedAt       time.Time      `json:"createdAt" gorm:"column:created_at"`
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

// LinkDomain 短链-域名关联(唯一约束 (domain_id, code))。
type LinkDomain struct {
	ID       int64 `gorm:"primaryKey"`
	LinkID   int64 `gorm:"column:link_id"`
	DomainID int64 `gorm:"column:domain_id"`
	Code     string
}

// LinkTarget 短链目标 URL(position 从 0 开始,按 position 升序;(link_id, position) 唯一)。
type LinkTarget struct {
	ID       int64 `gorm:"primaryKey"`
	LinkID   int64 `gorm:"column:link_id"`
	URL      string
	Position int
}

// fillLinkMeta 补充 domains(fqdn 列表)、targetUrls 与 visits 计数。
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
	return nil
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
		Count  int64 `gorm:"column:count"`
	}
	var visitRows []visitRow
	if err := s.db.WithContext(ctx).Table("visits").
		Select("link_id, count(*) AS count").
		Where("link_id IN ?", ids).
		Group("link_id").
		Scan(&visitRows).Error; err != nil {
		return err
	}
	visitMap := make(map[int64]int64, len(visitRows))
	for _, row := range visitRows {
		visitMap[row.LinkID] = row.Count
	}

	for _, l := range links {
		l.Domains = domainMap[l.ID]
		l.TargetURLs = targetMap[l.ID]
		l.Visits = visitMap[l.ID]
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

// CreateLink 创建短链并关联域名。
// 任一 (domain_id, code) 与既有关联冲突时返回唯一约束错误(整个创建回滚)。
func (s *Store) CreateLink(ctx context.Context, tenantID int64, code string, targetURLs []string, redirectStatus RedirectStatus, linkType, landingSource, landingURL string, domainIDs []int64) (*Link, error) {
	var linkID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		link := Link{TenantID: tenantID, Code: code, RedirectStatus: redirectStatus,
			LinkType: linkType, LandingSource: landingSource, LandingURL: landingURL, Status: "enabled"}
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
		linkID = link.ID
		for i, u := range targetURLs {
			lt := LinkTarget{LinkID: linkID, URL: u, Position: i}
			if err := tx.Create(&lt).Error; err != nil {
				return err
			}
		}
		for _, dID := range domainIDs {
			ld := LinkDomain{LinkID: linkID, DomainID: dID, Code: code}
			if err := tx.Create(&ld).Error; err != nil {
				return err
			}
		}
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
	var total int64
	if err := s.db.WithContext(ctx).Model(&Link{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Link
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Order("id DESC").
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
	// DomainIDs 非空时整体替换关联域名(空数组 = 清空关联,由调用方保证不合法场景已拦截)。
	DomainIDs *[]int64
}

// UpdateLink 更新短链(租户隔离)。domainIDs 替换时若与既有关联冲突返回唯一约束错误。
func (s *Store) UpdateLink(ctx context.Context, tenantID, id int64, upd LinkUpdate) (*Link, error) {
	cur, err := s.GetLinkByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		redirectStatus, status := cur.RedirectStatus, cur.Status
		linkType, landingSource, landingURL := cur.LinkType, cur.LandingSource, cur.LandingURL
		if upd.RedirectStatus != nil {
			redirectStatus = *upd.RedirectStatus
		}
		if upd.Status != nil {
			status = *upd.Status
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
		if err := tx.Model(&Link{}).Where("id = ? AND tenant_id = ?", id, tenantID).
			Updates(map[string]any{"redirect_status": redirectStatus, "status": status,
				"link_type": linkType, "landing_source": landingSource, "landing_url": landingURL}).Error; err != nil {
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

// PurgeLink 物理删除短链(连同关联与访问记录)。
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

// CountActiveLinks 按"尚未物理删除"计数(含逻辑删除行),用于配额校验。
func (s *Store) CountActiveLinks(ctx context.Context, tenantID int64) (int, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Link{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// ResolveLink 跳转/落地路由:在 active 域名下按短码命中未删除、启用的短链。
// 不推进轮询计数(选目标用 PickTarget);未命中返回 ErrNotFound。
func (s *Store) ResolveLink(ctx context.Context, domainID int64, code string) (*Link, *Domain, error) {
	var row struct {
		LinkID          int64
		LinkTenantID    int64
		Code            string
		RedirectStatus  RedirectStatus
		LinkType        string
		LandingSource   string
		LandingURL      string
		LinkStatus      string
		LinkCreatedAt   time.Time
		DomainID        int64
		DomainTenantID  int64
		FQDN            string
		Description     string
		Origin          string
		DomainStatus    string
		CertStatus      string
		ActivatedAt     *time.Time
		DomainCreatedAt time.Time
	}
	res := s.db.WithContext(ctx).Raw(
		`SELECT l.id AS link_id, l.tenant_id AS link_tenant_id, l.code,
		        l.redirect_status, l.link_type, l.landing_source, l.landing_url,
		        l.status AS link_status, l.created_at AS link_created_at,
		        d.id AS domain_id, d.tenant_id AS domain_tenant_id, d.fqdn, d.description,
		        d.origin, d.status AS domain_status, d.cert_status, d.activated_at,
		        d.created_at AS domain_created_at
		 FROM link_domains ld
		 JOIN links l ON l.id = ld.link_id
		 JOIN domains d ON d.id = ld.domain_id
		 JOIN tenants t ON t.id = d.tenant_id AND t.id = l.tenant_id
		 WHERE ld.domain_id = ? AND ld.code = ?
		   AND l.deleted_at IS NULL AND l.status = 'enabled'
		   AND d.status = 'active' AND t.status = 'active'`,
		domainID, code).Scan(&row)
	if res.Error != nil {
		return nil, nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil, ErrNotFound
	}
	targets, err := s.linkTargetURLs(ctx, row.LinkID)
	if err != nil {
		return nil, nil, err
	}
	return &Link{
			ID: row.LinkID, TenantID: row.LinkTenantID, Code: row.Code,
			RedirectStatus: row.RedirectStatus, LinkType: row.LinkType,
			LandingSource: row.LandingSource, LandingURL: row.LandingURL,
			Status: row.LinkStatus, CreatedAt: row.LinkCreatedAt, TargetURLs: targets,
		}, &Domain{
			ID: row.DomainID, TenantID: row.DomainTenantID, FQDN: row.FQDN, Description: row.Description,
			Origin: row.Origin, Status: row.DomainStatus, CertStatus: row.CertStatus,
			ActivatedAt: row.ActivatedAt, CreatedAt: row.DomainCreatedAt,
		}, nil
}

// PickTarget 轮询选一个目标 URL(rr_index 自增);无目标返回 ErrNotFound。
func (s *Store) PickTarget(ctx context.Context, linkID int64) (string, error) {
	targets, err := s.linkTargetURLs(ctx, linkID)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", ErrNotFound
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
