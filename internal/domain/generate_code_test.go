package domain

// 短码生成的既有行为契约:长度、字符集、随机性。
//
// 随机源从 math/rand/v2 换成 crypto/rand(口径与 MintVerifyToken 统一)不改变
// 任何可观察行为,这组测试就是那条"不变量"的守门人:换源之后字符集/长度/分布
// 必须与之前一致,否则前端与 e2e(短码字母表刻意去掉易混淆字符)都会踩坑。

import (
	"strings"
	"testing"
)

func TestGenerateCodeShapeAndAlphabet(t *testing.T) {
	for _, n := range []int{1, AutoCodeLength, 32, MaxCodeLen} {
		got := GenerateCode(n)
		if len(got) != n {
			t.Fatalf("GenerateCode(%d) 长度 = %d", n, len(got))
		}
		if !IsValidCode(got) {
			t.Fatalf("GenerateCode(%d) = %q 不是合法短码(字符集之外?):短码会被 IsValidCode 拒掉", n, got)
		}
		for _, c := range got {
			if !strings.ContainsRune(CodeAlphabet, c) {
				t.Fatalf("GenerateCode(%d) = %q 含字符集之外的字符 %q", n, got, c)
			}
		}
	}
	// 长度 0 合法(调用方不会这么用,但不能 panic)
	if got := GenerateCode(0); got != "" {
		t.Fatalf("GenerateCode(0) = %q, want 空串", got)
	}
}

// 短码必须真的随机:批量生成几乎不重复(短码是公开凭据,可预测就可被猜到)。
func TestGenerateCodeIsRandom(t *testing.T) {
	const n = 200
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		seen[GenerateCode(AutoCodeLength)] = struct{}{}
	}
	if len(seen) < n-2 {
		t.Fatalf("%d 次生成只得到 %d 个不同短码:随机性不足", n, len(seen))
	}
}

// 字符集里的每个字符都必须可能被取到(拒绝采样的上界不能把尾部字符排除掉)。
func TestGenerateCodeCoversWholeAlphabet(t *testing.T) {
	seen := make(map[rune]bool, len(CodeAlphabet))
	// 每个字符期望出现约 1/56;取足够多的样本,让"某个字符永不出现"这种偏置暴露出来。
	for i := 0; i < 2000 && len(seen) < len(CodeAlphabet); i++ {
		for _, c := range GenerateCode(AutoCodeLength) {
			seen[c] = true
		}
	}
	for _, c := range CodeAlphabet {
		if !seen[c] {
			t.Fatalf("字符 %q 在 2000 个短码里一次都没出现:取样有偏(拒绝采样的边界错了?)", c)
		}
	}
}
