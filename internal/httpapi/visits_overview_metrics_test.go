package httpapi_test

// 总览新增指标(失败与风险拦截 / 语言分布 / 环比上一周期)的黑盒契约。
//
// 需求:总览此前只有"成功访问"的维度。三块缺口:
//  1. 失败访问不可见 —— 规则拦截(404/429)与短链失效(停用/无目标/落地页缺失)
//     都已在 visits 里留痕(outcome='failed' + reason),但总览一个数都不显示;
//  2. 语言分布缺失 —— visits.lang 是规则引擎 13 个访客字段之一,却没有对应图表;
//  3. 没有对比 —— 只看绝对值分不清"今天有量"还是"趋势在涨"。
//
// 契约(与既有 overview 端点同一响应,不新增端点):
//  1. facets.failureReasons:[{value: <reason 枚举>, count}] —— 窗口内失败行按原因分桶,
//     与访问计数同一租户/未删除短链/时间窗口口径;失败行不灌水 totals.visits。
//     (link_deleted 只可能在短链被逻辑删除后产生,而该短链随后已不在总览作用域内,
//      所以这一桶在实践中不会出现,前端仍需能显示它。)
//  2. facets.languages:[{value: <Accept-Language 首标签>, count}] —— 空标签归入
//     「未携带」桶(占比才能凑满 100%),另附 languageCoverage:
//     lang 是客户端可控请求头,基数有上限(与 UA 同样的 500),截断时前端要说明。
//  3. previousTotals:{visits, landingVisits, clicks} —— **紧邻当前窗口之前、
//     等长**的上一周期计数,用于环比。上一周期 = [上一周期左端, 当前窗口左端),
//     左闭右开;窗口按自然日平移(7d 的上一周期就是再往前 7 个自然日)。
//
// 待人类评审的取舍:
//   - 失败率的分母(前端)= 成功访问 + 失败;(只统计能归属到短链的行,
//     短码未命中的 404 本来就不落行,见 README §1.2)
//   - 上一周期左闭右开,且不跨保留期做特殊处理(更早的行已被清理,自然为 0)。

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"janus/internal/store"
	"janus/internal/testutil"
)

// overviewPreviousResp previousTotals 的响应形状。
type overviewPreviousResp struct {
	Visits        int64 `json:"visits"`
	LandingVisits int64 `json:"landingVisits"`
	Clicks        int64 `json:"clicks"`
}

// overviewMetricsResp 新增字段在实现之前的本地解码形状(实现后应落在
// store.OverviewStats / OverviewFacets 上;window 类型复用范围测试里的定义)。
type overviewMetricsResp struct {
	Window overviewWindowResp `json:"window"`
	Totals struct {
		Visits        int64 `json:"visits"`
		LandingVisits int64 `json:"landingVisits"`
		Clicks        int64 `json:"clicks"`
	} `json:"totals"`
	PreviousTotals overviewPreviousResp `json:"previousTotals"`
	Facets         struct {
		FailureReasons   []store.FacetCount  `json:"failureReasons"`
		Languages        []store.FacetCount  `json:"languages"`
		LanguageCoverage store.FacetCoverage `json:"languageCoverage"`
	} `json:"facets"`
}

// recordVisitWithStatus 与 recordVisitAt 同一手法(真实请求 + 回溯时间),
// 但显式声明期望状态码 —— 失败行的响应是 404/429,不能复用只认 302 的助手。
func recordVisitWithStatus(t *testing.T, env *testutil.Env, host, path string, linkID int64,
	at time.Time, hdr map[string]string, wantStatus int) {
	t.Helper()
	resp := redirectGetWithHeaders(t, env, host, path, hdr)
	assertStatus(t, resp, wantStatus)

	var id int64
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT id FROM visits WHERE link_id = $1 ORDER BY id DESC LIMIT 1`, linkID).Scan(&id); err != nil {
		t.Fatalf("read latest visit id: %v", err)
	}
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`UPDATE visits SET created_at = $1 WHERE id = $2`, at, id); err != nil {
		t.Fatalf("backdate visit %d: %v", id, err)
	}
}

func getOverviewMetrics(t *testing.T, c *testClient, query string) overviewMetricsResp {
	t.Helper()
	resp := c.get("/api/visits/overview" + query)
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[overviewMetricsResp](t, resp)
}

// TestVisitsOverviewFailureReasonsAndLanguages 锁住两块新分布:
// 失败按原因分桶、语言按 Accept-Language 首标签分桶(空标签单列),且都受窗口约束。
func TestVisitsOverviewFailureReasonsAndLanguages(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}

	ok := createLink(t, c, map[string]any{
		"code": "okay", "targetUrls": []string{"https://a.example.com"}, "domainIds": ids})
	off := createLink(t, c, map[string]any{
		"code": "offx", "targetUrls": []string{"https://b.example.com"}, "domainIds": ids})
	blocked := createLink(t, c, map[string]any{
		"code": "deny", "targetUrls": []string{"https://c.example.com"}, "domainIds": ids})
	throttled := createLink(t, c, map[string]any{
		"code": "thrt", "targetUrls": []string{"https://d.example.com"}, "domainIds": ids})
	createRule(t, c, map[string]any{"name": "拦下", "action": store.RuleActionNotfound, "conditions": ruleOnPath(blocked.Code)})
	createRule(t, c, map[string]any{"name": "限流", "action": store.RuleActionThrottle, "conditions": ruleOnPath(throttled.Code)})

	now := time.Now()
	old := now.AddDate(0, 0, -40) // 7 天窗口外、一年窗口内

	// 成功访问的语言分布:窗口内 zh-CN 与"未携带"各一次,窗口外一个 en-US。
	recordVisitAt(t, env, "localhost", "/"+ok.Code, ok.ID, now, map[string]string{"Accept-Language": "zh-CN,zh;q=0.9"})
	recordVisitAt(t, env, "localhost", "/"+ok.Code, ok.ID, now, nil)
	recordVisitAt(t, env, "localhost", "/"+ok.Code, ok.ID, old, map[string]string{"Accept-Language": "en-US"})

	// 失败:窗口内 停用×1、规则拦截×2、规则限流×1;窗口外 停用×1。
	recordVisitWithStatus(t, env, "localhost", "/"+blocked.Code, blocked.ID, now, nil, http.StatusNotFound)
	recordVisitWithStatus(t, env, "localhost", "/"+blocked.Code, blocked.ID, now, nil, http.StatusNotFound)
	recordVisitWithStatus(t, env, "localhost", "/"+throttled.Code, throttled.ID, now, nil, http.StatusTooManyRequests)
	// 停用放在最后:这次 PATCH 之后该短链的后续访问都会以 link_disabled 落行。
	resp := c.patch("/api/links/"+strconv.FormatInt(off.ID, 10), map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	recordVisitWithStatus(t, env, "localhost", "/"+off.Code, off.ID, now, nil, http.StatusNotFound)
	recordVisitWithStatus(t, env, "localhost", "/"+off.Code, off.ID, old, nil, http.StatusNotFound)

	week := getOverviewMetrics(t, c, "?range=7d")
	// 失败不进访问量:窗口内只有 2 次成功访问。
	if week.Totals.Visits != 2 {
		t.Errorf("7d visits = %d, want 2(失败行不得灌水)", week.Totals.Visits)
	}
	wantWeekFailures := map[string]int64{
		store.VisitReasonRuleBlocked:   2,
		store.VisitReasonRuleThrottled: 1,
		store.VisitReasonLinkDisabled:  1,
	}
	if got := countsByValue(week.Facets.FailureReasons); !sameCounts(got, wantWeekFailures) {
		t.Errorf("7d failureReasons = %+v, want %+v", week.Facets.FailureReasons, wantWeekFailures)
	}
	wantWeekLangs := map[string]int64{"zh-CN": 1, "未携带": 1}
	if got := countsByValue(week.Facets.Languages); !sameCounts(got, wantWeekLangs) {
		t.Errorf("7d languages = %+v, want %+v", week.Facets.Languages, wantWeekLangs)
	}
	if got := week.Facets.LanguageCoverage; got.Total != 2 || got.Returned != 2 || got.Truncated {
		t.Errorf("7d languageCoverage = %+v, want total=2 returned=2 truncated=false", got)
	}

	year := getOverviewMetrics(t, c, "?range=1y")
	wantYearFailures := map[string]int64{
		store.VisitReasonRuleBlocked:   2,
		store.VisitReasonRuleThrottled: 1,
		store.VisitReasonLinkDisabled:  2,
	}
	if got := countsByValue(year.Facets.FailureReasons); !sameCounts(got, wantYearFailures) {
		t.Errorf("1y failureReasons = %+v, want %+v", year.Facets.FailureReasons, wantYearFailures)
	}
	wantYearLangs := map[string]int64{"zh-CN": 1, "未携带": 1, "en-US": 1}
	if got := countsByValue(year.Facets.Languages); !sameCounts(got, wantYearLangs) {
		t.Errorf("1y languages = %+v, want %+v", year.Facets.Languages, wantYearLangs)
	}
}

// TestVisitsOverviewPreviousWindow 环比窗口的边界语义:
// 上一周期 = 当前窗口左端往前平移等长的自然日数,左闭右开。
func TestVisitsOverviewPreviousWindow(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}
	redirect := createLink(t, c, map[string]any{
		"code": "prev", "targetUrls": []string{"https://a.example.com"}, "domainIds": ids})
	landing := createLink(t, c, map[string]any{
		"code": "page", "targetUrls": []string{"https://b.example.com"}, "domainIds": ids,
		"linkType": "landing", "landingSource": "url", "landingUrl": "https://land.example.com/lp"})

	now := time.Now()
	today := localDayStart(now)
	sevenDay := today.AddDate(0, 0, -6)    // 7d 窗口左端
	prevFrom := sevenDay.AddDate(0, 0, -7) // 7d 的上一周期左端

	// 当前 7 天窗口内:2 次跳转(现在 / 昨天)+ 1 次落地页视图与点击。
	recordVisitAt(t, env, "localhost", "/"+redirect.Code, redirect.ID, now, nil)
	recordVisitAt(t, env, "localhost", "/"+redirect.Code, redirect.ID, today.Add(-time.Second), nil)
	recordVisitAt(t, env, "localhost", "/"+landing.Code, landing.ID, now, nil)
	recordVisitAt(t, env, "localhost", "/"+landing.Code+"/click", landing.ID, now, nil)
	// 上一周期内:1 次跳转 + 1 次落地页视图与点击。
	recordVisitAt(t, env, "localhost", "/"+redirect.Code, redirect.ID, sevenDay.Add(-time.Second), nil)
	recordVisitAt(t, env, "localhost", "/"+landing.Code, landing.ID, sevenDay.Add(-time.Minute), nil)
	recordVisitAt(t, env, "localhost", "/"+landing.Code+"/click", landing.ID, sevenDay.Add(-time.Minute), nil)
	// 更早:两个窗口都不含。
	recordVisitAt(t, env, "localhost", "/"+redirect.Code, redirect.ID, prevFrom.Add(-time.Second), nil)

	week := getOverviewMetrics(t, c, "?range=7d")
	if week.Totals.Visits != 3 || week.Totals.LandingVisits != 1 || week.Totals.Clicks != 1 {
		t.Errorf("7d totals = %+v, want visits=3 landing=1 clicks=1", week.Totals)
	}
	if week.PreviousTotals.Visits != 2 || week.PreviousTotals.LandingVisits != 1 || week.PreviousTotals.Clicks != 1 {
		t.Errorf("7d previousTotals = %+v, want visits=2 landing=1 clicks=1", week.PreviousTotals)
	}

	// today 的上一周期就是"昨天 00:00 ~ 今天 00:00":只该数到 yesterday 那一行。
	day := getOverviewMetrics(t, c, "?range=today")
	if day.Totals.Visits != 2 || day.Totals.Clicks != 1 {
		t.Errorf("today totals = %+v, want visits=2 clicks=1", day.Totals)
	}
	if day.PreviousTotals.Visits != 1 || day.PreviousTotals.Clicks != 0 || day.PreviousTotals.LandingVisits != 0 {
		t.Errorf("today previousTotals = %+v, want visits=1 clicks=0 landing=0", day.PreviousTotals)
	}

	// 30d 把更早的那一行也收进当前窗口,而上一周期(再往前 30 天)为空。
	month := getOverviewMetrics(t, c, "?range=30d")
	if month.Totals.Visits != 6 {
		t.Errorf("30d visits = %d, want 6", month.Totals.Visits)
	}
	if month.PreviousTotals.Visits != 0 || month.PreviousTotals.Clicks != 0 {
		t.Errorf("30d previousTotals = %+v, want 全 0", month.PreviousTotals)
	}
}

// sameCounts 比较两个分桶计数字典(比较内容,不比较顺序)。
func sameCounts(got, want map[string]int64) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
