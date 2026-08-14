package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Domain struct {
	ID          int64      `json:"id" gorm:"primaryKey"`
	TenantID    int64      `json:"-" gorm:"column:tenant_id"`
	FQDN        string     `json:"fqdn"`
	Description string     `json:"description"`
	Origin      string     `json:"origin"`
	Status      string     `json:"status"`
	CertStatus  string     `json:"certStatus" gorm:"column:cert_status"`
	ActivatedAt *time.Time `json:"activatedAt" gorm:"column:activated_at"`
	CreatedAt   time.Time  `json:"createdAt" gorm:"column:created_at"`
}

// CreateDomain 创建域名记录(fqdn 唯一冲突返回唯一约束错误)。
func (s *Store) CreateDomain(ctx context.Context, tenantID int64, fqdn, origin, description string) (*Domain, error) {
	d := Domain{TenantID: tenantID, FQDN: fqdn, Origin: origin, Status: "pending", CertStatus: "pending", Description: description}
	if err := s.db.WithContext(ctx).Create(&d).Error; err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, d.ID)
}

// CreatePlatformDomain 创建平台默认域名:创建即 active(泛域名解析已指向本机),cert_status=pending。
func (s *Store) CreatePlatformDomain(ctx context.Context, tenantID int64, fqdn string) (*Domain, error) {
	d := Domain{TenantID: tenantID, FQDN: fqdn, Origin: "platform", Status: "active", CertStatus: "pending", ActivatedAt: nowPtr()}
	if err := s.db.WithContext(ctx).Create(&d).Error; err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, d.ID)
}

func nowPtr() *time.Time {
	t := time.Now()
	return &t
}

func (s *Store) GetDomainByID(ctx context.Context, id int64) (*Domain, error) {
	var d Domain
	if err := s.db.WithContext(ctx).First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDomainByFQDN(ctx context.Context, fqdn string) (*Domain, error) {
	var d Domain
	if err := s.db.WithContext(ctx).Where("fqdn = ?", fqdn).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDomainsByTenant(ctx context.Context, tenantID int64) ([]*Domain, error) {
	var out []*Domain
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListDomainsByStatus 供后台任务扫描(如 DNS 重试队列)。
func (s *Store) ListDomainsByStatus(ctx context.Context, statuses ...string) ([]*Domain, error) {
	var out []*Domain
	if err := s.db.WithContext(ctx).Where("status IN ?", statuses).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetDomainStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Update("status", status).Error
}

// SetDomainActive 标记 DNS 校验通过:置 active、记录校验时间与激活时间。
func (s *Store) SetDomainActive(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":         "active",
		"dns_checked_at": gorm.Expr("now()"),
		"activated_at":   gorm.Expr("COALESCE(activated_at, now())"),
	}).Error
}

func (s *Store) MarkDomainDNSChecked(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Update("dns_checked_at", gorm.Expr("now()")).Error
}

func (s *Store) SetDomainCertStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).
		Updates(map[string]any{"cert_status": status, "cert_probed_at": gorm.Expr("now()")}).Error
}

// SetDomainCertStatusByFQDN 按 fqdn 更新证书状态(验证通过后触发探活用)。
func (s *Store) SetDomainCertStatusByFQDN(ctx context.Context, fqdn, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("fqdn = ?", fqdn).
		Updates(map[string]any{"cert_status": status, "cert_probed_at": gorm.Expr("now()")}).Error
}

// DeleteDomain 物理删除域名(调用方须先保证其短链关联已清空)。
func (s *Store) DeleteDomain(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&Domain{}, id).Error
}

// DomainAuth 返回授权端点所需的域名与租户状态。
type DomainAuth struct {
	Status       string
	Origin       string
	TenantStatus string
}

// GetDomainAuth 按 fqdn 查询域名与所属租户状态(用于 Caddy 授权端点)。
func (s *Store) GetDomainAuth(ctx context.Context, fqdn string) (*DomainAuth, error) {
	var a DomainAuth
	res := s.db.WithContext(ctx).Raw(
		`SELECT d.status, d.origin, t.status AS tenant_status FROM domains d
		 JOIN tenants t ON t.id = d.tenant_id WHERE d.fqdn = ?`, fqdn).Scan(&a)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &a, nil
}

// CountNonDeletedLinksOnDomain 统计某域名上"未删除"短链的关联数。
func (s *Store) CountNonDeletedLinksOnDomain(ctx context.Context, domainID int64) (int, error) {
	var n int64
	err := s.db.WithContext(ctx).Table("link_domains").
		Joins("JOIN links l ON l.id = link_domains.link_id").
		Where("link_domains.domain_id = ? AND l.deleted_at IS NULL", domainID).Count(&n).Error
	return int(n), err
}

// DetachDomain 删除域名:清空该域名全部关联;对"仅关联该域名"且已逻辑删除的短链做物理清除(连同访问记录)。
// 调用方须先确认无未删除短链关联(CountNonDeletedLinksOnDomain == 0)。
func (s *Store) DetachDomain(ctx context.Context, domainID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 该域名上"已逻辑删除"的短链,若其关联数(全部域名)为 1(即仅此域名),物理删除
		if err := tx.Exec(`DELETE FROM links WHERE id IN (
			SELECT l.id FROM links l
			JOIN link_domains ld ON ld.link_id = l.id AND ld.domain_id = ?
			WHERE l.deleted_at IS NOT NULL
			  AND (SELECT count(*) FROM link_domains WHERE link_id = l.id) = 1
		)`, domainID).Error; err != nil {
			return err
		}
		if err := tx.Where("domain_id = ?", domainID).Delete(&LinkDomain{}).Error; err != nil {
			return err
		}
		if err := tx.Where("domain_id = ?", domainID).Delete(&Visit{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Domain{}, domainID).Error
	})
}

// DomainFQDNExists 判断 fqdn 是否已存在(用于注册 slug 与域名冲突校验)。
func (s *Store) DomainFQDNExists(ctx context.Context, fqdn string) (bool, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Domain{}).Where("fqdn = ?", fqdn).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
