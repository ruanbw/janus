package domain

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// ProbeCert 通过 HTTPS 访问域名,触发 Caddy on-demand TLS 授权签发。
//
// 判定不是"握手成功"而是"对端实际出示的叶子证书覆盖了这个 FQDN":域名被指到
// 另一台同样跑 TLS 的主机(共享 IP 的邻居、CDN 的默认站点、别人的泛解析兜底)也会
// 握手成功,而 cert_status 一旦被误判成 issued,worker 就再也不探它 —— 后台会一直
// 显示"已签发",Caddy 手里却没有证书。InsecureSkipVerify 下拿到的正是对端真实证书
// (不走系统信任链),所以直接比对 SAN 就够,不需要验签链(开发环境是本地 CA)。
func ProbeCert(ctx context.Context, fqdn string) bool {
	return probeCert(ctx, fqdn, probeClient())
}

// probeClient 探活用的客户端:整个进程共用一份。
//
// 原先每次探活都 newProbeClient(),于是每个调用方自带一个 http.Transport —— 而
// 零值 Transport 的 IdleConnTimeout 是 0(空闲连接永不超时),探活留下的
// readLoop/writeLoop goroutine 与它的 fd 再也不会回收。worker 每 5 分钟最多扫
// 500 个域名、每次探活产生 2 条 idle 连接(HTTPS 的 TCP + 可能的重连),进程跑
// 几小时后就 too many open files,连 HTTP 服务与 DB 连接一起失败。
//
// 单例 + DisableKeepAlives 一起用:单例消除"每次调用新建 Transport"(泄漏的来源),
// DisableKeepAlives 保证探活这种一次性短连接用完即关 —— 探活只关心对端出示了什么
// 证书,没有连接复用价值,留着 idle 连接反而占着 fd。
var probeClientOnce = sync.OnceValue(newProbeClient)

func probeClient() *http.Client {
	return probeClientOnce()
}

func newProbeClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		// 只看第一跳:探活要问的是"目标域名自己出示了什么证书",跟随跳转会
		// 把别处的证书算到它头上。
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 见上
			// 探活是一次性短连接:不复用连接,也就不会留下 idle 连接等待超时。
			DisableKeepAlives: true,
			// 自定义拨号:拒绝把探活打到内网地址上去(见 probeDialContext)。
			DialContext: probeDialContext,
		},
	}
}

// cgnatPrefix RFC 6598 的运营商级 NAT 段:IsPrivate 不覆盖它,而云上内网与
// 元数据服务(如 100.100.100.200)常落在这里。
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// probeIPAllowed 探活是否允许连到这个地址:只允许公网单播地址。
//
// 为什么必须在这里挡:域名归属校验证明的是**提交时**"这个域名的 A/AAAA 指向本机"
// 加上"其权威 DNS 里有我们的 TXT"。提交者随后可以把域名改指到内网 IP
// (169.254.169.254 云元数据、10.x 内网服务、127.0.0.1 本机管理口)—— 探活用的是
// http.Client,解析发生在**请求时刻**,默认拨号器会照直连过去。一次探活就变成对
// 内网的 SSRF 请求,而且"握手成功/出示了什么证书"还会回写到 cert_status,给攻击者
// 提供一条盲注信道。所以判定必须落在真正要连的那个 IP 上,而不是域名文本上。
//
// 代价说清楚:loopback 与私网地址一律拒绝,所以本地开发环境
// (JANUS_SERVER_PUBLIC_IP=127.0.0.1 + hosts 把域名指到 127.0.0.1)的证书探活会
// 失败 —— 这是刻意的取舍:探活的安全边界按生产口径定,本地若要验证 on-demand
// 签发请直接在浏览器里访问域名。更严格的替代方案是"必须等于 ServerPublicIP",
// 需要把 IP 传进探活客户端(持有者在 httpapi),属跨包装配。
func probeIPAllowed(addr netip.Addr) bool {
	a := addr.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() {
		return false
	}
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	if cgnatPrefix.Contains(a) {
		return false
	}
	return true
}

// probeDialContext 探活专用的拨号器:解析域名 → 逐个校验地址 → 只连公网单播。
//
// 校验与连接之间没有二次解析(连接用的是校验过的那个 IP 字面量),所以不存在
// "校验时是公网、连接时变内网"的 DNS rebinding 窗口。
func probeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range addrs {
		if !probeIPAllowed(ip) {
			lastErr = fmt.Errorf("探活拒绝连接: %s 解析到非公网地址 %s", host, ip)
			continue
		}
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("探活目标 %s 没有可用地址", host)
	}
	return nil, lastErr
}

// probeCert 是可注入 client 的实现,便于单测用 TLS 测试服务器验证"证书是否覆盖 FQDN"。
func probeCert(ctx context.Context, fqdn string, client *http.Client) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+fqdn+"/", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	// 响应码不参与判定:Caddy 对未授权域名返 403、对无短码返 404,都说明它已经
	// 为这个域名握手成功了。真正要问的是"出示的证书是不是这个域名的"。
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return false
	}
	return resp.TLS.PeerCertificates[0].VerifyHostname(fqdn) == nil
}
