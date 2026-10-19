package rules

import (
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/ruanbw/janus/internal/store"
)

// countQueryParses 把 parseQueryFunc 换成计数器,返回读取计数的函数。
// 只统计"整条查询串被完整解析"的次数 —— 这正是本次修复要压到 1 的量。
func countQueryParses(t *testing.T) func() int32 {
	t.Helper()
	var calls atomic.Int32
	prev := parseQueryFunc
	parseQueryFunc = func(raw string) (url.Values, error) {
		calls.Add(1)
		return url.ParseQuery(raw)
	}
	t.Cleanup(func() { parseQueryFunc = prev })
	return calls.Load
}

// TestQueryStringParsedOncePerRequest 是"query: 动态字段不再逐字段重复解析整条查询串"的回归测试。
//
// 旧实现里每个**不同**的 query: 参数名都会独立走一次 url.ParseQuery(整条查询串):
// N 条 query: 条件 = N 次全串解析,而解析结果本该是全部字段共享的一份。
// 规则求值链路更明显 —— 诊断留痕(collectTraces)与判定(match)会反复问到同一批字段。
func TestQueryStringParsedOncePerRequest(t *testing.T) {
	t.Run("逐字段取值只解析一次", func(t *testing.T) {
		parses := countQueryParses(t)

		req := httptest.NewRequest("GET", "http://x.test/p?a=1&b=2&c=3&d=4&e=5", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		vc := AcquireVisitorContext(req, "", "").WithIP("203.0.113.9")
		defer ReleaseVisitorContext(vc)

		want := map[string]string{"query:a": "1", "query:b": "2", "query:c": "3", "query:d": "4", "query:e": "5"}
		for field, exp := range want {
			got, ok := vc.Field(field)
			if !ok || got != exp {
				t.Fatalf("Field(%q) = %q, %v; want %q, true", field, got, ok, exp)
			}
		}
		if got := parses(); got != 1 {
			t.Fatalf("5 个 query: 字段解析了整条查询串 %d 次, want 1", got)
		}
	})

	t.Run("规则求值链路只解析一次", func(t *testing.T) {
		parses := countQueryParses(t)

		req := httptest.NewRequest("GET", "http://x.test/p?a=1&b=2&c=3&d=4&e=5", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		vc := AcquireVisitorContext(req, "", "").WithIP("203.0.113.9")
		defer ReleaseVisitorContext(vc)

		// 三条 query: 条件:判定与诊断留痕都会各自问到这些字段
		snap := NewSnapshot([]store.Rule{oneRule(1,
			store.RuleCondition{Field: "query:a", Operator: OpEq, Values: []string{"1"}},
			store.RuleCondition{Field: "query:b", Operator: OpEq, Values: []string{"2"}},
			store.RuleCondition{Field: "query:c", Operator: OpContains, Values: []string{"3"}},
		)}, discardLog)

		if _, matched := snap.Evaluate(vc, 1); !matched {
			t.Fatal("前置条件不成立:三条 query: 条件应全部命中")
		}
		res := snap.Simulate(vc, 1, nil, nil)
		if !res.Matched {
			t.Fatalf("仿真应命中: %+v", res)
		}
		if got := parses(); got != 1 {
			t.Fatalf("求值+仿真解析了整条查询串 %d 次, want 1", got)
		}
	})
}
