// 访客画像提取:把一次访问(一个 *http.Request)抽成规则可判定的 13 个字段(spec D5)。
//
// 字段只收敛到"后端从请求即可真实求值"的集合:不把拿不到数据的字段落库,
// 否则规则就是"能配不能跑"的假能力。缺数据源的字段的处理方式:
//   - country / asn:读请求上下文里的地理值(GeoIP/ASN 未接入,当前恒空),
//     空值恒不命中——绝不因为"取不到"而默认放行或默认拦截;
//   - 被移出 v1 的字段(region / city / tz / screen / tls(JA3) / canvas / cookie):
//     缺 GeoIP 粒度、JS 探针与 JA3 采集,一律不在 Fact 上出现。
//     将来接回来时:地理粒度补 mmdb 查询、screen/canvas 补 JS 探针回传(需新增一张
//     指纹表并改写求值入口的入参)、tls 补 JA3 采集,tz 走 JS 探针。
//
// 判定全部是轻量字符串匹配,不引入任何新依赖:UA 只区分到"够用"的粒度
// (devtype/os/browser 四种浏览器 + 其他),不做完整 UA 解析。
package rules

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// 条件字段(spec D5 的 13 个,库内以字符串落库)。
const (
	FieldIP      = "ip"      // 请求来源 IP,支持 CIDR
	FieldIPAttr  = "ipattr"  // private / loopback / linklocal
	FieldCountry = "country" // 恒空:GeoIP 未接入
	FieldASN     = "asn"     // 恒空:ASN 未接入
	FieldLang    = "lang"    // Accept-Language 首标签
	FieldRef     = "ref"     // Referer 主机名
	FieldUTM     = "utm"     // utm_source 查询参数
	FieldUA      = "ua"      // User-Agent 原串
	FieldDevType = "devtype" // bot / mobile / tablet / desktop
	FieldOS      = "os"      // iOS / Android / Windows / macOS / Linux / 其他
	FieldBrowser = "browser" // Chrome / Safari / Firefox / Edge / 其他
	FieldPath    = "path"    // 请求路径,如 /abc
	FieldDomain  = "domain"  // 请求 Host
)

// IP 属性取值(ipattr 字段)。
const (
	IPAttrPrivate   = "private"
	IPAttrLoopback  = "loopback"
	IPAttrLinkLocal = "linklocal"
)

// 设备类型取值(devtype 字段)。
const (
	DevTypeBot     = "bot"
	DevTypeMobile  = "mobile"
	DevTypeTablet  = "tablet"
	DevTypeDesktop = "desktop"
)

// Fact 一次访问的访客画像。字段值全部是"字符串原样/归一后"的形态,
// 求值期不再做任何解析(解析都在预编译期完成)。
type Fact struct {
	IP      string // 来源 IP(字符串,已按可信代理口径解析)
	IPAttr  string // private / loopback / linklocal,IP 非法时为空
	Country string // 恒空:GeoIP 未接入
	ASN     string // 恒空:ASN 未接入
	Lang    string // Accept-Language 首标签(小写)
	Ref     string // Referer 主机名(小写,不含端口);无主机名时为空
	UTM     string // utm_source 查询参数(原样)
	UA      string // User-Agent 原串
	DevType string // bot / mobile / tablet / desktop
	OS      string // iOS / Android / Windows / macOS / Linux / 其他
	Browser string // Chrome / Safari / Firefox / Edge / 其他
	Path    string // 请求路径
	Domain  string // 请求 Host(小写,不含端口)

	// netIP 是 IP 的解析结果(随画像解析一次):求值期每条用到 ip 的条件都会
	// 拿访客 IP 去比预编译好的 IP/CIDR 集合,每次都重新 ParseIP 在规则多时很贵。
	netIP net.IP

	// Seen 是"重复出现"运算符(duplicated)的计数来源,键见 SeenKey。
	// 当前平台没有任何指纹/计数器数据源,所以恒为 nil,该运算符恒不命中;
	// 将来接上"同一指纹最近 N 次出现次数"的滑动窗口后,调用方把计数填进来即可,
	// 求值逻辑不需要改。
	Seen map[string]int
}

// geoKey 地理值的请求上下文键(未注入时 country/asn 恒空)。
type geoKey struct{}

// Geo 一次访问的地理值。当前无数据源,仅作为注入通道存在。
type Geo struct {
	Country string
	ASN     string
}

// WithGeo 把地理值挂到请求上下文(接入 GeoIP/ASN 后由跳转链路调用)。
// 现在没有任何调用方:接入前 country/asn 恒空,依赖它们的条件恒不命中。
func WithGeo(ctx context.Context, country, asn string) context.Context {
	return context.WithValue(ctx, geoKey{}, Geo{Country: country, ASN: asn})
}

// SeenKey 生成 Fact.Seen 的键(调用方与本包用同一份键空间)。
func SeenKey(field, value string) string { return field + "\x00" + value }

// value 按字段名取值(命中为 false 表示"该值不可用/不存在")。
// 空值必须由求值侧当作"恒不命中"处理:GeoIP 之类取不到数据时,
// 既不能当成"匹配"也不能当成"不匹配的反面"(见 eval.go 的关键不变式)。
func (f *Fact) value(field string) (string, bool) {
	switch field {
	case FieldIP:
		return f.IP, f.IP != ""
	case FieldIPAttr:
		return f.IPAttr, f.IPAttr != ""
	case FieldCountry:
		return f.Country, f.Country != ""
	case FieldASN:
		return f.ASN, f.ASN != ""
	case FieldLang:
		return f.Lang, f.Lang != ""
	case FieldRef:
		return f.Ref, f.Ref != ""
	case FieldUTM:
		return f.UTM, f.UTM != ""
	case FieldUA:
		return f.UA, f.UA != ""
	case FieldDevType:
		return f.DevType, f.DevType != ""
	case FieldOS:
		return f.OS, f.OS != ""
	case FieldBrowser:
		return f.Browser, f.Browser != ""
	case FieldPath:
		return f.Path, f.Path != ""
	case FieldDomain:
		return f.Domain, f.Domain != ""
	}
	return "", false
}

// WithIP 返回一份改写了来源 IP 的画像。
// 跳转链路建议用它:把"已经写进访问明细的那个 IP"传给求值,
// 保证访问明细与规则裁决看到同一个来源 IP,不会因为解析口径不同而对不上。
func (f Fact) WithIP(ip string) Fact {
	f.IP = ip
	f.netIP = net.ParseIP(ip)
	f.IPAttr = ipAttr(f.IP)
	return f
}

// parsedIP 取访客 IP 的解析结果(解析一次即缓存)。
// Fact 由调用方手工构造时 netIP 为空,按需解析。
func (f *Fact) parsedIP() net.IP {
	if f.netIP == nil {
		f.netIP = net.ParseIP(f.IP)
	}
	return f.netIP
}

// FromRequest 从请求抽出访客画像。
// 纯内存:只读头、URL 与 RemoteAddr,不做任何 IO。
func FromRequest(r *http.Request) Fact {
	f := Fact{
		IP:   sourceIP(r),
		UA:   r.UserAgent(),
		Path: r.URL.Path,
	}
	f.netIP = net.ParseIP(f.IP)
	f.IPAttr = ipAttr(f.IP)
	f.Lang = firstLangTag(r.Header.Get("Accept-Language"))
	f.Ref = refererHost(r.Header.Get("Referer"))
	f.Domain = hostOnly(r.Host)
	f.UTM = utmSource(r.URL.RawQuery)
	f.DevType, f.OS, f.Browser = uaFacts(f.UA)
	if geo, ok := r.Context().Value(geoKey{}).(Geo); ok {
		f.Country = geo.Country
		f.ASN = geo.ASN
	}
	return f
}

// sourceIP 解析请求来源 IP:仅当直连来源是内网/回环(即部署前置反代)时才信任
// X-Forwarded-For 首段,公网直连时忽略该头,防止访客伪造来源 IP 骗过规则。
// 口径与 httpapi 写入访问明细的 clientIP 保持一致(同一处判定,避免明细与裁决对不上)。
func sourceIP(r *http.Request) string {
	peer := net.ParseIP(hostOnly(r.RemoteAddr))
	if peer != nil && (peer.IsLoopback() || peer.IsPrivate()) {
		for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
			if ip := net.ParseIP(strings.TrimSpace(part)); ip != nil {
				return ip.String()
			}
		}
	}
	return hostOnly(r.RemoteAddr)
}

// hostOnly 去掉端口并小写(Host/RemoteAddr 通用)。
func hostOnly(h string) string {
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	} else {
		h = strings.Trim(h, "[]")
	}
	return strings.ToLower(strings.TrimSpace(h))
}

// ipAttr 由 IP 判定属性(私网/回环/链路本地);IP 非法时返回空。
func ipAttr(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	switch {
	case parsed.IsLoopback():
		return IPAttrLoopback
	case parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast():
		return IPAttrLinkLocal
	case parsed.IsPrivate():
		return IPAttrPrivate
	}
	return ""
}

// firstLangTag 取 Accept-Language 的首个标签并小写化
// ("zh-CN,zh;q=0.9" → "zh-cn");无标签时返回空。
func firstLangTag(header string) string {
	first := header
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	return strings.ToLower(strings.TrimSpace(first))
}

// utmSource 从查询串里取 utm_source。
// 不走 url.ParseQuery:那会把全部查询参数解析成 map,而这里只需要一个参数——
// 在每次访问都要跑的热路径上,少一次整串解析就是少一份分配。
func utmSource(rawQuery string) string {
	const key = "utm_source="
	for rawQuery != "" {
		pair := rawQuery
		if i := strings.IndexByte(rawQuery, '&'); i >= 0 {
			pair, rawQuery = rawQuery[:i], rawQuery[i+1:]
		} else {
			rawQuery = ""
		}
		if !strings.HasPrefix(pair, key) {
			continue
		}
		v := pair[len(key):]
		if unescaped, err := url.QueryUnescape(v); err == nil {
			return unescaped
		}
		return v
	}
	return ""
}

// refererHost 取 Referer 的主机名(小写,不含端口)。
// 解析不出主机名就返回空:ref 字段的语义是"来源主机名",
// 拿不到就当没有来源,不该把整条 Referer 原串塞进去误命中条件。
// 手工切主机名而不是 url.Parse:在每次访问都要跑的热路径上,
// 为一个 Referer 分配一个 url.URL 不划算。取值口径与 url.Parse(...).Hostname() 一致。
func refererHost(ref string) string {
	if ref == "" {
		return ""
	}
	i := strings.Index(ref, "://")
	if i <= 0 {
		// 没有 scheme(或 scheme 为空)就没有主机名(与 url.Parse 的取值一致)
		return ""
	}
	rest := ref[i+3:]
	for j := 0; j < len(rest); j++ {
		if c := rest[j]; c == '/' || c == '?' || c == '#' {
			rest = rest[:j]
			break
		}
	}
	if at := strings.LastIndexByte(rest, '@'); at >= 0 { // 去掉 user:pass@
		rest = rest[at+1:]
	}
	return hostOnly(rest)
}

// botTokens 命中即判为爬虫。覆盖常见搜索引擎/爬虫/监控 UA。
var botTokens = []string{
	"bot", "spider", "crawler", "slurp", "curl/", "wget/", "python-requests",
	"python-urllib", "go-http-client", "headlesschrome", "phantomjs",
	"facebookexternalhit", "bingpreview", "ahrefs", "semrush", "mj12", "dotbot",
	"petalbot", "yandex", "baiduspider", "duckduckbot",
}

// tabletTokens 命中即判为平板。
var tabletTokens = []string{
	"ipad", "tablet", "kindle", "silk", "playbook", "sm-t", "nexus 7", "nexus 9", "nexus 10",
}

// mobileTokens 命中即判为手机。
var mobileTokens = []string{
	"mobile", "iphone", "ipod", "android", "windows phone", "blackberry",
	"opera mini", "iemobile", "webos",
}

// uaFacts 一次访问的 UA 三个判定结果。
// 三个判定共用一次小写化:UA 通常上百字符,每次访问都做三次 ToLower 是白花的开销。
func uaFacts(ua string) (dev, osName, browser string) {
	if ua == "" {
		return "", "", ""
	}
	lower := strings.ToLower(ua)
	return devTypeOf(lower), osOf(lower), browserOf(lower)
}

// devType 由 UA 判定设备类型。判为爬虫优先于其他类型:
// 爬虫常伪装成桌面 UA,但租户的风控意图是先把它摘出去。
// UA 为空时返回空(不猜):把"没有 UA"直接判成爬虫会让所有 API 客户端被误伤。
func devType(ua string) string {
	if ua == "" {
		return ""
	}
	return devTypeOf(strings.ToLower(ua))
}

func devTypeOf(lower string) string {
	if containsAny(lower, botTokens) {
		return DevTypeBot
	}
	if containsAny(lower, tabletTokens) {
		return DevTypeTablet
	}
	if containsAny(lower, mobileTokens) {
		return DevTypeMobile
	}
	return DevTypeDesktop
}

// osTokens 由 UA 判定操作系统,按顺序命中即返回。
// 抽成包级变量是为了不每次调用都构造切片(在跳转热路径上)。
var (
	osIOS     = []string{"iphone", "ipad", "ipod", "ios"}
	osAndroid = []string{"android"}
	osWindows = []string{"windows nt", "windows phone", "win64", "win32", "windows"}
	osMac     = []string{"mac os x", "macintosh", "macos"}
	osLinux   = []string{"linux", "x11"}
)

// osName 由 UA 判定操作系统。UA 为空返回空;不认识的操作系统归"其他"
// (不是"未知"——未知必须恒不命中,才能让"空值恒不命中"这条不变式保持干净)。
func osName(ua string) string {
	if ua == "" {
		return ""
	}
	return osOf(strings.ToLower(ua))
}

func osOf(lower string) string {
	switch {
	case containsAny(lower, osIOS):
		return "iOS"
	case containsAny(lower, osAndroid):
		return "Android"
	case containsAny(lower, osWindows):
		return "Windows"
	case containsAny(lower, osMac):
		return "macOS"
	case containsAny(lower, osLinux):
		return "Linux"
	}
	return "其他"
}

// edgeTokens / firefoxTokens / chromeTokens / safariTokens 按顺序判定浏览器。
// 顺序有意义:Edge 的 UA 里也含 Chrome/Safari 字样,Firefox 的 UA 里也含 Safari。
var (
	edgeTokens    = []string{"edg/", "edga/", "edgios/", "edge/"}
	firefoxTokens = []string{"firefox/", "fxios/"}
	chromeTokens  = []string{"crios/", "chrome/", "chromium/"}
	safariTokens  = []string{"safari/"}
	// chromiumForks 是"内核像 Chrome 但浏览器不是 Chrome"的一票否决名单。
	// 认它们是为了不撒谎:租户配 browser=Chrome 时不该把 Opera/Brave 误伤进来,
	// 判不准的一律归"其他"。
	chromiumForks = []string{"opr/", "opera", "vivaldi", "brave/", "yabrowser",
		"samsungbrowser", "ucbrowser", "quark/", "miuibrowser", "heytapbrowser", "electron"}
)

// browserName 由 UA 判定浏览器(Chrome / Safari / Firefox / Edge / 其他)。
func browserName(ua string) string {
	if ua == "" {
		return ""
	}
	return browserOf(strings.ToLower(ua))
}

func browserOf(lower string) string {
	if containsAny(lower, edgeTokens) {
		return "Edge"
	}
	if containsAny(lower, firefoxTokens) {
		return "Firefox"
	}
	if containsAny(lower, chromeTokens) && !containsAny(lower, chromiumForks) {
		return "Chrome"
	}
	if containsAny(lower, safariTokens) && !containsAny(lower, chromiumForks) {
		return "Safari"
	}
	return "其他"
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
