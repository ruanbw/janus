// SMTPMailer 通过真实 SMTP 服务器发送邮件。
// 支持两种安全模式:465 隐式 TLS(SMTPS)与 587 STARTTLS(显式升级)。
// 安全约束:
//   - 非 465 端口必须 STARTTLS,服务器不支持则报错(拒绝明文 AUTH,防止凭据泄露);
//   - SMTP_USERNAME 为空时不调用 AUTH;
//   - SMTP_FROM 为空时回退 Username,两者都空直接报错;
//   - 整个 SMTP 会话设置 deadline,避免连接/命令无限阻塞。
//
// 仅使用标准库(net/smtp + crypto/tls),不引入第三方依赖。
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const (
	smtpDialTimeout = 15 * time.Second // TCP 拨号超时
	smtpCmdTimeout  = 30 * time.Second // SMTP 会话级 deadline(覆盖 TLS 握手、命令与 DATA)
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
	from := m.cfg.From
	if from == "" {
		from = m.cfg.Username
	}
	if from == "" {
		return errors.New("smtp: SMTP_FROM 与 SMTP_USERNAME 均为空,无法确定发件人地址")
	}
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))
	client, err := m.dial(addr)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return m.mail(client, auth, from, to, subject, body)
}

// dial 建立连接并完成安全升级:465 隐式 TLS;其他端口显式 STARTTLS(必须)。
func (m *SMTPMailer) dial(addr string) (*smtp.Client, error) {
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	// 会话级 deadline:同一底层连接上的 TLS 握手、SMTP 命令与 DATA 全部受控
	_ = conn.SetDeadline(time.Now().Add(smtpCmdTimeout))

	if m.cfg.Port == 465 {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: m.cfg.Host})
		client, err := smtp.NewClient(tlsConn, m.cfg.Host)
		if err != nil {
			_ = tlsConn.Close()
			return nil, fmt.Errorf("smtp client: %w", err)
		}
		return client, nil
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	ok, _ := client.Extension("STARTTLS")
	if !ok {
		_ = client.Close()
		return nil, errors.New("smtp: 服务器不支持 STARTTLS;非 465 端口拒绝明文 AUTH")
	}
	if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("smtp starttls: %w", err)
	}
	return client, nil
}

func (m *SMTPMailer) mail(client *smtp.Client, auth smtp.Auth, from, to, subject, body string) error {
	// 仅配置了用户名时才 AUTH;未配置则不发送 AUTH(避免明文凭据与无谓认证)。
	// StartTLS 之后 smtp.Client.Extension 会基于 TLS 会话内的 EHLO 能力重新查询。
	if m.cfg.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("smtp auth: 服务器不支持 AUTH,但已配置 SMTP_USERNAME")
		}
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
