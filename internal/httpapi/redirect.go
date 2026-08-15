package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 记 Visit → 302/301(Location=轮询目标);
// 16 — 落地页型:记 Visit → 固定 302 → landingUrl 或 /{code}/。
// 未命中/停用/逻辑删除/域名停用 → 404。

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/store"
)

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(h)
}

// resolveDomainByHost 按 Host 定位 active 域名;不存在/未激活/停用返回 nil。
func (a *API) resolveDomainByHost(c *gin.Context) *store.Domain {
	host := hostOnly(c.Request.Host)
	if host == "" {
		return nil
	}
	d, err := a.store.GetDomainByFQDN(c.Request.Context(), host)
	if err != nil || d.Status != "active" {
		return nil
	}
	return d
}

func (a *API) handleRedirect(c *gin.Context) {
	code := c.Param("code")
	if code == "" || strings.Contains(code, "/") {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	d := a.resolveDomainByHost(c)
	if d == nil {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	link, _, err := a.store.ResolveLink(c.Request.Context(), d.ID, code)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 落地页型(16):记一次 Visit(落地页视图)→ 固定 302 到落地页
	if link.LinkType == store.LinkTypeLanding {
		// 统计失败不阻断跳转
		_ = a.store.InsertVisit(c.Request.Context(), link.ID, d.ID, clientIP(c.Request), c.Request.UserAgent(), c.Request.Referer())
		dest := link.LandingURL
		if link.LandingSource == store.LandingSourceUpload {
			dest = "/" + code + "/"
		}
		c.Redirect(http.StatusFound, dest)
		return
	}
	// 跳转型:先选目标(无目标视为未命中,不记 Visit,维持原语义)
	targetURL, err := a.store.PickTarget(c.Request.Context(), link.ID)
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
	c.Redirect(status, targetURL)
}
