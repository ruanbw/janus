package geo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 1. 热门厂商 SaaS Provider 契约规格 (Given-When-Then)
// ============================================================================

// [IPinfo] 规格验证：解析 Country, ASN, IsDatacenter (Privacy.Hosting)
func TestIPinfoProvider_LookupIP(t *testing.T) {
	t.Run("Given 正常响应 When 查询 IP Then 正确提取国家/ASN/机房属性", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Path != "/8.8.8.8/json" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ip":      "8.8.8.8",
				"country": "US",
				"org":     "AS15169 Google LLC",
				"privacy": map[string]any{
					"vpn":     false,
					"proxy":   false,
					"tor":     false,
					"relay":   false,
					"hosting": true,
				},
			})
		}))
		defer ts.Close()

		p := NewIPinfoProvider(IPinfoConfig{
			Token:   "test-token",
			BaseURL: ts.URL,
		})

		ctx := context.Background()
		info, err := p.LookupIP(ctx, "8.8.8.8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Country != "US" {
			t.Errorf("Country = %q, want %q", info.Country, "US")
		}
		if info.ASN != "AS15169" {
			t.Errorf("ASN = %q, want %q", info.ASN, "AS15169")
		}
		if !info.IsDatacenter {
			t.Errorf("IsDatacenter = %v, want true", info.IsDatacenter)
		}
	})

	t.Run("Given 鉴权失败或配额耗尽 When 查询 IP Then 返回明确错误", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"status": 429, "error": "rate limited"}`))
		}))
		defer ts.Close()

		p := NewIPinfoProvider(IPinfoConfig{
			Token:   "invalid-token",
			BaseURL: ts.URL,
		})

		ctx := context.Background()
		_, err := p.LookupIP(ctx, "8.8.8.8")
		if err == nil {
			t.Fatal("expected error on 429 rate limit, got nil")
		}
	})
}

// [IPQualityScore] 规格验证：解析 Country, ASN, IsDatacenter (Proxy/VPN/ActiveVPN)
func TestIPQualityScoreProvider_LookupIP(t *testing.T) {
	t.Run("Given 正常风控响应 When 查询 IP Then 正确提取国家/ASN/机房属性", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/json/ip/test-key/1.1.1.1" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":      true,
				"country_code": "AU",
				"ASN":          13335,
				"proxy":        true,
				"vpn":          false,
				"tor":          false,
				"active_vpn":   false,
				"is_crawler":   false,
			})
		}))
		defer ts.Close()

		p := NewIPQualityScoreProvider(IPQualityScoreConfig{
			APIKey:  "test-key",
			BaseURL: ts.URL,
		})

		ctx := context.Background()
		info, err := p.LookupIP(ctx, "1.1.1.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Country != "AU" {
			t.Errorf("Country = %q, want %q", info.Country, "AU")
		}
		if info.ASN != "AS13335" {
			t.Errorf("ASN = %q, want %q", info.ASN, "AS13335")
		}
		if !info.IsDatacenter {
			t.Errorf("IsDatacenter = %v, want true", info.IsDatacenter)
		}
	})

	t.Run("Given API 返回业务失败 success=false When 查询 IP Then 返回错误", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "Quota exceeded",
			})
		}))
		defer ts.Close()

		p := NewIPQualityScoreProvider(IPQualityScoreConfig{
			APIKey:  "test-key",
			BaseURL: ts.URL,
		})

		_, err := p.LookupIP(context.Background(), "1.1.1.1")
		if err == nil {
			t.Fatal("expected error on success=false, got nil")
		}
	})
}

// [IP-API] 规格验证：支持 free/pro 格式解析
func TestIPAPIProvider_LookupIP(t *testing.T) {
	t.Run("Given 成功响应 When 查询 IP Then 正确提取 Country/ASN/Hosting", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":      "success",
				"countryCode": "JP",
				"as":          "AS2497 Internet Initiative Japan Inc.",
				"hosting":     true,
				"proxy":       false,
			})
		}))
		defer ts.Close()

		p := NewIPAPIProvider(IPAPIConfig{
			BaseURL: ts.URL,
		})

		info, err := p.LookupIP(context.Background(), "202.232.2.2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Country != "JP" {
			t.Errorf("Country = %q, want %q", info.Country, "JP")
		}
		if info.ASN != "AS2497" {
			t.Errorf("ASN = %q, want %q", info.ASN, "AS2497")
		}
		if !info.IsDatacenter {
			t.Errorf("IsDatacenter = %v, want true", info.IsDatacenter)
		}
	})

	t.Run("Given status=fail When 查询 IP Then 返回错误", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "fail",
				"message": "invalid query",
			})
		}))
		defer ts.Close()

		p := NewIPAPIProvider(IPAPIConfig{BaseURL: ts.URL})
		_, err := p.LookupIP(context.Background(), "invalid-ip")
		if err == nil {
			t.Fatal("expected error for fail status, got nil")
		}
	})
}

// ============================================================================
// 2. 本地数据库兜底架构契约规格 (Fallback Lookup)
// ============================================================================

// 模拟用 Provider
type mockProvider struct {
	name     string
	lookupFn func(ctx context.Context, ip string) (Info, error)
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) LookupIP(ctx context.Context, ip string) (Info, error) {
	return m.lookupFn(ctx, ip)
}

func TestFallbackLookup_GivenPrimarySuccess(t *testing.T) {
	// Given: SaaS Primary 服务正常返回完整情报，本地兜底也有自己的数据
	primary := &mockProvider{
		name: "primary-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			return Info{Country: "US", ASN: "AS15169", IsDatacenter: true}, nil
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{Country: "CN"} // 本地库只有国家
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
		Timeout:  100 * time.Millisecond,
	})

	// When: 执行 Lookup
	info := lookup.Lookup("8.8.8.8")

	// Then: 优先采信 Primary 数据
	if info.Country != "US" || info.ASN != "AS15169" || !info.IsDatacenter {
		t.Fatalf("unexpected info from primary: %+v", info)
	}
}

func TestFallbackLookup_GivenPrimaryFailure_FallsBackToLocal(t *testing.T) {
	// Given: SaaS 故障（报错 500、网络断开等），本地库正常
	var fallbackCalls int32
	primary := &mockProvider{
		name: "broken-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			return Info{}, errors.New("upstream saas 500 internal server error")
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		atomic.AddInt32(&fallbackCalls, 1)
		return Info{Country: "DE"}
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
		Timeout:  50 * time.Millisecond,
	})

	// When: 执行 Lookup
	info := lookup.Lookup("1.2.3.4")

	// Then: 平滑无缝降级到本地数据库，返回本地库结果，不崩溃、不报 panic
	if info.Country != "DE" {
		t.Fatalf("expected country from fallback DE, got %q", info.Country)
	}
	if atomic.LoadInt32(&fallbackCalls) != 1 {
		t.Fatalf("fallback should be called once, got %d", fallbackCalls)
	}
}

func TestFallbackLookup_GivenPrimaryTimeout_FallsBackToLocal(t *testing.T) {
	// Given: SaaS 响应延迟过高，超过了设定的保护超时预算 (50ms)
	primary := &mockProvider{
		name: "slow-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			select {
			case <-time.After(300 * time.Millisecond):
				return Info{Country: "US"}, nil
			case <-ctx.Done():
				return Info{}, ctx.Err()
			}
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{Country: "FR"}
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
		Timeout:  30 * time.Millisecond,
	})

	// When: 执行 Lookup
	start := time.Now()
	info := lookup.Lookup("1.2.3.4")
	duration := time.Since(start)

	// Then: 快速回退，耗时紧跟超时预算，结果回落到本地库
	if duration >= 150*time.Millisecond {
		t.Fatalf("lookup took too long: %v, expected timeout around 30ms", duration)
	}
	if info.Country != "FR" {
		t.Fatalf("expected country from fallback FR, got %q", info.Country)
	}
}

func TestFallbackLookup_GivenBothMiss_ReturnsZeroInfo(t *testing.T) {
	// Given: SaaS 与 本地库均无结果（例如保留私网地址、未知网段）
	primary := &mockProvider{
		name: "empty-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			return Info{}, nil
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{}
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
	})

	// When: 查询
	info := lookup.Lookup("192.168.1.1")

	// Then: 必须返回纯零值 Info（ADR 0009 核心不变式：空值恒不命中，不可填占位）
	if info != (Info{}) {
		t.Fatalf("expected empty Info{}, got %+v", info)
	}
}

// ============================================================================
// 3. 缓存整合契约测试 (Negative Caching & Protection)
// ============================================================================

func TestFallbackLookup_WithCaching_RemembersNegativeAndSavesQuota(t *testing.T) {
	// Given: 一个被缓存包装的 FallbackLookup
	var primaryCalls int32
	primary := &mockProvider{
		name: "metered-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			atomic.AddInt32(&primaryCalls, 1)
			return Info{}, errors.New("not found")
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{} // 本地库也没找到
	})

	rawLookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
		Timeout:  50 * time.Millisecond,
	})

	cachedLookup := Cached(rawLookup, 64)

	// When: 针对同一未知 IP 连续查询 5 次
	for i := 0; i < 5; i++ {
		info := cachedLookup.Lookup("198.51.100.1")
		if info != (Info{}) {
			t.Fatalf("expected empty info, got %+v", info)
		}
	}

	// Then: 负结果必须被缓存，外部 SaaS 只能被调用 1 次，严防配额被未知流量打穿
	if calls := atomic.LoadInt32(&primaryCalls); calls != 1 {
		t.Fatalf("primary calls = %d, want 1 (negative caching broken)", calls)
	}
}

func TestNewProvider_Factory(t *testing.T) {
	cases := []struct {
		name     string
		apiKey   string
		wantType string
	}{
		{"ipinfo", "tok1", "ipinfo"},
		{"IPINFO", "tok2", "ipinfo"},
		{"ipqualityscore", "key1", "ipqualityscore"},
		{"ipqs", "key2", "ipqualityscore"},
		{"ipapi", "key3", "ip-api"},
		{"ip-api", "key4", "ip-api"},
	}

	for _, tc := range cases {
		p := NewProvider(tc.name, tc.apiKey)
		if p == nil {
			t.Fatalf("NewProvider(%q) returned nil", tc.name)
		}
		if p.Name() != tc.wantType {
			t.Fatalf("NewProvider(%q).Name() = %q, want %q", tc.name, p.Name(), tc.wantType)
		}
	}

	if p := NewProvider("unknown", "key"); p != nil {
		t.Fatalf("expected nil for unknown provider, got %+v", p)
	}
}

func TestFallbackLookup_GivenPrimaryHasASNNoCountry_FallbackFillsCountry(t *testing.T) {
	// Given: SaaS 成功返回了 ASN 与机房标记，但 Country 缺失为空
	primary := &mockProvider{
		name: "partial-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			return Info{Country: "", ASN: "AS13335", IsDatacenter: true}, nil
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{Country: "AU"} // 本地离线库有国家数据
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:  primary,
		Fallback: fallback,
	})

	// When: 执行 Lookup
	info := lookup.Lookup("1.1.1.1")

	// Then: ASN 保持 Primary 提供的高精数据，Country 由本地兜底成功补全
	if info.ASN != "AS13335" {
		t.Errorf("ASN = %q, want AS13335", info.ASN)
	}
	if !info.IsDatacenter {
		t.Errorf("IsDatacenter = %v, want true", info.IsDatacenter)
	}
	if info.Country != "AU" {
		t.Errorf("Country = %q, want AU (should be filled from fallback)", info.Country)
	}
}

func TestFallbackLookup_CircuitBreaker(t *testing.T) {
	// Given: SaaS 持续报错，超过连续失败上限 2 次后开启熔断
	var calls int32
	primary := &mockProvider{
		name: "failing-saas",
		lookupFn: func(ctx context.Context, ip string) (Info, error) {
			atomic.AddInt32(&calls, 1)
			return Info{}, errors.New("upstream dead")
		},
	}
	fallback := LookupFunc(func(ip string) Info {
		return Info{Country: "JP"}
	})

	lookup := NewFallbackLookup(FallbackConfig{
		Primary:                primary,
		Fallback:               fallback,
		Timeout:                50 * time.Millisecond,
		MaxConsecutiveFailures: 2,
		CircuitBreakerCooldown: 100 * time.Millisecond,
	})

	// 第一次调用: 失败 1 次
	lookup.Lookup("1.2.3.4")
	// 第二次调用: 失败 2 次，触发熔断开启
	lookup.Lookup("1.2.3.5")
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}

	// 第三次和第四次调用: 处于熔断冷却期，直接走 fallback，不再发出外部调用
	for i := 0; i < 5; i++ {
		info := lookup.Lookup("1.2.3.6")
		if info.Country != "JP" {
			t.Fatalf("expected JP from fallback, got %q", info.Country)
		}
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("circuit breaker failed to fast-fail, expected calls to remain 2, got %d", calls)
	}

	// 等待熔断冷却期结束
	time.Sleep(120 * time.Millisecond)

	// 第五次调用: 冷却后放行 1 次探测
	lookup.Lookup("1.2.3.7")
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("expected 1 probe call after cooldown, got %d", calls)
	}
}

func TestNewProvider_RejectsEmptyKeyForRequiredVendors(t *testing.T) {
	// IPQualityScore 强鉴权: 缺少 apiKey 时必须返回 nil
	if p := NewProvider("ipqualityscore", ""); p != nil {
		t.Errorf("expected nil for ipqualityscore with empty apiKey, got %+v", p)
	}
	if p := NewProvider("ipqs", "   "); p != nil {
		t.Errorf("expected nil for ipqs with whitespace apiKey, got %+v", p)
	}
	if p := NewProvider("ipqs", "valid-key"); p == nil {
		t.Errorf("expected provider for ipqs with valid key, got nil")
	}
}
