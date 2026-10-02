// 访客画像提取:把一次访问(一个 *http.Request)抽成规则可判定的 13 个字段(spec D5)。
//
// 字段只收敛到"后端从请求即可真实求值"的集合:不把拿不到数据的字段落库,
// 否则规则就是"能配不能跑"的假能力。缺数据源的字段的处理方式:
//   - country:读请求上下文里的地理值(跳转链路用离线 ip2region 库按访客 IP 解析,
//     值是 ISO 3166-1 alpha-2 国家码);asn:当前没有 ASN 数据源,恒空。
//     取不到值一律恒不命中——绝不因为"取不到"而默认放行或默认拦截;
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
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

// 条件字段(spec D5 的 13 个,库内以字符串落库)。
const (
	FieldIP      = "ip"      // 请求来源 IP,支持 CIDR
	FieldIPAttr  = "ipattr"  // private / loopback / linklocal
	FieldCountry = "country" // ISO 3166-1 alpha-2 国家码,如 US / CN;取不到时为空
	FieldASN     = "asn"     // 当前无 ASN 数据源,恒空
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
	Country string // ISO 3166-1 alpha-2 国家码(取不到时为空)
	ASN     string // 自治系统号(当前无数据源,恒空)
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

// geoKey 地理值的请求上下文键(未注入时 country/asn 为空)。
type geoKey struct{}

// Geo 一次访问的地理值。值由 internal/geo 解析后注入(ADR 0009):
// 求值期只读这两个字符串,不做任何 IO——按 IP 查库的活已经在注入前干完了。
type Geo struct {
	Country string
	ASN     string
}

// WithGeo 把地理值挂到请求上下文,由跳转链路在构造 Fact 之前调用。
// 注入空值等价于不注入(取不到的 IP 就该恒不命中),所以调用方不需要
// 区分"查到了"和"没查到"两种情况,拿到什么填什么即可。
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

// VisitorContext 访客画像上下文抽象接口。
// 支持零分配与惰性求值:仅在规则引擎真正访问字段时才解析对应数据。
type VisitorContext interface {
	ClientIP() netip.Addr
	Field(name string) (string, bool)
}

var (
	_ VisitorContext = (*LazyVisitorContext)(nil)
	_ VisitorContext = (*Fact)(nil)
	_ VisitorContext = Fact{}
)

// ClientIP 返回请求来源 IP(netip.Addr 16 字节值类型,零堆分配)。
func (f Fact) ClientIP() netip.Addr {
	if f.netIP != nil {
		if addr, ok := netip.AddrFromSlice(f.netIP); ok {
			return addr.Unmap()
		}
	}
	if addr, err := netip.ParseAddr(f.IP); err == nil {
		return addr.Unmap()
	}
	return netip.Addr{}
}

// Field 按字段名获取求值所需的属性值。未命中或无数据时 ok 为 false。
func (f Fact) Field(name string) (string, bool) {
	return f.value(name)
}

const (
	flagIPAttr uint32 = 1 << iota
	flagLang
	flagRef
	flagUTM
	flagUA
	flagUAParsed
	flagPath
	flagDomain
)

// LazyVisitorContext 惰性求值的访客画像上下文。
// 通过 bitmask 记录各字段的解析状态,仅在规则引擎真正访问字段时才执行解析。
// 在多数无需 UA/设备判定的规则场景下,UA 小写化与特征词扫描完全跳过。
type LazyVisitorContext struct {
	req     *http.Request
	country string
	asn     string

	flags uint32

	clientIP netip.Addr
	ipStr    string
	ipAttr   string
	lang     string
	ref      string
	utm      string
	ua       string
	devType  string
	os       string
	browser  string
	path     string
	domain   string
}

var visitorContextPool = sync.Pool{
	New: func() any {
		return &LazyVisitorContext{}
	},
}

// AcquireVisitorContext 从对象池获取一个惰性访客上下文。
// 调用方需在请求处理完毕后通过 ReleaseVisitorContext 归还。
func AcquireVisitorContext(r *http.Request, country, asn string) *LazyVisitorContext {
	ctx := visitorContextPool.Get().(*LazyVisitorContext)
	ctx.req = r
	ctx.country = country
	ctx.asn = asn
	return ctx
}

// ReleaseVisitorContext 重置并将惰性上下文归还到对象池。
func ReleaseVisitorContext(ctx *LazyVisitorContext) {
	if ctx == nil {
		return
	}
	ctx.reset()
	visitorContextPool.Put(ctx)
}

func (c *LazyVisitorContext) reset() {
	*c = LazyVisitorContext{}
}

// WithIP 注入来源 IP —— **这是唯一的注入点**。
//
// 为什么本包不自己解析 X-Forwarded-For:可信代理的判定口径只有一处(httpapi 的
// clientIP,按 trusted-proxy 白名单取 XFF 最右段)。本包曾并行存在第二套实现
// (取 XFF **最左**段),两处口径相反,而 fields.go 的注释却写着"与 httpapi 保持一致"
// —— 那是假的:取最左段意味着访客可以自己往 XFF 前面塞一个 IP 来伪造来源 IP。
// 两个生产调用点都显式调了 WithIP,所以这个洞没被触发;但任何忘了调的调用方
// 都会静默恢复这条伪造通道。
//
// 删掉第二套实现之后,忘记调 WithIP 的后果是"取不到来源 IP"→ ip/ipattr 恒不命中
// (fail-safe:不放行也不误拦),而不是"用一个可能是伪造的值去裁决"。
func (c *LazyVisitorContext) WithIP(ip string) *LazyVisitorContext {
	c.ipStr = ip
	if addr, err := netip.ParseAddr(ip); err == nil {
		c.clientIP = addr.Unmap()
	} else {
		c.clientIP = netip.Addr{}
	}
	c.ipAttr = ipAttrFromAddr(c.clientIP)
	c.flags |= flagIPAttr
	return c
}

// UAParsed 返回 UA 是否已执行过设备/系统/浏览器解析(测试与观测用)。
func (c *LazyVisitorContext) UAParsed() bool {
	return c.flags&flagUAParsed != 0
}

func (c *LazyVisitorContext) rawUA() string {
	if c.flags&flagUA == 0 {
		c.flags |= flagUA
		if c.req != nil {
			c.ua = c.req.UserAgent()
		}
	}
	return c.ua
}

func (c *LazyVisitorContext) ensureUAParsed() {
	if c.flags&flagUAParsed != 0 {
		return
	}
	c.flags |= flagUAParsed
	ua := c.rawUA()
	if ua == "" {
		return
	}
	c.devType, c.os, c.browser = uaFacts(ua)
}

// ClientIP 返回**已注入**的来源 IP;没注入过就是无效地址。
//
// 它不做任何解析:来源 IP 的可信口径由 httpapi 决定后经 WithIP 传进来
// (见 WithIP 的注释)。求值期零解析是刻意的,热路径上不做第二次 IP 判定。
func (c *LazyVisitorContext) ClientIP() netip.Addr {
	return c.clientIP
}

// Field 按字段名获取求值所需的属性值。未命中或无数据时 ok 为 false。
func (c *LazyVisitorContext) Field(name string) (string, bool) {
	switch name {
	case FieldIP:
		if c.ipStr == "" {
			ip := c.ClientIP()
			if ip.IsValid() {
				c.ipStr = ip.String()
			} else if ip = remoteAddrIP(c.req); ip.IsValid() {
				// 兜底只取 TCP 直连方(RemoteAddr),不碰 X-Forwarded-For:
				// 直连方不是访客能伪造的,而 XFF 是。这里也不做 loopback/private
				// 的信任判定 —— 那是调用方(已按 trusted-proxy 白名单做过)的事。
				// 只接受能解析成 IP 的值:ip 字段的值必须是一个 IP 字符串,
				// 解析不出来就留空(ip 恒不命中),不能塞一个 host 名去和
				// 租户写的 `ip == "xxx"` 字面量比较。
				c.ipStr = ip.String()
			}
		}
		return c.ipStr, c.ipStr != ""

	case FieldIPAttr:
		if c.flags&flagIPAttr == 0 {
			c.flags |= flagIPAttr
			c.ipAttr = ipAttrFromAddr(c.ClientIP())
		}
		return c.ipAttr, c.ipAttr != ""

	case FieldCountry:
		return c.country, c.country != ""

	case FieldASN:
		return c.asn, c.asn != ""

	case FieldLang:
		if c.flags&flagLang == 0 {
			c.flags |= flagLang
			if c.req != nil {
				c.lang = firstLangTag(c.req.Header.Get("Accept-Language"))
			}
		}
		return c.lang, c.lang != ""

	case FieldRef:
		if c.flags&flagRef == 0 {
			c.flags |= flagRef
			if c.req != nil {
				c.ref = refererHost(c.req.Header.Get("Referer"))
			}
		}
		return c.ref, c.ref != ""

	case FieldUTM:
		if c.flags&flagUTM == 0 {
			c.flags |= flagUTM
			if c.req != nil && c.req.URL != nil {
				c.utm = utmSource(c.req.URL.RawQuery)
			}
		}
		return c.utm, c.utm != ""

	case FieldUA:
		ua := c.rawUA()
		return ua, ua != ""

	case FieldDevType:
		c.ensureUAParsed()
		return c.devType, c.devType != ""

	case FieldOS:
		c.ensureUAParsed()
		return c.os, c.os != ""

	case FieldBrowser:
		c.ensureUAParsed()
		return c.browser, c.browser != ""

	case FieldPath:
		if c.flags&flagPath == 0 {
			c.flags |= flagPath
			if c.req != nil && c.req.URL != nil {
				c.path = c.req.URL.Path
			}
		}
		return c.path, c.path != ""

	case FieldDomain:
		if c.flags&flagDomain == 0 {
			c.flags |= flagDomain
			if c.req != nil {
				c.domain = hostOnly(c.req.Host)
			}
		}
		return c.domain, c.domain != ""
	}
	return "", false
}

// remoteAddrIP 取 TCP 直连方地址。解析不出来就返回无效地址(调用方据此当作"取不到")。
// 只认 RemoteAddr,**不**读 X-Forwarded-For —— 见 WithIP 的注释。
func remoteAddrIP(r *http.Request) netip.Addr {
	if r == nil {
		return netip.Addr{}
	}
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr().Unmap()
	}
	if addr, err := netip.ParseAddr(strings.Trim(r.RemoteAddr, "[]")); err == nil {
		return addr.Unmap()
	}
	return netip.Addr{}
}

// ToFact 将惰性画像折成完整的 Fact 结构(向后兼容现有调用方与测试)。
func (c *LazyVisitorContext) ToFact() Fact {
	c.ensureUAParsed()
	ipStr, _ := c.Field(FieldIP)
	ipAttrVal, _ := c.Field(FieldIPAttr)
	lang, _ := c.Field(FieldLang)
	ref, _ := c.Field(FieldRef)
	utm, _ := c.Field(FieldUTM)
	ua, _ := c.Field(FieldUA)
	path, _ := c.Field(FieldPath)
	domain, _ := c.Field(FieldDomain)

	f := Fact{
		IP:      ipStr,
		IPAttr:  ipAttrVal,
		Country: c.country,
		ASN:     c.asn,
		Lang:    lang,
		Ref:     ref,
		UTM:     utm,
		UA:      ua,
		DevType: c.devType,
		OS:      c.os,
		Browser: c.browser,
		Path:    path,
		Domain:  domain,
	}
	if ipStr != "" {
		f.netIP = net.ParseIP(ipStr)
	}
	return f
}

// WithIP 返回一份改写了来源 IP 的画像。
// 跳转链路建议用它:把"已经写进访问明细的那个 IP"传给求值,
// 保证访问明细与规则裁决看到同一个来源 IP,不会因为解析口径不同而对不上。
// Fields 按 13 个可求值字段摊平成一张表,给诊断接口原样回显。
// 取不到数据的字段值是空串(与 Field() 的 ok=false 语义一致:空串=没有数据,不是"匹配空串")。
func (f Fact) Fields() map[string]string {
	out := make(map[string]string, 13)
	for _, name := range []string{
		FieldIP, FieldIPAttr, FieldCountry, FieldASN, FieldLang, FieldRef, FieldUTM,
		FieldUA, FieldDevType, FieldOS, FieldBrowser, FieldPath, FieldDomain,
	} {
		v, _ := f.Field(name)
		out[name] = v
	}
	return out
}

func (f Fact) WithIP(ip string) Fact {
	f.IP = ip
	f.netIP = net.ParseIP(ip)
	f.IPAttr = ipAttr(f.IP)
	return f
}

// FromRequest 从请求抽出访客画像。
// 纯内存:只读头、URL 与 RemoteAddr,不做任何 IO。
func FromRequest(r *http.Request) Fact {
	var country, asn string
	if geo, ok := r.Context().Value(geoKey{}).(Geo); ok {
		country = geo.Country
		asn = geo.ASN
	}
	ctx := AcquireVisitorContext(r, country, asn)
	defer ReleaseVisitorContext(ctx)
	return ctx.ToFact()
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

// ipAttrFromAddr 根据 netip.Addr 判断属性(私网/回环/链路本地)。
func ipAttrFromAddr(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	addr = addr.Unmap()
	switch {
	case addr.IsLoopback():
		return IPAttrLoopback
	case addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast():
		return IPAttrLinkLocal
	case addr.IsPrivate():
		return IPAttrPrivate
	}
	return ""
}

// ipAttr 由 IP 判定属性(私网/回环/链路本地);IP 非法时返回空。
func ipAttr(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	return ipAttrFromAddr(addr)
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

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
