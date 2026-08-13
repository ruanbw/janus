// SMTPMailer 通过真实 SMTP 服务器发送邮件。
// 支持两种安全模式:465 隐式 TLS(SMTPS)与 587 STARTTLS(显式升级)。
// 仅使用标准库(net/smtp + crypto/tls),不引入第三方依赖。
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPMailer struct {
	cfg     SMTPConfig
	baseURL string
}

func (m *SMTPMailer) SendVerifyEmail(to, token string) error {
	return m.send(to,
		"CLOAK 邮箱验证",
		fmt.Sprintf("点击以下链接完成邮箱验证(24 小时内有效):\n\n%s\n\n如果这不是你的操作,请忽略本邮件。", verifyURL(m.baseURL, token)))
}

func (m *SMTPMailer) SendResetEmail(to, token string) error {
	return m.send(to,
		"CLOAK 密码重置",
		fmt.Sprintf("点击以下链接设置新密码(1 小时内有效):\n\n%s\n\n如果这不是你的操作,请忽略本邮件。", resetURL(m.baseURL, token)))
}

// send 建立连接并发送一封纯文本邮件。
func (m *SMTPMailer) send(to, subject, body string) error {
	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))
	from := m.cfg.From
	if from == "" {
		from = m.cfg.Username
	}
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	// 465:隐式 TLS;587 及其他端口:STARTTLS 显式升级
	if m.cfg.Port == 465 {
		return m.sendSMTPS(addr, auth, from, to, subject, body)
	}
	return m.sendSTARTTLS(addr, auth, from, to, subject, body)
}

func (m *SMTPMailer) sendSMTPS(addr string, auth smtp.Auth, from, to, subject, body string) error {
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: m.cfg.Host})
	client, err := smtp.NewClient(tlsConn, m.cfg.Host)
	if err != nil {
		_ = tlsConn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()
	return m.mail(client, auth, from, to, subject, body)
}

func (m *SMTPMailer) sendSTARTTLS(addr string, auth smtp.Auth, from, to, subject, body string) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer func() { _ = client.Close() }()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	return m.mail(client, auth, from, to, subject, body)
}

func (m *SMTPMailer) mail(client *smtp.Client, auth smtp.Auth, from, to, subject, body string) error {
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", to),
		"Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?=",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}
