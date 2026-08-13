package httpapi

// 安全与防滥用(spec 决策 #12):注册/登录/忘记密码/验证邮箱/重置密码按来源 IP 做内存限流。
// 标准库实现(固定窗口计数器),可配置、可测试;limit<=0 表示关闭。

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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

type rateWindow struct {
	start time.Time
	count int
}

// rateLimiter 固定窗口计数器:每 (limit, window) 内允许 limit 次;limit<=0 关闭。
// maxRateKeys 内存上限:超过该数量时淘汰过期条目,防止恶意来源 IP 撑爆内存。
const maxRateKeys = 8192

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*rateWindow
	now     func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit <= 0 || window <= 0 {
		return &rateLimiter{limit: 0}
	}
	return &rateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*rateWindow),
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
	w, ok := l.buckets[key]
	if !ok || now.Sub(w.start) >= l.window {
		// 仅重置当前 key 的窗口,不得清空其他 IP 的计数(否则会削弱限流)。
		// 顺带在 map 偏大时淘汰过期条目,避免无限增长。
		if len(l.buckets) >= maxRateKeys {
			l.purgeExpired(now)
		}
		l.buckets[key] = &rateWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= l.limit
}

// purgeExpired 删除已过期窗口的条目,控制 map 大小。调用方须持有 mu。
func (l *rateLimiter) purgeExpired(now time.Time) {
	for k, w := range l.buckets {
		if now.Sub(w.start) >= l.window {
			delete(l.buckets, k)
		}
	}
}

// clientIP 解析请求来源 IP:优先取 X-Forwarded-For 首段(部署前置 Caddy 转发),
// 否则取 RemoteAddr。IPv6 去端口后原样返回(可被 [] 包裹)。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// authRateLimit 注册端点限流:超限 429。
func (a *API) rateLimit(w http.ResponseWriter, r *http.Request, l *rateLimiter) bool {
	if !l.Allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, errRateLimited, "请求过于频繁,请稍后再试")
		return false
	}
	return true
}
