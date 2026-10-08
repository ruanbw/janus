package rbac

// 单元测试:验证 RBAC 判权矩阵。表驱动用例覆盖——tenant 对契约放行路由返回 true;
// tenant 对平台管理(/api/admin)等未放行路由返回 false;superadmin 对 /api/* 全通;
// 未知角色、未授权方法、路径不匹配一律返回 false。

import (
	"net/http"
	"testing"
)

func TestEnforce(t *testing.T) {
	en, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	if en.Enforcer() == nil {
		t.Fatal("Enforcer() 返回 nil,底层 casbin enforcer 未就绪")
	}

	cases := []struct {
		name   string
		role   string
		method string
		path   string
		want   bool
	}{
		// tenant 对契约放行的路由:返回 true
		{"tenant GET /api/links", RoleTenant, http.MethodGet, "/api/links", true},
		{"tenant GET /api/links/5", RoleTenant, http.MethodGet, "/api/links/5", true},
		{"tenant GET /api/links/5/visits", RoleTenant, http.MethodGet, "/api/links/5/visits", true},
		{"tenant GET /api/links/5/stats", RoleTenant, http.MethodGet, "/api/links/5/stats", true},
		{"tenant POST /api/links", RoleTenant, http.MethodPost, "/api/links", true},
		{"tenant POST /api/links/5/purge", RoleTenant, http.MethodPost, "/api/links/5/purge", true},
		{"tenant POST /api/links/batch-delete", RoleTenant, http.MethodPost, "/api/links/batch-delete", true},
		{"tenant POST /api/links/batch-purge", RoleTenant, http.MethodPost, "/api/links/batch-purge", true},
		{"tenant PATCH /api/links/5", RoleTenant, http.MethodPatch, "/api/links/5", true},
		{"tenant DELETE /api/links/5", RoleTenant, http.MethodDelete, "/api/links/5", true},
		{"tenant GET /api/domains", RoleTenant, http.MethodGet, "/api/domains", true},
		{"tenant POST /api/domains", RoleTenant, http.MethodPost, "/api/domains", true},
		{"tenant GET /api/domains/5", RoleTenant, http.MethodGet, "/api/domains/5", true},
		{"tenant PATCH /api/domains/5", RoleTenant, http.MethodPatch, "/api/domains/5", true},
		{"tenant POST /api/domains/5/recheck", RoleTenant, http.MethodPost, "/api/domains/5/recheck", true},
		{"tenant GET /api/me", RoleTenant, http.MethodGet, "/api/me", true},
		{"tenant GET /api/me/error-pages", RoleTenant, http.MethodGet, "/api/me/error-pages", true},
		{"tenant PATCH /api/me/error-pages", RoleTenant, http.MethodPatch, "/api/me/error-pages", true},
		{"tenant GET /api/config", RoleTenant, http.MethodGet, "/api/config", true},
		{"tenant POST /api/auth/logout", RoleTenant, http.MethodPost, "/api/auth/logout", true},
		{"tenant GET /api/auth/me", RoleTenant, http.MethodGet, "/api/auth/me", true},
		{"tenant POST /api/auth/change-password", RoleTenant, http.MethodPost, "/api/auth/change-password", true},
		// 规则与「规则 ↔ 短链」关联
		{"tenant GET /api/rules", RoleTenant, http.MethodGet, "/api/rules", true},
		{"tenant POST /api/rules", RoleTenant, http.MethodPost, "/api/rules", true},
		{"tenant GET /api/rules/options", RoleTenant, http.MethodGet, "/api/rules/options", true},
		{"tenant GET /api/rules/5", RoleTenant, http.MethodGet, "/api/rules/5", true},
		{"tenant PATCH /api/rules/5", RoleTenant, http.MethodPatch, "/api/rules/5", true},
		{"tenant DELETE /api/rules/5", RoleTenant, http.MethodDelete, "/api/rules/5", true},
		{"tenant GET /api/links/5/rules", RoleTenant, http.MethodGet, "/api/links/5/rules", true},
		{"tenant PUT /api/links/5/rules", RoleTenant, http.MethodPut, "/api/links/5/rules", true},
		// keyMatch3 贪婪语义:/* 匹配任意多段后缀,故 /api/domains/* 与 /api/links/* 也覆盖深层路径
		{"tenant GET /api/domains/5/recheck", RoleTenant, http.MethodGet, "/api/domains/5/recheck", true},
		{"tenant GET /api/links/5/visits/extra", RoleTenant, http.MethodGet, "/api/links/5/visits/extra", true},

		// tenant 未放行的路由:一律返回 false(平台管理仅超管可见)
		{"tenant GET /api/admin/tenants", RoleTenant, http.MethodGet, "/api/admin/tenants", false},
		{"tenant PATCH /api/admin/tenants/5", RoleTenant, http.MethodPatch, "/api/admin/tenants/5", false},
		{"tenant DELETE /api/admin/domains/5", RoleTenant, http.MethodDelete, "/api/admin/domains/5", false},

		// 未授权方法:路径在矩阵中但方法未放行(方法必须与策略逐字一致)
		{"tenant GET /api/auth/logout", RoleTenant, http.MethodGet, "/api/auth/logout", false},
		{"tenant DELETE /api/domains", RoleTenant, http.MethodDelete, "/api/domains", false},
		{"tenant POST /api/config", RoleTenant, http.MethodPost, "/api/config", false},
		{"tenant POST /api/links/5/visits", RoleTenant, http.MethodPost, "/api/links/5/visits", false},
		{"tenant PUT /api/me", RoleTenant, http.MethodPut, "/api/me", false},
		// 关联只接受 GET/PUT:POST /api/links/5/rules 未放行
		{"tenant POST /api/links/5/rules", RoleTenant, http.MethodPost, "/api/links/5/rules", false},
		// keyMatch3 贪婪:/* 覆盖更深层路径,与上面 /api/links/5/rules 的既有行为一致
		{"tenant GET /api/rules/5/anything", RoleTenant, http.MethodGet, "/api/rules/5/anything", true},

		// 路径不匹配:裸路径/未知路径不在任何策略条目的匹配范围内
		{"tenant GET /api/admin", RoleTenant, http.MethodGet, "/api/admin", false},
		{"tenant GET /api/foo/bar", RoleTenant, http.MethodGet, "/api/foo/bar", false},

		// 未知角色:策略中不存在该主体,拒绝
		{"root GET /api/links", "root", http.MethodGet, "/api/links", false},
		{"root GET /api/admin/tenants", "root", http.MethodGet, "/api/admin/tenants", false},

		// superadmin:一条 "p, superadmin, /api/*, *" 覆盖全部路径与方法
		{"superadmin GET /api/admin/tenants", RoleSuperadmin, http.MethodGet, "/api/admin/tenants", true},
		{"superadmin PATCH /api/admin/tenants/5", RoleSuperadmin, http.MethodPatch, "/api/admin/tenants/5", true},
		{"superadmin DELETE /api/admin/domains/5", RoleSuperadmin, http.MethodDelete, "/api/admin/domains/5", true},
		{"superadmin GET /api/domains/5", RoleSuperadmin, http.MethodGet, "/api/domains/5", true},
		{"superadmin GET /api/links", RoleSuperadmin, http.MethodGet, "/api/links", true},
		// 方法通配:策略 act="*" 时任意方法均放行
		{"superadmin OPTIONS /api/links", RoleSuperadmin, http.MethodOptions, "/api/links", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := en.Enforce(tc.role, tc.method, tc.path); got != tc.want {
				t.Errorf("Enforce(%q, %q, %q) = %v, want %v", tc.role, tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// TestGrantTenantExtension 外部扩展路由:租户只被放行扩展实际注册的 (方法, 路径);
// /api/admin 下或首段为参数/通配的扩展路由视为超管专属。
func TestGrantTenantExtension(t *testing.T) {
	en, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	grants := []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/api/cloak/campaigns", true},
		{http.MethodPost, "/api/cloak/campaigns", true},
		{http.MethodPatch, "/api/cloak/campaigns/:id", true},
		{http.MethodGet, "/api/admin/cloak/stats", false}, // 管理命名空间:超管专属
		{http.MethodGet, "/api/:anything/x", false},       // 首段参数可命中 /api/admin
		{http.MethodGet, "/api/*all", false},              // 通配可命中一切
		{http.MethodGet, "/other/x", false},               // 不在 /api 下
	}
	for _, g := range grants {
		got, err := en.GrantTenantExtension(g.method, g.path)
		if err != nil {
			t.Fatalf("GrantTenantExtension(%s %s): %v", g.method, g.path, err)
		}
		if got != g.want {
			t.Errorf("GrantTenantExtension(%s %s) = %v, want %v", g.method, g.path, got, g.want)
		}
	}

	cases := []struct {
		role, method, path string
		want               bool
	}{
		{RoleTenant, http.MethodGet, "/api/cloak/campaigns", true},
		{RoleTenant, http.MethodPost, "/api/cloak/campaigns", true},
		{RoleTenant, http.MethodPatch, "/api/cloak/campaigns/5", true},
		// 未注册的方法/路径不会被顺带放行
		{RoleTenant, http.MethodDelete, "/api/cloak/campaigns/5", false},
		{RoleTenant, http.MethodGet, "/api/cloak/other", false},
		// 超管专属扩展与基座管理路由仍拒绝租户
		{RoleTenant, http.MethodGet, "/api/admin/cloak/stats", false},
		{RoleTenant, http.MethodGet, "/api/admin/tenants", false},
		{RoleTenant, http.MethodGet, "/api/foo/x", false},
		// 超管全通
		{RoleSuperadmin, http.MethodGet, "/api/admin/cloak/stats", true},
		{RoleSuperadmin, http.MethodDelete, "/api/cloak/campaigns/5", true},
	}
	for _, c := range cases {
		if got := en.Enforce(c.role, c.method, c.path); got != c.want {
			t.Errorf("Enforce(%q, %q, %q) = %v, want %v", c.role, c.method, c.path, got, c.want)
		}
	}
}

func TestGinPathToPolicyPath(t *testing.T) {
	cases := map[string]string{
		"/api/x/:id":         "/api/x/{id}",
		"/api/x/:id/y/*rest": "/api/x/{id}/y/*",
		"/api/plain":         "/api/plain",
	}
	for in, want := range cases {
		if got := GinPathToPolicyPath(in); got != want {
			t.Errorf("GinPathToPolicyPath(%q) = %q, want %q", in, got, want)
		}
	}
}
