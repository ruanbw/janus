package httpapi

// 06 — 租户设置与配额用量:GET/PATCH /api/me。

import (
	"encoding/json"
	"net/http"
)

// handleGetMe 返回租户信息与配额用量(后台/API 可查询)。
func (a *API) handleGetMe(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	usage, err := a.store.Usage(r.Context(), t.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	t.Usage = usage
	writeJSON(w, http.StatusOK, t)
}

type patchMeReq struct {
	CodeLength *int `json:"codeLength"`
}

// handlePatchMe 更新租户设置(自动生成短码长度)。
func (a *API) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	var req patchMeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if req.CodeLength != nil {
		if *req.CodeLength < 4 || *req.CodeLength > 32 {
			writeErr(w, http.StatusBadRequest, errValidation, "codeLength 必须在 4-32 之间")
			return
		}
		if err := a.store.SetTenantCodeLength(r.Context(), t.ID, *req.CodeLength); err != nil {
			writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	t, err := a.store.GetTenantByID(r.Context(), t.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	usage, err := a.store.Usage(r.Context(), t.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	t.Usage = usage
	writeJSON(w, http.StatusOK, t)
}
