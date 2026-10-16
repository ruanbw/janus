package domain

// pending 租户回收 pass 的接线测试(删除判据与分批行为在 store 层另有测试,
// 这里只钉"worker 真的把它接上了")。

import (
	"context"
	"errors"
	"testing"

	"janus/internal/store"
)

// 一轮清理必须删掉超期未验证的 pending 租户,并保留已验证(active)的租户。
func TestPendingTenantCleanupPassReclaimsStaleTenants(t *testing.T) {
	st, pool := setupDomainStore(t)
	ctx := context.Background()

	// Given 一个 8 天前注册、从未验证邮箱的 pending 租户
	stale, err := st.CreateTenant(ctx, "stale-pending-worker@test.io", "hash", "stale-pending-worker", false)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	execSQL(t, pool,
		`UPDATE tenants SET created_at = now() - interval '8 days' WHERE id = $1`, stale.ID)
	// 以及一个已验证的 active 租户(同样是 8 天前注册 —— 判据必须是"未验证",不是年龄)
	verified := newActiveTenant(t, st, "verified-worker@test.io", "verified-worker")
	execSQL(t, pool,
		`UPDATE tenants SET created_at = now() - interval '8 days' WHERE id = $1`, verified)

	// When 跑一轮清理
	workerWith(st, dnsErrorChecker()).pendingTenantCleanupPass(ctx)

	// Then 超期未验证的被回收,已验证的留下
	if _, err := st.GetTenantByID(ctx, stale.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("超期未验证的 pending 租户未被回收(err=%v)", err)
	}
	if _, err := st.GetTenantByID(ctx, verified); err != nil {
		t.Fatalf("已验证租户被误删: %v", err)
	}
}
