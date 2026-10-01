package httpapi_test

// JWT Bearer 认证黑盒测试:公开端点 POST /api/auth/token 签发 accessToken,
// 以及携带 Bearer 访问受保护 /me 端点(回归 Agent C 接线后的 Bearer 认证)。

import (
	"net/http"
	"strconv"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

// tokenResp 与 /api/auth/token 成功响应结构一致(契约)。
type tokenResp struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// postToken 调用公开 token 端点(无需 cookie/CSRF)。
func postToken(t *testing.T, env *testutil.Env, email, password string) *http.Response {
	t.Helper()
	return newClient(env).post("/api/auth/token", map[string]string{"email": email, "password": password})
}

func TestTokenEndpointWrongPassword(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	// active 租户错误密码 → 401(等时化比较后拒绝)
	resp := postToken(t, env, "alice@example.com", "wrongpass")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

func TestTokenEndpointPendingTenant(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")

	// 未验证(pending)租户 → 401
	resp := postToken(t, env, "alice@example.com", "password123")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

func TestTokenEndpointIssueAndBearerMe(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	// 注册+验证后登录 token 端点 → 200,accessToken/tokenType/expiresIn 符合契约
	resp := postToken(t, env, "alice@example.com", "password123")
	assertStatus(t, resp, http.StatusOK)
	tok := decodeBody[tokenResp](t, resp)
	if tok.AccessToken == "" {
		t.Error("accessToken empty")
	}
	if tok.TokenType != "Bearer" {
		t.Errorf("tokenType = %q, want Bearer", tok.TokenType)
	}
	if tok.ExpiresIn <= 0 {
		t.Errorf("expiresIn = %d, want > 0", tok.ExpiresIn)
	}

	// 携带该 Bearer 访问 GET /api/auth/me → 200 且租户邮箱正确
	// (testClient 的 do 不发送 Authorization 头,这里手工构造请求)
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/auth/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp2, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp2, http.StatusOK)
	me := decodeBody[store.Tenant](t, resp2)
	if me.Email != "alice@example.com" {
		t.Errorf("me email = %q, want alice@example.com", me.Email)
	}
}

// TestOldJWTRejectedAfterChangePassword 改密后,此前签发的 Bearer JWT 必须立即 401。
//
// 修复前这条是通的:改密只删 sessions 表,而 authenticate 的 JWT 分支根本不查
// sessions,别人手里的 accessToken 在 TTL(默认 24h)内一直能读写全部数据。
func TestOldJWTRejectedAfterChangePassword(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	// 改密前可用
	resp := bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = c.post("/api/auth/change-password", map[string]string{
		"oldPassword": "password123", "newPassword": "newpass456",
	})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 改密后旧 JWT 立即作废
	resp = bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 重新签发的 JWT 正常可用
	tok2 := tokenFromLogin(t, env, "alice@example.com", "newpass456")
	resp = bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok2)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

// TestOldJWTRejectedAfterPasswordReset 重置密码后旧 JWT 同样立即 401。
func TestOldJWTRejectedAfterPasswordReset(t *testing.T) {
	env := testutil.Setup(t)
	loggedInTenant(t, env, "alice")
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	_ = newClient(env).post("/api/auth/forgot-password", map[string]string{"email": "alice@example.com"})
	resp := newClient(env).post("/api/auth/reset-password", map[string]string{
		"token": env.LastToken(t), "newPassword": "newpass456",
	})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	resp = bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

// TestJWTRejectedAfterTenantBanned 封禁租户后其已签发的 JWT 立即 401。
// 封禁是每请求重查租户状态,所以这条在修复前就成立 —— 这里补的是 JWT 侧的覆盖
// (此前测试只验证了会话 cookie 会失效)。
func TestJWTRejectedAfterTenantBanned(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	alice := loggedInTenant(t, env, "alice")
	tok := tokenFromLogin(t, env, "alice@example.com", "password123")

	resp := bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = admin.patch("/api/admin/tenants/"+strconv.FormatInt(tenantIDOf(t, alice), 10),
		map[string]any{"status": "banned"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}
