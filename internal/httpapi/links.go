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
	TargetURL      string          `json:"targetUrl"`
	DomainIDs      []int64         `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
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
	writeJSON(c, http.StatusCreated, link)
}

// createLink 供后台与公开 API 共用(同一规则)。
func (a *API) createLink(c *gin.Context, t *store.Tenant, req createLinkReq) (*store.Link, error) {
	if !validTargetURL(req.TargetURL) {
		return nil, apiErr{http.StatusBadRequest, errValidation, "目标 URL 非法(不能包含控制字符)", nil}
	}
	redirectStatus := store.RedirectStatus302
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != "301" && *req.RedirectStatus != "302" {
			return nil, apiErr{http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302", nil}
		}
		redirectStatus = store.RedirectStatus(*req.RedirectStatus)
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
		link, err := a.store.CreateLink(c.Request.Context(), t.ID, req.Code, req.TargetURL, redirectStatus, domainIDs)
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
		link, err := a.store.CreateLink(c.Request.Context(), t.ID, code, req.TargetURL, redirectStatus, domainIDs)
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
func (a *API) validateLinkDomains(c *gin.Context, tenantID int64, ids []int64) ([]int64, error) {
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
	writeJSON(c, http.StatusOK, link)
}

type patchLinkReq struct {
	TargetURL      *string         `json:"targetUrl"`
	DomainIDs      *[]int64        `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
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
	upd := store.LinkUpdate{}
	if req.TargetURL != nil {
		if !validTargetURL(*req.TargetURL) {
			writeErr(c, http.StatusBadRequest, errValidation, "目标 URL 非法(不能包含控制字符)")
			return
		}
		upd.TargetURL = req.TargetURL
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
	writeJSON(c, http.StatusOK, link)
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
	writeNoContent(c)
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
	page, pageSize := pageParams(c)
	items, total, err := a.store.ListVisitsByLink(c.Request.Context(), id, page, pageSize)
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
	writeJSON(c, http.StatusOK, map[string]any{"visits": link.Visits})
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
