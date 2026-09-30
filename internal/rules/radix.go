package rules

import "net/netip"

// IPRadixTree 是 IPv4/IPv6 的位前缀树(CIDR → 访客地址的判定)。
//
// 为什么不直接扫 []*net.IPNet:一条规则的 ip 条件可能有几十条网段,
// 跳转热路径(spec D7)上每条 ip 条件都线性扫一遍是纯浪费——前缀树把
// "访客地址是否落在任一网段内"变成一趟定长的逐位下钻(v4 32 步、v6 128 步),
// 与网段条数无关。
//
// 节点存在一个连续切片里(下标代替指针):构建期一次性分配,
// 求值期只有下标运算,零分配、cache 友好。
type IPRadixTree struct {
	nodes []radixNode
	// 两个根的节点下标。分开建树而不是混在一棵:v4/v6 的位数不同,
	// 混在一起还得在每步判断"这是第几位",不如各走各的。
	v4Root int32
	v6Root int32
}

// radixNode 一个位节点。zero/one 是下一位为 0/1 的子节点下标,radixNone 表示不存在。
type radixNode struct {
	zero, one int32
	terminal  bool // 该节点对应的前缀已登记(根节点 terminal = 0.0.0.0/0,匹配一切)
}

// radixNone 表示"没有这个子节点"。用 -1 而不是 0:0 是根节点的下标。
const radixNone int32 = -1

// NewIPRadixTree 把 CIDR 前缀编译成位前缀树。非法/未映射的地址被忽略(调用方
// 应在编译期就已过滤,这里是最后一道防线);前缀会被 Masked 成规范形态,
// 免得 "192.168.1.7/24" 这种写法在树上走出一个永远匹配不上的分叉。
//
// 返回值恒非 nil:没有可用前缀时得到的是一棵空树,Contains 恒为 false。
func NewIPRadixTree(prefixes []netip.Prefix) *IPRadixTree {
	t := &IPRadixTree{v4Root: radixNone, v6Root: radixNone}
	for _, p := range prefixes {
		if !p.IsValid() {
			continue
		}
		t.insert(p)
	}
	return t
}

// insert 把一条前缀登记进树。只在加载期调用(编译规则时),允许分配。
func (t *IPRadixTree) insert(p netip.Prefix) {
	if !p.IsValid() {
		return
	}
	addr, bits := prefixAddr(p)
	if !addr.IsValid() {
		return
	}
	var (
		root int32
		buf  []byte
	)
	if addr.Is4() {
		a4 := addr.As4()
		buf = a4[:]
		root = t.ensureRoot4()
	} else {
		a16 := addr.As16()
		buf = a16[:]
		root = t.ensureRoot6()
	}
	cur := root
	for i := 0; i < bits; i++ {
		cur = t.child(cur, bitAt(buf, i))
	}
	t.nodes[cur].terminal = true
}

// ensureRoot4 / ensureRoot6 惰性建根。空树(没有可用前缀)的根是 radixNone,
// Contains 一进来就返回 false。
func (t *IPRadixTree) ensureRoot4() int32 {
	if t.v4Root == radixNone {
		t.v4Root = t.newNode()
	}
	return t.v4Root
}

func (t *IPRadixTree) ensureRoot6() int32 {
	if t.v6Root == radixNone {
		t.v6Root = t.newNode()
	}
	return t.v6Root
}

func (t *IPRadixTree) newNode() int32 {
	t.nodes = append(t.nodes, radixNode{zero: radixNone, one: radixNone})
	return int32(len(t.nodes) - 1)
}

// child 返回 cur 节点下 b 位的子节点,不存在就新建。
func (t *IPRadixTree) child(cur int32, b uint8) int32 {
	if b == 0 {
		if n := t.nodes[cur].zero; n != radixNone {
			return n
		}
	} else {
		if n := t.nodes[cur].one; n != radixNone {
			return n
		}
	}
	idx := t.newNode() // append 可能搬家:必须在 append 之后重新写回下标
	if b == 0 {
		t.nodes[cur].zero = idx
	} else {
		t.nodes[cur].one = idx
	}
	return idx
}

// prefixAddr 把前缀拆成「未映射的地址 + 在该地址上的掩码长度」。
// 掩码长度必须跟着 Unmap 一起改:::ffff:192.168.0.0/120 的掩码长度是 120,
// 直接套到 192.168.0.0 上就是一条错到离谱的 "IPv4 /120"。
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

// Contains 判断访客地址是否落在任一已登记的前缀内。求值期调用:零分配。
//
// 未映射(v4-in-v6)地址先 Unmap 再查——访客地址来自 TCP 连接,
// IPv4 客户端在双栈监听上会是 ::ffff:a.b.c.d,不做 Unmap 就会查不中 v4 网段。
func (t *IPRadixTree) Contains(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if t == nil {
		return false
	}
	var (
		cur  int32
		bits int
		buf  []byte
	)
	if addr.Is4() {
		cur = t.v4Root
		bits = 32
		a4 := addr.As4()
		buf = a4[:]
	} else {
		cur = t.v6Root
		bits = 128
		a16 := addr.As16()
		buf = a16[:]
	}
	// 每一步先看当前节点:前缀可以在任意位结束(含 /0)。
	for i := 0; ; i++ {
		if cur == radixNone {
			return false
		}
		if t.nodes[cur].terminal {
			return true
		}
		if i == bits {
			return false
		}
		if bitAt(buf, i) == 0 {
			cur = t.nodes[cur].zero
		} else {
			cur = t.nodes[cur].one
		}
	}
}

// bitAt 取地址第 i 位(自高位起)。i 越界返回 0——调用方已经用 bits 限住了循环。
func bitAt(b []byte, i int) uint8 {
	if i>>3 >= len(b) {
		return 0
	}
	return (b[i>>3] >> (7 - uint(i&7))) & 1
}
