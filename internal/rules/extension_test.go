package rules

import (
	"context"
	"net/http"
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
