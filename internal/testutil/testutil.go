// Package testutil 提供黑盒 HTTP 测试基础设施:
// 运行中的服务(httptest.Server)+ 真实 Postgres + 真实迁移,测试 seam 为 HTTP API 边界。
// 不 mock 内部函数;测试需本地 Postgres(默认 janus_test 库,见 JANUS_TEST_DATABASE_URL)。
package testutil

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"janus/internal/config"
	"janus/internal/db"
	"janus/internal/domain"
	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/mailer"
	"janus/internal/store"
)

// TestDatabaseURL 测试库连接串(与开发库分离,每次 Setup 清空业务表)。
var TestDatabaseURL = getenv("JANUS_TEST_DATABASE_URL",
	"postgres://janus:janus@localhost:5432/janus_test?sslmode=disable")

// PlatformDomain 测试用平台域名(janus.test,与开发一致)。
const PlatformDomain = "janus.test"

// TestCaddyAskToken 测试用 Caddy on-demand TLS 回调共享密钥。
// 生产环境由 JANUS_CADDY_ASK_TOKEN 配置;该端点在未配置时 fail-closed,
// 测试必须显式带上它(见 httpapi.CaddyAskTokenHeader)。
const TestCaddyAskToken = "test-caddy-ask-token"

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
// 测试库不可用时跳过(需先 docker compose up postgres 并创建 janus_test 库)。
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
		if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
			t.Fatalf("test database not available: %v", err)
		}
		t.Skipf("test database not available (%v); set JANUS_TEST_DB_REQUIRED=1 to fail instead of skip", err)
	}
	// 先注册回收再做任何校验：下面任何一步 Fatal 都会把连接池漏在进程里。
	// 上百个用例各自漏一个池，max_connections 会被很快耗光，
	// 后面的用例卡在 db.Connect 的重试循环里，整包表现成“跑了 10 分钟然后超时”，
	// 真实病因（某个前置校验没过）会被彻底掩盖。
	t.Cleanup(pool.Close)
	if !strings.Contains(TestDatabaseURL, "_test") {
		t.Fatalf("refusing to run tests against non-test database: %s", TestDatabaseURL)
	}
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
		Addr:                 ":0",
		DatabaseURL:          TestDatabaseURL,
		PlatformDomain:       PlatformDomain,
		ServerPublicIP:       "127.0.0.1",
		SuperadminEmail:      "",
		CookieSecure:         false,
		SessionTTL:           30 * 24 * time.Hour,
		SessionTTLShort:      24 * time.Hour,
		VerifyTokenTTL:       24 * time.Hour,
		ResetTokenTTL:        time.Hour,
		JWTSecret:            "test-jwt-secret",
		JWTTTL:               24 * time.Hour,
		CaddyAskToken:        TestCaddyAskToken,
		DNSRetryInterval:     50 * time.Millisecond,
		DNSMaxAge:            72 * time.Hour,
		VisitRetention:       90 * 24 * time.Hour,
		VisitCleanupEvery:    50 * time.Millisecond,
		VisitCleanupBatch:    500,
		LandingUploadDir:     filepath.Join(t.TempDir(), "uploads"),
		LandingMaxZipBytes:   10 << 20,
		LandingMaxTotalBytes: 10 << 20,
		LandingMaxFiles:      500,
		MaxTargetURLs:        50,
		MigrationsDir:        migrationsDir,
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
	m := mailer.NewMailer(mailer.Config{BaseURL: "https://app.janus.test"}, mailBuf)
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
		DomainOwnership: newFakeOwnership(),
	}))
	t.Cleanup(srv.Close)

	return &Env{Server: srv, Pool: pool, Store: st, Cfg: cfg, mail: mailBuf}
}

// MailOutput 返回控制台 mailer 的全部输出(用于黑盒提取验证/重置 token)。
func (e *Env) MailOutput() string { return e.mail.String() }

// fakeOwnership 是域名归属校验的测试替身。
//
// 生产实现靠「权威 DNS 上发布一次性 TXT」证明域名归属,并用 A/AAAA 确认指向本机。
// 黑盒测试既无法在权威 DNS 上发布 TXT,也不该依赖外网解析结果(同 geo.Disabled 的
// 理由:外部数据的准确性不该由单元测试负责)。
//
// 但它不是"一律通过"——那样会掩盖真实的判定分支。这里忠实复刻生产语义:
//   - localhost / *.localhost 解析到本机(JANUS_SERVER_PUBLIC_IP=127.0.0.1)→ 可激活;
//     刚创建还没有 token 时是 need_txt,拿到 token 后才 verified。
//   - 其他名字(如 .invalid)解析不到 → need_dns,域名停在 pending 进入重试队列。
//
// TXT 的比对逻辑本身(归一化、常量时间比较、一次性消费)在 internal/domain 的
// 单测里用假解析服务器覆盖,那里才是它的正确测试位置。
type fakeOwnership struct {
	tasks *domain.TaskQueue
}

func newFakeOwnership() *fakeOwnership {
	return &fakeOwnership{tasks: domain.NewTaskQueue(0, 0)}
}

func (f *fakeOwnership) Tasks() *domain.TaskQueue { return f.tasks }

func (f *fakeOwnership) Verify(_ context.Context, fqdn, token string) (domain.VerifyResult, error) {
	host := domain.NormalizeFQDN(fqdn)
	if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
		return domain.VerifyResult{Status: domain.VerifyNeedDNS}, nil
	}
	if token == "" {
		// 刚创建:已指向本机但还拿不到挑战 token,租户需要去 DNS 发布 TXT。
		return domain.VerifyResult{Status: domain.VerifyNeedTXT, PointsToServer: true}, nil
	}
	return domain.VerifyResult{
		Status:         domain.VerifyVerified,
		PointsToServer: true,
		TokenMatched:   true,
	}, nil
}

// StartWorker 启动后台任务(DNS 重试/证书探活/访问清理),测试结束时自动停止。
func (e *Env) StartWorker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		domain.NewWorker(e.Store, e.Cfg).Run(ctx)
	}()
	// 必须等 worker 真正退出再让下一个用例开始:它持有的会话级咨询锁会把别的
	// worker 全部挡掉(表现为 "咨询锁被其它副本持有,本轮跳过"),用例就会随机失败。
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("worker 未在 10s 内退出(通常卡在出网调用上)")
		}
	})
}

// BackdateDomain 把域名的 created_at、dns_checked_at 与 verify_token_created_at
// 一并回溯到 age 之前(模拟"过了 maxAge 仍未完成归属证明"的重试超时场景;测试数据准备)。
//
// 三个时间戳缺一不可,它们在后台扫描里各管一件事:
//   - dns_checked_at:决定这一轮要不要被选中复检(COALESCE(dns_checked_at, created_at));
//   - verify_token_created_at:决定是否超期(overdue → expired 终态);
//   - created_at:前两者的兜底。
//
// 只回溯其中一个,行要么压根不进扫描集,要么进了也不会被判超期。
func (e *Env) BackdateDomain(t *testing.T, domainID int64, age time.Duration) {
	t.Helper()
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE domains SET created_at = now() - $1::interval,
		                    dns_checked_at = now() - $1::interval,
		                    verify_token_created_at = now() - $1::interval
		 WHERE id=$2`, age, domainID); err != nil {
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
