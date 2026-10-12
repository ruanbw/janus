package httpapi_test

// 规则"动态字段"的 API 契约:请求头 `header:<name>` 与 URL 查询参数 `query:<name>`。
//
// 取值与命中语义在单元层钉住(见 internal/rules/dynamic_fields_test.go);这里锁的是 API 一侧:
//
//   - 合法字段名必须能落库、并**原样**回读。前端规则编辑器是"整体读回、整体写回",
//     后端若顺手把字段名归一化,用户下次保存就会被改写成另一个值。
//   - 非法字段名与不适用的运算符必须在落库前拒掉(400)。一条能存进库、
//     却永远取不到值的条件,就是"能配不能跑"的假能力。
//   - 规则模拟器必须能喂进任意请求头。不然这两个字段在控制台里根本没法验证:
//     用户配好规则、点开模拟器,永远只看见"没有数据"。

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

// TestRuleAcceptsHeaderAndQueryFields 合法动态字段条件落库并原样回读。
func TestRuleAcceptsHeaderAndQueryFields(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	body := baseRuleBody("活动请求头拦截")
	body["priority"] = 10
	body["conditions"] = []map[string]any{
		{"field": "header:X-Promo-Code", "operator": "eq", "values": []string{"VIP2026"}},
		{"field": "query:utm_content", "operator": "contains", "values": []string{"promo"}},
	}
	created := createRule(t, c, body)
	if created.Conditions.Len() != 2 {
		t.Fatalf("conditions 条数 = %d, want 2", created.Conditions.Len())
	}

	got := getRule(t, c, created.ID)
	if got.Conditions.Len() != 2 {
		t.Fatalf("详情 conditions 条数 = %d, want 2", got.Conditions.Len())
	}
	// 字段名原样回读:大小写是用户输入的一部分,回读不该替用户改写
	if field := got.Conditions.Leaves[0].Field; field != "header:X-Promo-Code" {
		t.Errorf("回读字段 = %q, want header:X-Promo-Code", field)
	}
	if field := got.Conditions.Leaves[1].Field; field != "query:utm_content" {
		t.Errorf("回读字段 = %q, want query:utm_content", field)
	}
}

// TestRuleRejectsInvalidDynamicFieldNames 非法动态字段名一律 400。
func TestRuleRejectsInvalidDynamicFieldNames(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	bad := []string{
		"header:",          // 空名字
		"query:",           // 空名字
		"HEADER:X-Token",   // 前缀必须小写
		"Query:lang",       //
		"header",           // 没有前缀
		"cookie:session",   // 未支持的前缀
		"header: bad",      // 前导空格
		"header:bad name",  // 名字里有空格
		"header:X-Token\n", // 控制字符
		"header:a:b",       // 冒号不是合法的头名字符
		"header:名字",        // 头名必须是 ASCII token
		"header:" + strings.Repeat("a", 257),
		"query:" + strings.Repeat("a", 257),
		"query:a&b",   // 解码后不可能出现的参数名
		"query:a=b",   //
		"query:a#b",   //
		"query:a%20b", // 名字按解码后写,不接受百分号编码
	}
	for i, field := range bad {
		body := baseRuleBody(fmt.Sprintf("坏字段-%d", i))
		body["conditions"] = []map[string]any{
			{"field": field, "operator": "eq", "values": []string{"x"}},
		}
		resp := c.post("/api/rules", body)
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status != http.StatusBadRequest {
			t.Errorf("字段 %q: status = %d, want 400", field, status)
		}
	}
}

// TestRuleRejectsCIDROperatorOnDynamicField CIDR 运算符只属于 ip 字段。
func TestRuleRejectsCIDROperatorOnDynamicField(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	cases := []map[string]any{
		{"field": "header:X-Forwarded-For", "operator": "in_cidr", "values": []string{"10.0.0.0/8"}},
		{"field": "query:ip", "operator": "not_in_cidr", "values": []string{"10.0.0.0/8"}},
	}
	for i, cond := range cases {
		body := baseRuleBody(fmt.Sprintf("cidr-非法-%d", i))
		body["conditions"] = []map[string]any{cond}
		resp := c.post("/api/rules", body)
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status != http.StatusBadRequest {
			t.Errorf("%v: status = %d, want 400", cond, status)
		}
	}
}

// TestSimulateResolvesDynamicFields 规则模拟器:自定义请求头入参 + URL 里的查询参数。
//
// 模拟器必须与线上同源:它走的是同一份 LazyVisitorContext,
// 自定义头只是在造"假想请求"时多写几个头,不另造一套取值逻辑。
func TestSimulateResolvesDynamicFields(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	createLink(t, c, map[string]any{
		"code":       "dynfd",
		"targetUrls": []string{"https://t1.example.com"},
		"domainIds":  []int64{lid},
	})

	createRule(t, c, map[string]any{
		"name": "活动头拦截", "priority": 10, "action": store.RuleActionNotfound,
		"conditions": []map[string]any{
			{"field": "header:X-Token", "operator": "eq", "values": []string{"vip"}},
			{"field": "query:lang", "operator": "eq", "values": []string{"en"}},
		},
	})
	url := "https://localhost/dynfd?lang=en"

	// 1. 带上请求头 + URL 参数 → 命中
	hit := simulate(t, c, map[string]any{"url": url, "headers": map[string]string{"X-Token": "vip"}})
	if !hit.Verdict.Matched || !hit.Verdict.Blocked {
		t.Fatalf("带头推演 = (matched=%v,blocked=%v), want 都 true; steps=%+v",
			hit.Verdict.Matched, hit.Verdict.Blocked, hit.Steps)
	}

	// 2. 头名大小写不敏感、取值比较忽略大小写
	hit2 := simulate(t, c, map[string]any{"url": url, "headers": map[string]string{"x-token": "VIP"}})
	if !hit2.Verdict.Matched {
		t.Errorf("头名大小写不同的推演未命中: %+v", hit2.Steps)
	}

	// 3. 不带这个头 → 不命中,且决策链如实留痕"没有数据"
	miss := simulate(t, c, map[string]any{"url": url})
	if miss.Verdict.Matched {
		t.Fatalf("没带请求头却命中了: %+v", miss.Verdict)
	}
	headTrace, ok := findSimCond(miss.Steps, "header:X-Token")
	if !ok {
		t.Fatalf("决策链里没有 header:X-Token 的留痕: %+v", miss.Steps)
	}
	if headTrace.Available || headTrace.Matched {
		t.Errorf("没带请求头时的留痕 = (available=%v,matched=%v), want 都 false",
			headTrace.Available, headTrace.Matched)
	}
	// 同一条规则里另一个条件(URL 参数)仍应如实命中
	queryTrace, ok := findSimCond(miss.Steps, "query:lang")
	if !ok || !queryTrace.Matched {
		t.Errorf("query:lang 的留痕 = (%+v, ok=%v), want 命中", queryTrace, ok)
	}

	// 4. 自定义头与快捷输入同名时,快捷输入优先(用户显式填的那个字段说了算)
	ua := simulate(t, c, map[string]any{
		"url":       url,
		"userAgent": "OverrideUA",
		"headers":   map[string]string{"User-Agent": "BaseUA"},
	})
	if got := ua.Facts["ua"]; got != "OverrideUA" {
		t.Errorf("facts.ua = %q, want OverrideUA(快捷输入优先)", got)
	}
}

// findSimCond 在决策链里按字段名取一条条件留痕。
func findSimCond(steps []simStep, field string) (simCond, bool) {
	for _, step := range steps {
		for _, cond := range step.Conditions {
			if cond.Field == field {
				return cond, true
			}
		}
	}
	return simCond{}, false
}
