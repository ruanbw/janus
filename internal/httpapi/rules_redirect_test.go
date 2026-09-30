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
	"strings"
	"sync"
	"testing"

	"cloak/internal/geo"
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

// fakeGeo 可变的地理值桩:同一套代码里先给一个国家、再换另一个,
// 用来区分"规则匹配上了"和"规则因为 country 有值而匹配上了"。
type fakeGeo struct {
	mu   sync.Mutex
	info geo.Info
}

func (g *fakeGeo) Lookup(string) geo.Info {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.info
}

func (g *fakeGeo) set(info geo.Info) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.info = info
}

// TestRuleCountryConditionUsesInjectedGeo 锁住地理链路端到端接通:
// 注入的 country 参与规则求值,并且写进访问明细与访问列表接口。
//
// 这条链路断掉时不会报错,只会静默退化成"所有 country 规则都不命中"
// (取不到值恒不命中,ADR 0009),界面上表现为国家一栏永远是 "—"。
func TestRuleCountryConditionUsesInjectedGeo(t *testing.T) {
	g := &fakeGeo{info: geo.Info{Country: "US"}}
	env := testutil.SetupWithGeo(t, g)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "rgeo", "targetUrls": []string{"https://t1.example.com"}, "domainIds": []int64{lid}})
	createRule(t, c, map[string]any{
		"name": "美国拦下", "action": store.RuleActionNotfound, "conditions": []map[string]any{
			{"field": "path", "operator": "eq", "values": []string{"/" + link.Code}},
			{"field": "country", "operator": "in", "values": []string{"US"}},
		}})

	// country=US:命中 → 404
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)
	_, _, reason, _, _ := ruleVisit(t, env, link.ID)
	if reason != store.VisitReasonRuleBlocked {
		t.Fatalf("reason = %q, want %q(country=US 应当命中)", reason, store.VisitReasonRuleBlocked)
	}
	var country string
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT country FROM visits WHERE link_id=$1 ORDER BY id DESC LIMIT 1`, link.ID).
		Scan(&country); err != nil {
		t.Fatalf("read visit country: %v", err)
	}
	if country != "US" {
		t.Fatalf("明细 country = %q, want US(裁决用的国家必须和明细记的是同一个)", country)
	}

	// country=CN:同一条规则必须不再命中 —— 这才证明 country 真的进了求值,
	// 而不是"只要有规则就会命中"
	g.set(geo.Info{Country: "CN"})
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusFound)

	// 访问列表接口要带出 country,否则前端访问明细页永远渲染不出国家
	if vp := listVisits(t, c, link.ID, ""); vp.Items[0].Country != "CN" {
		t.Fatalf("访问列表 country = %q, want CN", vp.Items[0].Country)
	}

	// 查不到(私网/回环/未收录网段)时:恒不命中,且明细留空而不是填占位符
	g.set(geo.Info{})
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusFound)
	if vp := listVisits(t, c, link.ID, ""); vp.Items[0].Country != "" {
		t.Fatalf("查不到时 country = %q, want 空(不能填猜测值)", vp.Items[0].Country)
	}
}

// TestLinkRulesEnabledToggleBypassesRule 测试短链 rulesEnabled 开关:
// rulesEnabled=false 时跳过规则求值直接重定向; rulesEnabled=true 正常执行规则裁决。
func TestLinkRulesEnabledToggleBypassesRule(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "bypass", "targetUrls": []string{"https://dest.example.com"}, "domainIds": []int64{lid},
	})
	if !link.RulesEnabled {
		t.Fatalf("link.RulesEnabled 默认值应当为 true")
	}

	// 创建一条拦截该短链的全局规则
	createRule(t, c, map[string]any{
		"name": "全局拦截", "action": store.RuleActionNotfound, "conditions": ruleOnPath(link.Code),
	})

	// 规则启用且短链 rulesEnabled=true: 应当被规则拦截为 404
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)

	// 将短链 rulesEnabled 置为 false
	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"rulesEnabled": false})
	assertStatus(t, resp, http.StatusOK)
	upd := decodeBody[store.Link](t, resp)
	if upd.RulesEnabled {
		t.Fatalf("PATCH rulesEnabled=false 后返回的 RulesEnabled 应当为 false")
	}

	// 再次访问短链: 应当跳过规则求值, 直接 302 重定向到目标
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusFound)

	// 恢复 rulesEnabled=true: 应当再次被规则拦截为 404
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"rulesEnabled": true})
	assertStatus(t, resp, http.StatusOK)
	assertStatus(t, redirectGet(t, env, "localhost", "/"+link.Code), http.StatusNotFound)
}

func TestVisitorErrorPages(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "testpage", "targetUrls": []string{"https://dest.example.com"}, "domainIds": []int64{lid},
	})

	// 1. 普通未命中: 返回系统默认 404 HTML
	resp := redirectGet(t, env, "localhost", "/nonexistent_code")
	assertStatus(t, resp, http.StatusNotFound)
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "404") || !strings.Contains(string(body), "页面未找到") {
		t.Fatalf("未命中默认 404 内容异常: %s", string(body))
	}

	// 2. 租户配置全局 404 页面后未命中: 返回租户全局自定义 HTML
	tenant, err := env.Store.GetTenantByEmail(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if err := env.Store.UpdateTenantErrorPages(context.Background(), tenant.ID, "<h1>Alice 404</h1>", "<h1>Alice 429</h1>"); err != nil {
		t.Fatalf("update tenant error pages: %v", err)
	}

	resp = redirectGet(t, env, "localhost", "/nonexistent_code")
	assertStatus(t, resp, http.StatusNotFound)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "<h1>Alice 404</h1>") {
		t.Fatalf("租户全局 404 未生效, got: %s", string(body))
	}

	// 3. 规则级自定义 404 页面
	rule := createRule(t, c, map[string]any{
		"name":       "专属404",
		"action":     store.RuleActionNotfound,
		"pageMode":   "custom",
		"customHtml": "<h1>Rule Custom 404</h1>",
		"conditions": ruleOnPath(link.Code),
	})
	_ = rule

	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "<h1>Rule Custom 404</h1>") {
		t.Fatalf("规则专属 404 未生效, got: %s", string(body))
	}

	// 4. 规则级 429 专属页面
	createRule(t, c, map[string]any{
		"name":       "专属429",
		"priority":   50, // 优先级更高
		"action":     store.RuleActionThrottle,
		"pageMode":   "custom",
		"customHtml": "<h1>Rule Custom 429</h1>",
		"conditions": ruleOnPath(link.Code),
	})

	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusTooManyRequests)
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "<h1>Rule Custom 429</h1>") {
		t.Fatalf("规则专属 429 未生效, got: %s", string(body))
	}

	// 5. 管理 API 依然返回 JSON
	apiResp := c.get("/api/nonexistent-route")
	assertStatus(t, apiResp, http.StatusNotFound)
	if ct := apiResp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("API Content-Type = %q, want application/json", ct)
	}
	_ = apiResp.Body.Close()
}
