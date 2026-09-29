package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 记 Visit → 302/301(Location=轮询目标);
// 16 — 落地页型:记 Visit → 固定 302 → landingUrl 或 /{code}/。
// 未命中(域名不解析/短码不存在)→ 404 且不记;命中但不可用(停用/逻辑删除/无目标/落地页文件缺失)
// → 404 并记一行 outcome=failed 的明细(可归属到该短链的失败不丢)。

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
	// 宽松命中:短链不可用时也返回它,由本函数把"为什么不可用"记进明细
	link, _, reason, err := a.store.LookupLinkForVisit(c.Request.Context(), d.ID, code)
	if err != nil {
		// 未命中(短码不存在):无法归属到任何短链,不记明细
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 动作按短链类型确定:跳转型记 redirect,落地页型记 landing_view
	action := store.VisitActionRedirect
	if link.LinkType == store.LinkTypeLanding {
		action = store.VisitActionLandingView
	}
	if reason != "" {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: reason,
		})
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 落地页型(16):记一次落地页视图 → 固定 302 到落地页
	if link.LinkType == store.LinkTypeLanding {
		dest := link.LandingURL
		if link.LandingSource == store.LandingSourceUpload {
			// upload 来源的目标是"短码/"路径下的托管文件;文件缺失时 404 比跳到空页面好,
			// 并把失败落一行明细,避免租户以为落地页还在正常收流量
			if !a.landingUploaded(link.ID) {
				a.recordVisit(c, store.VisitRecord{
					LinkID: link.ID, DomainID: d.ID,
					Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonLandingMissing,
				})
				writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
				return
			}
			dest = "/" + code + "/"
		}
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: dest,
		})
		c.Redirect(http.StatusFound, dest)
		return
	}
	// 跳转型:先选目标(无目标即失败,落一行明细)
	targetURL, err := a.store.PickTarget(c.Request.Context(), link.ID)
	if err != nil {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		writeErr(c, http.StatusNotFound, errNotFound, "short link not found")
		return
	}
	// 记录访问(短链、域名、IP、UA、来源、动作、结果、目标、时间);统计失败不阻断跳转
	a.recordVisit(c, store.VisitRecord{
		LinkID: link.ID, DomainID: d.ID,
		Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: targetURL,
	})
	status := http.StatusFound // 302
	if link.RedirectStatus == store.RedirectStatus301 {
		status = http.StatusMovedPermanently
	}
	c.Redirect(status, targetURL)
}
