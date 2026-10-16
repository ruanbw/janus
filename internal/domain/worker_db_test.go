package domain

// 需要真实 Postgres 的 worker 测试(归属复检/重试 pass 的落地行为)。
//
// 为什么必须打真库:这几个 pass 的输入是 store 里的扫描 SQL(取哪些行)、输出是
// 状态迁移(active → failed 之类)。mock 掉 store 只会把 SQL 换成一个谎话,而
// 事故恰恰发生在"SQL 取出的行"和"worker 对它的处置"的组合上。
//
// 与 store 包的测试同一约定:独占测试库(各包都 TRUNCATE 同一批表,不互斥会
// 互相清数据)。测试库不可用时整体跳过。

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"janus/internal/config"
	"janus/internal/db"
	"janus/internal/store"
)

// setupDomainStore 连接测试库、执行迁移、清空业务表,返回 store 与直连池
// (池用于准备测试数据:回溯时间戳这类操作 store 没有对应 API)。
func setupDomainStore(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("JANUS_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://janus:janus@localhost:5432/janus_test_domain?sslmode=disable"
	}
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("test database not available (%v)", err)
	}
	t.Cleanup(pool.Close)
	release, err := db.LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(release)
	if err := db.Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE tenants, tiers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)`); err != nil {
		t.Fatalf("insert free tier: %v", err)
	}
	gdb, err := db.OpenGORM(url)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return store.New(gdb), pool
}

// newActiveTenant 建一个已验证(active)的租户:worker 的扫描 SQL 都带
// t.status = 'active',不激活的话任何一轮 pass 都取不到行。
func newActiveTenant(t *testing.T, s *store.Store, email, slug string) int64 {
	t.Helper()
	ctx := context.Background()
	tn, err := s.CreateTenant(ctx, email, "hash-"+email, slug, false)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := s.SetTenantStatus(ctx, tn.ID, "active"); err != nil {
		t.Fatalf("activate tenant: %v", err)
	}
	return tn.ID
}

func execSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// queryTime 读一个 timestamptz 列(store 的 Domain 视图没有 dns_checked_at 字段,
// 而它正是"这次失败有没有被算成一次校验"的证据)。
func queryTime(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) time.Time {
	t.Helper()
	var ts time.Time
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&ts); err != nil {
		t.Fatalf("query time (%s): %v", sql, err)
	}
	return ts
}

// dnsErrorChecker 构造一个"所有解析都失败"的校验器:模拟上游 DNS 不可达 /
// resolv.conf 不可读这类通道故障。
func dnsErrorChecker() *DNSChecker {
	return &DNSChecker{
		ExpectedIP: "203.0.113.7",
		lookupAddr: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("dns: 没有可用的 nameserver")
		},
		lookupTXT: func(context.Context, string) ([]string, error) {
			return nil, errors.New("dns: 没有可用的 nameserver")
		},
	}
}

func workerWith(st *store.Store, checker *DNSChecker) *Worker {
	return workerWithCfg(st, testCfg(), checker)
}

func workerWithCfg(st *store.Store, cfg config.Config, checker *DNSChecker) *Worker {
	return &Worker{store: st, cfg: cfg, dns: checker}
}

func findScanRowByID(rows []store.DomainScanRow, id int64) *store.DomainScanRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

// 归属复检遇到 DNS 查询失败时不得降级 active 域名。
//
// 事故形态:上游 DNS 抖一下 → LookupAddrs 报错 → Verify 返回 VerifyNeedDNS 且
// err == nil → recheck 判"归属不成立" → DegradeDomain → Caddy 授权端点拒绝 →
// 该租户全部短链立刻 404。整个租户因为一次 DNS 抖动停服。
func TestOwnershipRecheckKeepsActiveDomainOnDNSError(t *testing.T) {
	st, pool := setupDomainStore(t)
	ctx := context.Background()
	tenantID := newActiveTenant(t, st, "recheck-dnserr@test.io", "recheck-dnserr")

	// Given 一个已按 TXT 挑战激活、且本轮复检到期的 self 域名
	d, err := st.CreateDomain(ctx, tenantID, "recheck-dnserr.test", "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := st.MintVerifyToken(ctx, d.ID, "tok-recheck"); err != nil {
		t.Fatalf("mint token: %v", err)
	}
	if err := st.ActivateDomainVerified(ctx, d.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	execSQL(t, pool,
		`UPDATE domains SET ownership_verified_at = now() - interval '25 hours' WHERE id = $1`, d.ID)

	// 前置条件:本轮扫描确实会把它取出来(否则这条测试什么都没验证)
	rows, err := st.ListDomainsForOwnershipRecheck(ctx, OwnershipRecheckInterval, store.DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list for recheck: %v", err)
	}
	if findScanRowByID(rows, d.ID) == nil {
		t.Fatalf("前置条件不成立:到期的 active 域名未进入复检队列(取到 %d 行)", len(rows))
	}

	// When 上游 DNS 全部失败,跑一轮复检
	w := workerWith(st, dnsErrorChecker())
	w.ownershipRecheckPass(ctx)

	// Then 域名状态保持 active(不降级)
	after, err := st.GetDomainByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("get domain: %v", err)
	}
	if after.Status != "active" {
		t.Fatalf("DNS 查询失败被当成归属不成立:status = %s, want active"+
			"(降级会让 Caddy 授权端点拒绝,该租户全部短链 404)", after.Status)
	}
}

// 重试队列遇到 DNS 查询失败时同样不下结论:不改 status,也不刷新重试时钟。
func TestDNSRetryKeepsDomainOnDNSError(t *testing.T) {
	st, pool := setupDomainStore(t)
	ctx := context.Background()
	tenantID := newActiveTenant(t, st, "retry-dnserr@test.io", "retry-dnserr")

	// Given 一条带挑战、等待校验的 pending 域名(到期:6 分钟前校验过)
	d, err := st.CreateDomain(ctx, tenantID, "retry-dnserr.test", "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := st.MintVerifyToken(ctx, d.ID, "tok-retry"); err != nil {
		t.Fatalf("mint token: %v", err)
	}
	execSQL(t, pool,
		`UPDATE domains SET dns_checked_at = now() - interval '6 minutes' WHERE id = $1`, d.ID)
	beforeCheckedAt := queryTime(t, pool, `SELECT dns_checked_at FROM domains WHERE id = $1`, d.ID)

	rows, err := st.ListDomainsForDNSCheck(ctx, testCfg().DNSMaxAge, testCfg().DNSRetryInterval, store.DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list for dns check: %v", err)
	}
	if findScanRowByID(rows, d.ID) == nil {
		t.Fatalf("前置条件不成立:到期的 pending 域名未进入重试队列(取到 %d 行)", len(rows))
	}

	// When 上游 DNS 全部失败,跑一轮重试
	w := workerWith(st, dnsErrorChecker())
	w.dnsRetryPass(ctx)

	// Then 状态与重试时钟都不动(不写 MarkDomainDNSChecked、不置 expired)
	after, err := st.GetDomainByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("get domain: %v", err)
	}
	if after.Status != "pending" {
		t.Fatalf("DNS 查询失败改变了域名状态:status = %s, want pending", after.Status)
	}
	afterCheckedAt := queryTime(t, pool, `SELECT dns_checked_at FROM domains WHERE id = $1`, d.ID)
	if !afterCheckedAt.Equal(beforeCheckedAt) {
		t.Fatalf("DNS 通道故障被当成一次校验:dns_checked_at 从 %v 被刷新为 %v,"+
			"这次失败无谓地推后了重试节奏", beforeCheckedAt, afterCheckedAt)
	}
}
