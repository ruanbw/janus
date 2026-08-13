package httpapi

// 05/06/07 — 短链:创建(自动/自定义短码、关联 active 域名、配额校验)、查询、
// 更新、逻辑删除/物理删除、访问列表与统计。

import (
	"encoding/json"
	"errors"
	"net/http"
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

type createLinkReq struct {
	Code           string  `json:"code"`
	TargetURL      string  `json:"targetUrl"`
	DomainIDs      []int64 `json:"domainIds"`
	RedirectStatus *int    `json:"redirectStatus"`
}

func (a *API) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	var req createLinkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	link, err := a.createLink(r, t, req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// createLink 供后台与公开 API 共用(同一规则)。
func (a *API) createLink(r *http.Request, t *store.Tenant, req createLinkReq) (*store.Link, error) {
	if !validTargetURL(req.TargetURL) {
		return nil, apiErr{http.StatusBadRequest, errValidation, "目标 URL 非法(不能包含控制字符)", nil}
	}
	redirectStatus := 302
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != 301 && *req.RedirectStatus != 302 {
			return nil, apiErr{http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302", nil}
		}
		redirectStatus = *req.RedirectStatus
	}
	if len(req.DomainIDs) == 0 {
		return nil, apiErr{http.StatusBadRequest, errValidation, "至少关联一个域名", nil}
	}
	// 域名必须属于当前租户且 active
	domainIDs, err := a.validateLinkDomains(r, t.ID, req.DomainIDs)
	if err != nil {
		return nil, err
	}
	// 短链配额:按"尚未物理删除"计数
	usage, err := a.store.Usage(r.Context(), t.ID)
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
		link, err := a.store.CreateLink(r.Context(), t.ID, req.Code, req.TargetURL, redirectStatus, domainIDs)
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
		link, err := a.store.CreateLink(r.Context(), t.ID, code, req.TargetURL, redirectStatus, domainIDs)
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
func (a *API) validateLinkDomains(r *http.Request, tenantID int64, ids []int64) ([]int64, error) {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		d, err := a.store.GetDomainByID(r.Context(), id)
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

func pageParams(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (a *API) handleListLinks(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	page, pageSize := pageParams(r)
	items, total, err := a.store.ListLinksByTenant(r.Context(), t.ID, page, pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Link{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) handleGetLink(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	link, err := a.store.GetLinkByID(r.Context(), t.ID, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

type patchLinkReq struct {
	TargetURL      *string `json:"targetUrl"`
	DomainIDs      *[]int64 `json:"domainIds"`
	RedirectStatus *int    `json:"redirectStatus"`
	Status         *string `json:"status"`
}

func (a *API) handlePatchLink(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	var req patchLinkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	upd := store.LinkUpdate{}
	if req.TargetURL != nil {
		if !validTargetURL(*req.TargetURL) {
			writeErr(w, http.StatusBadRequest, errValidation, "目标 URL 非法(不能包含控制字符)")
			return
		}
		upd.TargetURL = req.TargetURL
	}
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != 301 && *req.RedirectStatus != 302 {
			writeErr(w, http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302")
			return
		}
		upd.RedirectStatus = req.RedirectStatus
	}
	if req.Status != nil {
		if *req.Status != "enabled" && *req.Status != "disabled" {
			writeErr(w, http.StatusBadRequest, errValidation, "status 必须为 enabled 或 disabled")
			return
		}
		upd.Status = req.Status
	}
	if req.DomainIDs != nil {
		ids, err := a.validateLinkDomains(r, t.ID, *req.DomainIDs)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		upd.DomainIDs = &ids
	}
	link, err := a.store.UpdateLink(r.Context(), t.ID, id, upd)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, errNotFound, "link not found")
			return
		}
		if store.IsUniqueViolation(err) {
			writeErr(w, http.StatusConflict, errConflict, "同一域名下短码已存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (a *API) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.SoftDeleteLink(r.Context(), t.ID, id); err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeNoContent(w)
}

func (a *API) handlePurgeLink(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.PurgeLink(r.Context(), t.ID, id); err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeNoContent(w)
}

// ---------- 访问列表与统计(07) ----------

func (a *API) handleListVisits(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if _, err := a.store.GetLinkByID(r.Context(), t.ID, id); err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	page, pageSize := pageParams(r)
	items, total, err := a.store.ListVisitsByLink(r.Context(), id, page, pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Visit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) handleLinkStats(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	link, err := a.store.GetLinkByID(r.Context(), t.ID, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"visits": link.Visits})
}

// ---------- 错误封装 ----------

type apiErr struct {
	status  int
	code    string
	message string
	details any
}

func (e apiErr) Error() string { return e.message }

func writeAPIError(w http.ResponseWriter, err error) {
	var ae apiErr
	if errors.As(err, &ae) {
		if ae.details != nil {
			writeErrDetails(w, ae.status, ae.code, ae.message, ae.details)
		} else {
			writeErr(w, ae.status, ae.code, ae.message)
		}
		return
	}
	writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
}
