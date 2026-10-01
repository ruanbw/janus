// Package config 从环境变量加载 CLOAK 服务配置。
// 使用业界公认、声明式的 github.com/caarlos0/env/v11 结构体标签解析。
package config

import (
	"log"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Addr            string `env:"CLOAK_ADDR" envDefault:":8080"`
	DatabaseURL     string `env:"CLOAK_DATABASE_URL" envDefault:"postgres://cloak:cloak@localhost:5432/cloak?sslmode=disable"`
	PlatformDomain  string `env:"CLOAK_PLATFORM_DOMAIN" envDefault:"cloak.test"`
	ServerPublicIP  string `env:"CLOAK_SERVER_PUBLIC_IP" envDefault:"127.0.0.1"`
	SuperadminEmail string `env:"CLOAK_SUPERADMIN_EMAIL"`

	CookieSecure bool `env:"CLOAK_COOKIE_SECURE" envDefault:"true"`

	SessionTTL      time.Duration `env:"CLOAK_SESSION_TTL" envDefault:"720h"`
	SessionTTLShort time.Duration `env:"CLOAK_SESSION_TTL_SHORT" envDefault:"24h"`
	VerifyTokenTTL  time.Duration `env:"CLOAK_VERIFY_TOKEN_TTL" envDefault:"24h"`
	ResetTokenTTL   time.Duration `env:"CLOAK_RESET_TOKEN_TTL" envDefault:"1h"`
	JWTSecret       string        `env:"CLOAK_JWT_SECRET"`
	JWTTTL          time.Duration `env:"CLOAK_JWT_TTL" envDefault:"24h"`

	DNSRetryInterval  time.Duration `env:"CLOAK_DNS_RETRY_INTERVAL" envDefault:"5m"`
	DNSMaxAge         time.Duration `env:"CLOAK_DNS_MAX_AGE" envDefault:"72h"`
	VisitRetention    time.Duration `env:"CLOAK_VISIT_RETENTION" envDefault:"2160h"`
	VisitCleanupEvery time.Duration `env:"CLOAK_VISIT_CLEANUP_INTERVAL" envDefault:"24h"`

	LandingUploadDir   string `env:"CLOAK_LANDING_UPLOAD_DIR" envDefault:"uploads"`
	LandingMaxZipBytes int64  `env:"CLOAK_LANDING_MAX_ZIP_BYTES" envDefault:"10485760"`
	LandingMaxFiles    int    `env:"CLOAK_LANDING_MAX_FILES" envDefault:"500"`

	MigrationsDir string `env:"CLOAK_MIGRATIONS_DIR" envDefault:"migrations"`

	// SMTP 邮件(可选):配置后启用真实邮件发送,否则控制台 mailer
	PublicBaseURL string `env:"CLOAK_PUBLIC_BASE_URL" envDefault:"https://app.cloak.test"`
	SMTPHost      string `env:"CLOAK_SMTP_HOST"`
	SMTPPort      int    `env:"CLOAK_SMTP_PORT" envDefault:"465"`
	SMTPUsername  string `env:"CLOAK_SMTP_USERNAME"`
	SMTPPassword  string `env:"CLOAK_SMTP_PASSWORD"`
	SMTPFrom      string `env:"CLOAK_SMTP_FROM"`
}

func Load() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Printf("config.Load: failed to parse environment variables: %v", err)
	}
	return cfg
}
