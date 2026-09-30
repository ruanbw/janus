package geo

import (
	"strconv"
	"testing"
)

func TestCountryOf(t *testing.T) {
	cases := []struct {
		name   string
		region string
		want   string
	}{
		{"境外五段", "United States|California|0|Google LLC|US", "US"},
		{"境内五段", "中国|江苏省|南京市|0|CN", "CN"},
		{"有 ISP 的境内段", "中国|浙江省|杭州市|阿里|CN", "CN"},
		// 最要紧的一条:0 是 ip2region 的"查不到"占位,当成国家码会命中
		// 一条 country=0 的规则,而它在界面上看着像个正常国家码
		{"保留地址的 0", "Reserved|Reserved|Reserved|0|0", ""},
		{"IPv6 回环返回空串", "", ""},
		{"字段数不够", "中国|江苏省|南京市", ""},
		{"国家码位置是占位", "Reserved|Reserved|Reserved|0| ", ""},
		{"小写国家码归一", "United States|California|0|Google LLC|us", "US"},
		{"非字母", "x|中国|江苏|南京|1!", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countryOf(tc.region); got != tc.want {
				t.Fatalf("countryOf(%q) = %q, want %q", tc.region, got, tc.want)
			}
		})
	}
}

// TestXDBLookupKnownAddresses 跑真实库。embed 46MB 数据会让测试二进制变很大,
// 只在完整跑时执行(go test -short ./... 跳过)。
func TestXDBLookupKnownAddresses(t *testing.T) {
	if testing.Short() {
		t.Skip("需要内嵌的 ip2region 库,short 模式跳过")
	}
	l, err := NewXDB()
	if err != nil {
		t.Fatalf("NewXDB: %v", err)
	}
	cases := []struct{ ip, want string }{
		{"8.8.8.8", "US"},
		{"223.5.5.5", "CN"},
		// 以下三个都必须查不到:私网、回环、非法串
		{"192.168.1.1", ""},
		{"127.0.0.1", ""},
		{"::1", ""},
		{"", ""},
		{"not-an-ip", ""},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := l.Lookup(tc.ip).Country; got != tc.want {
				t.Fatalf("Lookup(%q).Country = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

// 缓存必须记住负结果:查不到的 IP 每次都要重新查的话,散落在未知网段里的
// 流量会把查询成本打满,而缓存的收益恰恰主要来自这些地址。
func TestCachedRemembersNegativeResults(t *testing.T) {
	calls := 0
	counting := LookupFunc(func(string) Info {
		calls++
		return Info{}
	})
	c := Cached(counting, 64)
	for i := 0; i < 5; i++ {
		c.Lookup("203.0.113.9")
	}
	if calls != 1 {
		t.Fatalf("负结果被缓存失效:底层被调用 %d 次,应为 1", calls)
	}
}

func TestCachedPerShardEviction(t *testing.T) {
	calls := 0
	counting := LookupFunc(func(string) Info {
		calls++
		return Info{Country: "US"}
	})
	// 上限 16 条 / 16 分片 = 每片 1 条,连续写入必然触发换代
	c := Cached(counting, 16)
	for i := 0; i < 200; i++ {
		c.Lookup(ipForIndex(i))
	}
	// 刚写进去的最后一个一定命中
	c.Lookup(ipForIndex(199))
	if calls < 200 {
		t.Fatalf("缓存写入后重复查询应命中,调用次数 %d", calls)
	}
}

// LookupFunc 把普通函数适配成 Lookup,测试里省掉一堆样板。
type LookupFunc func(ip string) Info

func (f LookupFunc) Lookup(ip string) Info { return f(ip) }

func ipForIndex(i int) string {
	return "198.51.100." + strconv.Itoa(i+1)
}
