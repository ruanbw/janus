package store

// store.go 平台管理侧写操作的语义测试:连真实 Postgres,验证"改不存在的行"必须报错。

import (
	"context"
	"errors"
	"testing"
)

// TestUpdateTenantAdminMissingTenantReturnsNotFound 固化平台管理端写操作的 RowsAffected 契约。
//
// 契约:对不存在的租户执行状态/等级更新,必须返回 ErrNotFound,而不是静默返回成功。
//
// 为什么必须报错:UPDATE ... WHERE id=? 命中 0 行时 GORM 不报错,调用方看到的是 200
// "封禁成功",而目标租户根本没被改(打错 id、租户已被删都会这样)。更糟的是被复用到
// worker 路径时,静默失败没有任何告警,只表现为"状态莫名其妙没生效"。
func TestUpdateTenantAdminMissingTenantReturnsNotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	status := "banned"
	tierID := int64(1)
	for name, tc := range map[string]struct {
		status *string
		tierID *int64
	}{
		"仅状态": {status: &status},
		"仅等级": {tierID: &tierID},
		"两者":  {status: &status, tierID: &tierID},
	} {
		t.Run(name, func(t *testing.T) {
			// Given 库里不存在该租户
			// When 平台管理端更新它
			err := s.UpdateTenantAdmin(ctx, 9_999_999, tc.status, tc.tierID)

			// Then 必须返回 ErrNotFound
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("改不存在的租户返回 err=%v, want ErrNotFound(静默成功等于谎报成功)", err)
			}
		})
	}
}

// TestUpdateTenantAdminExistingTenantSucceeds 是上面那条契约的正向对照:
// 租户存在时更新必须照常成功,不能被"加检查"误伤成误报 not found。
func TestUpdateTenantAdminExistingTenantSucceeds(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 一个存在的租户
	tenantID := newTenant(t, s, "admin-update@test.io")

	// When 封禁它(封禁必须同时自增 token_version 吊销在手 token)
	status := "banned"
	if err := s.UpdateTenantAdmin(ctx, tenantID, &status, nil); err != nil {
		t.Fatalf("更新存在的租户失败: %v", err)
	}

	// Then 状态已生效且 token_version 被自增
	got, err := s.GetTenantByID(ctx, tenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Status != "banned" {
		t.Fatalf("status = %q, want banned", got.Status)
	}
	if got.TokenVersion != 2 {
		t.Fatalf("token_version = %d, want 2(封禁必须吊销在手 token)", got.TokenVersion)
	}
}
