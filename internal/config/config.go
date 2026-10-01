// Package config 从环境变量加载 Janus 服务配置。
// 使用业界公认、声明式的 github.com/caarlos0/env/v11 结构体标签解析。
package config

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Addr            string `env:"JANUS_ADDR" envDefault:":8080"`
	DatabaseURL     string `env:"JANUS_DATABASE_URL" envDefault:"postgres://janus:janus@localhost:5432/janus?sslmode=disable"`
	PlatformDomain  string `env:"JANUS_PLATFORM_DOMAIN" envDefault:"janus.test"`
	ServerPublicIP  string `env:"JANUS_SERVER_PUBLIC_IP" envDefault:"127.0.0.1"`
	SuperadminEmail string `env:"JANUS_SUPERADMIN_EMAIL"`

	CookieSecure bool `env:"JANUS_COOKIE_SECURE" envDefault:"true"`

	SessionTTL      time.Duration `env:"JANUS_SESSION_TTL" envDefault:"720h"`
	SessionTTLShort time.Duration `env:"JANUS_SESSION_TTL_SHORT" envDefault:"24h"`
	VerifyTokenTTL  time.Duration `env:"JANUS_VERIFY_TOKEN_TTL" envDefault:"24h"`
	ResetTokenTTL   time.Duration `env:"JANUS_RESET_TOKEN_TTL" envDefault:"1h"`
	JWTSecret       string        `env:"JANUS_JWT_SECRET"`
	JWTTTL          time.Duration `env:"JANUS_JWT_TTL" envDefault:"24h"`

	// TrustedProxyCIDrs 显式声明「哪些对端可以被我采信 X-Forwarded-For」。
	// 为空 = 不采信任何来源的 XFF(只认 TCP 对端)。
	// 历史上实现是「对端是私网地址就采信」,这让任何能连到后端且源地址为私网的
	// 主机(即同网段的任何人,或任何多级代理)都能自带 XFF 伪造限流分桶与访问统计的
	// 来源 IP。判定可信代理必须是部署者显式声明的事实,不能从地址段推断。
	// 语法:逗号分隔的 IP 或 CIDR,如 "10.0.0.0/8,172.18.0.1,127.0.0.1/32"。
	TrustedProxyCIDrs []string `env:"JANUS_TRUSTED_PROXY_CIDRS" envSeparator:","`

	// CaddyAskToken Caddy on-demand TLS 回调 /internal/caddy/authorize 的共享密钥。
	// Caddy 侧以 header 传入,后端做常量时间比对。
	// 原来该端点只有"对端是私网地址"这一层判断:在 compose 网络里够用,但一旦
	// 后端被加上 ports: 映射,或前面再套一层反代(来源是内网 IP),判定恒真,
	// 无认证的 GET 就变成公网可调 —— 它能枚举出"哪些租户域名处于 active",
	// 且能引导 Caddy 为它们签发证书。IP 判定保留为第二层,不作为唯一防线。
	CaddyAskToken string `env:"JANUS_CADDY_ASK_TOKEN"`

	DNSRetryInterval  time.Duration `env:"JANUS_DNS_RETRY_INTERVAL" envDefault:"5m"`
	DNSMaxAge         time.Duration `env:"JANUS_DNS_MAX_AGE" envDefault:"72h"`
	VisitRetention    time.Duration `env:"JANUS_VISIT_RETENTION" envDefault:"2160h"`
	VisitCleanupEvery time.Duration `env:"JANUS_VISIT_CLEANUP_INTERVAL" envDefault:"24h"`
	// VisitCleanupBatch 单次清理最多删除的行数。0 = 不分批(不推荐)。
	// 不分批时一条 DELETE 会持有全部过期行的锁直到提交,高流量租户上是长事务。
	VisitCleanupBatch int `env:"JANUS_VISIT_CLEANUP_BATCH" envDefault:"10000"`

	LandingUploadDir   string `env:"JANUS_LANDING_UPLOAD_DIR" envDefault:"uploads"`
	LandingMaxZipBytes int64  `env:"JANUS_LANDING_MAX_ZIP_BYTES" envDefault:"10485760"`
	// LandingMaxTotalBytes 解压后所有文件的总大小上限。与 LandingMaxZipBytes 是两个
	// 语义不同的限制(上传包大小 vs 解压膨胀),合并成一个会让运维改一个连带改另一个。
	LandingMaxTotalBytes int64 `env:"JANUS_LANDING_MAX_TOTAL_BYTES" envDefault:"10485760"`
	LandingMaxFiles      int   `env:"JANUS_LANDING_MAX_FILES" envDefault:"500"`
	// MaxTargetURLs 一条短链可配置的目标 URL 个数上限(轮询池大小)。
	MaxTargetURLs int `env:"JANUS_MAX_TARGET_URLS" envDefault:"50"`

	MigrationsDir string `env:"JANUS_MIGRATIONS_DIR" envDefault:"migrations"`

	// SMTP 邮件(可选):配置后启用真实邮件发送,否则控制台 mailer
	PublicBaseURL string `env:"JANUS_PUBLIC_BASE_URL" envDefault:"https://app.janus.test"`
	SMTPHost      string `env:"JANUS_SMTP_HOST"`
	SMTPPort      int    `env:"JANUS_SMTP_PORT" envDefault:"465"`
	SMTPUsername  string `env:"JANUS_SMTP_USERNAME"`
	SMTPPassword  string `env:"JANUS_SMTP_PASSWORD"`
	SMTPFrom      string `env:"JANUS_SMTP_FROM"`
}

// Load 解析环境变量。解析失败直接返回错误,不再吞掉:
// env/v11 是逐字段聚合错误(成功字段已写入、失败字段留零值),继续启动等于带着
// 部分零值配置上线——例如把 JANUS_VISIT_RETENTION 写成 "90d"(Go duration 不认 d)
// 就会让保留期变 0,启动后第一趟清理直接删光全部访问记录。
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("无法解析环境变量(请检查 JANUS_* 变量的格式,例如时长必须写成 30h 而不是 30d): %w", err)
	}
	return cfg, nil
}

// Validate 检查那些「配错就会静默毁数据或让进程 panic」的字段。
// Load 只保证「能解析」,不保证「语义合理」:0 或负的时长会让 time.NewTicker panic,
// 而 worker 跑在 goroutine 里——那类 panic 会终止整个进程,连带 HTTP 服务一起死。
// 所以这里返回错误,由调用方决定拒绝启动。
func (c Config) Validate() error {
	positiveDurations := []struct {
		name string
		v    time.Duration
	}{
		{"JANUS_SESSION_TTL", c.SessionTTL},
		{"JANUS_SESSION_TTL_SHORT", c.SessionTTLShort},
		{"JANUS_VERIFY_TOKEN_TTL", c.VerifyTokenTTL},
		{"JANUS_RESET_TOKEN_TTL", c.ResetTokenTTL},
		{"JANUS_JWT_TTL", c.JWTTTL},
		{"JANUS_DNS_RETRY_INTERVAL", c.DNSRetryInterval},
		{"JANUS_DNS_MAX_AGE", c.DNSMaxAge},
		{"JANUS_VISIT_RETENTION", c.VisitRetention},
		{"JANUS_VISIT_CLEANUP_INTERVAL", c.VisitCleanupEvery},
	}
	for _, d := range positiveDurations {
		if d.v <= 0 {
			return fmt.Errorf("%s 必须为正数,当前为 %v(非正时长会让后台 worker 的 time.NewTicker panic 并终止进程)", d.name, d.v)
		}
	}
	positiveInts := []struct {
		name string
		v    int
	}{
		{"JANUS_VISIT_CLEANUP_BATCH", c.VisitCleanupBatch},
		{"JANUS_LANDING_MAX_FILES", c.LandingMaxFiles},
		{"JANUS_MAX_TARGET_URLS", c.MaxTargetURLs},
	}
	for _, i := range positiveInts {
		if i.v <= 0 {
			return fmt.Errorf("%s 必须为正数,当前为 %d", i.name, i.v)
		}
	}
	positiveInt64s := []struct {
		name string
		v    int64
	}{
		{"JANUS_LANDING_MAX_ZIP_BYTES", c.LandingMaxZipBytes},
		{"JANUS_LANDING_MAX_TOTAL_BYTES", c.LandingMaxTotalBytes},
	}
	for _, i := range positiveInt64s {
		if i.v <= 0 {
			return fmt.Errorf("%s 必须为正数,当前为 %d", i.name, i.v)
		}
	}
	if _, err := c.TrustedProxyNets(); err != nil {
		return fmt.Errorf("JANUS_TRUSTED_PROXY_CIDRS: %w", err)
	}
	return nil
}

// TrustedProxyNets 解析 TrustedProxyCIDrs。空的或写错的条目在 Validate 阶段报错,
// 这里只负责把已通过校验的配置变成可用的前缀集合。
func (c Config) TrustedProxyNets() ([]netip.Prefix, error) {
	nets := make([]netip.Prefix, 0, len(c.TrustedProxyCIDrs))
	for _, raw := range c.TrustedProxyCIDrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			nets = append(nets, p.Masked())
			continue
		}
		// 允许写单个 IP(等价 /32 或 /128)
		if a, err := netip.ParseAddr(s); err == nil {
			nets = append(nets, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		return nil, fmt.Errorf("无法解析 %q(需要 IP 或 CIDR,如 10.0.0.0/8)", s)
	}
	return nets, nil
}

// TrustedProxies 是预先解析好的可信代理前缀集合。
//
// 单独提供这个类型是为了让解析只发生一次:TrustedProxyNets() 每次调用都要重新
// 解析每个 CIDR,而「这个请求的对端是否可信」要在**每个**跳转请求上判断 ——
// 在热路径上反复解析是白烧 CPU。启动时构造一次,之后只做前缀匹配。
type TrustedProxies []netip.Prefix

// TrustedProxies 解析并缓存可信代理集合。空配置返回空的集合,
// 语义是「不采信任何来源的 X-Forwarded-For」,这是安全的默认值。
func (c Config) TrustedProxies() (TrustedProxies, error) {
	nets, err := c.TrustedProxyNets()
	if err != nil {
		return nil, err
	}
	return TrustedProxies(nets), nil
}

// Contains 判断某个来源地址是否落在可信代理网段内。
func (t TrustedProxies) Contains(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	// 与 CIDR 匹配口径一致:IPv4-mapped IPv6 归一到 IPv4 再比
	addr = addr.Unmap()
	for _, p := range t {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
