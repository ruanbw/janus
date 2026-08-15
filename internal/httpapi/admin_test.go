package httpapi_test

// 08 — superadmin 黑盒测试:环境变量初始化超管、首次登录设置密码、
// 查看全部租户、封禁/解封(授权端点联动)、调等级、平台强删违规域名。

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"cloak/internal/bootstrap"
	"cloak/internal/httpapi"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

// superadminClient 初始化超管并完成首次登录设置密码,返回超管客户端。
func superadminClient(t *testing.T, env *testutil.Env) *testClient {
	t.Helper()
	if err := bootstrap.Superadmin(context.Background(), env.Store, "admin@cloak.test"); err != nil {
		t.Fatalf("bootstrap superadmin: %v", err)
	}
	c := newClient(env)
	// 无密码首登:允许登录并带 firstLoginSetup 标记
	resp := c.post("/api/auth/login", map[string]string{"email": "admin@cloak.test", "password": "whatever"})
	assertStatus(t, resp, http.StatusOK)
	tenant := decodeBody[store.Tenant](t, resp)
	if !tenant.FirstLoginSetup {
		t.Fatal("superadmin first login should carry firstLoginSetup=true")
	}
	if !tenant.IsSuperAdmin {
		t.Fatal("superadmin tenant should be isSuperAdmin")
	}
	// 引导设置密码(无需旧密码)
	resp = c.post("/api/auth/change-password", map[string]string{"oldPassword": "", "newPassword": "adminpass123"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	return c
}

func TestSuperadminBootstrapAndLogin(t *testing.T) {
	env := testutil.Setup(t)
	_ = superadminClient(t, env)

	// 设置密码后再次登录:不再引导
	resp := newClient(env).post("/api/auth/login", map[string]string{"email": "admin@cloak.test", "password": "adminpass123"})
	assertStatus(t, resp, http.StatusOK)
	tenant := decodeBody[store.Tenant](t, resp)
	if tenant.FirstLoginSetup {
		t.Error("firstLoginSetup should be false after password set")
	}
	_ = resp.Body.Close()
}

func TestAdminTenantsAndForbidden(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	loggedInTenant(t, env, "alice")

	// 超管查看全部租户(含用量)
	resp := admin.get("/api/admin/tenants")
	assertStatus(t, resp, http.StatusOK)
	tenants := decodeBody[[]*store.Tenant](t, resp)
	if len(tenants) < 2 {
		t.Fatalf("tenants = %d, want >= 2", len(tenants))
	}
	for _, tt := range tenants {
		if tt.Usage == nil {
			t.Errorf("tenant %s missing usage", tt.Email)
		}
	}

	// 普通租户访问超管端点 → 403
	alice := loggedInTenant(t, env, "bob")
	resp = alice.get("/api/admin/tenants")
	assertStatus(t, resp, http.StatusForbidden)
	_ = resp.Body.Close()
}

func TestAdminBanAndUnban(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	c := loggedInTenant(t, env, "alice")

	// 封禁前授权放行
	resp := get(t, env, "/internal/caddy/authorize?domain=alice.cloak.test")
	assertStatus(t, resp, http.StatusOK)

	// 超管封禁
	aliceID := tenantIDOf(t, c)
	resp = admin.patch("/api/admin/tenants/"+strconv.FormatInt(aliceID, 10), map[string]any{"status": "banned"})
	assertStatus(t, resp, http.StatusOK)
	banned := decodeBody[store.Tenant](t, resp)
	if banned.Status != "banned" {
		t.Fatalf("status = %s, want banned", banned.Status)
	}

	// 封禁后:授权端点拒绝其默认域名;登录被拒;既有会话立即失效
	resp = get(t, env, "/internal/caddy/authorize?domain=alice.cloak.test")
	assertStatus(t, resp, http.StatusForbidden)
	resp = newClient(env).post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusForbidden)
	_ = resp.Body.Close()
	resp = c.get("/api/auth/me") // c 是封禁前登录的会话
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 解封恢复
	resp = admin.patch("/api/admin/tenants/"+strconv.FormatInt(aliceID, 10), map[string]any{"status": "active"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = get(t, env, "/internal/caddy/authorize?domain=alice.cloak.test")
	assertStatus(t, resp, http.StatusOK)
}

func TestAdminChangeTier(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	c := loggedInTenant(t, env, "alice")

	// 测试数据:新增 pro 等级
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`INSERT INTO tiers (name, max_links, max_domains) VALUES ('pro', 500, 50)`); err != nil {
		t.Fatalf("insert pro tier: %v", err)
	}
	var proID int64
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT id FROM tiers WHERE name='pro'`).Scan(&proID); err != nil {
		t.Fatal(err)
	}

	resp := admin.patch("/api/admin/tenants/"+strconv.FormatInt(tenantIDOf(t, c), 10), map[string]any{"tierId": proID})
	assertStatus(t, resp, http.StatusOK)
	upd := decodeBody[store.Tenant](t, resp)
	if upd.Tier.Name != "pro" || upd.Tier.MaxLinks != 500 {
		t.Fatalf("tier = %+v, want pro 500", upd.Tier)
	}

	// 无效等级
	resp = admin.patch("/api/admin/tenants/"+strconv.FormatInt(tenantIDOf(t, c), 10), map[string]any{"tierId": 99999})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAdminDeleteDomain(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	c := loggedInTenant(t, env, "alice")

	d := addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{d.ID}})

	// 超管强删违规域名(即使其上有未删除短链)
	resp := admin.del("/api/admin/domains/" + strconv.FormatInt(d.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 域名消失:跳转 404
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAdminCannotBanOrDemoteSelf(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	adminID := tenantIDOf(t, admin)
	path := "/api/admin/tenants/" + strconv.FormatInt(adminID, 10)

	// 封禁自己 → 400(不产生封禁,会话仍有效)
	resp := admin.patch(path, map[string]any{"status": "banned"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
	resp = admin.get("/api/auth/me")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 调整自己的等级 → 400
	resp = admin.patch(path, map[string]any{"tierId": 1})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAdminPatchStatusRejectsPending(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	c := loggedInTenant(t, env, "alice")

	resp := admin.patch("/api/admin/tenants/"+strconv.FormatInt(tenantIDOf(t, c), 10), map[string]any{"status": "pending"})
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_VALIDATION" {
		t.Fatalf("code = %s, want E_VALIDATION", body.Code)
	}
	_ = resp.Body.Close()
}

// TestVerifyEmailDoesNotUnban 已封禁租户的旧验证 token 不得将其复活(400,状态保持 banned)。
func TestVerifyEmailDoesNotUnban(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	register(t, env, "alice") // pending,邮件中已有 verify token
	token := env.LastToken(t)

	// 从超管列表取 alice 的 id(不登录 alice,避免干扰验证 token)
	tenants := decodeBody[[]*store.Tenant](t, admin.get("/api/admin/tenants"))
	var aliceID int64
	for _, tt := range tenants {
		if tt.Email == "alice@example.com" {
			aliceID = tt.ID
			break
		}
	}
	if aliceID == 0 {
		t.Fatal("alice tenant not found")
	}
	resp := admin.patch("/api/admin/tenants/"+strconv.FormatInt(aliceID, 10), map[string]any{"status": "banned"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 封禁后使用验证 token → 400,且不被置为 active
	resp = newClient(env).post("/api/auth/verify-email", map[string]string{"token": token})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	tenants = decodeBody[[]*store.Tenant](t, admin.get("/api/admin/tenants"))
	for _, tt := range tenants {
		if tt.ID == aliceID && tt.Status != "banned" {
			t.Fatalf("tenant status = %s, want banned (verify must not unban)", tt.Status)
		}
	}
}
