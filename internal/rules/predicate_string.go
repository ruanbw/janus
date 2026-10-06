package rules

import (
	"log/slog"
	"strings"

	"janus/internal/store"
)

// StringPredicate 专注于字符串常规运算符（in, not_in, eq, neq, contains, not_contains, starts_with, ends_with）。
type StringPredicate struct {
	field  string
	op     string
	raw    []string
	lits   []string            // 归一化后的小写字面量
	litMap map[string]struct{} // in / not_in 预编译 map (O(1) 匹配)
}

func (p *StringPredicate) Type() string { return "string_op" }
func (p *StringPredicate) Cost() int {
	switch p.op {
	case OpEq, OpNeq:
		return 1
	case OpIn, OpNotIn:
		return 2
	default:
		return 3
	}
}

func (p *StringPredicate) Match(ctx VisitorContext) bool {
	val, ok := ctx.Field(p.field)
	if !ok || val == "" {
		// 关键不变式: 取不到值一律恒不命中(包括 not_in/neq/not_contains，缺失值不能当成"不等于")
		return false
	}
	val = strings.ToLower(val)

	switch p.op {
	case OpIn:
		if p.litMap != nil {
			_, hit := p.litMap[val]
			return hit
		}
		for _, lit := range p.lits {
			if val == lit {
				return true
			}
		}
		return false
	case OpNotIn:
		if p.litMap != nil {
			_, hit := p.litMap[val]
			return !hit
		}
		for _, lit := range p.lits {
			if val == lit {
				return false
			}
		}
		return true
	case OpEq:
		if len(p.lits) == 0 {
			return false
		}
		return val == p.lits[0]
	case OpNeq:
		if len(p.lits) == 0 {
			return false
		}
		return val != p.lits[0]
	case OpContains:
		for _, lit := range p.lits {
			if strings.Contains(val, lit) {
				return true
			}
		}
		return false
	case OpNotContains:
		for _, lit := range p.lits {
			if strings.Contains(val, lit) {
				return false
			}
		}
		return true
	case OpStartsWith:
		for _, lit := range p.lits {
			if strings.HasPrefix(val, lit) {
				return true
			}
		}
		return false
	case OpEndsWith:
		for _, lit := range p.lits {
			if strings.HasSuffix(val, lit) {
				return true
			}
		}
		return false
	}
	return false
}

func (p *StringPredicate) Trace(ctx VisitorContext) ConditionTrace {
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

// compileStringPredicate 编译通用字符串谓词。
func compileStringPredicate(cond store.ConditionLeafNode, _ *slog.Logger) (ConditionPredicate, bool) {
	switch cond.Operator {
	case OpIn, OpNotIn, OpEq, OpNeq, OpContains, OpNotContains, OpStartsWith, OpEndsWith:
		lits := make([]string, 0, len(cond.Values))
		for _, v := range cond.Values {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				lits = append(lits, strings.ToLower(trimmed))
			}
		}
		if len(lits) == 0 {
			return nil, false
		}

		pred := &StringPredicate{
			field: cond.Field,
			op:    cond.Operator,
			raw:   cond.Values,
			lits:  lits,
		}
		if cond.Operator == OpIn || cond.Operator == OpNotIn {
			pred.litMap = make(map[string]struct{}, len(lits))
			for _, lit := range lits {
				pred.litMap[lit] = struct{}{}
			}
		}
		return pred, true
	}
	return nil, false
}
