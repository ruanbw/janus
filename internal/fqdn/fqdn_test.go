package fqdn

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Example.com", "example.com"},
		{"example.com.", "example.com"},
		{"  EXAMPLE.COM.  ", "example.com"},
		{"example.com..", "example.com"},
		{"", ""},
		{"   ", ""},
		{"localhost", "localhost"},
		{"a.b.c.d.", "a.b.c.d"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsValid(t *testing.T) {
	valid := []string{
		"example.com", "EXAMPLE.COM", "example.com.", "a-b.example.com",
		"localhost", "xn--80ak6aa92e.com", "1.example.com", "sub.domain.co.uk",
	}
	for _, s := range valid {
		if !IsValid(s) {
			t.Errorf("IsValid(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"", "   ", ".", "..", "example..com", "-example.com", "example-.com",
		"exa mple.com", "example.com/path", "http://example.com",
		"exam_ple.com", "localhost:8080",
	}
	for _, s := range invalid {
		if IsValid(s) {
			t.Errorf("IsValid(%q) = true, want false", s)
		}
	}
}

// 边界:总长 253 上限与单标签 63 上限,防止超长字符串绕过索引/协议限制。
func TestIsValid_LengthBounds(t *testing.T) {
	long := ""
	for i := 0; i < 63; i++ {
		long += "a"
	}
	if !IsValid(long) { // 单标签 63 恰好合法
		t.Errorf("单标签 63 字节应合法")
	}
	if IsValid(long + "a") { // 64 越界
		t.Errorf("单标签 64 字节应非法")
	}
	if IsValid(long + "." + long + "." + long + "." + long) { // 259 > 253
		t.Errorf("总长超 253 应非法")
	}
}

// IsValid 必须内部先归一化:否则 "EXAMPLE.COM." 这类合法输入会被判非法,
// 而旧的 domain.IsValidFQDN 一直是接受的(包装不得改变既有语义)。
func TestIsValid_NormalizesFirst(t *testing.T) {
	if !IsValid("  WWW.Example.COM.  ") {
		t.Errorf("带空白/大写/尾点的输入应先归一化再校验,结果应合法")
	}
}
