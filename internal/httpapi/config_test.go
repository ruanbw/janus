package httpapi_test

// 前端启动配置:GET /api/config — 未登录 401;返回服务器 IP/平台域名/当前租户配额;
// 不同租户用量相互隔离(每个租户按自身信息返回)。

import (
	"net/http"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

type appConfig struct {
	ServerIP       string      `json:"serverIp"`
	PlatformDomain string      `json:"platformDomain"`
	Usage          store.Usage `json:"usage"`
}

func TestGetConfig(t *testing.T) {
	env := testutil.Setup(t)

	// 未登录 → 401
	resp := newClient(env).get("/api/config")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	alice := loggedInTenant(t, env, "alice")

	// 登录后:返回服务器 IP、平台域名与初始配额(0/100, 0/10)
	resp = alice.get("/api/config")
	assertStatus(t, resp, http.StatusOK)
	cfg := decodeBody[appConfig](t, resp)
	if cfg.ServerIP != "127.0.0.1" {
		t.Errorf("serverIp = %q, want 127.0.0.1", cfg.ServerIP)
	}
	if cfg.PlatformDomain != testutil.PlatformDomain {
		t.Errorf("platformDomain = %q, want %q", cfg.PlatformDomain, testutil.PlatformDomain)
	}
	if cfg.Usage.Domains != 0 || cfg.Usage.Links != 0 {
		t.Errorf("usage = %+v, want 0/0", cfg.Usage)
	}

	// 租户隔离:alice 添加域名后,她的域名用量 +1;bob 不受影响
	addDomain(t, alice, "localhost")
	resp = alice.get("/api/config")
	assertStatus(t, resp, http.StatusOK)
	cfg = decodeBody[appConfig](t, resp)
	if cfg.Usage.Domains != 1 {
		t.Errorf("alice usage.domains = %d, want 1", cfg.Usage.Domains)
	}

	bob := loggedInTenant(t, env, "bob")
	resp = bob.get("/api/config")
	assertStatus(t, resp, http.StatusOK)
	cfg = decodeBody[appConfig](t, resp)
	if cfg.Usage.Domains != 0 {
		t.Errorf("bob usage.domains = %d, want 0 (租户隔离)", cfg.Usage.Domains)
	}
}
