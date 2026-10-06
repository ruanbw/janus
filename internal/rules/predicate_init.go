package rules

import (
	"log/slog"

	"janus/internal/store"
)

func init() {
	// 注册内置核心算子编译器
	// 1. IP / CIDR 算子
	RegisterPredicateCompiler(FieldIP, func(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
		// ip 字段在 in/eq/neq/not_in/in_cidr/not_in_cidr 下走高性能 Radix 树
		if pred, ok := compileIPRadix(cond, log); ok {
			return pred, true
		}
		// 如果是 ip 的 regex，转入 regex 编译器
		if pred, ok := compileRegex(cond, log); ok {
			return pred, true
		}
		// 其他情况走通用字符串匹配
		return compileStringPredicate(cond, log)
	})

	// 2. 注册兜底通用编译器：覆盖所有合法字段与运算符
	RegisterFallbackCompiler(func(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
		if !ValidField(cond.Field) || !ValidOperator(cond.Operator) {
			return nil, false
		}
		if pred, ok := compileRegex(cond, log); ok {
			return pred, true
		}
		if pred, ok := compileNumeric(cond, log); ok {
			return pred, true
		}
		if pred, ok := compileDuplicated(cond, log); ok {
			return pred, true
		}
		return compileStringPredicate(cond, log)
	})
}
