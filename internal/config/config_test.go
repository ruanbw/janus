package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigLoadDefaults(t *testing.T) {
	// 清理可能影响测试的环境变量
	os.Unsetenv("CLOAK_ADDR")
	os.Unsetenv("CLOAK_DATABASE_URL")
	os.Unsetenv("CLOAK_COOKIE_SECURE")
	os.Unsetenv("CLOAK_SESSION_TTL")

	cfg := Load()

	if cfg.Addr != ":8080" {
		t.Fatalf("cfg.Addr = %s, want :8080", cfg.Addr)
	}
	if cfg.PlatformDomain != "cloak.test" {
		t.Fatalf("cfg.PlatformDomain = %s, want cloak.test", cfg.PlatformDomain)
	}
	if !cfg.CookieSecure {
		t.Fatal("cfg.CookieSecure want true")
	}
	if cfg.SessionTTL != 30*24*time.Hour {
		t.Fatalf("cfg.SessionTTL = %v, want 720h", cfg.SessionTTL)
	}
	if cfg.LandingMaxZipBytes != 10*1024*1024 {
		t.Fatalf("cfg.LandingMaxZipBytes = %d, want 10485760", cfg.LandingMaxZipBytes)
	}
}

func TestConfigLoadOverrides(t *testing.T) {
	t.Setenv("CLOAK_ADDR", ":9090")
	t.Setenv("CLOAK_COOKIE_SECURE", "false")
	t.Setenv("CLOAK_SESSION_TTL", "12h")
	t.Setenv("CLOAK_SMTP_PORT", "587")

	cfg := Load()

	if cfg.Addr != ":9090" {
		t.Fatalf("cfg.Addr = %s, want :9090", cfg.Addr)
	}
	if cfg.CookieSecure {
		t.Fatal("cfg.CookieSecure want false")
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("cfg.SessionTTL = %v, want 12h", cfg.SessionTTL)
	}
	if cfg.SMTPPort != 587 {
		t.Fatalf("cfg.SMTPPort = %d, want 587", cfg.SMTPPort)
	}
}
