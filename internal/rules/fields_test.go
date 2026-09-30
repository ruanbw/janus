package rules

// 访客画像提取的单元测试:纯字符串判定,不需要数据库。

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

// mustRequest 造一个带 UA / Referer / Host 的请求。
func mustRequest(t testing.TB, url, referer, ua string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.Header.Set("User-Agent", ua)
	if referer != "" {
		r.Header.Set("Referer", referer)
	}
	return r
}

func TestFromRequestFields(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	r := mustRequest(t, "https://Shop.Example.com:8443/abc?utm_source=wechat&x=1",
		"https://REF.example.com/page?a=b", ua)
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	f := FromRequest(r)
	checks := []struct{ field, got, want string }{
		{FieldIP, f.IP, "203.0.113.9"},
		{FieldIPAttr, f.IPAttr, ""},
		{FieldLang, f.Lang, "zh-cn"},
		{FieldRef, f.Ref, "ref.example.com"},
		{FieldUTM, f.UTM, "wechat"},
		{FieldUA, f.UA, ua},
		{FieldDevType, f.DevType, DevTypeDesktop},
		{FieldOS, f.OS, "macOS"},
		{FieldBrowser, f.Browser, "Chrome"},
		{FieldPath, f.Path, "/abc"},
		{FieldDomain, f.Domain, "shop.example.com"},
		{FieldCountry, f.Country, ""},
		{FieldASN, f.ASN, ""},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	// 缺失的头不产生值(空值恒不命中)
	bare := FromRequest(mustRequest(t, "http://x.test/p", "", ""))
	if bare.UTM != "" || bare.Lang != "" || bare.Ref != "" {
		t.Fatalf("无头时不应有值: %+v", bare)
	}
	if bare.DevType != "" || bare.OS != "" || bare.Browser != "" {
		t.Fatalf("空 UA 不该判出设备/系统/浏览器: %+v", bare)
	}
	// 13 个字段之外取不到值
	if _, ok := bare.value("region"); ok {
		t.Fatal("v1 不该有 region 字段")
	}
}

func TestIPAttr(t *testing.T) {
	cases := []struct{ ip, want string }{
		{"10.1.2.3", IPAttrPrivate},
		{"192.168.0.1", IPAttrPrivate},
		{"172.16.0.1", IPAttrPrivate},
		{"127.0.0.1", IPAttrLoopback},
		{"::1", IPAttrLoopback},
		{"169.254.1.1", IPAttrLinkLocal},
		{"fe80::1", IPAttrLinkLocal},
		{"203.0.113.9", ""},
		{"fd00::1", IPAttrPrivate},
		{"不是IP", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := ipAttr(c.ip); got != c.want {
			t.Errorf("ipAttr(%q) = %q, want %q", c.ip, got, c.want)
		}
	}
}

// 内网反代才信任 X-Forwarded-For:公网直连时忽略该头,防止访客伪造来源骗过规则。
func TestSourceIPTrustsForwardedOnlyFromPrivatePeer(t *testing.T) {
	pub := mustRequest(t, "http://x.test/p", "", "ua")
	pub.RemoteAddr = "203.0.113.9:1234"
	pub.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := FromRequest(pub).IP; got != "203.0.113.9" {
		t.Fatalf("公网直连 IP = %q, want RemoteAddr", got)
	}
	behind := mustRequest(t, "http://x.test/p", "", "ua")
	behind.RemoteAddr = "127.0.0.1:1234"
	behind.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.9")
	if got := FromRequest(behind).IP; got != "198.51.100.7" {
		t.Fatalf("内网反代 IP = %q, want XFF 首段", got)
	}
	// 非法 XFF 回退 RemoteAddr
	bad := mustRequest(t, "http://x.test/p", "", "ua")
	bad.RemoteAddr = "127.0.0.1:1234"
	bad.Header.Set("X-Forwarded-For", "垃圾")
	if got := FromRequest(bad).IP; got != "127.0.0.1" {
		t.Fatalf("非法 XFF 时 IP = %q, want 127.0.0.1", got)
	}
}

func TestUADetection(t *testing.T) {
	cases := []struct {
		name                 string
		ua                   string
		dev, osName, browser string
	}{
		{"桌面 Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", DevTypeDesktop, "Windows", "Chrome"},
		{"桌面 Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", DevTypeDesktop, "macOS", "Safari"},
		{"桌面 Firefox", "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", DevTypeDesktop, "Linux", "Firefox"},
		{"Edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0", DevTypeDesktop, "Windows", "Edge"},
		{"iPhone Safari", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", DevTypeMobile, "iOS", "Safari"},
		{"iPad", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", DevTypeTablet, "iOS", "Safari"},
		{"Android Chrome", "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36", DevTypeMobile, "Android", "Chrome"},
		{"爬虫伪装桌面", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", DevTypeBot, "其他", "其他"},
		{"爬虫 curl", "curl/8.1.2", DevTypeBot, "其他", "其他"},
		{"Opera 不冒充 Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 OPR/105.0.0.0", DevTypeDesktop, "Windows", "其他"},
		{"微信内置浏览器", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/604.1 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.30", DevTypeMobile, "iOS", "其他"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := devType(c.ua); got != c.dev {
				t.Errorf("devtype = %q, want %q", got, c.dev)
			}
			if got := osName(c.ua); got != c.osName {
				t.Errorf("os = %q, want %q", got, c.osName)
			}
			if got := browserName(c.ua); got != c.browser {
				t.Errorf("browser = %q, want %q", got, c.browser)
			}
		})
	}
}

func TestFirstLangTag(t *testing.T) {
	cases := []struct{ in, want string }{
		{"zh-CN,zh;q=0.9,en;q=0.8", "zh-cn"},
		{"pt-BR", "pt-br"},
		{" EN-US ; q=1.0 , de", "en-us"},
		{"", ""},
		{"  ", ""},
		{"*", "*"},
	}
	for _, c := range cases {
		if got := firstLangTag(c.in); got != c.want {
			t.Errorf("firstLangTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRefererHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://REF.example.com/path?q=1", "ref.example.com"},
		{"http://ref.example.com:8080/x", "ref.example.com"},
		{"https://user:pw@ref.example.com:443/x", "ref.example.com"},
		{"https://ref.example.com", "ref.example.com"},
		{"HTTPS://Ref.Example.COM/P", "ref.example.com"},
		{"", ""},
		{"不是URL", ""},
		{"/relative/path", ""},
		{"ref.example.com/x", ""},
	}
	for _, c := range cases {
		if got := refererHost(c.in); got != c.want {
			t.Errorf("refererHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 手工切主机名的取值必须与 url.Parse(...).Hostname() 一致(否则两边口径会漂)
	for _, in := range []string{
		"https://a.test/x", "http://a.test:8080", "https://a.test?q=1", "https://a.test#f",
		"https://a.test", "https://u:p@a.test/x", "not a url", "/p", "a.test", "://a.test",
	} {
		want := ""
		if u, err := url.Parse(in); err == nil {
			want = u.Hostname()
		}
		if got := refererHost(in); !strings.EqualFold(got, want) {
			t.Errorf("refererHost(%q) = %q, want %q(url.Parse 口径)", in, got, want)
		}
	}
}

func TestUTMSource(t *testing.T) {
	cases := []struct{ in, want string }{
		{"utm_source=wechat", "wechat"},
		{"a=1&utm_source=wechat&b=2", "wechat"},
		{"a=1&b=2", ""},
		{"", ""},
		{"utm_source=", ""},
		{"utm_source=%E5%BE%AE%E4%BF%A1", "微信"},
		{"utm_source=x&utm_source=y", "x"},
		{"xutm_source=nope", ""},
	}
	for _, c := range cases {
		if got := utmSource(c.in); got != c.want {
			t.Errorf("utmSource(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLazyVisitorContext(t *testing.T) {
	t.Run("UASkippedIfNotQueried", func(t *testing.T) {
		ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
		r := httptest.NewRequest(http.MethodGet, "https://example.com/test?utm_source=google", nil)
		r.Header.Set("User-Agent", ua)
		r.Header.Set("Accept-Language", "en-US,en;q=0.9")
		r.RemoteAddr = "203.0.113.10:1234"

		ctx := AcquireVisitorContext(r, "US", "")
		defer ReleaseVisitorContext(ctx)

		if ctx.UAParsed() {
			t.Fatal("expected UA not parsed upon acquisition")
		}

		// Querying non-device fields should not trigger UA parsing
		if country, ok := ctx.Field(FieldCountry); !ok || country != "US" {
			t.Fatalf("expected country US, got %q (ok=%v)", country, ok)
		}
		if ctx.UAParsed() {
			t.Fatal("expected UA not parsed after querying country")
		}

		if ip, ok := ctx.Field(FieldIP); !ok || ip != "203.0.113.10" {
			t.Fatalf("expected ip 203.0.113.10, got %q (ok=%v)", ip, ok)
		}
		if ctx.UAParsed() {
			t.Fatal("expected UA not parsed after querying IP")
		}

		if utm, ok := ctx.Field(FieldUTM); !ok || utm != "google" {
			t.Fatalf("expected utm google, got %q (ok=%v)", utm, ok)
		}
		if ctx.UAParsed() {
			t.Fatal("expected UA not parsed after querying UTM")
		}

		// Querying device field SHOULD trigger UA parsing
		if dev, ok := ctx.Field(FieldDevType); !ok || dev != DevTypeDesktop {
			t.Fatalf("expected devtype %s, got %q (ok=%v)", DevTypeDesktop, dev, ok)
		}
		if !ctx.UAParsed() {
			t.Fatal("expected UA parsed after querying FieldDevType")
		}

		// Subsequent queries to OS and Browser should use cached UA facts
		if os, ok := ctx.Field(FieldOS); !ok || os != "macOS" {
			t.Fatalf("expected os macOS, got %q (ok=%v)", os, ok)
		}
		if browser, ok := ctx.Field(FieldBrowser); !ok || browser != "Chrome" {
			t.Fatalf("expected browser Chrome, got %q (ok=%v)", browser, ok)
		}
	})

	t.Run("NetipAddrParsing", func(t *testing.T) {
		tests := []struct {
			name       string
			remoteAddr string
			wantIP     string
			wantAttr   string
		}{
			{"IPv4 public", "203.0.113.9:5555", "203.0.113.9", ""},
			{"IPv4 private 10.x", "10.0.0.1:80", "10.0.0.1", IPAttrPrivate},
			{"IPv4 private 192.168.x", "192.168.1.1:443", "192.168.1.1", IPAttrPrivate},
			{"IPv4 private 172.16.x", "172.16.0.1:8080", "172.16.0.1", IPAttrPrivate},
			{"IPv4 loopback", "127.0.0.1:9090", "127.0.0.1", IPAttrLoopback},
			{"IPv4 link-local", "169.254.1.1:80", "169.254.1.1", IPAttrLinkLocal},
			{"IPv6 loopback", "[::1]:1234", "::1", IPAttrLoopback},
			{"IPv6 link-local", "[fe80::1]:1234", "fe80::1", IPAttrLinkLocal},
			{"IPv6 private ULA", "[fd00::1]:1234", "fd00::1", IPAttrPrivate},
			{"IPv6 public", "[2001:db8::1]:1234", "2001:db8::1", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				r.RemoteAddr = tt.remoteAddr
				ctx := AcquireVisitorContext(r, "", "")
				defer ReleaseVisitorContext(ctx)

				addr := ctx.ClientIP()
				if !addr.IsValid() {
					t.Fatalf("ClientIP() returned invalid addr for %s", tt.remoteAddr)
				}
				wantAddr := netip.MustParseAddr(tt.wantIP)
				if addr != wantAddr {
					t.Errorf("ClientIP() = %v, want %v", addr, wantAddr)
				}
				gotAttr, _ := ctx.Field(FieldIPAttr)
				if gotAttr != tt.wantAttr {
					t.Errorf("FieldIPAttr = %q, want %q", gotAttr, tt.wantAttr)
				}
			})
		}
	})

	t.Run("XForwardedForZeroAlloc", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "203.0.113.195, 10.0.0.1")

		ctx := AcquireVisitorContext(r, "", "")
		defer ReleaseVisitorContext(ctx)

		ip := ctx.ClientIP()
		if ip.String() != "203.0.113.195" {
			t.Fatalf("expected XFF IP 203.0.113.195, got %s", ip.String())
		}

		// Warm up pool
		for i := 0; i < 5; i++ {
			c := AcquireVisitorContext(r, "", "")
			_ = c.ClientIP()
			ReleaseVisitorContext(c)
		}

		allocs := testing.AllocsPerRun(100, func() {
			c := AcquireVisitorContext(r, "", "")
			_ = c.ClientIP()
			ReleaseVisitorContext(c)
		})
		if allocs > 0 {
			t.Errorf("AcquireVisitorContext + ClientIP + ReleaseVisitorContext allocated %v allocs/op, want 0", allocs)
		}
	})

	t.Run("PoolAcquireAndRelease", func(t *testing.T) {
		r1 := httptest.NewRequest(http.MethodGet, "http://shop.example.com/p1?utm_source=fb", nil)
		r1.RemoteAddr = "127.0.0.1:80"
		r1.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
		r1.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

		ctx1 := AcquireVisitorContext(r1, "US", "AS1234")
		if country, _ := ctx1.Field(FieldCountry); country != "US" {
			t.Fatalf("expected US, got %s", country)
		}
		if asn, _ := ctx1.Field(FieldASN); asn != "AS1234" {
			t.Fatalf("expected AS1234, got %s", asn)
		}
		dev, _ := ctx1.Field(FieldDevType)
		if dev != DevTypeMobile {
			t.Fatalf("expected mobile, got %s", dev)
		}
		if !ctx1.UAParsed() {
			t.Fatal("expected UAParsed true")
		}

		ReleaseVisitorContext(ctx1)

		r2 := httptest.NewRequest(http.MethodGet, "http://blog.example.com/p2", nil)
		r2.RemoteAddr = "203.0.113.50:443"
		ctx2 := AcquireVisitorContext(r2, "CN", "AS5678")

		if ctx2.UAParsed() {
			t.Fatal("expected ctx2 to have UAParsed=false after reset from pool")
		}
		if country, _ := ctx2.Field(FieldCountry); country != "CN" {
			t.Fatalf("expected CN, got %s", country)
		}
		if asn, _ := ctx2.Field(FieldASN); asn != "AS5678" {
			t.Fatalf("expected AS5678, got %s", asn)
		}
		if ip, _ := ctx2.Field(FieldIP); ip != "203.0.113.50" {
			t.Fatalf("expected 203.0.113.50, got %s", ip)
		}

		ReleaseVisitorContext(ctx2)
	})
}
