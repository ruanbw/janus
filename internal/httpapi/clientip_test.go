package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustsXFFOnlyFromPrivatePeer(t *testing.T) {
	public := httptest.NewRequest("GET", "/", nil)
	public.RemoteAddr = "203.0.113.9:1234"
	public.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(public); got != "203.0.113.9" {
		t.Fatalf("clientIP(public peer) = %q, want remote addr", got)
	}

	private := httptest.NewRequest("GET", "/", nil)
	private.RemoteAddr = "127.0.0.1:1234"
	private.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(private); got != "198.51.100.7" {
		t.Fatalf("clientIP(private peer) = %q, want forwarded addr", got)
	}
}

func TestClientIPIgnoresMalformedForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip, 198.51.100.7")
	if got := clientIP(req); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want last valid forwarded IP", got)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(req); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want remote fallback", got)
	}
}

// 客户端自带 X-Forwarded-For 时必须取代理追加的**最右**一段。
// 反向代理(Caddy 2 默认)是追加而非覆盖,取最左等于采信访客自选值:
// 注册/登录限流可绕过、visits.ip 与国家码被污染、规则的 ip/country 条件可规避。
func TestClientIPIgnoresClientSuppliedForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234" // 经内网反代(Caddy)
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 198.51.100.7")
	if got := clientIP(req); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want 代理追加的最右一段 198.51.100.7, got 访客自选值 %q", got, got)
	}
}
