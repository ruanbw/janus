package store

// 未验证 pending 租户回收的测试(直接打真实 Postgres:判据与级联删除都是 SQL 行为,
// mock 掉只会把 SQL 换成一句谎话)。

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// backdateTenant 回溯租户创建时间(测试里等不了 7 天)。
func backdateTenant(t *testing.T, s *Store, tenantID int64, sql string, args ...any) {
	t.Helper()
	if res := s.db.WithContext(context.Background()).Exec(sql, args...); res.Error != nil {
		t.Fatalf("backdate tenant %d: %v", tenantID, res.Error)
	}
}

// 超期未验证的 pending 租户被回收(连同它的默认域名,slug 与 fqdn 都被释放);
// 未超期的与已验证保留。
func TestDeleteStalePendingTenants(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 一个 8 天前注册、从未验证的 pending 租户(带平台默认域名)
	staleID := newTenant(t, s, "stale-pending@test.io")
	if _, err := s.CreatePlatformDomain(ctx, staleID, "stale-pending.janus.test"); err != nil {
		t.Fatalf("create platform domain: %v", err)
	}
	backdateTenant(t, s, staleID,
		`UPDATE tenants SET created_at = now() - interval '8 days' WHERE id = $1`, staleID)
	// 以及一个 1 天前的 pending、一个已验证 active 的租户(对照组)
	freshID := newTenant(t, s, "fresh-pending@test.io")
	activeID := newTenant(t, s, "active-tenant@test.io")
	if err := s.VerifyTenant(ctx, activeID); err != nil {
		t.Fatalf("verify tenant: %v", err)
	}
	backdateTenant(t, s, activeID,
		`UPDATE tenants SET created_at = now() - interval '8 days' WHERE id = $1`, activeID)
	// 超管即便处于 pending 也不得被回收
	adminID := newTenant(t, s, "admin@test.io")
	backdateTenant(t, s, adminID,
		`UPDATE tenants SET created_at = now() - interval '8 days', is_super_admin = true WHERE id = $1`, adminID)

	// When 回收超过保留期(7 天)的未验证 pending 租户
	n, err := s.DeleteStalePendingTenants(ctx, StalePendingTenantRetention)
	if err != nil {
		t.Fatalf("DeleteStalePendingTenants: %v", err)
	}

	// Then 只有那条 8 天前的 pending 被删
	if n != 1 {
		t.Fatalf("删除 %d 个租户, want 1", n)
	}
	if _, err := s.GetTenantByID(ctx, staleID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("超期未验证租户仍存在(err=%v)", err)
	}
	// 默认域名随租户级联删除,slug 与 fqdn 都被释放
	var domains int64
	if err := s.db.WithContext(ctx).Model(&Domain{}).Where("tenant_id = ?", staleID).Count(&domains).Error; err != nil {
		t.Fatalf("count domains: %v", err)
	}
	if domains != 0 {
		t.Fatalf("租户的默认域名未随之删除,还剩 %d 条:fqdn 仍被占着", domains)
	}
	if _, err := s.GetTenantByID(ctx, freshID); err != nil {
		t.Fatalf("未超期的 pending 租户被误删: %v", err)
	}
	if _, err := s.GetTenantByID(ctx, activeID); err != nil {
		t.Fatalf("已验证租户被误删: %v", err)
	}
	if _, err := s.GetTenantByID(ctx, adminID); err != nil {
		t.Fatalf("超管租户被误删: %v", err)
	}
}

// 保留期必须为正:传 0 会把"刚刚注册、还没来得及点验证邮件"的租户一起删掉。
func TestDeleteStalePendingTenantsRejectsNonPositiveRetention(t *testing.T) {
	s := setupStore(t)
	id := newTenant(t, s, "nonpositive@test.io")
	if _, err := s.DeleteStalePendingTenants(context.Background(), 0); err == nil {
		t.Fatal("保留期 <= 0 未被拒绝:会把刚注册的待验证租户直接删掉")
	}
	if _, err := s.GetTenantByID(context.Background(), id); err != nil {
		t.Fatalf("租户被误删: %v", err)
	}
}

// 超过单批上限时必须分批删完(不能只删第一批就返回)。
func TestDeleteStalePendingTenantsDeletesInBatches(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 比单批上限多一条的超期未验证租户
	want := pendingTenantDeleteBatch + 1
	for i := 0; i < want; i++ {
		newTenant(t, s, fmt.Sprintf("batch-%d@test.io", i))
	}
	if err := s.db.WithContext(ctx).Exec(
		`UPDATE tenants SET created_at = now() - interval '8 days' WHERE email LIKE 'batch-%@test.io'`).Error; err != nil {
		t.Fatalf("backdate tenants: %v", err)
	}

	// When
	n, err := s.DeleteStalePendingTenants(ctx, StalePendingTenantRetention)
	if err != nil {
		t.Fatalf("DeleteStalePendingTenants: %v", err)
	}

	// Then 全部删完,一个不留
	if n != int64(want) {
		t.Fatalf("删除 %d 个租户, want %d(分批循环未把剩余批次删完)", n, want)
	}
	var left int64
	if err := s.db.WithContext(ctx).Model(&Tenant{}).
		Where("email LIKE ?", "batch-%@test.io").Count(&left).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if left != 0 {
		t.Fatalf("还剩 %d 个超期未验证租户未回收", left)
	}
}
