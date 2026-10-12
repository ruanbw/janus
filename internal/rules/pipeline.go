package rules

import (
	"sort"
)

// ConditionPipeline 条件流水线求值器。
// 内部持有按序排列的 ConditionPredicate 切片，支持 AND / OR 短路求值与自适应排序。
type ConditionPipeline struct {
	logicAll   bool
	predicates []ConditionPredicate
}

// NewConditionPipeline 构造基础条件流水线。
func NewConditionPipeline(logicAll bool, predicates []ConditionPredicate) *ConditionPipeline {
	preds := make([]ConditionPredicate, len(predicates))
	copy(preds, predicates)
	return &ConditionPipeline{
		logicAll:   logicAll,
		predicates: preds,
	}
}

// NewOptimizedConditionPipeline 构造按执行成本（Cost）升序排序的优化流水线。
//
// 按 Cost 升序是 AND 模式的最优启发式：低开销算子排在前面，短路失败越早，
// 总代价越低。OR 模式并非如此——OR 在首个 true 时短路，最优排序应让**最可能命中**
// 的算子先跑（按命中率降序），而不是按 Cost 升序；这里只做排序启发式，
// 不改变裁决结果（AND 全真才真、OR 任一真即真，与顺序无关）。
//
// 排序会改变 Predicates() 返回的顺序（供诊断/测试观察），但不会改变 Match 的结论。
func NewOptimizedConditionPipeline(logicAll bool, predicates []ConditionPredicate) *ConditionPipeline {
	preds := make([]ConditionPredicate, len(predicates))
	copy(preds, predicates)
	// 稳定排序：开销相同保持原有配置顺序
	sort.SliceStable(preds, func(i, j int) bool {
		return preds[i].Cost() < preds[j].Cost()
	})
	return &ConditionPipeline{
		logicAll:   logicAll,
		predicates: preds,
	}
}

// Match 执行热路径求值（带短路特性）。
func (p *ConditionPipeline) Match(ctx VisitorContext) bool {
	if len(p.predicates) == 0 {
		return true
	}
	if p.logicAll {
		// AND 模式：首个 false 即退出
		for _, pred := range p.predicates {
			if !pred.Match(ctx) {
				return false
			}
		}
		return true
	}
	// OR 模式：首个 true 即退出
	for _, pred := range p.predicates {
		if pred.Match(ctx) {
			return true
		}
	}
	return false
}

// Trace 遍历整条流水线（不短路）并收集所有算子的诊断轨迹，供单测与诊断工具使用。
// 线上规则模拟器走的是 compiledCond 的统一留痕(snapshot.go 的 collectTraces),
// 不经过这里 —— 单一留痕实现,避免两套语义分叉。
func (p *ConditionPipeline) Trace(ctx VisitorContext) []ConditionTrace {
	traces := make([]ConditionTrace, 0, len(p.predicates))
	for _, pred := range p.predicates {
		traces = append(traces, pred.Trace(ctx))
	}
	return traces
}

// Predicates 返回流水线内部的算子切片快照。
func (p *ConditionPipeline) Predicates() []ConditionPredicate {
	out := make([]ConditionPredicate, len(p.predicates))
	copy(out, p.predicates)
	return out
}
