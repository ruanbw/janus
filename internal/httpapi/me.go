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

type patchMeReq struct {
	CodeLength *int `json:"codeLength"`
}

// handlePatchMe 更新租户设置(自动生成短码长度)。
func (a *API) handlePatchMe(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req patchMeReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if req.CodeLength != nil {
		if *req.CodeLength < 4 || *req.CodeLength > 32 {
			writeErr(c, http.StatusBadRequest, errValidation, "codeLength 必须在 4-32 之间")
			return
		}
		if err := a.store.SetTenantCodeLength(c.Request.Context(), t.ID, *req.CodeLength); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	t, err := a.store.GetTenantByID(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
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
