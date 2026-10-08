package httpapi_test

// 域名抢注防护 / 复检容错 / expired 回收 / 保留 slug 的黑盒测试(需要真实 Postgres)。
//
// testutil 的假归属校验器:localhost 系列在持有 token 时判 verified,其余名字 need_dns。
// 所以"认领方能发布 TXT"用 *.localhost 模拟,"认领方还没发布"用 .invalid 模拟。

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"janus/internal/domain"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
)

// 他人占位(pending、从未证明归属)的域名,能证明归属的租户可直接接管:
// 旧行被驱逐,认领方得到一条已激活的记录。
func TestClaimSquattedDomainWithOwnershipProof(t *testing.T) {
	env := testutil.Setup(t)
	squatter := loggedInTenant(t, env, "alice")
	owner := loggedInTenant(t, env, "bob")

	// 占位:只创建不重检,停在 pending。
	resp := squatter.post("/api/domains", map[string]string{"fqdn": "victim.localhost"})
	assertStatus(t, resp, http.StatusCreated)
	squat := decodeBody[store.Domain](t, resp)
	if squat.Status != "pending" {
		t.Fatalf("squat status = %s, want pending", squat.Status)
	}

	resp = owner.post("/api/domains", map[string]string{"fqdn": "victim.localhost"})
	assertStatus(t, resp, http.StatusCreated)
	claimed := decodeBody[store.Domain](t, resp)
	if claimed.Status != "active" || claimed.OwnershipVerifiedAt == nil {
		t.Fatalf("claimed = %+v, want active with ownershipVerifiedAt", claimed)
	}

	// 占位者那一行已经不存在了。
	resp = squatter.get("/api/domains/" + strconv.FormatInt(squat.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()

	// 授权端点放行的是认领方的 active 记录。
	resp = get(t, env, "/internal/caddy/authorize?domain=victim.localhost")
	assertStatus(t, resp, http.StatusOK)
}

// 认领方尚未发布 TXT:409 且 details 带 claimable 与该租户专属的 TXT 指引;
// 同一租户再次提交拿到的是同一枚 token(未过期前保持稳定)。
func TestClaimSquattedDomainNeedsTXT(t *testing.T) {
	env := testutil.Setup(t)
	squatter := loggedInTenant(t, env, "alice")
	owner := loggedInTenant(t, env, "bob")

	resp := squatter.post("/api/domains", map[string]string{"fqdn": "victim.invalid"})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	claimValue := func() string {
		resp := owner.post("/api/domains", map[string]string{"fqdn": "victim.invalid"})
		assertStatus(t, resp, http.StatusConflict)
		body := decodeBody[httpapi.ErrorBody](t, resp)
		details, ok := body.Details.(map[string]any)
		if !ok || details["claimable"] != true {
			t.Fatalf("details = %#v, want claimable=true", body.Details)
		}
		if details["verifyRecord"] != domain.VerifyRecordName("victim.invalid") {
			t.Fatalf("verifyRecord = %v", details["verifyRecord"])
		}
		v, _ := details["verifyValue"].(string)
		if v == "" {
			t.Fatal("verifyValue 为空")
		}
		return v
	}
	first := claimValue()
	if second := claimValue(); second != first {
		t.Fatalf("认领 token 未保持稳定: %s → %s", first, second)
	}
}

// 已证明归属 / 正在服务的域名不可认领:普通 409,没有 claimable。
func TestClaimRejectedForActiveDomain(t *testing.T) {
	env := testutil.Setup(t)
	holder := loggedInTenant(t, env, "alice")
	other := loggedInTenant(t, env, "bob")
	addDomain(t, holder, "held.localhost") // localhost 系列会被激活

	resp := other.post("/api/domains", map[string]string{"fqdn": "held.localhost"})
	assertStatus(t, resp, http.StatusConflict)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if m, ok := body.Details.(map[string]any); ok && m["claimable"] == true {
		t.Fatalf("active 域名不应可认领: %#v", body.Details)
	}
}

// 连续 N 次复检失败才降级;降级时 verify_token_created_at 被重置,
// 下一轮重试队列不会因 created_at 很老而立刻判 expired。
func TestOwnershipRecheckFailureThreshold(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "mature.localhost")
	// 模拟成熟域名:创建于一年前。
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE domains SET created_at = now() - interval '8760 hours' WHERE id = $1`, d.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	ctx := context.Background()
	for i := 1; i < domain.OwnershipRecheckFailThreshold; i++ {
		n, degraded, err := env.Store.RecordOwnershipRecheckFailure(ctx, d.ID, domain.OwnershipRecheckFailThreshold)
		if err != nil || degraded || n != i {
			t.Fatalf("第 %d 次失败: n=%d degraded=%v err=%v, want n=%d 未降级", i, n, degraded, err, i)
		}
		if got := getDomain(t, c, d.ID); got.Status != "active" {
			t.Fatalf("第 %d 次失败后 status = %s, want active", i, got.Status)
		}
	}
	_, degraded, err := env.Store.RecordOwnershipRecheckFailure(ctx, d.ID, domain.OwnershipRecheckFailThreshold)
	if err != nil || !degraded {
		t.Fatalf("达到阈值: degraded=%v err=%v, want degraded", degraded, err)
	}
	got := getDomain(t, c, d.ID)
	if got.Status != "failed" || got.RecheckFailures != 0 {
		t.Fatalf("降级后 = status %s failures %d, want failed/0", got.Status, got.RecheckFailures)
	}
	if got.VerifyTokenCreatedAt == nil || time.Since(*got.VerifyTokenCreatedAt) > time.Minute {
		t.Fatalf("降级后 verifyTokenCreatedAt = %v, want ≈now", got.VerifyTokenCreatedAt)
	}
	rows, err := env.Store.ListDomainsForDNSCheck(ctx, env.Cfg.DNSMaxAge, 0, store.DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.ID == d.ID && r.Overdue {
			t.Fatal("刚降级的成熟域名被判 overdue(会被立即打成 expired)")
		}
	}
}

// 成功复检清零失败计数。
func TestOwnershipRecheckSuccessResetsFailures(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "flaky.localhost")
	ctx := context.Background()
	if _, _, err := env.Store.RecordOwnershipRecheckFailure(ctx, d.ID, domain.OwnershipRecheckFailThreshold); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := env.Store.ActivateDomainVerified(ctx, d.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if got := getDomain(t, c, d.ID); got.RecheckFailures != 0 {
		t.Fatalf("recheckFailures = %d, want 0", got.RecheckFailures)
	}
}

// 长期 expired、从未承载流量的占位行被回收;新近 expired 的保留。
func TestGCExpiredDomains(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	resp := c.post("/api/domains", map[string]string{"fqdn": "old.invalid"})
	assertStatus(t, resp, http.StatusCreated)
	old := decodeBody[store.Domain](t, resp)
	resp = c.post("/api/domains", map[string]string{"fqdn": "recent.invalid"})
	assertStatus(t, resp, http.StatusCreated)
	recent := decodeBody[store.Domain](t, resp)

	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE domains SET status = 'expired', dns_checked_at = now() - interval '800 hours' WHERE id = $1`, old.ID); err != nil {
		t.Fatalf("expire old: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE domains SET status = 'expired', dns_checked_at = now() WHERE id = $1`, recent.ID); err != nil {
		t.Fatalf("expire recent: %v", err)
	}
	n, err := env.Store.GCExpiredDomains(context.Background(), store.ExpiredDomainRetention, store.DefaultDomainScanLimit)
	if err != nil || n != 1 {
		t.Fatalf("GC: n=%d err=%v, want 1", n, err)
	}
	resp = c.get("/api/domains/" + strconv.FormatInt(old.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	if got := getDomain(t, c, recent.ID); got.Status != "expired" {
		t.Fatalf("recent status = %s, want expired(保留)", got.Status)
	}
}

// 保留 slug 不可注册(之前只挡了 app)。
func TestRegisterReservedSlugs(t *testing.T) {
	env := testutil.Setup(t)
	for _, slug := range []string{"www", "mail", "api", "admin", "status"} {
		c := newClient(env)
		resp := c.post("/api/auth/register", map[string]string{
			"email": slug + "@example.com", "password": "password123", "slug": slug,
		})
		assertStatus(t, resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}
}
