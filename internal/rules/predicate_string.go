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

// Match 把字符串运算符套到字段的**全部**可用值上。
//
// 正向运算符(in/eq/contains/starts_with/ends_with):任一值满足即命中;
// 否定运算符(not_in/neq/not_contains):所有值都不满足正向形式才命中;
// 取不到任何值一律不命中(包括否定运算符)。
func (p *StringPredicate) Match(ctx VisitorContext) bool {
	if ctx == nil {
		return false
	}
	// 固定字段恒为单值:走 Field 的零分配快路径(谓词处于流水线热路径上)。
	if !isDynamicField(p.field) {
		raw, ok := ctx.Field(p.field)
		if !ok || raw == "" {
			return false
		}
		hit := p.posMatch(strings.ToLower(raw))
		if p.negated() {
			return !hit
		}
		return hit
	}
	return matchFieldValues(ctx, p.field, p.negated(), func(raw string) bool {
		return p.posMatch(strings.ToLower(raw))
	})
}

// negated 运算符是否为否定形式(多值语义需要看到全部值才取反,见 matchFieldValues)。
func (p *StringPredicate) negated() bool {
	return p.op == OpNeq || p.op == OpNotIn || p.op == OpNotContains
}

// posMatch 运算符的"肯定"形式(已小写归一的字段值是否满足)。
// 否定运算符先算肯定形式,取反由 matchFieldValues 在多值层面统一处理。
func (p *StringPredicate) posMatch(val string) bool {
	switch p.op {
	case OpIn, OpNotIn:
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
	case OpEq, OpNeq:
		if len(p.lits) == 0 {
			return false
		}
		return val == p.lits[0]
	case OpContains, OpNotContains:
		for _, lit := range p.lits {
			if strings.Contains(val, lit) {
				return true
			}
		}
		return false
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
	actual, avail := fieldValuesText(ctx, p.field)
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
