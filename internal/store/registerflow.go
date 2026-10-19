package store

// 注册流程的数据层实现:把「建租户 + 激活 + 建平台默认域名」压进同一个事务。
// 放在单独文件里是因为这是注册专有的组合写入,不属于任何单个实体的 CRUD。

import (
	"context"

	"gorm.io/gorm"
)

// RegisterTenant 在一个事务内创建 active 租户与它的平台默认域名,成功后返回带
// DefaultDomain 的租户。
//
// 为什么必须原子:原实现是三步裸跑(CreateTenant → CreatePlatformDomain → 发信)。
// 中间任何一步失败,租户行都已落库 —— 用户拿到 5xx,重试注册又被「username 已被注册」/
// 「slug 已被占用」挡成 409,而默认域名并不存在,没有任何自助恢复入口,只能进数据库
// 手工修。改成单事务后,注册要么整体成功、要么什么都没发生(唯一索引的并发撞车也一并回滚)。
//
// 注册即 active:身份模型改成 username + 密码后不再有邮箱验证,所以这里直接激活 ——
// pending 只留给 bootstrap(见 CreateTenant 与 internal/bootstrap)。
//
// fqdn 由调用方给出(默认域名形态是产品决策,不属于数据层),本函数不做归一化,
// 与 CreatePlatformDomain 的既有行为保持一致。
func (s *Store) RegisterTenant(ctx context.Context, username, passwordHash, slug, fqdn string, isSuperAdmin bool) (*Tenant, error) {
	var tenantID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 用绑定到 tx 的 Store 复用既有实现,而不是把两张表的行形态各抄一遍:
		// CreateTenant / CreatePlatformDomain 是这些行的唯一权威定义(字段、默认值、
		// 将来的归一化),复制只会让两条路径悄悄漂移。
		ts := &Store{db: tx}
		t, err := ts.CreateTenant(ctx, username, passwordHash, slug, isSuperAdmin, false)
		if err != nil {
			return err
		}
		if err := ts.SetTenantStatus(ctx, t.ID, "active"); err != nil {
			return err
		}
		if _, err := ts.CreatePlatformDomain(ctx, t.ID, fqdn); err != nil {
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
