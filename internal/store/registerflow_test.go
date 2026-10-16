package store

// 注册流程的原子性测试:直接打真实 Postgres(不 mock store、不经过 httpapi)。
//
// 为什么必须在这一层:要验证的是"中间一步失败时前面几步是否回滚",这是事务语义,
// 黑盒 HTTP 只能间接观察;而唯一冲突恰恰是注册流程最容易踩的失败。

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRegisterTenantRollsBackOnDomainConflict 默认域名撞唯一索引时,事务内已插入
// 的租户行(以及验证 token)必须一并回滚,不能留"有租户、没默认域名、没 token"的
// 半成品 —— 那种状态下用户重试注册只会拿到 409,无法自助恢复。
func TestRegisterTenantRollsBackOnDomainConflict(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// 另一路注册已占位该默认域名(等价于预检通过、插入前被抢先)。
	holder, err := s.CreateTenant(ctx, "holder@example.com", "hash", "holder", false)
	if err != nil {
		t.Fatalf("create holder: %v", err)
	}
	if _, err := s.CreatePlatformDomain(ctx, holder.ID, "race.janus.test"); err != nil {
		t.Fatalf("create holder domain: %v", err)
	}

	if _, err := s.RegisterTenant(ctx, "new@example.com", "hash", "race",
		"race.janus.test", "token-hash", time.Hour, false); err == nil {
		t.Fatal("RegisterTenant 应因默认域名唯一冲突失败")
	} else if !IsUniqueViolation(err) {
		t.Fatalf("err = %v, want unique violation", err)
	}

	if _, err := s.GetTenantByEmail(ctx, "new@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("残留租户(邮箱): err = %v", err)
	}
	if _, err := s.GetTenantBySlug(ctx, "race"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("残留租户(slug 占位): err = %v", err)
	}
	if n := countEmailTokens(t, s, "token-hash"); n != 0 {
		t.Fatalf("残留验证 token %d 条, want 0", n)
	}
	// 抢占者的域名必须原样保留。
	if d, err := s.GetDomainByFQDN(ctx, "race.janus.test"); err != nil || d.TenantID != holder.ID {
		t.Fatalf("抢占者的域名被破坏: d=%+v err=%v", d, err)
	}
}

// TestRegisterTenantSuccessCreatesAll 成功路径:租户(pending)、平台默认域名(active)、
// 验证 token 一次事务落库,返回的租户带默认域名(与原先四步裸跑的对外行为一致)。
func TestRegisterTenantSuccessCreatesAll(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	tn, err := s.RegisterTenant(ctx, "new@example.com", "hash", "new",
		"new.janus.test", "tok-ok", time.Hour, false)
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	if tn.Status != "pending" {
		t.Errorf("status = %s, want pending", tn.Status)
	}
	if tn.DefaultDomain != "new.janus.test" {
		t.Errorf("defaultDomain = %s, want new.janus.test", tn.DefaultDomain)
	}
	if n := countEmailTokens(t, s, "tok-ok"); n != 1 {
		t.Errorf("验证 token 条数 = %d, want 1", n)
	}
}

// countEmailTokens 按 token_hash 统计 email_tokens 行数(测试在包内,可直接用 s.db)。
func countEmailTokens(t *testing.T, s *Store, tokenHash string) int64 {
	t.Helper()
	var n int64
	if err := s.db.WithContext(context.Background()).
		Model(&EmailToken{}).Where("token_hash = ?", tokenHash).Count(&n).Error; err != nil {
		t.Fatalf("count email tokens: %v", err)
	}
	return n
}
