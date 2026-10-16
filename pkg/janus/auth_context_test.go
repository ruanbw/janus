package janus_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"janus/pkg/janus"
)

// TestGetAuthContextDoesNotDefaultRole 钉住「不静默补默认角色」契约:
// 上下文缺失角色时返回空 Role,而不是伪装成 "tenant"。
func TestGetAuthContextDoesNotDefaultRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Given 上下文只有租户 ID、没有角色 When GetAuthContext Then Role 为空且 ok 为 true", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(janus.CtxTenantIDKey, int64(7))

		ac, ok := janus.GetAuthContext(c)
		if !ok {
			t.Fatal("取到租户 ID 时第二个返回值应为 true")
		}
		if ac.Role != "" {
			t.Fatalf("缺失角色时不应补默认值,got Role=%q", ac.Role)
		}
		if ac.TenantID != 7 {
			t.Fatalf("TenantID = %d, want 7", ac.TenantID)
		}
	})

	t.Run("Given 上下文带角色 When GetAuthContext Then Role 原样保留", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(janus.CtxTenantIDKey, int64(9))
		c.Set(janus.CtxRoleKey, "superadmin")

		ac, ok := janus.GetAuthContext(c)
		if !ok {
			t.Fatal("ok 应为 true")
		}
		if ac.Role != "superadmin" {
			t.Fatalf("Role = %q, want superadmin", ac.Role)
		}
	})

	t.Run("Given 上下文无租户 When GetAuthContext Then ok 为 false", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		if _, ok := janus.GetAuthContext(c); ok {
			t.Fatal("无租户 ID 时 ok 应为 false")
		}
	})
}
