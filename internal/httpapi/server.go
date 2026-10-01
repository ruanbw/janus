// Package httpapi 实现全部 HTTP 端点(后台会话 API、Caddy 授权内部端点
// 与短链跳转路由)。Web 框架:Gin(路由/中间件/JSON 响应)。
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"cloak/internal/config"
	"cloak/internal/domain"
	"cloak/internal/geo"
	"cloak/internal/jwt"
	"cloak/internal/mailer"
	"cloak/internal/rbac"
	"cloak/internal/rules"
	"cloak/internal/store"
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
}

type API struct {
	store        *store.Store
	mailer       mailer.Mailer
	cfg          config.Config
	dns          *domain.DNSChecker
	registerRate *rateLimiter   // POST /api/auth/register
	authRate     *rateLimiter   // login/verify-email/forgot/reset
	rbacEnforcer *rbac.Enforcer // Casbin RBAC 授权(enforcer 线程安全,authorize 中间件使用)
	jwtMgr       *jwt.Manager   // Bearer JWT 校验(authenticate 中间件使用)
	ruleCache    *rules.Cache   // 规则快照(跳转热路径求值;nil 时求值恒为"无规则")
	geo          geo.Lookup     // IP → 国家码(跳转热路径在构造 Fact 之前查,ADR 0009)
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
	// (每次启动随机,重启后已签发 token 失效;生产必须配置 CLOAK_JWT_SECRET)。
	secret := d.Cfg.JWTSecret
	if secret == "" {
		secret = randomSecret()
		log.Printf("CLOAK_JWT_SECRET 未配置,已使用临时随机密钥,重启后已签发 token 失效(生产必须配置)")
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

	a := &API{
		store:        d.Store,
		mailer:       d.Mailer,
		cfg:          d.Cfg,
		dns:          &domain.DNSChecker{ExpectedIP: d.Cfg.ServerPublicIP},
		registerRate: newRateLimiter(rl.RegisterLimit, rl.RegisterWindow),
		authRate:     newRateLimiter(rl.AuthLimit, rl.AuthWindow),
		rbacEnforcer: rb,
		jwtMgr:       jwtMgr,
		ruleCache:    ruleCache,
		geo:          geoLookup,
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
	r.GET("/:code", a.handleRedirect)
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

// randomSecret 生成 32 字节随机 hex 密钥(CLOAK_JWT_SECRET 未配置时的回退,
// 用法参考 session.go newToken 的 crypto/rand 模式)。
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
