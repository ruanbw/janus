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
		{"tenant PATCH /api/me", RoleTenant, http.MethodPatch, "/api/me", true},
		{"tenant GET /api/me", RoleTenant, http.MethodGet, "/api/me", true},
		{"tenant GET /api/config", RoleTenant, http.MethodGet, "/api/config", true},
		{"tenant POST /api/auth/logout", RoleTenant, http.MethodPost, "/api/auth/logout", true},
		{"tenant GET /api/auth/me", RoleTenant, http.MethodGet, "/api/auth/me", true},
		{"tenant POST /api/auth/change-password", RoleTenant, http.MethodPost, "/api/auth/change-password", true},
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
