package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"janus/pkg/plugin"
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
