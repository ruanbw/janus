package httpapi

// 05 — 跳转路由:GET /{code},由 Host 决定域名;命中 → 记 Visit → 302/301(Location=轮询目标);
// 16 — 落地页型:记 Visit → 固定 302 → landingUrl 或 /{code}/。
// 未命中(域名不解析/短码不存在)→ 404 且不记;命中但不可用(停用/逻辑删除/无目标/落地页文件缺失)
// → 404 并记一行 outcome=failed 的明细(可归属到该短链的失败不丢)。
//
// 规则裁决按 spec D4 插在"短链可用性检查之后、原目标选择之前":
//
//	短链不可用 → 404 + 失败明细          ← 规则不参与
//	规则求值(priority 升序,首条命中即定) → redirect 改写目标 / notfound 404 / throttle 429 / pass 继续
//	未命中任何规则 → 原跳转流程
//
// 三条热路径上的硬约束:
//  1. 零 DB 查询:规则集合来自按租户缓存的内存快照(Cache.Get 只读一次 map)。
//     多数租户没有规则,此时连访客画像都不构造。
//  2. 零写放大:命中不 UPDATE 任何表,只多写本次访问那一行明细。
//     「24h 命中」是列表接口从 visits 读时聚合出来的(spec D9)。
//  3. fail-open:快照加载失败/求值 panic 一律按"未命中"继续(spec 风险章节),
//     风控规则不该把线上短链打成 500。

import (
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/httpapi/templates"
	"cloak/internal/rules"
	"cloak/internal/store"
)

func hostOnly(h string) string {
	host, _, err := net.SplitHostPort(h)
	if err == nil {
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
		a.renderVisitorError(c, http.StatusNotFound, 0, nil)
		return
	}
	d := a.resolveDomainByHost(c)
	if d == nil {
		a.renderVisitorError(c, http.StatusNotFound, 0, nil)
		return
	}
	// 宽松命中:短链不可用时也返回它,由本函数把"为什么不可用"记进明细
	link, _, reason, err := a.store.LookupLinkForVisit(c.Request.Context(), d.ID, code)
	if err != nil {
		// 未命中(短码不存在):无法归属到任何短链,不记明细
		a.renderVisitorError(c, http.StatusNotFound, d.TenantID, nil)
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
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 其余两种"短链自身不可用"同样排在规则之前(spec D4 / ADR 0008):
	// 落地页托管文件缺失、跳转型没有可用目标——此时讨论"这次访问该不该被规则改写"没有意义,
	// 访问直接失败,明细里也不带规则字段。
	var landingDest string
	if link.LinkType == store.LinkTypeLanding {
		landingDest = link.LandingURL
		if link.LandingSource == store.LandingSourceUpload {
			// upload 来源的目标是"短码/"路径下的托管文件;文件缺失时 404 比跳到空页面好,
			// 并把失败落一行明细,避免租户以为落地页还在正常收流量
			if !a.landingUploaded(link.ID) {
				a.recordVisit(c, store.VisitRecord{
					LinkID: link.ID, DomainID: d.ID,
					Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonLandingMissing,
				})
				a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
				return
			}
			landingDest = "/" + code + "/"
		}
	} else if len(link.TargetURLs) == 0 {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 规则裁决:pass 与未命中都落到「继续原跳转流程」,区别只在明细里记不记这次命中
	dec := a.ruleDecision(c, link)
	if dec.Action != "" && a.applyRuleDecision(c, link, d, action, dec) {
		return
	}
	ruleID, ruleAction := visitRuleFields(dec)
	// 落地页型(16):记一次落地页视图 → 固定 302 到落地页
	if link.LinkType == store.LinkTypeLanding {
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: landingDest,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		c.Redirect(http.StatusFound, landingDest)
		return
	}
	// 跳转型:先选目标(无目标即失败,落一行明细)
	targetURL, err := a.store.PickTarget(c.Request.Context(), link.ID)
	if err != nil {
		// 防御性分支:上面已按 len(TargetURLs)==0 判过一次,这里兜住并发改动
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID,
			Action: action, Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonNoTarget,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, nil)
		return
	}
	// 记录访问(短链、域名、IP、UA、来源、动作、结果、目标、时间);统计失败不阻断跳转
	a.recordVisit(c, store.VisitRecord{
		LinkID: link.ID, DomainID: d.ID,
		Action: action, Outcome: store.VisitOutcomeSuccess, TargetURL: targetURL,
		RuleID: ruleID, RuleAction: ruleAction,
	})
	a.redirectToTarget(c, link, targetURL)
}

// redirectToTarget 按短链配置的 301/302 重定向到目标(默认临时 302)。
func (a *API) redirectToTarget(c *gin.Context, link *store.Link, targetURL string) {
	status := http.StatusFound // 302
	if link.RedirectStatus == store.RedirectStatus301 {
		status = http.StatusMovedPermanently
	}
	c.Redirect(status, targetURL)
}

// ruleDecision 对本次访问求值该租户的规则快照(spec D4)。
//
// 无规则(空快照)时连访客画像都不构造:多数租户没有规则,这条链路不该为它付代价。
// 快照加载失败与求值 panic 都在 rules 包内 fail-open(记日志后按未命中返回),
// 这里不需要也不能"重试一次"——重试只是把同一份故障再打一遍。
func (a *API) ruleDecision(c *gin.Context, link *store.Link) rules.Decision {
	if !link.RulesEnabled {
		return rules.Decision{}
	}
	snap := a.ruleCache.Get(c.Request.Context(), link.TenantID)
	if snap == nil || len(snap.Rules) == 0 {
		return rules.Decision{}
	}
	// 用与访问明细同一个来源 IP 构造画像,保证"明细里记的 IP"与"规则看到的 IP"一致,
	// 不会因为两处解析口径不同而出现"规则按 A 拦截、明细却记着 B"。
	//
	// 地理值也在这之前查完并挂到请求的副本上:求值期必须零 IO(ADR 0009),
	// 查库的动作只能发生在这里。取不到就填空值,不填默认国家。
	req := c.Request
	g := a.visitGeo(c)
	if g.Country != "" || g.ASN != "" {
		req = req.WithContext(rules.WithGeo(req.Context(), g.Country, g.ASN))
	}
	vCtx := rules.AcquireVisitorContext(req, g.Country, g.ASN).WithIP(clientIP(req))
	defer rules.ReleaseVisitorContext(vCtx)

	dec, matched := snap.Evaluate(vCtx, link.ID)
	if !matched {
		return rules.Decision{}
	}
	return dec
}

// visitRuleFields 把裁决折成访问明细的两个字段:命中(含 pass)时非空,未命中时为空。
// pass 也要记:契约里 ruleId/ruleAction 记的是"本次访问的规则裁决结果",
// 放行同样是一次裁决,租户要能看出"这次访问被哪条规则看过"。
func visitRuleFields(dec rules.Decision) (*int64, string) {
	if dec.Action == "" {
		return nil, ""
	}
	id := dec.RuleID
	return &id, dec.Action
}

// applyRuleDecision 按裁决改变本次访问的结果;返回 true 表示已终结本次请求。
// pass(或改写目标为空时的兜底)返回 false,由调用方走原跳转流程——但这次命中仍会记进明细。
//
// 关键不变式:命中不写任何计数表(只落一行明细)。跳转是这条链路上 QPS 最高的部分,
// 每命中一次写一次库就是写放大,而"24h 命中"由规则列表从明细读时聚合(spec D9)。
func (a *API) applyRuleDecision(c *gin.Context, link *store.Link, d *store.Domain, action string, dec rules.Decision) bool {
	ruleID, ruleAction := visitRuleFields(dec)
	switch dec.Action {
	case store.RuleActionNotfound:
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonRuleBlocked,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.renderVisitorError(c, http.StatusNotFound, link.TenantID, &dec)
		return true
	case store.RuleActionThrottle:
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeFailed, Reason: store.VisitReasonRuleThrottled,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.renderVisitorError(c, http.StatusTooManyRequests, link.TenantID, &dec)
		return true
	case store.RuleActionRedirect:
		// 规则改写的目标不参与轮询(spec 风险章节):轮询是"在目标池里选一条",
		// 规则改写是"换一条路",语义不同,拿它去 PickTarget 会把改写目标轮询没了。
		if dec.Destination == "" {
			// 理论上写入时已校验过;真出现空目标时按"未命中"放行并记日志,
			// 而不是把访问者重定向到一个空地址。
			slog.Error("规则裁决为改写目标但 destination 为空,按未命中处理",
				"rule", dec.RuleID, "link", link.ID)
			return false
		}
		a.recordVisit(c, store.VisitRecord{
			LinkID: link.ID, DomainID: d.ID, Action: action,
			Outcome: store.VisitOutcomeSuccess, TargetURL: dec.Destination,
			RuleID: ruleID, RuleAction: ruleAction,
		})
		a.redirectToTarget(c, link, dec.Destination)
		return true
	}
	return false
}

// renderVisitorError 针对访客端重定向/未命中/拦截场景渲染 HTML 错误页面 (404 / 429)。
// 决议优先级:
// 1. 规则专属自定义页面 (dec.PageMode == "custom" && dec.CustomHTML != "")
// 2. 租户全局自定义页面 (tenantID > 0 时从 store 读取)
// 3. 系统内置默认自适应 HTML 页面
func (a *API) renderVisitorError(c *gin.Context, status int, tenantID int64, dec *rules.Decision) {
	if dec != nil && dec.PageMode == "custom" && dec.CustomHTML != "" {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(status, dec.CustomHTML)
		return
	}

	if tenantID > 0 {
		p404, p429, err := a.store.GetTenantErrorPages(c.Request.Context(), tenantID)
		if err == nil {
			if status == http.StatusTooManyRequests && p429 != "" {
				c.Header("Content-Type", "text/html; charset=utf-8")
				c.String(status, p429)
				return
			}
			if status == http.StatusNotFound && p404 != "" {
				c.Header("Content-Type", "text/html; charset=utf-8")
				c.String(status, p404)
				return
			}
		}
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if status == http.StatusTooManyRequests {
		c.String(status, templates.Default429HTML())
	} else {
		c.String(status, templates.Default404HTML())
	}
}
