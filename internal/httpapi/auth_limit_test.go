package httpapi_test

// 账号级失败退避(anti-credential-stuffing)黑盒测试。
//
// 背景:登录/token 原先只有每 IP 限流,换 IP 就能对同一账号无限撞库。
// 这里断言的是"按账号"的行为:阈值内正常用户不受影响;连续失败到阈值后,
// 即使密码正确也被退避;且退避只作用于该账号。

import (
	"net/http"
	"testing"

	"janus/internal/testutil"
)

// loginFailThreshold 与实现里的常量一致(黑盒测试不引用内部常量,改动需同步)。
const authLimitFailThreshold = 10

func loginAttempt(t *testing.T, c *testClient, email, password string) *http.Response {
	t.Helper()
	return c.post("/api/auth/login", map[string]string{"email": email, "password": password})
}

func TestLoginAccountBackoff(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	// ① 阈值内:几次失败不影响随后的正确登录(成功即清零)。
	for i := 0; i < authLimitFailThreshold/2; i++ {
		resp := loginAttempt(t, c, "alice@example.com", "wrong-password")
		assertStatus(t, resp, http.StatusUnauthorized)
		_ = resp.Body.Close()
	}
	resp := loginAttempt(t, c, "alice@example.com", "password123")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// ② 连续失败到阈值:此后**即使密码正确**也被退避(449 语义不成立,这里 429)。
	for i := 0; i < authLimitFailThreshold; i++ {
		resp := loginAttempt(t, c, "alice@example.com", "wrong-password")
		assertStatus(t, resp, http.StatusUnauthorized)
		_ = resp.Body.Close()
	}
	resp = loginAttempt(t, c, "alice@example.com", "password123")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("退避后正确密码的状态码 = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("退避响应缺少 Retry-After")
	}
	_ = resp.Body.Close()

	// ③ 同一凭据校验也服务 /api/auth/token,同样被退避。
	resp = postJSON(t, env.Server.URL+"/api/auth/token", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("退避后 token 端点状态码 = %d, want 429", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// ④ 退避是按账号的:别的账号完全不受影响。
	register(t, env, "bob")
	verifyLastEmail(t, env)
	resp = loginAttempt(t, newClient(env), "bob@example.com", "password123")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}
