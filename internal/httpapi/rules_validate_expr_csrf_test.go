package httpapi_test

// 表达式校验端点的一致性(审查项 COND4):handleValidateExpr 只做了 requireSession,
// 没有 requireCSRF,与同组其它写端点(POST/PATCH/DELETE rules、simulate)不一致。
// 该端点接收租户自由输入并触发编译,补齐 CSRF 双提交校验。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"janus/internal/testutil"
)

// TestValidateExprRequiresCSRF 缺 CSRF 头 → 403(与同组写端点口径一致)。
func TestValidateExprRequiresCSRF(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 带 CSRF:正常工作
	resp := c.post("/api/rules/validate-expr", map[string]any{"expression": `Country == "US"`})
	assertStatus(t, resp, http.StatusOK)
	body := decodeBody[map[string]any](t, resp)
	if body["valid"] != true {
		t.Fatalf("合法表达式 valid = %v, want true", body["valid"])
	}

	// 仅 cookie 会话、不带 CSRF 头 → 403
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]any{"expression": `Country == "US"`}); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/rules/validate-expr", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	noCSRF, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST validate-expr without csrf: %v", err)
	}
	assertStatus(t, noCSRF, http.StatusForbidden)
	_ = noCSRF.Body.Close()

	// 未登录仍是 401(先认证再 CSRF)
	anon := newClient(env)
	resp = anon.post("/api/rules/validate-expr", map[string]any{"expression": `Country == "US"`})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}
