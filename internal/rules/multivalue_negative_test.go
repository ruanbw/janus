package rules

// 多值取值 + 否定运算符(neq / not_in / not_contains)的语义契约:唯一,且扁平形态与等价树形态一致。
//
// 权威语义(前端与本包既有契约两处同口径):
//   - web/src/views/rules/ruleMeta.ts 的 OPERATOR_OPTIONS 文案:
//     in「访客值落在任一取值内」/ eq「与取值完全相等」/ neq「与取值均不相等」/
//     not_in「in 的反面:任一取值命中即不成立」/ not_contains「不包含任一取值」;
//   - web/src/views/rules/ruleSim.ts evalCondition 的 'in'/'eq'/'neq'/'not_in' 分支:
//     anyHit = values.some(matchValue);eq/in = anyHit;neq/not_in = actual != "" && !anyHit;
//     'not_contains' 分支:values.every((v) => !containsValue(actual, v));
//   - internal/rules/fields.go matchFieldValues 的注释:多值字段上正向运算符任一值命中即命中,
//     否定运算符所有值都不满足正向形式才成立。
//
// 即:取值的语义 = 「字段值与**所有**列出的取值都不相等 / 都不匹配」(any-of 的否定)。
//
// 同一条条件在同一条规则上会走两条不同的求值路径(接缝见 flat_tree_equivalence_test.go):
//   - 扁平形态(rules.logic + 一层 conditions):求值走谓词流水线 StringPredicate;
//   - 树形态(等价的一层 all 组):求值走 compiledCond。
//
// 本文件钉住:两条路径对 ≥2 个取值的 eq / neq / not_in / not_contains 必须给出**同一个**结论,
// 且该结论必须符合上面的 any-of 语义 —— 只钉"两形态一致"会放任两者都错。
//
// 纯单元测试,不需要数据库。

import (
	"fmt"
	"testing"

	"janus/internal/store"
)

// multivalueCase 一条"多值条件 + 一份画像"的语义断言。
type multivalueCase struct {
	name   string // 用例名(含命中 / 不命中方向)
	cond   store.RuleCondition
	fact   Fact
	actual string // 画像里该字段的实际取值,只用于诊断文案
	want   bool   // any-of 语义下的期望结论
	why    string // 期望依据(人读)
}

// multivalueFixedCases 固定字段(恒单值画像)上的多值条件:
// 取值 [/a, /b],画像字段值取 /a(第一个取值)、/b(第二个取值)、/c(都不等)与空串。
//
// 两个方向都覆盖:取值 [a,b] 时,字段值 = a 的 neq 应不命中、字段值 = c 的 neq 应命中。
func multivalueFixedCases() []multivalueCase {
	values := []string{"/a", "/b"}
	specs := []struct {
		name     string
		operator string
		path     string
		want     bool
		why      string
	}{
		{"eq 命中第一个取值", OpEq, "/a", true, "any-of:与 /a 相等"},
		{"eq 命中第二个取值", OpEq, "/b", true, "any-of:与 /b 相等"},
		{"eq 与两个取值都不等", OpEq, "/c", false, "any-of:与 /a、/b 都不相等"},
		{"neq 等于第一个取值", OpNeq, "/a", false, "any-of 的否定:字段值等于列出的取值之一"},
		{"neq 等于第二个取值", OpNeq, "/b", false, "any-of 的否定:字段值等于列出的取值之一"},
		{"neq 与所有取值都不等", OpNeq, "/c", true, "any-of 的否定:与 /a、/b 都不相等"},
		{"not_in 落在集合里(第一个取值)", OpNotIn, "/a", false, "in 的反面:任一取值命中即不成立"},
		{"not_in 落在集合里(第二个取值)", OpNotIn, "/b", false, "in 的反面:任一取值命中即不成立"},
		{"not_in 不在集合里", OpNotIn, "/c", true, "in 的反面:任一取值都没命中"},
		{"空字段值的 eq 多值", OpEq, "", false, "空值恒不命中(与运算符无关)"},
		{"空字段值的 neq 多值", OpNeq, "", false, "空值恒不命中(与运算符无关)"},
	}
	out := make([]multivalueCase, 0, len(specs))
	for _, s := range specs {
		out = append(out, multivalueCase{
			name:   s.name,
			cond:   store.RuleCondition{Field: FieldPath, Operator: s.operator, Values: values},
			fact:   Fact{Path: s.path, Domain: "s.test"},
			actual: s.path,
			want:   s.want,
			why:    s.why,
		})
	}
	return out
}

// multivalueContainsCases 固定字段上的多值 not_contains:取值 [alpha, beta]。
func multivalueContainsCases() []multivalueCase {
	values := []string{"alpha", "beta"}
	specs := []struct {
		name string
		ua   string
		want bool
		why  string
	}{
		{"not_contains 含第一个取值", "alpha", false, "任一取值被包含即不成立"},
		{"not_contains 含第二个取值", "beta", false, "任一取值被包含即不成立"},
		{"not_contains 含第一个取值(大小写不同)", "ALPHA", false, "子串比较忽略大小写"},
		{"not_contains 两个取值都不含", "gamma", true, "任一取值都没被包含"},
	}
	out := make([]multivalueCase, 0, len(specs))
	for _, s := range specs {
		out = append(out, multivalueCase{
			name:   s.name,
			cond:   store.RuleCondition{Field: FieldUA, Operator: OpNotContains, Values: values},
			fact:   Fact{UA: s.ua, Domain: "s.test"},
			actual: s.ua,
			want:   s.want,
			why:    s.why,
		})
	}
	return out
}

// multivalueDynamicCases 多值取值字段(同名请求头出现多次)上的多值条件。
//
// 两个画像覆盖多值取值的两端:头只出现一次(beta)、头出现两次(alpha, beta)。
// 取值的语义不因字段是单值还是多值而改变(见 fields.go matchFieldValues):
// neq / not_in / not_contains 要求字段的**全部**取值都不满足正向形式。
func multivalueDynamicCases(t testing.TB) []multivalueCase {
	t.Helper()
	single := mustRequest(t, "https://s.test/p", "", "")
	single.Header.Set("X-Multi", "beta")
	both := mustRequest(t, "https://s.test/p", "", "")
	both.Header.Add("X-Multi", "alpha")
	both.Header.Add("X-Multi", "beta")

	values := []string{"alpha", "beta"}
	contexts := []struct {
		name   string
		fact   Fact
		actual string
	}{
		{"头出现一次(beta)", FromRequest(single), "beta"},
		{"头出现两次(alpha, beta)", FromRequest(both), "alpha, beta"},
	}
	specs := []struct {
		name     string
		operator string
		want     bool
		why      string
	}{
		{"eq 命中集合里的取值", OpEq, true, "any-of:至少一个头值与列出的取值相等"},
		{"neq 有一个头值相等即不成立", OpNeq, false, "any-of 的否定:存在头值等于列出的取值"},
		{"not_in 有一个头值在集合内即不成立", OpNotIn, false, "in 的反面:存在头值命中集合"},
		{"not_contains 有一个头值被包含即不成立", OpNotContains, false, "任一取值被包含即不成立"},
	}
	out := make([]multivalueCase, 0, len(contexts)*len(specs))
	for _, c := range contexts {
		for _, s := range specs {
			out = append(out, multivalueCase{
				name:   s.name + " · " + c.name,
				cond:   store.RuleCondition{Field: "header:X-Multi", Operator: s.operator, Values: values},
				fact:   c.fact,
				actual: c.actual,
				want:   s.want,
				why:    s.why,
			})
		}
	}
	return out
}

// TestMultiValueNegativeSemanticsAgreesAcrossShapes 钉住契约:
// 对 ≥2 个取值的 eq / neq / not_in / not_contains,扁平形态与等价树形态必须给出同一个结论,
// 且该结论必须符合 any-of 语义(neq / not_in / not_contains = 与所有取值都不等 / 都不匹配)。
//
// Given: 一条多值条件 + 一份画像(命中 / 不命中两个方向都覆盖)。
// When:  分别以扁平形态(oneRule,求值走谓词流水线 StringPredicate)与等价树形态(oneTreeRule 的一层 all 组,求值走 compiledCond)编译并对同一份画像求值。
// Then:  两形态结论一致,且都等于 any-of 语义下的期望值。
func TestMultiValueNegativeSemanticsAgreesAcrossShapes(t *testing.T) {
	cases := append(append(multivalueFixedCases(), multivalueContainsCases()...), multivalueDynamicCases(t)...)

	tested := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flat := NewSnapshot([]store.Rule{oneRule(1, tc.cond)}, discardLog)
			tree := NewSnapshot([]store.Rule{oneTreeRule(1, store.Group("all", store.Leaf(tc.cond)))}, discardLog)
			if len(flat.Rules) != 1 || len(tree.Rules) != 1 {
				t.Fatalf("规则被整条丢弃:扁平 %d 条 / 树 %d 条,两者都该保留(条件 %s %s %v)",
					len(flat.Rules), len(tree.Rules), tc.cond.Field, tc.cond.Operator, tc.cond.Values)
			}
			if flat.Rules[0].pipeline == nil {
				t.Fatalf("扁平形态没建出谓词流水线,本用例覆盖不到两形态分叉(条件 %s %s %v)",
					tc.cond.Field, tc.cond.Operator, tc.cond.Values)
			}
			f := tc.fact
			_, flatMatched := flat.Evaluate(&f, 1)
			_, treeMatched := tree.Evaluate(&f, 1)
			tested++

			detail := fmt.Sprintf("条件 %s %s %v,画像字段值 %q", tc.cond.Field, tc.cond.Operator, tc.cond.Values, tc.actual)
			if flatMatched != treeMatched {
				t.Errorf("两形态不一致:扁平=%v 树=%v(%s;期望 %v —— %s)",
					flatMatched, treeMatched, detail, tc.want, tc.why)
			}
			if flatMatched != tc.want {
				t.Errorf("扁平形态(谓词流水线)= %v, want %v(%s;%s)", flatMatched, tc.want, detail, tc.why)
			}
			if treeMatched != tc.want {
				t.Errorf("树形态(compiledCond)= %v, want %v(%s;%s)", treeMatched, tc.want, detail, tc.why)
			}
		})
	}
	if tested == 0 {
		t.Fatal("一个组合都没测到,测试失效")
	}
}
