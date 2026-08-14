package httpapi

// 04 — 域名管理:添加(真实 DNS 校验)→ 激活 → 证书预签发探活;重试队列;停用/恢复;删除。

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
	"time"

	"cloak/internal/domain"
	"cloak/internal/store"
)

// validFQDN 校验域名格式:点分标签,字母/数字/连字符,标签不以连字符开头结尾。
// 允许单标签(如 localhost,开发环境 DNS 校验用)。
func validFQDN(s string) bool {
	s = strings.TrimSuffix(s, ".")
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
			if !ok || (c == '-' && (i == 0 || i == len(label)-1)) {
				return false
			}
		}
	}
	return true
}

func (a *API) handleListDomains(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	domains, err := a.store.ListDomainsByTenant(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if domains == nil {
		domains = []*store.Domain{}
	}
	writeJSON(c, http.StatusOK, domains)
}

type createDomainReq struct {
	FQDN        string `json:"fqdn"`
	Description string `json:"description"`
}

func (a *API) handleCreateDomain(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req createDomainReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	fqdn := strings.ToLower(strings.TrimSpace(req.FQDN))
	if !validFQDN(fqdn) {
		writeErr(c, http.StatusBadRequest, errValidation, "域名格式非法")
		return
	}
	desc := strings.TrimSpace(req.Description)
	if len([]rune(desc)) > domain.MaxDomainDescriptionLen {
		writeErr(c, http.StatusBadRequest, errValidation, "描述过长(最多 200 字)")
		return
	}
	if fqdn == a.cfg.PlatformDomain || fqdn == "app."+a.cfg.PlatformDomain {
		writeErr(c, http.StatusBadRequest, errValidation, "平台保留域名,不可添加")
		return
	}
	ctx := c.Request.Context()
	if exists, err := a.store.DomainFQDNExists(ctx, fqdn); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	} else if exists {
		writeErr(c, http.StatusConflict, errConflict, "域名已被占用")
		return
	}
	usage, err := a.store.Usage(ctx, t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if usage.Domains >= usage.MaxDomains {
		writeErrDetails(c, http.StatusForbidden, errDomainQuota,
			"域名数量已达上限", map[string]any{"usage": usage})
		return
	}
	d, err := a.store.CreateDomain(ctx, t.ID, fqdn, "self", desc)
	if err != nil {
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "域名已被占用")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 立即 DNS 校验(真实代码路径,读 /etc/hosts;dev 下 hosts 指向 127.0.0.1 即通过)
	okDNS, err := a.dns.Check(ctx, fqdn)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if okDNS {
		if err := a.store.SetDomainActive(ctx, d.ID); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		a.probeDomainAsync(d.ID)
	} else {
		// 未生效:进入重试队列(pending,worker 每 5 分钟重试,最长 72h)
		if err := a.store.MarkDomainDNSChecked(ctx, d.ID); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	updated, err := a.store.GetDomainByID(ctx, d.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusCreated, updated)
}

func (a *API) handleGetDomain(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	writeJSON(c, http.StatusOK, d)
}

// handleRecheckDomain 手动触发 DNS 重新校验(202)。
func (a *API) handleRecheckDomain(c *gin.Context) {
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
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	go a.recheckDomain(d.ID, d.FQDN)
	writeJSON(c, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (a *API) recheckDomain(id int64, fqdn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ok, err := a.dns.Check(ctx, fqdn)
	if err != nil {
		return
	}
	if ok {
		if err := a.store.SetDomainActive(ctx, id); err == nil {
			a.probeDomainAsync(id)
		}
		return
	}
	d, err := a.store.GetDomainByID(ctx, id)
	if err != nil {
		return
	}
	// 校验失败:保持当前状态(未激活保持 pending/failed,继续进重试队列)
	if d.Status == "failed" || d.Status == "pending" {
		_ = a.store.MarkDomainDNSChecked(ctx, id)
	}
}

type patchDomainReq struct {
	Status string `json:"status"`
}

// handlePatchDomain 停用/恢复域名。平台默认域名可停用(契约注明),不可删除。
func (a *API) handlePatchDomain(c *gin.Context) {
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
	var req patchDomainReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if req.Status != "stopped" && req.Status != "active" {
		writeErr(c, http.StatusBadRequest, errValidation, "status must be stopped or active")
		return
	}
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	// 恢复(active)前要求 DNS 仍指向本机(防域名易主后误服务);平台默认域名除外
	// (泛域名解析由部署者配置,开发环境 hosts 无子域记录)
	if req.Status == "active" && d.Status != "active" && d.Origin == "self" {
		okDNS, err := a.dns.Check(c.Request.Context(), d.FQDN)
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if !okDNS {
			writeErr(c, http.StatusConflict, errConflict, "DNS 未指向本服务器,无法恢复")
			return
		}
	}
	if err := a.store.SetDomainStatus(c.Request.Context(), id, req.Status); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if req.Status == "active" {
		// 恢复激活后确保证书探活
		a.probeDomainAsync(id)
	}
	updated, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, updated)
}

// handleDeleteDomain 删除域名:平台默认域名 400;存在未删除短链 409;否则物理删除。
func (a *API) handleDeleteDomain(c *gin.Context) {
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
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	if d.Origin == "platform" {
		writeErr(c, http.StatusBadRequest, errValidation, "平台默认域名不可删除(可停用)")
		return
	}
	n, err := a.store.CountNonDeletedLinksOnDomain(c.Request.Context(), id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if n > 0 {
		writeErrDetails(c, http.StatusConflict, errDomainInUse,
			"该域名下仍有未删除的短链,请先解除关联或删除短链",
			map[string]any{"links": n})
		return
	}
	if err := a.store.DetachDomain(c.Request.Context(), id); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(c)
}

// probeDomainAsync 后台触发证书预签发探活(HTTPS 访问触发 Caddy on-demand 签发)。
func (a *API) probeDomainAsync(id int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := a.store.GetDomainByID(ctx, id)
		if err != nil {
			return
		}
		if domain.ProbeCert(ctx, d.FQDN) {
			_ = a.store.SetDomainCertStatus(ctx, id, "issued")
		} else {
			_ = a.store.SetDomainCertStatus(ctx, id, "failed")
		}
	}()
}
