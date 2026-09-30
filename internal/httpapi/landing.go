package httpapi

// 16 — 落地页型短链:点击端点(计数+302 轮询目标)、每短链 SDK、上传落地页静态服务与 zip 上传。
// 设计见 docs/adr/0005-landing-pages-click-sdk.md,契约见 .scratch/cloak/api-contract.md。

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/domain"
	"cloak/internal/store"
)

// handleLandingFallback NoRoute 兜底:gin 路由树不支持 /:code 与 /:code/... 子路由并存,
// 落地页型短链的二级路径(click、sdk.js、静态文件)在此按 "短码/后缀" 手工分发。
func (a *API) handleLandingFallback(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/internal/") {
		writeErr(c, http.StatusNotFound, errNotFound, "route not found")
		return
	}
	d := a.resolveDomainByHost(c)
	var tenantID int64
	if d != nil {
		tenantID = d.TenantID
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	p := strings.TrimPrefix(c.Request.URL.Path, "/")
	code, rest, ok := strings.Cut(p, "/")
	if !ok || !domain.IsValidCode(code) {
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	switch rest {
	case "click":
		a.handleLandingClick(c, code)
	case "sdk.js":
		a.handleLandingSDK(c, code)
	default:
		a.handleLandingFile(c, code, rest)
	}
}

// landingLinkDir 上传落地页目录(每短链一个子目录,目录名 = 短链 ID)。
func (a *API) landingLinkDir(linkID int64) string {
	return filepath.Join(a.cfg.LandingUploadDir, strconv.FormatInt(linkID, 10))
}

// landingUploaded 是否已上传落地页(以 index.html 存在为准)。
func (a *API) landingUploaded(linkID int64) bool {
	_, err := os.Stat(filepath.Join(a.landingLinkDir(linkID), "index.html"))
	return err == nil
}

// withLandingUploaded 装饰 Link 的 landingUploaded 字段(仅 landing+upload 来源)。
func (a *API) withLandingUploaded(l *store.Link) *store.Link {
	if l.LinkType == store.LinkTypeLanding && l.LandingSource == store.LandingSourceUpload {
		l.LandingUploaded = a.landingUploaded(l.ID)
	}
	return l
}

// removeLandingFiles 删除短链的落地页文件目录(幂等)。
func (a *API) removeLandingFiles(linkID int64) {
	_ = os.RemoveAll(a.landingLinkDir(linkID))
}

// resolveLandingLink 按 Host+code 命中启用、未删除、landing 型短链;其余 ErrNotFound。
// 同时返回命中的域名(SDK 注入 canonical FQDN,避免信任任意 Host 头;点击明细也用它取 domain_id)。
func (a *API) resolveLandingLink(c *gin.Context, code string) (*store.Link, *store.Domain, error) {
	if code == "" || strings.Contains(code, "/") {
		return nil, nil, store.ErrNotFound
	}
	d := a.resolveDomainByHost(c)
	if d == nil {
		return nil, nil, store.ErrNotFound
	}
	link, resolved, err := a.store.ResolveLink(c.Request.Context(), d.ID, code)
	if err != nil {
		return nil, nil, err
	}
	if link.LinkType != store.LinkTypeLanding {
		return nil, nil, store.ErrNotFound
	}
	return link, resolved, nil
}

// handleLandingClick GET /{code}/click — 点击计数 +1、落一行 action=click 明细,
// 并 302 到轮询目标。计数放在选到目标之后:选不到目标的失败点击不应计入点击数。
func (a *API) handleLandingClick(c *gin.Context, code string) {
	link, d, err := a.resolveLandingLink(c, code)
	if err != nil {
		var tenantID int64
		if dom := a.resolveDomainByHost(c); dom != nil {
			tenantID = dom.TenantID
		}
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	targetURL, err := a.store.PickTarget(c.Request.Context(), link.ID)
	if err != nil {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: store.VisitActionClick, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	_ = a.store.IncrementClicks(c.Request.Context(), link.ID) // 计数失败不阻断跳转
	a.recordVisit(c, store.VisitRecord{
		LinkID: link.ID, DomainID: d.ID,
		Action: store.VisitActionClick, Outcome: store.VisitOutcomeSuccess, TargetURL: targetURL,
	})
	c.Redirect(http.StatusFound, targetURL) // 点击跳转固定 302
}

// landingSDKTemplate SDK 模板:__CLICK_URL__ 由服务器注入点击端点绝对地址。
// 注:闭包 + addEventListener;同一页面可引入多个 SDK(绑不同短链),window.Cloak 为最后加载者,
// 但 bind 返回的监听闭包各自持有自己 SDK 的 clickUrl,互不影响。
const landingSDKTemplate = `/* CLOAK 落地页 SDK:绑定按钮点击 → 平台点击端点(计数后跳转目标 URL)。 */
(function () {
  'use strict';
  var clickUrl = "__CLICK_URL__";
  function resolve(sel) {
    if (typeof sel === 'string') { return document.querySelector(sel); }
    return sel;
  }
  var cloak = {
    clickUrl: clickUrl,
    bind: function (selector, opts) {
      var target = resolve(selector);
      if (!target) { return false; }
      opts = opts || {};
      target.addEventListener('click', function (ev) {
        if (opts.newTab) {
          ev.preventDefault();
          var w = window.open(clickUrl, '_blank');
          if (w) { w.opener = null; }
        } else {
          window.location.href = clickUrl;
        }
      });
      return true;
    }
  };
  window.Cloak = cloak;
})();
`

// handleLandingSDK GET /{code}/sdk.js — 生成内嵌点击端点绝对地址的 SDK。
func (a *API) handleLandingSDK(c *gin.Context, code string) {
	link, domain, err := a.resolveLandingLink(c, code)
	if err != nil {
		var tenantID int64
		if dom := a.resolveDomainByHost(c); dom != nil {
			tenantID = dom.TenantID
		}
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	// 点击端点绝对地址:与 SDK 请求同源(Host 即短链域名),外部落地页跨域引用同样可用。
	proto := "https"
	if p := c.GetHeader("X-Forwarded-Proto"); p == "http" || p == "https" {
		proto = p
	} else if c.Request.TLS == nil {
		proto = "http"
	}
	// 使用数据库中命中的 FQDN 而不是请求 Host:Host 可被客户端任意构造,
	// 直接拼进 JS 字符串会形成响应注入面。
	clickURL := proto + "://" + domain.FQDN + "/" + link.Code + "/click"
	js := strings.ReplaceAll(landingSDKTemplate, "__CLICK_URL__", clickURL)
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.String(http.StatusOK, js)
}

// handleLandingFile GET /{code}/<path> — 上传落地页静态资源;
// rel 为 ""(即请求 /{code}/)时服务根 index.html。
func (a *API) handleLandingFile(c *gin.Context, code, rel string) {
	link, _, err := a.resolveLandingLink(c, code)
	if err != nil || link.LandingSource != store.LandingSourceUpload {
		var tenantID int64
		if dom := a.resolveDomainByHost(c); dom != nil {
			tenantID = dom.TenantID
		}
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = "index.html"
	}
	// 目录路径兜底:尝试目录下的 index.html
	if fi, statErr := os.Stat(filepath.Join(a.landingLinkDir(link.ID), rel)); statErr == nil && fi.IsDir() {
		rel = path.Join(rel, "index.html")
	}
	a.serveLandingFile(c, link, rel)
}

// serveLandingFile 安全地服务上传落地页中的单个文件(防路径穿越、禁目录列表)。
func (a *API) serveLandingFile(c *gin.Context, link *store.Link, rel string) {
	root := a.landingLinkDir(link.ID)
	clean := path.Clean("/" + rel)
	fp := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	absRoot, err1 := filepath.Abs(root)
	absFp, err2 := filepath.Abs(fp)
	if err1 != nil || err2 != nil || !strings.HasPrefix(absFp, absRoot+string(filepath.Separator)) {
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	f, err := os.Open(absFp)
	if err != nil {
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(absFp))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if path.Base(clean) == "index.html" {
		c.Header("Cache-Control", "no-cache") // 入口不缓存,重新上传即时生效
	} else {
		c.Header("Cache-Control", "public, max-age=300")
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, path.Base(clean), info.ModTime(), f)
}

// handleUploadLanding POST /api/links/{id}/landing — multipart zip 上传(替换式)。
func (a *API) handleUploadLanding(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	link, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	if link.LinkType != store.LinkTypeLanding {
		writeErr(c, http.StatusBadRequest, errValidation, "仅落地页型短链可上传落地页")
		return
	}
	// 限制请求体(multipart 头部冗余 64KB),zip 大小上限在读出后校验
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.cfg.LandingMaxZipBytes+64<<10)
	fh, err := c.FormFile("file")
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "需要 multipart 字段 file(zip 压缩包)")
		return
	}
	src, err := fh.Open()
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "无法读取上传文件")
		return
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, a.cfg.LandingMaxZipBytes+1))
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "读取上传文件失败")
		return
	}
	if int64(len(raw)) > a.cfg.LandingMaxZipBytes {
		writeErr(c, http.StatusBadRequest, errValidation, "压缩包超过大小上限")
		return
	}
	if err := a.installLandingZip(link.ID, raw); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, err.Error())
		return
	}
	// 落地页来源切到 upload,landingUrl 清空
	landingSource, landingURL := store.LandingSourceUpload, ""
	if _, err := a.store.UpdateLink(c.Request.Context(), t.ID, id, store.LinkUpdate{
		LandingSource: &landingSource, LandingURL: &landingURL,
	}); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	updated, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, a.withLandingUploaded(updated))
}

// zipErr 校验失败(用户可读原因)。
type zipErr struct{ msg string }

func (e zipErr) Error() string { return e.msg }

// installLandingZip 校验并解压 zip 到 uploads/{linkID}(先解到临时目录再原子替换)。
func (a *API) installLandingZip(linkID int64, raw []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return zipErr{"无法解析 zip 压缩包"}
	}
	type fentry struct {
		f   *zip.File
		rel string
	}
	var files []fentry
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if f.FileInfo().IsDir() {
			continue
		}
		rel, err := zipEntryRel(name)
		if err != nil {
			return err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return zipErr{"压缩包不允许包含符号链接:" + name}
		}
		files = append(files, fentry{f: f, rel: rel})
	}
	if len(files) == 0 {
		return zipErr{"压缩包为空"}
	}
	if len(files) > a.cfg.LandingMaxFiles {
		return zipErr{fmt.Sprintf("文件数超过上限(%d)", a.cfg.LandingMaxFiles)}
	}
	// 单层根文件夹自动剥离:所有条目共享同一首层目录时去掉
	if parts := strings.SplitN(files[0].rel, "/", 2); len(parts) == 2 {
		cand := parts[0]
		all := true
		for _, e := range files {
			p := strings.SplitN(e.rel, "/", 2)
			if len(p) != 2 || p[0] != cand {
				all = false
				break
			}
		}
		if all {
			for i := range files {
				files[i].rel = strings.TrimPrefix(files[i].rel, cand+"/")
			}
		}
	}
	hasIndex := false
	var total int64
	for _, e := range files {
		if e.rel == "index.html" {
			hasIndex = true
		}
		if !allowedLandingExt(e.rel) {
			return zipErr{"不允许的文件类型:" + e.rel}
		}
		// 先按 uint64 比较再转 int64:恶意 zip 可把 UncompressedSize64 声明为
		// 接近 MaxUint64,直接转 int64 会溢出为负数并绕过总大小检查。
		if e.f.UncompressedSize64 > uint64(a.cfg.LandingMaxZipBytes) {
			return zipErr{"解压后总大小超过上限"}
		}
		total += int64(e.f.UncompressedSize64)
		if total > a.cfg.LandingMaxZipBytes {
			return zipErr{"解压后总大小超过上限"}
		}
	}
	if !hasIndex {
		return zipErr{"压缩包必须包含 index.html"}
	}
	// 全部校验通过后解压:临时目录 → 删除旧目录 → 原子替换
	dest := a.landingLinkDir(linkID)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".landing-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, e := range files {
		fp := filepath.Join(tmp, filepath.FromSlash(e.rel))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(e.f, fp); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return nil
}

// zipEntryRel 归一化条目相对路径,拒绝绝对路径与路径穿越。
func zipEntryRel(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", zipErr{"非法路径:" + name}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", zipErr{"压缩包不允许路径穿越:" + name}
	}
	return clean, nil
}

// allowedLandingExt 落地页压缩包扩展名白名单。
func allowedLandingExt(rel string) bool {
	switch strings.ToLower(path.Ext(rel)) {
	case ".html", ".htm", ".css", ".js", ".json", ".txt",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".woff", ".woff2":
		return true
	}
	return false
}

// extractZipFile 解压单个条目,按声明的未压缩大小限读(防头部虚报)。
func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return zipErr{"压缩包损坏:" + f.Name}
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	n, err := io.CopyN(out, rc, int64(f.UncompressedSize64)+1)
	cerr := out.Close()
	if err != nil && err != io.EOF {
		return zipErr{"压缩包损坏:" + f.Name}
	}
	if n > int64(f.UncompressedSize64) {
		return zipErr{"压缩包条目大小与声明不符:" + f.Name}
	}
	if cerr != nil {
		return cerr
	}
	return nil
}
