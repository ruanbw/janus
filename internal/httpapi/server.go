// Package httpapi 实现全部 HTTP 端点(后台会话 API、Caddy 授权内部端点
// 与短链跳转路由)。Web 框架:Gin(路由/中间件/JSON 响应)。
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"janus/internal/config"
	"janus/internal/domain"
	"janus/internal/geo"
	"janus/internal/jwt"
	"janus/internal/mailer"
	"janus/internal/rbac"
	"janus/internal/rules"
	"janus/internal/store"
)

type Deps struct {
	Store  *store.Store
	Mailer mailer.Mailer
	Cfg    config.Config
	// RateLimit 认证端点限流配置;nil 时使用 DefaultRateLimit()。
	// 测试环境可注入高阈值/关闭(见 testutil)。
	RateLimit *RateLimitConfig
	// RuleCache 租户规则快照缓存;nil 时按 Store.RulesForTenant 自动构建。
	// 跳转热路径每次访问都要向它要一份快照(spec D7),它不查库。
	// 暴露在 Deps 里是为了测试能注入自定义 Loader/TTL,生产走默认构造。
	RuleCache *rules.Cache
	// GeoLookup IP → 地理值;nil 时用内嵌的离线 ip2region 库。
	// 测试注入 geo.Disabled 关掉地理解析(黑盒测试不该依赖外部数据的准确性)。
	GeoLookup geo.Lookup
	// DomainOwnership 域名归属校验;nil 时用真实 DNSChecker(查 TXT + A/AAAA)。
	// 测试注入假校验器:归属证明依赖公网 DNS,而黑盒测试既无法在权威 DNS 上
	// 发布 TXT,也不该依赖外网解析结果(同 GeoLookup 的理由)。
	DomainOwnership OwnershipChecker
}

// OwnershipChecker 抽象域名归属校验(见 Deps.DomainOwnership)。
// *domain.DNSChecker 是生产实现。
type OwnershipChecker interface {
	Verify(ctx context.Context, fqdn, token string) (domain.VerifyResult, error)
	Tasks() *domain.TaskQueue
}

type API struct {
	store        *store.Store
	mailer       mailer.Mailer
	cfg          config.Config
	dns          OwnershipChecker
	registerRate *rateLimiter // POST /api/auth/register
	authRate     *rateLimiter // login/verify-email/forgot/reset
	// setupRate 限制「首次设置密码 setup token」的补发速度(按租户邮箱分桶),
	// 防止任何知道超管邮箱的人把它当邮件炸弹:冷却窗口内最多一封。
	setupRate *rateLimiter
	// resendVerifyRate 限制 /api/auth/resend-verification 的发信速度(按邮箱分桶)。
	resendVerifyRate *rateLimiter
	// trustedProxyNets 是运维显式声明的可信代理网段(JANUS_TRUSTED_PROXY_CIDRS)。
	// 为空 = 不采信任何来源的 X-Forwarded-For(限流只认 TCP 对端)。
	trustedProxyNets []netip.Prefix
	rbacEnforcer     *rbac.Enforcer // Casbin RBAC 授权(enforcer 线程安全,authorize 中间件使用)
	jwtMgr           *jwt.Manager   // Bearer JWT 校验(authenticate 中间件使用)
	ruleCache        *rules.Cache   // 规则快照(跳转热路径求值;nil 时求值恒为"无规则")
	geo              geo.Lookup     // IP → 国家码(跳转热路径在构造 Fact 之前查,ADR 0009)
}

// New 构建 Gin 引擎:全局中间件(panic 恢复+访问日志、后台域名 SPA 分流)+ 全部路由。
// 返回 http.Handler(gin.Engine 实现),main.go 的 http.Server 无需改动。
func New(d Deps) http.Handler {
	rl := DefaultRateLimit()
	if d.RateLimit != nil {
		rl = *d.RateLimit
	}
	// RBAC enforcer:启动即构建,失败直接 panic(启动即失败优于静默降级;
	// httpapi.New 签名是 http.Handler,无法把错误传导给 main.go)。
	rb, err := rbac.New()
	if err != nil {
		panic(fmt.Sprintf("rbac init: %v", err))
	}
	// JWT 管理器:密钥优先取配置;为空时用 crypto/rand 生成 32 字节 hex
	// (每次启动随机,重启后已签发 token 失效;生产必须配置 JANUS_JWT_SECRET)。
	secret := d.Cfg.JWTSecret
	if secret == "" {
		secret = randomSecret()
		log.Printf("JANUS_JWT_SECRET 未配置,已使用临时随机密钥,重启后已签发 token 失效(生产必须配置)")
	}
	jwtMgr := jwt.NewManager(secret, d.Cfg.JWTTTL)

	// 规则快照缓存:整租户的启用规则一次性读进内存并预编译,跳转热路径只做纯内存求值。
	// 加载失败按"该租户没有规则"放行(spec 风险章节,fail-open)。
	ruleCache := d.RuleCache
	if ruleCache == nil {
		ruleCache = rules.NewCache(d.Store.RulesForTenant)
	}

	// 地理值:默认用内嵌的离线 ip2region 库(无网络、无 API Key、无挂载卷)。
	// 加载失败回落成 geo.Disabled(恒空)而不是启动失败:国家查不到只会让
	// country 条件恒不命中,不该因为一份附属数据而让整站起不来。
	geoLookup := d.GeoLookup
	if geoLookup == nil {
		g, err := geo.NewXDB()
		if err != nil {
			log.Printf("地理数据源不可用,国家相关规则将恒不命中: %v", err)
			geoLookup = geo.Disabled
		} else {
			geoLookup = geo.Cached(g, geo.DefaultCacheEntries)
		}
	}

	// 可信代理网段:配置写错时 fail-closed(等价于"不采信任何 XFF"),
	// 而不是 fail-open。正常路径上 cfg.Validate() 已经在启动前拒绝过非法值,
	// 这里只是防止 New() 被测试/嵌入方直接调用时静默降级成"全都信"。
	trustedProxyNets, err := d.Cfg.TrustedProxyNets()
	if err != nil {
		log.Printf("JANUS_TRUSTED_PROXY_CIDRS 解析失败(%v),已按「不采信任何 X-Forwarded-For」处理", err)
		trustedProxyNets = nil
	}

	a := &API{
		store:        d.Store,
		mailer:       d.Mailer,
		cfg:          d.Cfg,
		dns:          d.DomainOwnership,
		registerRate: newRateLimiter(rl.RegisterLimit, rl.RegisterWindow),
		authRate:     newRateLimiter(rl.AuthLimit, rl.AuthWindow),
		// 每 15 分钟最多一封:够运维在丢失邮件后自助补发,又挡得住邮件轰炸。
		setupRate:        newRateLimiter(1, 15*time.Minute),
		resendVerifyRate: newRateLimiter(1, 5*time.Minute),
		trustedProxyNets: trustedProxyNets,
		rbacEnforcer:     rb,
		jwtMgr:           jwtMgr,
		ruleCache:        ruleCache,
		geo:              geoLookup,
	}
	if a.dns == nil {
		a.dns = &domain.DNSChecker{ExpectedIP: d.Cfg.ServerPublicIP}
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 16:落地页上传托管用 /{code}/ 尾斜杠布局,关闭 gin 自动尾斜杠重定向
	r.RedirectTrailingSlash = false
	r.Use(a.panicRecoverAndLog())

	// 健康检查(docker compose healthcheck)
	r.GET("/healthz", func(c *gin.Context) {
		writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Caddy on-demand TLS 授权端点(仅内网可达)
	r.GET("/internal/caddy/authorize", a.handleCaddyAuthorize)

	// ---------- 公开路由(无需认证) ----------
	// 认证:注册/邮箱验证/登录/API Bearer JWT 签发/忘记密码/重置密码
	r.POST("/api/auth/register", a.handleRegister)
	r.POST("/api/auth/verify-email", a.handleVerifyEmail)
	r.POST("/api/auth/resend-verification", a.handleResendVerification) // 注册发信失败后的自助恢复入口
	r.POST("/api/auth/login", a.handleLogin)
	r.POST("/api/auth/token", a.handleToken) // API Bearer JWT 签发(公开,走 authRate 限流)
	r.POST("/api/auth/forgot-password", a.handleForgotPassword)
	r.POST("/api/auth/reset-password", a.handleResetPassword)

	// ---------- 受保护路由:先认证(会话 cookie 或 Bearer JWT)后授权(Casbin RBAC) ----------
	// 中间件顺序:authenticate 产出 context(租户/角色/认证方式),authorize 据此判权。
	prot := r.Group("/api", a.authenticate(), a.authorize())
	// 02:登出 / me
	prot.POST("/auth/logout", a.handleLogout)
	prot.GET("/auth/me", a.handleMe)
	// 03:修改密码
	prot.POST("/auth/change-password", a.handleChangePassword)
	// 04:域名
	prot.GET("/domains", a.handleListDomains)
	prot.POST("/domains", a.handleCreateDomain)
	prot.GET("/domains/:id", a.handleGetDomain)
	prot.POST("/domains/:id/recheck", a.handleRecheckDomain)
	prot.PATCH("/domains/:id", a.handlePatchDomain)
	prot.DELETE("/domains/:id", a.handleDeleteDomain)
	// 05/07:短链
	prot.GET("/links", a.handleListLinks)
	prot.POST("/links", a.handleCreateLink)
	prot.GET("/links/:id", a.handleGetLink)
	prot.PATCH("/links/:id", a.handlePatchLink)
	prot.DELETE("/links/:id", a.handleDeleteLink)
	prot.POST("/links/:id/purge", a.handlePurgeLink)
	prot.POST("/links/batch-delete", a.handleBatchDeleteLinks)
	prot.POST("/links/batch-purge", a.handleBatchPurgeLinks)
	prot.GET("/links/:id/visits", a.handleListVisits)
	prot.GET("/links/:id/stats", a.handleLinkStats)
	// 总览统计:按租户全量 GROUP BY 聚合各维度分布。
	// 不走"前端拉最近 N 条明细自己数"的路径 —— 那既会漏(超过 N 条就不准),
	// 又会让点击行与失败行混进访问量(违反 CONTEXT.md 的计数口径)。
	prot.GET("/visits/overview", a.handleVisitsOverview)
	// 16:落地页上传(zip 替换式)
	prot.POST("/links/:id/landing", a.handleUploadLanding)
	// 规则与「规则 ↔ 短链」关联(关联只存在规则一侧,spec D1;
	// 短链表单的勾选与规则编辑器的多选写的是同一份 rule_links)
	prot.GET("/rules", a.handleListRules)
	prot.POST("/rules", a.handleCreateRule)
	prot.GET("/rules/options", a.handleRuleOptions)
	prot.POST("/rules/simulate", a.handleSimulateRules)
	prot.POST("/rules/validate-expr", a.handleValidateExpr)
	prot.GET("/rules/:id", a.handleGetRule)
	prot.PATCH("/rules/:id", a.handlePatchRule)
	prot.DELETE("/rules/:id", a.handleDeleteRule)
	prot.GET("/links/:id/rules", a.handleListLinkRules)
	prot.PUT("/links/:id/rules", a.handlePutLinkRules)
	// 06:租户设置
	prot.GET("/me", a.handleGetMe)
	prot.GET("/me/error-pages", a.handleGetErrorPages)
	prot.PATCH("/me/error-pages", a.handlePatchErrorPages)
	// 前端启动配置:服务器 IP/平台域名/当前租户配额,按租户返回
	prot.GET("/config", a.handleGetConfig)
	// 08:平台管理(超管;tenant 角色由 authorize 直接 403,requireSuperadmin 保留作纵深防御)
	prot.GET("/admin/tenants", a.handleAdminListTenants)
	prot.GET("/admin/tiers", a.handleAdminListTiers)
	prot.GET("/admin/tenants/:id", a.handleAdminGetTenant)
	prot.PATCH("/admin/tenants/:id", a.handleAdminPatchTenant)
	prot.DELETE("/admin/domains/:id", a.handleAdminDeleteDomain)

	// 跳转(公开):路径首段为短码,由 Host 决定域名(在受保护组外注册)
	// 公开跳转路由挂访客限流:落地页型短链每次访问都要写一行访问明细,
	// 点击回传还要额外做一次地理解析,无节制的脚本刷量会同时撑大 visits 表
	// 与吃掉 CPU。阈值按"CGNAT/公司出口下正常用户感知不到"来定。
	r.GET("/:code", a.visitorGuard(), a.handleRedirect)
	// 16:落地页型短链二级路径(点击端点/每短链 SDK/上传落地页静态服务)。
	// gin 路由树不支持 /:code 与 /:code/... 子路由并存,统一由 NoRoute 兜底分发;
	// 关闭尾斜杠重定向,避免 gin 把 /{code}/ 重定向回 /{code} 造成环。
	r.NoRoute(a.handleLandingFallback)

	return r
}

// panicRecoverAndLog 全局中间件:panic 恢复为 JSON 500 + 访问日志(与原中间件行为一致)。
func (a *API) panicRecoverAndLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v\n%s", rec, debug.Stack())
				writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			}
			log.Printf("%s %s -> %d (%s)", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
		}()
		c.Next()
	}
}

// isPrivateAddr 判定来源地址是否为内网/回环地址(授权端点"仅内网可达")。
func isPrivateAddr(remote string) bool {
	ip := net.ParseIP(remoteHostOnly(remote))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// randomSecret 生成 32 字节随机 hex 密钥(JANUS_JWT_SECRET 未配置时的回退,
// 用法参考 session.go newToken 的 crypto/rand 模式)。
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
