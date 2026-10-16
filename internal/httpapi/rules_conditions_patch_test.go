package httpapi_test

// 条件写入边界(审查项 COND2)的黑盒测试:PATCH 的三种 conditions 语义。
//
// 根因:store.RuleConditions 的 IsZero() 同时承担"本次没传这个字段"哨兵与
// "解析失败被归零"两个职责,两者在原实现里不可区分。PATCH 路径永远传非 nil 的
// &w.conditions,于是写坏的 conditions 归零后被 IsZero() 当成"未设置"吞掉——
// 接口返回 200 但 conditions 一字未改,比"变恒命中"更隐蔽。

import (
	"net/http"
	"strconv"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

// TestPatchRuleRejectsMalformedConditions PATCH 写坏的 conditions → 400,且原条件不被改动。
func TestPatchRuleRejectsMalformedConditions(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	created := createRule(t, c, func() map[string]any {
		b := baseRuleBody("有条件的规则")
		b["conditions"] = []map[string]any{
			{"field": "country", "operator": "in", "values": []string{"US", "CA"}},
		}
		return b
	}())
	path := "/api/rules/" + strconv.FormatInt(created.ID, 10)

	for _, tc := range malformedConditions() {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.patch(path, map[string]any{"conditions": tc.cond})
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("状态码 = %d, want 400(不能是 200 静默丢弃)", resp.StatusCode)
			}
			_ = resp.Body.Close()
		})
	}
	// 原条件必须原样保留
	got := getRule(t, c, created.ID)
	if got.Conditions.Len() != 1 || got.Conditions.Leaves[0].Field != "country" {
		t.Fatalf("失败 PATCH 后条件被改写 = %+v", got.Conditions)
	}
}

// TestPatchRuleConditionsSemantics 锁住 conditions 的三种写入语义:
//   - 省略 conditions      → 原值不变
//   - 显式 conditions: []  → 清空生效(合法功能:无条件组 = 恒命中兜底)
//   - 坏 JSON             → 400(不是 200 静默丢弃)
func TestPatchRuleConditionsSemantics(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	created := createRule(t, c, func() map[string]any {
		b := baseRuleBody("语义规则")
		b["conditions"] = []map[string]any{
			{"field": "country", "operator": "in", "values": []string{"US"}},
		}
		return b
	}())
	path := "/api/rules/" + strconv.FormatInt(created.ID, 10)

	// ① 省略 → 原值不变(只改名字)
	resp := c.patch(path, map[string]any{"name": "语义规则(改名)"})
	assertStatus(t, resp, http.StatusOK)
	patched := decodeBody[store.Rule](t, resp)
	if patched.Conditions.Len() != 1 || patched.Conditions.Leaves[0].Field != "country" {
		t.Fatalf("省略 conditions 后条件被改写 = %+v", patched.Conditions)
	}

	// ② 显式 [] → 清空生效(无条件组,合法兜底功能)
	resp = c.patch(path, map[string]any{"conditions": []any{}})
	assertStatus(t, resp, http.StatusOK)
	patched = decodeBody[store.Rule](t, resp)
	if patched.Conditions.Len() != 0 || patched.Conditions.IsTree() {
		t.Fatalf("显式 [] 后条件 = %+v, want 空条件组", patched.Conditions)
	}
	// 落库形态必须是 `[]`(而不是 null),读回仍是空组
	var raw string
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT conditions::text FROM rules WHERE id = $1`, created.ID).Scan(&raw); err != nil {
		t.Fatalf("读库 conditions: %v", err)
	}
	if raw != "[]" {
		t.Fatalf("清空后 conditions 列 = %s, want []", raw)
	}

	// ③ 坏 JSON → 400,且条件仍保持清空态(未被悄悄改回)
	resp = c.patch(path, map[string]any{"conditions": "group: typo"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("坏 JSON 状态码 = %d, want 400", resp.StatusCode)
	}
	_ = resp.Body.Close()
	got := getRule(t, c, created.ID)
	if got.Conditions.Len() != 0 {
		t.Fatalf("坏 JSON 后条件 = %+v, want 仍为空", got.Conditions)
	}
}
