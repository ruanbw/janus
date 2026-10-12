package rules

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"janus/internal/store"
)

// RegexPredicate 预编译正则表达式断言算子。
type RegexPredicate struct {
	field string
	raw   []string
	res   []*regexp.Regexp
}

func (p *RegexPredicate) Type() string { return "regex" }
func (p *RegexPredicate) Cost() int    { return 20 } // 正则表达式计算成本较高，排在字符串之后

func (p *RegexPredicate) Match(ctx VisitorContext) bool {
	if ctx == nil {
		return false
	}
	if !isDynamicField(p.field) {
		raw, ok := ctx.Field(p.field)
		if !ok || raw == "" {
			return false
		}
		for _, re := range p.res {
			if re.MatchString(raw) {
				return true
			}
		}
		return false
	}
	return matchFieldValues(ctx, p.field, false, func(raw string) bool {
		for _, re := range p.res {
			if re.MatchString(raw) {
				return true
			}
		}
		return false
	})
}

func (p *RegexPredicate) Trace(ctx VisitorContext) ConditionTrace {
	actual, avail := fieldValuesText(ctx, p.field)
	matched := p.Match(ctx)
	tr := ConditionTrace{
		Field:     p.field,
		Operator:  OpRegex,
		Expected:  p.raw,
		Actual:    actual,
		Available: avail,
		Matched:   matched,
	}
	tr.Description = describeCond(&tr)
	return tr
}

func compileRegex(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
	if cond.Operator != OpRegex {
		return nil, false
	}
	var res []*regexp.Regexp
	for _, v := range cond.Values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		re, err := regexp.Compile(v)
		if err != nil {
			log.Warn("规则条件的正则表达式非法，丢弃该项", "field", cond.Field, "pattern", v, "err", err)
			continue
		}
		res = append(res, re)
	}
	if len(res) == 0 {
		return nil, false
	}
	return &RegexPredicate{
		field: cond.Field,
		raw:   cond.Values,
		res:   res,
	}, true
}

// NumericPredicate 数值大小比较断言算子 (gt / lt)。
type NumericPredicate struct {
	field string
	op    string
	raw   []string
	num   float64
}

func (p *NumericPredicate) Type() string { return "numeric" }
func (p *NumericPredicate) Cost() int    { return 2 }

func (p *NumericPredicate) Match(ctx VisitorContext) bool {
	if ctx == nil {
		return false
	}
	if !isDynamicField(p.field) {
		raw, ok := ctx.Field(p.field)
		if !ok || raw == "" {
			return false
		}
		return p.posMatch(raw)
	}
	return matchFieldValues(ctx, p.field, false, p.posMatch)
}

// posMatch 单个值的大小比较(非数值恒不命中)。
func (p *NumericPredicate) posMatch(raw string) bool {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return false
	}
	if p.op == OpGT {
		return f > p.num
	}
	return f < p.num
}

func (p *NumericPredicate) Trace(ctx VisitorContext) ConditionTrace {
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

func compileNumeric(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
	if cond.Operator != OpGT && cond.Operator != OpLT {
		return nil, false
	}
	if len(cond.Values) == 0 {
		return nil, false
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(cond.Values[0]), 64)
	if err != nil {
		log.Warn("数值条件阈值非数字", "field", cond.Field, "values", cond.Values, "err", err)
		return nil, false
	}
	return &NumericPredicate{
		field: cond.Field,
		op:    cond.Operator,
		raw:   cond.Values,
		num:   num,
	}, true
}

// DuplicatedPredicate 重复出现频次判定算子 (duplicated)。
type DuplicatedPredicate struct {
	field string
	raw   []string
	seen  int
}

func (p *DuplicatedPredicate) Type() string { return "duplicated" }
func (p *DuplicatedPredicate) Cost() int    { return 5 }

func (p *DuplicatedPredicate) Match(ctx VisitorContext) bool {
	val, ok := ctx.Field(p.field)
	if !ok || val == "" {
		return false
	}
	return seenCount(ctx, p.field, val) >= p.seen
}

func (p *DuplicatedPredicate) Trace(ctx VisitorContext) ConditionTrace {
	actual, avail := fieldValuesText(ctx, p.field)
	matched := p.Match(ctx)
	tr := ConditionTrace{
		Field:     p.field,
		Operator:  OpDuplicated,
		Expected:  p.raw,
		Actual:    actual,
		Available: avail,
		Matched:   matched,
		Seen:      seenCount(ctx, p.field, actual),
	}
	tr.Description = describeCond(&tr)
	return tr
}

func compileDuplicated(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
	if cond.Operator != OpDuplicated {
		return nil, false
	}
	if len(cond.Values) == 0 {
		return nil, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(cond.Values[0]))
	if err != nil || n < 1 {
		log.Warn("duplicated 重复频次不是正整数", "field", cond.Field, "values", cond.Values)
		return nil, false
	}
	return &DuplicatedPredicate{
		field: cond.Field,
		raw:   cond.Values,
		seen:  n,
	}, true
}
