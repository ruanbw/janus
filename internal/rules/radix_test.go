package rules

import (
	"net/netip"
	"testing"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", s, err)
	}
	return p
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("ParseAddr(%q): %v", s, err)
	}
	return a
}

// TestRadixExactAndPrefixMatch 前缀命中与落空都要准,IPv4/IPv6 各测一遍。
func TestRadixExactAndPrefixMatch(t *testing.T) {
	cases := []struct {
		name    string
		prefix  string
		addr    string
		want    bool
	}{
		{"v4 网段内", "10.0.0.0/8", "10.1.2.3", true},
		{"v4 网段外", "10.0.0.0/8", "11.0.0.1", false},
		{"v4 边界起始", "192.168.1.0/24", "192.168.1.0", true},
		{"v4 边界结束", "192.168.1.0/24", "192.168.1.255", true},
		{"v4 边界之外", "192.168.1.0/24", "192.168.2.1", false},
		{"v4 单点", "203.0.113.7/32", "203.0.113.7", true},
		{"v4 单点之外", "203.0.113.7/32", "203.0.113.8", false},
		{"v6 网段内", "2001:db8::/32", "2001:db8:1234::1", true},
		{"v6 网段外", "2001:db8::/32", "2001:db9::1", false},
		{"v6 单点", "2001:db8::1/128", "2001:db8::1", true},
		{"v6 单点之外", "2001:db8::1/128", "2001:db8::2", false},
		{"v4 配 /0 命中一切", "0.0.0.0/0", "8.8.8.8", true},
		{"v6 配 /0 命中一切", "::/0", "2001:db8::1", true},
		// 非字节对齐的掩码:逐位树天然支持,/12 的上界是 10.15.255.255
		{"非字节对齐网段内", "10.0.0.0/12", "10.9.1.1", true},
		{"非字节对齐网段外", "10.0.0.0/12", "10.16.0.1", false},
		// 未 Masked 的前缀:192.168.1.7/24 应等价于 192.168.1.0/24
		{"未掩码前缀网段内", "192.168.1.7/24", "192.168.1.9", true},
		{"未掩码前缀网段外", "192.168.1.7/24", "192.168.2.9", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewIPRadixTree([]netip.Prefix{mustPrefix(t, tc.prefix)})
			if got := tr.Contains(mustAddr(t, tc.addr)); got != tc.want {
				t.Fatalf("NewIPRadixTree(%s).Contains(%s) = %v, want %v", tc.prefix, tc.addr, got, tc.want)
			}
		})
	}
}

// TestRadixMultiPrefix 多个前缀共存:同族互不干扰,v4/v6 各走各的树。
func TestRadixMultiPrefix(t *testing.T) {
	tr := NewIPRadixTree([]netip.Prefix{
		mustPrefix(t, "10.0.0.0/8"),
		mustPrefix(t, "172.16.0.0/12"),
		mustPrefix(t, "192.168.0.0/16"),
		mustPrefix(t, "2001:db8::/32"),
	})
	cases := map[string]bool{
		"10.255.255.255": true,
		"11.0.0.0":       false,
		"172.16.5.5":     true,
		"172.32.0.1":     false,
		"192.168.42.1":   true,
		"2001:db8::99":   true,
		"2001:dba::99":   false,
		"::1":            false,
	}
	for addr, want := range cases {
		if got := tr.Contains(mustAddr(t, addr)); got != want {
			t.Fatalf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}
}

// TestRadixMappedIPv4Prefix v4-in-v6 前缀(::ffff:192.168.0.0/120)必须按 v4 网段处理,
// 且访客侧送来的映射地址(::ffff:192.168.1.5)要能命中同一棵树——双栈监听下
// IPv4 客户端就是这么过来的,不做 Unmap 就是整片 v4 网段失灵。
func TestRadixMappedIPv4Prefix(t *testing.T) {
	mapped := NewIPRadixTree([]netip.Prefix{mustPrefix(t, "::ffff:192.168.0.0/120")})
	if !mapped.Contains(mustAddr(t, "192.168.0.5")) {
		t.Fatal("映射前缀没有命中未映射的 v4 地址")
	}
	if !mapped.Contains(mustAddr(t, "::ffff:192.168.0.5")) {
		t.Fatal("映射前缀没有命中映射形态的访客地址")
	}
	if mapped.Contains(mustAddr(t, "192.168.1.5")) {
		t.Fatal("映射前缀命中了网段外地址")
	}

	native := NewIPRadixTree([]netip.Prefix{mustPrefix(t, "192.168.0.0/24")})
	if !native.Contains(mustAddr(t, "::ffff:192.168.0.5")) {
		t.Fatal("v4 网段没有命中映射形态的访客地址")
	}

	// 掩码短到吃进 ::ffff: 段的退化写法:整条前缀作废,不能变成"匹配一切"
	degenerate := NewIPRadixTree([]netip.Prefix{mustPrefix(t, "::/64")})
	if degenerate.Contains(mustAddr(t, "2001:db8::1")) {
		t.Fatal("v6 的 /64 命中了不相关的网段")
	}
}

// TestRadixDegenerate 空树 / 非法地址 / nil 树:一律不命中,绝不能 panic。
func TestRadixDegenerate(t *testing.T) {
	empty := NewIPRadixTree(nil)
	if empty.Contains(mustAddr(t, "10.0.0.1")) {
		t.Fatal("空树不该命中任何地址")
	}
	junk := NewIPRadixTree([]netip.Prefix{{}, mustPrefix(t, "10.0.0.0/8"), {}})
	if junk.Contains(mustAddr(t, "10.0.0.1")) != true {
		t.Fatal("非法前缀不该把合法前缀一起废掉")
	}
	if junk.Contains(netip.Addr{}) {
		t.Fatal("零值地址不该命中")
	}
	var nilTree *IPRadixTree
	if nilTree.Contains(mustAddr(t, "10.0.0.1")) {
		t.Fatal("nil 树不该命中")
	}
	if (*IPRadixTree)(nil).Contains(netip.Addr{}) {
		t.Fatal("nil 树 + 零值地址不该命中")
	}
}

// TestRadixDeduplicatesAndGrows 重复前缀与大量前缀都要能建树且不互相干扰。
func TestRadixDeduplicatesAndGrows(t *testing.T) {
	var ps []netip.Prefix
	for i := range 256 {
		ps = append(ps, mustPrefix(t, "10."+itoa(i)+".0.0/16"))
		ps = append(ps, mustPrefix(t, "10."+itoa(i)+".0.0/16")) // 故意重复
	}
	tr := NewIPRadixTree(ps)
	for i := range 256 {
		if !tr.Contains(mustAddr(t, "10."+itoa(i)+".7.7")) {
			t.Fatalf("10.%d.0.0/16 没命中自己的网段内地址", i)
		}
	}
	if tr.Contains(mustAddr(t, "11.0.0.1")) {
		t.Fatal("命中了所有网段之外的地址")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// BenchmarkRadixContains 网段条数多时查找耗时必须与条数无关(逐位下钻,32/128 步封顶)。
func BenchmarkRadixContains(b *testing.B) {
	var ps []netip.Prefix
	for i := range 1024 {
		ps = append(ps, netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(i >> 8), byte(i), 0}), 16))
	}
	tr := NewIPRadixTree(ps)
	addr := netip.AddrFrom4([4]byte{10, 0x03, 0x04, 0x05}) // 在 10.3.0.0/16 内
	if !tr.Contains(addr) {
		b.Fatal("前置条件不成立")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if !tr.Contains(addr) {
			b.Fatal("没命中")
		}
	}
}
