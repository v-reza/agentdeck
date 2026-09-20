package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// PasswordResetEmail is the message the caller delivers. The domain composes it
// so the security-relevant facts (single use, 30-minute window) are stated in
// one place instead of being retyped by whichever transport sends the mail.
type PasswordResetEmail struct {
	To    string
	Token string
}

// RequestPasswordReset implements US-AD88 AC1 and AC5.
//
// AC5 is the reason this never reports whether the email exists: an unknown
// address returns an empty token and no error, so the handler answers 202 for
// both cases and account existence cannot be probed. The caller sends mail only
// when a token comes back.
func (s *Store) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		// Still not an error: a blank email is answered exactly like an
		// unknown one so the two are indistinguishable.
		return "", nil
	}

	user, err := s.repo.GetUserByEmail(ctx, normalized)
	switch {
	case errors.Is(err, ErrUserNotFound):
		return "", nil
	case err != nil:
		return "", err
	case IsShadow(user.PasswordHash):
		// A pending invitation has no password to reset; it is claimed by
		// registering instead. Treated as unknown so the invite cannot be
		// probed through this endpoint either.
		return "", nil
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}
	if err := s.repo.CreatePasswordReset(ctx, tokenHash(token), user.ID, time.Now().Add(resetTokenTTL)); err != nil {
		return "", err
	}
	return token, nil
}

// ResetPassword implements US-AD88 AC2 and AC3.
//
// The order is deliberate: the new password is validated before the token is
// consumed, so a short password does not burn the link and leave the operator
// with neither the old nor a new one. Only once the password is acceptable is
// the token spent — and the spend is a single conditional UPDATE in the
// repository, so an expired, used, or unknown token all fail identically.
func (s *Store) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return ErrInvalidInput
	}
	hash := tokenHash(token)

	if err := s.repo.ConsumePasswordReset(ctx, hash); err != nil {
		return err
	}

	reset, err := s.repo.GetPasswordResetByTokenHash(ctx, hash)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateUserPassword(ctx, reset.UserID, hashPassword(newPassword)); err != nil {
		return err
	}
	// AC2: every session of the account is revoked, so a stolen cookie dies
	// with the old password.
	return s.repo.DeleteUserSessions(ctx, reset.UserID)
}
