package domain

// 归属校验(TXT 挑战)的纯逻辑单测。
//
// 不打真实 DNS:A/AAAA 与 TXT 都从注入的钩子取,断言的是"判定规则"本身 ——
// 有没有 token 才通过、token 一次性、A/AAAA 不可代替 TXT。

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

// fakeDNS 构造一个不依赖真实网络的 DNSChecker。
func fakeDNS(expectedIP string, addrs []netip.Addr, txt map[string][]string) *DNSChecker {
	return &DNSChecker{
		ExpectedIP: expectedIP,
		lookupAddr: func(context.Context, string) ([]netip.Addr, error) { return addrs, nil },
		lookupTXT: func(_ context.Context, name string) ([]string, error) {
			v, ok := txt[name]
			if !ok {
				return nil, errors.New("no such host")
			}
			return v, nil
		},
	}
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("parse addr %q: %v", s, err)
	}
	return a
}

func assertStatus(t *testing.T, got, want VerifyStatus) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %s, want %s", got, want)
	}
}

// 有 TXT token 且 A 记录指向本机 → 通过。
func TestVerifyPassesWithTXTToken(t *testing.T) {
	const token = "tok-123"
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")},
		map[string][]string{"_janus-verify.shop.customer.com": {token}})
	res, err := c.Verify(context.Background(), "shop.customer.com", token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyVerified)
	if !res.Verified() {
		t.Fatal("Verified() = false, want true")
	}
}

// 不持有 token(刚创建、或挑战已被消费)→ 不通过,且指引是"去加 TXT"。
func TestVerifyFailsWithoutToken(t *testing.T) {
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")},
		map[string][]string{"_janus-verify.shop.customer.com": {"whatever"}})
	res, err := c.Verify(context.Background(), "shop.customer.com", "")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyNeedTXT)
}

// token 不匹配 → 不通过(抄来的/过期的 token 都没用)。
func TestVerifyFailsOnWrongToken(t *testing.T) {
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")},
		map[string][]string{"_janus-verify.shop.customer.com": {"stale-token"}})
	res, err := c.Verify(context.Background(), "shop.customer.com", "fresh-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyNeedTXT)
}

// A/AAAA 指向本机但权威 DNS 里没有那条记录 → 不通过。
// 这正是问题 2 的核心场景:他人域名 CNAME 到本机、或攻击者在自己控制的域下
// 加一条 A 记录指向本机 IP,A 记录检查都会通过 —— 但归属没有被证明。
func TestVerifyNeedsTXTEvenWhenAPointsHere(t *testing.T) {
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")}, nil)
	res, err := c.Verify(context.Background(), "victim-customer.com", "tok-abc")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyNeedTXT)
	if !res.PointsToServer {
		t.Error("pointsToServer = false, want true")
	}
}

// A/AAAA 未指向本机 → 不通过,指引是"先加 A 记录"。
func TestVerifyNeedsDNSFirst(t *testing.T) {
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "198.51.100.1")}, nil)
	res, err := c.Verify(context.Background(), "elsewhere.com", "tok")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyNeedDNS)
}

// TXT 查询报错(NXDOMAIN / 超时)等同于"没有这条记录",不是系统错误。
func TestVerifyTreatsTXTFailureAsNotVerified(t *testing.T) {
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")}, nil)
	res, err := c.Verify(context.Background(), "shop.customer.com", "tok")
	if err != nil {
		t.Fatalf("Verify should not surface DNS errors: %v", err)
	}
	if res.Verified() {
		t.Fatal("verified = true, want false")
	}
}

// token 一次性:换发之后旧值不再对应当前挑战。
func TestVerifyTokenIsSingleUse(t *testing.T) {
	oldToken, err := MintVerifyToken()
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	newToken, err := MintVerifyToken()
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if oldToken == newToken {
		t.Fatal("两次签发得到同一个 token")
	}
	// 库里的挑战已换成 newToken,而 DNS 上还留着旧值 → 不能通过。
	c := fakeDNS("203.0.113.7", []netip.Addr{mustAddr(t, "203.0.113.7")},
		map[string][]string{"_janus-verify.shop.customer.com": {oldToken}})
	res, err := c.Verify(context.Background(), "shop.customer.com", newToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertStatus(t, res.Status, VerifyNeedTXT)
}

// TXT 查询名固定为 _janus-verify.<归一化 fqdn>。
func TestVerifyRecordName(t *testing.T) {
	if got := VerifyRecordName("Shop.Customer.COM."); got != "_janus-verify.shop.customer.com" {
		t.Fatalf("VerifyRecordName = %q", got)
	}
}

func TestNormalizeFQDN(t *testing.T) {
	cases := map[string]string{
		"  A.COM. ": "a.com",
		"a.com.":    "a.com",
		"A.com":     "a.com",
		"":          "",
		"..":        "",
	}
	for in, want := range cases {
		if got := NormalizeFQDN(in); got != want {
			t.Errorf("NormalizeFQDN(%q) = %q, want %q", in, got, want)
		}
	}
}

// 保留域名:等于平台域名或是其子域(问题 10)。
func TestIsReservedFQDN(t *testing.T) {
	cases := []struct {
		fqdn, platform string
		want           bool
	}{
		{"janus.test", "janus.test", true},
		{"app.janus.test", "janus.test", true},
		{"mail.janus.test", "janus.test", true},
		{"www.janus.test", "janus.test", true},
		{"deep.nested.janus.test", "janus.test", true},
		{"notjanus.test", "janus.test", false},
		{"janus.test.evil.com", "janus.test", false},
		{"customer.com", "janus.test", false},
		{"anything.com", "", false},
	}
	for _, tc := range cases {
		if got := IsReservedFQDN(tc.fqdn, tc.platform); got != tc.want {
			t.Errorf("IsReservedFQDN(%q, %q) = %v, want %v", tc.fqdn, tc.platform, got, tc.want)
		}
	}
}

func TestIsValidFQDN(t *testing.T) {
	ok := []string{"localhost", "example.com", "a.b.c.example.com", "xn--80ak6aa92e.com"}
	bad := []string{"", "-x.com", "x-.com", "bad_domain.com", "https://example.com", "a..com"}
	for _, s := range ok {
		if !IsValidFQDN(s) {
			t.Errorf("IsValidFQDN(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if IsValidFQDN(s) {
			t.Errorf("IsValidFQDN(%q) = true, want false", s)
		}
	}
}

func TestMintVerifyTokenShape(t *testing.T) {
	tok, err := MintVerifyToken()
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if len(tok) != VerifyTokenBytes*2 {
		t.Fatalf("token length = %d, want %d", len(tok), VerifyTokenBytes*2)
	}
}
