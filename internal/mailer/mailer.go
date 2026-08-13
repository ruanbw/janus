// Package mailer 定义可插拔邮件发送接口。
// SMTP 凭据由部署者后续提供(spec Out of Scope);未配置时使用控制台假 mailer。
package mailer

import (
	"fmt"
	"io"
)

type Mailer interface {
	// SendVerifyEmail 发送邮箱验证邮件。
	SendVerifyEmail(to, token string) error
	// SendResetEmail 发送密码重置邮件。
	SendResetEmail(to, token string) error
}

// ConsoleMailer 把邮件输出到指定 writer(开发环境用)。
type ConsoleMailer struct{ w io.Writer }

func NewConsoleMailer(w io.Writer) *ConsoleMailer { return &ConsoleMailer{w: w} }

func (m *ConsoleMailer) SendVerifyEmail(to, token string) error {
	_, err := fmt.Fprintf(m.w,
		"\n[CLOAK mailer] 邮箱验证 %s\n  token: %s\n  验证地址: /verify-email?token=%s\n\n",
		to, token, token)
	return err
}

func (m *ConsoleMailer) SendResetEmail(to, token string) error {
	_, err := fmt.Fprintf(m.w,
		"\n[CLOAK mailer] 密码重置 %s\n  token: %s\n  重置地址: /reset-password?token=%s\n\n",
		to, token, token)
	return err
}
