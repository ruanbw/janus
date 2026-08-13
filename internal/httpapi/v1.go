package httpapi

// 09 — 公开 REST API(Bearer API Key):/api/v1/links 的增删查;数据严格按租户隔离。

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"cloak/internal/store"
)

// apiKeyTenant 从 Authorization: Bearer <key> 解析租户;无效/已吊销 → nil。
func (a *API) apiKeyTenant(r *http.Request) (*store.Tenant, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, false
	}
	key := strings.TrimPrefix(h, "Bearer ")
	if key == "" {
		return nil, false
	}
	tenantID, err := a.store.GetTenantIDByAPIKeyHash(r.Context(), hashToken(key))
	if err != nil {
		return nil, false
	}
	t, err := a.store.GetTenantByID(r.Context(), tenantID)
	if err != nil {
		return nil, false
	}
	return t, true
}

func (a *API) requireAPIKey(w http.ResponseWriter, r *http.Request) (*store.Tenant, bool) {
	t, ok := a.apiKeyTenant(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, errUnauth, "invalid or revoked api key")
		return nil, false
	}
	return t, true
}

func (a *API) handleV1ListLinks(w http.ResponseWriter, r *http.Request) {
	t, ok := a.requireAPIKey(w, r)
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

func (a *API) handleV1CreateLink(w http.ResponseWriter, r *http.Request) {
	t, ok := a.requireAPIKey(w, r)
	if !ok {
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

func (a *API) handleV1GetLink(w http.ResponseWriter, r *http.Request) {
	t, ok := a.requireAPIKey(w, r)
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

func (a *API) handleV1DeleteLink(w http.ResponseWriter, r *http.Request) {
	t, ok := a.requireAPIKey(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.SoftDeleteLink(r.Context(), t.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, errNotFound, "link not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(w)
}
