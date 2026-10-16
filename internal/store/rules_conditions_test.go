package store

// 条件写入边界的严格解码测试(审查项 COND1)。
//
// 求值侧"零条件 = 恒命中"是刻意的兜底特性(rules/eval.go),引擎在求值期无法区分
// "故意兜底"与"输入被吞错归零",所以防线只能落在写入边界:一份写坏的 conditions
// 宁可 400,也绝不能静默归零成空条件组(空组 = 对全部流量生效)。

import (
	"context"
	"testing"
)

// TestDecodeRuleConditionsStrict 严格解码:任何"写坏"的输入都必须报错。
// 与 UnmarshalJSON/Scan(读库容错,解析失败降级为空组)语义刻意相反。
func TestDecodeRuleConditionsStrict(t *testing.T) {
	bad := []struct{ name, raw string }{
		{"字符串笔误", `"group: country in [US]"`},
		{"数字", `123`},
		{"布尔", `true`},
		{"null", `null`},
		{"空对象", `{}`},
		{"未知键对象", `{"foo":1}`},
		{"组缺 children", `{"logic":"all"}`},
		{"数组里混入数字", `[1,2,3]`},
		{"叶子字段类型错", `[{"field":1}]`},
		{"树里混入废节点", `{"logic":"all","children":[{"field":"country","operator":"eq","values":["CN"]},5]}`},
		{"空输入", ``},
		{"纯空白", `   `},
	}
	for _, tc := range bad {
		t.Run("坏-"+tc.name, func(t *testing.T) {
			got, err := DecodeRuleConditions([]byte(tc.raw))
			if err == nil {
				t.Fatalf("%q 被静默接受成 %#v, want 错误", tc.raw, got)
			}
		})
	}

	good := []struct {
		name   string
		raw    string
		leaves int
		tree   bool
	}{
		{"空数组", `[]`, 0, false},
		{"扁平叶子", `[{"field":"country","operator":"in","values":["US"]}]`, 1, false},
		{"裸字段树", `{"logic":"any","children":[{"field":"country","operator":"eq","values":["CN"]}]}`, 1, true},
		{"显式树", `{"type":"group","logic":"all","children":[{"type":"leaf","leaf":{"field":"path","operator":"contains","values":["/x"]}}]}`, 1, true},
	}
	for _, tc := range good {
		t.Run("好-"+tc.name, func(t *testing.T) {
			got, err := DecodeRuleConditions([]byte(tc.raw))
			if err != nil {
				t.Fatalf("%q 应被接受: %v", tc.raw, err)
			}
			if got.Len() != tc.leaves || got.IsTree() != tc.tree {
				t.Fatalf("%q 解析结果 = %#v", tc.raw, got)
			}
		})
	}
}

// TestUpdateRuleConditionsPointerIsExplicit 审查项 COND2:RuleUpdate.Conditions
// 非 nil 就表示"显式设置",即使是零值(零值 = 无条件组 = 合法的清空/恒命中兜底)。
//
// 旧实现用 `Conditions != nil && !Conditions.IsZero()` 双重条件,而 IsZero()
// 同时承担"本次没传这个字段"的哨兵职责,于是"显式清空"被误当成"未设置"静默丢弃:
// PATCH 写坏 conditions 时归零 → 哨兵吞掉 → 200 但条件一字未改。
// 把哨兵显式化为指针本身(与 RuleUpdate 其它字段一致的约定)后,显式零值必须真的落库。
func TestUpdateRuleConditionsPointerIsExplicit(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cond-sentinel@test.io")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "有条件的规则", Action: RuleActionPass,
		Conditions: Conditions(RuleCondition{Field: "country", Operator: "in", Values: []string{"US"}}),
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rawColumn(t, s, r.ID, "conditions") == "[]" {
		t.Fatalf("前置条件不成立:种子规则本就无条件")
	}

	// 显式设置为"无条件组":Conditions() 是无参形态(零值),也必须被当成显式清空。
	clear := Conditions()
	got, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{Conditions: &clear})
	if err != nil {
		t.Fatalf("update conditions: %v", err)
	}
	if raw := rawColumn(t, s, r.ID, "conditions"); raw != "[]" {
		t.Fatalf("显式清空后库内 conditions = %s, want [](被 IsZero 哨兵静默丢弃)", raw)
	}
	if got.Conditions.Len() != 0 || got.Conditions.IsTree() {
		t.Fatalf("显式清空后条件 = %#v, want 空条件组", got.Conditions)
	}

	// 反向确认:传 nil 指针(未设置)不动 conditions 列
	if _, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{Description: strPtr("只改描述")}); err != nil {
		t.Fatalf("update description: %v", err)
	}
	if raw := rawColumn(t, s, r.ID, "conditions"); raw != "[]" {
		t.Fatalf("未传 conditions 后库内 conditions = %s, want 保持 []", raw)
	}
}

// strPtr 取字符串指针(store 包内测试小工具)。
func strPtr(s string) *string { return &s }

// TestCreateRuleWithoutConditionsStoresEmptyArray 审查项 COND7:钉住现状。
//
// CreateRule 里 `if r.Conditions.IsZero() { r.Conditions = Conditions() }` 是无效死代码
// ——Conditions() 无参时 Leaves 仍为 nil(还是零值),赋值前后完全一致。
// 真正的兜底来自 MarshalJSON(`Leaves == nil → "[]"`)。删除该死代码前后:
//   - 未提供 conditions 与显式传零值 Conditions() 都必须落库 `[]`;
//   - 读回都是空条件组(求值侧 = 恒命中兜底),语义不变。
func TestCreateRuleWithoutConditionsStoresEmptyArray(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cond-create@test.io")

	created := []*Rule{}
	a, err := s.CreateRule(ctx, tenantID, Rule{Name: "未提供", Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create rule (未提供): %v", err)
	}
	created = append(created, a)

	b, err := s.CreateRule(ctx, tenantID, Rule{Name: "显式零值", Action: RuleActionPass, Conditions: Conditions()})
	if err != nil {
		t.Fatalf("create rule (显式零值): %v", err)
	}
	created = append(created, b)

	for _, r := range created {
		if raw := rawColumn(t, s, r.ID, "conditions"); raw != "[]" {
			t.Fatalf("%s: 库内 conditions = %s, want []", r.Name, raw)
		}
		got, err := s.GetRule(ctx, tenantID, r.ID)
		if err != nil {
			t.Fatalf("%s: get rule: %v", r.Name, err)
		}
		if got.Conditions.Len() != 0 || !got.Conditions.IsEmpty() || got.Conditions.IsTree() {
			t.Fatalf("%s: 读回条件 = %#v, want 空条件组(恒命中兜底)", r.Name, got.Conditions)
		}
	}
}
