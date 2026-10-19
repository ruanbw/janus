package metrics

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// 本文件锁定 WriteText 的输出契约(Prometheus 文本格式 0.0.4)。
// 这些断言看着琐碎,但抓取端对格式是严格解析的:一处不符,整个指标族被丢弃,
// 而且只会在 Prometheus 的 target 页面上留下一句 "text format parsing error"。

// mustWriteText 是所有 exposition 断言的入口,顺手断言 error 为 nil。
// 为什么:WriteText 的错误在生产里会被 handler 折叠成 500,只看文本断言会漏掉它。
func mustWriteText(t *testing.T, r *Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatalf("WriteText 返回错误 %v:它是 /metrics 的唯一输出路径,出错等于抓取永久失败", err)
	}
	return buf.String()
}

// sampleValue 从 exposition 文本里取一条样本行的值(样本行形如 `name{label="v"} 2`)。
func sampleValue(t *testing.T, body, sample string) (float64, bool) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i < 0 || line[:i] != sample {
			continue
		}
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			t.Fatalf("样本行 %q 的值不是数字: %v", line, err)
		}
		return v, true
	}
	return 0, false
}

// TestWriteTextExpositionFormat 用一份最小注册表逐字节锁定输出格式。
//
// 锁死三件事:
//  1. 每个指标族先 `# HELP` 再 `# TYPE`(顺序反了抓取端会当成格式错误);
//  2. 指标族按名字字典序输出 —— 抓取端不在乎顺序,但人要看两次抓取的 diff,
//     只有顺序稳定,diff 里才只剩数值变化;
//  3. CounterVec 的样本行是 `name{label="v"} 2`(花括号紧贴名字、值用双引号)。
func TestWriteTextExpositionFormat(t *testing.T) {
	reg := New()
	// 注册顺序刻意与字典序相反。
	gauge := reg.Gauge("janus_b_gauge", "B gauge.")
	gauge.Set(3.5)
	reg.Counter("janus_a_total", "A counter.").Add(2)
	reg.CounterVec("janus_c_requests_total", "C counter vec.", "result").Inc("ok")

	want := "# HELP janus_a_total A counter.\n" +
		"# TYPE janus_a_total counter\n" +
		"janus_a_total 2\n" +
		"# HELP janus_b_gauge B gauge.\n" +
		"# TYPE janus_b_gauge gauge\n" +
		"janus_b_gauge 3.5\n" +
		"# HELP janus_c_requests_total C counter vec.\n" +
		"# TYPE janus_c_requests_total counter\n" +
		"janus_c_requests_total{result=\"ok\"} 1\n"

	if got := mustWriteText(t, reg); got != want {
		t.Fatalf("文本格式(0.0.4)不符:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// histogramBucket 是解析出来的一行 _bucket。
//
// le 用 float64 而不是原始字符串:le 的字面写法("1" 还是 "1.0")不是契约的一部分,
// 契约要求的是**语义**——上界升序、末尾 +Inf、计数累计。
type histogramBucket struct {
	le    float64
	inf   bool
	count float64
}

// parseBuckets 解析 `<family>_bucket{le="..."} <count>` 行。
func parseBuckets(t *testing.T, body, family string) []histogramBucket {
	t.Helper()
	prefix := family + "_bucket{le=\""
	var out []histogramBucket
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimPrefix(line, prefix)
		end := strings.Index(rest, "\"}")
		if end < 0 {
			t.Fatalf("桶行没有闭合的 le 标签: %q", line)
		}
		le := rest[:end]

		i := strings.LastIndexByte(line, ' ')
		if i < 0 {
			t.Fatalf("桶行缺少样本值: %q", line)
		}
		count, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			t.Fatalf("桶行 %q 的计数不是数字: %v", line, err)
		}

		b := histogramBucket{count: count}
		if le == "+Inf" {
			b.inf = true
		} else {
			bound, err := strconv.ParseFloat(le, 64)
			if err != nil {
				t.Fatalf("桶行 %q 的 le 不是数字也不是 +Inf: %v", line, err)
			}
			b.le = bound
		}
		out = append(out, b)
	}
	return out
}

// TestWriteTextHistogramBucketsAreCumulative 固化直方图的桶语义。
//
// 为什么重要:Prometheus 的 _bucket 是**累计**计数,histogram_quantile 直接按这个
// 前提插值。如果实现按"每个桶只记落在本区间的样本",分位数会算成负数或 0,
// 而这个错误在原始指标里完全看不出来(每个桶的数值都"合理")。
// 末尾的 le="+Inf" 桶同时承担"总样本数"的角色,缺了它整族直方图不可用。
func TestWriteTextHistogramBucketsAreCumulative(t *testing.T) {
	reg := New()
	h := reg.Histogram("janus_latency_seconds", "请求耗时。", []float64{0.1, 1, 5})
	// 0.5 落在 (0.1, 1]:它必须**同时**计入 le="1" 与 le="+Inf" 两个桶。
	h.Observe(0.5)
	// 2 落在 (1, 5]:计入 le="5" 与 le="+Inf"。
	h.Observe(2)

	body := mustWriteText(t, reg)
	if !strings.Contains(body, "# TYPE janus_latency_seconds histogram\n") {
		t.Fatalf("缺少 histogram 的 TYPE 行:\n%s", body)
	}

	buckets := parseBuckets(t, body, "janus_latency_seconds")
	if len(buckets) != 4 {
		t.Fatalf("期望 3 个 le 上界 + 1 个 +Inf 桶, 得到 %d 个: %+v", len(buckets), buckets)
	}

	wantBounds := []float64{0.1, 1, 5}
	for i, bound := range wantBounds {
		if buckets[i].inf || buckets[i].le != bound {
			t.Fatalf("第 %d 个桶的上界应为 %v(le 必须升序), 得到 %+v", i, bound, buckets[i])
		}
	}
	if !buckets[3].inf {
		t.Fatalf("末桶必须是 le=\"+Inf\", 得到 %+v", buckets[3])
	}

	// 累计计数:[0.1]=0、[1]=1(只有 0.5)、[5]=2(0.5 与 2 都 ≤5)、[+Inf]=2。
	wantCounts := []float64{0, 1, 2, 2}
	for i, want := range wantCounts {
		if buckets[i].count != want {
			t.Fatalf("第 %d 个桶应为累计值 %v, 得到 %v(每桶只记本区间是不对的)", i, want, buckets[i].count)
		}
	}

	if got, ok := sampleValue(t, body, "janus_latency_seconds_sum"); !ok || got != 2.5 {
		t.Fatalf("_sum 应为 0.5+2=2.5, 得到 %v (present=%v)\n%s", got, ok, body)
	}
	if got, ok := sampleValue(t, body, "janus_latency_seconds_count"); !ok || got != 2 {
		t.Fatalf("_count 应为观测次数 2, 得到 %v (present=%v)\n%s", got, ok, body)
	}
}

// TestWriteTextEmptyRegistryOutputsNothing 固化空注册表的输出。
//
// 为什么重要:/metrics 必须能在一个还没接线任何指标的进程上正常响应。
// 若空注册表也吐出一堆 "0" 样本行,抓取端会把这些"看起来存在"的指标写进
// 时序库,后续接线上线后数值会从 0 跳到真实值,历史上留下一段假数据。
func TestWriteTextEmptyRegistryOutputsNothing(t *testing.T) {
	reg := New()

	var buf bytes.Buffer
	if err := reg.WriteText(&buf); err != nil {
		t.Fatalf("空注册表 WriteText 返回错误: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("空注册表必须输出空内容, 实际输出 %q", buf.String())
	}
}

// TestWriteTextIsIdempotent 固化"连续两次导出逐字节相同"。
//
// 为什么重要:抓取端每 15s 拉一次。若 WriteText 会改写内部状态(例如把
// histogram 桶在原地累积换算、把 CounterVec 的子项重新排序后写回),
// 同一个时间窗口的两次抓取就会给出不同的数值,时序图上出现无中生有的抖动。
func TestWriteTextIsIdempotent(t *testing.T) {
	reg := New()
	reg.Counter("janus_idem_total", "幂等性。").Inc()
	vec := reg.CounterVec("janus_idem_requests_total", "幂等性(带标签)。", "result")
	vec.Inc("ok")
	vec.Inc("error")
	h := reg.Histogram("janus_idem_seconds", "幂等性(直方图)。", []float64{0.5, 1})
	h.Observe(0.7)

	first := mustWriteText(t, reg)
	second := mustWriteText(t, reg)
	if first != second {
		t.Fatalf("两次导出的输出不一致(WriteText 有副作用):\n--- 第一次 ---\n%s\n--- 第二次 ---\n%s", first, second)
	}
}
