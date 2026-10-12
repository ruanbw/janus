package rules

import (
	"log/slog"
	"net"
	"net/netip"

	"github.com/yl2chen/cidranger"
)

// IPRadixTree 使用业界成熟的 cidranger 路径压缩 Trie 树实现 IPv4/IPv6 CIDR 前缀匹配。
//
// 保持与原有 API 完全一致（NewIPRadixTree, Contains），内部完全托付给
// 广泛使用的成熟开源库 github.com/yl2chen/cidranger。
type IPRadixTree struct {
	ranger cidranger.Ranger
}

// NewIPRadixTree 将一组 netip.Prefix 编译进 cidranger 路径压缩 Trie 树中。
// 自动处理 Masked 规范化以及 v4-in-v6 映射前缀展开，非法前缀自动跳过。
func NewIPRadixTree(prefixes []netip.Prefix) *IPRadixTree {
	ranger := cidranger.NewPCTrieRanger()
	for _, p := range prefixes {
		if !p.IsValid() {
			continue
		}
		addr, bits := prefixAddr(p)
		if !addr.IsValid() {
			continue
		}
		// 构造标准 *net.IPNet
		ip := net.IP(addr.AsSlice())
		mask := net.CIDRMask(bits, len(ip)*8)
		if mask == nil {
			continue
		}
		ipNet := &net.IPNet{
			IP:   ip.Mask(mask),
			Mask: mask,
		}
		if err := ranger.Insert(cidranger.NewBasicRangerEntry(*ipNet)); err != nil {
			// 单条前缀插不进去只应该让该网段失效,不能把整棵树废掉;
			// 静默吞掉又会让"规则里明明配了这条网段"变成一个查不到原因的恒不命中。
			slog.Warn("CIDR 前缀插入失败,该网段不参与匹配", "prefix", p.String(), "err", err)
		}
	}
	return &IPRadixTree{ranger: ranger}
}

// prefixAddr 把前缀拆成「未映射的规范地址 + 在该地址上的掩码长度」。
//
// 先 Masked 再判 Is4In6:Masked 会把掩码短于 96 位的映射前缀里那 16 位 0xffff 掩掉
// (::ffff:1.2.3.4/95 → ::fffe:0:0/95),于是它不再是 v4-in-v6 映射地址,
// 按普通 v6 前缀处理 —— 不存在"掩码短到吃进 ::ffff: 段的 v4 前缀"这种形态。
// 掩码不短于 96 位的映射前缀才是「v4 网段的另一种写法」,
// 展开成未映射的 v4 地址 + 减去 96 的位数。
func prefixAddr(p netip.Prefix) (netip.Addr, int) {
	p = p.Masked()
	if p.Addr().Is4In6() {
		return p.Addr().Unmap(), p.Bits() - 96
	}
	return p.Addr(), p.Bits()
}

// Contains 判断访客地址是否落在任一已登记的前缀内。
// 访客地址在双栈监听下可能是 ::ffff:a.b.c.d,先 Unmap 再查以确保 v4 与 v6 均可命中。
//
// 查找代价是 O(位数)(v4 32 步 / v6 128 步封顶,与网段条数无关),
// 但**不是零分配**:实测每次查询 2 次小分配、8 B/op
// (net.IP(addr.AsSlice()) 逃逸 + cidranger 构造 NetworkNumber;
// 见 BenchmarkRadixContains -benchmem)。这是查询路径上固定的小常数,
// 不值得为它重写 trie —— 逐位下钻的语义才是要保留的。
func (t *IPRadixTree) Contains(addr netip.Addr) bool {
	if t == nil || t.ranger == nil || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	ip := net.IP(addr.AsSlice())
	contained, err := t.ranger.Contains(ip)
	if err != nil {
		return false
	}
	return contained
}
