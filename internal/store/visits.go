package store

import (
	"context"
	"time"
)

type Visit struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	LinkID    int64     `json:"linkId" gorm:"column:link_id"`
	DomainID  int64     `json:"-" gorm:"column:domain_id"`
	Domain    string    `json:"domain" gorm:"->"` // 只读字段,查询时 join 填充
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent" gorm:"column:user_agent"`
	Referer   string    `json:"referer"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at"`
}

// InsertVisit 记录一次成功跳转(含访问者 IP)。
func (s *Store) InsertVisit(ctx context.Context, linkID, domainID int64, ip, userAgent, referer string) error {
	v := Visit{LinkID: linkID, DomainID: domainID, IP: ip, UserAgent: userAgent, Referer: referer}
	return s.db.WithContext(ctx).Create(&v).Error
}

func (s *Store) CountVisitsByLink(ctx context.Context, linkID int64) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).Where("link_id = ?", linkID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListVisitsByLink 分页访问列表(按时间倒序)。
func (s *Store) ListVisitsByLink(ctx context.Context, linkID int64, page, pageSize int) ([]*Visit, int, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).Where("link_id = ?", linkID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Visit
	err := s.db.WithContext(ctx).Table("visits v").
		Select("v.id, v.link_id, d.fqdn AS domain, v.ip, v.user_agent, v.referer, v.created_at").
		Joins("JOIN domains d ON d.id = v.domain_id").
		Where("v.link_id = ?", linkID).Order("v.id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).Scan(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

// CleanupVisitsBefore 清理保留期前的访问记录。
func (s *Store) CleanupVisitsBefore(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&Visit{})
	return res.RowsAffected, res.Error
}
