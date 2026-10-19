package rules

import (
	"log/slog"
	"net/netip"
	"strings"

	"github.com/ruanbw/janus/internal/store"
)

// IPRadixPredicate 专注于 IP 地址及 CIDR 网段高效判定的原子谓词。
// 内部托付给已有的 IPRadixTree (基于 cidranger 路径压缩 Trie 树)。
type IPRadixPredicate struct {
	field   string
	op      string
	negated bool
	raw     []string
	radix   *IPRadixTree
}

func (p *IPRadixPredicate) Type() string { return "ip_radix" }
func (p *IPRadixPredicate) Cost() int    { return 5 } // Radix 树查询极快 (纳秒级)

// Match 判定访客 IP 是否落在预编译的 IP/CIDR 集合内。
// ctx == nil 与"取不到地址"同义:恒不命中,且否定运算符也不得因此翻成命中
// (对齐 StringPredicate / 数值 / 正则 / duplicated 三个谓词的同一道防御)。
func (p *IPRadixPredicate) Match(ctx VisitorContext) bool {
	if ctx == nil {
		return false
	}
	if p.radix == nil {
		return false
	}
	addr := ctx.ClientIP()
	if !addr.IsValid() {
		return false
	}
	hit := p.radix.Contains(addr)
	if p.negated {
		return !hit
	}
	return hit
}

func (p *IPRadixPredicate) Trace(ctx VisitorContext) ConditionTrace {
	actual, avail := ctx.Field(p.field)
	matched := p.Match(ctx)
	tr := ConditionTrace{
		Field:     p.field,
		Operator:  p.op,
		Expected:  p.raw,
		Actual:    actual,
		Available: avail,
		Matched:   matched,
	}
	tr.Description = describeCond(&tr)
	return tr
}

// compileIPRadix 编译 IP 字段的集合比较条件。
func compileIPRadix(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
	if cond.Field != FieldIP {
		return nil, false
	}
	switch cond.Operator {
	case OpIn, OpEq, OpNeq, OpNotIn, OpInCIDR, OpNotInCIDR:
		var prefixes []netip.Prefix
		for _, rawVal := range cond.Values {
			v := strings.TrimSpace(rawVal)
			if v == "" {
				continue
			}
			if p, err := netip.ParsePrefix(v); err == nil {
				prefixes = append(prefixes, p)
				continue
			}
			if a, err := netip.ParseAddr(v); err == nil {
				prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
				continue
			}
		}
		if len(prefixes) == 0 {
			log.Warn("ip 条件值既不是合法 IP 也不是合法 CIDR", "field", cond.Field, "values", cond.Values)
			return nil, false
		}
		tree := NewIPRadixTree(prefixes)
		negated := cond.Operator == OpNeq || cond.Operator == OpNotIn || cond.Operator == OpNotInCIDR
		return &IPRadixPredicate{
			field:   cond.Field,
			op:      cond.Operator,
			negated: negated,
			raw:     cond.Values,
			radix:   tree,
		}, true
	}
	return nil, false
}
