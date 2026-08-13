package domain

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

// ProbeCert 通过 HTTPS 访问域名,触发 Caddy on-demand TLS 授权签发。
// 任意 HTTP 响应都说明 TLS 握手成功、证书已签发(本地 CA/Let's Encrypt 由 Caddy 管理)。
// 不校验证书链:本探活目的仅为触发签发,开发环境使用本地 CA(见 ADR-0002/0004)。
func ProbeCert(ctx context.Context, fqdn string) bool {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 见上
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+fqdn+"/", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return true
}
