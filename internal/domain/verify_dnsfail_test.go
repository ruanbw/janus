package domain

// Verify 对 DNS 通道故障的判定:必须是"不确定",不能退化成"没指向本机"。
//
// 这两个结论在下游天差地别:VerifyNeedDNS 表示"归属不成立"(复检会降级域名,
// Caddy 授权端点立刻拒绝,该租户全部短链 404),而渠道故障只是"这一轮问不到"。

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

// LookupAddrs 报错 → 不确定态(不是 need_dns)。
func TestVerifyReportsDNSUnavailableOnAddrLookupError(t *testing.T) {
	c := &DNSChecker{
		ExpectedIP: "203.0.113.7",
		lookupAddr: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("dns: 没有可用的 nameserver")
		},
	}
	// Given 有 token(否则会先被 token 分支短路)
	// When A/AAAA 查询本身失败
	res, err := c.Verify(context.Background(), "shop.customer.com", "tok-1")
	// Then 得到"本轮无结论",既不是"没指向本机"也不是"已验证"
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Status != VerifyDNSUnavailable {
		t.Fatalf("status = %s, want %s(查询失败不能被当成归属不成立)", res.Status, VerifyDNSUnavailable)
	}
	if res.Status == VerifyNeedDNS {
		t.Fatal("查询失败退化成了 need_dns:下游会据此降级域名")
	}
	if res.Verified() || res.PointsToServer {
		t.Fatalf("查询失败时 Verified=%v PointsToServer=%v,二者都应为 false", res.Verified(), res.PointsToServer)
	}
}

// 对照组:查得到、但没指向本机 → 仍是 need_dns(这条语义不能被上面的修复带走)。
func TestVerifyStillReportsNeedDNSWhenLookupSucceeds(t *testing.T) {
	c := &DNSChecker{
		ExpectedIP: "203.0.113.7",
		lookupAddr: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("198.51.100.1")}, nil
		},
	}
	res, err := c.Verify(context.Background(), "shop.customer.com", "tok-1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Status != VerifyNeedDNS {
		t.Fatalf("status = %s, want %s", res.Status, VerifyNeedDNS)
	}
}
