package httpapi_test

// 单值运算符的取值个数(审查项 COND6):gt / lt / duplicated 只消费 values[0],
// 引擎同口径忽略 values[1:]。多值属于"配了不生效"的死配置,写入时直接拒。

import (
	"net/http"
	"strconv"
	"testing"

	"janus/internal/testutil"
)

// TestRuleSingleValueOperatorsRejectMultipleValues 多值 400,单值仍可落库。
func TestRuleSingleValueOperatorsRejectMultipleValues(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	seed := createRule(t, c, baseRuleBody("种子规则"))
	patchPath := "/api/rules/" + strconv.FormatInt(seed.ID, 10)

	for _, op := range []string{"gt", "lt", "duplicated"} {
		t.Run(op, func(t *testing.T) {
			cond := map[string]any{"field": "ip", "operator": op, "values": []string{"1", "2"}}

			post := c.post("/api/rules", map[string]any{
				"name": "多值-" + op, "action": "notfound",
				"conditions": []map[string]any{cond},
			})
			if post.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST 多值 %s 状态码 = %d, want 400(多余的值永远不会生效)", op, post.StatusCode)
			}
			_ = post.Body.Close()

			patch := c.patch(patchPath, map[string]any{"conditions": []map[string]any{cond}})
			if patch.StatusCode != http.StatusBadRequest {
				t.Fatalf("PATCH 多值 %s 状态码 = %d, want 400", op, patch.StatusCode)
			}
			_ = patch.Body.Close()
		})
	}

	// 单值(合法)仍可用,确保没把闸门做过头
	for _, cond := range []map[string]any{
		{"field": "ip", "operator": "gt", "values": []string{"1"}},
		{"field": "ip", "operator": "lt", "values": []string{"10"}},
		{"field": "ip", "operator": "duplicated", "values": []string{"2"}},
	} {
		resp := c.post("/api/rules", map[string]any{
			"name": "单值-" + cond["operator"].(string), "action": "notfound",
			"conditions": []map[string]any{cond},
		})
		assertStatus(t, resp, http.StatusCreated)
		_ = resp.Body.Close()
	}
}
