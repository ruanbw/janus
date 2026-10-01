package httpapi_test

// 12 — auth rate limiting 黑盒测试(spec 决策 #12):注册/登录/忘记密码等
// 认证端点按来源 IP 限流,超限 429 + E_RATE_LIMITED。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/mailer"
	"janus/internal/testutil"
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

// serveWithTrustedProxies 以指定的可信代理网段起一个 API 实例(httptest.Server 默认
// 从 127.0.0.1 连过来,正好落在 127.0.0.1/32 内)。
func serveWithTrustedProxies(t *testing.T, env *testutil.Env, cidrs []string, rl httpapi.RateLimitConfig) *httptest.Server {
	t.Helper()
	cfg := env.Cfg
	cfg.TrustedProxyCIDrs = cidrs
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store:     env.Store,
		Mailer:    mailer.NewMailer(mailer.Config{BaseURL: "https://app.janus.test"}, &bytes.Buffer{}),
		Cfg:       cfg,
		RateLimit: &rl,
		GeoLookup: geo.Disabled,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// postLoginFrom 以指定 X-Forwarded-For 发起一次登录,返回状态码。
func postLoginFrom(t *testing.T, srv *httptest.Server, xff string) int {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": "nobody@example.com", "password": "password123"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// TestRateLimitIgnoresForgedXFFWhenNoTrustedProxy 未配置 JANUS_TRUSTED_PROXY_CIDRS 时,
// 即使 TCP 对端是私网/回环地址也**必须**忽略 X-Forwarded-For。
//
// 这是"限流可被伪造 XFF 绕过"那条洞的回归测试:旧实现按"对端是私网 ⇒ 可信"推断,
// 于是每次请求自带一个不同的 XFF 就换一个限流桶,注册/登录/找回密码的每 IP 限流
// 形同虚设(README 6.4 明确允许后端 :8080 裸跑,局域网内可直接连)。
func TestRateLimitIgnoresForgedXFFWhenNoTrustedProxy(t *testing.T) {
	env := testutil.Setup(t)
	rl := httpapi.RateLimitConfig{
		RegisterLimit: 100000, RegisterWindow: time.Minute,
		// 滚动窗口的语义是"任意连续 window 内最多 AuthLimit 次"(见 newRateLimiter),
		// 所以 AuthLimit=3 恰好放行前三次、第四次 429。
		AuthLimit: 3, AuthWindow: time.Minute,
	}
	srv := serveWithTrustedProxies(t, env, nil, rl)

	// 每次伪造一个不同的 XFF。若被采信,三次请求落在三个不同的桶上,应全部 401。
	for i, xff := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		if code := postLoginFrom(t, srv, xff); code != http.StatusUnauthorized {
			t.Fatalf("request %d (XFF %s) = %d, want 401", i+1, xff, code)
		}
	}
	// 第四次必然超限 —— 证明上面三次用的是同一个桶,XFF 没能换桶。
	if code := postLoginFrom(t, srv, "4.4.4.4"); code != http.StatusTooManyRequests {
		t.Fatalf("4th request = %d, want 429 (伪造 XFF 换到了新限流桶)", code)
	}
}

// TestRateLimitTrustsXFFFromConfiguredTrustedProxy 配置了可信代理网段后,来自该网段的
// XFF 必须被采信(否则前置 Caddy 部署下所有用户共用一个桶,等于全局限流)。
func TestRateLimitTrustsXFFFromConfiguredTrustedProxy(t *testing.T) {
	env := testutil.Setup(t)
	rl := httpapi.RateLimitConfig{
		RegisterLimit: 100000, RegisterWindow: time.Minute,
		AuthLimit: 1, AuthWindow: time.Minute,
	}
	srv := serveWithTrustedProxies(t, env, []string{"127.0.0.1/32"}, rl)

	// 每个 XFF 一个桶 → 两次都放行
	if code := postLoginFrom(t, srv, "1.1.1.1"); code != http.StatusUnauthorized {
		t.Fatalf("1st request = %d, want 401", code)
	}
	if code := postLoginFrom(t, srv, "2.2.2.2"); code != http.StatusUnauthorized {
		t.Fatalf("2nd request = %d, want 401 (可信代理的 XFF 应被采信并分桶)", code)
	}
	// 同一 XFF 再来一次 → 撞上自己的桶 → 429
	if code := postLoginFrom(t, srv, "2.2.2.2"); code != http.StatusTooManyRequests {
		t.Fatalf("repeat of same XFF = %d, want 429", code)
	}
}
