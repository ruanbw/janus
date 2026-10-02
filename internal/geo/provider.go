package geo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Provider 外部或三方 IP 情报服务提供者接口。
// 各厂商（SaaS）只需实现此接口，由 FallbackLookup 或应用层按需调度。
type Provider interface {
	Name() string
	LookupIP(ctx context.Context, ip string) (Info, error)
}

// formatASN 将各种形态的 ASN（如 15169 或 "AS15169 Google LLC"）归一化为标准的 "ASxxxxx"。
func formatASN(val any) string {
	switch v := val.(type) {
	case int:
		if v > 0 {
			return fmt.Sprintf("AS%d", v)
		}
	case int64:
		if v > 0 {
			return fmt.Sprintf("AS%d", v)
		}
	case float64:
		if v > 0 {
			return fmt.Sprintf("AS%d", int64(v))
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return ""
		}
		// 拆取首段，例如 "AS15169 Google LLC" -> "AS15169"
		fields := strings.Fields(s)
		if len(fields) > 0 {
			s = fields[0]
		}
		upper := strings.ToUpper(s)
		if strings.HasPrefix(upper, "AS") {
			numPart := upper[2:]
			if _, err := strconv.ParseInt(numPart, 10, 64); err == nil {
				return upper
			}
		} else if _, err := strconv.ParseInt(s, 10, 64); err == nil {
			return "AS" + s
		}
	}
	return ""
}

// NewProvider 根据厂商名称创建对应的 SaaS 情报提供者。
// 对于强依赖认证凭据的厂商(如 IPQualityScore)，若 apiKey 为空则直接返回 nil(避免无效外部请求)。
// 未知厂商返回 nil。
func NewProvider(name, apiKey string) Provider {
	trimmedKey := strings.TrimSpace(apiKey)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ipinfo":
		return NewIPinfoProvider(IPinfoConfig{Token: trimmedKey})
	case "ipqualityscore", "ipqs":
		if trimmedKey == "" {
			return nil
		}
		return NewIPQualityScoreProvider(IPQualityScoreConfig{APIKey: trimmedKey})
	case "ipapi", "ip-api":
		return NewIPAPIProvider(IPAPIConfig{APIKey: trimmedKey})
	default:
		return nil
	}
}
