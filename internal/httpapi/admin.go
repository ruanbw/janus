package httpapi

// 08 — 平台管理(超管):租户列表/详情/封禁解封/调等级;平台强删违规域名。
// 超管初始化见 cmd/cloak/main.go initSuperadmin(环境变量 CLOAK_SUPERADMIN_EMAIL)。

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"strconv"

	"cloak/internal/store"
)

func (a *API) requireSuperadmin(c *gin.Context) (*store.Tenant, *store.Session, bool) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return nil, nil, false
	}
	if !t.IsSuperAdmin {
		writeErr(c, http.StatusForbidden, errForbidden, "superadmin only")
		return nil, nil, false
	}
	return t, sess, true
}

func (a *API) handleAdminListTenants(c *gin.Context) {
	if _, _, ok := a.requireSuperadmin(c); !ok {
		return
	}
	tenants, err := a.store.ListTenants(c.Request.Context())
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if tenants == nil {
		tenants = []*store.Tenant{}
	}
	for _, t := range tenants {
		u, err := a.store.Usage(c.Request.Context(), t.ID)
		if err != nil {
			continue
		}
		t.Usage = u
	}
	writeJSON(c, http.StatusOK, tenants)
}

func (a *API) handleAdminGetTenant(c *gin.Context) {
	if _, _, ok := a.requireSuperadmin(c); !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	t, err := a.store.GetTenantByID(c.Request.Context(), id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "tenant not found")
		return
	}
	u, err := a.store.Usage(c.Request.Context(), t.ID)
	if err == nil {
		t.Usage = u
	}
	writeJSON(c, http.StatusOK, t)
}

type adminPatchTenantReq struct {
	Status *string `json:"status"`
	TierID *int64  `json:"tierId"`
}

// handleAdminPatchTenant 封禁/解封租户、调整等级。
// 契约仅允许 status=banned|active;禁止超管封禁/调整自己的等级(避免锁死)。
func (a *API) handleAdminPatchTenant(c *gin.Context) {
	admin, sess, ok := a.requireSuperadmin(c)
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
	var req adminPatchTenantReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if req.Status != nil {
		if *req.Status != "banned" && *req.Status != "active" {
			writeErr(c, http.StatusBadRequest, errValidation, "status 必须为 banned 或 active")
			return
		}
		if id == admin.ID && *req.Status == "banned" {
			writeErr(c, http.StatusBadRequest, errValidation, "不能封禁自己,请使用其他超管邮箱处理")
			return
		}
		if err := a.store.SetTenantStatus(c.Request.Context(), id, *req.Status); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	if req.TierID != nil {
		if id == admin.ID {
			writeErr(c, http.StatusBadRequest, errValidation, "不能调整自己的等级,请使用其他超管邮箱处理")
			return
		}
		if _, err := a.store.GetTier(c.Request.Context(), *req.TierID); err != nil {
			writeErr(c, http.StatusBadRequest, errValidation, "tier 不存在")
			return
		}
		if err := a.store.SetTenantTier(c.Request.Context(), id, *req.TierID); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	t, err := a.store.GetTenantByID(c.Request.Context(), id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "tenant not found")
		return
	}
	writeJSON(c, http.StatusOK, t)
}

// handleAdminDeleteDomain 平台强删违规域名(解除其短链关联,不要求短链已删除)。
func (a *API) handleAdminDeleteDomain(c *gin.Context) {
	_, sess, ok := a.requireSuperadmin(c)
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
	if _, err := a.store.GetDomainByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.store.DetachDomain(c.Request.Context(), id); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(c)
}
