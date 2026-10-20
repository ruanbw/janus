package httpapi

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/config"
	"github.com/ruanbw/janus/internal/linkcache"
	"github.com/ruanbw/janus/internal/metrics"
	"github.com/ruanbw/janus/internal/store"
	"github.com/ruanbw/janus/internal/visitqueue"
)

// 本文件锁定「热路径内部计数 → /metrics」的映射。
//
// 背景:跳转链路刚接上短链快照缓存与访问明细异步队列,两者各自维护 Stats(),
// 但 /metrics 只导出了跳转结果与写入方式的计数器 —— 缓存命中率、队列积压这些
// 决定「该不该调 TTL / 该不该开异步」的信号在监控里完全不可见。
//
// 断言的共同前提:上游 Stats() 是单调累计值且永不清零,所以抓取时可以安全地
// 做差分。若哪天上游改成「读后清零」,这里的第二次抓取会直接掉到 0,用例会先红。

// sampleOf 取一条样本行的值;缺失直接判失败(缺席比数值错更难被发现)。
func sampleOf(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i < 0 || line[:i] != name {
			continue
		}
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			t.Fatalf("样本行 %q 的值不是数字: %v", line, err)
		}
		return v
	}
	t.Fatalf("exposition 里没有 %s:\n%s", name, body)
	return 0
}

// writeExposition 导出注册表全文。
func writeExposition(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := reg.WriteText(&buf); err != nil {
		t.Fatalf("WriteText 返回错误 %v", err)
	}
	return buf.String()
}

// mustGetLink 读一条短链快照,断言错误与期望一致。
func mustGetLink(t *testing.T, c *linkcache.Cache, domainID int64, code string, wantErr bool) {
	t.Helper()
	_, err := c.Get(context.Background(), domainID, code)
	if wantErr && err == nil {
		t.Fatalf("Get(%d,%q) 期望失败,却成功了", domainID, code)
	}
	if !wantErr && err != nil {
		t.Fatalf("Get(%d,%q) 期望成功,却返回 %v", domainID, code, err)
	}
}

// newStatsTestCache 造一个可区分「命中 / 未命中 / 加载失败」的缓存。
func newStatsTestCache(t *testing.T) *linkcache.Cache {
	t.Helper()
	return linkcache.New(func(_ context.Context, domainID int64, code string) (*linkcache.Record, error) {
		switch code {
		case "abc":
			return &linkcache.Record{LinkID: 7, DomainID: domainID}, nil
		case "boom":
			return nil, errors.New("db down")
		default:
			return nil, linkcache.ErrNotFound
		}
	})
}

// TestStatsCollectorMirrorsHotPathCounters 固化缓存与队列的指标映射。
func TestStatsCollectorMirrorsHotPathCounters(t *testing.T) {
	ctx := context.Background()

	cache := newStatsTestCache(t)
	mustGetLink(t, cache, 1, "abc", false) // misses=1 loads=1
	mustGetLink(t, cache, 1, "abc", false) // hits=1
	mustGetLink(t, cache, 1, "nope", true) // misses=2(未找到不计 loadErrors)
	mustGetLink(t, cache, 1, "boom", true) // misses=3 loadErrors=1

	writer := visitqueue.New(visitqueue.Options{
		BatchSize:     10,
		FlushInterval: time.Hour,
		Sink:          func(context.Context, []store.VisitRecord) error { return nil },
	})
	t.Cleanup(func() {
		if err := writer.Close(ctx); err != nil {
			t.Errorf("Close 返回错误 %v", err)
		}
	})
	for _, rec := range []store.VisitRecord{{LinkID: 1}, {LinkID: 2}} {
		if err := writer.Enqueue(ctx, rec); err != nil {
			t.Fatalf("Enqueue 返回错误 %v", err)
		}
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("Flush 返回错误 %v", err)
	}

	reg := metrics.New()
	collector := newStatsCollector(reg, cache, writer)

	collector.collect()
	body := writeExposition(t, reg)

	want := map[string]float64{
		"janus_linkcache_hits_total":              1,
		"janus_linkcache_misses_total":            3,
		"janus_linkcache_load_errors_total":       1,
		"janus_linkcache_entries":                 1,
		"janus_visit_writer_enabled":              1,
		"janus_visit_writer_enqueued_total":       2,
		"janus_visit_writer_written_total":        2,
		"janus_visit_writer_sync_fallback_total":  0,
		"janus_visit_writer_batch_failures_total": 0,
		"janus_visit_writer_pending":              0,
	}
	for name, value := range want {
		if got := sampleOf(t, body, name); got != value {
			t.Fatalf("%s 期望 %v, 得到 %v\n%s", name, value, got, body)
		}
	}

	// 再抓一次:上游计数没变,指标就不能再涨 —— 否则「每秒跳转数」会被
	// 抓取次数放大,线上看到的是抓取频率而不是业务量。
	collector.collect()
	body = writeExposition(t, reg)
	for name, value := range want {
		if got := sampleOf(t, body, name); got != value {
			t.Fatalf("第二次抓取后 %s 应从 %v 保持不变, 得到 %v", name, value, got)
		}
	}

	// 上游继续增长时,指标只增加差额部分。
	mustGetLink(t, cache, 1, "abc", false) // hits=2
	collector.collect()
	if got := sampleOf(t, writeExposition(t, reg), "janus_linkcache_hits_total"); got != 2 {
		t.Fatalf("新增一次命中后 hits_total 应为 2, 得到 %v", got)
	}
}

// TestStatsCollectorReportsAsyncDisabled 固化「同步模式」下的读数。
//
// 为什么要显式导出 0 而不是干脆不注册:仪表盘与告警按指标名配置。系列在同步
// 模式下消失,面板会显示「no data」并被当成抓取故障,而真实语义是「异步没开」。
func TestStatsCollectorReportsAsyncDisabled(t *testing.T) {
	reg := metrics.New()
	newStatsCollector(reg, nil, nil).collect()
	body := writeExposition(t, reg)

	for name, value := range map[string]float64{
		"janus_visit_writer_enabled":        0,
		"janus_visit_writer_pending":        0,
		"janus_visit_writer_enqueued_total": 0,
		"janus_linkcache_entries":           0,
	} {
		if got := sampleOf(t, body, name); got != value {
			t.Fatalf("%s 期望 %v, 得到 %v\n%s", name, value, got, body)
		}
	}
}

// TestMetricsEndpointExposesVisitWriterStats 固化接线:handler 抓取前必须同步一次。
//
// 单独测 collector 只能证明"算了",这条用例证明 /metrics 真的调了它 ——
// 少了这一步所有热路径指标都会恒为 0,而且没有任何报错。
func TestMetricsEndpointExposesVisitWriterStats(t *testing.T) {
	ctx := context.Background()
	writer := visitqueue.New(visitqueue.Options{
		BatchSize:     10,
		FlushInterval: time.Hour,
		Sink:          func(context.Context, []store.VisitRecord) error { return nil },
	})
	t.Cleanup(func() {
		if err := writer.Close(ctx); err != nil {
			t.Errorf("Close 返回错误 %v", err)
		}
	})
	for _, rec := range []store.VisitRecord{{LinkID: 1}, {LinkID: 2}} {
		if err := writer.Enqueue(ctx, rec); err != nil {
			t.Fatalf("Enqueue 返回错误 %v", err)
		}
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("Flush 返回错误 %v", err)
	}

	h := New(Deps{
		Cfg:          config.Config{PlatformDomain: "janus.test"},
		MetricsToken: "tok",
		VisitWriter:  writer,
	})

	w := doGet(h, "/metrics?token=tok", "127.0.0.1:1234", "")
	if w.Code != 200 {
		t.Fatalf("/metrics 期望 200, 得到 %d: %s", w.Code, w.Body.String())
	}
	if got := sampleOf(t, w.Body.String(), "janus_visit_writer_written_total"); got != 2 {
		t.Fatalf("写入 2 条后 written_total 应为 2, 得到 %v", got)
	}
}
