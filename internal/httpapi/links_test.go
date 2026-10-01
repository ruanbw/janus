package httpapi_test

// 05 — shortlink-core 黑盒测试:创建/关联域名/跳转/未命中/停用删除/同码跨域名/配额。
// 07 — visits-stats 黑盒测试:每次跳转记录 Visit、计数增长、访问列表。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"janus/internal/domain"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
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
		"targetUrls": []string{"https://example.com/landing?utm=x"},
		"domainIds":  ids[:1],
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
		"targetUrls":     []string{"https://example.com/permanent"},
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
		if d.FQDN == "alice.janus.test" {
			platformID = d.ID
		}
	}
	if localID == 0 || platformID == 0 {
		t.Fatalf("need localhost + alice.janus.test domains, got %+v", domains)
	}

	createLink(t, c, map[string]any{"code": "dup", "targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localID}})
	createLink(t, c, map[string]any{"code": "dup", "targetUrls": []string{"https://b.example.com"}, "domainIds": []int64{platformID}})

	resp := redirectGet(t, env, "localhost", "/dup")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://a.example.com" {
		t.Errorf("localhost/dup Location = %q", loc)
	}
	resp = redirectGet(t, env, "alice.janus.test", "/dup")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://b.example.com" {
		t.Errorf("alice.janus.test/dup Location = %q", loc)
	}
}

// TestRedirectRoundRobin 多目标短链默认按轮询选择:连续 3 次依次 t1、t2、t1。
func TestRedirectRoundRobin(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)

	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":  []int64{localID},
	})
	if len(link.TargetURLs) != 2 || link.TargetURLs[0] != "https://t1.example.com" || link.TargetURLs[1] != "https://t2.example.com" {
		t.Fatalf("targetUrls = %v", link.TargetURLs)
	}
	want := []string{"https://t1.example.com", "https://t2.example.com", "https://t1.example.com"}
	for i, w := range want {
		resp := redirectGet(t, env, "localhost", "/"+link.Code)
		assertStatus(t, resp, http.StatusFound)
		if loc := resp.Header.Get("Location"); loc != w {
			t.Errorf("round %d Location = %q, want %q", i+1, loc, w)
		}
	}
}

func TestDuplicateCodeConflict(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	createLink(t, c, map[string]any{"code": "abc234", "targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})
	resp := c.post("/api/links", map[string]any{
		"code": "abc234", "targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1]})
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
		{"crlf in target", map[string]any{"targetUrls": []string{"https://a.example.com/\r\nX-Injected: 1"}, "domainIds": ids[:1]}, http.StatusBadRequest},
		{"empty targetUrls", map[string]any{"targetUrls": []string{}, "domainIds": ids[:1]}, http.StatusBadRequest},
		{"forbidden char in code", map[string]any{"code": "l000se", "targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]}, http.StatusBadRequest},
		{"no domains", map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{}}, http.StatusBadRequest},
		{"unknown domain", map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{99999}}, http.StatusBadRequest},
		{"bad redirect status", map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1], "redirectStatus": 303}, http.StatusBadRequest},
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
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localID}})

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
	link2 := createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": []int64{localID}})
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

	l1 := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localID}})
	createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": []int64{localID}})

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
		if d.FQDN == "alice.janus.test" {
			platformID = d.ID
		}
	}
	resp = c.patch("/api/links/"+strconv.FormatInt(l1.ID, 10), map[string]any{
		"targetUrls": []string{"https://updated.example.com"}, "domainIds": []int64{platformID}})
	assertStatus(t, resp, http.StatusOK)
	upd := decodeBody[store.Link](t, resp)
	if len(upd.TargetURLs) != 1 || upd.TargetURLs[0] != "https://updated.example.com" {
		t.Errorf("targetUrls = %v", upd.TargetURLs)
	}
	if len(upd.Domains) != 1 || upd.Domains[0] != "alice.janus.test" {
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

// TestPatchReplacesTargets PATCH 传新 targetUrls 时整体替换,访问验证 Location 为新的第一个。
func TestPatchReplacesTargets(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)

	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://old1.example.com", "https://old2.example.com"},
		"domainIds":  []int64{localID},
	})
	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{
		"targetUrls": []string{"https://new1.example.com", "https://new2.example.com"},
	})
	assertStatus(t, resp, http.StatusOK)
	upd := decodeBody[store.Link](t, resp)
	if len(upd.TargetURLs) != 2 || upd.TargetURLs[0] != "https://new1.example.com" || upd.TargetURLs[1] != "https://new2.example.com" {
		t.Errorf("targetUrls after patch = %v", upd.TargetURLs)
	}
	// 整体替换后首次访问 → 新的第一个目标
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://new1.example.com" {
		t.Errorf("Location after patch = %q, want new1", loc)
	}
}

func TestLinkQuota(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	env.SetTierLimits(t, tenantIDOf(t, c), 1, 10)
	createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})
	resp := c.post("/api/links", map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1]})
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
	resp = c.post("/api/links", map[string]any{"targetUrls": []string{"https://c.example.com"}, "domainIds": ids[:1]})
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
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localhostDomainID(t, c)}})

	// 三次访问(不同 UA/referer;第二条带 X-Forwarded-For 模拟 Caddy 反代,应记录转发 IP)
	reqUA := []struct{ ua, ref, xff, wantIP string }{
		{"Mozilla/5.0 (iPhone)", "https://google.com/", "", "127.0.0.1"},
		{"curl/8.0", "", "203.0.113.9", "203.0.113.9"},
		{"Mozilla/5.0 (Macintosh)", "https://x.com/", "", "127.0.0.1"},
	}
	for _, v := range reqUA {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/"+link.Code, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "localhost"
		req.Header.Set("User-Agent", v.ua)
		req.Header.Set("Referer", v.ref)
		if v.xff != "" {
			req.Header.Set("X-Forwarded-For", v.xff)
		}
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
	// IP 记录:无 XFF 时取 RemoteAddr(127.0.0.1);带 XFF 时取转发 IP
	foundIP := map[string]bool{}
	for _, v := range vl.Items {
		foundIP[v.IP] = true
	}
	for _, v := range reqUA {
		if !foundIP[v.wantIP] {
			t.Errorf("visit with IP %q not recorded (got %v)", v.wantIP, foundIP)
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
	link := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localhostDomainID(t, c)}})

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

// TestPatchRejectsEmptyDomainIDs 编辑时清空全部关联域名会让短链永久无法访问,
// 与创建语义一致,应返回 400。
func TestPatchRejectsEmptyDomainIDs(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localID},
	})

	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"domainIds": []int64{}})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 关联未被清空,短链仍可跳转
	resp = redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
}

// ---------- 回收站:软删释放短码 / 还原重新占用(issue 01) ----------

// listDeletedLinks 读回收站列表(includeDeleted=true)。
func listDeletedLinks(t *testing.T, c *testClient) []*store.Link {
	t.Helper()
	resp := c.get("/api/links?includeDeleted=true")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	return list.Items
}

// callRestore 调 PATCH {"deletedAt": null}(还原逻辑删除的短链)。
func callRestore(t *testing.T, c *testClient, id int64) *http.Response {
	t.Helper()
	return c.patch("/api/links/"+strconv.FormatInt(id, 10), map[string]any{"deletedAt": nil})
}

// TestRecycleBinFreesCodeAndRestoreReclaims issue 01 的主用例:
// 软删 → 同域名同短码可重建 → 还原时短码又变回占用 → 彻底删除腾出后再还原成功。
func TestRecycleBinFreesCodeAndRestoreReclaims(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)

	first := createLink(t, c, map[string]any{
		"code":       "reuse",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{lid},
	})
	resp := c.del("/api/links/" + strconv.FormatInt(first.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// 列表:默认不含已删除项,includeDeleted 才看得见
	if got := len(listLinks(t, c)); got != 0 {
		t.Fatalf("软删后默认列表条数 = %d, want 0", got)
	}
	trash := listDeletedLinks(t, c)
	if len(trash) != 1 || trash[0].ID != first.ID || trash[0].DeletedAt == nil {
		t.Fatalf("回收站 = %+v, want 1 条且 deletedAt 非空", trash)
	}

	// 核心:软删后同域名同短码可以重建(修复前这里是 409 —— 短码被永久锁死)
	second := createLink(t, c, map[string]any{
		"code":       "reuse",
		"targetUrls": []string{"https://b.example.com"},
		"domainIds":  []int64{lid},
	})

	// 短码被占用时还原失败:数据库的部分唯一索引直接拒,应用层不预先自查
	resp = callRestore(t, c, first.ID)
	assertStatus(t, resp, http.StatusConflict)
	if body := decodeBody[httpapi.ErrorBody](t, resp); body.Code != "E_CONFLICT" {
		t.Errorf("还原冲突 code = %s, want E_CONFLICT", body.Code)
	}

	// 彻底删除占着短码的那条,腾出短码后还原成功
	resp = c.post("/api/links/"+strconv.FormatInt(second.ID, 10)+"/purge", nil)
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	resp = callRestore(t, c, first.ID)
	assertStatus(t, resp, http.StatusOK)
	restored := decodeBody[store.Link](t, resp)
	if restored.DeletedAt != nil {
		t.Errorf("还原后 deletedAt = %v, want nil", restored.DeletedAt)
	}
	if got := len(listDeletedLinks(t, c)); got != 0 {
		t.Errorf("还原后回收站条数 = %d, want 0", got)
	}

	// 还原后短码重新被占用:再建同短码回到 409
	resp = c.post("/api/links", map[string]any{
		"code":       "reuse",
		"targetUrls": []string{"https://c.example.com"},
		"domainIds":  []int64{lid},
	})
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

// TestRecycleBinIsTenantScoped 回收站只列本租户的已删除短链。
func TestRecycleBinIsTenantScoped(t *testing.T) {
	env := testutil.Setup(t)
	alice := loggedInTenant(t, env, "alice")
	bob := loggedInTenant(t, env, "bob")
	addDomain(t, alice, "localhost")
	link := createLink(t, alice, map[string]any{
		"code":       "apart",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, alice)},
	})
	resp := alice.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	if got := len(listDeletedLinks(t, bob)); got != 0 {
		t.Errorf("bob 的回收站条数 = %d, want 0(回收站必须租户隔离)", got)
	}
	if got := len(listDeletedLinks(t, alice)); got != 1 {
		t.Errorf("alice 的回收站条数 = %d, want 1", got)
	}
	// 跨租户还原 → 404(既不是 409,也不是静默成功)
	resp = callRestore(t, bob, link.ID)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestListLinksIncludeDeletedRejectsGarbage includeDeleted 只接受 true/false,
// 拼错的参数值返回 400 而不是悄悄当成 false(否则用户只会以为列表坏了)。
func TestListLinksIncludeDeletedRejectsGarbage(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	resp := c.get("/api/links?includeDeleted=yes")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// TestRestoreRejectsNonNullTimestamp deletedAt 只接受显式 null;传具体时间戳
// 会被明确拒绝,而不是被当成"没这个字段"从而静默走普通 PATCH。
func TestRestoreRejectsNonNullTimestamp(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"code":       "tsd",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})
	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10),
		map[string]any{"deletedAt": "2020-01-01T00:00:00Z"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// ---------- 配额并发(issue 02) ----------

// concurrentPost 用只读的会话 cookie 快照并发发请求。
// 不用 testClient.do 是因为它会写 c.cookies:多 goroutine 共享一个 client
// 就是并发写 map(数据竞争),报错也会互相串。
func concurrentPost(env *testutil.Env, cookies []*http.Cookie, csrf, path string, body map[string]any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := env.Server.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes(), nil
}

// TestCreateLinkQuotaConcurrent issue 02 的主用例。
// 修复前是「先 Usage() 读计数、再 CreateLink() 插一条」,两者不在同一事务也没有锁:
// 上限 5 条时并发 20 个请求可以在计数停在 4 的窗口里全部通过检查,写出 20 条。
// 现在判上限与插入在同一个持锁事务里,结果必须精确等于上限。
func TestCreateLinkQuotaConcurrent(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	env.SetTierLimits(t, tenantIDOf(t, c), 5, 10)

	cookies, csrf := snapshotCookies(c), c.csrfToken()

	const workers = 20
	statuses := make([]int, workers)
	var gate sync.WaitGroup
	gate.Add(1)
	var done sync.WaitGroup
	for i := 0; i < workers; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			gate.Wait()
			st, _, err := concurrentPost(env, cookies, csrf, "/api/links", map[string]any{
				"targetUrls": []string{fmt.Sprintf("https://t%d.example.com", i)},
				"domainIds":  ids[:1],
			})
			if err != nil {
				t.Errorf("concurrent create: %v", err)
				return
			}
			statuses[i] = st
		}(i)
	}
	gate.Done()
	done.Wait()

	created, limited := 0, 0
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusForbidden:
			limited++
		default:
			t.Errorf("并发创建返回了意外状态 %d", s)
		}
	}
	if created != 5 {
		t.Errorf("成功创建 %d 条,want 5(超发就是 TOCTOU 回归)", created)
	}
	if limited != workers-5 {
		t.Errorf("被配额拒绝 %d 条,want %d", limited, workers-5)
	}
	// 真正写进库里的条数必须与上限一致(并发下最容易在这里露馅)
	if got := len(listLinks(t, c)); got != 5 {
		t.Errorf("库里实际短链数 = %d, want 5", got)
	}
}

// TestCreateLinkQuotaErrorShape 超限时仍是 403 + E_LINK_LIMIT + details.usage。
func TestCreateLinkQuotaErrorShape(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	env.SetTierLimits(t, tenantIDOf(t, c), 1, 10)
	createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})

	resp := c.post("/api/links", map[string]any{
		"targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1],
	})
	assertStatus(t, resp, http.StatusForbidden)
	body := decodeBody[httpapi.ErrorBody](t, resp)
	if body.Code != "E_LINK_LIMIT" {
		t.Fatalf("code = %s, want E_LINK_LIMIT", body.Code)
	}
	details, ok := body.Details.(map[string]any)
	if !ok {
		t.Fatalf("details = %#v, want {usage: ...}", body.Details)
	}
	usage, ok := details["usage"].(map[string]any)
	if !ok {
		t.Fatalf("details.usage = %#v", details["usage"])
	}
	if usage["links"] != float64(1) || usage["maxLinks"] != float64(1) {
		t.Errorf("details.usage = %v, want links=1 maxLinks=1", usage)
	}
}

// ---------- 落地页型一律 302(issue 03) ----------

// TestLandingTypeResetsRedirectStatus 「301 跳转型 → 落地页型」的切换必须把 301 归零。
// 前端切类型时不发 redirectStatus,UpdateLink 对缺省字段保留原值,于是 301 会留在库里。
func TestLandingTypeResetsRedirectStatus(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":           "kredir",
		"targetUrls":     []string{"https://a.example.com"},
		"domainIds":      []int64{lid},
		"redirectStatus": "301",
	})
	// 跳转型确实按 301 发
	resp := redirectGet(t, env, "localhost", "/kredir")
	assertStatus(t, resp, http.StatusMovedPermanently)
	_ = resp.Body.Close()

	// 切类型:请求里**不带** redirectStatus(前端切类型时就是这个形状)
	resp = c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{
		"linkType": "landing", "landingUrl": "https://page.example.com/lp",
	})
	assertStatus(t, resp, http.StatusOK)
	up := decodeBody[store.Link](t, resp)
	if up.RedirectStatus != store.RedirectStatus302 {
		t.Fatalf("切到落地页型后 redirectStatus = %q, want 302", up.RedirectStatus)
	}

	// 端到端:规则裁决命中 action=redirect 时走的是 link.RedirectStatus(redirectToTarget),
	// 那条路径上绝不能出现 301。
	rule := createRule(t, c, map[string]any{
		"name": "落地页规则改写", "action": store.RuleActionRedirect,
		"conditions":  ruleOnPath(link.Code),
		"destination": "https://rule.example.com/landing",
	})
	resp = redirectGet(t, env, "localhost", "/kredir")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://rule.example.com/landing" {
		t.Errorf("Location = %q", loc)
	}
	_ = resp.Body.Close()
	_ = rule

	// 落库值也必须是 302(给 UI 与后续读侧一个稳定口径)
	var stored int
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT redirect_status FROM links WHERE id=$1`, link.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 302 {
		t.Errorf("库里 redirect_status = %d, want 302", stored)
	}
}

// TestLandingCreateNeverKeeps301 创建时就指定落地页型,也不该把 301 带进来。
func TestLandingCreateNeverKeeps301(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	resp := c.post("/api/links", map[string]any{
		"code": "kredirb", "targetUrls": []string{"https://a.example.com"},
		"domainIds": []int64{lid}, "linkType": "landing",
		"landingSource": "url", "landingUrl": "https://page.example.com/lp",
		"redirectStatus": "301",
	})
	assertStatus(t, resp, http.StatusCreated)
	link := decodeBody[store.Link](t, resp)
	if link.RedirectStatus != store.RedirectStatus302 {
		t.Errorf("落地页型创建后 redirectStatus = %q, want 302", link.RedirectStatus)
	}
}

// ---------- 目标 URL 数量/体积上限与去重(issue 06 / 10) ----------

// TestCreateLinkRejectsTooManyTargets 超过 cfg.MaxTargetURLs(测试环境 50)→ 400。
func TestCreateLinkRejectsTooManyTargets(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	tooMany := make([]string, 0, 51)
	for i := 0; i < 51; i++ {
		tooMany = append(tooMany, fmt.Sprintf("https://t%d.example.com", i))
	}
	resp := c.post("/api/links", map[string]any{"targetUrls": tooMany, "domainIds": ids[:1]})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 恰好 50 条允许
	link := createLink(t, c, map[string]any{"targetUrls": tooMany[:50], "domainIds": ids[:1]})
	if len(link.TargetURLs) != 50 {
		t.Errorf("落库目标数 = %d, want 50", len(link.TargetURLs))
	}
}

// TestCreateLinkRejectsOversizedBody 请求体超上限 → 400。
// 体积校验是数量校验的另一半:几万条短 URL 数量上完全合规,却能撑出几百 MB 请求体。
func TestCreateLinkRejectsOversizedBody(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	// 每条都合法、总数也远小于 50,但整体体积超过 body 上限
	huge := strings.Repeat("a", 300<<10)
	resp := c.post("/api/links", map[string]any{
		"targetUrls": []string{"https://x.example.com/" + huge},
		"domainIds":  ids[:1],
	})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
	if got := len(listLinks(t, c)); got != 0 {
		t.Errorf("超限请求写进了 %d 条短链,want 0", got)
	}
}

// TestDuplicateTargetsAreDeduped 重复目标 URL 必须去重,否则轮询会静默加权。
// ["a","a","b"] 里 a 原本能拿到 2/3 的流量。
func TestDuplicateTargetsAreDeduped(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code": "kdup",
		"targetUrls": []string{
			"https://a.example.com", "https://a.example.com", "https://b.example.com",
		},
		"domainIds": []int64{lid},
	})
	if len(link.TargetURLs) != 2 {
		t.Fatalf("落库目标 = %v, want 2 条(去重后)", link.TargetURLs)
	}

	// 轮询 4 次应该严格 a、b 交替(a 不再占 2/3)
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		resp := redirectGet(t, env, "localhost", "/kdup")
		assertStatus(t, resp, http.StatusFound)
		seen[resp.Header.Get("Location")]++
	}
	if seen["https://a.example.com"] != 2 || seen["https://b.example.com"] != 2 {
		t.Errorf("轮询分布 = %v, want a=2 b=2", seen)
	}
}

// TestPatchDuplicateTargetsDeduped 编辑路径同样去重。
func TestPatchDuplicateTargetsDeduped(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1],
	})
	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{
		"targetUrls": []string{"https://x.example.com", "https://x.example.com"},
	})
	assertStatus(t, resp, http.StatusOK)
	up := decodeBody[store.Link](t, resp)
	if len(up.TargetURLs) != 1 || up.TargetURLs[0] != "https://x.example.com" {
		t.Errorf("编辑去重后目标 = %v, want 1 条", up.TargetURLs)
	}
}

// ---------- 单目标短链不写 rr_index(issue 05) ----------

// rrIndex 读短链的轮询指针。
func rrIndex(t *testing.T, env *testutil.Env, linkID int64) int64 {
	t.Helper()
	var n int64
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT rr_index FROM links WHERE id=$1`, linkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestPickTargetSkipsWriteForSingleTarget 单目标短链的跳转不该再 UPDATE links。
// 轮询对单目标是恒等映射,而那条 UPDATE 的代价是实的:同一条 links 行上的热行锁
// 让一条热门短链的所有并发跳转排队,外加行版本膨胀与 autovacuum 压力。
func TestPickTargetSkipsWriteForSingleTarget(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "ksoto",
		"targetUrls": []string{"https://only.example.com"},
		"domainIds":  []int64{lid},
	})
	for i := 0; i < 5; i++ {
		resp := redirectGet(t, env, "localhost", "/ksoto")
		assertStatus(t, resp, http.StatusFound)
		if loc := resp.Header.Get("Location"); loc != "https://only.example.com" {
			t.Fatalf("Location = %q", loc)
		}
		_ = resp.Body.Close()
	}
	if got := rrIndex(t, env, link.ID); got != 0 {
		t.Errorf("单目标跳转 5 次后 rr_index = %d, want 0(单目标不该写库)", got)
	}
}

// TestPickTargetStillRoundRobinsMultiTarget 多目标仍要推进轮询指针,
// 且保持单语句 UPDATE ... RETURNING 的原子语义(见 issue 05 的约束)。
func TestPickTargetStillRoundRobinsMultiTarget(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "kmuto",
		"targetUrls": []string{"https://a.example.com", "https://b.example.com"},
		"domainIds":  []int64{lid},
	})
	for i := 0; i < 4; i++ {
		resp := redirectGet(t, env, "localhost", "/kmuto")
		assertStatus(t, resp, http.StatusFound)
		_ = resp.Body.Close()
	}
	if got := rrIndex(t, env, link.ID); got != 4 {
		t.Errorf("多目标跳转 4 次后 rr_index = %d, want 4", got)
	}
}

// ---------- 并发 PATCH 不互相覆盖(issue 09) ----------

// concurrentPatch 用只读 cookie 快照并发 PATCH(与 concurrentPost 同理,
// 不能共用会写 cookie map 的 testClient)。
func concurrentPatch(env *testutil.Env, cookies []*http.Cookie, csrf, path string, body map[string]any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPatch, env.Server.URL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := env.Server.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes(), nil
}

// snapshotCookies 复制 testClient 的 cookie,供并发调用共享(之后全是只读)。
func snapshotCookies(c *testClient) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(c.cookies))
	for _, ck := range c.cookies {
		out = append(out, ck)
	}
	return out
}

// TestConcurrentPatchKeepsBothWrites 两个并发 PATCH 改不同字段,谁都不能覆盖对方。
// 修复前 UpdateLink 在事务外读当前值,两个请求拿到同一份旧快照,后提交的那个
// 会用自己的旧 link_type/status 覆盖掉前一个的。
func TestConcurrentPatchKeepsBothWrites(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "krace",
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{lid},
	})
	base := "/api/links/" + strconv.FormatInt(link.ID, 10)
	cookies, csrf := snapshotCookies(c), c.csrfToken()

	var gate sync.WaitGroup
	gate.Add(1)
	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		gate.Wait()
		_, _, _ = concurrentPatch(env, cookies, csrf, base, map[string]any{
			"linkType": "landing", "landingUrl": "https://page.example.com/lp",
		})
	}()
	go func() {
		defer done.Done()
		gate.Wait()
		_, _, _ = concurrentPatch(env, cookies, csrf, base, map[string]any{
			"status": "disabled",
		})
	}()
	gate.Done()
	done.Wait()

	resp := c.get(base)
	assertStatus(t, resp, http.StatusOK)
	got := decodeBody[store.Link](t, resp)
	if got.LinkType != store.LinkTypeLanding {
		t.Errorf("并发 PATCH 后 linkType = %q, want landing(被覆盖说明读当前值没生效)", got.LinkType)
	}
	if got.Status != "disabled" {
		t.Errorf("并发 PATCH 后 status = %q, want disabled", got.Status)
	}
}
