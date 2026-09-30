package geo

// ip2region 静态库实现:两个 xdb 文件用 go:embed 编进二进制,运行期零 IO。
// 数据文件说明与升级方式见 data/README.md。

import (
	_ "embed"
	"fmt"
	"net"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

//go:embed data/ip2region_v4.xdb
var xdbV4 []byte

//go:embed data/ip2region_v6.xdb
var xdbV6 []byte

// region 字段数与下标。ip2region 的 region 串格式为
// 国家|省份|城市|ISP|iso-alpha2-code,本项目只取第 5 段(ISO 国家码)。
const (
	regionFieldCount = 5
	regionCountryIdx = 4
)

// xdbLookup 基于内嵌 xdb 的 Lookup。
//
// 只持有两个不可变的字节切片(embed 出来的,运行期不写),没有别的状态。
type xdbLookup struct{}

// NewXDB 返回基于内嵌 ip2region 库的实现。
// 启动时构造一次;embed 缺失是编译期错误,所以这里返回 error 只可能来自
// 库的版本不匹配——那种情况必须启动即失败,不能等第一次访问才发现国家全是空的。
func NewXDB() (Lookup, error) {
	for _, v := range []struct {
		name string
		ver  *xdb.Version
		buf  []byte
	}{
		{"IPv4", xdb.IPv4, xdbV4},
		{"IPv6", xdb.IPv6, xdbV6},
	} {
		s, err := xdb.NewWithBuffer(v.ver, v.buf)
		if err != nil {
			return nil, fmt.Errorf("ip2region %s 库不可用: %w", v.name, err)
		}
		s.Close()
	}
	return xdbLookup{}, nil
}

func (xdbLookup) Lookup(ip string) Info {
	if ip == "" {
		return Info{}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Info{}
	}
	// 内网/回环/链路本地直接返回:这些地址的地理结果没有意义,连查都不查。
	// (ip2region 对它们会返回 Reserved|...|0,落到下面同样会变成空值,
	//  这里短路只是为了不让开发环境的每一次本地访问都白跑一次二分查找。)
	if parsed.IsLoopback() || parsed.IsPrivate() ||
		parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() ||
		parsed.IsUnspecified() {
		return Info{}
	}
	version, err := xdb.VersionFromIP(ip)
	if err != nil {
		return Info{}
	}
	buf := xdbV4
	if version == xdb.IPv6 {
		buf = xdbV6
	}
	// 每次查询现建一个 Searcher,不复用共享实例:xdb.Searcher 会在 Search 里写
	// 自己的 ioCount 字段(统计磁盘读的计数器),共享一个实例并发查询是数据竞争。
	// NewWithBuffer 在 cBuff 非 nil 时只是组装一个结构体,不做任何校验与 IO,
	// 代价可以忽略;真要省掉这次分配,就得给每次查询上锁,那比分配贵得多。
	searcher, err := xdb.NewWithBuffer(version, buf)
	if err != nil {
		return Info{}
	}
	region, err := searcher.Search(ip)
	if err != nil {
		return Info{}
	}
	// 当前静态库不含 ASN 与机房 IP 数据:两个字段恒为零值,不猜、不填占位。
	return Info{Country: countryOf(region)}
}

// countryOf 从 region 串里取 ISO 国家码;取不到时返回空。
//
// ip2region 用字面量 "0" 表示"查不到"(内网/回环会返回 Reserved|Reserved|Reserved|0|0,
// 未收录的地址第 5 段同样是 0),所以必须把 "0" 判成空值:
// 直接把 "0" 存进 country 会让一条 country=0 的规则匹配上所有查不到的流量,
// 更糟的是它在 UI 上看起来像个正常国家码,没人会去怀疑。
func countryOf(region string) string {
	parts := strings.Split(region, "|")
	if len(parts) < regionFieldCount {
		return ""
	}
	code := strings.ToUpper(strings.TrimSpace(parts[regionCountryIdx]))
	if !isAlpha2(code) {
		return ""
	}
	return code
}

// isAlpha2 校验是不是两个 ASCII 字母:国家码必须是这个形状,
// 认不出来(占位、格式变化、脏数据)就当查不到,绝不把原样透传给规则。
func isAlpha2(s string) bool {
	if len(s) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		c := s[i] | 0x20 // 大小写归一
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
