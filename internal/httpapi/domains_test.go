package httpapi_test

// 04 — domain-management 黑盒测试:
// 添加自有域名(真实 DNS 校验,dev 下 hosts/localhost 指向 127.0.0.1 通过)→ active;
// 未生效进入重试队列 → 72h 超时 failed;手动重检;停用/恢复;删除(平台默认 400);
// 授权端点放行 active 自有域名、拒绝停用域名。

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"cloak/internal/httpapi"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

// loggedInTenant 注册+验证+登录,返回带会话的客户端。
func loggedInTenant(t *testing.T, env *testutil.Env, slug string) *testClient {
	t.Helper()
	c := register(t, env, slug)
	verifyLastEmail(t, env)
	resp := c.post("/api/auth/login", map[string]string{"email": slug + "@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	return c
}

func addDomain(t *testing.T, c *testClient, fqdn string) *store.Domain {
	t.Helper()
	resp := c.post("/api/domains", map[string]string{"fqdn": fqdn})
	assertStatus(t, resp, http.StatusCreated)
	d := decodeBody[store.Domain](t, resp)
	return &d
}

func TestCreateDomainDNSActive(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// localhost 解析到 127.0.0.1,CLOAK_SERVER_PUBLIC_IP=127.0.0.1 → 真实 DNS 校验通过
	d := addDomain(t, c, "localhost")
	if d.Status != "active" {
		t.Fatalf("domain status = %s, want active", d.Status)
	}
	if d.Origin != "self" {
		t.Errorf("origin = %s, want self", d.Origin)
	}
	// 证书探活为异步且测试环境无 Caddy:状态为 pending(未探活)或 failed(探活失败)均可
	if d.CertStatus != "pending" && d.CertStatus != "failed" {
		t.Errorf("certStatus = %s, want pending/failed", d.CertStatus)
	}
}

func TestCreateDomainValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	cases := []struct {
		name string
		fqdn string
		want int
	}{
		{"empty", "", http.StatusBadRequest},
		{"underscore", "bad_domain.com", http.StatusBadRequest},
		{"leading dash label", "-x.com", http.StatusBadRequest},
		{"platform root", "cloak.test", http.StatusBadRequest},
		{"platform app", "app.cloak.test", http.StatusBadRequest},
		{"scheme", "https://example.com", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.post("/api/domains", map[string]string{"fqdn": tc.fqdn})
			assertStatus(t, resp, tc.want)
			_ = resp.Body.Close()
		})
	}
}

func TestCreateDomainConflicts(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")

	// 同租户重复
	resp := c.post("/api/domains", map[string]string{"fqdn": "localhost"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()

	// 其他租户占用
	c2 := loggedInTenant(t, env, "bob")
	resp = c2.post("/api/domains", map[string]string{"fqdn": "localhost"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

func TestCreateDomainPendingAndTimeoutFailed(t *testing.T) {
	env := testutil.Setup(t)
	env.StartWorker(t)
	c := loggedInTenant(t, env, "alice")

	// .invalid 保留 TLD 保证不解析 → 校验失败进入重试队列(pending)
	d := addDomain(t, c, "nonexistent.invalid")
	if d.Status != "pending" {
		t.Fatalf("domain status = %s, want pending (retry queue)", d.Status)
	}

	// 手动重检:202,状态仍 pending
	resp := c.post("/api/domains/"+strconv.FormatInt(d.ID, 10)+"/recheck", nil)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()
	resp = c.get("/api/domains/" + strconv.FormatInt(d.ID, 10))
	assertStatus(t, resp, http.StatusOK)
	d2 := decodeBody[store.Domain](t, resp)
	if d2.Status != "pending" {
		t.Fatalf("after recheck status = %s, want pending", d2.Status)
	}

	// 超过最长重试时长(72h)→ worker 置 failed
	env.BackdateDomain(t, d.ID, 73*time.Hour)
	testutil.Poll(t, 5*time.Second, "domain failed", func() bool {
		resp := c.get("/api/domains/" + strconv.FormatInt(d.ID, 10))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		dd := decodeBody[store.Domain](t, resp)
		return dd.Status == "failed"
	})

	// failed 后可手动重检(仍失败保持 failed)
	resp = c.post("/api/domains/"+strconv.FormatInt(d.ID, 10)+"/recheck", nil)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()
}

func TestDomainStopAndRestore(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "localhost")

	// 停用
	resp := c.patch("/api/domains/"+strconv.FormatInt(d.ID, 10), map[string]string{"status": "stopped"})
	assertStatus(t, resp, http.StatusOK)
	d2 := decodeBody[store.Domain](t, resp)
	if d2.Status != "stopped" {
		t.Fatalf("status = %s, want stopped", d2.Status)
	}

	// 恢复(要求 DNS 仍指向本机:localhost 通过)
	resp = c.patch("/api/domains/"+strconv.FormatInt(d.ID, 10), map[string]string{"status": "active"})
	assertStatus(t, resp, http.StatusOK)
	d3 := decodeBody[store.Domain](t, resp)
	if d3.Status != "active" {
		t.Fatalf("status = %s, want active", d3.Status)
	}

	// 非法状态
	resp = c.patch("/api/domains/"+strconv.FormatInt(d.ID, 10), map[string]string{"status": "bogus"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestDeleteDomain(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "localhost")

	// 平台默认域名不可删除
	domains := listDomains(t, c)
	var platformID int64
	for _, dd := range domains {
		if dd.Origin == "platform" {
			platformID = dd.ID
		}
	}
	resp := c.del("/api/domains/" + strconv.FormatInt(platformID, 10))
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 删除自有域名 → 204,再查 404
	resp = c.del("/api/domains/" + strconv.FormatInt(d.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.get("/api/domains/" + strconv.FormatInt(d.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func listDomains(t *testing.T, c *testClient) []*store.Domain {
	t.Helper()
	resp := c.get("/api/domains")
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[[]*store.Domain](t, resp)
}

func TestAuthorizeTenantDomain(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "localhost")

	// active 自有域名放行
	resp := get(t, env, "/internal/caddy/authorize?domain=localhost")
	assertStatus(t, resp, http.StatusOK)

	// 租户默认域名(已验证)→ 放行
	resp = get(t, env, "/internal/caddy/authorize?domain=alice.cloak.test")
	assertStatus(t, resp, http.StatusOK)

	// 停用后拒绝
	resp = c.patch("/api/domains/"+strconv.FormatInt(d.ID, 10), map[string]string{"status": "stopped"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = get(t, env, "/internal/caddy/authorize?domain=localhost")
	assertStatus(t, resp, http.StatusForbidden)
}

func TestDomainQuota(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 配额超限:自有域名按"未删除"计数;平台默认域名不计
	env.SetTierLimits(t, tenantIDOf(t, c), 100, 2)
	addDomain(t, c, "localhost")
	addDomain(t, c, "second.localhost") // 无法解析 → pending,仍计入配额
	resp := c.post("/api/domains", map[string]string{"fqdn": "third.localhost"})
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_DOMAIN_LIMIT" {
		t.Errorf("code = %s, want E_DOMAIN_LIMIT", body.Code)
	}
}

// tenantIDOf 从 /api/auth/me 取租户 ID。
func tenantIDOf(t *testing.T, c *testClient) int64 {
	t.Helper()
	resp := c.get("/api/auth/me")
	assertStatus(t, resp, http.StatusOK)
	me := decodeBody[store.Tenant](t, resp)
	return me.ID
}
