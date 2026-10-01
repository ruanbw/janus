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
	"time"

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
	// 限流放在最前面:一旦放行,后面每一次点击都要跑一次 ip2region 地理解析 +
	// 插一行 visits 明细。限流命中时这些一次都不该发生,所以连域名解析都跳过
	// (renderVisitorError 传 tenantID=0,不去 DB 取租户自定义 429 页)。
	if !a.allowVisitor(c) {
		a.renderVisitorError(c, http.StatusTooManyRequests, 0, nil)
		return
	}
	// 宽松命中(与跳转侧同一口径):停用/逻辑删除的落地页被点击时,短码仍能归属到
	// 该短链,于是能落一行 action=click 的 failed 明细。用严格口径 ResolveLink
	// 会直接 ErrNotFound,租户从访问明细里根本看不到"停用之后还有人点"(issue 08)。
	link, d, reason, err := a.resolveLandingLinkForVisit(c, code)
	if err != nil {
		var tenantID int64
		if dom := a.resolveDomainByHost(c); dom != nil {
			tenantID = dom.TenantID
		}
		a.renderVisitorError(c, http.StatusNotFound, tenantID, nil)
		return
	}
	// 非落地页型没有"按钮"可言:404 且不记明细。这不是一条可归属到该短链的失败
	// (访问者把跳转型短链当落地页用了),记下来只会污染明细。
	if link.LinkType != store.LinkTypeLanding {
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	if reason != "" {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: store.VisitActionClick, Outcome: store.VisitOutcomeFailed, Reason: reason,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
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

// resolveLandingLinkForVisit 按 Host+code 宽松命中短链(短码命中即返回),
// 返回命中的域名(点击明细用它取 domain_id)与不可用原因
// (link_disabled / link_deleted;可用时为空串)。
func (a *API) resolveLandingLinkForVisit(c *gin.Context, code string) (*store.Link, *store.Domain, string, error) {
	if code == "" || strings.Contains(code, "/") {
		return nil, nil, "", store.ErrNotFound
	}
	d := a.resolveDomainByHost(c)
	if d == nil {
		return nil, nil, "", store.ErrNotFound
	}
	link, resolved, reason, err := a.store.LookupLinkForVisit(c.Request.Context(), d.ID, code)
	if err != nil {
		return nil, nil, "", err
	}
	return link, resolved, reason, nil
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
	// 清洗与越界校验提到入口,再做任何 os.Stat。
	// 原先那次 stat 用的是**未清洗**的 rel:穿越请求虽然最终仍会被 serveLandingFile
	// 的前缀校验拒掉,但 stat 已经发生过了 —— "某路径是否为目录"由此变成一个
	// 可测量的存在性信号(命中目录走 index.html、不命中直接 404)。
	abs, cleanRel, ok := a.resolveLandingFilePath(link.ID, rel)
	if !ok {
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 目录路径兜底:尝试目录下的 index.html(abs 已经确定落在该短链目录内)
	if fi, statErr := os.Stat(abs); statErr == nil && fi.IsDir() {
		var abs2, clean2 string
		var ok2 bool
		abs2, clean2, ok2 = a.resolveLandingFilePath(link.ID, path.Join(cleanRel, "index.html"))
		if !ok2 {
			a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
			return
		}
		abs, cleanRel = abs2, clean2
	}
	a.serveLandingFile(c, link, abs, cleanRel)
}

// resolveLandingFilePath 把相对路径清洗成短链目录内的绝对路径。
// 越界(路径穿越)返回 ok=false。清洗只在这里做一次,调用方拿到的就是最终路径,
// 不需要在"先 stat 看看"与"真正要读"之间保持两份一致的清洗逻辑。
func (a *API) resolveLandingFilePath(linkID int64, rel string) (abs string, cleanRel string, ok bool) {
	root := a.landingLinkDir(linkID)
	clean := path.Clean("/" + rel)
	fp := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	absRoot, err1 := filepath.Abs(root)
	absFp, err2 := filepath.Abs(fp)
	if err1 != nil || err2 != nil || !strings.HasPrefix(absFp, absRoot+string(filepath.Separator)) {
		return "", "", false
	}
	return absFp, strings.TrimPrefix(clean, "/"), true
}

// serveLandingFile 服务上传落地页中的单个文件(禁目录列表)。
// abs / cleanRel 来自 resolveLandingFilePath,已完成清洗与越界校验。
func (a *API) serveLandingFile(c *gin.Context, link *store.Link, absFp, cleanRel string) {
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
	base := path.Base(cleanRel)
	if base == "index.html" {
		c.Header("Cache-Control", "no-cache") // 入口不缓存,重新上传即时生效
	} else {
		c.Header("Cache-Control", "public, max-age=300")
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, base, info.ModTime(), f)
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
	commit, rollback, err := a.installLandingZip(link.ID, raw)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, err.Error())
		return
	}
	// 落地页型:来源切到 upload、landingUrl 清空。跳转型只装文件、不改来源 ——
	// 建链 → 上传 → PATCH 定型 是「创建时不允许 upload 来源(issue 04)」之后
	// 唯一走得通的顺序,所以上传必须接受跳转型短链;但"把别人的跳转型短链
	// 悄悄变成落地页"是远比"文件先装上、类型稍后由用户 PATCH 决定"更大的惊吓。
	// 注意顺序:文件先落地、来源后翻转,"upload 来源必有托管文件"这条不变式
	// 在任何时刻都成立(反过来先翻来源就会造出永久 404 的短链)。
	if link.LinkType == store.LinkTypeLanding {
		landingSource, landingURL := store.LandingSourceUpload, ""
		if _, err := a.store.UpdateLink(c.Request.Context(), t.ID, id, store.LinkUpdate{
			LandingSource: &landingSource, LandingURL: &landingURL,
		}); err != nil {
			// 写库失败 → 把刚装上去的落地页换回旧目录,不留孤儿文件
			rollback()
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	commit()
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

// landingMaxTotalBytes 解压后所有文件的大小合计上限。
// 与 LandingMaxZipBytes 是两个语义:前者管"磁盘被撑多大",后者管"上传的包有多大"。
// 用一个数兼两职会出现很别扭的组合(比如把上限调小到 2MB 后,连解压后 1.5MB 的
// 正常站点都传不上去),所以这里各自取各自的配置。
func (a *API) landingMaxTotalBytes() int64 {
	if a.cfg.LandingMaxTotalBytes > 0 {
		return a.cfg.LandingMaxTotalBytes
	}
	return a.cfg.LandingMaxZipBytes
}

// installLandingZip 校验并解压 zip 到 uploads/{linkID},返回 commit / rollback 两个闭包。
//
// 换目录是原子的:先解到临时目录,再把旧目录 rename 成备份名,最后把临时目录
// rename 成 dest。原实现是 `RemoveAll(dest)` 再 `Rename(tmp, dest)` —— 两步之间
// 有一个"目录不存在"的窗口,并发访问会读到 landing_missing,凭空多出一批假失败明细。
// rename(2) 在同一文件系统内是原子的,不存在这个窗口。
//
// 为什么还要 rollback:解压成功 ≠ 整件事成功。handleUploadLanding 随后还要写库
// (把 landing_source 切成 upload);写库失败时磁盘上已经装好了新目录,而库里的短链
// 还是旧来源 —— 那份文件永远没人会访问,就是纯磁盘泄漏。调用方写库失败时调
// rollback 把旧目录换回去,不留孤儿。
func (a *API) installLandingZip(linkID int64, raw []byte) (commit func(), rollback func(), err error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, nil, zipErr{"无法解析 zip 压缩包"}
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
			return nil, nil, err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return nil, nil, zipErr{"压缩包不允许包含符号链接:" + name}
		}
		files = append(files, fentry{f: f, rel: rel})
	}
	if len(files) == 0 {
		return nil, nil, zipErr{"压缩包为空"}
	}
	if len(files) > a.cfg.LandingMaxFiles {
		return nil, nil, zipErr{fmt.Sprintf("文件数超过上限(%d)", a.cfg.LandingMaxFiles)}
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
	maxTotal := a.landingMaxTotalBytes()
	for _, e := range files {
		if e.rel == "index.html" {
			hasIndex = true
		}
		if !allowedLandingExt(e.rel) {
			return nil, nil, zipErr{"不允许的文件类型:" + e.rel}
		}
		// 先按 uint64 比较再转 int64:恶意 zip 可把 UncompressedSize64 声明为
		// 接近 MaxUint64,直接转 int64 会溢出为负数并绕过总大小检查。
		if e.f.UncompressedSize64 > uint64(maxTotal) {
			return nil, nil, zipErr{"解压后总大小超过上限"}
		}
		total += int64(e.f.UncompressedSize64)
		if total > maxTotal {
			return nil, nil, zipErr{"解压后总大小超过上限"}
		}
	}
	if !hasIndex {
		return nil, nil, zipErr{"压缩包必须包含 index.html"}
	}
	// 全部校验通过后解压到临时目录,再原子换到 dest
	dest := a.landingLinkDir(linkID)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, nil, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".landing-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmp)
	for _, e := range files {
		fp := filepath.Join(tmp, filepath.FromSlash(e.rel))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return nil, nil, err
		}
		if err := extractZipFile(e.f, fp); err != nil {
			return nil, nil, err
		}
	}
	// 旧目录先退到备份名(rename 原子);不存在则备份名为空
	backup := ""
	if _, statErr := os.Stat(dest); statErr == nil {
		backup = dest + fmt.Sprintf(".old-%d", time.Now().UnixNano())
		if err := os.Rename(dest, backup); err != nil {
			return nil, nil, err
		}
	} else if !os.IsNotExist(statErr) {
		return nil, nil, statErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		// 换不上去就把旧目录换回来,别把租户已有的落地页弄丢
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return nil, nil, err
	}
	commit = func() {
		if backup != "" {
			_ = os.RemoveAll(backup)
		}
	}
	rollback = func() {
		_ = os.RemoveAll(dest)
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
	}
	return commit, rollback, nil
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
