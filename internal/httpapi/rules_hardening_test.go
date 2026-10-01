package httpapi_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"janus/internal/rules"
	"janus/internal/store"
	"janus/internal/testutil"
)

// ---------- 05:表达式长度上限 + 请求体上限 ----------

// TestRuleEndpointsRejectOversizeBody /api/rules* 的请求体必须有上限。
// 没有上限时,一个已登录租户能用一个几十 MB 的 body 把内存打满
// (json.Decoder 要先把它整份读进内存),而 customHtml 一条规则就允许 512KB。
func TestRuleEndpointsRejectOversizeBody(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 一条合法规则(用于对照:限的是体积,不是内容)
	createRule(t, c, baseRuleBody("正常规则"))

	// body 上限是 1MB(maxRuleBodyBytes)。下面三处 payload 必须**真的超过 1MB**,
	// 否则读到的是逐字段上限(customHtml 512KB / 表达式长度上限)那条路径,
	// 测不到 body 上限本身 —— simulate 与 validate-expr 没有字段上限,600KB 会
	// 被正常接受并返回 200。
	huge := map[string]any{
		"name": "超大 body", "scope": "global", "action": "notfound",
		"pageMode": "custom", "customHtml": strings.Repeat("x", 1200*1024),
	}
	resp := c.post("/api/rules", huge)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("超大 body 状态码 = %d, want 400(不能是 500)", resp.StatusCode)
	}
	if msg := errMessage(t, resp); !strings.Contains(msg, "上限") {
		t.Fatalf("报错应说明是体积上限: %s", msg)
	}

	// PATCH 同样受限
	id := createRule(t, c, baseRuleBody("待改规则")).ID
	resp = c.patch("/api/rules/"+strconv.FormatInt(id, 10), huge)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH 超大 body 状态码 = %d, want 400", resp.StatusCode)
	}

	// simulate 也接一个自由形状的 JSON,同样要限
	resp = c.post("/api/rules/simulate", map[string]any{
		"url":       "https://localhost/x",
		"userAgent": strings.Repeat("u", 1200*1024),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("simulate 超大 body 状态码 = %d, want 400", resp.StatusCode)
	}

	// 校验表达式端点同样受限
	resp = c.post("/api/rules/validate-expr", map[string]any{
		"expression": strings.Repeat("a", 1200*1024),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("validate-expr 超大 body 状态码 = %d, want 400", resp.StatusCode)
	}
}

// TestRuleExpressionLengthLimit 超长表达式 400,并指明是长度问题。
func TestRuleExpressionLengthLimit(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 刚好在上限内的合法表达式仍然可用(别把上限定得太紧)
	okExpr := `Country == "` + strings.Repeat("x", rules.MaxExpressionLen-16) + `"`
	if len(okExpr) > rules.MaxExpressionLen {
		t.Fatalf("测试构造错误:%d", len(okExpr))
	}
	resp := c.post("/api/rules", map[string]any{
		"name": "长表达式", "scope": "global", "action": "notfound",
		"ruleType": "expression", "expression": okExpr,
	})
	assertStatus(t, resp, http.StatusCreated)

	// 超出一字符就 400
	overExpr := `Country == "` + strings.Repeat("x", rules.MaxExpressionLen) + `"`
	resp = c.post("/api/rules", map[string]any{
		"name": "超长表达式", "scope": "global", "action": "notfound",
		"ruleType": "expression", "expression": overExpr,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("超长表达式状态码 = %d, want 400", resp.StatusCode)
	}
	if msg := errMessage(t, resp); !strings.Contains(msg, "长度") {
		t.Fatalf("报错应说明是长度问题: %s", msg)
	}
}

// TestRuleExpressionLengthLimitAppliesToVisualRules 上限对两种 ruleType 一视同仁。
//
// visual 规则虽然不会被求值(compileRule 只在 ruleType=expression 时编译表达式),
// 但它照样把 expression 写进库 —— 一条 visual 规则带着 1MB 垃圾表达式,
// 白占库的 TOAST 空间,列表接口读它时也要付 IO。
func TestRuleExpressionLengthLimitAppliesToVisualRules(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	over := `Country == "` + strings.Repeat("x", rules.MaxExpressionLen) + `"`

	resp := c.post("/api/rules", map[string]any{
		"name": "visual 带超长表达式", "scope": "global", "action": "notfound",
		"ruleType": "visual", "expression": over,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("visual 规则的超长 expression 状态码 = %d, want 400", resp.StatusCode)
	}
	if msg := errMessage(t, resp); !strings.Contains(msg, "长度") {
		t.Fatalf("报错应说明是长度问题: %s", msg)
	}
}

// TestValidateExprEndpointRejectsMethodPayloads P0 的 API 侧回归:
// 绕过 refs 闸门的方法调用必须在**保存前**就被拒,不能落库。
func TestValidateExprEndpointRejectsMethodPayloads(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	payloads := []string{
		`Fields()["asn"] == ""`,
		`Fields()["country"] != "US"`,
		`Field("country") == ""`,
		`ClientIP() != nil`,
		`WithIP("1.2.3.4") != nil`,
	}
	for _, expr := range payloads {
		// 校验端点用 200 + valid=false 表达"不合法"
		resp := c.post("/api/rules/validate-expr", map[string]any{"expression": expr})
		assertStatus(t, resp, http.StatusOK)
		if body := decodeBody[map[string]any](t, resp); body["valid"] != false {
			t.Errorf("表达式 %q 应 valid=false, got %+v", expr, body)
		}

		// 创建规则时必须 400
		resp = c.post("/api/rules", map[string]any{
			"name": "绕过闸门", "scope": "global", "action": "notfound",
			"ruleType": "expression", "expression": expr,
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("表达式 %q 落库状态码 = %d, want 400", expr, resp.StatusCode)
		}
	}

	// 确认真的没落库(否则就是"报了错但还是存进去了")
	if n := countRules(t, env, tenantIDOf(t, c)); n != 0 {
		t.Fatalf("绕过闸门的表达式落库了 %d 条规则", n)
	}
}

// TestRuleExpressionRejectsCostlyBuiltins 高耗函数在 API 侧也被拒。
func TestRuleExpressionRejectsCostlyBuiltins(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	for _, expr := range []string{
		`reduce([1,2,3], #acc + #, 0) == 6`,
		`map([1,2,3], # + 1) == nil`,
		`filter([1,2], # > 1) == nil`,
		`split(UA, " ")[0] == ""`,
		`repeat("a", 1000000) == ""`,
		`fromJSON(UA) == nil`,
		`now() != nil`,
	} {
		resp := c.post("/api/rules", map[string]any{
			"name": "高耗", "scope": "global", "action": "notfound",
			"ruleType": "expression", "expression": expr,
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("表达式 %q 状态码 = %d, want 400", expr, resp.StatusCode)
		}
	}
}

// ---------- 09:asn 对齐前端置灰 ----------

// TestRuleRejectsASNField asn 没有数据源,条件恒不命中。前端已置灰
// (ruleMeta.ts 的 pending),后端这里对齐 —— 否则租户能从 API/脚本写进一条
// 永不生效、界面上却显示"已启用"的规则。
func TestRuleRejectsASNField(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	// 扁平条件
	resp := c.post("/api/rules", map[string]any{
		"name": "asn 条件", "scope": "global", "action": "notfound",
		"conditions": []any{
			map[string]any{"field": "asn", "operator": "in", "values": []string{"AS15169"}},
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("扁平条件用 asn 状态码 = %d, want 400", resp.StatusCode)
	}
	if msg := errMessage(t, resp); !strings.Contains(msg, "asn") {
		t.Fatalf("报错要点名 asn: %s", msg)
	}

	// 嵌套条件树
	resp = c.post("/api/rules", map[string]any{
		"name": "asn 嵌套条件", "scope": "global", "action": "notfound",
		"conditions": map[string]any{
			"type": "group", "logic": "any", "children": []any{
				map[string]any{
					"type": "group", "logic": "all", "children": []any{
						map[string]any{"field": "asn", "operator": "eq", "values": []string{"AS1"}},
					},
				},
			},
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("嵌套条件用 asn 状态码 = %d, want 400", resp.StatusCode)
	}

	// 表达式形态同样要拦(`asn == ""` 是一条永不命中的规则)
	resp = c.post("/api/rules", map[string]any{
		"name": "asn 表达式", "scope": "global", "action": "notfound",
		"ruleType": "expression", "expression": `asn == ""`,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("表达式用 asn 状态码 = %d, want 400", resp.StatusCode)
	}

	// PATCH 引入 asn 也要拦
	id := createRule(t, c, baseRuleBody("正常规则")).ID
	resp = c.patch("/api/rules/"+strconv.FormatInt(id, 10), map[string]any{
		"conditions": []any{
			map[string]any{"field": "asn", "operator": "eq", "values": []string{"AS1"}},
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH 引入 asn 状态码 = %d, want 400", resp.StatusCode)
	}

	// 其它字段不受影响(别把闸门做过头)
	createRule(t, c, baseRuleBody("country 条件"))
}

// ---------- 06:规则数上限在事务内判定 ----------

// TestRuleCountLimitIsEnforcedInTransaction 上限必须在**事务内**判定。
//
// 旧实现是 handleCreateRule 里先 CountTenantRules 再 CreateRule,两者都在事务外:
// 一批并发创建可以在计数停在 199 时全部通过检查,写出远超 200 条规则。
// 而规则集合整租户常驻内存供热路径求值,超上限直接变成内存与求值成本问题。
func TestRuleCountLimitIsEnforcedInTransaction(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	tenantID := tenantIDOf(t, c)

	// 直接灌到上限前一条:这里只测"并发能否越界",不测 HTTP 往返
	bulkInsertRules(t, env, tenantID, store.MaxRulesPerTenant-1)

	// 10 个并发创建抢最后 1 个名额。每个 goroutine 一个独立客户端:
	// testClient 的 cookie jar 无锁,共享会在读 map 时被 race 抓。
	const n = 10
	clients := make([]*testClient, n)
	for i := range clients {
		clients[i] = loginAs(t, env, "alice")
	}
	var wg sync.WaitGroup
	codes := make([]int, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // 尽量让它们挤在同一个时刻发起
			resp := clients[i].post("/api/rules", baseRuleBody(fmt.Sprintf("并发规则 %d", i)))
			codes[i] = resp.StatusCode
			_ = resp.Body.Close()
		}(i)
	}
	close(start)
	wg.Wait()

	created, forbidden := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusForbidden:
			forbidden++
		default:
			t.Errorf("意外状态码 %d", code)
		}
	}
	if created != 1 {
		t.Fatalf("并发抢最后 1 个名额:成功 %d 个(403 共 %d 个), want 1", created, forbidden)
	}
	// 关键断言:库里必须**恰好**卡在上限,一条不多
	if got := countRules(t, env, tenantID); got != store.MaxRulesPerTenant {
		t.Fatalf("规则数 = %d, want %d(并发越过了上限)", got, store.MaxRulesPerTenant)
	}

	// 上限错误要带上用量与上限,前端要能展示
	resp := c.post("/api/rules", baseRuleBody("再来一条"))
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[map[string]any](t, resp)
	if body["code"] != errQuotaCode {
		t.Fatalf("错误码 = %v, want %s", body["code"], errQuotaCode)
	}
	details, _ := body["details"].(map[string]any)
	if fmt.Sprint(details["usage"]) != fmt.Sprint(store.MaxRulesPerTenant) ||
		fmt.Sprint(details["limit"]) != fmt.Sprint(store.MaxRulesPerTenant) {
		t.Fatalf("details 应带 usage/limit: %+v", details)
	}
}

// TestRuleLimitDoesNotBlockOtherTenants 上限判定锁的是租户行:
// 一个租户撞到上限,不该把别的租户一起卡住。
func TestRuleLimitDoesNotBlockOtherTenants(t *testing.T) {
	env := testutil.Setup(t)
	alice := loggedInTenant(t, env, "alice")
	bulkInsertRules(t, env, tenantIDOf(t, alice), store.MaxRulesPerTenant)

	resp := alice.post("/api/rules", baseRuleBody("alice 满了"))
	assertStatus(t, resp, http.StatusForbidden)

	// bob 不受影响
	bob := loggedInTenant(t, env, "bob")
	resp = bob.post("/api/rules", baseRuleBody("bob 的规则"))
	assertStatus(t, resp, http.StatusCreated)
}

// TestRuleCountLimitCountsCurrentRows 上限数的是**当前**条数:
// 删掉几条之后又能再建,更新一条既有规则不占名额。
func TestRuleCountLimitCountsCurrentRows(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	tenantID := tenantIDOf(t, c)
	bulkInsertRules(t, env, tenantID, store.MaxRulesPerTenant)
	path := "/api/rules/" + strconv.FormatInt(firstRuleID(t, env, tenantID), 10)

	// 满员时改既有规则:不占用名额
	resp := c.patch(path, map[string]any{"description": "改个描述"})
	assertStatus(t, resp, http.StatusOK)

	// 删一条,腾出一个名额
	assertStatus(t, c.del(path), http.StatusNoContent)
	resp = c.post("/api/rules", baseRuleBody("补回一条"))
	assertStatus(t, resp, http.StatusCreated)
	if got := countRules(t, env, tenantID); got != store.MaxRulesPerTenant {
		t.Fatalf("规则数 = %d, want %d", got, store.MaxRulesPerTenant)
	}
}

// ---------- 工具 ----------

// loginAs 登录已注册的租户,返回一个独立客户端。
// 并发测试里每个 goroutine 要一个自己的客户端:testClient 的 cookie jar 无锁。
func loginAs(t *testing.T, env *testutil.Env, slug string) *testClient {
	t.Helper()
	c := newClient(env)
	resp := c.post("/api/auth/login", map[string]string{
		"email": slug + "@example.com", "password": "password123",
	})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	return c
}

// bulkInsertSQL 批量灌规则用的 SQL(generate_series 一次插完,不走 200 次往返)。
const bulkInsertSQL = `INSERT INTO rules (tenant_id, name, action) ` +
	`SELECT $1, 'bulk-'||g, 'pass' FROM generate_series(1, $2) g`

// bulkInsertRules 直接往库里灌 n 条规则(绕开 HTTP 与上限判定,只为把计数推到位)。
func bulkInsertRules(t *testing.T, env *testutil.Env, tenantID int64, n int) {
	t.Helper()
	if _, err := env.Pool.Exec(testutil.Ctx(), bulkInsertSQL, tenantID, n); err != nil {
		t.Fatalf("bulkInsertRules(%d): %v", n, err)
	}
}

func countRules(t *testing.T, env *testutil.Env, tenantID int64) int {
	t.Helper()
	n, err := env.Store.CountTenantRules(t.Context(), tenantID)
	if err != nil {
		t.Fatalf("CountTenantRules: %v", err)
	}
	return n
}

func firstRuleID(t *testing.T, env *testutil.Env, tenantID int64) int64 {
	t.Helper()
	items, _, err := env.Store.ListRules(t.Context(), tenantID, 1, 1)
	if err != nil || len(items) == 0 {
		t.Fatalf("ListRules: %v", err)
	}
	return items[0].ID
}

// errMessage 取错误响应的 message 字段。
func errMessage(t *testing.T, resp *http.Response) string {
	t.Helper()
	return fmt.Sprint(decodeBody[map[string]any](t, resp)["message"])
}
