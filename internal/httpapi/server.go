// Package httpapi 实现全部 HTTP 端点(后台会话 API、Caddy 授权内部端点
// 与短链跳转路由)。Web 框架:Gin(路由/中间件/JSON 响应)。
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ruanbw/janus/internal/config"
	"github.com/ruanbw/janus/internal/domain"
	"github.com/ruanbw/janus/internal/geo"
	"github.com/ruanbw/janus/internal/jwt"
	"github.com/ruanbw/janus/internal/linkcache"
	"github.com/ruanbw/janus/internal/metrics"
	"github.com/ruanbw/janus/internal/rbac"
	"github.com/ruanbw/janus/internal/rules"
	"github.com/ruanbw/janus/internal/store"
	"github.com/ruanbw/janus/internal/visitqueue"
)

type Deps struct {
	Store *store.Store
	Cfg   config.Config
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
	// ProtectedRoutes 外部注入的受保护控制面路由注入回调
	ProtectedRoutes []func(rg *gin.RouterGroup)
	// Ready 是 /readyz 的探活函数。nil 时用 (*store.Store).Ping。
	//
	// 暴露成函数而不是只依赖 Store:探活接口的正确性不能靠"一个真的能连上的
	// Postgres"才能被验证 —— 那样它和它要检查的东西一起坏掉。测试注入假实现,
	// 生产走默认值。
	Ready func(ctx context.Context) error
	// Metrics 指标注册表。/metrics 导出它;nil 时新建一份空的。
	// 调用方可以持有同一份注册表自己注册指标(如扩展的路由计数)。
	Metrics *metrics.Registry
	// MetricsToken 是 /metrics 的共享 token。为空时该端点一律 403(fail-closed):
	// 指标里有租户数、域名状态与规则 fail-open 次数,足以反推业务规模与故障。
	MetricsToken string
	// VisitWriter 是访问明细的异步批量写入器;nil 表示同步逐行写入(默认)。
	//
	// 生命周期归调用方:关停时必须先停 HTTP 再 Close 它,否则停机瞬间还在跳转的
	// 请求会把明细丢在队列里。基座不给它加"自动关闭"的钩子,是因为 httpapi.New
	// 的返回值是 http.Handler,没有可挂的关闭语义。
	VisitWriter *visitqueue.Writer
}

// OwnershipChecker 抽象域名归属校验(见 Deps.DomainOwnership)。
// *domain.DNSChecker 是生产实现。
type OwnershipChecker interface {
	Verify(ctx context.Context, fqdn, token string) (domain.VerifyResult, error)
	Tasks() *domain.TaskQueue
}

type API struct {
	store        *store.Store
	cfg          config.Config
	dns          OwnershipChecker
	registerRate *rateLimiter // POST /api/auth/register
	authRate     *rateLimiter // login/token
	// loginFailures 是账号级失败退避(登录 / token / 改密的旧密码猜测)。
	// 按归一化用户名累计失败并指数退避,与上面的每 IP 限流正交 —— 每 IP 限流挡不住
	// "换 IP 打同一个账号"的撞库(见 auth_limit.go)。
	loginFailures *loginFailureLimiter
	// trustedProxyNets 是运维显式声明的可信代理网段(JANUS_TRUSTED_PROXY_CIDRS)。
	// 为空 = 不采信任何来源的 X-Forwarded-For(限流只认 TCP 对端)。
	trustedProxyNets []netip.Prefix
	rbacEnforcer     *rbac.Enforcer              // Casbin RBAC 授权(enforcer 线程安全,authorize 中间件使用)
	jwtMgr           *jwt.Manager                // Bearer JWT 校验(authenticate 中间件使用)
	ruleCache        *rules.Cache                // 规则快照(跳转热路径求值;nil 时求值恒为"无规则")
	links            *linkcache.Cache            // 短链快照(跳转热路径定位短链;nil 时未装配)
	geo              geo.Lookup                  // IP → 国家码(跳转热路径在构造 Fact 之前查,ADR 0009)
	metrics          *metrics.Registry           // 指标注册表(/metrics 导出)
	ready            func(context.Context) error // /readyz 探活(nil 时不探)
	metricsToken     string                      // /metrics 共享 token(空 = fail-closed)
	visits           *visitqueue.Writer          // 访问明细异步写入;nil = 同步
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

	// 短链快照缓存:跳转热路径不再每次回源。
	//
	// 缓存键是 (domainID, code):同一租户的多个域名下允许同名短码。
	// 加载失败与未命中都不缓存(短码刚创建必须立刻可用);失效由管理端写路径
	// 显式 Invalidate(见 invalidateLinksFor*),TTL 只是漏调时的兜底。
	linkStore := d.Store
	linkCache := linkcache.New(func(ctx context.Context, domainID int64, code string) (*linkcache.Record, error) {
		if linkStore == nil {
			// 嵌入方可能只挂部分路由;/:code 此时本就不可用,返回未命中而不是 panic。
			return nil, linkcache.ErrNotFound
		}
		l, _, _, err := linkStore.LookupLinkForVisit(ctx, domainID, code)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, linkcache.ErrNotFound
			}
			return nil, err
		}
		status, err := strconv.Atoi(string(l.RedirectStatus))
		if err != nil {
			return nil, fmt.Errorf("短链 %d 的 redirect_status 非法: %w", l.ID, err)
		}
		return &linkcache.Record{
			LinkID:         l.ID,
			TenantID:       l.TenantID,
			DomainID:       domainID,
			Status:         l.Status,
			Deleted:        l.DeletedAt != nil,
			LinkType:       l.LinkType,
			LandingSource:  l.LandingSource,
			LandingURL:     l.LandingURL,
			RedirectStatus: status,
			RulesEnabled:   l.RulesEnabled,
			Targets:        l.TargetURLs,
		}, nil
	})

	// 地理/情报值:基座直接使用内嵌的离线 ip2region 库(带缓存,无网络、无 API Key、无挂载卷)。
	geoLookup := d.GeoLookup
	if geoLookup == nil {
		g, err := geo.NewXDB()
		if err != nil {
			log.Printf("地理离线库不可用: %v", err)
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
	if len(trustedProxyNets) == 0 && rl.AuthLimit > 0 {
		// 启动即提示,而不是等运维来问「为什么全站用户一起被限流」。
		//
		// 可信代理网段为空时,trustedClientIP 只认 TCP 对端。若前面有 Caddy/NGINX
		// 这类反代(默认部署就是这样),所有请求的对端都是**同一个**代理 IP,
		// 于是「每 IP 20 次/分」实际变成「全站共享 20 次/分」:一个人连续试密码,
		// 所有人一起吃 429。
		//
		// 这里只告警不改行为:配错网段的后果是限流被彻底绕过(注册/登录/找回密码
		// 全无防护),比限流偏严危险得多,所以默认 fail-closed 是对的取舍。修法是
		// 运维显式声明 JANUS_TRUSTED_PROXY_CIDRS。
		log.Printf("WARN: JANUS_TRUSTED_PROXY_CIDRS 未配置,限流只按 TCP 对端计数。若 Janus 前面有反向代理(如默认部署的 Caddy),所有请求将共用代理这一个来源 IP,认证类限流会退化成「全站共享 %d 次/%v」,请配置该环境变量后重启", rl.AuthLimit, rl.AuthWindow)
	}

	// 指标注册表:共享同一份,调用方(扩展/自定义 handler)可以自己往里注册。
	metricsRegistry := d.Metrics
	if metricsRegistry == nil {
		metricsRegistry = metrics.New()
	}

	// 就绪探针:默认探 DB。Store 为 nil 时返回错误而不是 panic —— 嵌入方
	// 可能只挂一部分路由,此时 /readyz 应该是 503 而不是把进程打挂。
	readinessProbe := d.Ready
	if readinessProbe == nil {
		st := d.Store
		readinessProbe = func(ctx context.Context) error {
			if st == nil {
				return fmt.Errorf("store 未装配")
			}
			return st.Ping(ctx)
		}
	}

	a := &API{
		store:            d.Store,
		cfg:              d.Cfg,
		dns:              d.DomainOwnership,
		registerRate:     newRateLimiter(rl.RegisterLimit, rl.RegisterWindow),
		authRate:         newRateLimiter(rl.AuthLimit, rl.AuthWindow),
		loginFailures:    newLoginFailureLimiter(),
		trustedProxyNets: trustedProxyNets,
		rbacEnforcer:     rb,
		jwtMgr:           jwtMgr,
		ruleCache:        ruleCache,
		links:            linkCache,
		geo:              geoLookup,
		metrics:          metricsRegistry,
		ready:            readinessProbe,
		metricsToken:     d.MetricsToken,
		visits:           d.VisitWriter,
	}
	if a.dns == nil {
		a.dns = &domain.DNSChecker{ExpectedIP: d.Cfg.ServerPublicIP}
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 16:落地页上传托管用 /{code}/ 尾斜杠布局,关闭 gin 自动尾斜杠重定向
	r.RedirectTrailingSlash = false
	r.Use(a.panicRecoverAndLog())
	r.Use(a.bodyLimit())

	// 健康检查:
	//   /healthz 保持原语义(恒定 200)以兼容既有 compose healthcheck;
	//   /livez   恒 200,只回答"进程活着";
	//   /readyz  探 DB,失败 503 —— 编排器据此摘流而不重启进程;
	//   /metrics 仅内网 + token(见 health.go)。
	r.GET("/healthz", func(c *gin.Context) {
		writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.GET("/livez", a.handleLivez)
	r.GET("/readyz", a.handleReadyz)
	r.GET("/metrics", a.handleMetrics)

	// Caddy on-demand TLS 授权端点(仅内网可达)
	r.GET("/internal/caddy/authorize", a.handleCaddyAuthorize)

	// ---------- 公开路由(无需认证) ----------
	// 认证:注册/登录/API Bearer JWT 签发
	r.POST("/api/auth/register", a.handleRegister)
	r.POST("/api/auth/login", a.handleLogin)
	r.POST("/api/auth/token", a.handleToken) // API Bearer JWT 签发(公开,走 authRate 限流)

	// ---------- 受保护路由:先认证(会话 cookie 或 Bearer JWT)后授权(Casbin RBAC) ----------
	// 中间件顺序:authenticate 产出 context(租户/角色/认证方式)→ requirePasswordChange 拦截
	// 被强制改密的租户(白名单见 password_change.go)→ authorize 据此判权。
	//
	// 闸门挂在 protAuth 上(而不只是业务组),所以外部注入的受保护路由同样受它约束。
	protAuth := r.Group("/api", a.authenticate(), a.requirePasswordChange())
	prot := protAuth.Group("", a.authorize())
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
	// 又会让点击行与失败行混进访问量(违反统计计数口径)。
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

	// 挂载外部受保护扩展路由。
	//
	// 契约:传入的是 protAuth(authenticate + 强制改密闸门,**没有** Casbin authorize)。
	// 基座只做认证与闸门,授权由扩展自行完成 —— 需要授权就用 Deps 里的 Enforcer()/Casbin
	// 规则自行校验。此前 authz 中间件里有一个 is_protected_extension 放行分支,已删除:
	// 它没有任何写入点(永不触发),却是一个"谁设置谁绕过"的后门开关。不要依赖它,
	// 也不要以任何形式把它加回来。
	for _, inject := range d.ProtectedRoutes {
		if inject != nil {
			inject(protAuth)
		}
	}

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

// maxJSONBodyBytes 未单独设限的 JSON 端点的统一请求体上限(1.5MiB)。
//
// 为什么必须限:json.Decoder 在任何认证/校验之前就把整份 body 读进内存,
// 没有上限时一个匿名请求(如 POST /api/auth/login)就能用它把内存打满。
//
// 取值口径与 maxRuleBodyBytes / maxCreateLinkBodyBytes 一致 ——「合法最大 payload + 余量」:
//   - 合法最大 payload 出现在 PATCH /api/me/error-pages:两个错误页各自允许
//     maxErrorPageHTMLBytes(512KB),合起来已接近 1MB,再加 JSON 转义与字段名开销
//     会略超 1MB —— 直接用 1MB 会把这份完全合法的请求一起打死。
//   - 再往上(2MB 量级)余量就薄到只剩贴边,失去了「拦住整份 body 进内存」的意义。
//
// 自带更紧上限的端点不受影响:links / rules 的 handler 会在这一层之外再包一次
// 更小的 MaxBytesReader,更小的那个先触发(见 handleCreateLink、decodeRuleBody)。
const maxJSONBodyBytes = 3 << 19 // 1.5MiB

// bodyLimit 全局中间件:给所有可能带请求体的方法统一加体积上限。
//
// 逐个 handler 手写会漏 —— 全仓 16 处 json.Decode 只有 3 处设了限,漏掉的端点
// 在解码前对体积没有任何拦截。这里按「方法 + 路由」一次性兜底。
func (a *API) bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.bodyLimitBytes(c.FullPath()))
		}
		c.Next()
	}
}

// bodyLimitBytes 本次请求的 body 上限:默认统一值,multipart 上传端点用配置里的 zip 上限。
//
// 落地页上传的合法上限由 LandingMaxZipBytes 决定(默认 10MB),远大于任何 JSON 端点;
// 套用默认值会先于 handler 把它拦掉,上传功能整体失效。
func (a *API) bodyLimitBytes(fullPath string) int64 {
	// 与 New() 里 prot.POST("/links/:id/landing", ...) 注册的路由同一路径。
	if fullPath == "/api/links/:id/landing" {
		return a.cfg.LandingMaxZipBytes + 64<<10
	}
	return maxJSONBodyBytes
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

// ginPathToCasbinPath 将 Gin 路由路径中的动态参数(:param 或 *param)转换为 Casbin keyMatch3 支持的通配符 *
func ginPathToCasbinPath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		} else if strings.HasPrefix(part, "*") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
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
