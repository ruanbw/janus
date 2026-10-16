package config

import (
	"strings"
	"testing"
)

// TestValidateJWTSecretMinLength 钉住非空 JWTSecret 的最小长度(≥32 字节)。
// 空密钥的处置(生产模式拒绝启动)由另一条工作流负责,这里只覆盖"已配置密钥"。
func TestValidateJWTSecretMinLength(t *testing.T) {
	t.Run("Given 非空但过短的 JWTSecret When Validate Then 报错", func(t *testing.T) {
		for _, short := range []string{"x", "test-jwt-secret", strings.Repeat("a", 31)} {
			cfg := validConfig()
			cfg.JWTSecret = short
			if err := cfg.Validate(); err == nil {
				t.Fatalf("JWTSecret 长度 %d 应被拒绝", len(short))
			}
		}
	})

	t.Run("Given 恰好 32 字节的 JWTSecret When Validate Then 通过", func(t *testing.T) {
		cfg := validConfig()
		cfg.JWTSecret = strings.Repeat("a", 32)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("32 字节密钥应通过,got %v", err)
		}
	})

	t.Run("Given 空 JWTSecret When Validate Then 不因此报错(由生产模式开关负责)", func(t *testing.T) {
		cfg := validConfig()
		cfg.JWTSecret = ""
		if err := cfg.Validate(); err != nil {
			t.Fatalf("空密钥不应在本校验中报错,got %v", err)
		}
	})
}
