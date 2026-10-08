package bootstrap

// 单元测试(无需数据库):超管邮箱对应既有账号时的提权与封禁处理。

import (
	"context"
	"testing"

	"janus/internal/store"
)

type fakeStore struct {
	tenants  map[string]*store.Tenant
	promoted []int64
	statuses map[int64]string
	created  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{tenants: map[string]*store.Tenant{}, statuses: map[int64]string{}}
}

func (f *fakeStore) GetTenantByEmail(_ context.Context, email string) (*store.Tenant, error) {
	t, ok := f.tenants[email]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeStore) CreateTenant(_ context.Context, email, _, slug string, isSuperAdmin bool) (*store.Tenant, error) {
	f.created++
	t := &store.Tenant{ID: int64(100 + f.created), Email: email, Slug: slug, IsSuperAdmin: isSuperAdmin, Status: "pending"}
	f.tenants[email] = t
	return t, nil
}

func (f *fakeStore) SetTenantStatus(_ context.Context, id int64, status string) error {
	f.statuses[id] = status
	for _, t := range f.tenants {
		if t.ID == id {
			t.Status = status
		}
	}
	return nil
}

func (f *fakeStore) PromoteToSuperAdminResetCredentials(_ context.Context, id int64) error {
	f.promoted = append(f.promoted, id)
	for _, t := range f.tenants {
		if t.ID == id {
			t.IsSuperAdmin = true
			t.PasswordHash = nil
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }

// TestSuperadminPromotesExistingAccountAndResetsCredentials 回归:抢先用超管邮箱
// 注册的普通账号被提权时,必须走「作废凭据」的原子提权(不得保留原密码)。
func TestSuperadminPromotesExistingAccountAndResetsCredentials(t *testing.T) {
	st := newFakeStore()
	st.tenants["admin@janus.test"] = &store.Tenant{ID: 7, Email: "admin@janus.test", Status: "pending", PasswordHash: strPtr("$2a$attacker")}

	if err := superadmin(context.Background(), st, " Admin@Janus.test "); err != nil {
		t.Fatalf("superadmin: %v", err)
	}
	if len(st.promoted) != 1 || st.promoted[0] != 7 {
		t.Fatalf("promoted = %v, want [7] (credentials must be reset on promotion)", st.promoted)
	}
	got := st.tenants["admin@janus.test"]
	if !got.IsSuperAdmin || got.PasswordHash != nil {
		t.Fatalf("tenant after promotion: superadmin=%v passwordHash=%v", got.IsSuperAdmin, got.PasswordHash)
	}
	if got.Status != "active" {
		t.Fatalf("pending tenant should converge to active, got %q", got.Status)
	}
	if st.created != 0 {
		t.Fatal("must not create a new tenant when one exists")
	}
}

// TestSuperadminDoesNotReactivateBanned 封禁账号:可以提权(凭据照样作废),但绝不解封。
func TestSuperadminDoesNotReactivateBanned(t *testing.T) {
	for _, already := range []bool{false, true} {
		st := newFakeStore()
		st.tenants["admin@janus.test"] = &store.Tenant{ID: 9, Email: "admin@janus.test", Status: "banned", IsSuperAdmin: already}
		if err := superadmin(context.Background(), st, "admin@janus.test"); err != nil {
			t.Fatalf("superadmin: %v", err)
		}
		if s, ok := st.statuses[9]; ok {
			t.Fatalf("alreadySuperadmin=%v: status must not be touched for banned tenant, got set to %q", already, s)
		}
		if st.tenants["admin@janus.test"].Status != "banned" {
			t.Fatal("banned tenant must stay banned")
		}
	}
}

// TestSuperadminExistingSuperadminUntouched 已是超管的账号不再重置凭据(否则每次重启
// 都会把运维设好的密码清掉),pending 仍收敛为 active(崩溃恢复路径)。
func TestSuperadminExistingSuperadminUntouched(t *testing.T) {
	st := newFakeStore()
	st.tenants["admin@janus.test"] = &store.Tenant{ID: 3, Email: "admin@janus.test", Status: "pending", IsSuperAdmin: true, PasswordHash: strPtr("$2a$ops")}
	if err := superadmin(context.Background(), st, "admin@janus.test"); err != nil {
		t.Fatalf("superadmin: %v", err)
	}
	if len(st.promoted) != 0 {
		t.Fatalf("existing superadmin must not be re-promoted/reset, promoted=%v", st.promoted)
	}
	if st.statuses[3] != "active" {
		t.Fatalf("pending superadmin should converge to active, got %q", st.statuses[3])
	}
}

// TestSuperadminCreatesWhenMissing 不存在时创建超管并置 active,不走提权路径。
func TestSuperadminCreatesWhenMissing(t *testing.T) {
	st := newFakeStore()
	if err := superadmin(context.Background(), st, "admin@janus.test"); err != nil {
		t.Fatalf("superadmin: %v", err)
	}
	got := st.tenants["admin@janus.test"]
	if got == nil || !got.IsSuperAdmin || got.Status != "active" {
		t.Fatalf("created tenant = %+v", got)
	}
	if len(st.promoted) != 0 {
		t.Fatal("fresh superadmin must not go through promotion")
	}
}
