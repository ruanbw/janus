package httpapi_test

// 02 — auth-register-login 黑盒测试:注册/验证/登录/登出/me/CSRF。
// 03 — password-reset 黑盒测试:忘记密码/重置/改密。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
)

// testClient 维护会话 cookie 与 CSRF token(模拟浏览器行为)。
type testClient struct {
	env     *testutil.Env
	cookies map[string]*http.Cookie
}

func newClient(env *testutil.Env) *testClient {
	return &testClient{env: env, cookies: map[string]*http.Cookie{}}
}

func (c *testClient) csrfToken() string {
	if ck, ok := c.cookies["cloak_csrf"]; ok {
		return ck.Value
	}
	return ""
}

func (c *testClient) do(method, path string, body any) *http.Response {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.env.Server.URL+path, rd)
	if err != nil {
		panic(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	if method != http.MethodGet && method != http.MethodHead && c.csrfToken() != "" {
		req.Header.Set("X-CSRF-Token", c.csrfToken())
	}
	resp, err := c.env.Server.Client().Do(req)
	if err != nil {
		panic(fmt.Sprintf("%s %s: %v", method, path, err))
	}
	for _, ck := range resp.Cookies() {
		c.cookies[ck.Name] = ck
	}
	return resp
}

func (c *testClient) get(path string) *http.Response { return c.do(http.MethodGet, path, nil) }
func (c *testClient) post(path string, body any) *http.Response {
	return c.do(http.MethodPost, path, body)
}
func (c *testClient) patch(path string, body any) *http.Response {
	return c.do(http.MethodPatch, path, body)
}
func (c *testClient) del(path string) *http.Response { return c.do(http.MethodDelete, path, nil) }

// register 注册并返回响应(测试用;成功后自动从 mailer 提取验证 token 存入返回结构)。
func register(t *testing.T, env *testutil.Env, slug string) *testClient {
	t.Helper()
	c := newClient(env)
	resp := c.post("/api/auth/register", map[string]string{
		"email":    slug + "@example.com",
		"password": "password123",
		"slug":     slug,
	})
	assertStatus(t, resp, http.StatusCreated)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return c
}

// verifyLastEmail 从 mailer 输出提取最近 token 并调用验证端点。
func verifyLastEmail(t *testing.T, env *testutil.Env) {
	t.Helper()
	token := env.LastToken(t)
	resp := newClient(env).post("/api/auth/verify-email", map[string]string{"token": token})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestRegisterCreatesPendingTenantWithDefaultDomain(t *testing.T) {
	env := testutil.Setup(t)
	c := newClient(env)
	resp := c.post("/api/auth/register", map[string]string{
		"email":    "alice@example.com",
		"password": "password123",
		"slug":     "alice",
	})
	assertStatus(t, resp, http.StatusCreated)
	tenant := decodeBody[store.Tenant](t, resp)
	if tenant.Status != "pending" {
		t.Errorf("tenant status = %s, want pending", tenant.Status)
	}
	if tenant.DefaultDomain != "alice.cloak.test" {
		t.Errorf("defaultDomain = %s, want alice.cloak.test", tenant.DefaultDomain)
	}
	if tenant.Tier.Name != "free" || tenant.Tier.MaxLinks != 100 || tenant.Tier.MaxDomains != 10 {
		t.Errorf("tier = %+v, want free 100/10", tenant.Tier)
	}
	if tenant.CodeLength != 6 {
		t.Errorf("codeLength = %d, want 6", tenant.CodeLength)
	}
}

func TestRegisterValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := newClient(env)
	cases := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{"bad email", map[string]string{"email": "not-an-email", "password": "password123", "slug": "bob"}, http.StatusBadRequest},
		{"bad slug uppercase", map[string]string{"email": "bob@example.com", "password": "password123", "slug": "Bob"}, http.StatusBadRequest},
		{"bad slug leading dash", map[string]string{"email": "bob@example.com", "password": "password123", "slug": "-bob"}, http.StatusBadRequest},
		{"short password", map[string]string{"email": "bob@example.com", "password": "short", "slug": "bob"}, http.StatusBadRequest},
		{"reserved app slug", map[string]string{"email": "app@example.com", "password": "password123", "slug": "app"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.post("/api/auth/register", tc.body)
			assertStatus(t, resp, tc.status)
			_ = resp.Body.Close()
		})
	}
}

func TestRegisterConflicts(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")

	c := newClient(env)
	// 重复邮箱
	resp := c.post("/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": "password123", "slug": "alice2"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
	// 重复 slug
	resp = c.post("/api/auth/register", map[string]string{
		"email": "alice2@example.com", "password": "password123", "slug": "alice"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
	// slug 与既有默认域名冲突(他人占用 alice.cloak.test):通过添加自有域名后 slug 碰撞
	resp = c.post("/api/auth/register", map[string]string{
		"email": "carol@example.com", "password": "password123", "slug": "alice"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

func TestVerifyThenLogin(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")

	// 未验证登录被拒
	c := newClient(env)
	resp := c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 验证邮箱
	verifyLastEmail(t, env)

	// 登录成功:200 + Set-Cookie + tenant active
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	tenant := decodeBody[store.Tenant](t, resp)
	if tenant.Status != "active" {
		t.Errorf("tenant status = %s, want active", tenant.Status)
	}
	var hasSession bool
	for _, ck := range resp.Cookies() {
		if ck.Name == "cloak_session" && ck.Value != "" {
			hasSession = true
			if !ck.HttpOnly {
				t.Error("session cookie must be HttpOnly")
			}
		}
	}
	if !hasSession {
		t.Error("login did not set cloak_session cookie")
	}

	// 错误密码
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "wrongpass"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

func TestMeAndLogout(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	// 未登录访问 me → 401
	resp := newClient(env).get("/api/auth/me")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	c := newClient(env)
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 登录后 me → 200
	resp = c.get("/api/auth/me")
	assertStatus(t, resp, http.StatusOK)
	me := decodeBody[store.Tenant](t, resp)
	if me.Email != "alice@example.com" || me.Slug != "alice" {
		t.Errorf("me = %+v", me)
	}

	// 登出(带 CSRF)→ 204,再访问 me → 401
	resp = c.post("/api/auth/logout", nil)
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.get("/api/auth/me")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

func TestCSRFRequiredForWrites(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	resp := c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 手动去掉 CSRF header 再登出 → 403
	req, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/auth/logout", nil)
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	resp2, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp2, http.StatusForbidden)
	_ = resp2.Body.Close()
	// 会话仍有效
	resp = c.get("/api/auth/me")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

// ---------- 03 password-reset ----------

func TestForgotAndResetPassword(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	resp := c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// forgot-password:202,且对不存在的邮箱同样 202(不泄露存在性)
	for _, email := range []string{"alice@example.com", "ghost@example.com"} {
		resp := c.post("/api/auth/forgot-password", map[string]string{"email": email})
		assertStatus(t, resp, http.StatusAccepted)
		_ = resp.Body.Close()
	}

	// 从 mailer 提取重置 token(最后一个是 alice 的)并重置
	output := env.MailOutput()
	if !strings.Contains(output, "密码重置") {
		t.Fatalf("mailer output missing reset email: %s", output)
	}
	token := env.LastToken(t)
	resp = c.post("/api/auth/reset-password", map[string]string{"token": token, "newPassword": "newpass456"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 旧密码失效,新密码可登录
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "newpass456"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestResetTokenReuseRejected(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	_ = c.post("/api/auth/forgot-password", map[string]string{"email": "alice@example.com"})
	token := env.LastToken(t)
	resp := c.post("/api/auth/reset-password", map[string]string{"token": token, "newPassword": "newpass456"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	// 再次使用同一 token → 400
	resp = c.post("/api/auth/reset-password", map[string]string{"token": token, "newPassword": "another789"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
	// 无效 token → 400
	resp = c.post("/api/auth/reset-password", map[string]string{"token": "bogus-token-123", "newPassword": "another789"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestChangePassword(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	resp := c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 旧密码错误 → 400
	resp = c.post("/api/auth/change-password", map[string]string{"oldPassword": "wrong-old", "newPassword": "newpass456"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 正确 → 204
	resp = c.post("/api/auth/change-password", map[string]string{"oldPassword": "password123", "newPassword": "newpass456"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 新密码登录
	resp = c.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "newpass456"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

// TestResetTokenConcurrentConsume 并发重置同一 token:原子消费保证恰好一个成功,
// 另一个 400(防止双重消费竞态)。
func TestResetTokenConcurrentConsume(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c := newClient(env)
	_ = c.post("/api/auth/forgot-password", map[string]string{"email": "alice@example.com"})
	token := env.LastToken(t)

	const n = 4
	results := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := c.post("/api/auth/reset-password", map[string]string{"token": token, "newPassword": "newpass456"})
			results <- resp.StatusCode
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	close(results)

	ok, rejected := 0, 0
	for code := range results {
		switch code {
		case http.StatusNoContent:
			ok++
		case http.StatusBadRequest:
			rejected++
		default:
			t.Fatalf("unexpected status %d", code)
		}
	}
	if ok != 1 || rejected != n-1 {
		t.Fatalf("consumes = %d success / %d rejected, want 1/%d", ok, rejected, n-1)
	}
}
