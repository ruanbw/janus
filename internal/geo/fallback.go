package geo

import (
	"context"
	"net"
	"sync"
	"time"
)

// FallbackConfig 带有本地兜底的 Lookup 配置
type FallbackConfig struct {
	// Primary 首选远程情报 SaaS Provider
	Primary Provider
	// Fallback 本地离线库兜底 Lookup (如 xdb)
	Fallback Lookup
	// Timeout 单次外部查询超时上限;若 <= 0 则默认为 500ms
	Timeout time.Duration
	// MaxConsecutiveFailures 触发熔断的连续失败次数; <= 0 时默认为 5
	MaxConsecutiveFailures int
	// CircuitBreakerCooldown 熔断器开启后的冷却恢复期; <= 0 时默认为 30s
	CircuitBreakerCooldown time.Duration
}

type fallbackLookup struct {
	primary  Provider
	fallback Lookup
	timeout  time.Duration

	maxFailures int
	cooldown    time.Duration

	mu              sync.RWMutex
	consecFailures  int
	circuitOpenTill time.Time
	probing         bool
}

// NewFallbackLookup 构建优先查询外部 SaaS、失败或超时平滑回退到本地库的复合 Lookup。
// 内置连续失败熔断机制:当外部 SaaS 连续不可用时自动熔断冷却,避免每个冷请求等待超时。
func NewFallbackLookup(cfg FallbackConfig) Lookup {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	fb := cfg.Fallback
	if fb == nil {
		fb = Disabled
	}
	maxFailures := cfg.MaxConsecutiveFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}
	cooldown := cfg.CircuitBreakerCooldown
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &fallbackLookup{
		primary:     cfg.Primary,
		fallback:    fb,
		timeout:     timeout,
		maxFailures: maxFailures,
		cooldown:    cooldown,
	}
}

// tryPrimary 判断本次是否应请求 primary。
// 熔断开启时一律拒绝;熔断冷却到期后放行正好一个探测请求(半开状态),
// 防止冷却结束的一瞬间所有在途并发一起冲向已故障的 primary,
// 第一次失败就又立刻全开(全员去接 3s 超时)。
func (f *fallbackLookup) tryPrimary() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Now().Before(f.circuitOpenTill) {
		return false
	}
	if f.circuitOpenTill.IsZero() {
		return true // 从未熔断
	}
	if f.probing {
		return false // 已有探测在途
	}
	f.probing = true
	return true
}

func (f *fallbackLookup) recordSuccess() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consecFailures = 0
	f.circuitOpenTill = time.Time{}
	f.probing = false
}

func (f *fallbackLookup) recordFailure() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consecFailures++
	if f.consecFailures >= f.maxFailures {
		f.circuitOpenTill = time.Now().Add(f.cooldown)
		f.probing = false
	}
}

func (f *fallbackLookup) Lookup(ip string) Info {
	if ip == "" {
		return Info{}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Info{}
	}
	// 内网/回环/未指定地址直接短路返回零值，避免无效外部调用
	if isNonPublicIP(parsed) {
		return Info{}
	}

	if f.primary != nil && f.tryPrimary() {
		ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
		info, err := f.primary.LookupIP(ctx, ip)
		cancel()
		if err == nil {
			// 半开探测成功:熔断复位。
			f.mu.Lock()
			f.probing = false
			f.mu.Unlock()
		}

		if err == nil {
			f.recordSuccess()
			// 如果 Primary 未获取到国家码，尝试用本地兜底补齐国家
			if info.Country == "" {
				fallbackInfo := f.fallback.Lookup(ip)
				if fallbackInfo.Country != "" {
					info.Country = fallbackInfo.Country
				}
			}
			return info
		}

		// 记录外部失败并可能触发熔断
		f.recordFailure()
	}

	// 远程故障、超时、熔断或未提供 Primary 时，平滑回落到本地库
	return f.fallback.Lookup(ip)
}

// isNonPublicIP 内网/回环/链路本地/未指定地址:无地理意义且不应发起外部查询。
func isNonPublicIP(p net.IP) bool {
	return p.IsLoopback() || p.IsPrivate() ||
		p.IsLinkLocalUnicast() || p.IsLinkLocalMulticast() ||
		p.IsUnspecified()
}
