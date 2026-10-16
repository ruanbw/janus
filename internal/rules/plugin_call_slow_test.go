package rules

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"janus/pkg/plugin"
)

// 测试替身:耗时可控的富化器 / 拦截器(基座注册表与 pkg/plugin 注册表各一套)。

type slowEnricher struct {
	name  string
	delay time.Duration
}

func (s slowEnricher) Name() string { return s.name }
func (s slowEnricher) Enrich(_ context.Context, _ *http.Request, _ *Fact) {
	time.Sleep(s.delay)
}

type slowInterceptor struct {
	name  string
	delay time.Duration
}

func (s slowInterceptor) Name() string { return s.name }
func (s slowInterceptor) Intercept(_ context.Context, _ *http.Request, _ int64, _ *Fact) (*Decision, bool) {
	time.Sleep(s.delay)
	return nil, false
}

type slowPluginEnricher struct {
	name  string
	delay time.Duration
}

func (s slowPluginEnricher) Name() string { return s.name }
func (s slowPluginEnricher) Enrich(_ context.Context, _ *http.Request, _ *plugin.Fact) {
	time.Sleep(s.delay)
}

type slowPluginInterceptor struct {
	name  string
	delay time.Duration
}

func (s slowPluginInterceptor) Name() string { return s.name }
func (s slowPluginInterceptor) Intercept(_ context.Context, _ *http.Request, _ int64, _ *plugin.Fact) (*plugin.Decision, bool) {
	time.Sleep(s.delay)
	return nil, false
}

// resetPluginRegistries 清空两侧注册表(两套注册表都要清,否则用例互相污染)。
func resetPluginRegistries(t *testing.T) {
	t.Helper()
	ResetEnrichers()
	ResetInterceptors()
	plugin.ResetEnrichers()
	plugin.ResetInterceptors()
	t.Cleanup(func() {
		ResetEnrichers()
		ResetInterceptors()
		plugin.ResetEnrichers()
		plugin.ResetInterceptors()
	})
}

// captureDefaultLog 捕获全局 slog 输出(插件告警走的是 slog.Error 这个默认出口)。
func captureDefaultLog(t *testing.T) (*strings.Builder, func() string) {
	t.Helper()
	var b strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &b, b.String
}

// TestPluginCallSlowPathIsLogged 是"插件热路径调用有耗时护栏"的回归测试。
//
// 插件契约要求微秒级纯内存计算(见 FactEnricher 的文档注释),但 panic 隔离
// 保护不了"慢":一个 sleep 1s 的富化器会直接把整条跳转链路挂住,而且没有任何信号。
// 修复:每次插件调用外层记一次耗时,超阈值(pluginCallSlowThreshold)就告警 ——
// 结果照常采用,只增加可观测性,不改 fail-open 语义。
func TestPluginCallSlowPathIsLogged(t *testing.T) {
	t.Run("超过阈值的插件必须告警", func(t *testing.T) {
		resetPluginRegistries(t)
		buf, output := captureDefaultLog(t)

		// Given: 四个在热路径上睡 3ms 的插件(默认阈值 1ms)
		RegisterEnricher(slowEnricher{name: "slow-enricher", delay: 3 * time.Millisecond})
		RegisterInterceptor(slowInterceptor{name: "slow-interceptor", delay: 3 * time.Millisecond})
		plugin.RegisterEnricher(slowPluginEnricher{name: "slow-plugin-enricher", delay: 3 * time.Millisecond})
		plugin.RegisterInterceptor(slowPluginInterceptor{name: "slow-plugin-interceptor", delay: 3 * time.Millisecond})

		// When: 走一次真实的富化 + 拦截链路
		ApplyEnrichers(context.Background(), nil, &Fact{})
		CheckInterceptors(context.Background(), nil, 1, &Fact{})

		// Then: 两侧注册表的慢调用都留下了告警,且带标识
		got := output()
		if !strings.Contains(got, "耗时超过") {
			t.Fatalf("慢插件没有产生耗时告警:\n%s", got)
		}
		for _, want := range []string{"slow-enricher", "slow-interceptor", "pkg/plugin"} {
			if !strings.Contains(got, want) {
				t.Errorf("告警里缺少标识 %q:\n%s", want, got)
			}
		}
		if buf.Len() == 0 {
			t.Fatal("日志缓冲为空")
		}
	})

	t.Run("阈值内的插件不告警", func(t *testing.T) {
		resetPluginRegistries(t)
		_, output := captureDefaultLog(t)
		// Given: 把阈值抬到一小时 —— 任何真实调用都落在阈值内,断言与机器快慢无关
		prev := pluginCallSlowThreshold
		pluginCallSlowThreshold = time.Hour
		t.Cleanup(func() { pluginCallSlowThreshold = prev })

		RegisterEnricher(slowEnricher{name: "fast-enricher"})
		RegisterInterceptor(slowInterceptor{name: "fast-interceptor"})
		plugin.RegisterEnricher(slowPluginEnricher{name: "fast-plugin-enricher"})
		plugin.RegisterInterceptor(slowPluginInterceptor{name: "fast-plugin-interceptor"})

		ApplyEnrichers(context.Background(), nil, &Fact{})
		CheckInterceptors(context.Background(), nil, 1, &Fact{})

		if got := output(); strings.Contains(got, "耗时超过") {
			t.Fatalf("阈值内的插件不该告警:\n%s", got)
		}
	})
}
