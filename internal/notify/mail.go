// Package notify delivers outbound messages. It exists so the auth domain can
// state *what* must be sent (single-use, 30-minute window) without knowing how
// the bytes leave the process: a self-hosted install with no SMTP relay still
// has to work, and a test must not need a mail server.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// LogMailer is the default when no SMTP relay is configured. It writes the
// reset link to the process log so a self-hosted operator can complete the flow
// locally without an email provider, and so an install that never configured
// mail fails loudly in the log instead of silently dropping the message.
type LogMailer struct {
	Logger *slog.Logger
}

func (m LogMailer) SendPasswordReset(_ context.Context, to, link string) error {
	if m.Logger == nil {
		return fmt.Errorf("notify: LogMailer needs a logger")
	}
	m.Logger.Info("password reset link (no SMTP configured)", "to", to, "link", link)
	return nil
}

// SMTPConfig is a plain relay: host, port, and optional credentials. It is
// deliberately not a provider SDK — a self-hosted product should talk to
// whatever relay the operator already runs.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// SMTPMailer delivers over SMTP. It is the configured path once SMTP_HOST is
// set; otherwise LogMailer is used.
type SMTPMailer struct {
	Config SMTPConfig
}

func (m SMTPMailer) SendPasswordReset(_ context.Context, to, link string) error {
	if m.Config.Host == "" || m.Config.From == "" {
		return fmt.Errorf("notify: SMTP_HOST and SMTP_FROM are required")
	}
	port := m.Config.Port
	if port == "" {
		port = "587"
	}
	address := m.Config.Host + ":" + port

	var auth smtp.Auth
	if m.Config.Username != "" {
		auth = smtp.PlainAuth("", m.Config.Username, m.Config.Password, m.Config.Host)
	}

	// The window and the single-use rule are stated in the message because the
	// recipient is the one who has to act on them.
	body := strings.Join([]string{
		"From: " + m.Config.From,
		"To: " + to,
		"Subject: Reset password AgentDeck",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Ada permintaan reset password untuk akun ini.",
		"",
		"Buka tautan berikut untuk membuat password baru:",
		link,
		"",
		"Tautan berlaku 30 menit dan hanya bisa dipakai sekali.",
		"Kalau bukan Anda yang meminta, abaikan email ini; password tidak berubah.",
	}, "\r\n")

	if err := smtp.SendMail(address, auth, m.Config.From, []string{to}, []byte(body)); err != nil {
		return fmt.Errorf("notify: send password reset: %w", err)
	}
	return nil
}
