package httpapi

// 05/06/07 — 短链:创建(自动/自定义短码、关联 active 域名、配额校验)、查询、
// 更新、逻辑删除/物理删除、访问列表与统计。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"strconv"

	"cloak/internal/domain"
	"cloak/internal/store"
)

// validTargetURL 目标 URL 任意协议(开放重定向),但拒绝控制字符(CRLF header 注入防护)。
func validTargetURL(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// validTargetURLs 校验目标 URL 列表:至少 1 个,逐项沿用 validTargetURL 规则。
func validTargetURLs(urls []string) error {
	if len(urls) == 0 {
		return errors.New("targetUrls 至少需要一个目标 URL")
	}
	for _, u := range urls {
		if !validTargetURL(u) {
			return errors.New("目标 URL 非法(不能为空、超长或包含控制字符)")
		}
	}
	return nil
}

// RedirectStatus 跳转方式(契约枚举:"301" | "302")。
// 兼容字符串("301"/"302")与数字(301/302)两种 JSON 表示。
type RedirectStatus string

// UnmarshalJSON 接受字符串或数字形式的 301/302。
func (s *RedirectStatus) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch n := v.(type) {
	case string:
		*s = RedirectStatus(n)
	case float64:
		*s = RedirectStatus(strconv.Itoa(int(n)))
	default:
		return fmt.Errorf("redirectStatus must be \"301\" or \"302\"")
	}
	return nil
}

type createLinkReq struct {
	Code           string          `json:"code"`
	TargetURLs     []string        `json:"targetUrls"`
	DomainIDs      []int64         `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
	LinkType       string          `json:"linkType"`
	LandingSource  string          `json:"landingSource"`
	LandingURL     string          `json:"landingUrl"`
}

// normalizeLanding 校验并归一化落地页字段(16):返回 linkType/landingSource/landingURL。
func normalizeLandingFields(linkType, landingSource, landingURL string) (string, string, string, error) {
	if linkType == "" {
		linkType = store.LinkTypeRedirect
	}
	if landingSource == "" {
		landingSource = store.LandingSourceURL
	}
	if linkType != store.LinkTypeRedirect && linkType != store.LinkTypeLanding {
		return "", "", "", errors.New("linkType 必须为 redirect 或 landing")
	}
	if landingSource != store.LandingSourceURL && landingSource != store.LandingSourceUpload {
		return "", "", "", errors.New("landingSource 必须为 url 或 upload")
	}
	switch {
	case linkType == store.LinkTypeRedirect:
		landingSource, landingURL = store.LandingSourceURL, ""
	case landingSource == store.LandingSourceUpload:
		landingURL = ""
	default: // landing + url
		if !validTargetURL(landingURL) {
			return "", "", "", errors.New("落地页型短链(url 来源)必须填写合法的落地页地址")
		}
	}
	return linkType, landingSource, landingURL, nil
}

func (a *API) handleCreateLink(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req createLinkReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	link, err := a.createLink(c, t, req)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	writeJSON(c, http.StatusCreated, a.withLandingUploaded(link))
}

// createLink 供后台短链创建共用。
func (a *API) createLink(c *gin.Context, t *store.Tenant, req createLinkReq) (*store.Link, error) {
	if err := validTargetURLs(req.TargetURLs); err != nil {
		return nil, apiErr{http.StatusBadRequest, errValidation, err.Error(), nil}
	}
	redirectStatus := store.RedirectStatus302
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != "301" && *req.RedirectStatus != "302" {
			return nil, apiErr{http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302", nil}
		}
		redirectStatus = store.RedirectStatus(*req.RedirectStatus)
	}
	linkType, landingSource, landingURL, err := normalizeLandingFields(req.LinkType, req.LandingSource, req.LandingURL)
	if err != nil {
		return nil, apiErr{http.StatusBadRequest, errValidation, err.Error(), nil}
	}
	if len(req.DomainIDs) == 0 {
		return nil, apiErr{http.StatusBadRequest, errValidation, "至少关联一个域名", nil}
	}
	// 域名必须属于当前租户且 active
	domainIDs, err := a.validateLinkDomains(c, t.ID, req.DomainIDs)
	if err != nil {
		return nil, err
	}
	// 短链配额:按"尚未物理删除"计数
	usage, err := a.store.Usage(c.Request.Context(), t.ID)
	if err != nil {
		return nil, apiErr{http.StatusInternalServerError, errInternal, "internal error", nil}
	}
	if usage.Links >= usage.MaxLinks {
		return nil, apiErr{http.StatusForbidden, errLinkQuota,
			"短链数量已达上限", map[string]any{"usage": usage}}
	}

	if req.Code != "" {
		if !domain.IsValidCode(req.Code) {
			return nil, apiErr{http.StatusBadRequest, errValidation,
				"短码非法(字符集不含 0/O/1/l/I,长度 1-64)", nil}
		}
		link, err := a.store.CreateLink(c.Request.Context(), t.ID, req.Code, req.TargetURLs, redirectStatus, linkType, landingSource, landingURL, domainIDs)
		if err != nil {
			if store.IsUniqueViolation(err) {
				return nil, apiErr{http.StatusConflict, errConflict, "同一域名下短码已存在", nil}
			}
			return nil, err
		}
		return link, nil
	}
	// 自动生成短码:随机生成直到无冲突(生成失败重试 10 次)
	for i := 0; i < 10; i++ {
		code := domain.GenerateCode(t.CodeLength)
		link, err := a.store.CreateLink(c.Request.Context(), t.ID, code, req.TargetURLs, redirectStatus, linkType, landingSource, landingURL, domainIDs)
		if err == nil {
			return link, nil
		}
		if store.IsUniqueViolation(err) {
			continue
		}
		return nil, err
	}
	return nil, apiErr{http.StatusInternalServerError, errInternal, "生成短码失败,请重试", nil}
}

// validateLinkDomains 校验域名归属与激活状态,返回去重后的 domainID 列表。
// 创建与编辑都要求至少保留一个关联域名;传空数组意味着短链将无法通过任何域名访问。
func (a *API) validateLinkDomains(c *gin.Context, tenantID int64, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, apiErr{http.StatusBadRequest, errValidation, "至少关联一个域名", nil}
	}
	seen := map[int64]bool{}
	var out []int64
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		d, err := a.store.GetDomainByID(c.Request.Context(), id)
		if err != nil || d.TenantID != tenantID {
			return nil, apiErr{http.StatusBadRequest, errValidation, "域名不存在或不属于当前租户", nil}
		}
		if d.Status != "active" {
			return nil, apiErr{http.StatusBadRequest, errValidation, "域名未激活,无法关联短链", nil}
		}
		out = append(out, id)
	}
	return out, nil
}

func pageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Request.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(c.Request.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (a *API) handleListLinks(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	items, total, err := a.store.ListLinksByTenant(c.Request.Context(), t.ID, page, pageSize)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Link{}
	}
	for _, l := range items {
		a.withLandingUploaded(l)
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) handleGetLink(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
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
	writeJSON(c, http.StatusOK, a.withLandingUploaded(link))
}

type patchLinkReq struct {
	TargetURLs     *[]string       `json:"targetUrls"`
	DomainIDs      *[]int64        `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
	LinkType       *string         `json:"linkType"`
	LandingSource  *string         `json:"landingSource"`
	LandingURL     *string         `json:"landingUrl"`
	Status         *string         `json:"status"`
}

func (a *API) handlePatchLink(c *gin.Context) {
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
	var req patchLinkReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	// 落地页字段组合校验(16):按当前值叠加请求值算出有效组合
	cur, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	linkType, landingSource, landingURL := cur.LinkType, cur.LandingSource, cur.LandingURL
	if req.LinkType != nil {
		linkType = *req.LinkType
	}
	if req.LandingSource != nil {
		landingSource = *req.LandingSource
	}
	if req.LandingURL != nil {
		landingURL = *req.LandingURL
	}
	linkType, landingSource, landingURL, nerr := normalizeLandingFields(linkType, landingSource, landingURL)
	if nerr != nil {
		writeErr(c, http.StatusBadRequest, errValidation, nerr.Error())
		return
	}
	upd := store.LinkUpdate{LinkType: &linkType, LandingSource: &landingSource, LandingURL: &landingURL}
	if req.TargetURLs != nil {
		if err := validTargetURLs(*req.TargetURLs); err != nil {
			writeErr(c, http.StatusBadRequest, errValidation, err.Error())
			return
		}
		upd.TargetURLs = req.TargetURLs
	}
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != "301" && *req.RedirectStatus != "302" {
			writeErr(c, http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302")
			return
		}
		v := store.RedirectStatus(*req.RedirectStatus)
		upd.RedirectStatus = &v
	}
	if req.Status != nil {
		if *req.Status != "enabled" && *req.Status != "disabled" {
			writeErr(c, http.StatusBadRequest, errValidation, "status 必须为 enabled 或 disabled")
			return
		}
		upd.Status = req.Status
	}
	if req.DomainIDs != nil {
		ids, err := a.validateLinkDomains(c, t.ID, *req.DomainIDs)
		if err != nil {
			writeAPIError(c, err)
			return
		}
		upd.DomainIDs = &ids
	}
	link, err := a.store.UpdateLink(c.Request.Context(), t.ID, id, upd)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusNotFound, errNotFound, "link not found")
			return
		}
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "同一域名下短码已存在")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 切到 redirect 型或 url 来源时删除已上传落地页文件(16)
	if link.LinkType == store.LinkTypeRedirect || link.LandingSource == store.LandingSourceURL {
		a.removeLandingFiles(id)
	}
	writeJSON(c, http.StatusOK, a.withLandingUploaded(link))
}

func (a *API) handleDeleteLink(c *gin.Context) {
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
	if err := a.store.SoftDeleteLink(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeNoContent(c)
}

func (a *API) handlePurgeLink(c *gin.Context) {
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
	if err := a.store.PurgeLink(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	a.removeLandingFiles(id) // 彻底删除连同落地页文件(16)
	writeNoContent(c)
}

// maxBatchLinkIDs 单次批量删除的短链数量上限(与前端多选交互上限一致)。
const maxBatchLinkIDs = 200

type batchLinkIDsReq struct {
	IDs []int64 `json:"ids"`
}

// parseBatchLinkIDs 解析并校验批量操作请求体:ids 非空、数量 ≤ maxBatchLinkIDs、
// 每项均为正整数;去重并保持请求顺序。非法时写 400 并返回 ok=false。
func parseBatchLinkIDs(c *gin.Context) ([]int64, bool) {
	var req batchLinkIDsReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return nil, false
	}
	if len(req.IDs) == 0 {
		writeErr(c, http.StatusBadRequest, errValidation, "ids 不能为空")
		return nil, false
	}
	if len(req.IDs) > maxBatchLinkIDs {
		writeErr(c, http.StatusBadRequest, errValidation, "单次最多处理 200 个短链")
		return nil, false
	}
	seen := make(map[int64]bool, len(req.IDs))
	ids := make([]int64, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id <= 0 {
			writeErr(c, http.StatusBadRequest, errValidation, "ids 只能包含正整数")
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, true
}

// handleBatchDeleteLinks POST /api/links/batch-delete — 批量逻辑删除(前端多选)。
// 等价于逐条 SoftDeleteLink:deleted_at 置位,记录/关联/访问明细保留;
// 跨租户/已删除/不存在的 id 静默跳过(幂等),deleted 为实际置位行数。
func (a *API) handleBatchDeleteLinks(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	ids, ok := parseBatchLinkIDs(c)
	if !ok {
		return
	}
	n, err := a.store.SoftDeleteLinks(c.Request.Context(), t.ID, ids)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{"deleted": n})
}

// handleBatchPurgeLinks POST /api/links/batch-purge — 批量物理删除(前端多选)。
// 连同 visits/link_targets/link_domains(库内 ON DELETE CASCADE)与已上传落地页文件;
// 跨租户/不存在的 id 静默跳过(幂等),deleted 为实际删除行数。
// 注意:落地页文件按短链 ID 存放在租户间共享的目录,只能清理本次真正删掉的那些 id,
// 否则跨租户混入的 id 会把他人的落地页文件一并删掉。
func (a *API) handleBatchPurgeLinks(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	ids, ok := parseBatchLinkIDs(c)
	if !ok {
		return
	}
	purged, err := a.store.PurgeLinks(c.Request.Context(), t.ID, ids)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	for _, id := range purged {
		a.removeLandingFiles(id) // 彻底删除连同落地页文件(16)
	}
	writeJSON(c, http.StatusOK, map[string]any{"deleted": len(purged)})
}

// ---------- 访问列表与统计(07) ----------

func (a *API) handleListVisits(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if _, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	// action 可选过滤(redirect / landing_view / click),省略则不过滤;
	// 非法值 400,避免拼错参数时静默返回全量明细
	action := c.Request.URL.Query().Get("action")
	if !store.ValidVisitAction(action) {
		writeErr(c, http.StatusBadRequest, errValidation, "action 必须为 redirect / landing_view / click")
		return
	}
	page, pageSize := pageParams(c)
	items, total, err := a.store.ListVisitsByLink(c.Request.Context(), id, action, page, pageSize)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Visit{}
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) handleLinkStats(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
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
	writeJSON(c, http.StatusOK, map[string]any{"visits": link.Visits, "clicks": link.Clicks})
}

// ---------- 错误封装 ----------

type apiErr struct {
	status  int
	code    string
	message string
	details any
}

func (e apiErr) Error() string { return e.message }

func writeAPIError(c *gin.Context, err error) {
	var ae apiErr
	if errors.As(err, &ae) {
		if ae.details != nil {
			writeErrDetails(c, ae.status, ae.code, ae.message, ae.details)
		} else {
			writeErr(c, ae.status, ae.code, ae.message)
		}
		return
	}
	writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
}
