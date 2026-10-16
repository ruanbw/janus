// Package config 从环境变量加载 Janus 服务配置。
// 使用业界公认、声明式的 github.com/caarlos0/env/v11 结构体标签解析。
package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"janus/internal/fqdn"
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

	// MigrationsDir 初始化脚本目录(内含唯一的 init.sql,见 AGENTS.md「数据库与 Schema 约定」)。
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
	return parseEnvFrom(env.ToMap(os.Environ()))
}

// LoadWithEnvFile 用「进程环境 + 指定配置文件(.env)」解析配置。
// 文件只补充进程环境里没有的键(与 docker compose / godotenv / viper 的约定一致:
// 真实环境变量优先,部署时可用环境覆盖文件);解析结果只落在返回的 Config 上,
// 不写回进程环境——库改 os.Setenv 会串味到同进程的其它 App 与测试。
func LoadWithEnvFile(path string) (Config, error) {
	fileVars, err := parseEnvFile(path)
	if err != nil {
		return Config{}, err
	}
	environ := env.ToMap(os.Environ())
	for key, value := range fileVars {
		if _, exists := environ[key]; !exists {
			environ[key] = value
		}
	}
	return parseEnvFrom(environ)
}

func parseEnvFrom(environment map[string]string) (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environment}); err != nil {
		return cfg, fmt.Errorf("无法解析环境变量(请检查 JANUS_* 变量的格式,例如时长必须写成 30h 而不是 30d): %w", err)
	}
	// 平台域名在这里归一化(Load/LoadWithEnvFile 的唯一公共漏斗)。此前归一化责任被
	// 下放给各消费点,于是同一份运维输入("Example.com" 或带尾点)会在三处得到不同形态:
	// store 原样入库 → 默认域名永远查不到;授权端点拿归一化值与裸配置值比较 → 永不命中
	// → 平台自己签不出证书;更新路径按裸值比对 → 0 行不报错。
	cfg.PlatformDomain = fqdn.Normalize(cfg.PlatformDomain)
	return cfg, nil
}

// parseEnvFile 解析 .env 风格配置文件:每行 KEY=VALUE,空行与 # 注释忽略,允许 export
// 前缀,值两侧的成对引号会被去掉。不认识的键(如给 compose 用的 JANUS_DB_*)留给别的
// 读取方;但缺少 = 的行直接报错,不静默丢键——静默丢键的结果是「配置看起来生效了,
// 实际没生效」。
func parseEnvFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	vars := make(map[string]string)
	for i, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("配置文件 %s 第 %d 行不是 KEY=VALUE: %q", path, i+1, rawLine)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		vars[key] = value
	}
	return vars, nil
}

// Validate 检查那些「配错就会静默毁数据或让进程 panic」的字段。
// Load 只保证「能解析」,不保证「语义合理」:0 或负的时长会让 time.NewTicker panic,
// 而 worker 跑在 goroutine 里——那类 panic 会终止整个进程,连带 HTTP 服务一起死。
// 所以这里返回错误,由调用方决定拒绝启动。
//
// 零值容忍:嵌入方(janus SDK)常只注入少数字段(如仅 Addr),其余留零值 —— 这不是
// "配了个 0",而是"没配置"。因此每组字段按"整块未配置则跳过,整块出现非零值则要求
// 全部为正"判定;只有非零值才会被校验正负,零值不再误伤轻量配置。
func (c Config) Validate() error {
	// runtimeDurations 是「运行期时长」块。整块为零 = 调用方未配置运行时,跳过;
	// 一旦块内出现非零值,即视为显式配置过,要求整块为正(非正时长会让 worker 的
	// time.NewTicker panic 并终止进程)。
	runtimeDurations := []struct {
		name string
		v    time.Duration
	}{
		{"JANUS_SESSION_TTL", c.SessionTTL},
		{"JANUS_SESSION_TTL_SHORT", c.SessionTTLShort},
		{"JANUS_VERIFY_TOKEN_TTL", c.VerifyTokenTTL},
		{"JANUS_RESET_TOKEN_TTL", c.ResetTokenTTL},
		{"JANUS_DNS_RETRY_INTERVAL", c.DNSRetryInterval},
		{"JANUS_DNS_MAX_AGE", c.DNSMaxAge},
		{"JANUS_VISIT_RETENTION", c.VisitRetention},
		{"JANUS_VISIT_CLEANUP_INTERVAL", c.VisitCleanupEvery},
	}
	if anyDurationNonZero(runtimeDurations) {
		for _, d := range runtimeDurations {
			if d.v <= 0 {
				return fmt.Errorf("%s 必须为正数,当前为 %v(非正时长会让后台 worker 的 time.NewTicker panic 并终止进程)", d.name, d.v)
			}
		}
	}
	// JWTTTL 单独判定:它是 SDK 覆盖的最小字段之一,常与其它节奏字段不同步,
	// 只要非零就必须为正。
	if c.JWTTTL < 0 {
		return fmt.Errorf("JANUS_JWT_TTL 必须为正数,当前为 %v", c.JWTTTL)
	}
	positiveInts := []struct {
		name string
		v    int
	}{
		{"JANUS_VISIT_CLEANUP_BATCH", c.VisitCleanupBatch},
		{"JANUS_LANDING_MAX_FILES", c.LandingMaxFiles},
		{"JANUS_MAX_TARGET_URLS", c.MaxTargetURLs},
	}
	if anyIntNonZero(positiveInts) {
		for _, i := range positiveInts {
			if i.v <= 0 {
				return fmt.Errorf("%s 必须为正数,当前为 %d", i.name, i.v)
			}
		}
	}
	positiveInt64s := []struct {
		name string
		v    int64
	}{
		{"JANUS_LANDING_MAX_ZIP_BYTES", c.LandingMaxZipBytes},
		{"JANUS_LANDING_MAX_TOTAL_BYTES", c.LandingMaxTotalBytes},
	}
	if anyInt64NonZero(positiveInt64s) {
		for _, i := range positiveInt64s {
			if i.v <= 0 {
				return fmt.Errorf("%s 必须为正数,当前为 %d", i.name, i.v)
			}
		}
	}
	if _, err := c.TrustedProxyNets(); err != nil {
		return fmt.Errorf("JANUS_TRUSTED_PROXY_CIDRS: %w", err)
	}
	// 平台域名 fail-fast:格式非法时启动即失败,而不是等到证书签发时才以"授权端点
	// 永不命中"的形式表现出来(那时排查成本高得多)。
	//
	// 空值放行(与本文件其它块"整块为零视为未配置"的约定一致):直接构造 Config 的
	// 调用方(测试、SDK)不一定关心平台域名,这不是一个"配错了"的输入。
	if c.PlatformDomain != "" && !fqdn.IsValid(c.PlatformDomain) {
		return fmt.Errorf("JANUS_PLATFORM_DOMAIN 不是合法域名(允许单标签,如 localhost): %q", c.PlatformDomain)
	}
	// SMTPPort = 0 表示未配置(envDefault 会填 465);非零时必须落在 1-65535。
	if c.SMTPPort != 0 && (c.SMTPPort < 1 || c.SMTPPort > 65535) {
		return fmt.Errorf("JANUS_SMTP_PORT 必须是 1-65535,当前为 %d", c.SMTPPort)
	}
	if c.PublicBaseURL != "" {
		if u, err := url.Parse(c.PublicBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("JANUS_PUBLIC_BASE_URL 不是合法 URL: %q", c.PublicBaseURL)
		}
	}
	if c.ServerPublicIP != "" && net.ParseIP(c.ServerPublicIP) == nil {
		return fmt.Errorf("JANUS_SERVER_PUBLIC_IP 不是合法 IP: %q", c.ServerPublicIP)
	}
	if c.JWTSecret == "" {
		// 为空时 httpapi 会退回每次重启随机的临时密钥(已签发 token 全部失效),
		// 生产环境必须显式配置——这里大声告警,不再静默。
		fmt.Fprintln(os.Stderr, "WARN: JANUS_JWT_SECRET 未配置,将使用每次重启随机的临时密钥,生产环境必须配置")
	} else if len(c.JWTSecret) < 32 {
		// 非空密钥必须够长:HS256 密钥强度直接决定 token 可被离线爆破的难度,
		// 短密钥(如猜得到的口令)会让签名形同虚设。空密钥的处置(生产模式拒绝启动)
		// 由另一条工作流负责,这里只校验"已配置的密钥"长度。
		return fmt.Errorf("JANUS_JWT_SECRET 至少需要 32 字节(HS256 密钥强度要求),当前为 %d 字节", len(c.JWTSecret))
	}
	if c.SMTPHost == "" {
		// 控制台 mailer 会把 token 打印到日志,生产环境不可用。
		fmt.Fprintln(os.Stderr, "WARN: 未配置 JANUS_SMTP_HOST,使用 ConsoleMailer,验证/重置链接会打印到后端日志,仅适合开发环境")
	}
	return nil
}

// anyDurationNonZero / anyIntNonZero / anyInt64NonZero 判定一组字段是否"整块未配置"。
// 用于 Validate 的零值容忍:整块为零即视为未配置并跳过校验。
func anyDurationNonZero(items []struct {
	name string
	v    time.Duration
}) bool {
	for _, it := range items {
		if it.v != 0 {
			return true
		}
	}
	return false
}

func anyIntNonZero(items []struct {
	name string
	v    int
}) bool {
	for _, it := range items {
		if it.v != 0 {
			return true
		}
	}
	return false
}

func anyInt64NonZero(items []struct {
	name string
	v    int64
}) bool {
	for _, it := range items {
		if it.v != 0 {
			return true
		}
	}
	return false
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
