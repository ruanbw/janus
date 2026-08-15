package httpapi_test

// JWT Bearer 认证黑盒测试:公开端点 POST /api/auth/token 签发 accessToken,
// 以及携带 Bearer 访问受保护 /me 端点(回归 Agent C 接线后的 Bearer 认证)。

import (
	"net/http"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
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
