package rules

// 扁平形态(rules.logic + 一层 conditions)与"等价的单层树"必须对同一份访客画像
// 给出完全一致的求值结果。这是纯单元测试,不需要数据库。
//
// 为什么值得单独钉:两条形态在同一条规则上走的是**两条不同的求值路径**。
//
//   - 扁平形态:compileRule 同时建出 compiledNode(root) 与 ConditionPipeline。
//     root 收下每一条 compileCond 能编译的叶子;pipeline 只收下 CompilePredicate
//     能编译出谓词的叶子。求值期 (*nativeVisualEvaluator).Match 只要 pipeline 非空
//     就走 pipeline —— 编译不出谓词的那条叶子在这里被**静默丢掉**。
//   - 树形态:compileNode 递归求值 root,根本没有 pipeline 这条旁路。
//
// 于是"同一份条件 + 同一份画像",两种写法可以给出相反的命中结果。
//
// 现状的分叉点(本文件钉住它):ip 字段注册了 exact compiler(predicate_init.go),
// 它对 gt / lt / duplicated 直接 return false,且**不回落到** fallback 通用编译器
// ——而 compileCond 是接受这三个组合的。结果:
//
//	flat [country==US, ip gt 1]    → pipeline 里只剩 country    → 命中
//	tree all[country==US, ip gt 1] → ip 那一条恒不成立          → 不命中
//
// 既有测试看不见它:TestOperators 只走 compiledCond(树形态),
// TestSnapshotSimulateAgreesWithEvaluate 只比较 verdict(两边都来自 pipeline)。

import (
	"regexp"
	"testing"

	"janus/internal/store"
)

// probeUA 一份"能判出设备/系统/浏览器"的 iPhone Safari UA。
const probeUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) " +
	"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"

// probeFields 覆盖全部可求值字段:13 个固定字段 + 两类动态字段各一个。
func probeFields() []string {
	return []string{
		FieldIP, FieldIPAttr, FieldCountry, FieldASN, FieldLang, FieldRef, FieldUTM,
		FieldUA, FieldDevType, FieldOS, FieldBrowser, FieldPath, FieldDomain,
		"header:x-probe",
		"query:probe",
	}
}

// probeOperatorsFor 返回某个字段上"合法"的运算符集合。
//
// 覆盖 ValidOperator 白名单里的全部 14 个运算符;in_cidr / not_in_cidr 只对 ip
// 字段成立(eval.go 的 compileCond 明确丢弃非 ip 字段的它们)。这类落库即被丢弃的
// 组合是配置错误,不属于"同一条合法叶子"的等价契约 —— 它们对应的是
// "扁平丢单条 / 树丢整条"的坏条件策略,由 TestNestedTreeBadLeafDropsWholeRule 钉住。
func probeOperatorsFor(field string) []string {
	ops := []string{
		OpIn, OpNotIn, OpEq, OpNeq, OpContains, OpNotContains,
		OpStartsWith, OpEndsWith, OpGT, OpLT, OpRegex, OpDuplicated,
	}
	if field == FieldIP {
		ops = append(ops, OpInCIDR, OpNotInCIDR)
	}
	return ops
}

// probeBaseFact 一个"每个字段都有值"的画像,让探针叶子尽可能成立。
func probeBaseFact(t testing.TB) Fact {
	t.Helper()
	req := mustRequest(t, "https://s.test/promo?probe=vip&utm_source=42",
		"https://www.google.com/", probeUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-Probe", "vip")
	f := FromRequest(req).WithIP("10.1.2.3")
	f.Country = "US"
	f.ASN = "AS13335"
	// duplicated 的计数来源:让"每个字段都有值"的画像同时有计数数据。
	f.Seen = map[string]int{}
	for _, field := range probeFields() {
		if v, ok := f.Field(field); ok {
			f.Seen[SeenKey(field, v)] = 3
		}
	}
	return f
}

// probeValues 给一条探针叶子造字面量:能成立就尽量让它成立(正向运算符取字段实际值),
// 不能成立(如非数值字段的 gt)就如实造一个——两种形态只需对同一份画像给出一致结论。
//
// ip 字段的集合比较(in/eq/neq/not_in/in_cidr/not_in_cidr)只接受能解析成 IP/CIDR 的
// 字面量,所以这里的"肯定不相等"必须换一个合法 IP,而不是随手一个字符串。
func probeValues(field, op, v string) []string {
	switch op {
	case OpNotIn, OpNeq:
		if field == FieldIP {
			return []string{"192.168.0.1"}
		}
		return []string{"no-such-literal"}
	case OpNotContains:
		return []string{"no-such-literal"}
	case OpInCIDR:
		return []string{"10.1.0.0/16"}
	case OpNotInCIDR:
		return []string{"192.168.0.0/16"}
	case OpGT:
		return []string{"1"}
	case OpLT:
		return []string{"1000000"}
	case OpDuplicated:
		return []string{"1"}
	case OpRegex:
		return []string{"^" + regexp.QuoteMeta(v) + "$"}
	}
	return []string{v}
}

// TestFlatAndTreeFormsAgreeForEveryValidCombination 钉住契约:同一个叶子放进
// 扁平形态(rules.logic=all + conditions)与等价的树形态里,对同一份访客画像
// 必须命中与否一致。
//
// 两种形态都测:
//   - 单叶:flat [probe] 对 tree all[probe] —— 任务里"构造同一个叶子"的字面形态;
//   - 锚点+探针:flat [anchor, probe] 对 tree all[anchor, probe] —— 多一条恒成立的锚点,
//     扁平形态才会走 pipeline(compileRule 只在 len(preds) > 0 时建 pipeline),
//     也正是这个接缝上编译不出谓词的叶子被静默丢掉。
//
// Given: 一条编译得出来的探针叶子 + 一条恒成立的锚点叶子(country == US)。
// When:  分别以扁平形态与树形态编译,并对同两份画像求值。
// Then:  两种形态的命中结果必须相同 —— 扁平形态不得静默丢掉探针叶子。
func TestFlatAndTreeFormsAgreeForEveryValidCombination(t *testing.T) {
	anchor := store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"US"}}
	base := probeBaseFact(t)
	facts := []struct {
		name string
		fact Fact
	}{
		// 每个字段都有值:探针叶子对多数组合成立。
		{"字段齐备", base},
		// 只有锚点字段有值:锚点成立、探针字段全部无数据(空值恒不命中)。
		{"只有锚点字段有值", Fact{Country: "US", Domain: "s.test"}},
	}

	tested := 0
	for _, field := range probeFields() {
		for _, op := range probeOperatorsFor(field) {
			t.Run(field+" "+op, func(t *testing.T) {
				v, ok := base.Field(field)
				if !ok {
					t.Fatalf("探针字段 %q 在基准画像里取不到值", field)
				}
				probe := store.RuleCondition{Field: field, Operator: op, Values: probeValues(field, op, v)}
				if _, ok := compileCond(probe, 1, discardLog); !ok {
					t.Fatalf("探针叶子 %q %q %v 编译不出来,测试范围与实现不一致", field, op, probe.Values)
				}
				shapes := []struct {
					name string
					flat []store.RuleCondition
					tree store.ConditionNode
				}{
					{"单叶", []store.RuleCondition{probe}, store.Group("all", store.Leaf(probe))},
					{"锚点+探针", []store.RuleCondition{anchor, probe},
						store.Group("all", store.Leaf(anchor), store.Leaf(probe))},
				}
				for _, shape := range shapes {
					flatSnap := NewSnapshot([]store.Rule{oneRule(1, shape.flat...)}, discardLog)
					treeSnap := NewSnapshot([]store.Rule{oneTreeRule(1, shape.tree)}, discardLog)
					if len(flatSnap.Rules) != 1 || len(treeSnap.Rules) != 1 {
						t.Fatalf("[%s] 规则被整条丢弃: 扁平 %d 条 / 树 %d 条,两者都该保留",
							shape.name, len(flatSnap.Rules), len(treeSnap.Rules))
					}
					tested++
					for _, tc := range facts {
						f := tc.fact
						_, flatMatched := flatSnap.Evaluate(&f, 1)
						_, treeMatched := treeSnap.Evaluate(&f, 1)
						if flatMatched != treeMatched {
							t.Errorf("[%s/%s] 扁平形态命中=%v,树形态命中=%v(叶子 %s %s %v,画像 %+v)",
								shape.name, tc.name, flatMatched, treeMatched, field, op, probe.Values, f)
						}
					}
				}
			})
		}
	}
	if tested == 0 {
		t.Fatal("一个组合都没测到,测试失效")
	}
}

// TestSimulateTraceAgreesWithVerdict 钉住契约:Simulate 的留痕与裁决不得自相矛盾。
//
// 同一份 CONDITIONS 来自同一条规则的 root(每条 compileCond 收下的叶子都留痕),
// 而裁决来自 matchAll —— 扁平形态下后者走 pipeline。两者一旦分家,
// 模拟器会给出"追踪到 ip gt 未满足,却报这条规则命中"的结论,是最误导人的一种诊断。
//
// Given: 扁平规则 all[country == US, ip gt/lt/duplicated 1],画像 country=US。
// When:  Simulate 推演。
// Then:  裁决与留痕必须**双向自洽**:报命中时留痕不得有未满足的条件(all 语义下必须
//
//	条条成立);未命中时留痕必须指出是哪个条件未满足 —— 只报未命中而不给理由,
//	与「报命中却不给支撑」同样是误导人的诊断。
func TestSimulateTraceAgreesWithVerdict(t *testing.T) {
	anchor := store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"US"}}
	probes := []store.RuleCondition{
		{Field: FieldIP, Operator: OpGT, Values: []string{"1"}},
		{Field: FieldIP, Operator: OpLT, Values: []string{"1"}},
		{Field: FieldIP, Operator: OpDuplicated, Values: []string{"1"}},
	}
	fact := Fact{Country: "US", IP: "203.0.113.7", Domain: "s.test"}

	for _, probe := range probes {
		t.Run(probe.Operator, func(t *testing.T) {
			// 对照:树形态下 ip 叶子真的参与判定,规则不命中,留痕与裁决自洽。
			treeSnap := NewSnapshot([]store.Rule{
				oneTreeRule(1, store.Group("all", store.Leaf(anchor), store.Leaf(probe))),
			}, discardLog)
			assertTraceAgreesWithVerdict(t, treeSnap, &fact)

			// 扁平形态:ip 叶子编译不出谓词,整条规则回落到树求值 —— 与上面的树形态
			// 同一条路径,因此两者必须给出同一个结论。
			//
			// 而 ip + gt/lt/duplicated 在 ip 字段上是「能配不能跑」的假能力:matchNumber
			// 对 "203.0.113.7" 解析失败恒不命中,duplicated 因 Fact.Seen 恒为 nil 也恒
			// 不命中。所以 all 语义下规则**不得命中**,且留痕必须指出是哪个条件没满足。
			// 旧实现把这个叶子静默丢掉,于是规则「命中」而留痕显示未满足 —— 正是本测试
			// 要钉住的自相矛盾。
			flatSnap := NewSnapshot([]store.Rule{oneRule(1, anchor, probe)}, discardLog)
			res := flatSnap.Simulate(&fact, 1, nil, nil)
			if res.Matched {
				t.Fatalf("扁平规则不该命中:ip %s 是 ip 字段上的假能力,恒不命中,all 语义下整条规则必须不成立", probe.Operator)
			}
			assertTraceAgreesWithVerdict(t, flatSnap, &fact)
			assertTraceExplainsMiss(t, res, probe)
		})
	}
}

// assertTraceAgreesWithVerdict 校验仿真结果自洽:报命中的那一步,留痕必须支持它。
// all 语义要求条条成立,any 语义要求至少一条成立;留痕里出现"未满足"就不能报命中。
func assertTraceAgreesWithVerdict(t testing.TB, snap *Snapshot, fact *Fact) {
	t.Helper()
	res := snap.Simulate(fact, 1, nil, nil)
	if res.Error != "" {
		t.Fatalf("仿真报错: %s", res.Error)
	}
	for _, step := range res.Steps {
		if step.Status != StepStatusHit {
			continue
		}
		anyMatched, allMatched := false, true
		for _, c := range step.Conditions {
			if c.Matched {
				anyMatched = true
			} else {
				allMatched = false
			}
		}
		if step.Logic == store.RuleLogicAny {
			if !anyMatched {
				t.Fatalf("规则 %d 报命中(any),但没有任何一条留痕成立: %+v", step.RuleID, step.Conditions)
			}
			continue
		}
		if !allMatched {
			t.Fatalf("规则 %d 报命中(all),但留痕里有条件未满足: %+v", step.RuleID, step.Conditions)
		}
	}
}

// assertTraceExplainsMiss 校验「未命中」同样有留痕支撑:规则未命中时,留痕里必须存在
// 那条未满足的条件,而不是只给一个没有理由的 false。
func assertTraceExplainsMiss(t testing.TB, res SimulationResult, probe store.RuleCondition) {
	t.Helper()
	for _, step := range res.Steps {
		if step.Status == StepStatusHit {
			continue
		}
		for _, c := range step.Conditions {
			if c.Field == probe.Field && c.Operator == probe.Operator && !c.Matched {
				return
			}
		}
	}
	t.Fatalf("规则未命中,但留痕里找不到未满足的 %s %s 条件: %+v", probe.Field, probe.Operator, res.Steps)
}
