package httpapi_test

// 改密链路的旧密码猜测限流:与登录共用同一套账号级退避机制，
// 但落在独立命名空间(不能因为改密失败把登录一起锁死)。

import (
	"net/http"
	"testing"

	"janus/internal/testutil"
)

func TestChangePasswordOldPasswordBackoff(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	for i := 0; i < authLimitFailThreshold; i++ {
		resp := c.post("/api/auth/change-password", map[string]string{
			"oldPassword": "wrong-old", "newPassword": "newpass456",
		})
		assertStatus(t, resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}

	// 阈值之后:即使旧密码正确也被退避(否则"始终猜同一个密码"没有成本上界)。
	resp := c.post("/api/auth/change-password", map[string]string{
		"oldPassword": "password123", "newPassword": "newpass456",
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("退避后正确旧密码的状态码 = %d, want 429", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 独立命名空间:改密被退避不影响同一账号的正常登录。
	resp2 := newClient(env).post("/api/auth/login", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	assertStatus(t, resp2, http.StatusOK)
	_ = resp2.Body.Close()
}
