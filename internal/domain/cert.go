package domain

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
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
		},
	}
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
