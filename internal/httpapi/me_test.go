package httpapi_test

// 06 — quota-tiers 黑盒测试:配额用量查询(/api/me)、自动短码长度设置、
// 平台默认域名不计入域名配额、新租户默认免费档。

import (
	"net/http"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
)

func TestMeUsageAndCodeLength(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 初始用量:短链 0、自有域名 0(平台默认域名不计)、上限来自免费档
	resp := c.get("/api/me")
	assertStatus(t, resp, http.StatusOK)
	me := decodeBody[store.Tenant](t, resp)
	if me.Usage == nil {
		t.Fatal("usage missing")
	}
	if me.Usage.Links != 0 || me.Usage.Domains != 0 {
		t.Fatalf("usage = %+v, want 0/0", me.Usage)
	}
	if me.Usage.MaxLinks != 100 || me.Usage.MaxDomains != 10 {
		t.Fatalf("usage limits = %d/%d, want 100/10", me.Usage.MaxLinks, me.Usage.MaxDomains)
	}

	// 添加自有域名 + 短链后用量增长
	addDomain(t, c, "localhost")
	ids := domainIDsOf(t, c)
	createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})
	resp = c.get("/api/me")
	assertStatus(t, resp, http.StatusOK)
	me = decodeBody[store.Tenant](t, resp)
	if me.Usage.Links != 1 || me.Usage.Domains != 1 {
		t.Fatalf("usage = %+v, want 1/1", me.Usage)
	}

	// 设置自动短码长度 → 新短链使用新长度
	resp = c.patch("/api/me", map[string]any{"codeLength": 8})
	assertStatus(t, resp, http.StatusOK)
	me = decodeBody[store.Tenant](t, resp)
	if me.CodeLength != 8 {
		t.Fatalf("codeLength = %d, want 8", me.CodeLength)
	}
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1]})
	if len(link.Code) != 8 {
		t.Fatalf("auto code length = %d, want 8", len(link.Code))
	}

	// 非法长度
	resp = c.patch("/api/me", map[string]any{"codeLength": 1})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}
