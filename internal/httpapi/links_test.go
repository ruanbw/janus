package httpapi_test

// 05 — shortlink-core 黑盒测试:创建/关联域名/跳转/未命中/停用删除/同码跨域名/配额。
// 07 — visits-stats 黑盒测试:每次跳转记录 Visit、计数增长、访问列表。

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"cloak/internal/domain"
	"cloak/internal/httpapi"
	"cloak/internal/store"
	"cloak/internal/testutil"
)

func noFollowClient(env *testutil.Env) *http.Client {
	c := env.Server.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// redirectGet 直接请求跳转路径(指定 Host,不跟随重定向)。
func redirectGet(t *testing.T, env *testutil.Env, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := noFollowClient(env).Do(req)
	if err != nil {
		t.Fatalf("redirect GET %s (host %s): %v", path, host, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// domainIDsOf 返回租户全部域名 id(含平台默认域名)。
func domainIDsOf(t *testing.T, c *testClient) []int64 {
	t.Helper()
	domains := listDomains(t, c)
	ids := make([]int64, 0, len(domains))
	for _, d := range domains {
		ids = append(ids, d.ID)
	}
	return ids
}

// localhostDomainID 返回租户的 localhost 域名 id(跳转测试用;需先 addDomain)。
func localhostDomainID(t *testing.T, c *testClient) int64 {
	t.Helper()
	for _, d := range listDomains(t, c) {
		if d.FQDN == "localhost" {
			return d.ID
		}
	}
	t.Fatal("localhost domain not found; call addDomain(t, c, \"localhost\") first")
	return 0
}

func createLink(t *testing.T, c *testClient, body map[string]any) *store.Link {
	t.Helper()
	resp := c.post("/api/links", body)
	assertStatus(t, resp, http.StatusCreated)
	l := decodeBody[store.Link](t, resp)
	return &l
}

func TestCreateLinkAutoCodeAndRedirect(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}

	link := createLink(t, c, map[string]any{
		"targetUrl": "https://example.com/landing?utm=x",
		"domainIds": ids[:1],
	})
	if len(link.Code) != 6 {
		t.Errorf("auto code length = %d, want 6", len(link.Code))
	}
	if !domain.IsValidCode(link.Code) {
		t.Errorf("auto code %q contains forbidden chars", link.Code)
	}
	if link.RedirectStatus != "302" {
		t.Errorf(`redirectStatus = %s, want default "302"`, link.RedirectStatus)
	}
	if link.Status != "enabled" {
		t.Errorf("status = %s, want enabled", link.Status)
	}

	// 访问 → 302 Location=目标
	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://example.com/landing?utm=x" {
		t.Errorf("Location = %q", loc)
	}
}

func TestCustomCodeAnd301(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	ids := []int64{localhostDomainID(t, c)}

	link := createLink(t, c, map[string]any{
		"code":           "go3ab",
		"targetUrl":      "https://example.com/permanent",
		"domainIds":      ids[:1],
		"redirectStatus": 301,
	})
	if link.Code != "go3ab" {
		t.Errorf("code = %s, want go3ab", link.Code)
	}
	resp := redirectGet(t, env, "localhost", "/go3ab")
	assertStatus(t, resp, http.StatusMovedPermanently)
	if loc := resp.Header.Get("Location"); loc != "https://example.com/permanent" {
		t.Errorf("Location = %q", loc)
	}
}

// TestSameCodeAcrossDomains 同一短码在不同域名下指向不同目标。
func TestSameCodeAcrossDomains(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	domains := listDomains(t, c)
	var localID, platformID int64
	for _, d := range domains {
		if d.FQDN == "localhost" {
			localID = d.ID
		}
		if d.FQDN == "alice.cloak.test" {
			platformID = d.ID
		}
	}
	if localID == 0 || platformID == 0 {
		t.Fatalf("need localhost + alice.cloak.test domains, got %+v", domains)
	}

	createLink(t, c, map[string]any{"code": "dup", "targetUrl": "https://a.example.com", "domainIds": []int64{localID}})
	createLink(t, c, map[string]any{"code": "dup", "targetUrl": "https://b.example.com", "domainIds": []int64{platformID}})

	resp := redirectGet(t, env, "localhost", "/dup")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://a.example.com" {
		t.Errorf("localhost/dup Location = %q", loc)
	}
	resp = redirectGet(t, env, "alice.cloak.test", "/dup")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://b.example.com" {
		t.Errorf("alice.cloak.test/dup Location = %q", loc)
	}
}

func TestDuplicateCodeConflict(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	createLink(t, c, map[string]any{"code": "abc234", "targetUrl": "https://a.example.com", "domainIds": ids[:1]})
	resp := c.post("/api/links", map[string]any{
		"code": "abc234", "targetUrl": "https://b.example.com", "domainIds": ids[:1]})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

func TestLinkValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"crlf in target", map[string]any{"targetUrl": "https://a.example.com/\r\nX-Injected: 1", "domainIds": ids[:1]}, http.StatusBadRequest},
		{"forbidden char in code", map[string]any{"code": "l000se", "targetUrl": "https://a.example.com", "domainIds": ids[:1]}, http.StatusBadRequest},
		{"no domains", map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{}}, http.StatusBadRequest},
		{"unknown domain", map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{99999}}, http.StatusBadRequest},
		{"bad redirect status", map[string]any{"targetUrl": "https://a.example.com", "domainIds": ids[:1], "redirectStatus": 303}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.post("/api/links", tc.body)
			assertStatus(t, resp, tc.want)
			_ = resp.Body.Close()
		})
	}
}

func TestRedirectMissDisabledDeleted(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{localID}})

	// 未命中
	resp := redirectGet(t, env, "localhost", "/zzzzzz")
	assertStatus(t, resp, http.StatusNotFound)

	// 停用短链 → 404
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)

	// 恢复 → 200 跳转
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"status": "enabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)

	// 逻辑删除 → 404
	resp = c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusNotFound)

	// 停用域名 → 404(域名停用后其下所有短码未命中)
	link2 := createLink(t, c, map[string]any{"targetUrl": "https://b.example.com", "domainIds": []int64{localID}})
	resp = c.patch("/api/domains/"+strconv.FormatInt(localID, 10), map[string]any{"status": "stopped"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/"+link2.Code)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestLinkListAndPatch(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)

	l1 := createLink(t, c, map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{localID}})
	createLink(t, c, map[string]any{"targetUrl": "https://b.example.com", "domainIds": []int64{localID}})

	resp := c.get("/api/links?page=1&pageSize=10")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	if list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("total=%d items=%d, want 2/2", list.Total, len(list.Items))
	}
	if len(list.Items[0].Domains) == 0 {
		t.Error("link domains not populated")
	}

	// PATCH 改目标与域名(关联到平台默认域名)
	var platformID int64
	for _, d := range listDomains(t, c) {
		if d.FQDN == "alice.cloak.test" {
			platformID = d.ID
		}
	}
	resp = c.patch("/api/links/"+strconv.FormatInt(l1.ID, 10), map[string]any{
		"targetUrl": "https://updated.example.com", "domainIds": []int64{platformID}})
	assertStatus(t, resp, http.StatusOK)
	upd := decodeBody[store.Link](t, resp)
	if upd.TargetURL != "https://updated.example.com" {
		t.Errorf("targetUrl = %s", upd.TargetURL)
	}
	if len(upd.Domains) != 1 || upd.Domains[0] != "alice.cloak.test" {
		t.Errorf("domains = %v", upd.Domains)
	}

	// 逻辑删除后列表不含
	resp = c.del("/api/links/" + strconv.FormatInt(l1.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.get("/api/links")
	assertStatus(t, resp, http.StatusOK)
	list = decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	if list.Total != 1 {
		t.Errorf("total after soft delete = %d, want 1", list.Total)
	}

	// purge:彻底删除
	resp = c.post("/api/links/"+strconv.FormatInt(l1.ID, 10)+"/purge", nil)
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.get("/api/links/" + strconv.FormatInt(l1.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestLinkQuota(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	env.SetTierLimits(t, tenantIDOf(t, c), 1, 10)
	createLink(t, c, map[string]any{"targetUrl": "https://a.example.com", "domainIds": ids[:1]})
	resp := c.post("/api/links", map[string]any{"targetUrl": "https://b.example.com", "domainIds": ids[:1]})
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_LINK_LIMIT" {
		t.Errorf("code = %s, want E_LINK_LIMIT", body.Code)
	}
	// 逻辑删除仍占配额(按"尚未物理删除"计数)
	links := listLinks(t, c)
	resp = c.del("/api/links/" + strconv.FormatInt(links[0].ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.post("/api/links", map[string]any{"targetUrl": "https://c.example.com", "domainIds": ids[:1]})
	assertStatus(t, resp, http.StatusForbidden)
	_ = resp.Body.Close()
}

func listLinks(t *testing.T, c *testClient) []*store.Link {
	t.Helper()
	resp := c.get("/api/links")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	return list.Items
}

// ---------- 07 visits-stats ----------

func TestVisitsRecordedAndCounted(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{localhostDomainID(t, c)}})

	// 三次访问(不同 UA/referer)
	reqUA := []struct{ ua, ref string }{
		{"Mozilla/5.0 (iPhone)", "https://google.com/"},
		{"curl/8.0", ""},
		{"Mozilla/5.0 (Macintosh)", "https://x.com/"},
	}
	for _, v := range reqUA {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/"+link.Code, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "localhost"
		req.Header.Set("User-Agent", v.ua)
		req.Header.Set("Referer", v.ref)
		resp, err := noFollowClient(env).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, resp, http.StatusFound)
		_ = resp.Body.Close()
	}

	// 未命中不记录
	resp := redirectGet(t, env, "localhost", "/nope42")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()

	// stats:访问数 = 3
	resp = c.get("/api/links/" + strconv.FormatInt(link.ID, 10) + "/stats")
	assertStatus(t, resp, http.StatusOK)
	stats := decodeBody[struct {
		Visits int64 `json:"visits"`
	}](t, resp)
	if stats.Visits != 3 {
		t.Fatalf("visits = %d, want 3", stats.Visits)
	}

	// 列表含 UA 与来源
	resp = c.get("/api/links/" + strconv.FormatInt(link.ID, 10) + "/visits?page=1&pageSize=10")
	assertStatus(t, resp, http.StatusOK)
	vl := decodeBody[struct {
		Items []*store.Visit `json:"items"`
		Total int            `json:"total"`
	}](t, resp)
	if vl.Total != 3 {
		t.Fatalf("visit total = %d, want 3", vl.Total)
	}
	foundUA := map[string]bool{}
	for _, v := range vl.Items {
		foundUA[v.UserAgent] = true
		if v.Domain != "localhost" {
			t.Errorf("visit domain = %s, want localhost", v.Domain)
		}
	}
	for _, v := range reqUA {
		if !foundUA[v.ua] {
			t.Errorf("visit with UA %q not recorded", v.ua)
		}
	}

	// 列表页携带 visits 计数
	resp = c.get("/api/links")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	if list.Items[0].Visits != 3 {
		t.Errorf("link visits in list = %d, want 3", list.Items[0].Visits)
	}
}

// TestVisitCleanup 超过保留期的访问被后台任务清理。
func TestVisitCleanup(t *testing.T) {
	env := testutil.Setup(t)
	env.StartWorker(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{"targetUrl": "https://a.example.com", "domainIds": []int64{localhostDomainID(t, c)}})

	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	_ = resp.Body.Close()

	// 把访问记录改为 91 天前(保留期 90 天)
	if _, err := env.Pool.Exec(testutil.Ctx(),
		`UPDATE visits SET created_at = now() - interval '91 days'`); err != nil {
		t.Fatalf("backdate visits: %v", err)
	}
	testutil.Poll(t, 5*time.Second, "visits cleaned", func() bool {
		var n int
		if err := env.Pool.QueryRow(testutil.Ctx(),
			`SELECT count(*) FROM visits`).Scan(&n); err != nil {
			return false
		}
		return n == 0
	})
}
