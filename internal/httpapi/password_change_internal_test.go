package httpapi

// 强制改密闸门(must_change_password)的白名单与拦截行为。
//
// 全是纯逻辑:白名单是 (method, path) 判定,中间件只读 gin.Context,两者都不查库。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ruanbw/janus/internal/store"
)

func TestPasswordChangeExemptWhitelist(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/auth/change-password"},
		{http.MethodPost, "/api/auth/logout"},
	}
	for _, c := range allowed {
		if !passwordChangeExempt(c.method, c.path) {
			t.Errorf("passwordChangeExempt(%s %s) = false, want true", c.method, c.path)
		}
	}

	blocked := []struct{ method, path, why string }{
		{http.MethodGet, "/api/me", "租户设置接口不在白名单(白名单只有 /api/auth/me)"},
		{http.MethodPost, "/api/auth/me", "方法必须精确匹配"},
		{http.MethodPatch, "/api/auth/change-password", "改密只允许 POST"},
		{http.MethodGet, "/api/auth/change-password", "改密只允许 POST"},
		{http.MethodDelete, "/api/auth/logout", "登出只允许 POST"},
		{http.MethodPost, "/api/auth/logout/extra", "路径必须精确匹配,不允许前缀放行"},
		{http.MethodGet, "/api/links", "普通业务端点一律拦截"},
		{http.MethodGet, "/api/config", "普通业务端点一律拦截"},
	}
	for _, c := range blocked {
		if passwordChangeExempt(c.method, c.path) {
			t.Errorf("passwordChangeExempt(%s %s) = true, want false(%s)", c.method, c.path, c.why)
		}
	}
}

// newPasswordGateEngine 搭一个最小路由树:注入租户(替身 authenticate)→ 闸门 → 终点。
// 路径与真实路由一致,所以 c.FullPath() 在闸门里取到的就是真实路由模板。
func newPasswordGateEngine(t *testing.T, mustChange bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := &API{}
	g := r.Group("/api",
		func(c *gin.Context) {
			c.Set(ctxTenantKey, &store.Tenant{ID: 1, Username: "superadmin", MustChangePassword: mustChange})
			c.Next()
		},
		api.requirePasswordChange(),
	)
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	g.GET("/auth/me", ok)
	g.POST("/auth/change-password", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	g.POST("/auth/logout", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	g.GET("/me", ok)
	g.GET("/links", ok)
	return r
}

func doGateRequest(t *testing.T, r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	r.ServeHTTP(w, req)
	return w
}

func TestRequirePasswordChangeBlocksNonWhitelisted(t *testing.T) {
	r := newPasswordGateEngine(t, true)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodGet, "/api/links"},
	} {
		w := doGateRequest(t, r, c.method, c.path)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, w.Code)
			continue
		}
		if body := w.Body.String(); !strings.Contains(body, errPasswordChangeRequired) || !strings.Contains(body, "首次登录必须修改密码") {
			t.Errorf("%s %s 的响应体缺错误码/文案: %s", c.method, c.path, body)
		}
	}
}

func TestRequirePasswordChangeAllowsWhitelist(t *testing.T) {
	r := newPasswordGateEngine(t, true)

	for _, c := range []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/api/auth/me", http.StatusOK},
		{http.MethodPost, "/api/auth/change-password", http.StatusNoContent},
		{http.MethodPost, "/api/auth/logout", http.StatusNoContent},
	} {
		w := doGateRequest(t, r, c.method, c.path)
		if w.Code != c.wantStatus {
			t.Errorf("%s %s = %d, want %d(白名单必须放行,否则用户无法自救)", c.method, c.path, w.Code, c.wantStatus)
		}
	}
}

func TestRequirePasswordChangePassesThroughWhenFlagClear(t *testing.T) {
	r := newPasswordGateEngine(t, false)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodGet, "/api/links"},
	} {
		if w := doGateRequest(t, r, c.method, c.path); w.Code != http.StatusOK {
			t.Errorf("%s %s = %d, want 200(must_change_password=false 时不得拦截)", c.method, c.path, w.Code)
		}
	}
}
