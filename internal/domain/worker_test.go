package domain

// worker 与证书探活的单测:间隔校验、panic 隔离、探活是否真的看证书覆盖范围。

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cloak/internal/config"
)

func testCfg() config.Config {
	return config.Config{
		DNSRetryInterval:  5 * time.Minute,
		DNSMaxAge:         72 * time.Hour,
		VisitRetention:    90 * 24 * time.Hour,
		VisitCleanupEvery: 24 * time.Hour,
	}
}

// 间隔必须为正:time.NewTicker(<=0) 会 panic,而 panic 发生在 goroutine 里会终止
// 整个进程(连带 HTTP 服务),所以这里必须在启动前就拒绝。
func TestValidateIntervals(t *testing.T) {
	if err := ValidateIntervals(testCfg()); err != nil {
		t.Fatalf("valid cfg rejected: %v", err)
	}
	bad := []struct {
		name string
		mut  func(*config.Config)
	}{
		{"DNSRetryInterval", func(c *config.Config) { c.DNSRetryInterval = 0 }},
		{"DNSRetryInterval<0", func(c *config.Config) { c.DNSRetryInterval = -time.Second }},
		{"DNSMaxAge", func(c *config.Config) { c.DNSMaxAge = 0 }},
		{"VisitRetention", func(c *config.Config) { c.VisitRetention = 0 }},
		{"VisitCleanupEvery", func(c *config.Config) { c.VisitCleanupEvery = 0 }},
	}
	for _, tc := range bad {
		cfg := testCfg()
		tc.mut(&cfg)
		if err := ValidateIntervals(cfg); err == nil {
			t.Errorf("%s: ValidateIntervals = nil, want error", tc.name)
		}
	}
}

// loop 在间隔非法时必须拒绝启动循环(而不是 panic)。
func TestLoopRejectsNonPositiveInterval(t *testing.T) {
	w := &Worker{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.loop(context.Background(), 0, func(context.Context) {}, "test")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop with 0 interval did not return (ticker panic?)")
	}
	w.guardedLoopTest()
}

func (w *Worker) guardedLoopTest() {
	w.guardedLoop(context.Background(), -time.Second, func(context.Context) {}, "test")
}

// pass 里的 panic 被吞掉,循环本身活着(下一轮还能跑)。
func TestSafePassRecoversPanic(t *testing.T) {
	w := &Worker{}
	runs := 0
	pass := func(context.Context) {
		runs++
		panic("boom")
	}
	w.safePass(context.Background(), pass, "test")
	w.safePass(context.Background(), pass, "test")
	if runs != 2 {
		t.Fatalf("pass ran %d times, want 2 (panic 不应中断后续轮次)", runs)
	}
}

// 证书探活必须看"叶子证书是否覆盖目标 FQDN",而不是"握手成功"。
// httptest 的内置证书覆盖 example.com,用它当"别的域名"就能造出假阳性场景。
func TestProbeCertChecksHostnameCoverage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}))
	defer srv.Close()
	// 让任意主机名都连到测试服务器,只考察 TLS 证书内容。
	client := srv.Client()
tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", client.Transport)
	}
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}
	if ProbeCertFor(srv.Client(), tr) != true {
		t.Fatal("unreachable")
	}
}
