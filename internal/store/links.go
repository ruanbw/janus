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
	ID             int64          `json:"id" gorm:"primaryKey"`
	TenantID       int64          `json:"-" gorm:"column:tenant_id"`
	Code           string         `json:"code"`
	TargetURL      string         `json:"targetUrl" gorm:"column:target_url"`
	RedirectStatus RedirectStatus `json:"redirectStatus" gorm:"column:redirect_status"`
	Status         string         `json:"status"`
	DeletedAt      *time.Time     `json:"-" gorm:"column:deleted_at"`
	Domains        []string       `json:"domains" gorm:"-"`
	Visits         int64          `json:"visits" gorm:"-"`
	CreatedAt      time.Time      `json:"createdAt" gorm:"column:created_at"`
}

// LinkDomain 短链-域名关联(唯一约束 (domain_id, code))。
type LinkDomain struct {
	ID       int64 `gorm:"primaryKey"`
	LinkID   int64 `gorm:"column:link_id"`
	DomainID int64 `gorm:"column:domain_id"`
	Code     string
}

// fillLinkMeta 补充 domains(fqdn 列表)与 visits 计数。
func (s *Store) fillLinkMeta(ctx context.Context, l *Link) error {
	domains, err := s.linkDomainFQDNs(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Domains = domains
	n, err := s.CountVisitsByLink(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Visits = n
	return nil
}

func (s *Store) linkDomainFQDNs(ctx context.Context, linkID int64) ([]string, error) {
	var out []string
	err := s.db.WithContext(ctx).Table("link_domains ld").
		Select("d.fqdn").Joins("JOIN domains d ON d.id = ld.domain_id").
		Where("ld.link_id = ?", linkID).Order("d.id").Pluck("fqdn", &out).Error
	return out, err
}

// CreateLink 创建短链并关联域名。
// 任一 (domain_id, code) 与既有关联冲突时返回唯一约束错误(整个创建回滚)。
func (s *Store) CreateLink(ctx context.Context, tenantID int64, code, targetURL string, redirectStatus RedirectStatus, domainIDs []int64) (*Link, error) {
	var linkID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		link := Link{TenantID: tenantID, Code: code, TargetURL: targetURL, RedirectStatus: redirectStatus, Status: "enabled"}
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
		linkID = link.ID
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
	for _, l := range out {
		if err := s.fillLinkMeta(ctx, l); err != nil {
			return nil, 0, err
		}
	}
	return out, int(total), nil
}

// LinkUpdate 短链局部更新字段(指针非空才更新)。
type LinkUpdate struct {
	TargetURL      *string
	RedirectStatus *RedirectStatus
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
		targetURL, redirectStatus, status := cur.TargetURL, cur.RedirectStatus, cur.Status
		if upd.TargetURL != nil {
			targetURL = *upd.TargetURL
		}
		if upd.RedirectStatus != nil {
			redirectStatus = *upd.RedirectStatus
		}
		if upd.Status != nil {
			status = *upd.Status
		}
		if err := tx.Model(&Link{}).Where("id = ? AND tenant_id = ?", id, tenantID).
			Updates(map[string]any{"target_url": targetURL, "redirect_status": redirectStatus, "status": status}).Error; err != nil {
			return err
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

// ResolveRedirect 跳转路由:在 active 域名下按短码命中未删除、启用的短链。
// 返回短链与命中的域名记录;未命中返回 ErrNotFound。
func (s *Store) ResolveRedirect(ctx context.Context, domainID int64, code string) (*Link, *Domain, error) {
	var row struct {
		LinkID          int64
		LinkTenantID    int64
		Code            string
		TargetURL       string
		RedirectStatus  RedirectStatus
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
		`SELECT l.id AS link_id, l.tenant_id AS link_tenant_id, l.code, l.target_url,
		        l.redirect_status, l.status AS link_status, l.created_at AS link_created_at,
		        d.id AS domain_id, d.tenant_id AS domain_tenant_id, d.fqdn, d.description,
		        d.origin, d.status AS domain_status, d.cert_status, d.activated_at,
		        d.created_at AS domain_created_at
		 FROM link_domains ld
		 JOIN links l ON l.id = ld.link_id
		 JOIN domains d ON d.id = ld.domain_id
		 WHERE ld.domain_id = ? AND ld.code = ?
		   AND l.deleted_at IS NULL AND l.status = 'enabled' AND d.status = 'active'`,
		domainID, code).Scan(&row)
	if res.Error != nil {
		return nil, nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil, ErrNotFound
	}
	return &Link{
			ID: row.LinkID, TenantID: row.LinkTenantID, Code: row.Code, TargetURL: row.TargetURL,
			RedirectStatus: row.RedirectStatus, Status: row.LinkStatus, CreatedAt: row.LinkCreatedAt,
		}, &Domain{
			ID: row.DomainID, TenantID: row.DomainTenantID, FQDN: row.FQDN, Description: row.Description,
			Origin: row.Origin, Status: row.DomainStatus, CertStatus: row.CertStatus,
			ActivatedAt: row.ActivatedAt, CreatedAt: row.DomainCreatedAt,
		}, nil
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
