// SMTPMailer 通过真实 SMTP 服务器发送邮件。
// 使用业界高质量开源库 github.com/wneessen/go-mail 驱动。
// 支持两种安全模式:465 隐式 TLS(SMTPS)与 587 STARTTLS(显式升级)。
package mailer

import (
	"errors"
	"fmt"
	"time"

	"github.com/wneessen/go-mail"
)

type SMTPMailer struct {
	cfg     SMTPConfig
	baseURL string
}

func (m *SMTPMailer) SendVerifyEmail(to, token string) error {
	return m.send(to,
		"Janus 邮箱验证",
		fmt.Sprintf("点击以下链接完成邮箱验证(24 小时内有效):\n\n%s\n\n如果这不是你的操作,请忽略本邮件。", verifyURL(m.baseURL, token)))
}

func (m *SMTPMailer) SendResetEmail(to, token string) error {
	return m.send(to,
		"Janus 密码重置",
		fmt.Sprintf("点击以下链接设置新密码(1 小时内有效):\n\n%s\n\n如果这不是你的操作,请忽略本邮件。", resetURL(m.baseURL, token)))
}

// send 建立连接并发送一封纯文本邮件。
func (m *SMTPMailer) send(to, subject, body string) error {
	from := m.cfg.From
	if from == "" {
		from = m.cfg.Username
	}
	if from == "" {
		return errors.New("smtp: SMTP_FROM 与 SMTP_USERNAME 均为空,无法确定发件人地址")
	}

	opts := []mail.Option{
		mail.WithPort(m.cfg.Port),
		mail.WithTimeout(30 * time.Second),
	}

	if m.cfg.Port == 465 {
		opts = append(opts, mail.WithSSL())
	} else {
		// 非 465 端口强制使用 STARTTLS，拒绝明文 AUTH
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}

	if m.cfg.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(m.cfg.Username),
			mail.WithPassword(m.cfg.Password),
		)
	}

	client, err := mail.NewClient(m.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("smtp client init: %w", err)
	}

	msg := mail.NewMsg()
	if err := msg.From(from); err != nil {
		return fmt.Errorf("smtp msg from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("smtp msg to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("smtp dial and send: %w", err)
	}
	return nil
}
