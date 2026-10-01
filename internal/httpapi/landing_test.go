package httpapi_test

// 16 — landing-pages 黑盒测试:落地页两来源的访问/点击计数、SDK、zip 上传校验、
// 类型与来源切换、purge 清理。

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"janus/internal/store"
	"janus/internal/testutil"
)

// makeZip 按 entries 构造 zip(压缩包内容,字节)。
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// uploadZip 以 multipart + CSRF 上传 zip 到 /api/links/{id}/landing。
func uploadZip(t *testing.T, c *testClient, linkID int64, zipBytes []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "landing.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(zipBytes); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost,
		c.env.Server.URL+fmt.Sprintf("/api/links/%d/landing", linkID), &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	if c.csrfToken() != "" {
		req.Header.Set("X-CSRF-Token", c.csrfToken())
	}
	resp, err := c.env.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// createLandingLink 创建落地页型短链并返回 link。
//
// source=="upload" 时走完整的「建链 → 上传 → 定型」三步(issue 04 之后,
// 创建请求里直接给 landingSource=upload 会被后端拒),**然后把托管目录删掉**,
// 造出「landing+upload 但文件不在盘上」的形态 —— 这是磁盘被清理 / 误删之后的
// 真实状态(访问要落一行 landing_missing),也是 API 已经造不出来的那一种。
// 需要「有托管文件」的落地页请用 createUploadLandingLink。
func createLandingLink(t *testing.T, c *testClient, domainID int64, code, source, landingURL string) *store.Link {
	t.Helper()
	if source == store.LandingSourceUpload {
		link := createUploadLandingLink(t, c, domainID, code,
			makeZip(t, map[string]string{"index.html": "<html>placeholder</html>"}))
		if err := os.RemoveAll(filepath.Join(c.env.Cfg.LandingUploadDir,
			strconv.FormatInt(link.ID, 10))); err != nil {
			t.Fatalf("remove placeholder landing dir: %v", err)
		}
		return link
	}
	resp := c.post("/api/links", map[string]any{
		"code":          code,
		"targetUrls":    []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":     []int64{domainID},
		"linkType":      "landing",
		"landingSource": source,
		"landingUrl":    landingURL,
	})
	assertStatus(t, resp, http.StatusCreated)
	l := decodeBody[store.Link](t, resp)
	return &l
}

// createUploadLandingLink 走完整的「建链 → 上传 → 定型」三步,返回一条
// landing+upload 且已托管 zip 的短链。
//
// 为什么测试也要分三步:issue 04 之后,「创建时直接给 landingSource=upload」
// 被后端拒了(新建短链不可能已有托管文件),而「建链 → 再上传」两次独立请求
// 的老写法会在两步之间留下一条永久 404 的空壳。前端与测试现在走同一条路径。
func createUploadLandingLink(t *testing.T, c *testClient, domainID int64, code string, z []byte) *store.Link {
	t.Helper()
	link := createLink(t, c, map[string]any{
		"code":       code,
		"targetUrls": []string{"https://t1.example.com", "https://t2.example.com"},
		"domainIds":  []int64{domainID},
	})
	resp := uploadZip(t, c, link.ID, z)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{
		"linkType": "landing", "landingSource": "upload",
	})
	assertStatus(t, resp, http.StatusOK)
	up := decodeBody[store.Link](t, resp)
	return &up
}

// stats 读取短链统计 {visits, clicks}。
func linkStats(t *testing.T, c *testClient, linkID int64) map[string]any {
	t.Helper()
	resp := c.get(fmt.Sprintf("/api/links/%d/stats", linkID))
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[map[string]any](t, resp)
}

func TestLandingURLVisitAndFields(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")

	if link.LinkType != "landing" || link.LandingSource != "url" || link.LandingURL != "https://page.example.com/lp" {
		t.Errorf("link fields = %+v", link)
	}
	if link.Clicks != 0 || link.LandingUploaded {
		t.Errorf("clicks=%d landingUploaded=%v, want 0/false", link.Clicks, link.LandingUploaded)
	}
	// 访问 → 固定 302 到落地页地址,visits +1
	resp := redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://page.example.com/lp" {
		t.Errorf("Location = %q", loc)
	}
	st := linkStats(t, c, link.ID)
	if st["visits"].(float64) != 1 || st["clicks"].(float64) != 0 {
		t.Errorf("stats = %v, want visits=1 clicks=0", st)
	}
}

func TestLandingClickCountsAndRoundRobin(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")

	// 点击端点:轮询目标、clicks+1、落一行 action=click 明细(不计入访问量)
	resp := redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Errorf("click1 Location = %q", loc)
	}
	resp = redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t2.example.com" {
		t.Errorf("click2 Location = %q", loc)
	}
	st := linkStats(t, c, link.ID)
	if st["visits"].(float64) != 0 || st["clicks"].(float64) != 2 {
		t.Errorf("stats = %v, want visits=0 clicks=2", st)
	}
	// 落地页视图仍计 Visit,点击端点不推进 visit
	resp = redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	st = linkStats(t, c, link.ID)
	if st["visits"].(float64) != 1 || st["clicks"].(float64) != 2 {
		t.Errorf("stats = %v, want visits=1 clicks=2", st)
	}
}

func TestRedirectTypeClickAndSDK404(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLink(t, c, map[string]any{
		"code":       "knorm",
		"targetUrls": []string{"https://x.example.com"},
		"domainIds":  []int64{lid},
	})
	if link.LinkType != "redirect" {
		t.Errorf("default linkType = %q, want redirect", link.LinkType)
	}
	resp := redirectGet(t, env, "localhost", "/knorm/click")
	assertStatus(t, resp, http.StatusNotFound)
	resp = redirectGet(t, env, "localhost", "/knorm/sdk.js")
	assertStatus(t, resp, http.StatusNotFound)
	// 跳转型 stats 仍含 clicks(恒 0)
	st := linkStats(t, c, link.ID)
	if st["clicks"].(float64) != 0 {
		t.Errorf("redirect clicks = %v, want 0", st["clicks"])
	}
}

func TestLandingSDKContent(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	createLandingLink(t, c, lid, "kpage", "url", "https://page.example.com/lp")

	resp := redirectGet(t, env, "localhost", "/kpage/sdk.js")
	assertStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "/kpage/click") || !strings.Contains(string(body), "Janus") {
		t.Errorf("sdk body missing click url / Janus: %s", body)
	}
}

func TestLandingUploadFlow(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	// 创建时直接选 upload 来源被拒(issue 04):不可能有"upload 来源却无文件"的短链
	resp := c.post("/api/links", map[string]any{
		"code": "kpage", "targetUrls": []string{"https://t1.example.com"},
		"domainIds": []int64{lid}, "linkType": "landing", "landingSource": "upload",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 走完整流程:建链(跳转型)→ 上传 → 定型。上传单层根文件夹应被自动剥离
	z := makeZip(t, map[string]string{
		"site/index.html":    "<html><body>JANUS-LANDING</body></html>",
		"site/css/style.css": "body{color:red}",
	})
	link := createUploadLandingLink(t, c, lid, "kpage", z)
	up := *link
	if up.LandingSource != "upload" || !up.LandingUploaded || up.LandingURL != "" {
		t.Errorf("after upload: %+v", up)
	}
	// 静态服务:根 index.html 与子资源
	resp = redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "JANUS-LANDING") {
		t.Errorf("index body = %s", body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control = %q", cc)
	}
	resp = redirectGet(t, env, "localhost", "/kpage/css/style.css")
	assertStatus(t, resp, http.StatusOK)
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=300") {
		t.Errorf("asset Cache-Control = %q", cc)
	}
	// 上传型下 SDK 与点击端点可用;点击计数 +1 并 302 目标
	resp = redirectGet(t, env, "localhost", "/kpage/sdk.js")
	assertStatus(t, resp, http.StatusOK)
	resp = redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Errorf("click Location = %q", loc)
	}
	st := linkStats(t, c, link.ID)
	if st["clicks"].(float64) != 1 {
		t.Errorf("clicks = %v, want 1", st["clicks"])
	}
}

func TestLandingZipValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)

	cases := []struct {
		name string
		z    []byte
	}{
		{"no index.html", makeZip(t, map[string]string{"a.html": "x"})},
		{"traversal", makeZip(t, map[string]string{"../evil.html": "x", "index.html": "ok"})},
		{"bad ext", makeZip(t, map[string]string{"index.html": "ok", "x.exe": "MZ"})},
		{"oversize", makeZip(t, map[string]string{"index.html": strings.Repeat("0", 11<<20)})},
		{"symlink", makeZipWithSymlink(t)},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := fmt.Sprintf("kpage%c", 'a'+i)
			// 用跳转型短链当容器:上传端点现在接受任意类型(见 issue 04 的三步流程),
			// 而 zip 校验失败应当在任何类型下都拒掉。
			link := createLink(t, c, map[string]any{
				"code": code, "targetUrls": []string{"https://t1.example.com"},
				"domainIds": []int64{lid},
			})
			resp := uploadZip(t, c, link.ID, tc.z)
			assertStatus(t, resp, http.StatusBadRequest)
			// 校验失败不产生文件(静态路径 404,且不产生成功的访问明细)
			resp = redirectGet(t, env, "localhost", "/"+code+"/")
			assertStatus(t, resp, http.StatusNotFound)
			st := linkStats(t, c, link.ID)
			if st["visits"].(float64) != 0 || st["clicks"].(float64) != 0 {
				t.Errorf("stats = %v, want 0/0", st)
			}
		})
	}

	// 创建校验:landing+url 缺 landingUrl → 400;非法类型 → 400
	resp := c.post("/api/links", map[string]any{
		"code": "bad1", "targetUrls": []string{"https://x.example.com"},
		"domainIds": []int64{lid}, "linkType": "landing", "landingSource": "url",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	resp = c.post("/api/links", map[string]any{
		"code": "bad2", "targetUrls": []string{"https://x.example.com"},
		"domainIds": []int64{lid}, "linkType": "nope",
	})
	assertStatus(t, resp, http.StatusBadRequest)
}

// makeZipWithSymlink 构造含符号链接条目的 zip。
func makeZipWithSymlink(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	h := &zip.FileHeader{Name: "link", Method: zip.Store}
	h.SetMode(os.ModeSymlink | 0o777)
	lf, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lf.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLandingSwitchAndPurge(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	z := makeZip(t, map[string]string{"index.html": "<html>UP</html>"})
	link := createUploadLandingLink(t, c, lid, "kpage", z)
	resp := redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 切 url 来源:删除已上传文件
	resp = c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{
		"landingSource": "url", "landingUrl": "https://page.example.com/new",
	})
	assertStatus(t, resp, http.StatusOK)
	resp = redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusNotFound)
	resp = redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://page.example.com/new" {
		t.Errorf("Location = %q", loc)
	}

	// 切 redirect 型:直达目标,点击端点/SDK 404
	resp = c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{
		"linkType": "redirect",
	})
	assertStatus(t, resp, http.StatusOK)
	resp = redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://t1.example.com" {
		t.Errorf("Location = %q", loc)
	}
	resp = redirectGet(t, env, "localhost", "/kpage/click")
	assertStatus(t, resp, http.StatusNotFound)
	resp = redirectGet(t, env, "localhost", "/kpage/sdk.js")
	assertStatus(t, resp, http.StatusNotFound)

	// purge 后全部 404
	resp = c.post(fmt.Sprintf("/api/links/%d/purge", link.ID), nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp = redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusNotFound)
}

// TestDomainDeleteRemovesPurgedLandingFiles 删除域名会物理清除仅关联该域名的
// 已逻辑删除短链,其上传落地页文件也必须一并清理,避免磁盘泄漏。
func TestDomainDeleteRemovesPurgedLandingFiles(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	d := addDomain(t, c, "localhost")
	link := createUploadLandingLink(t, c, d.ID, "kpage",
		makeZip(t, map[string]string{"index.html": "<html>UP</html>"}))

	dir := filepath.Join(env.Cfg.LandingUploadDir, strconv.FormatInt(link.ID, 10))
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("landing dir before delete: %v", err)
	}

	resp := c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = c.del("/api/domains/" + strconv.FormatInt(d.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) == false {
		t.Fatalf("landing dir after domain delete = %v, want removed", err)
	}
}

// ---------- 点击路径的失败明细(issue 08) ----------

// TestLandingClickRecordsUnavailableLink 停用/删除后的落地页被点击时,
// 要落一行 action=click 的 failed 明细(与跳转侧同一口径),而不是静默 404。
// 修复前点击走严格口径 ResolveLink,这两行明细压根不存在,租户从点击侧看不到
// "停用之后还有人点"。
func TestLandingClickRecordsUnavailableLink(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kcpick", "url", "https://page.example.com/lp")

	resp := c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/kcpick/click")
	assertStatus(t, resp, http.StatusNotFound)
	action, outcome, reason, _, _ := ruleVisit(t, env, link.ID)
	if action != store.VisitActionClick || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonLinkDisabled {
		t.Errorf("停用后点击的明细 = %q/%q/%q, want click/failed/link_disabled", action, outcome, reason)
	}

	resp = c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{"status": "enabled"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.del("/api/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = redirectGet(t, env, "localhost", "/kcpick/click")
	assertStatus(t, resp, http.StatusNotFound)
	action, outcome, reason, _, _ = ruleVisit(t, env, link.ID)
	if action != store.VisitActionClick || outcome != store.VisitOutcomeFailed ||
		reason != store.VisitReasonLinkDeleted {
		t.Errorf("删除后点击的明细 = %q/%q/%q, want click/failed/link_deleted", action, outcome, reason)
	}
}

// ---------- upload 来源必须已有托管文件(issue 04) ----------

// TestPatchLandingUploadRequiresHostedFile 编辑时把来源切到 upload,
// 但托管文件不在 → 400;上传之后同样的 PATCH 放行。
func TestPatchLandingUploadRequiresHostedFile(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kneed", "url", "https://page.example.com/lp")

	// 没有托管文件时切 upload → 400(否则这条短链会永久 404)
	resp := c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{"landingSource": "upload"})
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// 上传之后再切同样放行
	resp = uploadZip(t, c, link.ID, makeZip(t, map[string]string{"index.html": "<html>OK</html>"}))
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
	resp = c.patch(fmt.Sprintf("/api/links/%d", link.ID), map[string]any{"landingSource": "upload"})
	assertStatus(t, resp, http.StatusOK)
	up := decodeBody[store.Link](t, resp)
	if up.LandingSource != store.LandingSourceUpload || !up.LandingUploaded {
		t.Errorf("切换后 = %+v, want upload 来源且已托管", up)
	}
}
