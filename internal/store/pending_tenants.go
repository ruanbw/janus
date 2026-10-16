package store

// 未验证 pending 租户的回收。
//
// 注册流程会先落租户行(含 slug)与平台默认域名 <slug>.<平台域名>,只有邮箱验证
// 通过才置 active。验证邮件可能永远不被点开(写错邮箱、机器人注册),而
// tenants.slug 与 domains.fqdn 都是 UNIQUE —— 这些行会永久占住 slug 与默认域名:
// 真正想用这个 slug 的注册者会一直撞 409,平台也一直为这些域名保留行
// (证书探活的扫描 SQL 已用 t.status='active' 挡掉出网请求,但占位本身没被回收)。
//
// 保留期取 7 天:留给"验证邮件躺在收件箱里、过几天才点"的正常人,超过一周仍未验证
// 即判定为终结的注册尝试。

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

// DeleteStalePendingTenants 回收"从未验证邮箱且已超过保留期"的 pending 租户,
// 返回删除的租户数。
//
// 删除是物理删除,靠外键级联清掉该租户的域名/会话/邮箱 token(见 init.sql 的
// ON DELETE CASCADE);slug 与默认域名随之释放。这些租户在定义上从未登录过
// (登录要求 active),因此不会有短链、落地页文件或访问记录 —— 不需要额外的清理步骤
// (也就绕开了 visits.domain_id 尚无 ON DELETE CASCADE 这个已知问题)。
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
					  AND verified_at IS NULL
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
