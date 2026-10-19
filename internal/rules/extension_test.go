package rules

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ruanbw/janus/internal/store"
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

// fieldRewritingEnricher 按字段名写画像:模拟私有版富化器(ASN / 机房标记 / 探针回传 Token 等)。
// 它直接改 Fact 的导出字段,与 pkg/plugin 的公开 DTO 富化器是同一套语义。
type fieldRewritingEnricher struct {
	name  string
	field string
	value string
}

func (e *fieldRewritingEnricher) Name() string { return e.name }

func (e *fieldRewritingEnricher) Enrich(_ context.Context, _ *http.Request, fact *Fact) {
	setFactField(fact, e.field, e.value)
}

// setFactField 按字段名写入画像(测试脚手架:生产侧富化器直接改 Fact 字段)。
// 未覆盖的字段直接 panic —— 脚手架宁可炸,也不要静默漏掉一个字段。
func setFactField(f *Fact, field, value string) {
	switch field {
	case FieldIP:
		f.IP = value
	case FieldIPAttr:
		f.IPAttr = value
	case FieldCountry:
		f.Country = value
	case FieldASN:
		f.ASN = value
	case FieldLang:
		f.Lang = value
	case FieldRef:
		f.Ref = value
	case FieldUTM:
		f.UTM = value
	case FieldUA:
		f.UA = value
	case FieldDevType:
		f.DevType = value
	case FieldOS:
		f.OS = value
	case FieldBrowser:
		f.Browser = value
	case FieldPath:
		f.Path = value
	case FieldDomain:
		f.Domain = value
	default:
		panic("setFactField 未覆盖字段: " + field)
	}
}

// TestEnricherRewriteOfEveryFieldReachesEvaluation 富化器对画像的写入必须完整到达求值侧。
//
// 为什么值得单独钉:LazyVisitorContext.ApplyEnrichers 目前只回填 country / asn / ip,
// 富化器写进 fact 的其他字段会被静默丢弃 —— 而两个生产调用点
// (httpapi/redirect.go 的 ruleDecision、httpapi/rules.go 的 handleSimulateRules)
// 走的都是这条 LazyVisitorContext 路径。结果是"富化器注册了、规则也配好了,
// 线上就是不生效",且没有任何报错。
//
// ADR-0012 把 FactEnricher 定为私有版注入"ASN、机房标记及前端探针回传 Token"的
// 唯一入口,这条不变式就是那个扩展点的全部价值。
//
// country / asn / ip 已有回填(见 ApplyEnrichers),不在本表内;
// ip 与 ipattr 的派生关系另见下面两条测试。
//
// Given: 一个把某个字段改写成新值的富化器(以及一条与该值相等的规则)
// When:  在 LazyVisitorContext 上执行富化,再用同一个上下文求值
// Then:  该字段必须取到新值,规则必须命中 —— 写入不得被回填白名单吃掉。
func TestEnricherRewriteOfEveryFieldReachesEvaluation(t *testing.T) {
	defer ResetEnrichers()

	cases := []struct {
		field string
		value string
	}{
		{FieldDevType, DevTypeBot},
		{FieldOS, "Fuchsia"},
		{FieldBrowser, "PrivateBot"},
		{FieldLang, "zz"},
		{FieldRef, "enricher.example.com"},
		{FieldUTM, "enriched"},
		{FieldUA, "PrivateBot/1.0 (private enrichment)"},
		{FieldPath, "/enriched"},
		{FieldDomain, "enriched.example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			ResetEnrichers()
			RegisterEnricher(&fieldRewritingEnricher{
				name: "rewrite_" + tc.field, field: tc.field, value: tc.value})

			req, _ := http.NewRequest(http.MethodGet, "http://example.com/plain", nil)
			req.RemoteAddr = "198.51.100.7:1234"
			v := AcquireVisitorContext(req, "US", "").WithIP("198.51.100.7")
			defer ReleaseVisitorContext(v)

			v.ApplyEnrichers(context.Background(), req)

			got, ok := v.Field(tc.field)
			if !ok || got != tc.value {
				t.Fatalf("富化后 Field(%s) = (%q, %v), want (%q, true):富化器的写入被丢弃了",
					tc.field, got, ok, tc.value)
			}

			// 真正的契约是"规则能看到":字段取到值还不够,裁决必须跟着变。
			snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
				Field: tc.field, Operator: OpEq, Values: []string{tc.value},
			})}, discardLog)
			if _, matched := snap.Evaluate(v, 1); !matched {
				t.Fatalf("%s eq %q 未命中:富化后的值没有到达规则求值", tc.field, tc.value)
			}
		})
	}
}

// TestEnricherIPRewriteDoesNotLeaveStaleIPAttr 富化器改写 IP 后,派生字段 ipattr 不得残留旧值。
//
// WithIP 会把 ipattr 算好并打上"已解析"标志;而改写 IP 时如果只更新 ipStr / clientIP、
// 不动 ipAttr 与标志位,就会留下一个自相矛盾的画像:IP 是公网的,ipattr 还是 private。
// ipattr 是规则条件字段之一,这条残留会让 `ipattr eq private` 命中公网访客。
//
// Given: 一条从私网 IP 构造的画像(ipattr = private)
// When:  富化器把它改写成公网 IP(不碰 ipattr)
// Then:  ipattr 必须按新 IP 重算 —— 公网 IP 没有 private / loopback / linklocal 属性,
//
//	因此该字段取不到值(空值恒不命中),规则也不得命中。
func TestEnricherIPRewriteDoesNotLeaveStaleIPAttr(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	v := AcquireVisitorContext(req, "", "").WithIP("10.1.2.3")
	defer ReleaseVisitorContext(v)

	// 前置条件:改写前确实是 private,否则这条测试证明不了"残留"
	if got, ok := v.Field(FieldIPAttr); !ok || got != IPAttrPrivate {
		t.Fatalf("前置条件不成立:ipattr = (%q, %v), want (private, true)", got, ok)
	}

	RegisterEnricher(&ipRewritingEnricher{set: "203.0.113.9"})
	v.ApplyEnrichers(context.Background(), req)

	if got := v.ClientIP().String(); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want 203.0.113.9", got)
	}
	got, ok := v.Field(FieldIPAttr)
	if ok && got == IPAttrPrivate {
		t.Fatalf("ipattr = %q:IP 已被改写成公网地址,ipattr 仍是旧 IP 的 private", got)
	}

	snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
		Field: FieldIPAttr, Operator: OpEq, Values: []string{IPAttrPrivate},
	})}, discardLog)
	if _, matched := snap.Evaluate(v, 1); matched {
		t.Fatal("`ipattr eq private` 命中了一个公网 IP:派生字段残留")
	}
}

// TestEnricherExplicitIPAttrRewriteIsHonored 富化器**显式**写入 ipattr 时必须被尊重。
//
// 这是 ADR-0012 里私有版"机房标记"的落地路径:基座只判 private / loopback / linklocal,
// 机房 / 代理 / 探针这类判断只有富化器知道。它与上一条(IP 改写后重算 ipattr)不冲突:
// 富化器没碰 ipattr 就按新 IP 重算,碰了就以富化器的值为准。
//
// 待确认契约:若产品侧决定"ipattr 永远由基座派生、富化器不得覆写",
// 这条测试应当删掉,并在 ADR-0012 里写明机房标记改由哪个字段承载。
func TestEnricherExplicitIPAttrRewriteIsHonored(t *testing.T) {
	ResetEnrichers()
	defer ResetEnrichers()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	v := AcquireVisitorContext(req, "", "").WithIP("203.0.113.9")
	defer ReleaseVisitorContext(v)

	RegisterEnricher(&fieldRewritingEnricher{name: "hosting", field: FieldIPAttr, value: "hosting"})
	v.ApplyEnrichers(context.Background(), req)

	got, ok := v.Field(FieldIPAttr)
	if !ok || got != "hosting" {
		t.Fatalf("ipattr = (%q, %v), want (hosting, true):富化器显式写入的机房标记被丢弃了", got, ok)
	}
	snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
		Field: FieldIPAttr, Operator: OpEq, Values: []string{"hosting"},
	})}, discardLog)
	if _, matched := snap.Evaluate(v, 1); !matched {
		t.Fatal("`ipattr eq hosting` 未命中:机房标记没有到达规则求值")
	}
}
