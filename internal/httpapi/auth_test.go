package httpapi_test

// 02 — auth-register-login 黑盒测试:注册/验证/登录/登出/me/CSRF。
// 03 — password-reset 黑盒测试:忘记密码/重置/改密。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"janus/internal/bootstrap"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
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
	if ck, ok := c.cookies["janus_csrf"]; ok {
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
func (c *testClient) put(path string, body any) *http.Response {
	return c.do(http.MethodPut, path, body)
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
	if tenant.DefaultDomain != "alice.janus.test" {
		t.Errorf("defaultDomain = %s, want alice.janus.test", tenant.DefaultDomain)
	}
	if tenant.Tier.Name != "free" || tenant.Tier.MaxLinks != 100 || tenant.Tier.MaxDomains != 10 {
		t.Errorf("tier = %+v, want free 100/10", tenant.Tier)
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
	// slug 与既有默认域名冲突(他人占用 alice.janus.test):通过添加自有域名后 slug 碰撞
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
		if ck.Name == "janus_session" && ck.Value != "" {
			hasSession = true
			if !ck.HttpOnly {
				t.Error("session cookie must be HttpOnly")
			}
		}
	}
	if !hasSession {
		t.Error("login did not set janus_session cookie")
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

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	env := testutil.Setup(t)
	register(t, env, "alice")
	verifyLastEmail(t, env)

	c1 := newClient(env)
	resp := c1.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	c2 := newClient(env)
	resp = c2.post("/api/auth/login", map[string]string{"email": "alice@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = c1.post("/api/auth/change-password", map[string]string{"oldPassword": "password123", "newPassword": "newpass456"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 当前终端保留,其他终端立即失效
	resp = c1.get("/api/auth/me")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c2.get("/api/auth/me")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

// ---------- 超管首次设置密码:一次性 setup token ----------

// bootstrapSuperadmin 初始化一个尚无密码的超管(FirstLoginSetup=true)。
func bootstrapSuperadmin(t *testing.T, env *testutil.Env, email string) {
	t.Helper()
	if err := bootstrap.Superadmin(context.Background(), env.Store, email); err != nil {
		t.Fatalf("bootstrap superadmin: %v", err)
	}
}

// TestSuperadminFirstLoginRequiresSetupToken 超管首次登录必须持一次性 setup token。
//
// 这是本次最严重那条洞的回归测试:修复前 FirstLoginSetup=true 会**整段跳过 bcrypt**,
// 只要知道超管邮箱(证书透明度日志/DNS/GitHub 泄露都能推断)就能换到 superadmin 会话。
// 现在:无 token → 401 并补发一枚;错误 token → 401;正确 token → 200 且一次性(复用即拒)。
func TestSuperadminFirstLoginRequiresSetupToken(t *testing.T) {
	env := testutil.Setup(t)
	bootstrapSuperadmin(t, env, "admin@janus.test")
	c := newClient(env)

	// ① 只带任意密码:必须被拒(且该次失败会补发一枚 setup token)
	resp := c.post("/api/auth/login", map[string]string{"email": "admin@janus.test", "password": "whatever"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// ② 错误的 setup token:同样被拒
	resp = c.post("/api/auth/login", map[string]string{
		"email": "admin@janus.test", "password": "whatever", "setupToken": "not-a-real-token",
	})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// ③ 正确的 setup token → 200,且响应带 firstLoginSetup 标记
	token := env.LastToken(t)
	resp = c.post("/api/auth/login", map[string]string{
		"email": "admin@janus.test", "password": "whatever", "setupToken": token,
	})
	assertStatus(t, resp, http.StatusOK)
	tenant := decodeBody[store.Tenant](t, resp)
	if !tenant.FirstLoginSetup {
		t.Fatal("firstLoginSetup should be true before password is set")
	}
	if !tenant.IsSuperAdmin {
		t.Fatal("tenant should be superadmin")
	}

	// ④ token 一次性:换一个客户端复用同一枚 → 401
	resp = newClient(env).post("/api/auth/login", map[string]string{
		"email": "admin@janus.test", "password": "whatever", "setupToken": token,
	})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

// TestSuperadminWrongPasswordRejectedAfterSetup 设完密码后走正常 bcrypt 路径:
// 错误密码必须被拒(修复前这条路径对任何密码都放行)。
func TestSuperadminWrongPasswordRejectedAfterSetup(t *testing.T) {
	env := testutil.Setup(t)
	bootstrapSuperadmin(t, env, "admin@janus.test")
	c := newClient(env)

	// 触发补发并用 setup token 登录
	_ = c.post("/api/auth/login", map[string]string{"email": "admin@janus.test", "password": "x"})
	resp := c.post("/api/auth/login", map[string]string{
		"email": "admin@janus.test", "setupToken": env.LastToken(t),
	})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 设置密码
	resp = c.post("/api/auth/change-password", map[string]string{"newPassword": "adminpass123"})
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 错误密码 → 401(不再免密)
	resp = newClient(env).post("/api/auth/login", map[string]string{"email": "admin@janus.test", "password": "wrongpass"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 正确密码 → 200
	resp = newClient(env).post("/api/auth/login", map[string]string{"email": "admin@janus.test", "password": "adminpass123"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

// TestPasswordLengthUpperBound 密码超过 bcrypt 的 72 字节上限必须返回 E_VALIDATION,
// 而不是穿透到 bcrypt 变成 500(ErrPasswordTooLong)。72 字节本身仍然合法。
func TestPasswordLengthUpperBound(t *testing.T) {
	env := testutil.Setup(t)
	c := newClient(env)
	long := strings.Repeat("a", 73) // 73 > 72

	resp := c.post("/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": long, "slug": "alice",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_VALIDATION" {
		t.Fatalf("register code = %s, want E_VALIDATION", body.Code)
	}

	// 恰好 72 字节仍然合法(bcrypt 上界含端点)
	resp = c.post("/api/auth/register", map[string]string{
		"email": "bob@example.com", "password": strings.Repeat("b", 72), "slug": "bob",
	})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// 重置密码端点同样受上界约束
	resp = c.post("/api/auth/reset-password", map[string]string{
		"token": "whatever", "newPassword": long,
	})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// TestChangePasswordRejectsOverlongPassword 改密端点的上界校验。
func TestChangePasswordRejectsOverlongPassword(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	resp := c.post("/api/auth/change-password", map[string]string{
		"oldPassword": "password123", "newPassword": strings.Repeat("c", 73),
	})
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_VALIDATION" {
		t.Fatalf("code = %s, want E_VALIDATION", body.Code)
	}
	_ = resp.Body.Close()
}

// TestResendVerification 注册发信失败后的自助恢复入口:恒 202 不泄露邮箱存在性;
// pending 租户会真的收到新验证邮件,已验证的租户不再发信。
func TestResendVerification(t *testing.T) {
	env := testutil.Setup(t)
	c := newClient(env)

	// 不存在的邮箱 → 202(不泄露存在性)
	resp := c.post("/api/auth/resend-verification", map[string]string{"email": "ghost@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	// pending 租户 → 202 且真的补发(可用新 token 完成验证)
	register(t, env, "alice")
	resp = c.post("/api/auth/resend-verification", map[string]string{"email": "alice@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()
	resp = c.post("/api/auth/verify-email", map[string]string{"token": env.LastToken(t)})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 已验证的租户 → 202 且不触发发信
	before := len(env.MailOutput())
	resp = c.post("/api/auth/resend-verification", map[string]string{"email": "alice@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()
	if got := len(env.MailOutput()) - before; got != 0 {
		t.Errorf("active tenant triggered %d extra mail bytes, want 0", got)
	}
}

// TestSuperadminBootstrapPromotionResetsCredentials 回归:有人抢先用超管邮箱注册了普通
// 账号,随后部署配置 JANUS_SUPERADMIN_EMAIL 并启动。提权后原密码、会话、JWT 必须全部
// 作废,只能凭发往该邮箱的 setup token 首登。修复前原密码直接就是超管密码。
func TestSuperadminBootstrapPromotionResetsCredentials(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "squatter")
	tok := tokenFromLogin(t, env, "squatter@example.com", "password123")

	bootstrapSuperadmin(t, env, "squatter@example.com")

	// 旧会话、旧 JWT 立即失效
	resp := c.get("/api/auth/me")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
	resp = bearerReq(t, env, http.MethodGet, "/api/auth/me", nil, tok)
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 原密码不再可用(进入 setup 模式;该次失败补发一枚 setup token)
	resp = newClient(env).post("/api/auth/login", map[string]string{"email": "squatter@example.com", "password": "password123"})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 凭 setup token 才能登录,且是超管
	resp = newClient(env).post("/api/auth/login", map[string]string{
		"email": "squatter@example.com", "setupToken": env.LastToken(t),
	})
	assertStatus(t, resp, http.StatusOK)
	tenant := decodeBody[store.Tenant](t, resp)
	if !tenant.IsSuperAdmin || !tenant.FirstLoginSetup {
		t.Fatalf("superadmin=%v firstLoginSetup=%v, want both true", tenant.IsSuperAdmin, tenant.FirstLoginSetup)
	}
}

// TestSuperadminBootstrapKeepsBannedTenantBanned bootstrap 不得把封禁账号复活。
func TestSuperadminBootstrapKeepsBannedTenantBanned(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "banned")
	id := tenantIDOf(t, c)
	if err := env.Store.SetTenantStatus(context.Background(), id, "banned"); err != nil {
		t.Fatalf("ban tenant: %v", err)
	}

	bootstrapSuperadmin(t, env, "banned@example.com")

	got, err := env.Store.GetTenantByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Status != "banned" {
		t.Fatalf("status = %q, want banned (bootstrap must not reactivate)", got.Status)
	}
}
