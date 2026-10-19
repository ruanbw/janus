package metrics

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// 本文件的测试锁定"注册表本身"的语义:同名收敛、类型冲突、nil 接收者、并发。
// 文本导出格式与 Histogram 桶语义见 registry_exposition_test.go。

// TestRegistrySameNameSameTypeReturnsSameInstance 固化"同名同类型注册收敛到同一个对象"。
//
// 为什么重要:调用点分散在 handler、规则求值、后台任务里,每个地方都写一次
// registry.Counter("janus_xxx_total", ...) 是常态。若同名注册各返回新对象,
// 各调用点会各记各的计数,exposition 里只出现其中一份,线上表现是
// "指标数值永远对不上日志",而排查时几乎不会怀疑到注册表头上。
func TestRegistrySameNameSameTypeReturnsSameInstance(t *testing.T) {
	r := New()

	t.Run("Given 同名同类型注册两次, When 比较返回值, Then 是同一个对象", func(t *testing.T) {
		first := r.Counter("janus_same_total", "同类型重复注册。")
		second := r.Counter("janus_same_total", "同类型重复注册。")
		if first != second {
			t.Fatalf("同名同类型重复注册返回了不同对象: %p != %p", first, second)
		}

		// 计数不能被清零:通过两个句柄各加一次,总量必须是 2。
		first.Add(1)
		second.Add(1)
		body := mustWriteText(t, r)
		if got, ok := sampleValue(t, body, "janus_same_total"); !ok || got != 2 {
			t.Fatalf("重复注册后计数被清零: 期望 janus_same_total 2, 得到 %v (present=%v)\n%s", got, ok, body)
		}
	})
}

// TestRegistryDuplicateNameDifferentTypePanics 固化"同名不同类型必须 panic"。
//
// 为什么重要:同名不同类型几乎只可能是笔误(复制粘贴忘了改后缀、把 gauge 写成
// counter)。静默覆盖会让先注册的那个指标从 exposition 里彻底消失,而且不会有人
// 发现 —— 启动即 panic 才是唯一能让人当场看见的信号。panic 文本带
// "duplicate metric" 是给运维的定位线索,不是给程序的判断依据。
func TestRegistryDuplicateNameDifferentTypePanics(t *testing.T) {
	cases := []struct {
		name   string
		first  func(r *Registry)
		second func(r *Registry)
	}{
		{
			name:   "Given 已注册 Counter, When 再注册 Gauge, Then panic duplicate metric",
			first:  func(r *Registry) { r.Counter("janus_dup", "计数器。") },
			second: func(r *Registry) { r.Gauge("janus_dup", "仪表盘。") },
		},
		{
			name:   "Given 已注册 Gauge, When 再注册 Counter, Then panic duplicate metric",
			first:  func(r *Registry) { r.Gauge("janus_dup", "仪表盘。") },
			second: func(r *Registry) { r.Counter("janus_dup", "计数器。") },
		},
		{
			name:   "Given 已注册 CounterVec, When 再注册 Counter, Then panic duplicate metric",
			first:  func(r *Registry) { r.CounterVec("janus_dup", "带标签计数器。", "result") },
			second: func(r *Registry) { r.Counter("janus_dup", "计数器。") },
		},
		{
			name:   "Given 已注册 Histogram, When 再注册 Gauge, Then panic duplicate metric",
			first:  func(r *Registry) { r.Histogram("janus_dup", "直方图。", []float64{1}) },
			second: func(r *Registry) { r.Gauge("janus_dup", "仪表盘。") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New()
			tc.first(r)
			defer func() {
				rec := recover()
				if rec == nil {
					t.Errorf("同名不同类型注册没有 panic:先注册的指标会被静默覆盖,exposition 里再无它的踪迹")
					return
				}
				if msg := fmt.Sprint(rec); !strings.Contains(msg, "duplicate metric") {
					t.Errorf("panic 文本 %q 不含 %q:这句话是运维定位写错指标名的唯一线索", msg, "duplicate metric")
				}
			}()
			tc.second(r)
		})
	}
}

// TestNilMetricReceiversAreNoOps 固化"未接线时零成本"。
//
// 为什么重要:Deps.Metrics 为空是合法部署(单机调试、还没配抓取)。此时调用点
// 拿到的是零值指针,而调用点不应被迫写 if x != nil —— 那既啰嗦又容易漏。
// 所以「nil 接收者 = no-op」是这套 API 的契约,不是容错。
func TestNilMetricReceiversAreNoOps(t *testing.T) {
	t.Run("Given nil *Counter, When 调用 Inc 与 Add, Then 不 panic", func(t *testing.T) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("nil *Counter 调用 panic: %v", rec)
			}
		}()
		var c *Counter
		c.Inc()
		c.Add(3)
	})

	t.Run("Given nil *Gauge, When 调用 Set 与 Add, Then 不 panic", func(t *testing.T) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("nil *Gauge 调用 panic: %v", rec)
			}
		}()
		var g *Gauge
		g.Set(1)
		g.Add(1)
	})

	t.Run("Given nil *Histogram, When 调用 Observe, Then 不 panic", func(t *testing.T) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("nil *Histogram 调用 panic: %v", rec)
			}
		}()
		var h *Histogram
		h.Observe(0.5)
	})

	t.Run("Given nil *Counter, When 并发调用, Then 不 panic", func(t *testing.T) {
		// 真实调用点就在并发路径上(跳转热路径、规则求值),nil 保护若只做在
		// 入口而没做在方法里,并发下会露出 race 或 panic。
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("nil *Counter 并发调用 panic: %v", rec)
			}
		}()
		var c *Counter
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					c.Inc()
				}
			}()
		}
		wg.Wait()
	})
}

// TestCounterIncrementIsConcurrencySafe 固化"并发自增不丢计数"。
//
// 为什么重要:指标的唯一价值就是准确。丢更新的实现(裸 int64 自增)在低流量下
// 看不出问题,一到大促/热点短链就开始少记,而这种偏差永远无法事后补回。
// 期望值取精确的 50×200=10000,而不是"大约"。
func TestCounterIncrementIsConcurrencySafe(t *testing.T) {
	const (
		goroutines   = 50
		perGoroutine = 200
	)
	r := New()

	t.Run("Given 50 个 goroutine, When 每个 Inc 200 次, Then 精确等于 10000", func(t *testing.T) {
		c := r.Counter("janus_concurrent_total", "并发自增计数。")
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < perGoroutine; j++ {
					c.Inc()
				}
			}()
		}
		wg.Wait()

		want := float64(goroutines * perGoroutine)
		body := mustWriteText(t, r)
		if got, ok := sampleValue(t, body, "janus_concurrent_total"); !ok || got != want {
			t.Fatalf("并发自增丢了计数: 期望 %v, 得到 %v (present=%v)", want, got, ok)
		}
	})
}

// TestGaugeAddIsConcurrencySafe 固化"并发 Gauge.Add 不丢增量"。
//
// Gauge 的典型用法是 inflight 计数/队列长度:增减必须可交换、可累加。
// 与 Counter 同理,期望值是精确值。
func TestGaugeAddIsConcurrencySafe(t *testing.T) {
	const (
		goroutines   = 50
		perGoroutine = 200
	)
	r := New()

	t.Run("Given 50 个 goroutine, When 每个 Add(1) 200 次, Then 精确等于 2000", func(t *testing.T) {
		g := r.Gauge("janus_inflight", "并发增量仪表盘。")
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < perGoroutine; j++ {
					g.Add(1)
				}
			}()
		}
		wg.Wait()

		want := float64(goroutines * perGoroutine)
		body := mustWriteText(t, r)
		if got, ok := sampleValue(t, body, "janus_inflight"); !ok || got != want {
			t.Fatalf("Gauge.Add 丢了增量: 期望 %v, 得到 %v (present=%v)", want, got, ok)
		}
	})
}

// TestGaugeSetReplacesValue 固化 Set 与 Add 的语义差异。
//
// 为什么重要:两者写反是常见笔误 —— 用 Add 当 Set 会让 Gauge 单调爬升,
// 用 Set 当 Add 会让计数永远停在最后一次增量。
func TestGaugeSetReplacesValue(t *testing.T) {
	r := New()
	g := r.Gauge("janus_queue_depth", "队列长度。")

	g.Set(5)
	g.Add(2)
	if got, ok := sampleValue(t, mustWriteText(t, r), "janus_queue_depth"); !ok || got != 7 {
		t.Fatalf("Set(5) 后 Add(2) 期望 7, 得到 %v (present=%v)", got, ok)
	}

	g.Set(1)
	if got, ok := sampleValue(t, mustWriteText(t, r), "janus_queue_depth"); !ok || got != 1 {
		t.Fatalf("Set(1) 必须覆盖旧值, 得到 %v (present=%v)", got, ok)
	}
}

// TestCounterVecWithReturnsCounterForLabel 固化"With 复用同一个标签值对应的 Child"。
//
// 为什么重要:With 是热路径上最常见的写法(CounterVec.With(tenant).Inc())。
// 每次 With 都新建一个 Child 会让同一标签值的计数被拆成多份,exposition 里
// 只剩最后一份,标签维度就失效了。
func TestCounterVecWithReturnsCounterForLabel(t *testing.T) {
	r := New()
	vec := r.CounterVec("janus_rule_failopen_total", "规则 fail-open 次数。", "reason")

	vec.With("cache_miss").Inc()
	vec.With("cache_miss").Add(2)
	vec.With("db_error").Inc()

	body := mustWriteText(t, r)
	if got, ok := sampleValue(t, body, `janus_rule_failopen_total{reason="cache_miss"}`); !ok || got != 3 {
		t.Fatalf("同一标签值的计数应累加到 3, 得到 %v (present=%v)\n%s", got, ok, body)
	}
	if got, ok := sampleValue(t, body, `janus_rule_failopen_total{reason="db_error"}`); !ok || got != 1 {
		t.Fatalf("不同标签值必须独立计数, 得到 %v (present=%v)\n%s", got, ok, body)
	}
}
