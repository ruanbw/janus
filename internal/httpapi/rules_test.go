package httpapi_test

// 规则 CRUD(03)与「规则 ↔ 短链」关联(04)端到端黑盒测试。
// 关联模型见 spec D1:关联只存在规则一侧,短链表单的勾选与规则编辑器的多选是同一份数据。
// 本文件重点锁住三类"错了不会报错、只会静默出事"的行为:
//   - 读必须完整(GET 回完整 conditions/linkIds):读取不全会被前端原样写回;
//   - 写必须能区分"省略"与"空"(PATCH 的 linkIds 是 *[]int64);
//   - 校验不通过的写入必须被拒(非法字段 400 / 重名 409 / 规则数超限 403 / 跨租户 404)。

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"cloak/internal/httpapi"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

// rulePage 规则列表响应体。
type rulePage struct {
	Items []*store.Rule `json:"items"`
	Total int           `json:"total"`
}

// ruleOptionItem 规则下拉项。
type ruleOptionItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Hits24h  int64  `json:"hits24h"`
}

// linkRuleItem 短链适用规则(契约 linkRule)。
type linkRuleItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	Action    string `json:"action"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	LinkCount int    `json:"linkCount"`
}

type linkRulesResp struct {
	Items []linkRuleItem `json:"items"`
}

// createRule POST /api/rules 并断言 201。
func createRule(t *testing.T, c *testClient, body map[string]any) *store.Rule {
	t.Helper()
	resp := c.post("/api/rules", body)
	assertStatus(t, resp, http.StatusCreated)
	r := decodeBody[store.Rule](t, resp)
	return &r
}

// getRule GET /api/rules/{id}。
func getRule(t *testing.T, c *testClient, id int64) *store.Rule {
	t.Helper()
	resp := c.get("/api/rules/" + strconv.FormatInt(id, 10))
	assertStatus(t, resp, http.StatusOK)
	r := decodeBody[store.Rule](t, resp)
	return &r
}

// listRules GET /api/rules。
func listRules(t *testing.T, c *testClient) rulePage {
	t.Helper()
	resp := c.get("/api/rules?page=1&pageSize=100")
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[rulePage](t, resp)
}

// baseRuleBody 一条合法的建规则请求体(各用例只改自己关心的字段)。
func baseRuleBody(name string) map[string]any {
	return map[string]any{
		"name":   name,
		"action": store.RuleActionNotfound,
		"logic":  store.RuleLogicAll,
		"scope":  store.RuleScopeGlobal,
	}
}

// TestRuleCRUDRoundTrip 新建 → 读详情 → 改 → 删;详情必须回完整未截断的 conditions 与 linkIds。
func TestRuleCRUDRoundTrip(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	l1 := createLink(t, c, map[string]any{
		"code": "rza", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})
	l2 := createLink(t, c, map[string]any{
		"code": "rzb", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	body := baseRuleBody("拦爬虫")
	body["description"] = "只拦活动短链"
	body["priority"] = 10
	body["scope"] = store.RuleScopeLinks
	body["action"] = store.RuleActionRedirect
	body["destination"] = "https://blocked.example.com/"
	body["conditions"] = []map[string]any{
		{"field": "devtype", "operator": "eq", "values": []string{"bot"}},
		{"field": "path", "operator": "in", "values": []string{"/rza"}},
	}
	body["linkIds"] = []int64{l1.ID, l2.ID}
	created := createRule(t, c, body)

	if created.Name != "拦爬虫" || created.Priority != 10 || created.Scope != store.RuleScopeLinks {
		t.Fatalf("新建规则 = %+v", created)
	}
	if created.Enabled != true {
		t.Fatalf("enabled = %v, want 默认 true", created.Enabled)
	}
	if created.Destination != "https://blocked.example.com/" || created.Action != store.RuleActionRedirect {
		t.Fatalf("action/destination = %q/%q", created.Action, created.Destination)
	}
	if created.LinkCount != 2 || len(created.LinkIDs) != 2 {
		t.Fatalf("关联 = %v (count=%d), want 2 条", created.LinkIDs, created.LinkCount)
	}
	// 可读标识形如「短码@域名」,且不只给短码
	if len(created.LinkNames) != 2 || created.LinkNames[0] != "rza@localhost" {
		t.Fatalf("linkNames = %v, want [rla@localhost ...]", created.LinkNames)
	}

	// 详情:条件与关联必须完整(前端"原样回显、整体替换保存",截断会静默丢数据)
	got := getRule(t, c, created.ID)
	if len(got.Conditions) != 2 || got.Conditions[0].Field != "devtype" || got.Conditions[1].Field != "path" {
		t.Fatalf("详情条件 = %+v, want 完整 2 条", got.Conditions)
	}
	if len(got.LinkIDs) != 2 {
		t.Fatalf("详情 linkIds = %v, want 完整 2 条", got.LinkIDs)
	}

	// 改:只改名称,关联与条件必须原样保留(证明"省略 = 不动")
	desc := "改了描述"
	resp := c.patch("/api/rules/"+strconv.FormatInt(created.ID, 10), map[string]any{
		"name": "拦爬虫(改)", "description": desc})
	assertStatus(t, resp, http.StatusOK)
	patched := decodeBody[store.Rule](t, resp)
	if patched.Name != "拦爬虫(改)" || patched.Description != desc {
		t.Fatalf("PATCH 结果 = %+v", patched)
	}
	if len(patched.LinkIDs) != 2 || len(patched.Conditions) != 2 {
		t.Fatalf("只改名称后关联/条件被改写: %v / %+v", patched.LinkIDs, patched.Conditions)
	}

	// 删
	resp = c.del("/api/rules/" + strconv.FormatInt(created.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.get("/api/rules/" + strconv.FormatInt(created.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestRuleLinkIdsReplaceAndClear linkIds 的三种语义:省略=不动、[]=解除全部、[..]=整体替换。
// 这条最容易出静默事故:用裸 []int64 会把 [] 吃成"没传",于是"改成全局"变成
// "先清空关联再改作用域",规则从"作用于全部短链"退化成"一条都不命中"。
func TestRuleLinkIdsReplaceAndClear(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	l1 := createLink(t, c, map[string]any{
		"code": "ka", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})
	l2 := createLink(t, c, map[string]any{
		"code": "kb", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	body := baseRuleBody("可收窄")
	body["scope"] = store.RuleScopeLinks
	body["linkIds"] = []int64{l1.ID, l2.ID}
	rule := createRule(t, c, body)
	path := "/api/rules/" + strconv.FormatInt(rule.ID, 10)

	// 省略 linkIds:关联不动
	resp := c.patch(path, map[string]any{"priority": 7})
	assertStatus(t, resp, http.StatusOK)
	patched := decodeBody[store.Rule](t, resp)
	if patched.Priority != 7 || len(patched.LinkIDs) != 2 {
		t.Fatalf("省略 linkIds 的 PATCH = %+v, want 关联不变", patched)
	}

	// 整体替换为一条
	resp = c.patch(path, map[string]any{"linkIds": []int64{l2.ID}})
	assertStatus(t, resp, http.StatusOK)
	patched = decodeBody[store.Rule](t, resp)
	if len(patched.LinkIDs) != 1 || patched.LinkIDs[0] != l2.ID {
		t.Fatalf("整体替换后 linkIds = %v, want [%d]", patched.LinkIDs, l2.ID)
	}

	// 显式空数组 = 解除全部(零关联是合法状态:该规则永远不命中)
	resp = c.patch(path, map[string]any{"linkIds": []int64{}})
	assertStatus(t, resp, http.StatusOK)
	patched = decodeBody[store.Rule](t, resp)
	if len(patched.LinkIDs) != 0 || patched.LinkCount != 0 {
		t.Fatalf("空数组后 linkIds = %v, want 空", patched.LinkIDs)
	}
	// 零关联的 scoped 规则必须仍是 scope=links(不能被悄悄存成 global)
	if patched.Scope != store.RuleScopeLinks {
		t.Fatalf("scope = %q, want links(零关联不兜底成全局)", patched.Scope)
	}

	// 切回 links 并补回关联,再切到 global:关联必须被清空
	resp = c.patch(path, map[string]any{"linkIds": []int64{l1.ID, l2.ID}})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.patch(path, map[string]any{"scope": store.RuleScopeGlobal})
	assertStatus(t, resp, http.StatusOK)
	patched = decodeBody[store.Rule](t, resp)
	if patched.Scope != store.RuleScopeGlobal || len(patched.LinkIDs) != 0 {
		t.Fatalf("切到 global 后 = %q / %v, want global / 空", patched.Scope, patched.LinkIDs)
	}
}

// TestRuleCreateValidation 逐条 400:契约里列出的 9 个触发条件。
func TestRuleCreateValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	mine := createLink(t, c, map[string]any{
		"code": "vva", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	other := loggedInTenant(t, env, "bob")
	foreign := createLink(t, other, map[string]any{
		"code": "vvb", "targetUrls": []string{"https://a.example.com"},
		"domainIds": []int64{domainIDsOf(t, other)[0]}})

	ok := baseRuleBody("占位")
	ok["scope"] = store.RuleScopeLinks
	ok["linkIds"] = []int64{mine.ID}
	seed := createRule(t, c, ok)
	// 已逻辑删除的短链(关联一条看不见的短链只是误解来源,契约要求 400)
	if resp := c.del("/api/links/" + strconv.FormatInt(mine.ID, 10)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("soft delete link: status = %d", resp.StatusCode)
	}
	_ = seed

	cases := []struct {
		name string
		body map[string]any
	}{
		{"① name 缺失", func() map[string]any {
			b := baseRuleBody("x")
			delete(b, "name")
			return b
		}()},
		{"① name 全空白", baseRuleBody("   ")},
		{"② priority 为负", func() map[string]any {
			b := baseRuleBody("负优先级")
			b["priority"] = -1
			return b
		}()},
		{"③ scope 非法", func() map[string]any {
			b := baseRuleBody("坏作用域")
			b["scope"] = "everything"
			return b
		}()},
		{"④ action 非法", func() map[string]any {
			b := baseRuleBody("坏动作")
			b["action"] = "banish"
			return b
		}()},
		{"④ action 缺失", func() map[string]any {
			b := baseRuleBody("没动作")
			delete(b, "action")
			return b
		}()},
		{"⑤ redirect 缺 destination", func() map[string]any {
			b := baseRuleBody("缺目标")
			b["action"] = store.RuleActionRedirect
			return b
		}()},
		{"⑤ destination 含 CRLF", func() map[string]any {
			b := baseRuleBody("注入目标")
			b["action"] = store.RuleActionRedirect
			b["destination"] = "https://a.example.com/\r\nSet-Cookie: x=1"
			return b
		}()},
		{"⑥ logic 非法", func() map[string]any {
			b := baseRuleBody("坏逻辑")
			b["logic"] = "xor"
			return b
		}()},
		{"⑦ 条件字段不在 v1 字段集", func() map[string]any {
			b := baseRuleBody("坏字段")
			b["conditions"] = []map[string]any{
				{"field": "region", "operator": "eq", "values": []string{"CN"}},
			}
			return b
		}()},
		{"⑧ 运算符不在白名单", func() map[string]any {
			b := baseRuleBody("坏运算符")
			b["conditions"] = []map[string]any{
				{"field": "ua", "operator": "matches", "values": []string{"bot"}},
			}
			return b
		}()},
		{"⑨ linkIds 跨租户", func() map[string]any {
			b := baseRuleBody("跨租户关联")
			b["scope"] = store.RuleScopeLinks
			b["linkIds"] = []int64{foreign.ID}
			return b
		}()},
		{"⑨ linkIds 指向已删除短链", func() map[string]any {
			b := baseRuleBody("关联已删除")
			b["scope"] = store.RuleScopeLinks
			b["linkIds"] = []int64{mine.ID}
			return b
		}()},
		{"⑨ linkIds 非正整数", func() map[string]any {
			b := baseRuleBody("非法 id")
			b["scope"] = store.RuleScopeLinks
			b["linkIds"] = []int64{0}
			return b
		}()},
	}
	for _, tc := range cases {
		resp := c.post("/api/rules", tc.body)
		if resp.StatusCode != http.StatusBadRequest {
			errBody := decodeBody[httpapi.ErrorBody](t, resp)
			t.Errorf("%s: status = %d, want 400(%s)", tc.name, resp.StatusCode, errBody.Message)
		}
		_ = resp.Body.Close()
	}
	// 非法请求不得留下任何规则
	if page := listRules(t, c); page.Total != 1 {
		t.Errorf("非法创建后规则数 = %d, want 1(只有种子规则)", page.Total)
	}
}

// TestRuleDuplicateNameConflict 同租户重名 409;PATCH 改成别人的名字同样 409。
func TestRuleDuplicateNameConflict(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	createRule(t, c, baseRuleBody("重名"))
	resp := c.post("/api/rules", baseRuleBody("重名"))
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()

	second := createRule(t, c, baseRuleBody("另一个"))
	resp = c.patch("/api/rules/"+strconv.FormatInt(second.ID, 10), map[string]any{"name": "重名"})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

// TestRuleTenantIsolation 规则不存在/不属于本租户一律 404(不泄露"别人家确实有这条")。
func TestRuleTenantIsolation(t *testing.T) {
	env := testutil.Setup(t)
	alice := loggedInTenant(t, env, "alice")
	mine := createRule(t, alice, baseRuleBody("私有的"))
	bob := loggedInTenant(t, env, "bob")
	path := "/api/rules/" + strconv.FormatInt(mine.ID, 10)

	resp := bob.get(path)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	resp = bob.patch(path, map[string]any{"name": "改别人的"})
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	resp = bob.del(path)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()

	// 确认原规则没被动过
	if got := getRule(t, alice, mine.ID); got.Name != "私有的" {
		t.Fatalf("原规则被跨租户改写: %q", got.Name)
	}
	// 列表也互不可见
	if page := listRules(t, bob); page.Total != 0 {
		t.Fatalf("他人租户看到 %d 条规则, want 0", page.Total)
	}
}

// TestRuleQuotaExceeded 单租户规则数达上限后拒绝创建(403);上限是快照规模的护栏。
func TestRuleQuotaExceeded(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	tenantID := tenantIDOf(t, c)
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`INSERT INTO rules (tenant_id, name, action) SELECT $1, 'bulk-'||g, 'pass'
		 FROM generate_series(1, $2) g`, tenantID, store.MaxRulesPerTenant); err != nil {
		t.Fatalf("bulk insert rules: %v", err)
	}
	resp := c.post("/api/rules", baseRuleBody("第 201 条"))
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != errQuotaCode {
		t.Fatalf("code = %q, want %q", body.Code, errQuotaCode)
	}
	// 改已有规则不受配额限制(配额只挡新建)
	if page := listRules(t, c); page.Total != store.MaxRulesPerTenant {
		t.Fatalf("规则数 = %d, want %d", page.Total, store.MaxRulesPerTenant)
	}
}

// errQuotaCode 配额错误的 code(响应体断言用)。
const errQuotaCode = "E_QUOTA"

// TestRulePatchKeepsZeroPriority PATCH 不带 priority 时必须保留现值——
// priority=0 是合法配置(数字小者先评估),不能被"零值即未设置"的写法重置成默认值。
func TestRulePatchKeepsZeroPriority(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	rule := createRule(t, c, func() map[string]any {
		b := baseRuleBody("最早评估")
		b["priority"] = 0
		return b
	}())
	if rule.Priority != 0 {
		t.Fatalf("新建的 priority = %d, want 0", rule.Priority)
	}
	resp := c.patch("/api/rules/"+strconv.FormatInt(rule.ID, 10),
		map[string]any{"description": "只改描述"})
	assertStatus(t, resp, http.StatusOK)
	patched := decodeBody[store.Rule](t, resp)
	if patched.Priority != 0 {
		t.Fatalf("PATCH 后 priority = %d, want 0(现值必须被保留)", patched.Priority)
	}
	// 显式传 0 也必须写得进去
	resp = c.patch("/api/rules/"+strconv.FormatInt(rule.ID, 10),
		map[string]any{"priority": 7})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.patch("/api/rules/"+strconv.FormatInt(rule.ID, 10),
		map[string]any{"priority": 0})
	assertStatus(t, resp, http.StatusOK)
	patched = decodeBody[store.Rule](t, resp)
	if patched.Priority != 0 {
		t.Fatalf("显式置 0 后 priority = %d, want 0", patched.Priority)
	}
}

// TestRuleListTruncatesLinkNamesOnly 列表项只截断 linkNames,linkIds 与 conditions 保持完整。
func TestRuleListTruncatesLinkNamesOnly(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	ids := make([]int64, 0, 4)
	for _, code := range []string{"na", "nb", "nc", "nd"} {
		l := createLink(t, c, map[string]any{
			"code": code, "targetUrls": []string{"https://a.example.com"},
			"domainIds": []int64{lid}})
		ids = append(ids, l.ID)
	}
	body := baseRuleBody("四条关联")
	body["scope"] = store.RuleScopeLinks
	body["linkIds"] = ids
	body["conditions"] = []map[string]any{
		{"field": "path", "operator": "eq", "values": []string{"/na"}},
		{"field": "devtype", "operator": "eq", "values": []string{"bot"}},
	}
	rule := createRule(t, c, body)

	page := listRules(t, c)
	if len(page.Items) != 1 {
		t.Fatalf("列表条数 = %d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if len(item.LinkNames) != 3 {
		t.Fatalf("列表 linkNames = %v, want 前 3 个", item.LinkNames)
	}
	if item.LinkCount != 4 {
		t.Fatalf("列表 linkCount = %d, want 4(计数是全量)", item.LinkCount)
	}
	if len(item.LinkIDs) != 4 {
		t.Fatalf("列表 linkIds = %v, want 完整 4 条", item.LinkIDs)
	}
	if len(item.Conditions) != 2 {
		t.Fatalf("列表 conditions = %+v, want 完整 2 条", item.Conditions)
	}
	// 详情同样:linkIds 完整
	if got := getRule(t, c, rule.ID); len(got.LinkIDs) != 4 {
		t.Fatalf("详情 linkIds = %v, want 完整 4 条", got.LinkIDs)
	}
}

// TestRuleOptions 精简下拉列表:含 hits24h 与作用域,顺序与求值顺序一致。
func TestRuleOptions(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	createRule(t, c, func() map[string]any {
		b := baseRuleBody("先评估")
		b["priority"] = 5
		return b
	}())
	createRule(t, c, func() map[string]any {
		b := baseRuleBody("后评估")
		b["priority"] = 50
		b["scope"] = store.RuleScopeLinks
		return b
	}())

	resp := c.get("/api/rules/options")
	assertStatus(t, resp, http.StatusOK)
	items := decodeBody[[]ruleOptionItem](t, resp)
	if len(items) != 2 {
		t.Fatalf("options = %+v, want 2 条", items)
	}
	if items[0].Name != "先评估" || items[0].Scope != store.RuleScopeGlobal || items[0].Hits24h != 0 {
		t.Fatalf("options[0] = %+v", items[0])
	}
	if items[1].Scope != store.RuleScopeLinks {
		t.Fatalf("options[1].scope = %q, want links", items[1].Scope)
	}
	// 别人的规则不进下拉
	other := loggedInTenant(t, env, "bob")
	resp = other.get("/api/rules/options")
	assertStatus(t, resp, http.StatusOK)
	if got := decodeBody[[]ruleOptionItem](t, resp); len(got) != 0 {
		t.Fatalf("他人租户 options = %+v, want 空", got)
	}
}

// TestLinkRulesList 短链适用规则 = 全局继承 + 显式关联,source 是权威字段。
func TestLinkRulesList(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	l1 := createLink(t, c, map[string]any{
		"code": "zra", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})
	l2 := createLink(t, c, map[string]any{
		"code": "zrb", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	global := createRule(t, c, func() map[string]any {
		b := baseRuleBody("全局规则")
		b["priority"] = 20
		return b
	}())
	scoped := createRule(t, c, func() map[string]any {
		b := baseRuleBody("本条专属")
		b["priority"] = 10
		b["scope"] = store.RuleScopeLinks
		b["linkIds"] = []int64{l1.ID}
		return b
	}())
	elsewhere := createRule(t, c, func() map[string]any {
		b := baseRuleBody("别的短链")
		b["scope"] = store.RuleScopeLinks
		b["linkIds"] = []int64{l2.ID}
		return b
	}())
	// 停用规则也要看得见("适用"与"启用"是两件事)
	disabled := createRule(t, c, func() map[string]any {
		b := baseRuleBody("停用的")
		b["enabled"] = false
		b["scope"] = store.RuleScopeLinks
		b["linkIds"] = []int64{l1.ID}
		return b
	}())
	// 两条短链共用的规则:linkCount 必须是 2,否则短链列表页关掉它时没法告知影响面
	shared := createRule(t, c, func() map[string]any {
		b := baseRuleBody("两条共用")
		b["priority"] = 30
		b["scope"] = store.RuleScopeLinks
		b["linkIds"] = []int64{l1.ID, l2.ID}
		return b
	}())

	resp := c.get(fmt.Sprintf("/api/links/%d/rules", l1.ID))
	assertStatus(t, resp, http.StatusOK)
	items := decodeBody[linkRulesResp](t, resp).Items
	byID := make(map[int64]linkRuleItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	if len(items) != 4 {
		t.Fatalf("l1 适用规则 = %+v, want 4(全局 + 三条 scoped)", items)
	}
	if byID[global.ID].Source != "inherited" {
		t.Errorf("全局规则 source = %q, want inherited", byID[global.ID].Source)
	}
	if byID[global.ID].LinkCount != 0 {
		t.Errorf("全局规则 linkCount = %d, want 0", byID[global.ID].LinkCount)
	}
	if byID[scoped.ID].LinkCount != 1 {
		t.Errorf("只关联 l1 的规则 linkCount = %d, want 1", byID[scoped.ID].LinkCount)
	}
	if byID[shared.ID].LinkCount != 2 {
		t.Errorf("与 l1/l2 共用的规则 linkCount = %d, want 2", byID[shared.ID].LinkCount)
	}
	if byID[scoped.ID].Source != "scoped" || byID[disabled.ID].Source != "scoped" {
		t.Errorf("scoped 规则 source 标错: %+v", items)
	}
	if byID[disabled.ID].Enabled {
		t.Errorf("停用规则的 enabled = true")
	}
	if _, ok := byID[elsewhere.ID]; ok {
		t.Errorf("与本短链无关的规则出现在适用列表里: %+v", elsewhere.ID)
	}
	// 顺序按求值顺序(priority 升序):专属(10) → 全局(20) → 共用(30) → 停用的(100)
	if items[0].ID != scoped.ID || items[1].ID != global.ID || items[2].ID != shared.ID {
		t.Errorf("适用规则顺序 = %v, want 按 priority 升序", items)
	}
	// 短链不存在/他人短链 → 404
	resp = c.get(fmt.Sprintf("/api/links/%d/rules", l2.ID+99999))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	other := loggedInTenant(t, env, "bob")
	resp = other.get(fmt.Sprintf("/api/links/%d/rules", l1.ID))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestPutLinkRules 设置短链关联:只接受 scope=links,混入全局/跨租户/不存在一律 400。
func TestPutLinkRules(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	l1 := createLink(t, c, map[string]any{
		"code": "pra", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})
	l2 := createLink(t, c, map[string]any{
		"code": "prb", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	global := createRule(t, c, baseRuleBody("全局"))
	r1 := createRule(t, c, func() map[string]any {
		b := baseRuleBody("限定一")
		b["scope"] = store.RuleScopeLinks
		return b
	}())
	r2 := createRule(t, c, func() map[string]any {
		b := baseRuleBody("限定二")
		b["scope"] = store.RuleScopeLinks
		return b
	}())
	other := loggedInTenant(t, env, "bob")
	foreignRule := createRule(t, other, func() map[string]any {
		b := baseRuleBody("别人的")
		b["scope"] = store.RuleScopeLinks
		return b
	}())

	put := func(linkID int64, body map[string]any) *http.Response {
		return c.put("/api/links/"+strconv.FormatInt(linkID, 10)+"/rules", body)
	}
	scopedIDs := func(resp *http.Response) []int64 {
		var out []int64
		for _, it := range decodeBody[linkRulesResp](t, resp).Items {
			if it.Source == "scoped" {
				out = append(out, it.ID)
			}
		}
		return out
	}

	// 正常设置
	resp := put(l1.ID, map[string]any{"ruleIds": []int64{r1.ID, r2.ID}})
	assertStatus(t, resp, http.StatusOK)
	if got := scopedIDs(resp); len(got) != 2 {
		t.Fatalf("设置后 scoped = %v, want 2 条", got)
	}
	// 整体替换:只留一条
	resp = put(l1.ID, map[string]any{"ruleIds": []int64{r2.ID}})
	assertStatus(t, resp, http.StatusOK)
	if got := scopedIDs(resp); len(got) != 1 || got[0] != r2.ID {
		t.Fatalf("整体替换后 scoped = %v, want [%d]", got, r2.ID)
	}
	// 空数组 = 解除全部
	resp = put(l1.ID, map[string]any{"ruleIds": []int64{}})
	assertStatus(t, resp, http.StatusOK)
	if got := scopedIDs(resp); len(got) != 0 {
		t.Fatalf("空数组后 scoped = %v, want 空", got)
	}
	// 解除后另一条短链不受影响
	resp = c.get(fmt.Sprintf("/api/links/%d/rules", l2.ID))
	assertStatus(t, resp, http.StatusOK)

	// 三种 400
	for _, tc := range []struct {
		name string
		ids  []int64
	}{
		{"混入全局规则", []int64{global.ID}},
		{"跨租户规则", []int64{foreignRule.ID}},
		{"不存在的规则", []int64{99999999}},
	} {
		resp := put(l1.ID, map[string]any{"ruleIds": tc.ids})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	// 失败写入不得留下半条关联
	resp = c.get(fmt.Sprintf("/api/links/%d/rules", l1.ID))
	assertStatus(t, resp, http.StatusOK)
	if got := scopedIDs(resp); len(got) != 0 {
		t.Fatalf("失败写入后 scoped = %v, want 空", got)
	}
	// 短链不存在 / 他人短链 → 404
	resp = put(l1.ID+99999, map[string]any{"ruleIds": []int64{r1.ID}})
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	otherResp := other.put("/api/links/"+strconv.FormatInt(l1.ID, 10)+"/rules",
		map[string]any{"ruleIds": []int64{foreignRule.ID}})
	assertStatus(t, otherResp, http.StatusNotFound)
	_ = otherResp.Body.Close()
}

// TestLinkListRuleMeta 短链列表/详情带出适用规则数与前 3 个规则名。
func TestLinkListRuleMeta(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	l1 := createLink(t, c, map[string]any{
		"code": "zma", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})
	l2 := createLink(t, c, map[string]any{
		"code": "zmb", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{lid}})

	createRule(t, c, func() map[string]any {
		b := baseRuleBody("全局一")
		b["priority"] = 10
		return b
	}())
	createRule(t, c, func() map[string]any {
		b := baseRuleBody("全局二")
		b["priority"] = 20
		return b
	}())
	// 三条 scoped 只关联 l1(最后一条停用:行内开关要能拿到 enabled)
	for i, name := range []string{"sc1", "sc2", "sc3"} {
		createRule(t, c, func() map[string]any {
			b := baseRuleBody(name)
			b["priority"] = 30 + i
			b["scope"] = store.RuleScopeLinks
			b["linkIds"] = []int64{l1.ID}
			if name == "sc3" {
				b["enabled"] = false
			}
			return b
		}())
	}

	byID := func(links []*store.Link) *store.Link {
		for _, l := range links {
			if l.ID == l1.ID {
				return l
			}
		}
		t.Fatal("列表里没有该短链")
		return nil
	}
	listItem := byID(listLinks(t, c))
	if listItem.RuleCount != 5 {
		t.Errorf("列表 ruleCount = %d, want 5(2 全局 + 3 scoped)", listItem.RuleCount)
	}
	if len(listItem.RuleNames) != 3 {
		t.Errorf("列表 ruleNames = %v, want 前 3 个", listItem.RuleNames)
	}
	if listItem.RuleNames[0] != "全局一" {
		t.Errorf("ruleNames[0] = %q, want 全局一(按 priority 升序)", listItem.RuleNames[0])
	}
	// 行内开关的数据:只给显式关联的 scoped 规则,全局规则不进这里(单行里改不得)
	if len(listItem.Rules) != 3 {
		t.Fatalf("列表 rules = %+v, want 3 条 scoped", listItem.Rules)
	}
	for i, want := range []string{"sc1", "sc2", "sc3"} {
		if listItem.Rules[i].Name != want {
			t.Errorf("rules[%d].Name = %q, want %q(按 priority 升序)", i, listItem.Rules[i].Name, want)
		}
		if listItem.Rules[i].Scope != store.RuleScopeLinks {
			t.Errorf("rules[%d].Scope = %q, want links", i, listItem.Rules[i].Scope)
		}
	}
	if listItem.Rules[2].Enabled {
		t.Errorf("停用的 sc3 在列表里 enabled = true")
	}
	// 详情口径必须一致
	resp := c.get("/api/links/" + strconv.FormatInt(l1.ID, 10))
	assertStatus(t, resp, http.StatusOK)
	detail := decodeBody[store.Link](t, resp)
	if detail.RuleCount != 5 || len(detail.RuleNames) != 3 {
		t.Errorf("详情规则元数据 = %d / %v, want 5 / 3 个", detail.RuleCount, detail.RuleNames)
	}
	// 关联变化后元数据随之变化
	ruleIDs, err := env.Pool.Query(testutil.Ctx(),
		`SELECT id FROM rules WHERE tenant_id=$1 AND scope='links'`, tenantIDOf(t, c))
	if err != nil {
		t.Fatalf("query scoped rules: %v", err)
	}
	var first int64
	for ruleIDs.Next() {
		if err := ruleIDs.Scan(&first); err != nil {
			t.Fatalf("scan rule id: %v", err)
		}
		break
	}
	ruleIDs.Close()
	resp = c.put("/api/links/"+strconv.FormatInt(l1.ID, 10)+"/rules",
		map[string]any{"ruleIds": []int64{first}})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.get("/api/links/" + strconv.FormatInt(l1.ID, 10))
	assertStatus(t, resp, http.StatusOK)
	detail = decodeBody[store.Link](t, resp)
	if detail.RuleCount != 3 { // 2 全局 + 1 scoped
		t.Errorf("解除两条关联后 ruleCount = %d, want 3", detail.RuleCount)
	}
	// l2 只继承两条全局规则:Rules 为空(不是 null),界面才能直接 map
	resp = c.get("/api/links/" + strconv.FormatInt(l2.ID, 10))
	assertStatus(t, resp, http.StatusOK)
	detail2 := decodeBody[store.Link](t, resp)
	if detail2.RuleCount != 2 {
		t.Errorf("l2 ruleCount = %d, want 2(仅全局)", detail2.RuleCount)
	}
	if detail2.Rules == nil {
		t.Errorf("l2 rules = null, want 空数组")
	}
	if len(detail2.Rules) != 0 {
		t.Errorf("l2 rules = %+v, want 空(全局规则不给开关)", detail2.Rules)
	}
}
