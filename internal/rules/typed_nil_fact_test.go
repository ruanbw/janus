package rules

import (
	"testing"

	"janus/internal/store"
)

// TestTypedNilFactDoesNotPanicInSeenBranches 钉住 duplicated 计数分支的 typed-nil 防御。
//
// `var p *Fact; var ctx VisitorContext = p` 时接口非 nil、类型断言成功但值是 nil。
// duplicated 的两处读法原来只判到 `ok`,断言成功即解引用 nil:
//   - eval.go 的 matchOne:`if f, ok := ctx.(*Fact); ok && f.Seen != nil` → f.Seen 崩;
//   - snapshot.go 的 seenCount:`switch f := ctx.(type) { case *Fact: if f.Seen != nil` → 同上。
//
// FactFromContext 早就用 `f == nil` 双检挡住了同一类输入,这里补齐对齐。
func TestTypedNilFactDoesNotPanicInSeenBranches(t *testing.T) {
	var p *Fact
	var ctx VisitorContext = p

	// 求值分支:matchOne 的 duplicated
	cond := compiledCond{field: FieldIP, op: OpDuplicated, seen: 2}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("matchOne 对 typed-nil *Fact panic: %v", rec)
			}
		}()
		if cond.matchOne(ctx, "1.2.3.4") {
			t.Fatal("typed-nil *Fact 不该命中 duplicated")
		}
	}()

	// 诊断分支:seenCount
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("seenCount 对 typed-nil *Fact panic: %v", rec)
			}
		}()
		if n := seenCount(ctx, FieldIP, "1.2.3.4"); n != 0 {
			t.Fatalf("seenCount = %d, want 0(没有计数来源)", n)
		}
	}()

	// 端到端:Evaluate 保持 fail-open 语义,不得命中任何规则。
	snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
		Field: FieldIP, Operator: OpDuplicated, Values: []string{"2"},
	})}, discardLog)
	if _, matched := snap.Evaluate(ctx, 1); matched {
		t.Fatal("typed-nil *Fact 不该命中规则")
	}
}
