package httpapi_test

// 总览聚合端点(GET /api/visits/overview)的黑盒回归:
//
// 这段 SQL 曾在 overviewTotals 里拼出两个 WHERE(overviewScopeFrom 已以 WHERE 结尾,
// 后面又接了一个),端点恒 500;而当时没有任何测试访问过这个路由,整套测试照样全绿。
// 这里盯住两件事:
//  1. 端点本身能通(200)且各类计数口径正确;
//  2. 短链列表回传的 clickVisits 与总览的 clicks 同源同期(列表 CTR 的分子来源)。

import (
	"net/http"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

func TestVisitsOverviewAggregates(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "ovo")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}

	// 跳转型:1 次成功跳转 → visits.redirectVisits +1。
	redirectLink := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://example.com/r"},
		"domainIds":  ids,
	})
	resp := redirectGet(t, env, "localhost", "/"+redirectLink.Code)
	assertStatus(t, resp, http.StatusFound)

	// 落地页型(url 来源):1 次落地页视图 + 1 次点击。
	landingLink := createLink(t, c, map[string]any{
		"targetUrls":    []string{"https://example.com/t"},
		"domainIds":     ids,
		"linkType":      "landing",
		"landingSource": "url",
		"landingUrl":    "https://land.example.com/lp",
	})
	resp = redirectGet(t, env, "localhost", "/"+landingLink.Code)
	assertStatus(t, resp, http.StatusFound)
	resp = redirectGet(t, env, "localhost", "/"+landingLink.Code+"/click")
	assertStatus(t, resp, http.StatusFound)

	listResp := c.get("/api/visits/overview")
	assertStatus(t, listResp, http.StatusOK)
	stats := decodeBody[store.OverviewStats](t, listResp)

	if stats.Totals.Visits != 2 || stats.Totals.RedirectVisits != 1 || stats.Totals.LandingVisits != 1 {
		t.Errorf("totals = %+v, want visits=2 redirect=1 landing=1", stats.Totals)
	}
	if stats.Totals.Clicks != 1 {
		t.Errorf("clicks = %d, want 1", stats.Totals.Clicks)
	}
	if stats.Totals.Links != 2 || stats.Totals.LandingLinks != 1 {
		t.Errorf("links = %d landingLinks = %d, want 2/1", stats.Totals.Links, stats.Totals.LandingLinks)
	}
	if len(stats.TopLinks) == 0 {
		t.Error("topLinks 为空,聚合查询未返回排行")
	}

	// 列表的 clickVisits 必须与总览 clicks 同源同期(否则列表 CTR 会用永久计数器虚高)。
	linksResp := c.get("/api/links?page=1&pageSize=100")
	assertStatus(t, linksResp, http.StatusOK)
	page := decodeBody[struct {
		Items []*store.Link `json:"items"`
	}](t, linksResp)
	var got *store.Link
	for _, l := range page.Items {
		if l.ID == landingLink.ID {
			got = l
		}
	}
	if got == nil {
		t.Fatal("落地页短链未出现在 /api/links")
	}
	if got.ClickVisits != 1 || got.Visits != 1 {
		t.Errorf("link clickVisits=%d visits=%d, want 1/1", got.ClickVisits, got.Visits)
	}
}
