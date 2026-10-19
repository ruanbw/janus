package rules

import (
	"log/slog"
	"strings"

	"github.com/ruanbw/janus/internal/store"
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
		hit := p.posMatch(raw)
		if p.negated() {
			return !hit
		}
		return hit
	}
	return matchFieldValues(ctx, p.field, p.negated(), p.posMatch)
}

// negated 运算符是否为否定形式(多值语义需要看到全部值才取反,见 matchFieldValues)。
func (p *StringPredicate) negated() bool {
	return p.op == OpNeq || p.op == OpNotIn || p.op == OpNotContains
}

// posMatch 运算符的"肯定"形式(raw 未做小写归一,折叠与 compiledCond 同一套 helper)。
// 否定运算符先算肯定形式,取反由 matchFieldValues 在多值层面统一处理。
//
// 折叠语义必须与 compiledCond 逐字一致:旧实现用 strings.ToLower(raw) + == / Contains /
// HasPrefix / HasSuffix,那会把非 ASCII 字符折叠成 ASCII(如 KELVIN SIGN → k),
// 而 compiledCond 只在 ASCII 上折叠、非 ASCII 回退 strings.EqualFold,两者结论不同;
// 且每次求值都为整串分配一份副本(扁平规则每次求值一次堆分配)。
func (p *StringPredicate) posMatch(raw string) bool {
	switch p.op {
	case OpIn, OpNotIn, OpEq, OpNeq:
		// eq/neq 与 in/not_in 同为「取值集合」语义:任一取值命中即肯定形式成立
		// (多值取值集合不能只看 lits[0],否则 eq/neq 会静默收窄为第一个取值)。
		// litMap 是小写归一后的精确命中快路径;未命中再按折叠语义线性比对。
		if p.litMap != nil {
			if _, hit := p.litMap[raw]; hit {
				return true
			}
		}
		for _, lit := range p.lits {
			if equalFoldASCII(raw, lit) {
				return true
			}
		}
		return false
	case OpContains, OpNotContains:
		for _, lit := range p.lits {
			if containsFold(raw, lit) {
				return true
			}
		}
		return false
	case OpStartsWith:
		for _, lit := range p.lits {
			if hasPrefixFold(raw, lit) {
				return true
			}
		}
		return false
	case OpEndsWith:
		for _, lit := range p.lits {
			if hasSuffixFold(raw, lit) {
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
