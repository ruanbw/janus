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
		// 集合运算符的值全非法时,compileCond(eval.go)会整条丢弃这个叶子;
		// 这里绝不能回落到字符串比较 —— CompilePredicate 是导出函数,插件/衍生
		// 工程可以直接调用,回落到字符串会让同一个叶子出现两套语义。
		switch cond.Operator {
		case OpIn, OpEq, OpNeq, OpNotIn, OpInCIDR, OpNotInCIDR:
			return nil, false
		}
		// 其他运算符按顺序依次尝试：正则 → 数值(gt/lt) → 重复(duplicated) → 通用字符串。
		// 不能直接 return false：registry 命中精确编译器后不再回落兜底编译器，
		// 而 compileCond 是接受 ip + gt/lt/duplicated 的。一旦这里返回 false，
		// 整条规则会静默关闭 pipeline、回落到树求值，两种形态结论分叉。
		if pred, ok := compileRegex(cond, log); ok {
			return pred, true
		}
		if pred, ok := compileNumeric(cond, log); ok {
			return pred, true
		}
		if pred, ok := compileDuplicated(cond, log); ok {
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
