package store

// 未激活 pending 租户的回收。
//
// 注册流程现在直接建 active 租户(身份模型已改为 username + 密码,不再有邮箱验证),
// 所以 pending 只剩 bootstrap 在「创建超管」与「置 active」之间的一个瞬时态。这个回收
// 保留的意义是防御性的一条底线:slug 与 domains.fqdn 都是 UNIQUE,任何原因卡在 pending
// 的租户会永久占住一个 slug 与默认域名,而真正想用这个名字的注册者会一直撞 409。
//
// 保留期取 7 天:留给正常的部署重试窗口,超过一周仍未激活即判定为终结的注册尝试。

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	// StalePendingTenantRetention 未验证 pending 租户的保留期,超过即回收。
	StalePendingTenantRetention = 7 * 24 * time.Hour

	// pendingTenantDeleteBatch 单批删除的租户行数上限。
	//
	// 分批的理由与 visits 清理相同:一条 DELETE 会把全部命中行锁到提交,批量回收时
	// 是长事务 + WAL 暴涨(租户行还会级联删掉它的域名)。每批独立提交,批间让出。
	pendingTenantDeleteBatch = 500
)

// DeleteStalePendingTenants 回收「长期停在 pending 且从未激活」的租户,返回删除的租户数。
//
// 删除是物理删除,靠外键级联清掉该租户的域名/会话(见 init.sql 的 ON DELETE CASCADE);
// slug 与默认域名随之释放。这些租户在定义上从未登录过(登录要求非 pending),因此不会
// 有短链、落地页文件或访问记录 —— 不需要额外的清理步骤(也就绕开了 visits.domain_id
// 尚无 ON DELETE CASCADE 这个已知问题)。
//
// 不删的东西:超管租户(无论状态:删掉它等于平台失去管理入口)与已封禁租户
// (banned 是管理动作的结果,不是待清理的垃圾)。
func (s *Store) DeleteStalePendingTenants(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, fmt.Errorf("store: pending 租户保留期必须为正,当前为 %v", olderThan)
	}
	var total int64
	for {
		var n int64
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Exec(`
				DELETE FROM tenants
				WHERE id IN (
					SELECT id FROM tenants
					WHERE status = 'pending'
					  AND is_super_admin = false
					  AND created_at < now() - ?::interval
					ORDER BY id
					LIMIT ?
				)`, olderThan.String(), pendingTenantDeleteBatch)
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
		if n < pendingTenantDeleteBatch {
			return total, nil
		}
		// 批间让出:给注册等热路径留出窗口,也让 ctx 取消能及时生效。
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
	}
}
