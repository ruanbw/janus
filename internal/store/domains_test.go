package store

// 域名重试队列的过期判定测试:直接打真实 Postgres(不 mock store、不经过 httpapi)。
//
// 为什么必须在这一层:overdue 是 SQL 里的一个表达式,而它是 worker.dnsRetryPass 的
// 唯一入口判据 —— worker 在发出任何 DNS 查询之前就 `if d.Overdue { MarkDomainExpired }`。
// mock 掉 store 只会把这条 SQL 换成一句谎话,真实库才验证得了
// COALESCE(verify_token_created_at, created_at) 的回落行为。
//
// 沿用 setupStore 的测试库互斥约定,不引入 testutil(store 的测试不依赖 httpapi)。

import (
	"context"
	"testing"
	"time"
)

// createSelfDomainWithChallenge 走真实创建路径落一条 self 域名(带 TXT 挑战),
// 返回其 id。用于构造"挑战已签发"的历史,而不是手工拼一行数据。
func createSelfDomainWithChallenge(t *testing.T, s *Store, tenantID int64, fqdn, token string) int64 {
	t.Helper()
	d, err := CreateDomainChalleged(s.db.WithContext(context.Background()), tenantID, fqdn, token, "")
	if err != nil {
		t.Fatalf("create domain with challenge: %v", err)
	}
	return d.ID
}

// backdateDomainClock 回溯域名的时间戳(测试数据准备:测试里等不了 73 小时,
// 也等不了一个 5 分钟的重试间隔)。
func backdateDomainClock(t *testing.T, s *Store, domainID int64, sql string, args ...any) {
	t.Helper()
	if res := s.db.WithContext(context.Background()).Exec(sql, args...); res.Error != nil {
		t.Fatalf("backdate domain %d: %v", domainID, res.Error)
	}
}

// findScanRow 从扫描结果里取出指定域名的行(不存在返回 nil)。
func findScanRow(rows []DomainScanRow, id int64) *DomainScanRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

// TestDegradedDomainNotOverdueOnNextRetry 归属复检失败的域名降级到 failed 之后,
// 必须在下一轮重试里被重新尝试校验,而不是被判 overdue 直接打成 expired 终态。
//
// 契约:ListDomainsForDNSCheck(maxAge=72h, interval=5m) 返回的行里,这条降级域名
// 不得 overdue=true。
//
// 为什么这会踩坑:这条域名是"曾经 active"的 —— 激活时 ActivateDomainVerified 把
// verify_token_created_at 清空(挑战已被消费),而 DegradeDomain 只改
// status/cert_status/dns_checked_at。于是扫描 SQL 里的
// COALESCE(verify_token_created_at, created_at) 回落到 73 小时前的 created_at,
// 判成 overdue;worker 在任何 DNS 查询之前就 MarkDomainExpired。
// 结果是 store/worker 两处注释承诺的"failed 仍在重试队列里,租户修好 DNS 后自动恢复"
// 变成空话:域名在第一次复检失败后被静默打死,租户永远等不到自动恢复。
func TestDegradedDomainNotOverdueOnNextRetry(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "degrade-retry@test.io")

	// Given 一个 73 小时前创建并按 TXT 挑战激活、一直 active 到本轮复检失败才降级的 self 域名
	id := createSelfDomainWithChallenge(t, s, tenantID, "degrade-retry.test", "token-degrade-retry")
	if err := s.ActivateDomainVerified(ctx, id); err != nil {
		t.Fatalf("activate verified: %v", err)
	}
	if err := s.DegradeDomain(ctx, id); err != nil {
		t.Fatalf("degrade domain: %v", err)
	}
	// 真实时间线:73h 前创建并激活 → 一路 active(24h 一次的归属复检此前都通过,
	// 最近一次在 25h 前)→ 本轮复检到期、失败,6 分钟前降级。
	// 测试里等不了这么久,只能回溯时间戳(本包既有的时间回溯式数据准备)。
	// 注意三个时间戳都在:activated_at 与 created_at 一样是 73h 前 —— 这条域名
	// 已经 active 了很久,所以"拿 activated_at 当这次尝试的年龄"同样会把刚降级的
	// 域名判成超期;能表达"当前这次尝试挂了多久"的只有挑战签发时间。
	backdateDomainClock(t, s, id, `UPDATE domains SET
		created_at = now() - interval '73 hours',
		activated_at = now() - interval '73 hours',
		ownership_verified_at = now() - interval '25 hours',
		dns_checked_at = now() - interval '6 minutes'
		WHERE id = $1`, id)

	// 前置条件:降级已发生(status=failed)。verify_token_created_at 是保持 NULL
	// 还是被重置,属于实现自由 —— 契约只约束可观察结果:它必须仍在重试队列里,
	// 且不得被判超期。
	d, err := s.GetDomainByID(ctx, id)
	if err != nil {
		t.Fatalf("get domain: %v", err)
	}
	if d.Status != "failed" {
		t.Fatalf("前置条件不成立:降级后 status = %s, want failed", d.Status)
	}

	// When 下一轮重试扫描取到期域名(与 worker.dnsRetryPass 同一组参数)
	rows, err := s.ListDomainsForDNSCheck(ctx, 72*time.Hour, 5*time.Minute, DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list domains for dns check: %v", err)
	}

	// Then 它仍在重试队列里,且不得被判超期
	got := findScanRow(rows, id)
	if got == nil {
		t.Fatalf("降级后的域名未进入下一轮重试队列(扫描到 %d 行):租户修好 DNS 后无从自动恢复", len(rows))
	}
	if got.Overdue {
		t.Fatalf("降级后的域名在下一轮被判 overdue=true:worker 会在发 DNS 查询之前就 MarkDomainExpired," +
			"域名被静默打成终态(创建于 73h 前、挑战已在激活时消费,不能拿 created_at 当这次尝试的年龄)")
	}
}

// TestStaleChallengeStillOverdue 是上面那条契约的正向对照:过期判定本身必须还在生效。
//
// 契约:一条 73 小时前签发挑战、从未激活过的 pending 域名,必须被判 overdue=true
// (否则它永远达不到 expired 终态,超期后每轮仍被扫到、被无意义地反复校验与写库)。
//
// 没有这条对照,任何"把 overdue 规则整个拆掉/换成恒 false"的改动都能让上面那条
// 契约变绿 —— 那种改法只是把域名永久留在队列里,不是修复。
func TestStaleChallengeStillOverdue(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "stale-challenge@test.io")

	// Given 一个 73 小时前签发挑战、从未完成归属证明的 self 域名
	id := createSelfDomainWithChallenge(t, s, tenantID, "stale-challenge.test", "token-stale")
	backdateDomainClock(t, s, id, `UPDATE domains
		SET created_at = now() - interval '73 hours',
		    verify_token_created_at = now() - interval '73 hours',
		    dns_checked_at = now() - interval '6 minutes'
		WHERE id = $1`, id)

	// When 下一轮重试扫描取到期域名
	rows, err := s.ListDomainsForDNSCheck(ctx, 72*time.Hour, 5*time.Minute, DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list domains for dns check: %v", err)
	}

	// Then 这枚挂了 73 小时的挑战必须被判超期
	got := findScanRow(rows, id)
	if got == nil {
		t.Fatalf("超期域名未进入扫描集(扫描到 %d 行)", len(rows))
	}
	if !got.Overdue {
		t.Fatalf("挑战已挂 73 小时的域名未被判 overdue:它会永远留在重试队列里被反复校验写库,达不到 expired 终态")
	}
}
