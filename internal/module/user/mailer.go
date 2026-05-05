package user

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// Mailer 定义发送邮件的接口，方便测试时替换为 stub。
type Mailer interface {
	SendPasswordReset(toEmail, resetURL string) error
}

// SMTPMailer 通过标准库 net/smtp 发送邮件。
type SMTPMailer struct {
	host string // SMTP 服务器主机，如 "smtp.gmail.com"
	port string // SMTP 端口，如 "587"
	user string // SMTP 认证用户名
	pass string // SMTP 认证密码
	from string // 发件人地址
}

// NewSMTPMailer 创建 SMTPMailer。
func NewSMTPMailer(host, port, user, pass, from string) *SMTPMailer {
	return &SMTPMailer{host: host, port: port, user: user, pass: pass, from: from}
}

// SendPasswordReset 发送密码重置邮件。
func (m *SMTPMailer) SendPasswordReset(toEmail, resetURL string) error {
	slog.Debug("SMTPMailer.SendPasswordReset called", "to", toEmail, "reset_url", resetURL)

	subject := "【日本語学習】パスワードのリセット"
	body := fmt.Sprintf(
		"パスワードのリセットをご希望の場合は、以下のリンクをクリックしてください。\n\n"+
			"%s\n\n"+
			"このリンクは30分間有効です。\n"+
			"このメールに心当たりがない場合は無視してください。",
		resetURL,
	)

	msg := buildMIMEMessage(m.from, toEmail, subject, body)

	addr := m.host + ":" + m.port
	var auth smtp.Auth
	if m.user != "" && m.pass != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}

	if err := smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg)); err != nil {
		slog.Error("SMTPMailer.SendPasswordReset: smtp.SendMail failed", "err", err, "to", toEmail)
		return fmt.Errorf("user.SMTPMailer.SendPasswordReset: %w", err)
	}

	slog.Info("password reset email sent", "to", toEmail)
	return nil
}

// buildMIMEMessage formats a plain-text email message.
func buildMIMEMessage(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}

// StubMailer 测试用存根，不实际发送邮件，直接打印到日志。
type StubMailer struct{}

// SendPasswordReset logs the reset URL instead of sending an email.
func (s *StubMailer) SendPasswordReset(toEmail, resetURL string) error {
	slog.Info("StubMailer: (no email sent) password reset URL",
		"to", toEmail, "reset_url", resetURL)
	return nil
}
