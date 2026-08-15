package httpapi_test

// JWT + Casbin RBAC 端到端黑盒测试(Agent C 接线验证):
// 覆盖 Bearer JWT 认证、Casbin 角色授权、CSRF 豁免(header 认证)、
// 超管策略放行与篡改 token 拒绝;同时回归 cookie 会话原有流程不受影响。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"cloak/internal/bootstrap"
	"cloak/internal/httpapi"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

// bearerReq 手工构造携带 Authorization: Bearer 的请求(不带 cookie/CSRF,
// 模拟纯 API 客户端;testClient 的 do 不发送 Authorization 头)。
func bearerReq(t *testing.T, env *testutil.Env, method, path string, body any, token string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, env.Server.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// tokenFromLogin 走公开 token 端点(POST /api/auth/token)黑盒签发 accessToken。
// tokenResp 结构见 jwt_token_test.go(同包复用)。
func tokenFromLogin(t *testing.T, env *testutil.Env, email, password string) string {
	t.Helper()
	resp := newClient(env).post("/api/auth/token", map[string]string{"email": email, "password": password})
	assertStatus(t, resp, http.StatusOK)
	tok := decodeBody[tokenResp](t, resp)
	if tok.AccessToken == "" {
		t.Fatal("accessToken empty")
	}
	return tok.AccessToken
}

// TestAuthzCookieRegression ① 回归:会话 cookie 原有流程不受影响
// (GET /api/links 200;带 CSRF 的 POST /api/domains 201)。
func TestAuthzCookieRegression(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	resp := c.get("/api/links")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 写操作仍需 CSRF(双提交),addDomain 内部断言 201
	d := addDomain(t, c, "localhost")
	if d.Status != "active" {
		t.Fatalf("domain status = %s, want active", d.Status)
	}
}

// TestAuthzBearerMe ② Bearer 访问 GET /api/auth/me → 200 且邮箱正确
// (与 jwt_token_test.go 用例④呼应)。
func TestAuthzBearerMe(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	resp := bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusOK)
	me := decodeBody[store.Tenant](t, resp)
	if me.Email != "alice@example.com" {
		t.Errorf("me email = %q, want alice@example.com", me.Email)
	}
}

// TestAuthzBearerWriteNoCSRF ③ Bearer 写操作不带任何 CSRF 头 → 201
// (header 认证免疫 CSRF)。
func TestAuthzBearerWriteNoCSRF(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	resp := bearerReq(t, env, http.MethodPost, "/api/domains", map[string]string{"fqdn": "localhost"}, tok)
	assertStatus(t, resp, http.StatusCreated)
	d := decodeBody[store.Domain](t, resp)
	if d.Status != "active" {
		t.Fatalf("domain status = %s, want active", d.Status)
	}
}

// TestAuthzTenantForbiddenAdmin ④ tenant 角色访问平台管理端点 → 403,
// 且 body 为统一错误体(code=E_FORBIDDEN)。
func TestAuthzTenantForbiddenAdmin(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	resp := bearerReq(t, env, http.MethodGet, "/api/admin/tenants", nil, tok)
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_FORBIDDEN" {
		t.Fatalf("code = %s, want E_FORBIDDEN", body.Code)
	}
}

// TestAuthzSuperadminBearer ⑤ 超管走 token 端点签发 Bearer → 访问
// GET /api/admin/tenants → 200(超管无密码,FirstLoginSetup=true,任意密码可签发)。
func TestAuthzSuperadminBearer(t *testing.T) {
	env := testutil.Setup(t)
	if err := bootstrap.Superadmin(context.Background(), env.Store, "admin@cloak.test"); err != nil {
		t.Fatalf("bootstrap superadmin: %v", err)
	}
	tok := tokenFromLogin(t, env, "admin@cloak.test", "whatever")

	resp := bearerReq(t, env, http.MethodGet, "/api/admin/tenants", nil, tok)
	assertStatus(t, resp, http.StatusOK)
	tenants := decodeBody[[]*store.Tenant](t, resp)
	if len(tenants) < 1 {
		t.Fatal("tenants empty")
	}
}

// TestAuthzTamperedToken ⑥ 篡改 token(改末尾一个字符)→ 签名校验失败 → 401。
func TestAuthzTamperedToken(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	last := tok[len(tok)-1]
	alt := byte('x')
	if last == alt {
		alt = 'y'
	}
	tampered := tok[:len(tok)-1] + string(alt)
	resp := bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tampered)
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestAuthzNoCredentials ⑦ 不带任何凭证访问受保护路由 → 401。
func TestAuthzNoCredentials(t *testing.T) {
	env := testutil.Setup(t)
	resp := get(t, env, "/api/config")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}
