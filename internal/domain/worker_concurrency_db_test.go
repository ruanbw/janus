package domain

// 单轮 pass 的有界并发在真库上的行为测试(与 worker_bounded_test.go 的纯逻辑
// 测试互补:那边钉 runBounded 的语义,这边钉 pass 确实用上了它)。

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"janus/internal/store"
)

// 单轮 pass 必须并发处理整批,而不是一条一条地等。
//
// 顺序处理时,一轮 500 条的耗时无上界(dns-retry 最坏 ≈75 分钟、cert-probe
// ≈83 分钟),远超 5 分钟的轮询间隔,新 pending 域名永远排在队尾。这里用
// "同时在处理的域名条数"来钉这个不变量(不看耗时,避免负载相关的抖动):
// 顺序实现下它恒为 1。
func TestDNSRetryPassProcessesBatchConcurrently(t *testing.T) {
	st, pool := setupDomainStore(t)
	ctx := context.Background()
	tenantID := newActiveTenant(t, st, "retry-conc@test.io", "retry-conc")

	const n = 8
	expected := netip.MustParseAddr("203.0.113.7")
	var (
		inFlight    int64
		maxInFlight int64
	)
	note := func() {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
				break
			}
		}
		// 让同批的其它域名有机会同时在处理:没有这段等待,并发度会退化成 1,
		// 断言就成了"顺序实现也能通过"的假阳性。
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
	}

	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		fqdn := fmt.Sprintf("retry-conc-%d.test", i)
		d, err := st.CreateDomain(ctx, tenantID, fqdn, "self", "")
		if err != nil {
			t.Fatalf("create domain %s: %v", fqdn, err)
		}
		if err := st.MintVerifyToken(ctx, d.ID, "tok-"+fqdn); err != nil {
			t.Fatalf("mint token: %v", err)
		}
		execSQL(t, pool,
			`UPDATE domains SET dns_checked_at = now() - interval '6 minutes' WHERE id = $1`, d.ID)
		ids = append(ids, d.ID)
	}
	rows, err := st.ListDomainsForDNSCheck(ctx, testCfg().DNSMaxAge, testCfg().DNSRetryInterval, store.DefaultDomainScanLimit)
	if err != nil {
		t.Fatalf("list for dns check: %v", err)
	}
	for _, id := range ids {
		if findScanRowByID(rows, id) == nil {
			t.Fatalf("前置条件不成立:域名 %d 未进入重试队列", id)
		}
	}

	// A/AAAA 指向本机、TXT 不匹配:走到"记一次校验"为止,不触发出网 HTTPS 探活,
	// 让这条测试保持离线(探活另有单测)。
	checker := &DNSChecker{
		ExpectedIP: expected.String(),
		lookupAddr: func(context.Context, string) ([]netip.Addr, error) {
			note()
			return []netip.Addr{expected}, nil
		},
		lookupTXT: func(context.Context, string) ([]string, error) {
			note()
			return []string{"不匹配的 token"}, nil
		},
	}

	// When 跑一轮重试
	start := time.Now()
	workerWith(st, checker).dnsRetryPass(ctx)

	// Then 整批都被处理(每条都刷新了 dns_checked_at),且同时处理的条数 > 1
	for _, id := range ids {
		after, err := st.GetDomainByID(ctx, id)
		if err != nil {
			t.Fatalf("get domain %d: %v", id, err)
		}
		if after.Status != "pending" {
			t.Fatalf("域名 %d 状态被改成 %s, want pending", id, after.Status)
		}
		if checkedAt := queryTime(t, pool, `SELECT dns_checked_at FROM domains WHERE id = $1`, id); !checkedAt.After(start) {
			t.Fatalf("域名 %d 的 dns_checked_at(%v) 未在本轮刷新(round start %v):"+
				"整批没有全部被处理", id, checkedAt, start)
		}
	}
	got := atomic.LoadInt64(&maxInFlight)
	if got < 2 {
		t.Fatalf("同时处理的域名条数峰值为 %d:整批仍是顺序处理,"+
			"500 条一轮的耗时会超过轮询间隔", got)
	}
	// 上界写成字面量而不是引用 domainPassWorkers:这条断言要钉的是"出网并发
	// 有上限"这个契约,常量被改大时测试仍应拦下(而不是跟着一起放宽)。
	if got > 4 {
		t.Fatalf("同时处理的域名条数峰值为 %d, 超过有界并发上限 4", got)
	}
}
