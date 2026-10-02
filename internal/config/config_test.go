package config

import (
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestConfigLoadDefaults(t *testing.T) {
	// 清理可能影响测试的环境变量
	os.Unsetenv("JANUS_ADDR")
	os.Unsetenv("JANUS_DATABASE_URL")
	os.Unsetenv("JANUS_COOKIE_SECURE")
	os.Unsetenv("JANUS_SESSION_TTL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Fatalf("cfg.Addr = %s, want :8080", cfg.Addr)
	}
	if cfg.PlatformDomain != "janus.test" {
		t.Fatalf("cfg.PlatformDomain = %s, want janus.test", cfg.PlatformDomain)
	}
	if !cfg.CookieSecure {
		t.Fatal("cfg.CookieSecure want true")
	}
	if cfg.SessionTTL != 30*24*time.Hour {
		t.Fatalf("cfg.SessionTTL = %v, want 720h", cfg.SessionTTL)
	}
	if cfg.LandingMaxZipBytes != 10*1024*1024 {
		t.Fatalf("cfg.LandingMaxZipBytes = %d, want 10485760", cfg.LandingMaxZipBytes)
	}
}

func TestConfigLoadOverrides(t *testing.T) {
	t.Setenv("JANUS_ADDR", ":9090")
	t.Setenv("JANUS_COOKIE_SECURE", "false")
	t.Setenv("JANUS_SESSION_TTL", "12h")
	t.Setenv("JANUS_SMTP_PORT", "587")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != ":9090" {
		t.Fatalf("cfg.Addr = %s, want :9090", cfg.Addr)
	}
	if cfg.CookieSecure {
		t.Fatal("cfg.CookieSecure want false")
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("cfg.SessionTTL = %v, want 12h", cfg.SessionTTL)
	}
	if cfg.SMTPPort != 587 {
		t.Fatalf("cfg.SMTPPort = %d, want 587", cfg.SMTPPort)
	}
}

func TestLoadRejectsMalformedDuration(t *testing.T) {
	// "30d" 不是合法的 time.Duration,env.Parse 会把该字段留成零值。
	// 旧实现吞掉这个错误继续启动,于是 SessionTTL 变 0 —— 这类"解析失败被吞"
	// 必须变成显式失败,否则零值会一路流到运行时(最坏情况:保留期 0 → 删光访问记录)。
	t.Setenv("JANUS_SESSION_TTL", "30d")
	if _, err := Load(); err == nil {
		t.Fatal("Load() 应在时长格式非法时返回错误")
	}
}

func validConfig() Config {
	return Config{
		SessionTTL:           time.Hour,
		SessionTTLShort:      time.Hour,
		VerifyTokenTTL:       time.Hour,
		ResetTokenTTL:        time.Hour,
		JWTTTL:               time.Hour,
		DNSRetryInterval:     time.Minute,
		DNSMaxAge:            time.Hour,
		VisitRetention:       time.Hour,
		VisitCleanupEvery:    time.Hour,
		VisitCleanupBatch:    1000,
		LandingMaxFiles:      500,
		MaxTargetURLs:        50,
		LandingMaxZipBytes:   1 << 20,
		LandingMaxTotalBytes: 1 << 20,
		GeoTimeout:           500 * time.Millisecond,
	}
}

func TestValidateRejectsNonPositiveDuration(t *testing.T) {
	// time.NewTicker(0) 会 panic,而 worker 在 goroutine 里 → 整个进程终止。
	// 零值必须在启动时被拦住。
	cfg := validConfig()
	cfg.VisitCleanupEvery = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() 应拒绝 JANUS_VISIT_CLEANUP_INTERVAL=0")
	}

	cfg = validConfig()
	cfg.VisitRetention = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() 应拒绝 JANUS_VISIT_RETENTION=0")
	}

	cfg = validConfig()
	cfg.DNSRetryInterval = -time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() 应拒绝负的 JANUS_DNS_RETRY_INTERVAL")
	}
}

func TestValidateRejectsNonPositiveCounts(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"JANUS_VISIT_CLEANUP_BATCH":     func(c *Config) { c.VisitCleanupBatch = 0 },
		"JANUS_LANDING_MAX_FILES":       func(c *Config) { c.LandingMaxFiles = 0 },
		"JANUS_MAX_TARGET_URLS":         func(c *Config) { c.MaxTargetURLs = 0 },
		"JANUS_LANDING_MAX_ZIP_BYTES":   func(c *Config) { c.LandingMaxZipBytes = 0 },
		"JANUS_LANDING_MAX_TOTAL_BYTES": func(c *Config) { c.LandingMaxTotalBytes = 0 },
	} {
		cfg := validConfig()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() 应拒绝 %s=0", name)
		}
	}
}

func TestValidateAcceptsDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("默认配置应通过 Validate(),got %v", err)
	}
}

func TestTrustedProxyNets(t *testing.T) {
	t.Run("解析 CIDR 与单个 IP", func(t *testing.T) {
		cfg := validConfig()
		cfg.TrustedProxyCIDrs = []string{"10.0.0.0/8", " 172.18.0.1 ", "127.0.0.1", "2001:db8::/32"}
		nets, err := cfg.TrustedProxyNets()
		if err != nil {
			t.Fatalf("TrustedProxyNets() error = %v", err)
		}
		if len(nets) != 4 {
			t.Fatalf("len(nets) = %d, want 4", len(nets))
		}
		if nets[1].Bits() != 32 {
			t.Errorf("单个 IP 应转成 /32,got /%d", nets[1].Bits())
		}
	})

	t.Run("拒绝垃圾输入", func(t *testing.T) {
		cfg := validConfig()
		cfg.TrustedProxyCIDrs = []string{"10.0.0.0/8", "not-an-ip"}
		if _, err := cfg.TrustedProxyNets(); err == nil {
			t.Fatal("TrustedProxyNets() 应拒绝非法条目")
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() 应拒绝非法的 JANUS_TRUSTED_PROXY_CIDRS")
		}
	})

	t.Run("空配置表示不采信任何 XFF", func(t *testing.T) {
		cfg := validConfig()
		nets, err := cfg.TrustedProxyNets()
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(nets) != 0 {
			t.Fatalf("空配置应得到空前缀集,got %d 条", len(nets))
		}
	})
}

func TestTrustedProxiesContains(t *testing.T) {
	cfg := validConfig()
	cfg.TrustedProxyCIDrs = []string{"172.18.0.0/16", "10.1.2.3"}
	tp, err := cfg.TrustedProxies()
	if err != nil {
		t.Fatalf("TrustedProxies() error = %v", err)
	}

	cases := []struct {
		addr string
		want bool
	}{
		{"172.18.0.5", true},        // 在 CGNAT 网段内
		{"172.19.0.5", false},       // 不在
		{"10.1.2.3", true},          // 精确匹配的单个 IP
		{"10.1.2.4", false},         // 精确匹配不覆盖邻近地址
		{"::ffff:172.18.0.5", true}, // IPv4-mapped 归一后应命中
		{"8.8.8.8", false},          // 公网地址:永不可信
	}
	for _, c := range cases {
		got := tp.Contains(netip.MustParseAddr(c.addr))
		if got != c.want {
			t.Errorf("Contains(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
	if tp.Contains(netip.Addr{}) {
		t.Error("无效地址不应被判定为可信")
	}
}

func TestEmptyTrustedProxiesTrustsNothing(t *testing.T) {
	// 默认配置下必须「谁都不采信」—— 这是安全的默认值:
	// 宁可信直连 IP 也不采信来路不明的 XFF。
	tp, err := validConfig().TrustedProxies()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if tp.Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Error("未配置可信代理时,回环地址也不应采信 XFF")
	}
	if tp.Contains(netip.MustParseAddr("192.168.1.10")) {
		t.Error("未配置可信代理时,私网地址也不应采信 XFF")
	}
}
