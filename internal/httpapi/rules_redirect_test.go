package httpapi_test

// 跳转链路接入规则求值(05)的黑盒测试。
//
// 裁决顺序(spec D4)在这里被逐条锁住,因为它的每一步顺序错了都不会报错、只会静默走偏:
//
//	短链不可用(停用/已删/无目标/落地页缺失) → 404 + 失败明细      ← 规则不参与
//	规则求值(priority 升序,首条命中即定) → redirect / notfound / throttle / pass
//	未命中 → 原跳转流程
//
// 另有两条硬约束:
//   - 访问次数口径不变:被规则拦下的访问是 failed,不灌水访问量;
//   - 命中零写放大:命中只多写一行明细,不更新任何计数表
//     (「24h 命中」由列表接口从明细读时聚合出来)。

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"cloak/internal/httpapi"
	"cloak/internal/rules"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

// ruleOnPath 造一条「该短码一访问就命中」的规则条件。
// 用 path 而不是 UA:请求头要靠测试逐个构造,而短码路径是确定性的,少一处不确定性。
func ruleOnPath(code string) []map[string]any {
	return []map[string]any{
		{"field": "path", "operator": "eq", "values": []string{"/" + code}},
	}
}

// ruleVisit 读某短链最新一行明细里的规则字段。
func ruleVisit(t *testing.T, env *testutil.Env, linkID int64) (action, outcome, reason string, ruleID *int64, ruleAction string) {
	t.Helper()
	err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT action, outcome, reason, rule_id, rule_action FROM visits
		  WHERE link_id=$1 ORDER BY id DESC LIMIT 1`, linkID).
		Scan(&action, &outcome, &reason, &ruleID, &ruleAction)
	if err != nil {
		t.Fatalf("read latest visit: %v", err)
	}
	return action, outcome, reason, ruleID, ruleAction
}

// TestRuleRedirectRewritesTarget 裁决 redirect:改写 Location 为规则的 destination,
// 不参与目标轮询(轮询是"在目标池里选",规则改写是"换一条路"),记一行成功明细。
func TestRuleRedirectRewritesTarget(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "rpa",
		"targetUrls": []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":  []int64{lid},
	})
	rule := createRule(t, c, map[string]any{
		"name": "改写到活动页", "action": store.RuleActionRedirect,
		"destination": "https://campaign.example.com/promo",
		"conditions":  ruleOnPath(link.Code),
	})

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://campaign.example.com/promo" {
		t.Fatalf("Location = %q, want 规则 destination", loc)
	}

	action, outcome, reason, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeSuccess || reason != "" {
		t.Fatalf("明细 = %q/%q/%q, want redirect/success/空", action, outcome, reason)
	}
	if ruleID == nil || *ruleID != rule.ID || ruleAction != store.RuleActionRedirect {
		t.Fatalf("明细里的裁决 = %v/%q, want %d/redirect", ruleID, ruleAction, rule.ID)
	}
	// 改写目标的访问确实重定向出去了 → 计入访问次数
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 1 {
		t.Fatalf("link.visits = %v, want 1", got)
	}

	// 301 型短链:规则改写同样走 301
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10),
		map[string]any{"redirectStatus": 301})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusMovedPermanently)
	if loc := resp.Header.Get("Location"); loc != "https://campaign.example.com/promo" {
		t.Fatalf("301 Location = %q", loc)
	}
}

// TestRuleNotfoundAndThrottle 裁决 notfound → 404 + failed/rule_blocked;
// throttle → 429 + failed/rule_throttled。两者都不计入访问次数。
func TestRuleNotfoundAndThrottle(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)

	blocked := createLink(t, c, map[string]any{
		"code": "rpb", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	throttled := createLink(t, c, map[string]any{
		"code": "rpc", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	// 基线:各一次成功访问,证明后面的"没涨"是规则拦下的
	for _, code := range []string{blocked.Code, throttled.Code} {
		resp := redirectGet(t, env, "localhost", "/"+code)
		assertStatus(t, resp, http.StatusFound)
	}
	ruleBlocked := createRule(t, c, map[string]any{
		"name": "拦下", "action": store.RuleActionNotfound, "conditions": ruleOnPath(blocked.Code)})
	createRule(t, c, map[string]any{
		"name": "限流", "action": store.RuleActionThrottle, "conditions": ruleOnPath(throttled.Code)})

	resp := redirectGet(t, env, "localhost", "/"+blocked.Code)
	assertStatus(t, resp, http.StatusNotFound)
	action, outcome, reason, ruleID, ruleAction := ruleVisit(t, env, blocked.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonRuleBlocked {
		t.Fatalf("被拦下的明细 = %q/%q/%q, want redirect/failed/rule_blocked", action, outcome, reason)
	}
	if ruleID == nil || *ruleID != ruleBlocked.ID || ruleAction != store.RuleActionNotfound {
		t.Fatalf("被拦下的裁决 = %v/%q", ruleID, ruleAction)
	}
	if got := linkStats(t, c, blocked.ID)["visits"].(float64); got != 1 {
		t.Fatalf("link.visits = %v, want 1(被拦下的不灌水)", got)
	}
	// 明细里必须带出规则裁决(否则租户看不出这次 404 是规则干的)
	if vp := listVisits(t, c, blocked.ID, ""); vp.Items[0].RuleID == nil ||
		vp.Items[0].RuleAction != store.RuleActionNotfound {
		t.Fatalf("访问列表缺规则字段: %+v", vp.Items[0])
	}

	resp = redirectGet(t, env, "localhost", "/"+throttled.Code)
	assertStatus(t, resp, http.StatusTooManyRequests)
	action, outcome, reason, _, ruleAction = ruleVisit(t, env, throttled.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonRuleThrottled || ruleAction != store.RuleActionThrottle {
		t.Fatalf("被限流的明细 = %q/%q/%q/%q", action, outcome, reason, ruleAction)
	}
	if got := linkStats(t, c, throttled.ID)["visits"].(float64); got != 1 {
		t.Fatalf("link.visits = %v, want 1(被限流的不灌水)", got)
	}
}

// TestRulePassAndNoMatch 原流程不受影响:pass 与未命中都继续走原目标选择。
func TestRulePassAndNoMatch(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "rpd",
		"targetUrls": []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":  []int64{lid},
	})
	// pass 规则:命中只被记录,不改写结果
	pass := createRule(t, c, map[string]any{
		"name": "放行并记录", "action": store.RuleActionPass, "conditions": ruleOnPath(link.Code)})

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Fatalf("pass 规则的 Location = %q, want 原目标", loc)
	}
	action, outcome, _, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeSuccess {
		t.Fatalf("pass 明细 = %q/%q, want redirect/success", action, outcome)
	}
	if ruleID == nil || *ruleID != pass.ID || ruleAction != store.RuleActionPass {
		t.Fatalf("pass 明细的裁决 = %v/%q, want %d/pass", ruleID, ruleAction, pass.ID)
	}
	// 停用该规则后再访问:未命中任何规则,明细不带规则字段
	resp = c.patch("/api/rules/"+strconv.FormatInt(pass.ID, 10), map[string]any{"enabled": false})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	_, _, _, ruleID, ruleAction = ruleVisit(t, env, link.ID)
	if ruleID != nil || ruleAction != "" {
		t.Fatalf("停用后仍有规则裁决: %v/%q", ruleID, ruleAction)
	}
}

// TestRuleScopeAppliesOnlyToLinkedLinks 作用域:links 只对关联的短链生效;
// 零关联的 scoped 规则永不命中(spec D2:不兜底成全局)。
func TestRuleScopeAppliesOnlyToLinkedLinks(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	target := createLink(t, c, map[string]any{
		"code": "rpe", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	other := createLink(t, c, map[string]any{
		"code": "rpf", "targetUrls": []string{"https://t2.example.com"}, "domainIds": []int64{lid}})

	createRule(t, c, map[string]any{
		"name": "只管一条", "action": store.RuleActionNotfound,
		"scope": store.RuleScopeLinks, "linkIds": []int64{target.ID},
		"conditions": []map[string]any{
			{"field": "path", "operator": "in", "values": []string{"/" + target.Code, "/" + other.Code}},
		},
	})
	// 零关联的 scoped 规则:条件一样匹配,也不会生效
	createRule(t, c, map[string]any{
		"name": "未关联", "action": store.RuleActionNotfound,
		"scope": store.RuleScopeLinks,
		"conditions": []map[string]any{
			{"field": "path", "operator": "in", "values": []string{"/" + other.Code}},
		},
	})

	resp := redirectGet(t, env, "localhost", "/"+target.Code)
	assertStatus(t, resp, http.StatusNotFound)
	resp = redirectGet(t, env, "localhost", "/"+other.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t2.example.com" {
		t.Fatalf("未关联的短链 Location = %q, want 原目标(零关联规则不兜底)", loc)
	}
}

// TestRuleFirstMatchWins 首条命中即裁决:两条都命中的规则,按 priority 升序取第一条,
// 后面的不再参与(不做叠加,避免放行规则把拦截救回来)。
func TestRuleFirstMatchWins(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "rpg", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	// 先建优先级大的(数字大者后评估),再建优先级小的
	later := createRule(t, c, map[string]any{
		"name": "后评估-放行", "priority": 50, "action": store.RuleActionPass,
		"conditions": ruleOnPath(link.Code)})
	first := createRule(t, c, map[string]any{
		"name": "先评估-拦截", "priority": 10, "action": store.RuleActionNotfound,
		"conditions": ruleOnPath(link.Code)})
	_ = later

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	_, _, _, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if ruleID == nil || *ruleID != first.ID || ruleAction != store.RuleActionNotfound {
		t.Fatalf("裁决 = %v/%q, want %d/notfound(优先级小的那条)", ruleID, ruleAction, first.ID)
	}
}

// TestRuleRanksAfterLinkAvailability 规则排在短链可用性之后:
// 短链自己已经 404 的访问不参与裁决,明细里也不带规则字段。
func TestRuleRanksAfterLinkAvailability(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	disabled := createLink(t, c, map[string]any{
		"code": "rph", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	noTarget := createLink(t, c, map[string]any{
		"code": "rpi", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	// 一条全局的 notfound 规则(对所有短链都匹配):若规则先于可用性判断,两次都会变成 rule_blocked
	rule := createRule(t, c, map[string]any{
		"name": "全局拦截", "action": store.RuleActionNotfound, "conditions": ruleOnPath(disabled.Code)})
	createRule(t, c, map[string]any{
		"name": "全局拦截2", "action": store.RuleActionNotfound, "conditions": ruleOnPath(noTarget.Code)})

	// 停用
	resp := c.patch("/api/links/"+strconv.FormatInt(disabled.ID, 10),
		map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+disabled.Code)
	assertStatus(t, resp, http.StatusNotFound)
	_, outcome, reason, ruleID, ruleAction := ruleVisit(t, env, disabled.ID)
	if outcome != store.VisitOutcomeFailed || reason != store.VisitReasonLinkDisabled {
		t.Fatalf("停用短链的明细 = %q/%q, want failed/link_disabled", outcome, reason)
	}
	if ruleID != nil || ruleAction != "" {
		t.Fatalf("短链不可用时仍参与了规则裁决: %v/%q", ruleID, ruleAction)
	}

	// 无目标(目标列表被清空)
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`DELETE FROM link_targets WHERE link_id=$1`, noTarget.ID); err != nil {
		t.Fatalf("clear targets: %v", err)
	}
	resp = redirectGet(t, env, "localhost", "/"+noTarget.Code)
	assertStatus(t, resp, http.StatusNotFound)
	_, outcome, reason, ruleID, ruleAction = ruleVisit(t, env, noTarget.ID)
	if outcome != store.VisitOutcomeFailed || reason != store.VisitReasonNoTarget {
		t.Fatalf("无目标短链的明细 = %q/%q, want failed/no_target", outcome, reason)
	}
	if ruleID != nil || ruleAction != "" {
		t.Fatalf("无目标时仍参与了规则裁决: %v/%q", ruleID, ruleAction)
	}
	_ = rule
}

// TestRuleAppliesToLandingLink 落地页型短链同样参与裁决(规则是租户级的,可以限定到任意短链)。
func TestRuleAppliesToLandingLink(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "rpk", "url", "https://page.example.com/lp")
	rule := createRule(t, c, map[string]any{
		"name": "落地页也拦", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	action, outcome, reason, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if action != store.VisitActionLandingView || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonRuleBlocked {
		t.Fatalf("落地页被拦下的明细 = %q/%q/%q, want landing_view/failed/rule_blocked", action, outcome, reason)
	}
	if ruleID == nil || *ruleID != rule.ID || ruleAction != store.RuleActionNotfound {
		t.Fatalf("裁决 = %v/%q, want %d/notfound", ruleID, ruleAction, rule.ID)
	}
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 0 {
		t.Fatalf("link.visits = %v, want 0(被拦下的落地页视图不灌水)", got)
	}
}

// TestRuleHits24hFromVisits 「24h 命中」读时聚合:成功的与被拦下的都算,
// 没有规则参与的访问不计入任何规则。
func TestRuleHits24hFromVisits(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "rpq", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	rule := createRule(t, c, map[string]any{
		"name": "命中计数", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})
	createRule(t, c, baseRuleBody("没人命中"))

	for i := 0; i < 3; i++ {
		assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)
	}
	// 未关联/未命中的规则恒为 0
	if got := listRules(t, c); len(got.Items) != 2 {
		t.Fatalf("规则数 = %d, want 2", len(got.Items))
	}
	page := listRules(t, c)
	hits := map[string]int64{}
	for _, item := range page.Items {
		hits[item.Name] = item.Hits24h
	}
	if hits["命中计数"] != 3 {
		t.Fatalf("命中计数的 hits24h = %d, want 3", hits["命中计数"])
	}
	if hits["没人命中"] != 0 {
		t.Fatalf("没命中的规则 hits24h = %d, want 0", hits["没人命中"])
	}
	// 详情里的 hits24h 与列表一致
	if got := getRule(t, c, rule.ID); got.Hits24h != 3 {
		t.Fatalf("详情 hits24h = %d, want 3", got.Hits24h)
	}
	// 下拉项也带 hits24h
	resp := c.get("/api/rules/options")
	assertStatus(t, resp, http.StatusOK)
	for _, o := range decodeBody[[]ruleOptionItem](t, resp) {
		if o.Name == "命中计数" && o.Hits24h != 3 {
			t.Fatalf("options hits24h = %d, want 3", o.Hits24h)
		}
	}
}

// TestRuleChangeTakesEffectImmediately 规则变更/关联变更后快照必须失效,
// 否则刚保存的规则要等到 TTL 兜底才生效(spec D7)。
func TestRuleChangeTakesEffectImmediately(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "rpm", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	// 先访问一次:此时还没有规则,快照被缓存下来
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusFound)
	// 之后新建一条必定命中的规则 → 立刻生效
	rule := createRule(t, c, map[string]any{
		"name": "新建即拦", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code)})
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)

	// 改成 redirect → 立刻改写目标
	resp := c.patch("/api/rules/"+strconv.FormatInt(rule.ID, 10), map[string]any{
		"action": store.RuleActionRedirect, "destination": "https://other.example.com/"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://other.example.com/" {
		t.Fatalf("改写后 Location = %q", loc)
	}

	// 收窄成"指定短链"并挂上本短链:仍是 redirect 裁决,立刻改写目标
	resp = c.patch("/api/rules/"+strconv.FormatInt(rule.ID, 10),
		map[string]any{"scope": store.RuleScopeLinks, "linkIds": []int64{link.ID}})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://other.example.com/" {
		t.Fatalf("收窄后 Location = %q, want 改写目标", loc)
	}

	// 从短链侧解除全部关联 → 立刻回到原跳转流程
	resp = c.put("/api/links/"+strconv.FormatInt(link.ID, 10)+"/rules",
		map[string]any{"ruleIds": []int64{}})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Fatalf("解除关联后 Location = %q, want 原目标", loc)
	}
}

// TestRuleSnapshotLoadFailureFailsOpen 快照加载失败(fail-open):短链照常跳转,
// 不能因为规则系统的故障把线上短链打成 500。
func TestRuleSnapshotLoadFailureFailsOpen(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "rpn", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})

	// 注入一个只会失败的 Loader(模拟数据库抖动),静默日志不刷屏
	broken := rules.NewCache(func(context.Context, int64) ([]store.Rule, error) {
		return nil, errors.New("模拟的数据库故障")
	}, rules.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store: env.Store, Cfg: env.Cfg, RuleCache: broken,
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/"+link.Code, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost"
	cli := srv.Client()
	cli.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("redirect GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302(快照加载失败必须放行)", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Fatalf("Location = %q, want 原目标", loc)
	}
	// 放行的访问照常记账(没有规则参与)
	_, _, _, ruleID, ruleAction := ruleVisit(t, env, link.ID)
	if ruleID != nil || ruleAction != "" {
		t.Fatalf("放行时却记了规则裁决: %v/%q", ruleID, ruleAction)
	}
}
