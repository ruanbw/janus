package rbac

import (
	"net/http"
	"testing"
)

// TestEnforcerAccessorStable 钉住 Enforcer() 的只读契约:多次返回同一底层实例,
// 且 Enforce 的判权行为不因暴露访问器而改变。
func TestEnforcerAccessorStable(t *testing.T) {
	en, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	if en.Enforcer() == nil {
		t.Fatal("Enforcer() 返回 nil")
	}
	if en.Enforcer() != en.Enforcer() {
		t.Fatal("Enforcer() 应稳定返回同一底层实例")
	}
	// 暴露访问器不影响判权结果。
	if !en.Enforce(RoleSuperadmin, http.MethodGet, "/api/anything") {
		t.Fatal("superadmin 应放行 /api/*")
	}
	if en.Enforce(RoleTenant, http.MethodGet, "/api/admin/tenants") {
		t.Fatal("tenant 不应放行 /api/admin/*")
	}
}
