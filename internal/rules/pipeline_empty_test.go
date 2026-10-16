package rules

import (
	"testing"
)

// TestEmptyPipelineMatchesTreeSemantics 钉住"空条件组的求值语义只有一套"。
//
// 树求值里空组是明确的:all([]) = true(可配一条"无条件即执行"的规则),
// any([]) = false。而 ConditionPipeline.Match 原来在谓词切片为空时**无条件返回 true** ——
// 同一份"空条件组"在两条求值路径上给出相反结论:
//
//	p.Match(...)                 → true  (无论 logic)
//	compiledNode{op: all/any}.match(...) → all: true / any: false
//
// 空流水线目前进不去快照(compileRule 只在 kept > 0 时构造 pipeline),但
// ConditionPipeline 是导出类型,构造器也是导出的 —— 语义分叉不该靠"调用方不会那样用"兜着。
func TestEmptyPipelineMatchesTreeSemantics(t *testing.T) {
	fact := Fact{IP: "203.0.113.9"}

	for _, logicAll := range []bool{true, false} {
		// Given: 空的树节点与空的流水线(同一套 logic)
		tree := compiledNode{op: nodeOpFor(logicAll)}
		pipe := NewConditionPipeline(logicAll, nil)
		opt := NewOptimizedConditionPipeline(logicAll, nil)

		want := tree.match(fact)
		if got := pipe.Match(fact); got != want {
			t.Fatalf("logicAll=%v: 空流水线 Match = %v, 树求值 = %v —— 两套语义必须一致",
				logicAll, got, want)
		}
		if got := opt.Match(fact); got != want {
			t.Fatalf("logicAll=%v: 空优化流水线 Match = %v, 树求值 = %v", logicAll, got, want)
		}
	}

	// 把两端的期望值本身也钉住:空组在 all 下恒真(兜底规则),在 any 下恒假。
	if !NewConditionPipeline(true, nil).Match(fact) {
		t.Fatal("all([]) 应为 true:无条件的兜底规则必须仍然生效")
	}
	if NewConditionPipeline(false, nil).Match(fact) {
		t.Fatal("any([]) 应为 false:没有任何条件可满足的规则不该命中")
	}
}
