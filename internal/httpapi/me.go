package httpapi

// 06 — 租户设置与配额用量:GET/PATCH /api/me。

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGetMe 返回租户信息与配额用量(后台/API 可查询)。
func (a *API) handleGetMe(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	usage, err := a.store.Usage(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	t.Usage = usage
	writeJSON(c, http.StatusOK, t)
}

const maxErrorPageHTMLBytes = 512 * 1024

type errorPagesResp struct {
	Custom404HTML string `json:"custom404Html"`
	Custom429HTML string `json:"custom429Html"`
}

type patchErrorPagesReq struct {
	Custom404HTML *string `json:"custom404Html"`
	Custom429HTML *string `json:"custom429Html"`
}

// handleGetErrorPages GET /api/me/error-pages
func (a *API) handleGetErrorPages(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	p404, p429, err := a.store.GetTenantErrorPages(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, errorPagesResp{
		Custom404HTML: p404,
		Custom429HTML: p429,
	})
}

// handlePatchErrorPages PATCH /api/me/error-pages
func (a *API) handlePatchErrorPages(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req patchErrorPagesReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	p404, p429, err := a.store.GetTenantErrorPages(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if req.Custom404HTML != nil {
		if len(*req.Custom404HTML) > maxErrorPageHTMLBytes {
			writeErr(c, http.StatusBadRequest, errValidation, "custom404Html 超过大小上限(512KB)")
			return
		}
		p404 = *req.Custom404HTML
	}
	if req.Custom429HTML != nil {
		if len(*req.Custom429HTML) > maxErrorPageHTMLBytes {
			writeErr(c, http.StatusBadRequest, errValidation, "custom429Html 超过大小上限(512KB)")
			return
		}
		p429 = *req.Custom429HTML
	}
	if err := a.store.UpdateTenantErrorPages(c.Request.Context(), t.ID, p404, p429); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 只失效错误页快照,**不动规则快照**:两者是不同的数据。
	// 原先这里调 invalidateRules,名字既不描述它做的事,也让"改错误页会
	// 失效规则快照"这个假因果看起来像是刻意设计。
	a.invalidateErrorPages(t.ID)
	writeJSON(c, http.StatusOK, errorPagesResp{
		Custom404HTML: p404,
		Custom429HTML: p429,
	})
}
