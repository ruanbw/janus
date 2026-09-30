// Package testutil 提供黑盒 HTTP 测试基础设施:
// 运行中的服务(httptest.Server)+ 真实 Postgres + 真实迁移,测试 seam 为 HTTP API 边界。
// 不 mock 内部函数;测试需本地 Postgres(默认 cloak_test 库,见 CLOAK_TEST_DATABASE_URL)。
package testutil

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cloak/internal/config"
	"cloak/internal/db"
	"cloak/internal/domain"
	"cloak/internal/geo"
	"cloak/internal/httpapi"
	"cloak/internal/mailer"
	"cloak/internal/store"
)

// TestDatabaseURL 测试库连接串(与开发库分离,每次 Setup 清空业务表)。
var TestDatabaseURL = getenv("CLOAK_TEST_DATABASE_URL",
	"postgres://cloak:cloak@localhost:5432/cloak_test?sslmode=disable")

// PlatformDomain 测试用平台域名(cloak.test,与开发一致)。
const PlatformDomain = "cloak.test"

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// syncBuffer 并发安全的邮件输出缓冲(黑盒读取验证 token)。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type Env struct {
	Server *httptest.Server
	Pool   *pgxpool.Pool
	Store  *store.Store
	Cfg    config.Config
	mail   *syncBuffer
}

// Setup 启动测试环境:连接测试库、执行迁移、清空业务表、启动 HTTP 服务。
// 测试库不可用时跳过(需先 docker compose up postgres 并创建 cloak_test 库)。
// 默认注入高阈值限流,避免通用测试被限流干扰;需要验证限流行为的测试用
// SetupWithRateLimit 自行控制。
func Setup(t *testing.T) *Env {
	return setup(t, nil, nil)
}

// SetupWithRateLimit 以自定义认证限流配置启动测试环境(用于限流黑盒测试)。
func SetupWithRateLimit(t *testing.T, rc httpapi.RateLimitConfig) *Env {
	return setup(t, &rc, nil)
}

// SetupWithGeo 注入自定义地理解析启动测试环境(用于国家/地区相关行为的黑盒测试)。
// 默认(Setup)关掉了地理解析,生产默认走内嵌的离线库。
func SetupWithGeo(t *testing.T, lookup geo.Lookup) *Env {
	return setup(t, nil, lookup)
}

func setup(t *testing.T, rc *httpapi.RateLimitConfig, geoLookup geo.Lookup) *Env {
	t.Helper()
	ctx := context.Background()

	pool, err := db.Connect(ctx, TestDatabaseURL)
	if err != nil {
		t.Skipf("test database not available (%v); run: docker compose up -d postgres && docker exec -i cloak-postgres-1 psql -U cloak -d cloak -c 'CREATE DATABASE cloak_test'", err)
	}
	t.Cleanup(pool.Close)
	// 独占测试库到本包测试结束:各包都连同一个库并 TRUNCATE 业务表,
	// 而 go test 并行跑各包,不加互斥会互相把对方的数据清掉
	release, err := db.LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(release)

	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := db.Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	truncateAll(t, pool)

	mailBuf := &syncBuffer{}
	cfg := config.Config{
		Addr:               ":0",
		DatabaseURL:        TestDatabaseURL,
		PlatformDomain:     PlatformDomain,
		ServerPublicIP:     "127.0.0.1",
		SuperadminEmail:    "",
		CookieSecure:       false,
		SessionTTL:         30 * 24 * time.Hour,
		SessionTTLShort:    24 * time.Hour,
		VerifyTokenTTL:     24 * time.Hour,
		ResetTokenTTL:      time.Hour,
		JWTSecret:          "test-jwt-secret",
		JWTTTL:             24 * time.Hour,
		DNSRetryInterval:   50 * time.Millisecond,
		DNSMaxAge:          72 * time.Hour,
		VisitRetention:     90 * 24 * time.Hour,
		VisitCleanupEvery:  50 * time.Millisecond,
		LandingUploadDir:   filepath.Join(t.TempDir(), "uploads"),
		LandingMaxZipBytes: 10 << 20,
		LandingMaxFiles:    500,
		MigrationsDir:      migrationsDir,
	}

	gdb, err := db.OpenGORM(TestDatabaseURL)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := gdb.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	st := store.New(gdb)
	m := mailer.NewMailer(mailer.Config{BaseURL: "https://app.cloak.test"}, mailBuf)
	if rc == nil {
		high := 100000
		rc = &httpapi.RateLimitConfig{
			RegisterLimit: high, RegisterWindow: time.Minute,
			AuthLimit: high, AuthWindow: time.Minute,
		}
	}
	// GeoLookup: 默认 geo.Disabled —— 黑盒测试关掉地理解析。两个原因:
	//   断言不该依赖外部数据集的准确性(某个网段哪天被重新划分就挂了),
	//   以及别让每个测试二进制都把 46MB 的离线库带上。
	//   要验证国家相关行为,用 SetupWithGeo 注入一个假 Lookup。
	if geoLookup == nil {
		geoLookup = geo.Disabled
	}
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store: st, Mailer: m, Cfg: cfg, RateLimit: rc, GeoLookup: geoLookup,
	}))
	t.Cleanup(srv.Close)

	return &Env{Server: srv, Pool: pool, Store: st, Cfg: cfg, mail: mailBuf}
}

// MailOutput 返回控制台 mailer 的全部输出(用于黑盒提取验证/重置 token)。
func (e *Env) MailOutput() string { return e.mail.String() }

// StartWorker 启动后台任务(DNS 重试/证书探活/访问清理),测试结束时自动停止。
func (e *Env) StartWorker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go domain.NewWorker(e.Store, e.Cfg).Run(ctx)
}

// BackdateDomain 把域名 created_at 改为 age 之前(模拟 72h 重试超时场景;测试数据准备)。
func (e *Env) BackdateDomain(t *testing.T, domainID int64, age time.Duration) {
	t.Helper()
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE domains SET created_at = now() - $1::interval WHERE id=$2`, age, domainID); err != nil {
		t.Fatalf("backdate domain: %v", err)
	}
}

// SetTierLimits 调整租户等级配额(测试数据准备,用于配额超限场景)。
func (e *Env) SetTierLimits(t *testing.T, tenantID int64, maxLinks, maxDomains int) {
	t.Helper()
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE tiers SET max_links=$1, max_domains=$2
		 WHERE id=(SELECT tier_id FROM tenants WHERE id=$3)`,
		maxLinks, maxDomains, tenantID); err != nil {
		t.Fatalf("set tier limits: %v", err)
	}
}

// Poll 轮询直到条件满足或超时(黑盒观察异步行为,如 worker 状态流转)。
func Poll(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// Ctx 返回测试用背景上下文。
func Ctx() context.Context { return context.Background() }

var tokenRe = regexp.MustCompile(`token: ([A-Za-z0-9_-]{40,})`)

// LastToken 从 mailer 输出提取最近一个 token(黑盒方式获取验证/重置 token)。
func (e *Env) LastToken(t *testing.T) string {
	t.Helper()
	all := tokenRe.FindAllStringSubmatch(e.MailOutput(), -1)
	if len(all) == 0 {
		t.Fatalf("no token found in mailer output: %s", e.MailOutput())
	}
	return all[len(all)-1][1]
}

// TruncateAll 清空业务表(保留 tiers 种子)。
func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE sessions, email_tokens, visits, link_domains, links, domains, tenants, tiers
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// 重新插入等级种子:免费档(短链 100 / 域名 10),新租户默认免费档
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)`); err != nil {
		t.Fatalf("insert free tier: %v", err)
	}
}
