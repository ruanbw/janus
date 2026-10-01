package httpapi

import (
	"crypto/subtle"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"janus/internal/store"
)

// CaddyAskTokenHeader Caddy 回调时携带共享密钥的请求头名。
// CaddyAskTokenQuery Caddy 回调时携带共享密钥的查询参数名。
//
// 两个都收:Caddy 2.11 的 Caddyfile **不能**给 ask 请求加自定义头
// (`ask <url> { header ... }` 直接报 unrecognized parameter '{';`permission` 的块
// 里的子指令在该版本被静默忽略),所以 Caddyfile 只能用查询参数传密钥
// (`?token={$CADDY_ASK_TOKEN}`,{$ENV} 由 Caddyfile 适配器在 adapt 时展开)。
// 头部的形式留给支持它的 Caddy 版本与手工测试 —— 两侧比对逻辑完全相同。
const CaddyAskTokenHeader = "X-Janus-Caddy-Token"
const CaddyAskTokenQuery = "token"

// 授权端点的限流:按 TCP 对端 IP 分桶(Caddy 在 compose 网络里只有一个来源)。
// 它必须排在密钥比对之前 —— 没有密钥的洪水不该打到数据库。
const (
	caddyAuthLimit  = 120
	caddyAuthWindow = time.Minute
)

// caddyAuthLimiters 给每个 API 实例一份独立的授权端点限流器。
//
// 限流桶是进程内状态,而 *API 上没有可加字段的位置(API 结构体在 server.go 里,
// 不属于本模块的改动范围),因此按接收者建表取用。生产单进程只有一个实例;
// 测试里每个 httptest.Server 一个实例 —— 包级单例反而会让互相无关的测试共享
// 同一个桶,把对方的请求限掉。
var caddyAuthLimiters sync.Map // *API -> *rateLimiter

func (a *API) caddyAuthLimiter() *rateLimiter {
	if v, ok := caddyAuthLimiters.Load(a); ok {
		return v.(*rateLimiter)
	}
	l := newRateLimiter(caddyAuthLimit, caddyAuthWindow)
	actual, _ := caddyAuthLimiters.LoadOrStore(a, l)
	return actual.(*rateLimiter)
}

// missingAskTokenLogged 保证"未配置 JANUS_CADDY_ASK_TOKEN"只告警一次,而不是每次请求刷屏。
var missingAskTokenLogged sync.Once

// handleCaddyAuthorize 是 Caddy on-demand TLS 的授权端点(见 ADR-0002、spec 决策 #8)。
// 放行条件:
//   - 请求来自本网络(第二层,不是唯一防线);
//   - 携带正确的共享密钥 JANUS_CADDY_ASK_TOKEN(第一层);
//   - 平台后台域名(裸平台域名)始终放行;
//   - 域名记录 active 且所属租户 active(未封禁/已邮箱验证)。
//
// 仅内网可达;放行 200,拒绝 403。
func (a *API) handleCaddyAuthorize(c *gin.Context) {
	if !isPrivateAddr(c.Request.RemoteAddr) {
		writeErr(c, http.StatusForbidden, errForbidden, "internal endpoint only")
		return
	}
	// IP 维度限流。用 TCP 对端而非 XFF:这个端点的调用方是本网络的 Caddy,
	// 而 XFF 是客户端可伪造的头,拿它分桶等于没有分桶。
	if !a.caddyAuthLimiter().Allow(remoteHostOnly(c.Request.RemoteAddr)) {
		writeErr(c, http.StatusTooManyRequests, errRateLimited, "too many requests")
		return
	}
	// 共享密钥:常量时间比较,失败即拒绝(fail-closed)。
	//
	// 为什么必须有它:原先只有"对端是内网"这一层。一旦运维给 backend 加了 ports: 映射,
	// 或前面再套一层反代(来源是内网 IP),判定恒真,这个无认证 GET 就公网可调 ——
	// 它能枚举出"哪些租户域名处于 active",并引导 Caddy 为它们签发证书。
	//
	// 未配置 JANUS_CADDY_ASK_TOKEN 时同样 fail-closed:发不出证书是可见的部署故障,
	// 而一个可被公网调用的枚举端点是静默的安全缺口。开发环境要在 Caddyfile 与
	// 后端进程两边配同一个值(见 docker-compose.yml 与 .env.example)。
	if !a.caddyAskTokenOK(c) {
		return
	}
	fqdn := store.NormalizeFQDN(c.Request.URL.Query().Get("domain"))
	if fqdn == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "domain query parameter required")
		return
	}
	// 注意:这里只放行裸平台域名与后台专用域名,**不做**"平台域名子域"的保留判断 ——
	// 租户默认域名 <slug>.<平台域名> 是真实注册在 domains 表里的合法域名,必须走
	// 下面对表查询(active 且租户 active)。"子域即保留"的规则只属于"租户添加
	// 自有域名"那条路径(见 domains.go):那里拒绝租户占住平台自己的子域。
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

// caddyAskTokenOK 校验共享密钥;任何不通过的情形都直接写回 403 并返回 false。
func (a *API) caddyAskTokenOK(c *gin.Context) bool {
	want := a.cfg.CaddyAskToken
	if want == "" {
		missingAskTokenLogged.Do(func() {
			log.Printf("JANUS_CADDY_ASK_TOKEN 未配置:/internal/caddy/authorize 一律拒绝(fail-closed)。" +
				"证书将无法签发 —— 请配置该变量,并把同一个值注入 Caddy 容器(Caddyfile 的 on_demand_tls.ask header)")
		})
		writeErr(c, http.StatusForbidden, errForbidden, "ask token not configured")
		return false
	}
	// 头部优先,其次查询参数(见两个常量的说明:2.11 的 Caddyfile 只能走后者)。
	got := c.GetHeader(CaddyAskTokenHeader)
	if got == "" {
		got = c.Request.URL.Query().Get(CaddyAskTokenQuery)
	}
	// 常量时间比较:比较耗时与内容无关,不用 strings.Compare / == 那样的短路写法。
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		writeErr(c, http.StatusForbidden, errForbidden, "invalid ask token")
		return false
	}
	return true
}
