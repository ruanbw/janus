package httpapi

// 安全与防滥用:注册/登录/忘记密码/验证邮箱/重置密码按来源 IP 做内存限流。
// 使用 Go 官方扩展库 golang.org/x/time/rate 令牌桶算法，成熟、稳定、抗突发。

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
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
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiter 基于 golang.org/x/time/rate 的令牌桶限流器包装。
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	rLimit  rate.Limit
	burst   int
	buckets map[string]*visitorBucket
	now     func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit <= 0 || window <= 0 {
		return &rateLimiter{limit: 0}
	}
	// 速率: limit 次 / window 时间
	rLimit := rate.Every(window / time.Duration(limit))
	return &rateLimiter{
		limit:   limit,
		window:  window,
		rLimit:  rLimit,
		burst:   limit,
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
		b = &visitorBucket{
			limiter:  rate.NewLimiter(l.rLimit, l.burst),
			lastSeen: now,
		}
		l.buckets[key] = b
	}

	b.lastSeen = now
	return b.limiter.Allow()
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
// 才信任 X-Forwarded-For 首段;公网直连时忽略该头,防止客户端伪造来源 IP
// 绕过限流或污染访问统计。首段必须是合法 IP,否则回退 RemoteAddr。
func forwardedClientIP(r *http.Request) string {
	peer := net.ParseIP(remoteHostOnly(r.RemoteAddr))
	if peer == nil || (!peer.IsLoopback() && !peer.IsPrivate()) {
		return ""
	}
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		first := strings.TrimSpace(part)
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// clientIP 解析请求来源 IP:内网反代场景取可信的 X-Forwarded-For 首段,
// 否则取 RemoteAddr。
func clientIP(r *http.Request) string {
	if ip := forwardedClientIP(r); ip != "" {
		return ip
	}
	return remoteHostOnly(r.RemoteAddr)
}

// authRateLimit 注册端点限流:超限 429。
func (a *API) rateLimit(c *gin.Context, l *rateLimiter) bool {
	if !l.Allow(clientIP(c.Request)) {
		writeErr(c, http.StatusTooManyRequests, errRateLimited, "请求过于频繁,请稍后再试")
		return false
	}
	return true
}
