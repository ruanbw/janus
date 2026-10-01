package mailer

import (
	"strings"
	"testing"
)

func TestSMTPMailerConfigValidation(t *testing.T) {
	mailer := &SMTPMailer{
		cfg: SMTPConfig{
			Host: "smtp.example.com",
			Port: 587,
		},
		baseURL: "http://app.cloak.test",
	}

	// 缺少 From 和 Username 时应该直接报错
	err := mailer.SendVerifyEmail("user@example.com", "token123")
	if err == nil {
		t.Fatal("expected error when From and Username are empty")
	}
	if !strings.Contains(err.Error(), "无法确定发件人地址") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 补全 From 之后，由于目标主机不可达，应该在 dial 时返回连接错误而非 panic
	mailer.cfg.From = "no-reply@cloak.test"
	err = mailer.SendVerifyEmail("user@example.com", "token123")
	if err == nil {
		t.Fatal("expected connection error to non-existent host")
	}
}

func TestSMTPMailerMessagePreparation(t *testing.T) {
	mailer := &SMTPMailer{
		cfg: SMTPConfig{
			Host:     "127.0.0.1",
			Port:     465,
			Username: "sender@cloak.test",
			Password: "secretpassword",
		},
		baseURL: "https://app.cloak.test",
	}

	err := mailer.SendResetEmail("target@cloak.test", "reset_token_xyz")
	if err == nil {
		t.Fatal("expected connection error to 127.0.0.1:465")
	}
}
