package rules

import (
	"strings"
	"testing"

	"janus/internal/store"
)

// chainTree 造一棵 depth 层深的链式条件树:每层一个 all 组,最内层挂 leaf。
// 单叶子子组会在 compileNode 里被内联,所以这棵树编译后只剩一个 cond ——
// 结构上限必须按**原始** store 树判定,否则内联会把它藏起来。
func chainTree(depth int, leaf store.RuleCondition) store.ConditionNode {
	n := store.ConditionNode{Type: store.ConditionTypeLeaf, Leaf: &leaf}
	for i := 1; i < depth; i++ {
		n = store.ConditionNode{
			Type:  store.ConditionTypeGroup,
			Group: &store.ConditionGroupNode{Logic: store.RuleLogicAll, Children: []store.ConditionNode{n}},
		}
	}
	return n
}

// starTree 造一棵两层树:根组下挂 n 个叶子。
func starTree(n int, leaf store.RuleCondition) store.ConditionNode {
	children := make([]store.ConditionNode, 0, n)
	for i := 0; i < n; i++ {
		l := leaf
		children = append(children, store.ConditionNode{Type: store.ConditionTypeLeaf, Leaf: &l})
	}
	return store.ConditionNode{
		Type:  store.ConditionTypeGroup,
		Group: &store.ConditionGroupNode{Logic: store.RuleLogicAll, Children: children},
	}
}

// TestEngineRejectsOversizedConditionTrees 是引擎层"条件树深度 ≤ 8 / 叶子 ≤ 64"的回归测试。
//
// API 层(httpapi 的 validateConditionTree)已经拦住经接口写入的规则,但**直写库 /
// 导入**通道没有这道校验:那种规则同样会进快照、在跳转热路径上被逐条求值,
// 加载期的编译递归与求值递归也按树深走。引擎层因此需要自己一条防线 ——
// 超限整条规则不参与求值(与树里出现坏叶子同一处置),而不是截断或截深。
func TestEngineRejectsOversizedConditionTrees(t *testing.T) {
	leaf := store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"US"}}

	t.Run("树深度超限整条丢弃", func(t *testing.T) {
		log, buf := logBuf()
		snap := NewSnapshot([]store.Rule{oneTreeRule(1, chainTree(maxConditionTreeDepth+1, leaf))}, log)
		if len(snap.Rules) != 0 {
			t.Fatalf("深度 %d 的条件树不该进快照", maxConditionTreeDepth+1)
		}
		if !strings.Contains(buf.String(), "深度超过") {
			t.Fatalf("超限丢弃没有留痕:\n%s", buf.String())
		}
	})

	t.Run("树深度等于上限仍可用", func(t *testing.T) {
		snap := NewSnapshot([]store.Rule{oneTreeRule(1, chainTree(maxConditionTreeDepth, leaf))}, discardLog)
		if len(snap.Rules) != 1 {
			t.Fatalf("深度 %d 的条件树应正常进快照, got %d 条", maxConditionTreeDepth, len(snap.Rules))
		}
	})

	t.Run("树形态叶子数超限整条丢弃", func(t *testing.T) {
		log, buf := logBuf()
		snap := NewSnapshot([]store.Rule{oneTreeRule(1, starTree(maxConditionLeaves+1, leaf))}, log)
		if len(snap.Rules) != 0 {
			t.Fatalf("%d 个叶子的条件树不该进快照", maxConditionLeaves+1)
		}
		if !strings.Contains(buf.String(), "叶子数超过") {
			t.Fatalf("超限丢弃没有留痕:\n%s", buf.String())
		}
	})

	t.Run("树形态叶子数等于上限仍可用", func(t *testing.T) {
		snap := NewSnapshot([]store.Rule{oneTreeRule(1, starTree(maxConditionLeaves, leaf))}, discardLog)
		if len(snap.Rules) != 1 {
			t.Fatalf("%d 个叶子的条件树应正常进快照, got %d 条", maxConditionLeaves, len(snap.Rules))
		}
	})

	t.Run("扁平形态叶子数超限整条丢弃", func(t *testing.T) {
		conds := make([]store.RuleCondition, 0, maxConditionLeaves+1)
		for i := 0; i <= maxConditionLeaves; i++ {
			conds = append(conds, leaf)
		}
		log, buf := logBuf()
		snap := NewSnapshot([]store.Rule{oneRule(1, conds...)}, log)
		if len(snap.Rules) != 0 {
			t.Fatalf("%d 条扁平条件不该进快照", maxConditionLeaves+1)
		}
		if !strings.Contains(buf.String(), "叶子数超过") {
			t.Fatalf("超限丢弃没有留痕:\n%s", buf.String())
		}
	})

	t.Run("扁平形态叶子数等于上限仍可用", func(t *testing.T) {
		conds := make([]store.RuleCondition, 0, maxConditionLeaves)
		for i := 0; i < maxConditionLeaves; i++ {
			conds = append(conds, leaf)
		}
		snap := NewSnapshot([]store.Rule{oneRule(1, conds...)}, discardLog)
		if len(snap.Rules) != 1 {
			t.Fatalf("%d 条扁平条件应正常进快照, got %d 条", maxConditionLeaves, len(snap.Rules))
		}
	})
}
