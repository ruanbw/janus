package httpapi_test

// httpapi 业务接口层审查项(BIZ*)的黑盒回归测试。见 .workbuddy 审查报告对应条目。

import (
	"net/http"
	"strconv"
	"testing"

	"janus/internal/testutil"
)

// bizCreateLink 建一条跳转型短链,返回短码。
func bizCreateLink(t *testing.T, c *testClient, code string, body map[string]any) {
	t.Helper()
	if _, ok := body["code"]; !ok {
		body["code"] = code
	}
	if _, ok := body["domainIds"]; !ok {
		body["domainIds"] = []int64{localhostDomainID(t, c)}
	}
	createLink(t, c, body)
}

// BIZ6: 删除/彻底删除短链区分"不存在"(404)与"内部错误"(500 + 日志)。
// 这里断言正常路径语义不变:存在 → 204;不存在/已删 → 404。
func TestDeleteLinkStatusSemantics(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "bizdel")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "bizdxyz",
		"targetUrls": []string{"https://example.com/"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	resp := c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 再次逻辑删除 → 已不存活 → 404
	resp = c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()

	// 彻底删除不存在的短链 → 404
	resp = c.post("/api/links/999999/purge", nil)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// BIZ3: 301 跳转必须带 Cache-Control: no-cache, must-revalidate;302 保持不变。
func TestRedirectCacheControlPerStatus(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "bizcache")
	addDomain(t, c, "localhost")

	bizCreateLink(t, c, "bizperm", map[string]any{
		"targetUrls":     []string{"https://example.com/"},
		"redirectStatus": "301",
	})
	bizCreateLink(t, c, "biztemp", map[string]any{
		"targetUrls":     []string{"https://example.com/"},
		"redirectStatus": "302",
	})

	resp301 := redirectGet(t, env, "localhost", "/bizperm")
	assertStatus(t, resp301, http.StatusMovedPermanently)
	if got := resp301.Header.Get("Cache-Control"); got != "no-cache, must-revalidate" {
		t.Fatalf("301 Cache-Control = %q, want %q", got, "no-cache, must-revalidate")
	}

	resp302 := redirectGet(t, env, "localhost", "/biztemp")
	assertStatus(t, resp302, http.StatusFound)
	if got := resp302.Header.Get("Cache-Control"); got != "" {
		t.Fatalf("302 Cache-Control = %q, want empty(保持既有行为)", got)
	}
}
