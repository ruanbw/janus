package httpapi

// 09 — 公开 REST API(Bearer API Key):/api/v1/links 的增删查;数据严格按租户隔离。

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"strconv"
	"strings"

	"cloak/internal/store"
)

// apiKeyTenant 从 Authorization: Bearer <key> 解析租户;无效/已吊销 → nil。
func (a *API) apiKeyTenant(c *gin.Context) (*store.Tenant, bool) {
	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, false
	}
	key := strings.TrimPrefix(h, "Bearer ")
	if key == "" {
		return nil, false
	}
	tenantID, err := a.store.GetTenantIDByAPIKeyHash(c.Request.Context(), hashToken(key))
	if err != nil {
		return nil, false
	}
	t, err := a.store.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		return nil, false
	}
	// 与 currentTenant 一致:封禁后 API Key 立即失效(Caddy 授权端点同样拒绝)
	if t.Status == "banned" {
		return nil, false
	}
	return t, true
}

func (a *API) requireAPIKey(c *gin.Context) (*store.Tenant, bool) {
	t, ok := a.apiKeyTenant(c)
	if !ok {
		writeErr(c, http.StatusUnauthorized, errUnauth, "invalid or revoked api key")
		return nil, false
	}
	return t, true
}

func (a *API) handleV1ListLinks(c *gin.Context) {
	t, ok := a.requireAPIKey(c)
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

func (a *API) handleV1CreateLink(c *gin.Context) {
	t, ok := a.requireAPIKey(c)
	if !ok {
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

func (a *API) handleV1GetLink(c *gin.Context) {
	t, ok := a.requireAPIKey(c)
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

func (a *API) handleV1DeleteLink(c *gin.Context) {
	t, ok := a.requireAPIKey(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.SoftDeleteLink(c.Request.Context(), t.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusNotFound, errNotFound, "link not found")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(c)
}
