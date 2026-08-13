package httpapi_test

// 01 — bootstrap-env 黑盒测试:健康检查与 Caddy 授权端点(仅内网可达、平台域名放行、未注册域名拒绝)。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloak/internal/testutil"
)

func TestHealthz(t *testing.T) {
	env := testutil.Setup(t)
	resp := get(t, env, "/healthz")
	assertStatus(t, resp, http.StatusOK)
}

func TestAuthorizePlatformDomains(t *testing.T) {
	env := testutil.Setup(t)
	cases := []struct {
		name   string
		domain string
		want   int
	}{
		{"platform root domain", "cloak.test", http.StatusOK},
		{"app subdomain", "app.cloak.test", http.StatusOK},
		{"unknown external domain", "attacker.example.com", http.StatusForbidden},
		{"unregistered subdomain", "nobody.cloak.test", http.StatusForbidden},
		{"missing domain param", "", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := get(t, env, "/internal/caddy/authorize?domain="+c.domain)
			assertStatus(t, resp, c.want)
		})
	}
}

// TestAuthorizeInternalOnly 授权端点仅内网可达:非内网来源直接 403。
func TestAuthorizeInternalOnly(t *testing.T) {
	env := testutil.Setup(t)
	// httptest.NewRequest 默认 RemoteAddr=192.0.2.1(TEST-NET,非内网)
	req := httptest.NewRequest(http.MethodGet, "/internal/caddy/authorize?domain=cloak.test", nil)
	rec := httptest.NewRecorder()
	env.Server.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-private source authorize = %d, want 403 (body %s)", rec.Code, rec.Body)
	}
}

// ---------- helpers ----------

func get(t *testing.T, env *testutil.Env, path string) *http.Response {
	t.Helper()
	resp, err := env.Server.Client().Get(env.Server.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, want, body)
	}
}

func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return v
}
