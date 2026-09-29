package rules

// 访客画像提取的单元测试:纯字符串判定,不需要数据库。

import (
	"net/http"
	"net/http/httptest"
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
