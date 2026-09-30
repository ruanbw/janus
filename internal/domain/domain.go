// Package domain 承载域名生命周期、跳转相关的领域逻辑:
// DNS 激活校验、证书预签发探活、短码生成与校验、后台任务。
package domain

import (
	"context"
	"math/rand/v2"
	"net"
	"strings"
)

// CodeAlphabet 短码字符集:a-zA-Z0-9 去掉易混淆字符 0/O/1/l/I(spec 决策)。
// 小写去 l,大写去 I/O,数字去 0/1,共 56 个字符。
const CodeAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// MaxCodeLen 短码最大长度(自定义短码)。
const MaxCodeLen = 64

// AutoCodeLength 自动生成短码的固定长度(租户不可配)。
const AutoCodeLength = 6

// MaxDomainDescriptionLen 域名描述最大长度(按字符计,前端 textarea maxlength 同步此值)。
const MaxDomainDescriptionLen = 200

// IsValidCode 校验短码合法性(字符集 + 长度 1..64)。
func IsValidCode(s string) bool {
	if s == "" || len(s) > MaxCodeLen {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune(CodeAlphabet, c) {
			return false
		}
	}
	return true
}

// GenerateCode 随机生成指定长度的短码(均匀取自 CodeAlphabet)。
func GenerateCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = CodeAlphabet[rand.IntN(len(CodeAlphabet))]
	}
	return string(b)
}

// IsValidSlug 校验租户前缀 slug:小写字母/数字开头结尾,中间可含连字符,长度 1..63。
func IsValidSlug(s string) bool {
	if len(s) < 1 || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// DNSChecker 校验域名 A/AAAA 记录是否包含本服务器 IP。
// 使用 Go 默认解析器(读取 /etc/hosts),开发环境 hosts 指向 127.0.0.1 即走真实代码路径通过。
type DNSChecker struct {
	ExpectedIP string
}

// Check 返回域名是否解析到 ExpectedIP。解析失败或未命中均视为"未通过"(不视为错误)。
func (c *DNSChecker) Check(ctx context.Context, fqdn string) (bool, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, fqdn)
	if err != nil {
		return false, nil
	}
	for _, a := range addrs {
		if a.IP.String() == c.ExpectedIP {
			return true, nil
		}
	}
	return false, nil
}
