package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 302/301(Location=目标);
// 未命中/停用/逻辑删除/域名停用 → 404;每次成功跳转记录一条 Visit(07)。

import (
	"net"
	"net/http"
	"strings"

	"cloak/internal/store"
)

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(h)
}

func (a *API) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" || strings.Contains(code, "/") {
		writeErr(w, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	host := hostOnly(r.Host)
	if host == "" {
		writeErr(w, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	d, err := a.store.GetDomainByFQDN(r.Context(), host)
	if err != nil || d.Status != "active" {
		// 域名未激活/停用/不存在:未命中
		writeErr(w, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	link, _, err := a.store.ResolveRedirect(r.Context(), d.ID, code)
	if err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 记录访问(短链、域名、UA、来源、时间)
	if err := a.store.InsertVisit(r.Context(), link.ID, d.ID, r.UserAgent(), r.Referer()); err != nil {
		// 统计失败不阻断跳转
		_ = err
	}
	status := http.StatusFound // 302
	if link.RedirectStatus == store.RedirectStatus301 {
		status = http.StatusMovedPermanently
	}
	http.Redirect(w, r, link.TargetURL, status)
}
