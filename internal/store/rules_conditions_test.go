package store

// 条件写入边界的严格解码测试(审查项 COND1)。
//
// 求值侧"零条件 = 恒命中"是刻意的兜底特性(rules/eval.go),引擎在求值期无法区分
// "故意兜底"与"输入被吞错归零",所以防线只能落在写入边界:一份写坏的 conditions
// 宁可 400,也绝不能静默归零成空条件组(空组 = 对全部流量生效)。

import "testing"

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
