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

	"janus/internal/bootstrap"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
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
	// 不在这里断言域名状态:这个用例验的是「Bearer 通道不需要 CSRF token」,
	// 而域名激活现在要求一次 TXT 归属挑战(创建时只有 A 记录,状态是 pending,
	// 需要重检才可能 active)。激活路径由 domains_test.go 单独覆盖。
	_ = decodeBody[store.Domain](t, resp)
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
// GET /api/admin/tenants → 200。
//
// 超管无密码时**不再凭任意密码签发 token**:必须先证明能读到超管邮箱
// (一次性 setup token)。否则"知道超管邮箱"就等于拿到整个平台。
func TestAuthzSuperadminBearer(t *testing.T) {
	env := testutil.Setup(t)
	if err := bootstrap.Superadmin(context.Background(), env.Store, "admin@janus.test"); err != nil {
		t.Fatalf("bootstrap superadmin: %v", err)
	}
	// 任意密码 → 401(认证旁路已封);该次失败会触发一枚 setup token 补发
	resp := postToken(t, env, "admin@janus.test", "whatever")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	resp = newClient(env).post("/api/auth/token", map[string]string{
		"email": "admin@janus.test", "password": "whatever", "setupToken": env.LastToken(t),
	})
	assertStatus(t, resp, http.StatusOK)
	tok := decodeBody[tokenResp](t, resp).AccessToken
	if tok == "" {
		t.Fatal("accessToken empty")
	}

	resp = bearerReq(t, env, http.MethodGet, "/api/admin/tenants", nil, tok)
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
