// Package geo 由 IP 解析出地理值,供规则求值与访问明细共用。
//
// 接口按能力切(ADR 0009),不按供应商切:调用方要的是"这个 IP 在哪",
// 不是"这个 IP 在 MaxMind 里被标成什么"。换数据源只换实现,调用方不动。
//
// 取不到值时一律返回空值(不是猜测、不是默认国家)。这条是包里最重要的不变式:
// country 条件在空值下恒不命中,而不是"因为取不到就当匹配"或"当不匹配的反面"。
// 一个假的国家码会让租户的风控规则在故障时悄悄失效。
package geo

// Info 一个 IP 的地理值。字段语义与 rules.Fact 的 Country / ASN 对齐。
//
// 三个字段都是"能拿到才有值",没有数据源的字段恒为零值——
// 不用哨兵值、不用估算值占位,调用方按"空即查不到"处理即可。
type Info struct {
	// Country ISO 3166-1 alpha-2 国家码,如 "US" / "CN";查不到时为空。
	// 存码不存国名:规则条件、数据库列与这份数据用的是同一套码,前端按需翻译。
	Country string
	// ASN 自治系统号。当前静态库不含该数据,恒为空。
	ASN string
	// IsDatacenter 是否机房 IP。当前静态库不含该数据,恒为 false。
	// 规则侧也还没有对应可判定字段——要"拦截机房 IP"得另开一个决策。
	IsDatacenter bool
}

// Lookup 由 IP 取地理值。实现必须线程安全:跳转链路是并发的。
//
// 返回零值 Info 表示"查不到",调用方必须按"取不到"处理(见包注释)。
type Lookup interface {
	Lookup(ip string) Info
}

// disabled 是没有数据源时的实现:恒返回零值。
// 显式存在而不是让调用方判 nil:调用方(跳转热路径)不该为"可能没有数据源"
// 写分支,那会让"没配数据源"和"配了但查不到"两条路径在代码里长得不一样。
type disabled struct{}

func (disabled) Lookup(string) Info { return Info{} }

// Disabled 恒返回空地理值的 Lookup(无数据源 / 数据源加载失败时的兜底)。
var Disabled Lookup = disabled{}
