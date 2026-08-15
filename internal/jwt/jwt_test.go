package jwt

// 单元测试:JWT 签发/解析往返、过期、错误密钥、畸形 token、伪造 issuer。
// 白盒测试(package jwt):直接访问 Manager.secret 构造非法载荷。

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret"

func TestIssueParseRoundTrip(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	token, err := m.Issue(42, "superadmin")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token == "" {
		t.Fatal("Issue returned empty token")
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Subject != "42" {
		t.Errorf("subject = %q, want 42", claims.Subject)
	}
	if claims.Role != "superadmin" {
		t.Errorf("role = %q, want superadmin", claims.Role)
	}
	if claims.Issuer != Issuer {
		t.Errorf("issuer = %q, want %q", claims.Issuer, Issuer)
	}
	if claims.ID == "" {
		t.Error("jti empty, want random id")
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Before(time.Now()) {
		t.Error("expiresAt missing or already expired")
	}
	if claims.IssuedAt == nil || claims.IssuedAt.After(time.Now()) {
		t.Error("issuedAt missing or in the future")
	}
}

func TestExpiredToken(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	// 手工构造已过期载荷(过期时间在过去),用同一密钥签名
	claims := Claims{
		Role: "tenant",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   "1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			ID:        "expired-jti",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	_, err = m.Parse(token)
	if err == nil {
		t.Fatal("Parse expired token: want error, got nil")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("error = %v, want jwt.ErrTokenExpired", err)
	}
}

func TestWrongSecret(t *testing.T) {
	issuer := NewManager("secret-a", time.Hour)
	verifier := NewManager("secret-b", time.Hour)
	token, err := issuer.Issue(7, "tenant")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := verifier.Parse(token); err == nil {
		t.Fatal("Parse with wrong secret: want error, got nil")
	}
}

func TestMalformedToken(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	for _, tok := range []string{
		"",
		"not-a-jwt",
		"abc.def.ghi",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.invalidsig",
	} {
		if _, err := m.Parse(tok); err == nil {
			t.Errorf("Parse(%q): want error, got nil", tok)
		}
	}
}

func TestForgedIssuer(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	// 用合法密钥签发但伪造 issuer 的 token,解析必须拒绝
	claims := Claims{
		Role: "superadmin",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "evil",
			Subject:   "1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			ID:        "forged-jti",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign forged token: %v", err)
	}
	_, err = m.Parse(token)
	if err == nil {
		t.Fatal("Parse forged issuer: want error, got nil")
	}
	if !errors.Is(err, jwt.ErrTokenInvalidIssuer) {
		t.Errorf("error = %v, want jwt.ErrTokenInvalidIssuer", err)
	}
}

func TestMissingExpiration(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	// 缺 exp 声明的 token 也必须拒绝(WithExpirationRequired)
	claims := Claims{
		Role: "tenant",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  Issuer,
			Subject: "1",
			ID:      "no-exp-jti",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if _, err := m.Parse(token); err == nil {
		t.Fatal("Parse without exp: want error, got nil")
	}
}

func TestJTIUnique(t *testing.T) {
	m := NewManager(testSecret, time.Hour)
	a, err := m.Issue(1, "tenant")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	b, err := m.Issue(1, "tenant")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	ca, _ := m.Parse(a)
	cb, _ := m.Parse(b)
	if ca.ID == cb.ID {
		t.Errorf("jti not unique: %q", ca.ID)
	}
}
