package httpapi

// 外部受保护扩展路由(WithProtectedRoutes)的授权与 CSRF 单元测试(无需数据库):
// 用假认证中间件代替 authenticate(),直接按 header 写入角色/认证方式/会话,
// 其后是真实的 authorize()(Casbin)与 mountProtectedExtensions 挂载逻辑。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"janus/internal/rbac"
	"janus/internal/store"
)

const testCSRF = "csrf-123"

// fakeAuthenticate 按 X-Test-Role / X-Test-Method 写入认证 context;
// cookie 方式附带一个 CSRFToken=testCSRF 的会话。
func fakeAuthenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetHeader("X-Test-Role")
		if role == "" {
			writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
			c.Abort()
			return
		}
		method := c.GetHeader("X-Test-Method")
		c.Set(ctxTenantKey, &store.Tenant{ID: 1, IsSuperAdmin: role == rbac.RoleSuperadmin})
		c.Set(ctxRoleKey, role)
		c.Set(ctxMethodKey, method)
		if method == "cookie" {
			c.Set(ctxSessionKey, &store.Session{TenantID: 1, CSRFToken: testCSRF})
		}
		c.Next()
	}
}

func newExtensionTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rb, err := rbac.New()
	if err != nil {
		t.Fatalf("rbac.New: %v", err)
	}
	a := &API{rbacEnforcer: rb}
	r := gin.New()
	protAuth := r.Group("/api", fakeAuthenticate())
	prot := protAuth.Group("", a.authorize())
	// 一条基座路由,确保挂载前后的路由表对比只登记扩展自己的路由
	prot.GET("/admin/tenants", func(c *gin.Context) { c.Status(http.StatusOK) })
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	a.mountProtectedExtensions(r, prot, []func(rg *gin.RouterGroup){
		func(rg *gin.RouterGroup) {
			rg.GET("/cloak/campaigns", ok)
			rg.POST("/cloak/campaigns", ok)
			rg.DELETE("/cloak/campaigns/:id", ok)
			rg.GET("/admin/cloak/stats", ok) // 超管专属
		},
		nil, // nil 回调忽略
	})
	return r
}

func doExt(r *gin.Engine, method, path, role, authMethod, csrf string) int {
	req := httptest.NewRequest(method, path, nil)
	if role != "" {
		req.Header.Set("X-Test-Role", role)
		req.Header.Set("X-Test-Method", authMethod)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestProtectedExtensionRoutesAuthorizeAndCSRF(t *testing.T) {
	r := newExtensionTestEngine(t)
	const (
		tenant = rbac.RoleTenant
		sa     = rbac.RoleSuperadmin
	)
	cases := []struct {
		name                     string
		method, path, role, auth string
		csrf                     string
		want                     int
	}{
		{"未认证 401", http.MethodGet, "/api/cloak/campaigns", "", "", "", http.StatusUnauthorized},
		{"租户 GET 扩展路由放行", http.MethodGet, "/api/cloak/campaigns", tenant, "cookie", "", http.StatusOK},
		{"租户 JWT POST 免 CSRF", http.MethodPost, "/api/cloak/campaigns", tenant, "jwt", "", http.StatusOK},
		{"租户 cookie POST 无 CSRF → 403", http.MethodPost, "/api/cloak/campaigns", tenant, "cookie", "", http.StatusForbidden},
		{"租户 cookie POST 错误 CSRF → 403", http.MethodPost, "/api/cloak/campaigns", tenant, "cookie", "wrong", http.StatusForbidden},
		{"租户 cookie POST 正确 CSRF", http.MethodPost, "/api/cloak/campaigns", tenant, "cookie", testCSRF, http.StatusOK},
		{"租户 cookie DELETE 带参路径 正确 CSRF", http.MethodDelete, "/api/cloak/campaigns/7", tenant, "cookie", testCSRF, http.StatusOK},
		{"超管 cookie DELETE 无 CSRF → 403", http.MethodDelete, "/api/cloak/campaigns/7", sa, "cookie", "", http.StatusForbidden},
		{"租户访问超管专属扩展 → 403", http.MethodGet, "/api/admin/cloak/stats", tenant, "jwt", "", http.StatusForbidden},
		{"超管访问超管专属扩展", http.MethodGet, "/api/admin/cloak/stats", sa, "jwt", "", http.StatusOK},
		{"基座管理路由不受扩展登记影响", http.MethodGet, "/api/admin/tenants", tenant, "jwt", "", http.StatusForbidden},
		{"未知角色 → 403", http.MethodGet, "/api/cloak/campaigns", "root", "jwt", "", http.StatusForbidden},
		{"认证方式缺失的写请求 fail-closed", http.MethodPost, "/api/cloak/campaigns", tenant, "", "", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := doExt(r, c.method, c.path, c.role, c.auth, c.csrf); got != c.want {
				t.Fatalf("%s %s role=%q auth=%q csrf=%q → %d, want %d", c.method, c.path, c.role, c.auth, c.csrf, got, c.want)
			}
		})
	}
}
