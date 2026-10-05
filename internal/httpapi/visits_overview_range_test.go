package httpapi_test

// 总览时间范围筛选(GET /api/visits/overview?range=...)的黑盒契约。
//
// 需求:总览页要能按时间范围看数据,并提供快捷选择(今日 / 3 天 / 7 天 / 一个月 / 年)。
// 本文件是这批改动的规格测试,先于实现存在:窗口语义、边界、作用面与非法取值都定在这里。
//
// 契约:
//  1. range ∈ {today, 3d, 7d, 30d, 1y};缺省或空串 = 7d;其它取值 400 E_VALIDATION。
//  2. 窗口按**服务器本地时区的自然日**切分(响应回显窗口,展示端不自己推算):
//        today = [今日 00:00, 现在]
//        3d    = [今日 - 2 天 00:00, 现在]
//        7d    = [今日 - 6 天 00:00, 现在]
//        30d   = [今日 - 29 天 00:00, 现在]
//        1y    = [今日 - 364 天 00:00, 现在]
//     左闭:created_at 恰好落在 from 上计入;右端是请求时刻,不是当日 24:00。
//  3. 同一个窗口一致作用于 totals / topLinks / facets;clicks 与 landingVisits 也同
//     窗口 —— CTR 绝不跨窗口拼分子分母。
//  4. 响应回显 window{range, from, to, retentionDays}:UI 据此如实标注统计窗口,
//     并在 1y 超出保留期(默认 90 天,更早的明细已被清理任务删除)时给出提示。
//
// 待人类评审的取舍(改断言的这里就是改契约):
//   - 自然日窗口,而不是滚动 N×24 小时;
//   - 缺省 range = 7d(改版前的行为是"保留期内全部");
//   - 窗口右端 = 请求时刻;若产品要"今日到 24:00"或整日桶,改这里的断言。

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
)

// overviewWindowResp 契约中的窗口回显。生产实现应落在 store.OverviewStats 上;
// 在实现之前用本地结构解出响应,测试才能先编译、再以断言失败的方式变红。
type overviewWindowResp struct {
	Range string    `json:"range"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	// RetentionDays 访问明细保留期(天)。1y 窗口必然超出它,UI 要据此说明
	// "窗口内更早的数据已被清理",而不是把不足一年的数据冒充一年。
	RetentionDays int `json:"retentionDays"`
}

// overviewRangeResp GET /api/visits/overview 在加了 window 之后的完整响应。
type overviewRangeResp struct {
	Window   overviewWindowResp      `json:"window"`
	Totals   store.OverviewTotals    `json:"totals"`
	TopLinks []store.OverviewTopLink `json:"topLinks"`
	Facets   store.OverviewFacets    `json:"facets"`
}

// localDayStart 返回 t 所在自然日的本地 00:00(与窗口口径同一规则)。
// 测试与 httptest 服务同进程,time.Local 一致;生产以容器时区为准。
func localDayStart(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// recordVisitAt 先走一次真实跳转(完整写入路径),再把该行明细回溯到 at,
// 用来构造"窗口内 / 窗口外"的时间分布,而不绕过写路径直接造数。
func recordVisitAt(t *testing.T, env *testutil.Env, host, path string, linkID int64, at time.Time, hdr map[string]string) {
	t.Helper()
	resp := redirectGetWithHeaders(t, env, host, path, hdr)
	assertStatus(t, resp, http.StatusFound)

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

// getOverview 请求总览端点并按含 window 的契约解码。
func getOverview(t *testing.T, c *testClient, query string) overviewRangeResp {
	t.Helper()
	resp := c.get("/api/visits/overview" + query)
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[overviewRangeResp](t, resp)
}

// TestVisitsOverviewRangeDailyMatrix 固定五个范围各自的时间窗口与计数:
// 每条窗口边界的内外两侧各放一行明细,计数就是边界语义本身。
func TestVisitsOverviewRangeDailyMatrix(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "rng",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	now := time.Now()
	today := localDayStart(now)
	threeDay := today.AddDate(0, 0, -2)
	sevenDay := today.AddDate(0, 0, -6)
	thirtyDay := today.AddDate(0, 0, -29)
	yearDay := today.AddDate(0, 0, -364)

	rows := []struct {
		name string
		at   time.Time
	}{
		{"今日内", now},
		{"昨日内(今日窗口外、3 日窗口内)", today.Add(-time.Second)},
		{"7 日窗口首秒(左闭)", sevenDay},
		{"3 日窗口首秒(左闭)", threeDay},
		{"3 日窗口外", threeDay.Add(-time.Second)},
		{"7 日窗口外", sevenDay.Add(-time.Second)},
		{"30 日窗口外", thirtyDay.Add(-time.Second)},
		{"一年窗口外", yearDay.Add(-time.Second)},
	}
	for _, r := range rows {
		recordVisitAt(t, env, "localhost", "/"+link.Code, link.ID, r.at, nil)
	}

	before := time.Now()
	cases := []struct {
		rangeValue string
		want       int64
		from       time.Time
	}{
		{"today", 1, today},
		{"3d", 3, threeDay},
		{"7d", 5, sevenDay},
		{"30d", 6, thirtyDay},
		{"1y", 7, yearDay},
	}
	for _, tc := range cases {
		got := getOverview(t, c, "?range="+tc.rangeValue)
		if got.Totals.Visits != tc.want {
			t.Errorf("range=%s visits = %d, want %d", tc.rangeValue, got.Totals.Visits, tc.want)
		}
		if got.Window.Range != tc.rangeValue {
			t.Errorf("range=%s window.range = %q", tc.rangeValue, got.Window.Range)
		}
		if !got.Window.From.Equal(tc.from) {
			t.Errorf("range=%s window.from = %s, want %s(本地自然日 00:00)",
				tc.rangeValue, got.Window.From, tc.from)
		}
		// 右端是请求时刻(±1s 容差),不是当日 24:00 —— 否则差出的是小时级。
		if got.Window.To.Before(before.Add(-time.Second)) || got.Window.To.After(time.Now().Add(time.Second)) {
			t.Errorf("range=%s window.to = %s, want 请求时刻", tc.rangeValue, got.Window.To)
		}
	}

	// 缺省与空串同义:都取默认窗口(近 7 天)。
	for _, query := range []string{"", "?range="} {
		got := getOverview(t, c, query)
		if got.Window.Range != "7d" || got.Totals.Visits != 5 {
			t.Errorf("query=%q window=%q visits=%d, want 默认 7d / 5",
				query, got.Window.Range, got.Totals.Visits)
		}
	}

	// 1y 必然超出默认保留期:回显保留期,UI 才能如实提示数据覆盖范围。
	wantRetention := int(env.Cfg.VisitRetention / (24 * time.Hour))
	if got := getOverview(t, c, "?range=1y"); got.Window.RetentionDays != wantRetention {
		t.Errorf("window.retentionDays = %d, want %d", got.Window.RetentionDays, wantRetention)
	}
}

// TestVisitsOverviewRangeScopesEveryAggregate 锁住"一个窗口,所有图":
// 排行 / 来源 / UA / 国家 / 点击与落地页访问都必须只统计窗口内的行。
func TestVisitsOverviewRangeScopesEveryAggregate(t *testing.T) {
	g := &fakeGeo{}
	env := testutil.SetupWithGeo(t, g)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}

	winner := createLink(t, c, map[string]any{
		"code":       "now",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  ids,
	})
	loser := createLink(t, c, map[string]any{
		"code":       "aged",
		"targetUrls": []string{"https://b.example.com"},
		"domainIds":  ids,
	})
	landing := createLink(t, c, map[string]any{
		"code":          "page",
		"targetUrls":    []string{"https://c.example.com"},
		"domainIds":     ids,
		"linkType":      "landing",
		"landingSource": "url",
		"landingUrl":    "https://land.example.com/lp",
	})

	now := time.Now()
	inWindow := now.Add(-time.Minute)
	outWindow := now.AddDate(0, 0, -40) // 7 天窗口外、一年窗口内

	inUA, inRef := "RangeInBot/1.0", "https://www.tiktok.com/ads"
	outUA, outRef := "RangeOutBot/1.0", "https://www.google.com/ads"
	inHeaders := map[string]string{"User-Agent": inUA, "Referer": inRef}
	outHeaders := map[string]string{"User-Agent": outUA, "Referer": outRef}

	g.set(geo.Info{Country: "US"})
	// 窗口内:2 次跳转 + 1 次落地页视图 + 1 次点击。
	recordVisitAt(t, env, "localhost", "/"+winner.Code, winner.ID, inWindow, inHeaders)
	recordVisitAt(t, env, "localhost", "/"+winner.Code, winner.ID, inWindow, inHeaders)
	recordVisitAt(t, env, "localhost", "/"+landing.Code, landing.ID, inWindow, inHeaders)
	recordVisitAt(t, env, "localhost", "/"+landing.Code+"/click", landing.ID, inWindow, inHeaders)

	g.set(geo.Info{Country: "JP"})
	// 窗口外:3 次跳转 + 1 次落地页视图 + 1 次点击。
	for i := 0; i < 3; i++ {
		recordVisitAt(t, env, "localhost", "/"+loser.Code, loser.ID, outWindow, outHeaders)
	}
	recordVisitAt(t, env, "localhost", "/"+landing.Code, landing.ID, outWindow, outHeaders)
	recordVisitAt(t, env, "localhost", "/"+landing.Code+"/click", landing.ID, outWindow, outHeaders)

	week := getOverview(t, c, "?range=7d")
	if week.Totals.Visits != 3 || week.Totals.RedirectVisits != 2 || week.Totals.LandingVisits != 1 {
		t.Errorf("7d totals = %+v, want visits=3 redirect=2 landing=1", week.Totals)
	}
	if week.Totals.Clicks != 1 {
		t.Errorf("7d clicks = %d, want 1(窗口外的点击不得进分子)", week.Totals.Clicks)
	}
	if len(week.TopLinks) != 2 || week.TopLinks[0].Code != "now" || week.TopLinks[0].Visits != 2 {
		t.Errorf("7d topLinks = %+v, want [new:2, page:1]", week.TopLinks)
	}
	for _, l := range week.TopLinks {
		if l.Code == "aged" {
			t.Errorf("7d 排行混入了窗口外短链: %+v", week.TopLinks)
		}
	}
	if got := countsByName(week.Facets.Sources); len(got) != 1 || got["TikTok Ads"] != 3 {
		t.Errorf("7d sources = %+v, want 仅 TikTok Ads:3", week.Facets.Sources)
	}
	if got := countsByValue(week.Facets.UserAgents); len(got) != 1 || got[inUA] != 3 {
		t.Errorf("7d userAgents = %+v, want 仅 %s:3", week.Facets.UserAgents, inUA)
	}
	if got := countsByValue(week.Facets.Countries); len(got) != 1 || got["US"] != 3 {
		t.Errorf("7d countries = %+v, want 仅 US:3", week.Facets.Countries)
	}

	// 一年窗口把 40 天前的行全部收回:各维度同时变大,而不是只有总量变。
	year := getOverview(t, c, "?range=1y")
	if year.Totals.Visits != 7 || year.Totals.LandingVisits != 2 || year.Totals.Clicks != 2 {
		t.Errorf("1y totals = %+v, want visits=7 landing=2 clicks=2", year.Totals)
	}
	byCode := map[string]int64{}
	for _, l := range year.TopLinks {
		byCode[l.Code] = l.Visits
	}
	if len(year.TopLinks) != 3 || byCode["aged"] != 3 || byCode["now"] != 2 || byCode["page"] != 2 {
		t.Errorf("1y topLinks = %+v, want aged:3 now:2 page:2", year.TopLinks)
	}
	// 窗口外的 4 行(3 次跳转 + 1 次落地页视图)都带 Google 来源。
	if got := countsByName(year.Facets.Sources); got["Google Ads"] != 4 || got["TikTok Ads"] != 3 {
		t.Errorf("1y sources = %+v, want Google:4 TikTok:3", year.Facets.Sources)
	}
	if got := countsByValue(year.Facets.Countries); got["JP"] != 4 || got["US"] != 3 {
		t.Errorf("1y countries = %+v, want JP:4 US:3", year.Facets.Countries)
	}
}

// TestVisitsOverviewRangeRejectsUnknownValues 非法范围必须是 400(而不是静默
// 退回默认窗口,让人以为看的是别的范围)。错误码沿用参数校验约定 E_VALIDATION。
func TestVisitsOverviewRangeRejectsUnknownValues(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	for _, bad := range []string{"13d", "week", "7D", "all"} {
		resp := c.get("/api/visits/overview?range=" + url.QueryEscape(bad))
		assertStatus(t, resp, http.StatusBadRequest)
		if body := decodeBody[httpapi.ErrorBody](t, resp); body.Code != "E_VALIDATION" {
			t.Errorf("range=%q code = %q, want E_VALIDATION", bad, body.Code)
		}
	}
}

func countsByName(items []store.SourceCount) map[string]int64 {
	out := make(map[string]int64, len(items))
	for _, it := range items {
		out[it.Name] = it.Count
	}
	return out
}

func countsByValue(items []store.FacetCount) map[string]int64 {
	out := make(map[string]int64, len(items))
	for _, it := range items {
		out[it.Value] = it.Count
	}
	return out
}
