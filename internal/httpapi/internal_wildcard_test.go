package httpapi

// 授权端点在泛域名证书模式下的判定(不连库):平台子域一律拒绝 on-demand 签发,
// 裸平台域名与 app 后台域名照常放行。命中的分支都在查库之前,store 留 nil 即可 ——
// 一旦误走到查库,测试会因空指针 panic 暴露出来。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"janus/internal/config"
)

func askRecorder(a *API, fqdn string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/internal/caddy/authorize?domain="+fqdn, nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set(CaddyAskTokenHeader, "ask-secret")
	c.Request = req
	a.handleCaddyAuthorize(c)
	return w
}

func TestAuthorizeWildcardModeDeniesPlatformSubdomains(t *testing.T) {
	a := &API{cfg: config.Config{PlatformDomain: "example.com", CaddyAskToken: "ask-secret", WildcardTLS: true}}
	for _, fqdn := range []string{"alice.example.com", "ALICE.example.com.", "a.b.example.com"} {
		if w := askRecorder(a, fqdn); w.Code != http.StatusForbidden {
			t.Errorf("wildcard 模式 %s: status = %d, want 403", fqdn, w.Code)
		}
	}
	for _, fqdn := range []string{"example.com", "app.example.com"} {
		if w := askRecorder(a, fqdn); w.Code != http.StatusOK {
			t.Errorf("wildcard 模式 %s: status = %d, want 200", fqdn, w.Code)
		}
	}
}
