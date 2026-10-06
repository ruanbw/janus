package rules

// ConditionPredicate 规则条件的原子断言接口。
// 每一个规则项（IP段、国家、UA正则、自定义风控分等）都是实现了该接口的独立结构体。
type ConditionPredicate interface {
	// Type 算子类型标识（如 "ip_radix", "string_in", "regex", "custom_header"）
	Type() string

	// Cost 评估计算成本（用于流水线排序短路优化：纳秒级排前，正则/外部计算排后）
	Cost() int

	// Match 线上热路径执行匹配（纯内存、零锁、fail-open）
	Match(ctx VisitorContext) bool

	// Trace 供规则模拟器使用，返回该条件在当前访客下的诊断推演数据
	Trace(ctx VisitorContext) ConditionTrace
}
