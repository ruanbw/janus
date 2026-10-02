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

// IPAPIConfig ip-api.com 服务配置
type IPAPIConfig struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

type ipapiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewIPAPIProvider 创建基于 IP-API 的情报提供者
func NewIPAPIProvider(cfg IPAPIConfig) Provider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://ip-api.com"
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &ipapiProvider{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: baseURL,
		client:  client,
	}
}

func (p *ipapiProvider) Name() string { return "ip-api" }

type ipapiResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	CountryCode string `json:"countryCode"`
	AS          string `json:"as"`
	Hosting     bool   `json:"hosting"`
	Proxy       bool   `json:"proxy"`
}

func (p *ipapiProvider) LookupIP(ctx context.Context, ip string) (Info, error) {
	reqURL := fmt.Sprintf("%s/json/%s?fields=status,message,countryCode,as,hosting,proxy", p.baseURL, url.PathEscape(ip))
	if p.apiKey != "" {
		reqURL += "&key=" + url.QueryEscape(p.apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Info{}, fmt.Errorf("ip-api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res ipapiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return Info{}, fmt.Errorf("decode ip-api json: %w", err)
	}

	if strings.ToLower(res.Status) != "success" {
		msg := res.Message
		if msg == "" {
			msg = "request failed"
		}
		return Info{}, fmt.Errorf("ip-api error: %s", msg)
	}

	country := strings.ToUpper(strings.TrimSpace(res.CountryCode))
	if !isAlpha2(country) {
		country = ""
	}
	asn := formatASN(res.AS)
	isDC := res.Hosting || res.Proxy

	return Info{
		Country:      country,
		ASN:          asn,
		IsDatacenter: isDC,
	}, nil
}
