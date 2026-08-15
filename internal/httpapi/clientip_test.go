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
		t.Fatalf("clientIP = %q, want first valid forwarded IP", got)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(req); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want remote fallback", got)
	}
}
