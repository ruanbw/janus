# ip2region 离线数据

本目录是 ip2region 的 **xdb 离线库**,由 Go 代码用 `go:embed` 编进二进制,
运行期不做任何网络请求、不需要挂载卷、不需要 API Key(ADR 0009)。

| 文件 | 大小 | 覆盖 |
| --- | --- | --- |
| `ip2region_v4.xdb` | 10.6 MB | IPv4,精确到城市 |
| `ip2region_v6.xdb` | 35.6 MB | IPv6,精确到城市 |

两个文件都收:只带 IPv4 的话,IPv6 访客会**查不到国家**,而规则里的
国家条件是"取不到恒不命中"——行为正确但覆盖不全,等于给 IPv6 用户
发了一张沉默的通行证。

## 升级方式

数据源是 `github.com/lionsoul2014/ip2region`(纯数据仓库,不含 Go 代码;
Go 客户端是另一个模块 `github.com/lionsoul2014/ip2region/binding/golang`)。

```bash
go get github.com/lionsoul2014/ip2region@v3.18.0+incompatible
M=$(go env GOMODCACHE)/github.com/lionsoul2014/ip2region@v3.18.0+incompatible
cp $M/data/ip2region_v4.xdb $M/data/ip2region_v6.xdb internal/geo/data/
```

## region 字段格式

查询结果是一行 `国家|省份|城市|ISP|iso-alpha2-code`,本项目只取第 5 段
(ISO 3166-1 alpha-2 国家码,与规则 `country` 条件的取值口径一致)。
`0` 是 ip2region 对"查不到"的占位(内网/回环会返回
`Reserved|Reserved|Reserved|0|0`),实现里必须当成空值。

## 许可

ip2region 双许可 Apache-2.0 / MIT,可自由分发 —— 这也是选它而不选
GeoLite2 的原因之一(GeoLite2 需要注册拿 key,且许可不允许再分发)。
