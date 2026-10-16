package httpapi_test

// 规则文本字段的长度上限(审查项 COND5):name / description 此前无任何上限,
// 已登录租户可写入任意长的字符串。name 参与 (tenant_id, name) 唯一索引,
// description 也会进列表响应。给一个防御性的硬上界。

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"janus/internal/testutil"
)

// TestRuleNameAndDescriptionLengthLimits 超限 400,恰好到界仍可落库。
func TestRuleNameAndDescriptionLengthLimits(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 边界绿灯:恰好到上限可用(别把上限定得过紧)
	resp := c.post("/api/rules", map[string]any{
		"name":        strings.Repeat("n", 128),
		"description": strings.Repeat("d", 512),
		"action":      "notfound",
	})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// POST 超限 → 400
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"name 超 1 字节", map[string]any{"name": strings.Repeat("n", 129), "action": "notfound"}},
		{"description 超 1 字节", map[string]any{"name": "desc-over", "description": strings.Repeat("d", 513), "action": "notfound"}},
	} {
		t.Run("POST-"+tc.name, func(t *testing.T) {
			resp := c.post("/api/rules", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("状态码 = %d, want 400", resp.StatusCode)
			}
			_ = resp.Body.Close()
		})
	}

	// PATCH 同样受限
	created := createRule(t, c, baseRuleBody("待改字段"))
	path := "/api/rules/" + strconv.FormatInt(created.ID, 10)

	resp = c.patch(path, map[string]any{"name": strings.Repeat("n", 129)})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	resp = c.patch(path, map[string]any{"description": strings.Repeat("d", 513)})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 失败 PATCH 不得改动原值
	got := getRule(t, c, created.ID)
	if got.Name != "待改字段" || got.Description != "" {
		t.Fatalf("失败 PATCH 改动了字段 = %q / %q", got.Name, got.Description)
	}
}
