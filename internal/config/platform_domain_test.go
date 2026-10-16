package config

import "testing"

// 平台域名必须在唯一的公共漏斗(parseEnvFrom)里归一化。
//
// 回归的故障链:运维把 JANUS_PLATFORM_DOMAIN 写成 "Example.com" 或带尾点,
// 而归一化责任此前散落在各消费点 —— store 原样入库、授权端点拿归一化值与裸配置值
// 比较(永不命中 → 平台自己签不出证书)、更新路径按裸值比对(0 行不报错)。
func TestParseEnvFrom_NormalizesPlatformDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Example.com", "example.com"},
		{"example.com.", "example.com"},
		{"  EXAMPLE.COM.  ", "example.com"},
		{"localhost", "localhost"},
		{"janus.test", "janus.test"},
	}
	for _, c := range cases {
		cfg, err := parseEnvFrom(map[string]string{"JANUS_PLATFORM_DOMAIN": c.in})
		if err != nil {
			t.Fatalf("parseEnvFrom(%q) 报错: %v", c.in, err)
		}
		if cfg.PlatformDomain != c.want {
			t.Errorf("parseEnvFrom(%q).PlatformDomain = %q, want %q", c.in, cfg.PlatformDomain, c.want)
		}
	}
}

func TestValidate_RejectsInvalidPlatformDomain(t *testing.T) {
	// 非空但格式非法 → fail-fast
	for _, bad := range []string{"exa mple.com", "-example.com", "example..com", "example.com/path", "a..b"} {
		cfg := validConfig()
		cfg.PlatformDomain = bad
		if err := cfg.Validate(); err == nil {
			t.Errorf("PlatformDomain=%q 应 fail-fast,但校验通过", bad)
		}
	}

	// 大小写/尾点这类"能归一化成合法值"的形态,校验也必须放行(归一化在 parseEnvFrom 做,
	// 但 Validate 是值接收者,允许调用方传未归一化的合法值)
	cfg := validConfig()
	cfg.PlatformDomain = "Example.com"
	if err := cfg.Validate(); err != nil {
		t.Errorf("合法但未归一化的 PlatformDomain 不该被拒: %v", err)
	}

	// 空值放行:直接构造 Config 的调用方(测试/SDK)不一定关心平台域名
	cfg = validConfig()
	cfg.PlatformDomain = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("空 PlatformDomain 应视为未配置而放行,got %v", err)
	}
}
