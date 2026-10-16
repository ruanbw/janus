package httpapi_test

// 429 必须带 Retry-After(RFC 6585 §4),且必须是**该 key 精确**的等待时间:
// 客户端据此可以正确地等一次再重试,而不是整窗干等或反复撞限流。

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/testutil"
)

func TestRateLimit_RetryAfterHeaderPresent(t *testing.T) {
	env := testutil.Setup(t)

	// 把限流压到 2 次/10 分钟,保证第 3 次必超限且等待时间可预期
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store:     env.Store,
		Cfg:       env.Cfg,
		GeoLookup: geo.Disabled,
		RateLimit: &httpapi.RateLimitConfig{
			RegisterLimit: 2, RegisterWindow: 10 * 60 * 1e9, // 10 分钟
			AuthLimit: 2, AuthWindow: 10 * 60 * 1e9,
		},
	}))
	t.Cleanup(srv.Close)

	// 打满前两次
	for i := 0; i < 2; i++ {
		resp, err := http.Post(srv.URL+"/api/auth/login", "application/json", nil)
		if err != nil {
			t.Fatalf("第 %d 次登录请求失败: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// 第 3 次:必须 429,且带 Retry-After
	resp, err := http.Post(srv.URL+"/api/auth/login", "application/json", nil)
	if err != nil {
		t.Fatalf("第 3 次登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("超限请求 = %d, want 429", resp.StatusCode)
	}
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		t.Fatalf("429 响应缺少 Retry-After 头(客户端无从得知何时该重试)")
	}
	sec, err := strconv.Atoi(ra)
	if err != nil {
		t.Fatalf("Retry-After 不是整数秒: %q", ra)
	}
	if sec < 1 {
		t.Errorf("Retry-After = %d,应为正数", sec)
	}
	// 窗口是 10 分钟,刚打满就超限 → 等待时间应接近整窗(>9 分钟),
	// 若退化成"1 秒"这类无意义值,说明精确计算没生效。
	if sec < 540 {
		t.Errorf("Retry-After = %d 秒,应接近窗口剩余(>540 秒),精确等待时间未生效", sec)
	}
}
