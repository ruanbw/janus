package domain

// 证书探活客户端的生命周期测试。
//
// 为什么值得单测:探活的连接泄漏是"每探活一个域名就新建一个 client/Transport"
// 造成的,而 Transport 一旦被丢弃,它持有的 idle 连接、readLoop/writeLoop
// goroutine 都不会回收 —— 这个 bug 只在进程跑了几小时、探了几万个域名之后才
// 以 too many open files 的形式暴露出来,靠集成测试根本抓不到。这里把不变量
// 钉在"探活共用同一个 client 实例"上,它是"Transport 只建一次"的直接证明。

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// 探活必须复用同一个 client(同一个 Transport),而不是每次调用新建一份。
func TestProbeClientIsSharedSingleton(t *testing.T) {
	if got := probeClient(); got != probeClient() {
		t.Fatal("probeClient() 每次返回新实例:每次探活都会新建 Transport," +
			"其 idle 连接与 readLoop/writeLoop goroutine 永不回收 → fd/goroutine 单调泄漏")
	}
	// 并发下也必须收敛到同一个实例(sync.OnceValue 的语义,防止未来改成懒加载时退化)。
	const n = 8
	seen := make([]*http.Client, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seen[i] = probeClient()
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if seen[i] != seen[0] {
			t.Fatalf("并发调用 probeClient() 得到多个实例:seen[%d] != seen[0]", i)
		}
	}
}

// 探活是一次性短连接:必须禁用 keep-alive,不留下等待超时的 idle 连接。
func TestProbeClientDisablesKeepAlives(t *testing.T) {
	tr, ok := probeClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("probeClient().Transport = %T, want *http.Transport", probeClient().Transport)
	}
	if !tr.DisableKeepAlives {
		t.Fatal("探活客户端未禁用 keep-alive:每次探活都会留下一条 idle 连接," +
			"而探活只关心对端出示的证书,没有复用价值")
	}
}

// 单例化不能改变探活的超时与"只看第一跳"语义。
func TestProbeClientKeepsTimeoutAndNoRedirect(t *testing.T) {
	c := probeClient()
	if c.Timeout != 10*time.Second {
		t.Fatalf("probeClient().Timeout = %v, want 10s", c.Timeout)
	}
	if c.CheckRedirect == nil {
		t.Fatal("probeClient() 缺少 CheckRedirect:跟随跳转会把别处的证书算到目标域名头上")
	}
}
