package httpapi_test

// 12 — auth rate limiting 黑盒测试(spec 决策 #12):注册/登录/忘记密码等
// 认证端点按来源 IP 限流,超限 429 + E_RATE_LIMITED。

import (
	"net/http"
	"testing"
	"time"

	"cloak/internal/httpapi"
	"cloak/internal/testutil"
)

func TestRegisterRateLimit(t *testing.T) {
	env := testutil.SetupWithRateLimit(t, httpapi.RateLimitConfig{
		RegisterLimit: 2, RegisterWindow: time.Minute,
		AuthLimit: 100000, AuthWindow: time.Minute,
	})
	c := newClient(env)
	for i, slug := range []string{"alice", "bob", "carol"} {
		resp := c.post("/api/auth/register", map[string]string{
			"email": slug + "@example.com", "password": "password123", "slug": slug,
		})
		if i < 2 {
			assertStatus(t, resp, http.StatusCreated)
		} else {
			assertStatus(t, resp, http.StatusTooManyRequests)
			body := decodeBody[httpapi.ErrorBody](t, resp)
			if body.Code != "E_RATE_LIMITED" {
				t.Fatalf("code = %s, want E_RATE_LIMITED", body.Code)
			}
		}
		_ = resp.Body.Close()
	}
}

func TestAuthRateLimitSharedAcrossLoginAndForgot(t *testing.T) {
	env := testutil.SetupWithRateLimit(t, httpapi.RateLimitConfig{
		RegisterLimit: 100000, RegisterWindow: time.Minute,
		AuthLimit: 2, AuthWindow: time.Minute,
	})
	c := newClient(env)

	// 两次认证类调用(登录 401 + 忘记密码 202)后,第三次超限
	resp := c.post("/api/auth/login", map[string]string{"email": "ghost@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	resp = c.post("/api/auth/forgot-password", map[string]string{"email": "ghost@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	resp = c.post("/api/auth/login", map[string]string{"email": "ghost@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusTooManyRequests)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_RATE_LIMITED" {
		t.Fatalf("code = %s, want E_RATE_LIMITED", body.Code)
	}
	_ = resp.Body.Close()
}
