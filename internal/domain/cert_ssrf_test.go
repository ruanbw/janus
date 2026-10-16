package domain

// 探活目标地址的准入判定测试(单测校验函数本身,不依赖网络)。

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// 私网 / 回环 / 链路本地 / CGNAT / 多播 / 未指定地址一律拒绝。
func TestProbeIPAllowedRejectsInternalAddresses(t *testing.T) {
	refused := []string{
		"127.0.0.1",       // 本机管理口
		"127.1.2.3",       // 整个 127/8 都是回环
		"::1",             // IPv6 回环
		"10.1.2.3",        // RFC1918
		"172.16.0.1",      // RFC1918
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // 云元数据(链路本地)
		"169.254.0.1",
		"fe80::1",         // IPv6 链路本地
		"100.64.0.1",      // CGNAT(RFC 6598)
		"100.100.100.200", // 云元数据(CGNAT 段内)
		"fd00::1",         // IPv6 ULA
		"0.0.0.0",         // 未指定
		"::",
		"224.0.0.1", // 多播
		"::ffff:10.0.0.1",
		"::ffff:127.0.0.1",
	}
	for _, s := range refused {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if probeIPAllowed(addr) {
			t.Errorf("probeIPAllowed(%s) = true, want false:探活会被引到内网/本机", s)
		}
	}
}

// 公网单播地址必须放行(否则探活被一刀切死,证书永远签不出来)。
func TestProbeIPAllowedAllowsPublicAddresses(t *testing.T) {
	allowed := []string{
		"203.0.113.7", // TEST-NET-3:单测里当"公网 IP"用
		"8.8.8.8",
		"2606:4700:4700::1111",
	}
	for _, s := range allowed {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if !probeIPAllowed(addr) {
			t.Errorf("probeIPAllowed(%s) = false, want true", s)
		}
	}
}

// 拨号器对真实 loopback 监听地址必须直接拒绝(不建立连接)。
func TestProbeDialContextRejectsLoopbackListener(t *testing.T) {
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	if _, err := probeDialContext(context.Background(), "tcp", srv.Listener.Addr().String()); err == nil {
		t.Fatal("probeDialContext 允许连到 loopback 监听地址")
	}
}
