package rules

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruanbw/janus/internal/store"
	"github.com/ruanbw/janus/pkg/plugin"
)

type pubEnricher struct{}

func (pubEnricher) Name() string { return "asn" }
func (pubEnricher) Enrich(_ context.Context, _ *http.Request, f *plugin.Fact) {
	f.ASN = "AS13335"
	f.Country = "JP"
}

type pubInterceptor struct{ hit bool }

func (*pubInterceptor) Name() string { return "pub" }
func (p *pubInterceptor) Intercept(_ context.Context, _ *http.Request, linkID int64, f *plugin.Fact) (*plugin.Decision, bool) {
	p.hit = true
	if f.ASN != "AS13335" {
		return nil, false
	}
	return &plugin.Decision{RuleID: -1, Action: "notfound", Destination: "x"}, true
}

type panicPubEnricher struct{}

func (panicPubEnricher) Name() string { return "boom" }
func (panicPubEnricher) Enrich(context.Context, *http.Request, *plugin.Fact) {
	panic("boom")
}

// 通过公开 API(pkg/plugin)注册的富化器与拦截器必须在基座热路径上生效。
func TestPublicPluginRegistryIsWired(t *testing.T) {
	plugin.ResetEnrichers()
	plugin.ResetInterceptors()
	t.Cleanup(func() { plugin.ResetEnrichers(); plugin.ResetInterceptors() })

	plugin.RegisterEnricher(pubEnricher{})
	ic := &pubInterceptor{}
	plugin.RegisterInterceptor(ic)

	if !HasASNProvider() {
		t.Fatal("HasASNProvider 应识别公开 API 注册的 asn 富化器")
	}

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	vc := AcquireVisitorContext(req, "", "").WithIP("1.2.3.4")
	defer ReleaseVisitorContext(vc)
	vc.ApplyEnrichers(req.Context(), req)

	fact := vc.Fact()
	if fact.ASN != "AS13335" || fact.Country != "JP" {
		t.Fatalf("富化结果未回填: asn=%q country=%q", fact.ASN, fact.Country)
	}
	d, ok := CheckInterceptors(req.Context(), req, 42, &fact)
	if !ic.hit || !ok || d == nil || d.Action != "notfound" {
		t.Fatalf("公开拦截器未生效: hit=%v ok=%v d=%+v", ic.hit, ok, d)
	}
}

// 公开 API 注册的富化器 panic 不得冒到热路径。
func TestPublicPluginEnricherPanicIsIsolated(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(panicPubEnricher{})

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	vc := AcquireVisitorContext(req, "US", "").WithIP("1.2.3.4")
	defer ReleaseVisitorContext(vc)
	vc.ApplyEnrichers(req.Context(), req)
	if got := vc.Fact().Country; got != "US" {
		t.Fatalf("country = %q, want US", got)
	}
}

// 未注册任何公开插件时,拦截器链必须原样放行。
func TestPublicPluginRegistryEmptyIsNoop(t *testing.T) {
	plugin.ResetInterceptors()
	f := Fact{IP: "1.2.3.4"}
	if d, ok := CheckInterceptors(context.Background(), nil, 1, &f); ok || d != nil {
		t.Fatalf("空注册表不应拦截: ok=%v d=%+v", ok, d)
	}
}

// pluginIPEnricher 通过公开 API 注册的富化器:改写 IP 和/或 ipattr(空串=不改写)。
type pluginIPEnricher struct {
	ip     string
	ipAttr string
}

func (pluginIPEnricher) Name() string { return "pub_ip_rewriter" }

func (e pluginIPEnricher) Enrich(_ context.Context, _ *http.Request, f *plugin.Fact) {
	if e.ip != "" {
		f.IP = e.ip
	}
	if e.ipAttr != "" {
		f.IPAttr = e.ipAttr
	}
}

// TestPluginEnricherIPRewriteReachesFact 插件富化器改写 IP 后,画像自己必须跟上。
//
// Fact.ClientIP() 优先用随画像解析好的 netIP,而 mergePluginFact 原来只写 IP
// 不更新 netIP —— 「ip in_cidr」仍按旧地址判定:插件改了 IP,规则却看不见。
func TestPluginEnricherIPRewriteReachesFact(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(pluginIPEnricher{ip: "203.0.113.9"})

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	fact := Fact{}.WithIP("10.1.2.3") // 基线是私网地址:ipattr = private
	if fact.IPAttr != IPAttrPrivate {
		t.Fatalf("前置条件不成立: ipattr = %q, want %q", fact.IPAttr, IPAttrPrivate)
	}

	applyPluginEnrichers(req.Context(), req, &fact)

	if got := fact.ClientIP().String(); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want 203.0.113.9(插件改写后的地址)", got)
	}
	if got := fact.IPAttr; got != "" {
		t.Fatalf("ipattr = %q, want 空:IP 已改成公网地址,不该残留旧 IP 的 private", got)
	}
	if !matchesCIDR(t, fact, "203.0.113.0/24") {
		t.Fatal("`ip in_cidr 203.0.113.0/24` 没看到插件改写后的地址")
	}
	if matchesCIDR(t, fact, "10.1.2.0/24") {
		t.Fatal("旧地址仍然命中:netIP 没有跟着插件改写更新")
	}
}

// matchesCIDR 用真实规则求值路径判断画像是否落在某个 CIDR 内。
func matchesCIDR(t *testing.T, fact Fact, cidr string) bool {
	t.Helper()
	snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
		Field: FieldIP, Operator: OpInCIDR, Values: []string{cidr},
	})}, discardLog)
	_, matched := snap.Evaluate(fact, 1)
	return matched
}

// TestPluginEnricherExplicitIPAttrIsHonored 插件显式写 ipattr(ADR-0012 的机房/代理标记)
// 必须被尊重:它没改 IP,所以派生关系不参与,写入原样保留。
func TestPluginEnricherExplicitIPAttrIsHonored(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(pluginIPEnricher{ipAttr: "hosting"})

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	fact := Fact{}.WithIP("203.0.113.9")
	applyPluginEnrichers(req.Context(), req, &fact)

	if got := fact.IPAttr; got != "hosting" {
		t.Fatalf("ipattr = %q, want hosting:插件显式写入被丢弃了", got)
	}
	snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
		Field: FieldIPAttr, Operator: OpEq, Values: []string{"hosting"},
	})}, discardLog)
	if _, matched := snap.Evaluate(fact, 1); !matched {
		t.Fatal("`ipattr eq hosting` 未命中:机房标记没有到达规则求值")
	}
}

// TestPluginEnricherInvalidIPKeepsBaseline 插件写了无法解析的 IP 时,
// 整条保留基线 IP 并留一条 warn:宁可忽略这次改写,也不能让画像里的 IP
// 变成一个无法求值的字符串(ip / ipattr / in_cidr 会同时失去依据)。
func TestPluginEnricherInvalidIPKeepsBaseline(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(pluginIPEnricher{ip: "not-an-ip"})

	buf := &syncLogBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	fact := Fact{}.WithIP("198.51.100.7")
	applyPluginEnrichers(req.Context(), req, &fact)

	if got := fact.ClientIP().String(); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want 保留基线 198.51.100.7", got)
	}
	if !strings.Contains(buf.String(), "无法解析") {
		t.Fatalf("非法 IP 被静默忽略,没有留痕: %q", buf.String())
	}
}

// TestPluginEnricherIPv4MappedIsNormalized 插件写入 v4-mapped IPv6(::ffff:1.2.3.4)时,
// 画像里的 IP 必须与 LazyVisitorContext.backfillEnriched 同一口径归一成 1.2.3.4。
//
// 旧实现直接把插件写的原串塞进 dst.IP:于是 Field("ip") 返回 "::ffff:1.2.3.4",
// 而 ClientIP() 走 netIP/ParseAddr 归一后返回 "1.2.3.4" —— 同一个访客,
// `ip eq "1.2.3.4"` 与 `ip in_cidr "1.2.3.0/24"` 会给出相反结论。
func TestPluginEnricherIPv4MappedIsNormalized(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(pluginIPEnricher{ip: "::ffff:1.2.3.4"})

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	fact := Fact{}.WithIP("10.1.2.3")
	applyPluginEnrichers(req.Context(), req, &fact)

	if got := fact.IP; got != "1.2.3.4" {
		t.Fatalf("fact.IP = %q, want 1.2.3.4(v4-mapped 必须归一)", got)
	}
	if v, ok := fact.Field(FieldIP); !ok || v != "1.2.3.4" {
		t.Fatalf("Field(ip) = %q, %v; want \"1.2.3.4\", true:Field 不能与 ClientIP 分叉", v, ok)
	}
	if got := fact.ClientIP().String(); got != "1.2.3.4" {
		t.Fatalf("ClientIP = %q, want 1.2.3.4", got)
	}
	if !matchesCIDR(t, fact, "1.2.3.0/24") {
		t.Fatal("归一后的地址没有进 CIDR 判定")
	}
}

// TestPluginEnricherInvalidIPKeepsExplicitIPAttr 插件同时写了非法 IP 和显式 ipattr 时:
// IP 回退基线,但显式 ipattr 必须独立生效。
//
// 旧实现把 IP 与 ipattr 绑在同一个 return 里整条回滚(dst.IP, dst.IPAttr = base, base),
// 显式标记被静默丢弃;而 backfillEnriched 的口径是"显式写入优先"(ipattr 与 IP 是否
// 可解析无关)。两条富化路径必须给出同一个画像,否则同一份插件配置经不同入口
// 得到的裁决不同。
func TestPluginEnricherInvalidIPKeepsExplicitIPAttr(t *testing.T) {
	plugin.ResetEnrichers()
	t.Cleanup(plugin.ResetEnrichers)
	plugin.RegisterEnricher(pluginIPEnricher{ip: "not-an-ip", ipAttr: "hosting"})

	req := httptest.NewRequest(http.MethodGet, "http://go.example.com/abc", nil)
	fact := Fact{}.WithIP("198.51.100.7")
	applyPluginEnrichers(req.Context(), req, &fact)

	if got := fact.IP; got != "198.51.100.7" {
		t.Fatalf("fact.IP = %q, want 保留基线 198.51.100.7", got)
	}
	if got := fact.IPAttr; got != "hosting" {
		t.Fatalf("ipattr = %q, want hosting:非法 IP 回退不该连带丢弃显式 ipattr", got)
	}
}
