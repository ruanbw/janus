// Package domain 承载域名生命周期、跳转相关的领域逻辑:
// 域名归属校验(DNS TXT 挑战 + A/AAAA 比对)、证书预签发探活、短码生成与校验、后台任务。
package domain

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	randv2 "math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// CodeAlphabet 短码字符集:a-zA-Z0-9 去掉易混淆字符 0/O/1/l/I(spec 决策)。
// 小写去 l,大写去 I/O,数字去 0/1,共 56 个字符。
const CodeAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// MaxCodeLen 短码最大长度(自定义短码)。
const MaxCodeLen = 64

// AutoCodeLength 自动生成短码的固定长度(租户不可配)。
const AutoCodeLength = 6

// MaxDomainDescriptionLen 域名描述最大长度(按字符计,前端 textarea maxlength 同步此值)。
const MaxDomainDescriptionLen = 200

// VerifyRecordPrefix TXT 挑战记录的主机名前缀。
//
// 归属证明要问的是"这个域名的权威 DNS 是否由提交者控制",而不是"它是否解析到本机"。
// 解析到本机只能证明流量会到这台服务器:共享 IP / anycast / CDN 回源 / 他人 CNAME
// 指向同一台机器,以及攻击者在自己控制的域下加一条 A 记录指向本机 IP,都能让 A 记录
// 检查通过 —— 而 ADR-0002 声称这道闸门正是为了防"他人把任意域名解析到本服务器"。
// 权威 DNS 里的一次性 token 只有控制该 zone 的人能发布,所以它才是归属证明。
const VerifyRecordPrefix = "_janus-verify."

// VerifyTokenBytes TXT 挑战 token 的熵(bytes),渲染成 2 倍长度 hex。
const VerifyTokenBytes = 24

// VerifyRecordName 返回该域名应发布 TXT 的主机名。
func VerifyRecordName(fqdn string) string {
	return VerifyRecordPrefix + NormalizeFQDN(fqdn)
}

// MintVerifyToken 生成一次性 TXT 挑战 token。
func MintVerifyToken() (string, error) {
	b := make([]byte, VerifyTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 TXT 挑战 token 失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NormalizeFQDN 统一域名形态:去空白、转小写、去掉根标签尾点。
//
// 入库前必须过这一道:校验时剥了尾点而入库只 TrimSpace,会让 "a.com" 与 "a.com."
// 在 UNIQUE 索引下成为两行 —— 同一个真实主机被两个租户各绑一份、各计一份配额。
// 读路径(FQDN → 域名、授权端点)也要走它,否则 Host 头带尾点时查不到已归一化的行。
func NormalizeFQDN(s string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(s)), ".")
}

// IsValidFQDN 校验域名格式:点分标签,字母/数字/连字符,标签不以连字符开头结尾。
// 允许单标签(如 localhost,开发环境 DNS 校验用)。
func IsValidFQDN(s string) bool {
	s = NormalizeFQDN(s)
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
			if !ok || (c == '-' && (i == 0 || i == len(label)-1)) {
				return false
			}
		}
	}
	return true
}

// IsReservedFQDN 判断域名是否属于平台自己:等于平台域名,或是它的子域。
//
// 原来是全等比较,于是 www.<平台域名>、mail.<平台域名> 都能绕过,而后者因为平台泛解析
// 存在,会通过 DNS 校验并被授权端点放行 —— 租户可以占住平台自己的子域。子域判断必须
// 做成后缀匹配。
func IsReservedFQDN(fqdn, platformDomain string) bool {
	fqdn = NormalizeFQDN(fqdn)
	platform := NormalizeFQDN(platformDomain)
	if platform == "" || fqdn == "" {
		return false
	}
	return fqdn == platform || strings.HasSuffix(fqdn, "."+platform)
}

// IsValidCode 校验短码合法性(字符集 + 长度 1..64)。
func IsValidCode(s string) bool {
	if s == "" || len(s) > MaxCodeLen {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune(CodeAlphabet, c) {
			return false
		}
	}
	return true
}

// GenerateCode 随机生成指定长度的短码(均匀取自 CodeAlphabet)。
func GenerateCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = CodeAlphabet[randv2.IntN(len(CodeAlphabet))]
	}
	return string(b)
}

// IsValidSlug 校验租户前缀 slug:小写字母/数字开头结尾,中间可含连字符,长度 1..63。
func IsValidSlug(s string) bool {
	if len(s) < 1 || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// reservedSlugs 不允许租户注册的 slug。
//
// 租户默认域名是 <slug>.<平台域名>,而平台自身(以及部署者日后)需要一批约定俗成的
// 子域:后台(app/admin/console)、邮件(mail/smtp/mx/autodiscover,邮件客户端会
// 按约定探测)、API/状态页/文档、DNS(ns1/ns2)等。被租户先占住后,平台要么无法再
// 使用这些名字,要么它们会以"租户短链"的身份对外服务 —— 例如 mail.<平台域名>
// 被租户拿去做跳转,或 status.<平台域名> 显示的是租户内容,都是可被利用的钓鱼面。
// 列表只做注册闸门(超管 slug 由系统内部生成,不受影响)。
var reservedSlugs = map[string]struct{}{
	"app": {}, "www": {}, "api": {}, "admin": {}, "administrator": {}, "root": {},
	"console": {}, "dashboard": {}, "portal": {}, "panel": {}, "manage": {},
	"mail": {}, "email": {}, "smtp": {}, "imap": {}, "pop": {}, "pop3": {}, "mx": {},
	"webmail": {}, "autodiscover": {}, "autoconfig": {}, "postmaster": {}, "hostmaster": {},
	"webmaster": {}, "abuse": {}, "security": {}, "noreply": {}, "no-reply": {},
	"ns": {}, "ns1": {}, "ns2": {}, "ns3": {}, "ns4": {}, "dns": {}, "ftp": {}, "sftp": {},
	"status": {}, "health": {}, "healthz": {}, "metrics": {}, "internal": {},
	"auth": {}, "login": {}, "logout": {}, "signin": {}, "signup": {}, "register": {},
	"sso": {}, "oauth": {}, "account": {}, "accounts": {}, "billing": {}, "pay": {},
	"support": {}, "help": {}, "docs": {}, "doc": {}, "blog": {}, "news": {},
	"static": {}, "assets": {}, "cdn": {}, "img": {}, "images": {}, "media": {}, "files": {},
	"dev": {}, "test": {}, "staging": {}, "stage": {}, "beta": {}, "demo": {}, "sandbox": {},
	"localhost": {}, "janus": {},
}

// IsReservedSlug slug 是否为平台保留(大小写不敏感)。
func IsReservedSlug(slug string) bool {
	_, ok := reservedSlugs[strings.ToLower(strings.TrimSpace(slug))]
	return ok
}

// IsPlatformSubdomain 判断 fqdn 是否是平台域名的(任意层级)子域,不含平台域名本身。
func IsPlatformSubdomain(fqdn, platformDomain string) bool {
	fqdn = NormalizeFQDN(fqdn)
	platform := NormalizeFQDN(platformDomain)
	if platform == "" || fqdn == "" {
		return false
	}
	return strings.HasSuffix(fqdn, "."+platform)
}

// VerifyStatus 一次归属校验的三态结果。区分开是为了给租户不同的下一步指引,
// 而不是把所有失败揉成一句"校验失败"。
type VerifyStatus string

const (
	// VerifyNeedDNS A/AAAA 未指向本机:租户先加 A/AAAA 记录。
	VerifyNeedDNS VerifyStatus = "need_dns"
	// VerifyNeedTXT A/AAAA 已指向本机,但权威 DNS 里还没有那条 TXT(或与 token 不匹配):
	// 归属未被证明,不能激活。
	VerifyNeedTXT VerifyStatus = "need_txt"
	// VerifyVerified TXT 挑战通过(且 A/AAAA 指向本机):可以激活。
	VerifyVerified VerifyStatus = "verified"
)

// VerifyResult 归属校验结果。TXTValues 是该域名当前发布的挑战记录值,只用于后台
// 诊断展示(让租户看清"我加的 TXT 到底被读成了什么")。
type VerifyResult struct {
	Status         VerifyStatus `json:"status"`
	PointsToServer bool         `json:"pointsToServer"`
	TokenMatched   bool         `json:"tokenMatched"`
	TXTValues      []string     `json:"txtValues,omitempty"`
}

// Verified 是"可以激活"的唯一判据。
func (r VerifyResult) Verified() bool { return r.Status == VerifyVerified }

// ErrDNSLookup DNS 查询本身失败(超时 / SERVFAIL / 没有可用解析器),
// 即"这一次没能得到答案",而不是"答案是否定的"。
//
// 两者必须区分开:原实现把 A/AAAA 查询错误直接丢弃、把 TXT 查询错误当成
// "没有这条记录",于是一次解析器抖动在 24h 归属复检里就等于"归属不再成立",
// 成熟域名被降级下线。调用方看到这个错误应当**跳过本轮**(不降级、不计失败),
// 下一轮再查;同步接口可以把它当作"暂时无法判定"。
// NXDOMAIN / 无记录不属于此类:那是 DNS 明确给出的否定答案,照常返回 need_dns/need_txt。
var ErrDNSLookup = errors.New("dns lookup failed")

// isDNSNotFound 判断一次 TXT 查询错误是否是"明确不存在"(NXDOMAIN / 无该类型记录)。
func isDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// dnsQueryTimeout 单次 DNS 查询的上限。校验会走同步 HTTP 路径(创建/重检)与后台
// 循环,不能让一个不响应的解析器把请求挂住。
const dnsQueryTimeout = 3 * time.Second

// DNSChecker 校验域名归属:TXT 挑战(权威判定) + A/AAAA 指向本机(前置快速失败与兜底)。
//
// 解析路径刻意不用 net.DefaultResolver:它读 /etc/hosts,而 /etc/hosts 是本机文件,
// 部署者(或任何能写它的人)加一行就能让任意域名"解析到本机",判定权就旁落到本地
// 文件上了。A/AAAA 走手写的 wire 查询(dns.go),TXT 走标准库 LookupTXT(它不查 hosts)。
// 详见 dns.go 顶部的取舍说明。
type DNSChecker struct {
	// ExpectedIP 本服务器公网 IP。
	ExpectedIP string
	// Nameservers 显式指定 DNS 服务器;为空则读 /etc/resolv.conf。
	// 单测注入假解析服务器也走这里。
	Nameservers []string
	// Timeout 单次查询上限,<=0 取 dnsQueryTimeout。
	Timeout time.Duration

	// 以下两个钩子仅供单测注入真实 DNS 之外的实现;生产路径不设置。
	lookupTXT  func(ctx context.Context, name string) ([]string, error)
	lookupAddr func(ctx context.Context, name string) ([]netip.Addr, error)

	resolverOnce sync.Once
	txtResolver  *net.Resolver
	tasksOnce    sync.Once
	tasks        *TaskQueue
}

// Tasks 返回与本校验器绑定的后台任务队列(有界并发 + single-flight)。
//
// 挂在校验器上而不是全局:一次 API 实例 = 一个校验器 = 一份队列。原先的
// `go func()` 是每个请求一个 goroutine,没有并发上限、没有去重,也没有队列上限。
func (c *DNSChecker) Tasks() *TaskQueue {
	c.tasksOnce.Do(func() {
		c.tasks = NewTaskQueue(defaultTaskWorkers, defaultTaskQueue)
	})
	return c.tasks
}

// Verify 校验域名归属,返回三态结果。
//
// 判定顺序刻意是 A/AAAA → TXT:A/AAAA 失败是最常见、也最容易自查的原因,先把它分出去
// 既能给精确指引,又省掉那些根本不会激活的域名的 TXT 查询(后台重试队列每轮都跑)。
//
// 查询本身失败(超时/SERVFAIL)时返回包裹 ErrDNSLookup 的错误,而不是一个否定的
// 三态结果:"没查到"不等于"不成立",调用方据此跳过而不是降级。
func (c *DNSChecker) Verify(ctx context.Context, fqdn, token string) (VerifyResult, error) {
	fqdn = NormalizeFQDN(fqdn)
	if !IsValidFQDN(fqdn) {
		return VerifyResult{}, fmt.Errorf("域名格式非法: %q", fqdn)
	}
	addrs, err := c.LookupAddrs(ctx, fqdn)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("%w: A/AAAA %s: %v", ErrDNSLookup, fqdn, err)
	}
	if !c.anyMatchExpected(addrs) {
		return VerifyResult{Status: VerifyNeedDNS}, nil
	}
	// 没有 token(刚创建、或挑战已被消费)就不可能证明归属 —— 这正是把激活从
	// "解析到本机"改成"发布一次性 TXT"的地方。TXT 查询放在 token 存在之后,
	// 免得为不可能成功的场景付出一次出网查询。
	if token == "" {
		return VerifyResult{Status: VerifyNeedTXT, PointsToServer: true}, nil
	}
	values, err := c.LookupTXT(ctx, VerifyRecordName(fqdn))
	if err != nil {
		if !isDNSNotFound(err) {
			return VerifyResult{}, fmt.Errorf("%w: TXT %s: %v", ErrDNSLookup, VerifyRecordName(fqdn), err)
		}
		// NXDOMAIN / 没有 TXT:明确的否定答案,走下面的 need_txt。
		values = nil
	}
	matched := false
	for _, v := range values {
		// 常量时间比较:token 是一次性的,但比较耗时不该泄露前缀信息。
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(v)), []byte(token)) == 1 {
			matched = true
			break
		}
	}
	status := VerifyNeedTXT
	if matched {
		status = VerifyVerified
	}
	return VerifyResult{
		Status:         status,
		PointsToServer: true,
		TokenMatched:   matched,
		TXTValues:      values,
	}, nil
}

// LookupTXT 查询 TXT 记录(标准库,不读 hosts)。
func (c *DNSChecker) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if c.lookupTXT != nil {
		return c.lookupTXT(ctx, name)
	}
	return c.resolverForTXT().LookupTXT(ctx, name)
}

// LookupAddrs 查询 A/AAAA 记录(wire 查询,绕开 hosts)。
func (c *DNSChecker) LookupAddrs(ctx context.Context, fqdn string) ([]netip.Addr, error) {
	if c.lookupAddr != nil {
		return c.lookupAddr(ctx, fqdn)
	}
	servers := c.Nameservers
	if len(servers) == 0 {
		servers = nameserversFromResolvConf()
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("没有可用的 DNS 服务器(/etc/resolv.conf 里没有 nameserver)")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = dnsQueryTimeout
	}
	var (
		out     []netip.Addr
		lastErr error
		ok      bool
	)
	for _, qtype := range []uint16{dnsTypeA, dnsTypeAAAA} {
		addrs, _, err := queryRecord(ctx, servers, fqdn, qtype, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		ok = true
		out = append(out, addrs...)
	}
	if !ok && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// anyMatchExpected A/AAAA 结果里是否命中本服务器 IP。
func (c *DNSChecker) anyMatchExpected(addrs []netip.Addr) bool {
	want, err := netip.ParseAddr(strings.TrimSpace(c.ExpectedIP))
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if a.Unmap() == want.Unmap() {
			return true
		}
	}
	return false
}

// txtLookupResolver 构造显式的 TXT 解析器:纯 Go 实现(不走 cgo/系统解析器),指定了
// Nameservers 时直接连第一个,避免依赖容器里的 hosts 与 nsswitch 配置。
func (c *DNSChecker) resolverForTXT() *net.Resolver {
	c.resolverOnce.Do(func() {
		r := &net.Resolver{PreferGo: true}
		if len(c.Nameservers) > 0 {
			addr := net.JoinHostPort(c.Nameservers[0], "53")
			r.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: dnsQueryTimeout}).DialContext(ctx, network, addr)
			}
		}
		c.txtResolver = r
	})
	return c.txtResolver
}

// NormalizeEmail 邮箱归一化:去空白、转小写。登录/注册/找回/重置共用同一个口径。
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
