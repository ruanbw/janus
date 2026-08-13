// Package mailer 定义可插拔邮件发送接口(spec 决策 #3)。
// 配置 SMTP 时使用真实邮件发送;未配置时退回控制台 mailer(开发环境)。
package mailer

import (
	"fmt"
	"io"
	"strings"
)

// Mailer 邮件发送接口(验证邮件 / 密码重置邮件)。
type Mailer interface {
	SendVerifyEmail(to, token string) error
	SendResetEmail(to, token string) error
}

// SMTPConfig SMTP 服务器配置;nil 表示未配置(使用控制台 mailer)。
type SMTPConfig struct {
	Host     string // 如 smtp.163.com
	Port     int    // 465(SMTPS)或 587(STARTTLS)
	Username string
	Password string
	From     string // 发件人地址;空则用 Username
}

// Config mailer 通用配置。
type Config struct {
	BaseURL string // 后台访问地址(邮件中的验证/重置链接前缀),如 https://app.cloak.test
	SMTP    *SMTPConfig
}

// NewMailer 按配置选择实现:配置了 SMTP 用 SMTPMailer,否则 ConsoleMailer。
// w 仅控制台模式使用;SMTP 模式下忽略。
func NewMailer(cfg Config, w io.Writer) Mailer {
	if cfg.SMTP != nil && cfg.SMTP.Host != "" {
		return &SMTPMailer{cfg: *cfg.SMTP, baseURL: strings.TrimRight(cfg.BaseURL, "/")}
	}
	return NewConsoleMailer(w, cfg.BaseURL)
}

func verifyURL(baseURL, token string) string {
	return fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
}

func resetURL(baseURL, token string) string {
	return fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
}

// ConsoleMailer 把邮件输出到指定 writer(开发环境用,无需真实 SMTP)。
type ConsoleMailer struct {
	w       io.Writer
	baseURL string
}

func NewConsoleMailer(w io.Writer, baseURL string) *ConsoleMailer {
	return &ConsoleMailer{w: w, baseURL: strings.TrimRight(baseURL, "/")}
}

func (m *ConsoleMailer) SendVerifyEmail(to, token string) error {
	_, err := fmt.Fprintf(m.w,
		"\n[CLOAK mailer] 邮箱验证 %s\n  token: %s\n  验证地址: %s\n\n",
		to, token, verifyURL(m.baseURL, token))
	return err
}

func (m *ConsoleMailer) SendResetEmail(to, token string) error {
	_, err := fmt.Fprintf(m.w,
		"\n[CLOAK mailer] 密码重置 %s\n  token: %s\n  重置地址: %s\n\n",
		to, token, resetURL(m.baseURL, token))
	return err
}
