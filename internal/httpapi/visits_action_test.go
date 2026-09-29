package httpapi_test

// 短链访问明细(action/outcome/geo):跳转、落地页视图、点击三类动作落行,
// 可归属的失败(停用/逻辑删除/无目标/落地页文件缺失)同样落行,
// 点击行不灌水访问量,访问列表支持按动作过滤,未命中不记行。

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
)

// visitPage 访问列表响应体。
type visitPage struct {
	Items []*store.Visit `json:"items"`
	Total int            `json:"total"`
}

// listVisits 读取短链访问明细(action 非空时按动作过滤)。
func listVisits(t *testing.T, c *testClient, linkID int64, action string) visitPage {
	t.Helper()
	path := fmt.Sprintf("/api/links/%d/visits?page=1&pageSize=100", linkID)
	if action != "" {
		path += "&action=" + action
	}
	resp := c.get(path)
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[visitPage](t, resp)
}

// allVisitsInDB 直接查库统计 visits 总行数(未命中场景无法按 link_id 观察)。
func allVisitsInDB(t *testing.T, env *testutil.Env) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(testutil.Ctx(), `SELECT count(*) FROM visits`).Scan(&n); err != nil {
		t.Fatalf("count visits: %v", err)
	}
	return n
}

// latestVisitRow 直接查库取该短链最新一行明细(逻辑删除后 API 不可读)。
func latestVisitRow(t *testing.T, env *testutil.Env, linkID int64) (action, outcome, reason string) {
	t.Helper()
	err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT action, outcome, reason FROM visits WHERE link_id=$1 ORDER BY id DESC LIMIT 1`,
		linkID).Scan(&action, &outcome, &reason)
	if err != nil {
		t.Fatalf("read latest visit: %v", err)
	}
	return action, outcome, reason
}

// redirectGetWithHeaders 请求短链路径并带自定义请求头(用于 UA/来源/语言头)。
func redirectGetWithHeaders(t *testing.T, env *testutil.Env, host, path string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noFollowClient(env).Do(req)
	if err != nil {
		t.Fatalf("redirect GET %s (host %s): %v", path, host, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestVisitActionRedirectSuccess 跳转型短链一次成功访问:
// 明细行 action=redirect / outcome=success / targetUrl=轮询选中的目标,访问计数 +1。
func TestVisitActionRedirectSuccess(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "rdrta",
		"targetUrls": []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Errorf("Location = %q", loc)
	}

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 1 || len(vp.Items) != 1 {
		t.Fatalf("visit total=%d items=%d, want 1/1", vp.Total, len(vp.Items))
	}
	v := vp.Items[0]
	if v.Action != store.VisitActionRedirect || v.Outcome != store.VisitOutcomeSuccess || v.Reason != "" {
		t.Errorf("visit = action %q outcome %q reason %q, want redirect/success/空",
			v.Action, v.Outcome, v.Reason)
	}
	if v.TargetURL != "https://t1.example.com" {
		t.Errorf("targetUrl = %q, want https://t1.example.com", v.TargetURL)
	}
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 1 {
		t.Errorf("link.visits = %v, want 1", got)
	}
}

// TestVisitActionFailedForDisabledAndDeleted 短链停用/逻辑删除后的访问:
// 返回 404,并各落一行 failed 明细(link_disabled / link_deleted);
// 失败行不计入访问量(link.visits 保持停用前的值)。
func TestVisitActionFailedForDisabledAndDeleted(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "offa",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})
	// 基线:一次成功访问
	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)

	// 停用 → 404 + failed/link_disabled
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 2 {
		t.Fatalf("visit total after disabled = %d, want 2", vp.Total)
	}
	failed := vp.Items[0] // 按 id 倒序,最新在前
	if failed.Action != store.VisitActionRedirect || failed.Outcome != store.VisitOutcomeFailed ||
		failed.Reason != store.VisitReasonLinkDisabled {
		t.Errorf("disabled visit = action %q outcome %q reason %q, want redirect/failed/link_disabled",
			failed.Action, failed.Outcome, failed.Reason)
	}
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 1 {
		t.Errorf("link.visits after disabled = %v, want 1(失败行不得灌水)", got)
	}
	// 列表页聚合(fillLinksMeta 的 visits SQL)必须与单条详情口径一致
	if got := listLinks(t, c)[0].Visits; got != 1 {
		t.Errorf("link.visits in list after disabled = %d, want 1", got)
	}

	// 逻辑删除 → 404 + failed/link_deleted(删除后后台 API 不可读,直接查库)
	resp = c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)

	action, outcome, reason := latestVisitRow(t, env, link.ID)
	if action != store.VisitActionRedirect || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonLinkDeleted {
		t.Errorf("deleted visit = action %q outcome %q reason %q, want redirect/failed/link_deleted",
			action, outcome, reason)
	}
	if n := allVisitsInDB(t, env); n != 3 {
		t.Errorf("visits rows in db = %d, want 3", n)
	}
}

// TestVisitActionFailedForNoTarget 短链没有可用目标(目标列表被清空)时:
// 返回 404 并落一行 failed/no_target。目标为空的短链无法经创建接口构造
// (targetUrls 至少 1 个且逐项非空),故直接在库中清空目标模拟该状态。
func TestVisitActionFailedForNoTarget(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "notgt",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`DELETE FROM link_targets WHERE link_id=$1`, link.ID); err != nil {
		t.Fatalf("clear link targets: %v", err)
	}

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 1 {
		t.Fatalf("visit total = %d, want 1", vp.Total)
	}
	if vp.Items[0].Outcome != store.VisitOutcomeFailed || vp.Items[0].Reason != store.VisitReasonNoTarget {
		t.Errorf("no-target visit = outcome %q reason %q, want failed/no_target",
			vp.Items[0].Outcome, vp.Items[0].Reason)
	}
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 0 {
		t.Errorf("link.visits = %v, want 0", got)
	}
}

// TestVisitActionLandingViewAndClick 落地页型短链:
// 访问落 action=landing_view(计入访问量),点击落 action=click(不计入访问量、clicks+1)。
func TestVisitActionLandingViewAndClick(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")

	// 落地页视图
	resp := redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://page.example.com/lp" {
		t.Errorf("landing Location = %q", loc)
	}

	// 点击
	resp = redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Errorf("click Location = %q", loc)
	}

	st := linkStats(t, c, link.ID)
	if st["clicks"].(float64) != 1 {
		t.Errorf("clicks = %v, want 1", st["clicks"])
	}
	if st["visits"].(float64) != 1 {
		t.Errorf("link.visits = %v, want 1(点击行不得灌水访问量)", st["visits"])
	}
	// 列表页聚合(fillLinksMeta 的 visits SQL)必须与单条详情口径一致
	if got := listLinks(t, c)[0].Visits; got != 1 {
		t.Errorf("link.visits in list = %d, want 1", got)
	}

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 2 {
		t.Fatalf("visit total = %d, want 2", vp.Total)
	}
	click := vp.Items[0]
	if click.Action != store.VisitActionClick || click.Outcome != store.VisitOutcomeSuccess ||
		click.TargetURL != "https://t1.example.com" {
		t.Errorf("click row = action %q outcome %q targetUrl %q, want click/success/https://t1.example.com",
			click.Action, click.Outcome, click.TargetURL)
	}
	view := vp.Items[1]
	if view.Action != store.VisitActionLandingView || view.Outcome != store.VisitOutcomeSuccess ||
		view.TargetURL != "https://page.example.com/lp" {
		t.Errorf("landing view row = action %q outcome %q targetUrl %q, want landing_view/success/https://page.example.com/lp",
			view.Action, view.Outcome, view.TargetURL)
	}
}

// TestVisitActionLandingMissing landing+upload 来源但托管文件缺失:
// 访问落一行 failed/landing_missing 并返回 404(不计入访问量)。
func TestVisitActionLandingMissing(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "upload", "")

	resp := redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusNotFound)

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 1 {
		t.Fatalf("visit total = %d, want 1", vp.Total)
	}
	v := vp.Items[0]
	if v.Action != store.VisitActionLandingView || v.Outcome != store.VisitOutcomeFailed ||
		v.Reason != store.VisitReasonLandingMissing {
		t.Errorf("landing missing row = action %q outcome %q reason %q, want landing_view/failed/landing_missing",
			v.Action, v.Outcome, v.Reason)
	}
	if got := linkStats(t, c, link.ID)["visits"].(float64); got != 0 {
		t.Errorf("link.visits = %v, want 0", got)
	}
}

// TestVisitActionClickFailureNotCounted 落地页点击选不到目标时:
// 返回 404 并落一行 action=click / failed / no_target,且点击计数不增长
// (计数必须在选到目标之后,否则失败点击会虚增 clicks)。
// 目标为空的短链无法经创建接口构造,故直接在库中清空目标模拟该状态。
func TestVisitActionClickFailureNotCounted(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`DELETE FROM link_targets WHERE link_id=$1`, link.ID); err != nil {
		t.Fatalf("clear link targets: %v", err)
	}

	resp := redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusNotFound)

	vp := listVisits(t, c, link.ID, "")
	if vp.Total != 1 {
		t.Fatalf("visit total = %d, want 1", vp.Total)
	}
	v := vp.Items[0]
	if v.Action != store.VisitActionClick || v.Outcome != store.VisitOutcomeFailed ||
		v.Reason != store.VisitReasonNoTarget {
		t.Errorf("failed click row = action %q outcome %q reason %q, want click/failed/no_target",
			v.Action, v.Outcome, v.Reason)
	}
	if got := linkStats(t, c, link.ID)["clicks"].(float64); got != 0 {
		t.Errorf("clicks = %v, want 0(选不到目标的点击不得计数)", got)
	}
}

// TestVisitActionFilter 访问列表按 action 过滤;非法动作值返回 400。
func TestVisitActionFilter(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")

	for _, p := range []string{"/kpage", "/kpage/click", "/kpage/click"} {
		resp := redirectGet(t, env, "localhost", p)
		assertStatus(t, resp, http.StatusFound)
	}

	clickOnly := listVisits(t, c, link.ID, store.VisitActionClick)
	if clickOnly.Total != 2 || len(clickOnly.Items) != 2 {
		t.Fatalf("?action=click total=%d items=%d, want 2/2", clickOnly.Total, len(clickOnly.Items))
	}
	for _, v := range clickOnly.Items {
		if v.Action != store.VisitActionClick {
			t.Errorf("?action=click returned action %q", v.Action)
		}
	}
	viewOnly := listVisits(t, c, link.ID, store.VisitActionLandingView)
	if viewOnly.Total != 1 || len(viewOnly.Items) != 1 {
		t.Fatalf("?action=landing_view total=%d items=%d, want 1/1", viewOnly.Total, len(viewOnly.Items))
	}
	// 全部(空 action)不过滤
	if all := listVisits(t, c, link.ID, ""); all.Total != 3 {
		t.Errorf("unfiltered total = %d, want 3", all.Total)
	}
	// 非法动作 → 400
	resp := c.get(fmt.Sprintf("/api/links/%d/visits?action=bogus", link.ID))
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// TestVisitMissRecordsNoRow 短码未命中返回 404 且不产生任何明细行(无法归属到短链)。
func TestVisitMissRecordsNoRow(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	createLink(t, c, map[string]any{
		"code":       "okaa",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	resp := redirectGet(t, env, "localhost", "/missa")
	assertStatus(t, resp, http.StatusNotFound)

	if n := allVisitsInDB(t, env); n != 0 {
		t.Errorf("visits rows after miss = %d, want 0", n)
	}
}

// TestVisitLangFromAcceptLanguage 语言取 Accept-Language 首标签(剥离 q 参数),
// 超长标签被截断,防止请求头撑爆明细行。
func TestVisitLangFromAcceptLanguage(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "tongue",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})

	cases := []struct {
		name, header, want string
	}{
		{"first tag with q", "zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"},
		{"single tag", "fr", "fr"},
		{"oversized tag truncated", "en-" + strings.Repeat("x", 200), "en-" + strings.Repeat("x", 61)},
	}
	for _, tc := range cases {
		resp := redirectGetWithHeaders(t, env, "localhost", "/"+link.Code,
			map[string]string{"Accept-Language": tc.header})
		assertStatus(t, resp, http.StatusFound)
		vp := listVisits(t, c, link.ID, "")
		if got := vp.Items[0].Lang; got != tc.want {
			t.Errorf("%s: lang = %q(len %d), want %q", tc.name, got, len(got), tc.want)
		}
	}
}
