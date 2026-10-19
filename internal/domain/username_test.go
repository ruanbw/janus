package domain

import "testing"

// 用户名是身份模型的唯一标识(登录/注册/token 三处共用),校验口径必须钉死:
// 它同时决定了 tenant JSON 的 username 字段与登录时的归一化结果。
func TestNormalizeUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Alice  ", "alice"},
		{"SUPERADMIN", "superadmin"},
		{"a_b-1", "a_b-1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeUsername(c.in); got != c.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsValidUsername(t *testing.T) {
	valid := []string{
		"abc",
		"a_b",
		"a-b",
		"123",
		"user_name-1",
		"abcabcabcabcabcabcabcabcabcabcab", // 32 字符(上界)
	}
	for _, s := range valid {
		if !IsValidUsername(s) {
			t.Errorf("IsValidUsername(%q) = false, want true", s)
		}
	}

	invalid := map[string]string{
		"ab":                                "过短(下界 3)",
		"abcabcabcabcabcabcabcabcabcabcabc": "过长(33 字符,上界 32)",
		"_abc":                              "首字符必须是字母或数字",
		"-abc":                              "首字符必须是字母或数字",
		"abc_":                              "尾字符必须是字母或数字",
		"abc-":                              "尾字符必须是字母或数字",
		"a b":                               "不允许空格(归一化只 trim 两端)",
		"Abc":                               "大写必须先归一化,校验口径只认小写",
		"a.b":                               "点号不在字符集内",
		"a@b":                               "邮箱形态不再被接受",
		"":                                  "空串",
	}
	for s, why := range invalid {
		if IsValidUsername(s) {
			t.Errorf("IsValidUsername(%q) = true, want false(%s)", s, why)
		}
	}
}

// 归一化后再校验是调用方的固定顺序:大写输入经归一化必须变成合法用户名,
// 而不是在校验处被拒。
func TestNormalizedUsernamePassesValidation(t *testing.T) {
	for _, in := range []string{"  Alice ", "SUPERADMIN", "User_1 "} {
		if got := NormalizeUsername(in); !IsValidUsername(got) {
			t.Errorf("NormalizeUsername(%q) = %q 应通过校验", in, got)
		}
	}
}
