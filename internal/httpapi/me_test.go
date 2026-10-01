package httpapi_test

// 06 — quota-tiers 黑盒测试:配额用量查询(/api/me)、自动短码固定长度、
// 平台默认域名不计入域名配额、新租户默认免费档。

import (
	"net/http"
	"testing"

	"janus/internal/domain"
	"janus/internal/store"
	"janus/internal/testutil"
)

func TestMeUsageAndAutoCodeLength(t *testing.T) {
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

	// 自动生成的短码固定为 domain.AutoCodeLength 位
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1]})
	if len(link.Code) != domain.AutoCodeLength {
		t.Fatalf("auto code length = %d, want %d", len(link.Code), domain.AutoCodeLength)
	}
}

func TestTenantErrorPagesAPI(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 1. GET 初始为空
	resp := c.get("/api/me/error-pages")
	assertStatus(t, resp, http.StatusOK)
	type epResp struct {
		Custom404HTML string `json:"custom404Html"`
		Custom429HTML string `json:"custom429Html"`
	}
	ep := decodeBody[epResp](t, resp)
	if ep.Custom404HTML != "" || ep.Custom429HTML != "" {
		t.Fatalf("expected empty error pages, got: %+v", ep)
	}

	// 2. PATCH 更新有效页面
	resp = c.patch("/api/me/error-pages", map[string]any{
		"custom404Html": "<h1>Not Found</h1>",
		"custom429Html": "<h1>Too Many Requests</h1>",
	})
	assertStatus(t, resp, http.StatusOK)
	ep = decodeBody[epResp](t, resp)
	if ep.Custom404HTML != "<h1>Not Found</h1>" || ep.Custom429HTML != "<h1>Too Many Requests</h1>" {
		t.Fatalf("expected updated error pages, got: %+v", ep)
	}

	// 3. GET 确认持久化
	resp = c.get("/api/me/error-pages")
	assertStatus(t, resp, http.StatusOK)
	ep = decodeBody[epResp](t, resp)
	if ep.Custom404HTML != "<h1>Not Found</h1>" || ep.Custom429HTML != "<h1>Too Many Requests</h1>" {
		t.Fatalf("expected persisted error pages, got: %+v", ep)
	}

	// 4. 超大内容 (>512KB) 返回 400
	hugeHTML := make([]byte, 513*1024)
	for i := range hugeHTML {
		hugeHTML[i] = 'a'
	}
	resp = c.patch("/api/me/error-pages", map[string]any{
		"custom404Html": string(hugeHTML),
	})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}
