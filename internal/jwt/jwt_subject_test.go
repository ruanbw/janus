package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestParseRejectsInvalidSubject 钉住 Subject 必须是合法租户 ID(十进制正整数)。
// 畸形 Subject 必须在认证入口被拒,而不是穿透到业务层被当成 0/解析失败静默降级。
func TestParseRejectsInvalidSubject(t *testing.T) {
	m := NewManager(testSecret, time.Hour)

	sign := func(sub string) string {
		t.Helper()
		claims := Claims{
			Role: "tenant",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    Issuer,
				Subject:   sub,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				ID:        "sub-test-jti",
			},
		}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign token (sub=%q): %v", sub, err)
		}
		return tok
	}

	for _, sub := range []string{"abc", "12.5", "0", "-3", " 42", "1e3"} {
		if _, err := m.Parse(sign(sub)); err == nil {
			t.Errorf("Parse(sub=%q): want error, got nil", sub)
		}
	}

	// 合法十进制正整数租户 ID 仍应通过。
	if _, err := m.Parse(sign("42")); err != nil {
		t.Fatalf("Parse(sub=42): want ok, got %v", err)
	}
}
