package httpapi_test

// 认证链路「发信失败可观测」黑盒测试:发信错误必须记日志,不能被静默吞掉。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/mailer"
	"janus/internal/testutil"
)

// failingMailer 是恒失败的 mailer 替身:用来观察发信失败路径是否留痕。
type failingMailer struct{}

func (failingMailer) SendVerifyEmail(_, _ string) error { return errors.New("smtp down") }
func (failingMailer) SendResetEmail(_, _ string) error  { return errors.New("smtp down") }

// serveWithMailer 复用现有测试库/配置,只替换 mailer 起一个新的 API 实例。
// 默认 testutil 注入的是控制台 mailer(不会失败),无法覆盖失败路径。
func serveWithMailer(t *testing.T, env *testutil.Env, m mailer.Mailer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store:     env.Store,
		Mailer:    m,
		Cfg:       env.Cfg,
		GeoLookup: geo.Disabled,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// postJSON 直接向指定 server 发一个 JSON POST(不携带 cookie/CSRF)。
func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// TestForgotPasswordLogsMailSendFailure 发信失败必须留痕(slog error),
// 而不是被 `_ =` 吞掉 —— 否则线上"用户收不到重置邮件"完全无从归因。
func TestForgotPasswordLogsMailSendFailure(t *testing.T) {
	env := testutil.Setup(t)
	// 让邮箱真实存在,才能走到发信分支。
	if _, err := env.Store.CreateTenant(context.Background(), "reset@example.com", "hash", "resetuser", false); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	logs := captureLogs(t)
	srv := serveWithMailer(t, env, failingMailer{})

	resp := postJSON(t, srv.URL+"/api/auth/forgot-password", map[string]string{"email": "reset@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	testutil.Poll(t, 2*time.Second, "发信失败日志", func() bool {
		return strings.Contains(logs(), "发送重置邮件失败")
	})
}
