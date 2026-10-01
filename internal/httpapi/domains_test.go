package httpapi_test

// 04 — domain-management 黑盒测试:
// 添加自有域名(真实 DNS 校验,dev 下 hosts/localhost 指向 127.0.0.1 通过)→ active;
// 未生效进入重试队列 → 72h 超时 failed;手动重检;停用/恢复;删除(平台默认 400);
// 授权端点放行 active 自有域名、拒绝停用域名。

import (
	"net/http"
	"strconv"
	"strings"
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
	// 归属证明现在要求「权威 DNS 上发布一次性 TXT」,创建时还拿不到挑战 token,
	// 所以域名会以 pending 落库,需要一次重检才可能激活(见 handleCreateDomain
	// 的注释)。testutil 注入的假校验器只认 localhost 系列,于是:
	//   - localhost / *.localhost:重检 → verified → 激活。绝大多数用例要的是
	//     "一个可用的自有域名",这一步对它们是必要的。
	//   - 其他名字(如 .invalid):假校验器给 need_dns,保持 pending ——
	//     有些用例正是要断言"解析不到 → 进重试队列",不能在这里替它激活。
	if isLocalthostFQDN(fqdn) {
		activateDomain(t, c, d.ID)
		d = getDomain(t, c, d.ID)
	}
	return &d
}

func isLocalthostFQDN(fqdn string) bool {
	f := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(fqdn), "."))
	return f == "localhost" || strings.HasSuffix(f, ".localhost")
}

// activateDomain 重检并等到域名真正激活。重检是异步任务(TXT 查询 + A/AAAA),
// 这里轮询到终态,避免"提交了就当成功"导致后续用例偶发拿不到可用域名。
func activateDomain(t *testing.T, c *testClient, id int64) {
	t.Helper()
	resp := c.post("/api/domains/"+strconv.FormatInt(id, 10)+"/recheck", nil)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if getDomain(t, c, id).Status == "active" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("域名 %d 在 3s 内未激活(最后状态 %q)", id, getDomain(t, c, id).Status)
}

func getDomain(t *testing.T, c *testClient, id int64) store.Domain {
	t.Helper()
	resp := c.get("/api/domains/" + strconv.FormatInt(id, 10))
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[store.Domain](t, resp)
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

func TestCreateDomainDescription(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 携带描述创建 → 返回描述
	resp := c.post("/api/domains", map[string]string{"fqdn": "desc.example.com", "description": "生产环境主站,用于产品文档"})
	assertStatus(t, resp, http.StatusCreated)
	d := decodeBody[store.Domain](t, resp)
	if d.Description != "生产环境主站,用于产品文档" {
		t.Fatalf("description = %q, want 生产环境主站,用于产品文档", d.Description)
	}

	// 描述超长 → 400
	longDesc := strings.Repeat("长", 201)
	resp = c.post("/api/domains", map[string]string{"fqdn": "long.example.com", "description": longDesc})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 不传描述 → 空字符串
	resp = c.post("/api/domains", map[string]string{"fqdn": "nodesc.example.com"})
	assertStatus(t, resp, http.StatusCreated)
	d = decodeBody[store.Domain](t, resp)
	if d.Description != "" {
		t.Fatalf("description = %q, want empty", d.Description)
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

	// 超过最长重试时长(72h)→ worker 置终态 expired。
	//
	// 为什么是 expired 而不是 failed:failed 仍是重试队列的成员,超期后每轮
	// 都会被扫到并无条件写一次库(写放大且永远没有出口)。expired 让它离开
	// 扫描集合,租户想重来必须显式点「重新校验」。
	env.BackdateDomain(t, d.ID, 73*time.Hour)
	testutil.Poll(t, 5*time.Second, "domain expired", func() bool {
		resp := c.get("/api/domains/" + strconv.FormatInt(d.ID, 10))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		dd := decodeBody[store.Domain](t, resp)
		return dd.Status == "expired"
	})

	// 终态可被手动重检复活:重新签发挑战 token 并回到 pending
	resp = c.post("/api/domains/"+strconv.FormatInt(d.ID, 10)+"/recheck", nil)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()
	testutil.Poll(t, 5*time.Second, "domain revived", func() bool {
		resp := c.get("/api/domains/" + strconv.FormatInt(d.ID, 10))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		dd := decodeBody[store.Domain](t, resp)
		return dd.Status == "pending"
	})
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
