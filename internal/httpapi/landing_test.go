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
	"strings"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
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
func createLandingLink(t *testing.T, c *testClient, domainID int64, code, source, landingURL string) *store.Link {
	t.Helper()
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

	// 点击端点:轮询目标、clicks+1、不记 Visit
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
	if !strings.Contains(string(body), "/kpage/click") || !strings.Contains(string(body), "Cloak") {
		t.Errorf("sdk body missing click url / Cloak: %s", body)
	}
}

func TestLandingUploadFlow(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	lid := localhostDomainID(t, c)
	link := createLandingLink(t, c, lid, "kpage", "upload", "")
	if link.LandingUploaded {
		t.Errorf("landingUploaded = true before upload")
	}
	// 未上传:访问裸短码 302 到 /kpage/,但静态目录 404
	resp := redirectGet(t, env, "localhost", "/kpage")
	assertStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "/kpage/" {
		t.Errorf("Location = %q", loc)
	}
	resp = redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusNotFound)

	// 上传(单层根文件夹应被自动剥离)
	z := makeZip(t, map[string]string{
		"site/index.html":    "<html><body>CLOAK-LANDING</body></html>",
		"site/css/style.css": "body{color:red}",
	})
	resp = uploadZip(t, c, link.ID, z)
	assertStatus(t, resp, http.StatusOK)
	up := decodeBody[store.Link](t, resp)
	if up.LandingSource != "upload" || !up.LandingUploaded || up.LandingURL != "" {
		t.Errorf("after upload: %+v", up)
	}
	// 静态服务:根 index.html 与子资源
	resp = redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "CLOAK-LANDING") {
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
			link := createLandingLink(t, c, lid, code, "upload", "")
			resp := uploadZip(t, c, link.ID, tc.z)
			assertStatus(t, resp, http.StatusBadRequest)
			// 校验失败不产生文件(静态路径 404,且不记 Visit)
			resp = redirectGet(t, env, "localhost", "/"+code+"/")
			assertStatus(t, resp, http.StatusNotFound)
			st := linkStats(t, c, link.ID)
			if st["visits"].(float64) != 0 || st["clicks"].(float64) != 0 {
				t.Errorf("stats = %v, want 0/0", st)
			}
		})
	}

	// 非 landing 型不可上传
	plain := createLink(t, c, map[string]any{
		"code":       "knorm2",
		"targetUrls": []string{"https://x.example.com"},
		"domainIds":  []int64{lid},
	})
	resp := uploadZip(t, c, plain.ID, makeZip(t, map[string]string{"index.html": "x"}))
	assertStatus(t, resp, http.StatusBadRequest)

	// 创建校验:landing+url 缺 landingUrl → 400;非法类型 → 400
	resp = c.post("/api/links", map[string]any{
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
	link := createLandingLink(t, c, lid, "kpage", "upload", "")
	z := makeZip(t, map[string]string{"index.html": "<html>UP</html>"})
	resp := uploadZip(t, c, link.ID, z)
	assertStatus(t, resp, http.StatusOK)
	resp = redirectGet(t, env, "localhost", "/kpage/")
	assertStatus(t, resp, http.StatusOK)

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
