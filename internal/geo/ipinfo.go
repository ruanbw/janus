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

// IPinfoConfig ipinfo.io 服务配置
type IPinfoConfig struct {
	Token   string
	BaseURL string
	Client  *http.Client
}

type ipinfoProvider struct {
	token   string
	baseURL string
	client  *http.Client
}

// NewIPinfoProvider 创建基于 IPinfo 的情报提供者
func NewIPinfoProvider(cfg IPinfoConfig) Provider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://ipinfo.io"
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &ipinfoProvider{
		token:   strings.TrimSpace(cfg.Token),
		baseURL: baseURL,
		client:  client,
	}
}

func (p *ipinfoProvider) Name() string { return "ipinfo" }

type ipinfoResponse struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	Org     string `json:"org"`
	Privacy struct {
		VPN     bool `json:"vpn"`
		Proxy   bool `json:"proxy"`
		Tor     bool `json:"tor"`
		Relay   bool `json:"relay"`
		Hosting bool `json:"hosting"`
	} `json:"privacy"`
}

func (p *ipinfoProvider) LookupIP(ctx context.Context, ip string) (Info, error) {
	reqURL := fmt.Sprintf("%s/%s/json", p.baseURL, url.PathEscape(ip))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Info{}, err
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Info{}, fmt.Errorf("ipinfo status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res ipinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return Info{}, fmt.Errorf("decode ipinfo json: %w", err)
	}

	country := strings.ToUpper(strings.TrimSpace(res.Country))
	if !isAlpha2(country) {
		country = ""
	}
	asn := formatASN(res.Org)
	isDC := res.Privacy.Hosting || res.Privacy.Proxy || res.Privacy.VPN || res.Privacy.Tor || res.Privacy.Relay

	return Info{
		Country:      country,
		ASN:          asn,
		IsDatacenter: isDC,
	}, nil
}
