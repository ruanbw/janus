package rules

// 谓词算子(扁平形态的 ConditionPipeline)与 compiledCond(树形态)的等价性契约。
//
// 同一条叶子会走两条求值路径(接缝见 flat_tree_equivalence_test.go):
//   - 扁平形态(rules.logic + 一层 conditions):裁决走 ConditionPipeline 里的谓词算子;
//   - 树形态(等价的一层 all 组):裁决走 compiledCond。
//
// 本文件钉住 4 处历史语义分叉(全部按 compiledCond 侧对齐),外加前缀分发的确定性:
//  1. 数值空白:NumericPredicate 曾用 strconv.ParseFloat(raw) 而不去空白;
//  2. 大小写折叠:StringPredicate 曾用 strings.ToLower + == / Contains / HasPrefix / HasSuffix
//     (Unicode 全量小写 + 每次求值堆分配),compiledCond 用 ASCII 折叠 + 非 ASCII 回退 EqualFold;
//  3. duplicated 多值:谓词只取 ctx.Field 首值,compiledCond 任一取值达标即命中;
//  4. ip 精确编译器对 gt/lt/duplicated 返回 false 且不回落兜底,整条规则静默关闭 pipeline。
//
// 纯单元测试,不需要数据库。

import (
	"log/slog"
	"testing"

	"github.com/ruanbw/janus/internal/store"
)

// assertShapesAgree 对同一条件分别以扁平形态与等价树形态求值,断言两者结论一致,
// 并返回扁平形态的结论。同时钉住扁平形态真的建出了谓词流水线 —— 否则求值会回落到
// 树形态,两形态"一致"就成了废话,覆盖不到本文件要钉的分叉。
func assertShapesAgree(t *testing.T, cond store.RuleCondition, fact Fact) bool {
	t.Helper()
	flat := NewSnapshot([]store.Rule{oneRule(1, cond)}, discardLog)
	tree := NewSnapshot([]store.Rule{oneTreeRule(1, store.Group("all", store.Leaf(cond)))}, discardLog)
	if len(flat.Rules) != 1 || len(tree.Rules) != 1 {
		t.Fatalf("规则被整条丢弃:扁平 %d 条 / 树 %d 条(条件 %s %s %v)",
			len(flat.Rules), len(tree.Rules), cond.Field, cond.Operator, cond.Values)
	}
	if flat.Rules[0].pipeline == nil {
		t.Fatalf("扁平形态没建出谓词流水线,本用例覆盖不到两形态分叉(条件 %s %s %v)",
			cond.Field, cond.Operator, cond.Values)
	}
	f := fact
	_, flatMatched := flat.Evaluate(&f, 1)
	_, treeMatched := tree.Evaluate(&f, 1)
	if flatMatched != treeMatched {
		t.Errorf("两形态不一致:扁平=%v 树=%v(条件 %s %s %v,画像 %+v)",
			flatMatched, treeMatched, cond.Field, cond.Operator, cond.Values, f)
	}
	return flatMatched
}

// TestPredicateNumericWhitespaceAgreesAcrossShapes 钉住数值比较的空白口径。
//
// Given: Fact{UTM: " 42"} + 条件 utm gt 10。
// When:  扁平形态(谓词)与树形态(compiledCond)分别求值。
// Then:  两形态都必须为 true —— compiledCond 的 matchNumber 会 TrimSpace,
// 谓词侧必须同样去空白,否则 " 42" 在一边是 42、在另一边解析失败。
func TestPredicateNumericWhitespaceAgreesAcrossShapes(t *testing.T) {
	cond := store.RuleCondition{Field: FieldUTM, Operator: OpGT, Values: []string{"10"}}
	fact := Fact{UTM: " 42", Domain: "s.test"}
	if got := assertShapesAgree(t, cond, fact); !got {
		t.Errorf("utm gt 10 对 \" 42\" = false, want true(两侧都该 TrimSpace 后按 42 比较)")
	}
}

// TestPredicateNonASCIIFoldAgreesAcrossShapes 钉住大小写折叠口径。
//
// Given: Fact{Path: "/\u212A"}(KELVIN SIGN,Unicode 全量小写会折成 ASCII 的 'k',
// 但字节长度为 4,与 "/k" 的 2 不等)。
// When:  扁平形态(谓词)与树形态(compiledCond)分别求值 eq / contains / starts_with。
// Then:  两形态结论必须一致 —— 对齐后两者都只在 ASCII 上折叠、非 ASCII 走
// equalFoldASCII(它先比字节长度,长度不等直接 false),不再退化成
// strings.ToLower 的 Unicode 全量小写。
// 三个用例两形态都为 false:旧实现用 ToLower 会把 path 折成 "/k",eq 判成 true,
// 而 compiledCond 因字节长度不等恒为 false —— 正是本用例钉住的分叉。
func TestPredicateNonASCIIFoldAgreesAcrossShapes(t *testing.T) {
	const kelvinPath = "/\u212A" // "/K" with KELVIN SIGN U+212A
	fact := Fact{Path: kelvinPath, Domain: "s.test"}
	cases := []struct {
		name string
		cond store.RuleCondition
		want bool
	}{
		{"eq /k(字节长度不等,两形态都不命中)", store.RuleCondition{Field: FieldPath, Operator: OpEq, Values: []string{"/k"}}, false},
		{"contains k", store.RuleCondition{Field: FieldPath, Operator: OpContains, Values: []string{"k"}}, false},
		{"starts_with /k", store.RuleCondition{Field: FieldPath, Operator: OpStartsWith, Values: []string{"/k"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assertShapesAgree(t, tc.cond, fact); got != tc.want {
				t.Errorf("path %s %v 对 %q = %v, want %v",
					tc.cond.Operator, tc.cond.Values, kelvinPath, got, tc.want)
			}
		})
	}
}

// TestPredicateDuplicatedMultiValueAgreesAcrossShapes 钉住 duplicated 的多值口径。
//
// Given: X-Multi 同时带 alpha 与 beta,Seen 计数只落在 beta 上(5 次)。
// When:  条件 header:X-Multi duplicated 3 分别以两形态求值。
// Then:  两形态都必须为 true —— compiledCond 对多值字段是「任一取值计数达标即命中」,
// 谓词只取首值(alpha)会把画像判成不命中。
func TestPredicateDuplicatedMultiValueAgreesAcrossShapes(t *testing.T) {
	req := mustRequest(t, "https://s.test/p", "", "")
	req.Header.Add("X-Multi", "alpha")
	req.Header.Add("X-Multi", "beta")
	fact := FromRequest(req)
	fact.Seen = map[string]int{SeenKey("header:X-Multi", "beta"): 5}

	cond := store.RuleCondition{Field: "header:X-Multi", Operator: OpDuplicated, Values: []string{"3"}}
	if got := assertShapesAgree(t, cond, fact); !got {
		t.Errorf("header:X-Multi duplicated 3 = false, want true(beta 出现 5 次,任一取值达标即命中)")
	}
}

// TestPredicateIPNumericAndDuplicatedCompile 钉住 ip 精确编译器不再吞掉运算符。
//
// Given: ip 字段上的 gt / lt / duplicated 条件。
// When:  CompilePredicate 编译。
// Then:  必须成功 —— registry 命中精确编译器后不再回落兜底,这里返回 false 会让
// 整条规则静默关闭 pipeline、回落到树求值(两种形态结论分叉)。
func TestPredicateIPNumericAndDuplicatedCompile(t *testing.T) {
	for _, op := range []string{OpGT, OpLT, OpDuplicated} {
		t.Run(op, func(t *testing.T) {
			cond := store.ConditionLeafNode{Field: FieldIP, Operator: op, Values: []string{"1"}}
			pred, ok := CompilePredicate(cond, discardLog)
			if !ok || pred == nil {
				t.Fatalf("CompilePredicate(ip %s 1) 失败,期望编译成功", op)
			}
		})
	}

	// 编译得出来还不够:整条规则必须真的启用 pipeline(否则与树求值仍走两条路)。
	rule := oneRule(1,
		store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"US"}},
		store.RuleCondition{Field: FieldIP, Operator: OpGT, Values: []string{"1"}},
	)
	snap := NewSnapshot([]store.Rule{rule}, discardLog)
	if len(snap.Rules) != 1 || snap.Rules[0].pipeline == nil {
		t.Fatalf("ip gt 条件下扁平规则没有启用 pipeline,精确编译器仍在吞掉运算符")
	}
}

// prefixTagPredicate 一个只用于观测"哪个前缀编译器生效"的测试谓词。
type prefixTagPredicate struct{ tag string }

func (p *prefixTagPredicate) Type() string              { return p.tag }
func (p *prefixTagPredicate) Cost() int                 { return 1 }
func (p *prefixTagPredicate) Match(VisitorContext) bool { return false }
func (p *prefixTagPredicate) Trace(VisitorContext) ConditionTrace {
	return ConditionTrace{Field: p.tag, Operator: "test"}
}

// TestPredicateRegistryPrefixDispatchIsDeterministic 钉住前缀分发的确定性。
//
// Given: 注册两个都能命中 header:X-Det 的前缀编译器("h" 与 "header:")。
// When:  反复 CompilePredicate 同一条条件。
// Then:  结果必须始终来自最长前缀("header:")—— map 迭代顺序随机,
// 旧实现会随机命中"h"或"header:",编译结果不确定。
func TestPredicateRegistryPrefixDispatchIsDeterministic(t *testing.T) {
	RegisterPredicateCompilerPrefix("h", func(store.ConditionLeafNode, *slog.Logger) (ConditionPredicate, bool) {
		return &prefixTagPredicate{tag: "short"}, true
	})
	defer UnregisterPredicateCompilerPrefix("h")
	RegisterPredicateCompilerPrefix("header:", func(store.ConditionLeafNode, *slog.Logger) (ConditionPredicate, bool) {
		return &prefixTagPredicate{tag: "long"}, true
	})
	defer UnregisterPredicateCompilerPrefix("header:")

	cond := store.ConditionLeafNode{Field: "header:X-Det", Operator: OpEq, Values: []string{"v"}}
	for i := 0; i < 200; i++ {
		pred, ok := CompilePredicate(cond, discardLog)
		if !ok || pred == nil {
			t.Fatalf("第 %d 次 CompilePredicate 失败,期望命中前缀编译器", i)
		}
		if pred.Type() != "long" {
			t.Fatalf("第 %d 次编译命中了前缀 %q, want %q(必须取最长前缀)",
				i, pred.Type(), "long")
		}
	}
}
