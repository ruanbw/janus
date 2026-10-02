package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IPQualityScoreConfig IPQualityScore 配置
type IPQualityScoreConfig struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

type ipqsProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewIPQualityScoreProvider 创建基于 IPQualityScore 的情报提供者
func NewIPQualityScoreProvider(cfg IPQualityScoreConfig) Provider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://ipqualityscore.com"
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &ipqsProvider{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: baseURL,
		client:  client,
	}
}

func (p *ipqsProvider) Name() string { return "ipqualityscore" }

type ipqsResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	CountryCode string `json:"country_code"`
	ASN         any    `json:"ASN"`
	Proxy       bool   `json:"proxy"`
	VPN         bool   `json:"vpn"`
	Tor         bool   `json:"tor"`
	ActiveVPN   bool   `json:"active_vpn"`
	IsCrawler   bool   `json:"is_crawler"`
}

func (p *ipqsProvider) LookupIP(ctx context.Context, ip string) (Info, error) {
	reqURL := fmt.Sprintf("%s/json/ip/%s/%s", p.baseURL, url.PathEscape(p.apiKey), url.PathEscape(ip))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		// url.Error 会带上完整 URL(path 里含 key),统一脱敏
		return Info{}, fmt.Errorf("ipqualityscore request failed: %v", sanitizeURLErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Info{}, fmt.Errorf("ipqualityscore status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res ipqsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return Info{}, fmt.Errorf("decode ipqualityscore json: %w", err)
	}

	if !res.Success {
		msg := res.Message
		if msg == "" {
			msg = "unknown error"
		}
		return Info{}, fmt.Errorf("ipqualityscore request failed: %s", msg)
	}

	country := strings.ToUpper(strings.TrimSpace(res.CountryCode))
	if !isAlpha2(country) {
		country = ""
	}
	asn := formatASN(res.ASN)
	isDC := res.Proxy || res.VPN || res.Tor || res.ActiveVPN || res.IsCrawler

	return Info{
		Country:      country,
		ASN:          asn,
		IsDatacenter: isDC,
	}, nil
}
