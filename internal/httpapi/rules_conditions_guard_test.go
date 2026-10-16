package httpapi_test

// 条件写入边界(审查项 COND1)的黑盒测试:POST 写坏的 conditions 必须 400 且不落库。
//
// 背景:求值侧"零条件 = 恒命中"是刻意的兜底特性,引擎在求值期无法区分
// "故意兜底"与"输入被吞错归零",所以防线只能落在写入边界。
// 一份写坏的 conditions 若被静默归零,本该按国家/UA 生效的 notfound/throttle
// 规则会静默变成对全部流量生效。

import (
	"net/http"
	"testing"

	"janus/internal/testutil"
)

// malformedConditions 各种形态的"写坏"conditions(全部必须被拒)。
func malformedConditions() []struct {
	name string
	cond any
} {
	return []struct {
		name string
		cond any
	}{
		{"字符串笔误", "group: country in [US]"},
		{"数字", 123},
		{"布尔", true},
		{"null", nil},
		{"空对象", map[string]any{}},
		{"未知键对象", map[string]any{"foo": 1}},
		{"组缺 children", map[string]any{"logic": "all"}},
		{"数组里混入数字", []any{1, 2, 3}},
		{"叶子字段类型错", []any{map[string]any{"field": 1}}},
		{"树里混入废节点", map[string]any{"logic": "all", "children": []any{
			map[string]any{"field": "country", "operator": "eq", "values": []string{"CN"}}, 5}}},
	}
}

// TestCreateRuleRejectsMalformedConditions POST 写坏的 conditions → 400,且绝不落库。
// 旧实现走容错 UnmarshalJSON 静默归零 → 落库 `[]` → 规则恒命中。
func TestCreateRuleRejectsMalformedConditions(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	tenantID := tenantIDOf(t, c)

	for _, tc := range malformedConditions() {
		t.Run(tc.name, func(t *testing.T) {
			body := baseRuleBody("坏条件-" + tc.name)
			body["conditions"] = tc.cond
			resp := c.post("/api/rules", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("状态码 = %d, want 400", resp.StatusCode)
			}
			_ = resp.Body.Close()
		})
	}
	// 一条都不能落库,更不能落成"无条件组 = 恒命中"
	if n := countRules(t, env, tenantID); n != 0 {
		t.Fatalf("坏 conditions 落库了 %d 条规则", n)
	}
}
