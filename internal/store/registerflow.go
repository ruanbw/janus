package store

// 注册流程的数据层实现:把「建租户 + 建平台默认域名 + 签发验证 token」压进同一个
// 事务。放在单独文件里是因为这是注册专有的组合写入,不属于任何单个实体的 CRUD。

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// RegisterTenant 在一个事务内创建 pending 租户、它的平台默认域名与一枚
// verify 邮箱 token,成功后返回带 DefaultDomain 的租户。
//
// 为什么必须原子:原实现是四步裸跑(CreateTenant → CreatePlatformDomain →
// CreateEmailToken → 发信)。中间任何一步失败,租户行都已落库 —— 用户拿到 5xx,
// 重试注册又被"邮箱已被注册"/"slug 已被占用"挡成 409,而默认域名或验证 token
// 并不存在,没有任何自助恢复入口,只能进数据库手工修。改成单事务后,注册要么
// 整体成功、要么什么都没发生(唯一索引的并发撞车也一并回滚)。
//
// fqdn 由调用方给出(默认域名形态是产品决策,不属于数据层),本函数不做归一化,
// 与 CreatePlatformDomain 的既有行为保持一致。
func (s *Store) RegisterTenant(ctx context.Context, email, passwordHash, slug, fqdn, tokenHash string, tokenTTL time.Duration, isSuperAdmin bool) (*Tenant, error) {
	var tenantID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tier Tier
		if err := tx.Where("name = 'free'").First(&tier).Error; err != nil {
			return err
		}

		t := Tenant{
			Email: email, Slug: slug, Status: "pending",
			IsSuperAdmin: isSuperAdmin, TierID: tier.ID,
			TokenVersion: 1,
		}
		if passwordHash != "" {
			t.PasswordHash = &passwordHash
		}
		if err := tx.Create(&t).Error; err != nil {
			return err
		}

		// 平台默认域名:创建即 active(泛域名解析已指向本机),证书待探活。
		d := Domain{
			TenantID: t.ID, FQDN: fqdn, Origin: "platform",
			Status: "active", CertStatus: "pending", ActivatedAt: nowPtr(),
		}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}

		tok := EmailToken{
			TenantID:  t.ID,
			TokenHash: tokenHash,
			Kind:      "verify",
			ExpiresAt: time.Now().Add(tokenTTL),
		}
		if err := tx.Create(&tok).Error; err != nil {
			return err
		}

		tenantID = t.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetTenantByID(ctx, tenantID)
}
