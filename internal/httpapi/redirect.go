package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 302/301(Location=目标);
// 未命中/停用/逻辑删除/域名停用 → 404;每次成功跳转记录一条 Visit(07)。

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"strings"

	"cloak/internal/store"
)

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(h)
}

func (a *API) handleRedirect(c *gin.Context) {
	code := c.Param("code")
	if code == "" || strings.Contains(code, "/") {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	host := hostOnly(c.Request.Host)
	if host == "" {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	d, err := a.store.GetDomainByFQDN(c.Request.Context(), host)
	if err != nil || d.Status != "active" {
		// 域名未激活/停用/不存在:未命中
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	link, _, err := a.store.ResolveRedirect(c.Request.Context(), d.ID, code)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 记录访问(短链、域名、IP、UA、来源、时间)
	if err := a.store.InsertVisit(c.Request.Context(), link.ID, d.ID, clientIP(c.Request), c.Request.UserAgent(), c.Request.Referer()); err != nil {
		// 统计失败不阻断跳转
		_ = err
	}
	status := http.StatusFound // 302
	if link.RedirectStatus == store.RedirectStatus301 {
		status = http.StatusMovedPermanently
	}
	c.Redirect(status, link.TargetURL)
}
