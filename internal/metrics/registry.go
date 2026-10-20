// Package metrics 是最小可用的 Prometheus 文本格式(0.0.4)指标注册表。
//
// 为什么自己写而不引入 prometheus/client_golang:
//   - 基座只需要三种原语(counter / gauge / histogram + 单标签 counter vec),
//     引入整个客户端库会带来一份不用的注册表、收集器与进程默认指标;
//   - 抓取端只认文本格式,格式本身是稳定契约(有测试逐字节钉住),
//     不存在"必须用官方库才兼容"的问题;
//   - 依赖越少,自托管镜像越小、审计面越小 —— 与项目"单二进制"的取舍一致。
//
// 三条使用约定:
//  1. 注册在启动/首次调用时完成,同名同类型重复注册返回同一个对象(计数不清零);
//     同名不同类型直接 panic —— 静默覆盖会让先注册的指标从 exposition 里消失,
//     而这种错误在运行时完全不可见。
//  2. 所有方法对 nil 接收者是 no-op:调用点不该为了"指标可能没接线"写 if 判断,
//     跳转热路径上尤其不能。
//  3. WriteText 无副作用:连续两次抓取必须逐字节相同(内部不做原地换算/重排序)。
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type kind string

const (
	kindCounter   kind = "counter"
	kindGauge     kind = "gauge"
	kindHistogram kind = "histogram"
)

// Registry 进程内指标注册表,可并发使用。
type Registry struct {
	mu       sync.RWMutex
	families map[string]*family
}

// family 一个指标族(名字 + help + 类型 + 具体原语)。
// 同一名字只能以一种类型存在,见 Registry.register。
type family struct {
	name  string
	help  string
	kind  kind
	label string // 仅 CounterVec:标签名
	c     *Counter
	g     *Gauge
	h     *Histogram
	vec   *CounterVec
}

// New 构建空注册表。
func New() *Registry {
	return &Registry{families: make(map[string]*family)}
}

// register 收敛同名注册;同名不同类型 panic。
func (r *Registry) register(name, help string, k kind, label string) *family {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.families == nil {
		r.families = make(map[string]*family)
	}
	if f, ok := r.families[name]; ok {
		// 同类型(含 CounterVec 的标签名)才允许复用。
		if f.kind != k || f.label != label {
			panic(fmt.Sprintf("metrics: duplicate metric %q registered as %s%s, want %s%s",
				name, f.kind, labelSuffix(f.label), k, labelSuffix(label)))
		}
		return f
	}
	f := &family{name: name, help: help, kind: k, label: label}
	r.families[name] = f
	return f
}

func labelSuffix(label string) string {
	if label == "" {
		return ""
	}
	return "/" + label
}

// Counter 单调递增计数器。
type Counter struct {
	mu sync.Mutex
	v  float64
}

func (c *Counter) Inc() { c.Add(1) }

func (c *Counter) Add(v float64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.v += v
	c.mu.Unlock()
}

func (c *Counter) value() float64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

// Gauge 可增可减、也可直接置值的仪表盘。
type Gauge struct {
	mu sync.Mutex
	v  float64
}

func (g *Gauge) Set(v float64) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.v = v
	g.mu.Unlock()
}

func (g *Gauge) Add(v float64) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.v += v
	g.mu.Unlock()
}

func (g *Gauge) value() float64 {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.v
}

// Histogram 固定分桶直方图。
//
// counts 存的是"落在本区间"的计数,导出时才累计成 Prometheus 要求的累计桶 ——
// 这样 WriteText 保持纯读,不会因为抓取而改写内部状态。
type Histogram struct {
	mu      sync.Mutex
	bounds  []float64
	counts  []float64 // 与 bounds 等长,另加一个 "+Inf" 位
	sum     float64
	samples float64
}

func (h *Histogram) Observe(v float64) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	idx := sort.SearchFloat64s(h.bounds, v)
	h.counts[idx]++
	h.sum += v
	h.samples++
}

// CounterVec 单标签计数器集合。标签值在第一次出现时才创建子计数器。
type CounterVec struct {
	mu     sync.Mutex
	label  string
	values map[string]*Counter
}

// With 取某个标签值对应的计数器(不存在则创建)。nil 接收者返回 nil(调用即 no-op)。
func (v *CounterVec) With(labelValue string) *Counter {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.values == nil {
		v.values = make(map[string]*Counter)
	}
	c, ok := v.values[labelValue]
	if !ok {
		c = &Counter{}
		v.values[labelValue] = c
	}
	return c
}

// Inc 等价于 With(labelValue).Inc(),省掉热路径上的临时变量。
func (v *CounterVec) Inc(labelValue string) { v.With(labelValue).Inc() }

// Counter 取(或注册)一个计数器。同名同类型复用同一个对象。
func (r *Registry) Counter(name, help string) *Counter {
	f := r.register(name, help, kindCounter, "")
	if f == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.c == nil {
		f.c = &Counter{}
	}
	return f.c
}

// CounterVec 取(或注册)一个单标签计数器集合。
func (r *Registry) CounterVec(name, help, label string) *CounterVec {
	f := r.register(name, help, kindCounter, label)
	if f == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.vec == nil {
		f.vec = &CounterVec{label: label}
	}
	return f.vec
}

// Gauge 取(或注册)一个仪表盘。
func (r *Registry) Gauge(name, help string) *Gauge {
	f := r.register(name, help, kindGauge, "")
	if f == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.g == nil {
		f.g = &Gauge{}
	}
	return f.g
}

// Histogram 取(或注册)一个直方图。buckets 会被升序复制一份作为桶上界。
func (r *Registry) Histogram(name, help string, buckets []float64) *Histogram {
	f := r.register(name, help, kindHistogram, "")
	if f == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.h == nil {
		bounds := append([]float64(nil), buckets...)
		sort.Float64s(bounds)
		f.h = &Histogram{bounds: bounds, counts: make([]float64, len(bounds)+1)}
	}
	return f.h
}

// WriteText 按 Prometheus 文本格式 0.0.4 导出全部指标。
//
// 输出顺序:指标族按名字字典序(抓取端不在乎顺序,但人要能 diff 两次抓取);
// 族内 HELP 必须在 TYPE 之前,样本行在两者之后。
func (r *Registry) WriteText(w io.Writer) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	names := make([]string, 0, len(r.families))
	families := make(map[string]*family, len(r.families))
	for name, f := range r.families {
		names = append(names, name)
		families[name] = f
	}
	r.mu.RUnlock()
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		f := families[name]
		// 空指标族不输出任何行:吐一个 "0" 样本会让时序库里留下一段假数据。
		if !f.hasSamples() {
			continue
		}
		fmt.Fprintf(&b, "# HELP %s %s\n", familyName(f), escapeHelp(f.help))
		fmt.Fprintf(&b, "# TYPE %s %s\n", familyName(f), f.kind)
		switch f.kind {
		case kindCounter:
			if f.vec != nil {
				f.vec.write(&b, name)
			} else {
				fmt.Fprintf(&b, "%s %s\n", name, formatFloat(f.c.value()))
			}
		case kindGauge:
			fmt.Fprintf(&b, "%s %s\n", name, formatFloat(f.g.value()))
		case kindHistogram:
			f.h.write(&b, name)
		}
	}
	if b.Len() == 0 {
		return nil
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// familyName 是 # HELP / # TYPE 里用的名字。
// CounterVec 的族名就是向量名(样本行才带标签),无需额外后缀。
func familyName(f *family) string { return f.name }

func (f *family) hasSamples() bool {
	switch {
	case f.vec != nil:
		return f.vec.size() > 0
	case f.kind == kindCounter:
		// 已注册的计数器即使是 0 也要导出:它表示"这个指标已接线",
		// 不导出会让时序库在接线上线前后的曲线出现断层。
		return f.c != nil
	case f.kind == kindGauge:
		// Gauge 的 0 是有意义的("队列为空"),必须导出。
		return f.g != nil
	case f.kind == kindHistogram:
		return f.h.samplesCount() > 0
	}
	return false
}

func (v *CounterVec) size() int {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.values)
}

func (v *CounterVec) write(b *strings.Builder, name string) {
	v.mu.Lock()
	values := make([]string, 0, len(v.values))
	counters := make(map[string]*Counter, len(v.values))
	for val, c := range v.values {
		values = append(values, val)
		counters[val] = c
	}
	v.mu.Unlock()
	sort.Strings(values)
	for _, val := range values {
		fmt.Fprintf(b, "%s{%s=%s} %s\n", name, v.label, quoteLabel(val), formatFloat(counters[val].value()))
	}
}

// familyName 由 registry 在导出时通过名字参数传入(CounterVec/Histogram 共用),
// 向量本身不另存一份名字,避免两处名字漂移。
func (h *Histogram) samplesCount() float64 {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.samples
}

func (h *Histogram) write(b *strings.Builder, name string) {
	h.mu.Lock()
	bounds := append([]float64(nil), h.bounds...)
	counts := append([]float64(nil), h.counts...)
	sum, samples := h.sum, h.samples
	h.mu.Unlock()

	var cumulative float64
	for i, bound := range bounds {
		cumulative += counts[i]
		fmt.Fprintf(b, "%s_bucket{le=%s} %s\n", name, quoteLabel(formatFloat(bound)), formatFloat(cumulative))
	}
	cumulative += counts[len(counts)-1]
	fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %s\n", name, formatFloat(cumulative))
	fmt.Fprintf(b, "%s_sum %s\n", name, formatFloat(sum))
	fmt.Fprintf(b, "%s_count %s\n", name, formatFloat(samples))
}

// formatFloat 用最短往返表示,整数不带 ".0"(3 而不是 3.0)。
// +Inf/NaN 在样本值里没有合法写法,退化成能解析的数字,避免抓取端整族丢弃。
func formatFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func quoteLabel(v string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return `"` + replacer.Replace(v) + `"`
}

func escapeHelp(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	return replacer.Replace(s)
}
