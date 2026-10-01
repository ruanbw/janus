package rules

import (
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
		_ = ranger.Insert(cidranger.NewBasicRangerEntry(*ipNet))
	}
	return &IPRadixTree{ranger: ranger}
}

// prefixAddr 把前缀拆成「未映射的规范地址 + 在该地址上的掩码长度」。
func prefixAddr(p netip.Prefix) (netip.Addr, int) {
	p = p.Masked()
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			// 掩码短到连 v4 的位都吃进了 ::ffff: 这段里:不是一条有意义的 IPv4 网段
			return netip.Addr{}, 0
		}
		return p.Addr().Unmap(), p.Bits() - 96
	}
	return p.Addr(), p.Bits()
}

// Contains 判断访客地址是否落在任一已登记的前缀内。
// 访客地址在双栈监听下可能是 ::ffff:a.b.c.d，先 Unmap 再查以确保 v4 与 v6 均可命中。
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
