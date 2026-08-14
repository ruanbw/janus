package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type APIKey struct {
	ID        int64      `json:"id" gorm:"primaryKey"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"createdAt" gorm:"column:created_at"`
	Key       string     `json:"key,omitempty" gorm:"-"` // 明文仅在创建响应中出现一次
	TenantID  int64      `json:"-" gorm:"column:tenant_id"`
	KeyHash   string     `json:"-" gorm:"column:key_hash"`
	RevokedAt *time.Time `json:"-" gorm:"column:revoked_at"`
}

// CreateAPIKey 创建 API Key(keyHash 为 SHA-256 哈希)。
func (s *Store) CreateAPIKey(ctx context.Context, tenantID int64, name, keyHash string) (*APIKey, error) {
	k := APIKey{TenantID: tenantID, Name: name, KeyHash: keyHash}
	if err := s.db.WithContext(ctx).Create(&k).Error; err != nil {
		return nil, err
	}
	return s.GetAPIKey(ctx, tenantID, k.ID)
}

func (s *Store) GetAPIKey(ctx context.Context, tenantID, id int64) (*APIKey, error) {
	var k APIKey
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND revoked_at IS NULL", id, tenantID).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	k.Key = ""
	return &k, nil
}

// ListAPIKeys 列出未吊销的 API Key。
func (s *Store) ListAPIKeys(ctx context.Context, tenantID int64) ([]*APIKey, error) {
	var out []*APIKey
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND revoked_at IS NULL", tenantID).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetTenantIDByAPIKeyHash 按 key 哈希查未吊销的 Key 所属租户。
func (s *Store) GetTenantIDByAPIKeyHash(ctx context.Context, keyHash string) (int64, error) {
	var tenantID int64
	res := s.db.WithContext(ctx).Model(&APIKey{}).
		Select("tenant_id").Where("key_hash = ? AND revoked_at IS NULL", keyHash).Scan(&tenantID)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return tenantID, nil
}

// RevokeAPIKey 吊销(软删除:置 revoked_at)。
func (s *Store) RevokeAPIKey(ctx context.Context, tenantID, id int64) error {
	res := s.db.WithContext(ctx).Model(&APIKey{}).
		Where("id = ? AND tenant_id = ? AND revoked_at IS NULL", id, tenantID).
		Update("revoked_at", gorm.Expr("now()"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
