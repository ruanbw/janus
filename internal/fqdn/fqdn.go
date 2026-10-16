// Package fqdn 是域名归一化与格式校验的**唯一实现**。
//
// 为什么单独抽一个无依赖叶子包:归一化(FQDN → 唯一键)是域名唯一性、读路径命中、
// 授权放行三处的共同前提,而它此前有两份逐字相同的物理隔离实现
// (internal/domain 与 internal/store 各一份)。两份实现漂移时没有编译器兜底,
// 一旦漂移,UNIQUE 索引就会被 "a.com"/"a.com." 这种同主机双份记录绕过。
//
// 单独成包而不是放在 domain 里:config 需要在解析环境变量时调用归一化,而
// domain 已经 import config(domain/worker.go),config 再 import domain 必成环。
// config 是依赖图叶子,所以这里必须是一个不依赖任何东西的包。
package fqdn

import "strings"

// Normalize 归一化域名:去首尾空白 → 转小写 → 去尾点。
//
// "Example.com" 与 "example.com." 必须归一到同一个键,否则 UNIQUE 索引放过同主机的
// 双份记录,读路径(FQDN → 域名、授权端点)也会因为 Host 头形态不一致而查不到行。
func Normalize(s string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(s)), ".")
}

// IsValid 校验域名格式:点分标签,字母/数字/连字符,标签不以连字符开头结尾。
// 允许单标签(如 localhost,开发环境 DNS 校验用)。
//
// 注意这里**先 Normalize 再校验**:这不是新增的宽容行为,而是原样保留被它取代的
// domain.IsValidFQDN 的既有语义(旧实现第一行就是 s = NormalizeFQDN(s))。
// 包装函数不得改变被包装者的可观察行为,否则所有现存调用点的语义会静默改变。
func IsValid(s string) bool {
	s = Normalize(s)
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
			if !ok || (c == '-' && (i == 0 || i == len(label)-1)) {
				return false
			}
		}
	}
	return true
}
