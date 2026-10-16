package httpapi_test

// httpapi 业务接口层审查项(BIZ*)的黑盒回归测试。见 .workbuddy 审查报告对应条目。

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
	"janus/pkg/plugin"
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

// BIZ11: X-Forwarded-Proto 只在可信代理对端时采信,否则忽略(回落 http)。
func TestLandingSDKIgnoresUntrustedForwardedProto(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "bizxfp")
	addDomain(t, c, "localhost")
	createLandingLink(t, c, localhostDomainID(t, c), "bizxfp", "url", "https://page.example.com/lp")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/bizxfp/sdk.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost"
	// 测试环境未配置可信代理网段 → 该头来自不可信的对端,必须被忽略。
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "http://localhost/bizxfp/click") {
		t.Fatalf("SDK 未按不可信来源回落 http: %s", body)
	}
	if strings.Contains(string(body), "https://localhost/bizxfp/click") {
		t.Fatalf("SDK 错误地采信了不可信来源的 X-Forwarded-Proto: %s", body)
	}
}

// BIZ12: 落地页二级路径对非 GET/HEAD 应回 405 + Allow,而不是 404。
func TestLandingFallbackMethodNotAllowed(t *testing.T) {
	env := testutil.Setup(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, env.Server.URL+"/foo/bar", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "localhost"
		resp, err := env.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("%s /foo/bar: %v", method, err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != "GET, HEAD" {
			t.Fatalf("%s Allow = %q, want %q", method, got, "GET, HEAD")
		}
		_ = resp.Body.Close()
	}

	// GET 不受影响(无域名/无短码 → 仍是 404)
	resp := redirectGet(t, env, "localhost", "/foo/bar")
	assertStatus(t, resp, http.StatusNotFound)
}

// BIZ7: page 参数非法(非整数/超上界)回 400,而不是溢出后 500 或静默当第 1 页。
func TestPageParamsValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "bizpage")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "bizp",
		"targetUrls": []string{"https://example.com/"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	for _, q := range []string{"?page=abc", "?page=99999999999999999999", "?page=20000000"} {
		resp := c.get("/api/links" + q)
		assertStatus(t, resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}
	// 仍合法的分页照常 200
	resp := c.get("/api/links?page=1&pageSize=10")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 访问列表同样走严格分页
	resp = c.get("/api/links/" + strconv.FormatInt(link.ID, 10) + "/visits?page=abc")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// bizEvilInterceptor 模拟"插件前置拦截器注入 javascript: 目标"——它绕过创建期的
// 校验,只能靠 applyRuleDecision 出口的统一校验拦住。
type bizEvilInterceptor struct{ dest string }

func (bizEvilInterceptor) Name() string { return "biz-evil-destination" }

func (b bizEvilInterceptor) Intercept(_ context.Context, _ *http.Request, _ int64, _ *plugin.Fact) (*plugin.Decision, bool) {
	return &plugin.Decision{Action: "redirect", Destination: b.dest}, true
}

// BIZ5: 目标 URL 协议白名单——创建/规则写入拒绝可执行协议;插件注入的非法目标
// 在跳转出口被拦下(走原跳转,而不是把 javascript: 写进 Location)。
func TestTargetURLProtocolWhitelist(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "bizproto")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)

	// 创建短链:javascript:/data:/file: 一律 400
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,<script>x</script>", "vbscript:msgbox(1)", "file:///etc/passwd"} {
		resp := c.post("/api/links", map[string]any{
			"targetUrls": []string{bad},
			"domainIds":  []int64{lid},
		})
		assertStatus(t, resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}
	// 合法 https 不受影响
	bizCreateLink(t, c, "bizprotok", map[string]any{
		"targetUrls": []string{"https://example.com/ok"},
	})

	// 规则写入同样拒绝(复用同一个 validTargetURL)
	resp := c.post("/api/rules", map[string]any{
		"name":        "bad-dest",
		"action":      store.RuleActionRedirect,
		"destination": "javascript:alert(1)",
		"conditions":  ruleOnPath("bizprotok"),
	})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 插件注入非法目标 → 出口拦下,回落到短链原目标
	plugin.ResetInterceptors()
	t.Cleanup(plugin.ResetInterceptors)
	plugin.RegisterInterceptor(bizEvilInterceptor{dest: "javascript:alert(1)"})

	resp = redirectGet(t, env, "localhost", "/bizprotok")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://example.com/ok" {
		t.Fatalf("Location = %q, want 原目标(非法 destination 应被出口拦下)", loc)
	}
}

// BIZ15: validateLinkDomains 改为一次批量取回,既有判定语义(归属/激活/去重)不变。
func TestValidateLinkDomainsBatchSemantics(t *testing.T) {
	env := testutil.Setup(t)
	alice := loggedInTenant(t, env, "bizowna")
	bob := loggedInTenant(t, env, "bizownb")

	addDomain(t, alice, "localhost")
	aliceLid := localhostDomainID(t, alice)
	bobDom := addDomain(t, bob, "bobhost.localhost")
	bobLid := bobDom.ID
	// 未激活(pending)域名:.invalid 解析不到,停在 pending
	pending := addDomain(t, alice, "pending.invalid")

	base := func(ids []int64) map[string]any {
		return map[string]any{
			"targetUrls": []string{"https://example.com/"},
			"domainIds":  ids,
		}
	}

	// 合法:alice 自己的 active 域名
	resp := alice.post("/api/links", base([]int64{aliceLid, aliceLid}))
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// 不存在 / 属于他人 → 400 "域名不存在或不属于当前租户"
	for _, ids := range [][]int64{{999999}, {bobLid}} {
		resp = alice.post("/api/links", base(ids))
		assertStatus(t, resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}

	// 未激活 → 400
	resp = alice.post("/api/links", base([]int64{pending.ID}))
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
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
