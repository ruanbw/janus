package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"strings"
)

// handleCaddyAuthorize 是 Caddy on-demand TLS 的授权端点(见 ADR-0002、spec 决策 #8)。
// 放行条件:
//   - 平台后台域名(裸平台域名)始终放行;
//   - 域名记录 active 且所属租户 active(未封禁/已邮箱验证)。
//
// 仅内网可达;放行 200,拒绝 403。
func (a *API) handleCaddyAuthorize(c *gin.Context) {
	if !isPrivateAddr(c.Request.RemoteAddr) {
		writeErr(c, http.StatusForbidden, errForbidden, "internal endpoint only")
		return
	}
	fqdn := strings.ToLower(strings.TrimSpace(c.Request.URL.Query().Get("domain")))
	if fqdn == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "domain query parameter required")
		return
	}
	if fqdn == a.cfg.PlatformDomain || fqdn == "app."+a.cfg.PlatformDomain {
		c.Status(http.StatusOK)
		return
	}
	auth, err := a.store.GetDomainAuth(c.Request.Context(), fqdn)
	if err != nil {
		// 未注册域名:拒绝签发,防止任意域名解析到本机即触发签发
		writeErr(c, http.StatusForbidden, errForbidden, "domain not authorized")
		return
	}
	// 租户封禁后,其自有域名与平台默认域名都不得继续服务/签发证书(issue 08)。
	ok := auth.Status == "active" && auth.TenantStatus == "active"
	if !ok {
		writeErr(c, http.StatusForbidden, errForbidden, "domain not authorized")
		return
	}
	c.Status(http.StatusOK)
}
