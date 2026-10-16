package httpapi_test

// forgot-password / resend-verification 的发信与响应路径解耦(等时化)黑盒测试。
//
// 威胁模型:这两个端点对外恒返回 202,靠"响应时间与邮箱是否存在无关"防枚举。
// 只要真实 SMTP(秒级 DialAndSend)留在响应路径上,"存在→真发信→慢"与
// "不存在→不发信→快"就能被时序区分,等时化形同虚设。

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"janus/internal/testutil"
)

// slowMailer 模拟慢 SMTP:每次发送固定耗时 delay。
type slowMailer struct{ delay time.Duration }

func (m slowMailer) SendVerifyEmail(_, _ string) error { time.Sleep(m.delay); return nil }
func (m slowMailer) SendResetEmail(_, _ string) error  { time.Sleep(m.delay); return nil }

// TestForgotPasswordRespondsWithoutWaitingForSMTP 发信耗时 2s 时,响应仍须立即
// 返回(阈值 1s);同步发信会必然超时。
func TestForgotPasswordRespondsWithoutWaitingForSMTP(t *testing.T) {
	env := testutil.Setup(t)
	if _, err := env.Store.CreateTenant(context.Background(), "slow@example.com", "hash", "slowuser", false); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	srv := serveWithMailer(t, env, slowMailer{delay: 2 * time.Second})

	start := time.Now()
	resp := postJSON(t, srv.URL+"/api/auth/forgot-password", map[string]string{"email": "slow@example.com"})
	elapsed := time.Since(start)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	if elapsed > time.Second {
		t.Fatalf("forgot-password 响应耗时 %v,等待了 SMTP 发送", elapsed)
	}
}

// TestResendVerificationRespondsWithoutWaitingForSMTP pending 租户的 resend 分支
// 同样不得等待 SMTP(该分支原先同步发信,与不存在邮箱的快速分支可被时序区分)。
func TestResendVerificationRespondsWithoutWaitingForSMTP(t *testing.T) {
	env := testutil.Setup(t)
	// pending 租户:resend 会真的补发(走发信分支)。
	if _, err := env.Store.CreateTenant(context.Background(), "pending@example.com", "hash", "pendinguser", false); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	srv := serveWithMailer(t, env, slowMailer{delay: 2 * time.Second})

	start := time.Now()
	resp := postJSON(t, srv.URL+"/api/auth/resend-verification", map[string]string{"email": "pending@example.com"})
	elapsed := time.Since(start)
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	if elapsed > time.Second {
		t.Fatalf("resend-verification 响应耗时 %v,等待了 SMTP 发送", elapsed)
	}
}

// TestForgotPasswordMailStillDelivered 异步化不得把邮件弄丢:响应之后必须仍然送达。
func TestForgotPasswordMailStillDelivered(t *testing.T) {
	env := testutil.Setup(t)
	if _, err := env.Store.CreateTenant(context.Background(), "deliver@example.com", "hash", "deliveruser", false); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	resp := postJSON(t, env.Server.URL+"/api/auth/forgot-password", map[string]string{"email": "deliver@example.com"})
	assertStatus(t, resp, http.StatusAccepted)
	_ = resp.Body.Close()

	testutil.Poll(t, 3*time.Second, "重置邮件异步送达", func() bool {
		return strings.Contains(env.MailOutput(), "deliver@example.com")
	})
}
