package httpapi

import (
	"net/http"
	"strings"
)

// handleCaddyAuthorize 是 Caddy on-demand TLS 的授权端点(见 ADR-0002、spec 决策 #8)。
// 放行条件:
//   - 平台后台域名(裸平台域名)始终放行;
//   - 域名记录 active 且:
//     - 自有域名:租户未封禁;
//     - 平台默认域名:租户已邮箱验证(active)且未封禁。
//
// 仅内网可达;放行 200,拒绝 403。
func (a *API) handleCaddyAuthorize(w http.ResponseWriter, r *http.Request) {
	if !isPrivateAddr(r.RemoteAddr) {
		writeErr(w, http.StatusForbidden, errForbidden, "internal endpoint only")
		return
	}
	fqdn := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if fqdn == "" {
		writeErr(w, http.StatusBadRequest, errValidation, "domain query parameter required")
		return
	}
	if fqdn == a.cfg.PlatformDomain || fqdn == "app."+a.cfg.PlatformDomain {
		w.WriteHeader(http.StatusOK)
		return
	}
	auth, err := a.store.GetDomainAuth(r.Context(), fqdn)
	if err != nil {
		// 未注册域名:拒绝签发,防止任意域名解析到本机即触发签发
		writeErr(w, http.StatusForbidden, errForbidden, "domain not authorized")
		return
	}
	ok := auth.Status == "active" &&
		(auth.Origin == "self" || (auth.Origin == "platform" && auth.TenantStatus == "active"))
	if !ok {
		writeErr(w, http.StatusForbidden, errForbidden, "domain not authorized")
		return
	}
	w.WriteHeader(http.StatusOK)
}
