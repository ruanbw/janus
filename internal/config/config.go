// Package config 从环境变量加载 CLOAK 服务配置。
// 环境差异全部由配置承载(见 spec 决策 #14):开发默认值可直接使用。
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr            string // HTTP 监听地址
	DatabaseURL     string
	PlatformDomain  string // 平台域名,如 cloak.test
	ServerPublicIP  string // 本服务器公网 IP,DNS 激活校验比对地址
	SuperadminEmail string // 超管邮箱(环境变量指定,启动时初始化)

	CookieSecure bool // 会话 cookie 的 Secure 标记;开发(http)置 false

	SessionTTL      time.Duration // 登录会话默认有效期(rememberMe=true/省略)
	SessionTTLShort time.Duration // rememberMe=false 时更短的会话有效期
	VerifyTokenTTL  time.Duration // 邮箱验证 token 有效期
	ResetTokenTTL   time.Duration // 密码重置 token 有效期
	JWTSecret       string        // JWT 签名密钥(生产必填;留空则每次启动随机生成,重启后已签发 token 失效)
	JWTTTL          time.Duration // JWT 访问 token 有效期(API Bearer 认证)

	DNSRetryInterval  time.Duration // DNS 校验重试间隔
	DNSMaxAge         time.Duration // DNS 校验最长重试时长(超时置 failed)
	VisitRetention    time.Duration // 访问记录保留时长
	VisitCleanupEvery time.Duration // 访问记录清理任务间隔

	MigrationsDir string // SQL 迁移文件目录

	// SMTP 邮件(可选):配置后启用真实邮件发送,否则控制台 mailer(spec 决策 #3)
	PublicBaseURL string // 后台访问地址,邮件链接前缀(如 https://app.cloak.test)
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func Load() Config {
	return Config{
		Addr:            getenv("CLOAK_ADDR", ":8080"),
		DatabaseURL:     getenv("CLOAK_DATABASE_URL", "postgres://cloak:cloak@localhost:5432/cloak?sslmode=disable"),
		PlatformDomain:  getenv("CLOAK_PLATFORM_DOMAIN", "cloak.test"),
		ServerPublicIP:  getenv("CLOAK_SERVER_PUBLIC_IP", "127.0.0.1"),
		SuperadminEmail: os.Getenv("CLOAK_SUPERADMIN_EMAIL"),
		CookieSecure:    getbool("CLOAK_COOKIE_SECURE", true),

		SessionTTL:      getdur("CLOAK_SESSION_TTL", 30*24*time.Hour),
		SessionTTLShort: getdur("CLOAK_SESSION_TTL_SHORT", 24*time.Hour),
		VerifyTokenTTL:  getdur("CLOAK_VERIFY_TOKEN_TTL", 24*time.Hour),
		ResetTokenTTL:   getdur("CLOAK_RESET_TOKEN_TTL", time.Hour),
		JWTSecret:       os.Getenv("CLOAK_JWT_SECRET"),
		JWTTTL:          getdur("CLOAK_JWT_TTL", 24*time.Hour),

		DNSRetryInterval:  getdur("CLOAK_DNS_RETRY_INTERVAL", 5*time.Minute),
		DNSMaxAge:         getdur("CLOAK_DNS_MAX_AGE", 72*time.Hour),
		VisitRetention:    getdur("CLOAK_VISIT_RETENTION", 90*24*time.Hour),
		VisitCleanupEvery: getdur("CLOAK_VISIT_CLEANUP_INTERVAL", 24*time.Hour),

		MigrationsDir: getenv("CLOAK_MIGRATIONS_DIR", "migrations"),

		PublicBaseURL: getenv("CLOAK_PUBLIC_BASE_URL", "https://app.cloak.test"), // 与 .env.example/开发 compose 一致(dev Caddy 映射宿主 443/80)
		SMTPHost:      os.Getenv("CLOAK_SMTP_HOST"),
		SMTPPort:      getint("CLOAK_SMTP_PORT", 465),
		SMTPUsername:  os.Getenv("CLOAK_SMTP_USERNAME"),
		SMTPPassword:  os.Getenv("CLOAK_SMTP_PASSWORD"),
		SMTPFrom:      os.Getenv("CLOAK_SMTP_FROM"),
	}
}

func getint(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
