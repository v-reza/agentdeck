package notify

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// InviteMailer delivers the "you have been added to <org>" notice required by
// US-AD04 AC1.
//
// It is a separate method rather than a parameter on SendPasswordReset because
// the two messages carry different facts: a reset link is a secret the recipient
// acts on, while an invite merely names a role the inviter already chose. The
// invite stays a notification — nothing in it is required for the invited member
// to sign in, because AddMember already wrote the membership row. That is also
// why no token is minted here.
func (m LogMailer) SendInvite(_ context.Context, to, orgName, role, signInLink string) error {
	if m.Logger == nil {
		return fmt.Errorf("notify: LogMailer needs a logger")
	}
	m.Logger.Info("member invite (no SMTP configured)",
		"to", to, "org", orgName, "role", role, "link", signInLink)
	return nil
}

func (m SMTPMailer) SendInvite(_ context.Context, to, orgName, role, signInLink string) error {
	if m.Config.Host == "" || m.Config.From == "" {
		return fmt.Errorf("notify: SMTP_HOST and SMTP_FROM are required")
	}
	port := m.Config.Port
	if port == "" {
		port = "587"
	}

	var auth smtp.Auth
	if m.Config.Username != "" {
		auth = smtp.PlainAuth("", m.Config.Username, m.Config.Password, m.Config.Host)
	}

	body := strings.Join([]string{
		"From: " + m.Config.From,
		"To: " + to,
		"Subject: Anda ditambahkan ke " + orgName + " di AgentDeck",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Anda ditambahkan ke workspace " + orgName + " di AgentDeck",
		"dengan role " + role + ".",
		"",
		"Masuk memakai alamat email ini untuk mulai bekerja:",
		signInLink,
	}, "\r\n")

	if err := smtp.SendMail(m.Config.Host+":"+port, auth, m.Config.From, []string{to}, []byte(body)); err != nil {
		return fmt.Errorf("notify: send invite: %w", err)
	}
	return nil
}
