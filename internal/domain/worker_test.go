package domain

// worker 与证书探活的单测:间隔校验、panic 隔离、探活是否真的看证书覆盖范围。

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"janus/internal/config"
	"janus/internal/store"
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
	ran := 0
	pass := func(context.Context) { ran++ }
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.loop(context.Background(), 0, pass, "test")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop with 0 interval did not return (ticker panic?)")
	}
	if ran != 0 {
		t.Fatalf("间隔非法时执行了 %d 轮 pass,应完全不执行", ran)
	}
	// 负间隔同理
	w.loop(context.Background(), -time.Second, pass, "test")
	if ran != 0 {
		t.Fatalf("负间隔时执行了 %d 轮 pass,应完全不执行", ran)
	}
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
	// httptest 内置证书覆盖 example.com / 127.0.0.1,对其他主机名一律不覆盖。
	if !probeCert(context.Background(), "example.com", srv.Client()) {
		t.Fatal("证书覆盖目标 FQDN 时应判定为已签发")
	}
	// 关键场景:域名被指到另一台同样跑 TLS 的主机 —— 握手会成功,
	// 但出示的证书不覆盖它。若这里误判为 true,cert_status 会被写成 issued
	// 且 worker 永不再探,后台显示"已签发"而 Caddy 手里并没有证书。
	if probeCert(context.Background(), "someone-elses-domain.com", srv.Client()) {
		t.Fatal("证书不覆盖目标 FQDN 时不应判定为已签发(握手成功不等于拿到本域证书)")
	}
}

// 归属复检遇到 DNS 查询失败必须**跳过**:不计失败、不降级、不写库。
// Worker 的 store 留 nil —— 只要 recheckOwnership 碰了库就会空指针 panic。
func TestRecheckOwnershipSkipsOnDNSError(t *testing.T) {
	for name, checker := range map[string]*DNSChecker{
		"A/AAAA 超时": {
			ExpectedIP: "203.0.113.7",
			lookupAddr: func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("i/o timeout") },
		},
		"TXT SERVFAIL": {
			ExpectedIP: "203.0.113.7",
			lookupAddr: func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
			},
			lookupTXT: func(_ context.Context, n string) ([]string, error) {
				return nil, &net.DNSError{Err: "server misbehaving", Name: n, IsTemporary: true}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := &Worker{dns: checker}
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("DNS 查询失败时仍访问了 store(应跳过): %v", rec)
				}
			}()
			w.recheckOwnership(context.Background(), store.DomainScanRow{ID: 1, FQDN: "shop.customer.com", Status: "active", VerifyToken: "tok"})
		})
	}
}

// 新增的复检节奏常量同样必须为正(ValidateIntervals 覆盖),且阈值至少 2 次才有容错意义。
func TestOwnershipRecheckTuning(t *testing.T) {
	if OwnershipRecheckFailThreshold < 2 {
		t.Fatalf("OwnershipRecheckFailThreshold = %d,单次失败即降级正是要修的问题", OwnershipRecheckFailThreshold)
	}
	if OwnershipRecheckRetryInterval <= 0 || OwnershipRecheckRetryInterval >= OwnershipRecheckInterval {
		t.Fatalf("失败重查间隔 %v 应为正且短于常规复检间隔 %v", OwnershipRecheckRetryInterval, OwnershipRecheckInterval)
	}
	if ownershipRecheckTick > OwnershipRecheckRetryInterval {
		t.Fatalf("扫描节奏 %v 慢于失败重查间隔 %v,重查会被拖慢", ownershipRecheckTick, OwnershipRecheckRetryInterval)
	}
}
