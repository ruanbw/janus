package httpapi

// 安全与防滥用:注册/登录/忘记密码/验证邮箱/重置密码按来源 IP 做内存限流。

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig 认证类端点的每 IP 限流配置(0 或负数 = 关闭)。
// 注册与其余认证端点分开计数,避免注册风暴拖垮登录限流。
type RateLimitConfig struct {
	RegisterLimit  int // 每窗口内 /api/auth/register 允许次数
	RegisterWindow time.Duration
	AuthLimit      int // 每窗口内 login/verify-email/forgot/reset 允许次数
	AuthWindow     time.Duration
}

// DefaultRateLimit 生产默认:注册每 IP 每分钟 10 次,其余认证操作每 IP 每分钟 20 次。
// 语义是**滚动窗口内最多 N 次**(见 rateLimiter.Allow),与这句话一致。
func DefaultRateLimit() RateLimitConfig {
	return RateLimitConfig{
		RegisterLimit:  10,
		RegisterWindow: time.Minute,
		AuthLimit:      20,
		AuthWindow:     time.Minute,
	}
}

// maxRateKeys 内存上限:超过该数量时淘汰过期条目,防止恶意来源 IP 撑爆内存。
const maxRateKeys = 8192

type visitorBucket struct {
	// hits 保存最近一个窗口内每一次命中时刻(unix nano),升序,长度 ≤ limit。
	hits     []int64
	lastSeen time.Time
}

// rateLimiter 滚动窗口计数器:同一 key 在**任意连续 window 区间内**最多放行 limit 次。
//
// 为什么不用 golang.org/x/time/rate 的令牌桶:令牌桶的 rLimit 只决定**补充**速度,
// 而新建的桶是满桶(burst = limit),于是第一个窗口里实际能打出去 limit + limit
// = 2 × limit 次 —— 文档承诺"每分钟 10 次",实测是 20 次。要让实际配额等于承诺值,
// 就得让初始桶不再白送 limit 个令牌,也就是把突发容忍度压到 0(burst = 1)。
//
// 但 burst = 1 意味着同一出口 IP(公司 NAT / 移动网络 / 家庭网关后面几十上百人)之间
// 互相挤兑:共享 IP 的用户登录后 3 秒内再提交一次就吃 429,而这不是任何单个人在滥用。
// 滚动窗口两头都占得住 —— 窗口内第 N 次之前都放行(不误伤),任何 60 秒滑动区间内又
// 绝不超过 N 次(不放水),与文档措辞逐字一致。
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*visitorBucket
	now     func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit <= 0 || window <= 0 {
		return &rateLimiter{limit: 0}
	}
	return &rateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*visitorBucket),
		now:     time.Now,
	}
}

// Allow 对 key 计数;未超限返回 true,超限返回 false。
func (l *rateLimiter) Allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		// 当 map 偏大时清理超过 2 个窗口未访问的条目
		if len(l.buckets) >= maxRateKeys {
			l.purgeExpired(now)
		}
		if len(l.buckets) >= maxRateKeys {
			return false
		}
		b = &visitorBucket{hits: make([]int64, 0, l.limit), lastSeen: now}
		l.buckets[key] = b
	}

	b.lastSeen = now
	// 先丢弃已滑出窗口的旧命中(hits 升序,从头扫即可)
	cutoff := now.Add(-l.window).UnixNano()
	drop := 0
	for drop < len(b.hits) && b.hits[drop] <= cutoff {
		drop++
	}
	if drop > 0 {
		// 复用底层数组(b.hits[:0] 与 b.hits[drop:] 有重叠,append 的逐元素拷贝
		// 从左向右进行,源永远在目标之后,不会覆盖未搬完的元素)
		b.hits = append(b.hits[:0], b.hits[drop:]...)
	}
	if len(b.hits) >= l.limit {
		return false
	}
	b.hits = append(b.hits, now.UnixNano())
	return true
}

// purgeExpired 删除过期条目，防止无界增长。调用方须持有 mu。
func (l *rateLimiter) purgeExpired(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) >= l.window*2 {
			delete(l.buckets, k)
		}
	}
}

// remoteHostOnly 去掉 RemoteAddr 的端口,返回可用于 net.ParseIP 的主机部分。
func remoteHostOnly(remote string) string {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	if ip := net.ParseIP(strings.Trim(remote, "[]")); ip != nil {
		return ip.String()
	}
	return remote
}

// forwardedClientIP 仅在直连来源是内网/回环(即部署前置 Caddy/本机代理)时,
// 才信任 X-Forwarded-For,且取**最右**一段——那才是本跳代理追加的真实客户端 IP。
//
// 为什么不是最左:反向代理(Caddy 2 默认行为)是把客户端 IP **追加**到既有
// XFF 之后,而不是覆盖。客户端自带 `X-Forwarded-For: 9.9.9.9` 时,后端看到的是
// `9.9.9.9, <真实 IP>`,取最左等于采信访客自选值 → 注册/登录限流可绕过、
// visits.ip 与国家码被污染、规则的 ip/ipattr/country 条件可被规避。
// 部署侧对应地在 Caddyfile 里用 `header_up X-Forwarded-For {remote_host}` 覆盖该头。
//
// ⚠️ 「对端是私网就可信」是推断,不是事实:README 6.4 允许后端 :8080 裸跑,此时局域网内
// 任何主机都能直连并自带 XFF 换限流桶。所以认证端点的限流**不再**走这里,改用
// (*API).trustedClientIP(显式可信代理网段,配置为空则永不采信)。
// 保留本函数是因为 visits/rules/redirect 仍共用它,而那三个文件不在本次改动的
// 所有权范围内 —— 把它们也切到 trustedClientIP 是同一件事的后续项。
func forwardedClientIP(r *http.Request) string {
	peer := net.ParseIP(remoteHostOnly(r.RemoteAddr))
	if peer == nil || (!peer.IsLoopback() && !peer.IsPrivate()) {
		return ""
	}
	return lastForwardedIP(r)
}

// lastForwardedIP 取 X-Forwarded-For 中最右侧的最后一个可解析 IP。
func lastForwardedIP(r *http.Request) string {
	var last string
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := net.ParseIP(strings.TrimSpace(part)); ip != nil {
			last = ip.String()
		}
	}
	return last
}

// clientIP 解析请求来源 IP:内网反代场景取可信的 X-Forwarded-For 最右段,
// 否则取 RemoteAddr。认证端点限流请用 (*API).trustedClientIP(见其注释)。
func clientIP(r *http.Request) string {
	if ip := forwardedClientIP(r); ip != "" {
		return ip
	}
	return remoteHostOnly(r.RemoteAddr)
}

// trustedClientIP 是**认证端点限流**使用的来源 IP:只有 TCP 对端落在运维显式声明的
// 可信代理网段(CLOAK_TRUSTED_PROXY_CIDRS)内,才采信 X-Forwarded-For;
// 配置为空则**永不**采信,只认 TCP 对端。
//
// 为什么不做「私网即信」的推断:谁在前置代理是部署事实,只有部署者知道 ——
// compose 网络里的同网段主机、裸跑时局域网内直连 :8080 的机器、多级代理链,
// 后端都无法从源地址猜出来。猜错的代价是限流被完全绕过(注册/登录/找回密码),
// 所以这里 fail-closed:宁可把代理 IP 当成所有用户的共同来源(限流偏严),
// 也不能把限流桶的归属权交给访客自选的 XFF。
func (a *API) trustedClientIP(r *http.Request) string {
	if ip := forwardedClientIPTrusted(r, a.trustedProxyNets); ip != "" {
		return ip
	}
	return remoteHostOnly(r.RemoteAddr)
}

// forwardedClientIPTrusted 在对端命中可信代理网段时,返回 X-Forwarded-For 最右一段。
func forwardedClientIPTrusted(r *http.Request, trusted []netip.Prefix) string {
	if !isTrustedProxyPeer(r.RemoteAddr, trusted) {
		return ""
	}
	return lastForwardedIP(r)
}

// isTrustedProxyPeer 判断 TCP 对端是否落在可信代理网段内。
// 可信网段为空 → 一律判为不可信(never trust XFF)。
func isTrustedProxyPeer(remote string, trusted []netip.Prefix) bool {
	if len(trusted) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(remoteHostOnly(remote))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// authRateLimit 注册端点限流:超限 429。
func (a *API) rateLimit(c *gin.Context, l *rateLimiter) bool {
	if !l.Allow(a.trustedClientIP(c.Request)) {
		writeErr(c, http.StatusTooManyRequests, errRateLimited, "请求过于频繁,请稍后再试")
		return false
	}
	return true
}
