package rules

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type dummyEnricher struct {
	name string
	asn  string
}

func (d *dummyEnricher) Name() string { return d.name }
func (d *dummyEnricher) Enrich(ctx context.Context, r *http.Request, fact *Fact) {
	if fact != nil {
		fact.ASN = d.asn
	}
}

func TestEnricherRegistrationAndExecution(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	if HasASNProvider() {
		t.Fatal("expected HasASNProvider to be false initially")
	}

	fact := &Fact{Country: "US"}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	ApplyEnrichers(context.Background(), req, fact)
	if fact.ASN != "" {
		t.Fatalf("expected ASN to be empty, got %q", fact.ASN)
	}

	RegisterEnricher(&dummyEnricher{name: "asn", asn: "AS15169"})
	if !HasASNProvider() {
		t.Fatal("expected HasASNProvider to be true after registration")
	}

	ApplyEnrichers(context.Background(), req, fact)
	if fact.ASN != "AS15169" {
		t.Fatalf("expected ASN = AS15169, got %q", fact.ASN)
	}
}

type dummyInterceptor struct {
	name     string
	decision *Decision
	match    bool
}

func (d *dummyInterceptor) Name() string { return d.name }
func (d *dummyInterceptor) Intercept(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	return d.decision, d.match
}

func TestInterceptorRegistrationAndExecution(t *testing.T) {
	ResetInterceptors()
	defer ResetInterceptors()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	fact := &Fact{Country: "US"}

	d, ok := CheckInterceptors(context.Background(), req, 100, fact)
	if ok || d != nil {
		t.Fatal("expected CheckInterceptors to return false initially")
	}

	RegisterInterceptor(&dummyInterceptor{
		name:     "safe_mode",
		decision: &Decision{Action: "pass"},
		match:    true,
	})

	d, ok = CheckInterceptors(context.Background(), req, 100, fact)
	if !ok || d == nil || d.Action != "pass" {
		t.Fatalf("expected intercepted decision pass, got %+v (ok=%v)", d, ok)
	}
}

// panicEnricher 故意炸掉的富化器:用来钉住"插件 panic 不得冒泡到调用方"。
type panicEnricher struct{}

func (panicEnricher) Name() string                                 { return "panic_enricher" }
func (panicEnricher) Enrich(context.Context, *http.Request, *Fact) { panic("enricher boom") }

// TestEnricherPanicIsolated 一个富化器 panic 不能连累后面的富化器,
// 也不能把 panic 抛回调用方(调用方是访客热路径)。
func TestEnricherPanicIsolated(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	RegisterEnricher(panicEnricher{})
	RegisterEnricher(&dummyEnricher{name: "asn", asn: "AS15169"})

	fact := &Fact{Country: "US"}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	ApplyEnrichers(context.Background(), req, fact)

	if fact.ASN != "AS15169" {
		t.Fatalf("ASN = %q, want panic 之后的富化器照常生效", fact.ASN)
	}
}

// TestInterceptorPanicIsolated 拦截器 panic 等价于"没拦住":继续问后面的拦截器,
// 全部问完仍不短路就交回基座求值。
func TestInterceptorPanicIsolated(t *testing.T) {
	ResetInterceptors()
	defer ResetInterceptors()

	RegisterInterceptor(panicInterceptor{})
	RegisterInterceptor(&dummyInterceptor{
		name:     "safe_mode",
		decision: &Decision{Action: "pass"},
		match:    true,
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	fact := &Fact{Country: "US"}
	d, ok := CheckInterceptors(context.Background(), req, 100, fact)
	if !ok || d == nil || d.Action != "pass" {
		t.Fatalf("期望后面拦截器的裁决生效, got %+v (ok=%v)", d, ok)
	}
}

type panicInterceptor struct{}

func (panicInterceptor) Name() string { return "panic_interceptor" }
func (panicInterceptor) Intercept(context.Context, *http.Request, int64, *Fact) (*Decision, bool) {
	panic("interceptor boom")
}

// syncLogBuf 并发安全的日志缓冲（slog handler 直接写它）。
type syncLogBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *syncLogBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *syncLogBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// TestEnricherHealthyRunLogsNothing 富化器正常返回时不得产出任何 panic 日志。
// 这是"降级要留痕"那条铁律的另一半:留痕只能发生在真的出事时。
// 判断反了(recover() 返回值比较写反)不会让 panic 漏出去——recover() 已经被调用,
// 后果是每次正常富化都记一条假错误,把真正的 panic 淹没在噪声里。
func TestEnricherHealthyRunLogsNothing(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	buf := &syncLogBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	RegisterEnricher(&dummyEnricher{name: "asn", asn: "AS15169"})
	fact := &Fact{Country: "US"}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	ApplyEnrichers(context.Background(), req, fact)

	if fact.ASN != "AS15169" {
		t.Fatalf("ASN = %q, want AS15169", fact.ASN)
	}
	if s := buf.String(); strings.Contains(s, "panic") {
		t.Fatalf("正常富化不得输出 panic 日志, got %q", s)
	}
}

// TestHasASNProviderNameContract asn 字段准入只认约定的富化器名:
// "asn" 与 "asn_enricher" 都算数,别的名字不能顺带把 asn 规则打开
// （否则一个只补国家码的富化器会让 asn 条件变成恒不命中的死规则）。
func TestHasASNProviderNameContract(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	RegisterEnricher(&dummyEnricher{name: "asn_enricher"})
	if !HasASNProvider() {
		t.Fatal(`Name()="asn_enricher" 同样应打开 asn 字段准入`)
	}

	ResetEnrichers()
	RegisterEnricher(&dummyEnricher{name: "geo_extra"})
	if HasASNProvider() {
		t.Fatal("与 ASN 无关的富化器不得打开 asn 准入")
	}
}

// TestCheckOnePanicYieldsNoInterception 直接钉住 checkOne 自己的契约:
// panic 一律翻译成 (nil, false)。
// 只从 CheckInterceptors 外面看是看不出这一条的——调用点的 `ok && d != nil`
// 会把「ok=true 但 d=nil」和「ok=false」当成同一件事,于是实现里
// "panic 时把 ok 写成 true" 这种错误能一路混过黑盒断言。
func TestCheckOnePanicYieldsNoInterception(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	fact := &Fact{Country: "US"}

	d, ok := checkOne(panicInterceptor{}, context.Background(), req, 100, fact)
	if ok || d != nil {
		t.Fatalf("checkOne 对 panic 拦截器应返回 (nil,false), got (%+v, %v)", d, ok)
	}
}

// TestInterceptorDecisionWithoutClaimIsIgnored 拦截器返回 (决议, false) 的含义是
// 「我看到了这次访问,但我不拦」——第二返回值才是裁决权,不是决议本身。
// 少问这一句的后果:一个只想记日志的拦截器会顺手把整个短链的裁决改掉。
func TestInterceptorDecisionWithoutClaimIsIgnored(t *testing.T) {
	ResetInterceptors()
	defer ResetInterceptors()

	RegisterInterceptor(&dummyInterceptor{
		name:     "observe_only",
		decision: &Decision{Action: "notfound"},
		match:    false, // 看见了,但不主张拦截
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	fact := &Fact{Country: "US"}
	d, ok := CheckInterceptors(context.Background(), req, 100, fact)
	if ok || d != nil {
		t.Fatalf("(决议, false) 不构成拦截, got (%+v, %v)", d, ok)
	}
}

// ipRewritingEnricher 把 Fact.IP 改成指定值(空串表示"清空")。
type ipRewritingEnricher struct{ set string }

func (e *ipRewritingEnricher) Name() string { return "ip_rewriter" }
func (e *ipRewritingEnricher) Enrich(_ context.Context, _ *http.Request, fact *Fact) {
	fact.IP = e.set
}

// TestEnricherIPRewriteBackfillsVisitorContext 富化器改写 Fact.IP 后,
// 访客上下文必须跟着换 IP —— 规则求值与访问明细读的都是这个上下文,
// 不同步就等于"富化器改了但没人看见"。
func TestEnricherIPRewriteBackfillsVisitorContext(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.RemoteAddr = "198.51.100.7:1234"
	v := AcquireVisitorContext(req, "US", "").WithIP("198.51.100.7")
	defer ReleaseVisitorContext(v)

	RegisterEnricher(&ipRewritingEnricher{set: "203.0.113.9"})
	v.ApplyEnrichers(context.Background(), req)

	if got := v.ClientIP().String(); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want 富化器改写后的 203.0.113.9", got)
	}
	if got, ok := v.Field(FieldIP); !ok || got != "203.0.113.9" {
		t.Fatalf("ip 字段 = (%q, %v), want 203.0.113.9", got, ok)
	}
}

// TestEnricherBlankIPKeepsResolvedIP 富化器把 Fact.IP 清空时,
// 已解析出来的来源 IP 必须原样保留。
// "没填 IP"是常态而不是异常:任何只补 ASN/国家的富化器都会走这条路,
// 一旦把访客 IP 清掉,ip/ipattr/CIDR 三类条件会同时失去依据,
// 而且表现为"规则莫名其妙全不命中",极难定位。
func TestEnricherBlankIPKeepsResolvedIP(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	v := AcquireVisitorContext(req, "US", "").WithIP("10.1.2.3")
	defer ReleaseVisitorContext(v)

	RegisterEnricher(&ipRewritingEnricher{set: ""}) // 只碰 IP,其余字段不动
	v.ApplyEnrichers(context.Background(), req)

	if got := v.ClientIP().String(); got != "10.1.2.3" {
		t.Fatalf("ClientIP = %q, want 保留原值 10.1.2.3", got)
	}
	if got, ok := v.Field(FieldIPAttr); !ok || got == "" {
		t.Fatalf("ipattr = (%q, %v), 访客 IP 被清空后 ipattr 不该跟着失效", got, ok)
	}
}
