package rules

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// 测试替身:Name() 自己就 panic 的插件(以及 Enrich/Intercept 也 panic 的组合)。
// Name() 是外部代码,它抛出的 panic 与 Enrich/Intercept 的 panic 必须被同一层隔离。

type panicNameEnricher struct{}

func (panicNameEnricher) Name() string                                 { panic("name boom") }
func (panicNameEnricher) Enrich(context.Context, *http.Request, *Fact) {}

type panicBothEnricher struct{}

func (panicBothEnricher) Name() string { panic("name boom") }
func (panicBothEnricher) Enrich(context.Context, *http.Request, *Fact) {
	panic("enrich boom")
}

type panicNameInterceptor struct{}

func (panicNameInterceptor) Name() string { panic("name boom") }
func (panicNameInterceptor) Intercept(context.Context, *http.Request, int64, *Fact) (*Decision, bool) {
	return nil, false
}

type panicBothInterceptor struct{}

func (panicBothInterceptor) Name() string { panic("name boom") }
func (panicBothInterceptor) Intercept(context.Context, *http.Request, int64, *Fact) (*Decision, bool) {
	panic("intercept boom")
}

// TestPluginNamePanicIsIsolated 钉住"插件 Name() 的 panic 也必须被隔离层接住"。
//
// panic 处理路径上再调 Name() 是致命的:deferred 函数里新起的 panic 已经没有任何
// recover 接得住,会直接冒到访客热路径(被全局中间件兜成 500)—— 隔离层等于失效。
// 修正是把名字在调用插件**之前**取出来,且取名字这一步自己再包一层。
func TestPluginNamePanicIsIsolated(t *testing.T) {
	_, output := captureDefaultLog(t)

	cases := []struct {
		name      string
		enricher  FactEnricher
		intercept RuleInterceptor
	}{
		{"只有 Name panic", panicNameEnricher{}, panicNameInterceptor{}},
		{"Name 与 Enrich/Intercept 都 panic", panicBothEnricher{}, panicBothInterceptor{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetPluginRegistries(t)
			RegisterEnricher(tc.enricher)
			RegisterInterceptor(tc.intercept)

			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("插件 Name() 的 panic 逃逸了隔离层: %v", rec)
					}
				}()
				ApplyEnrichers(context.Background(), nil, &Fact{})
				if d, ok := CheckInterceptors(context.Background(), nil, 1, &Fact{}); ok || d != nil {
					t.Fatalf("失灵的插件不该拦下请求: ok=%v d=%+v", ok, d)
				}
			}()

			// 隔离层仍在工作:两处都留下了日志(类型名兜底也算留痕)
			got := output()
			if !strings.Contains(got, "panic") {
				t.Fatalf("插件 panic 没有留痕:\n%s", got)
			}
		})
	}
}
