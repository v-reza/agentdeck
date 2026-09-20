package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// US-AD88 — reset password lewat email.
//
// The security rules live in the domain, not in the handler: a token is
// single-use, expires after 30 minutes, and a successful reset revokes every
// existing session. These tests pin those rules before the implementation
// exists, so a handler that forgets one cannot pass by looking plausible.

func newResetStore() *Store { return NewStore(NewMemoryRepository()) }

// registerForReset creates a real account and returns its id.
func registerForReset(t *testing.T, s *Store, email, password string) User {
	t.Helper()
	user, _, _, err := s.Register(context.Background(), email, password, "Reset Tester", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return user
}

// AC1 — a request for a registered email issues a token, and only its hash is
// persisted. The raw token exists solely in the email.
func TestRequestPasswordResetStoresOnlyTheHash(t *testing.T) {
	s := newResetStore()
	user := registerForReset(t, s, "ada@example.com", "password1")

	token, err := s.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token == "" {
		t.Fatal("AC1: expected a token for a registered email, got empty")
	}

	repo := s.repo.(*memoryRepository)
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if _, plaintext := repo.resets[tokenHash(token)]; !plaintext {
		t.Fatal("AC1: the token hash is not stored")
	}
	for hash := range repo.resets {
		if hash == token {
			t.Fatal("AC1: the raw token was stored; only its hash may be persisted")
		}
	}
	// The reset must be tied to the requesting user, not to the email string.
	for _, reset := range repo.resets {
		if reset.UserID != user.ID {
			t.Fatalf("AC1: reset bound to %q, want %q", reset.UserID, user.ID)
		}
	}
}

// AC5 — an unknown email must look identical to a known one: no error, and no
// token to leak whether the account exists.
func TestRequestPasswordResetHidesUnknownEmail(t *testing.T) {
	s := newResetStore()

	token, err := s.RequestPasswordReset(context.Background(), "nobody@example.com")
	if err != nil {
		t.Fatalf("AC5: unknown email must not error, got %v", err)
	}
	if token != "" {
		t.Fatalf("AC5: unknown email must not issue a token, got %q", token)
	}
}

// AC2 — a valid token with a new password updates the hash and revokes every
// session for that user.
func TestResetPasswordUpdatesHashAndRevokesSessions(t *testing.T) {
	s := newResetStore()
	user := registerForReset(t, s, "ada@example.com", "password1")

	// Two live sessions: the reset must revoke both.
	first, err := s.Login(context.Background(), "ada@example.com", "password1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	second, err := s.Login(context.Background(), "ada@example.com", "password1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	token, err := s.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := s.ResetPassword(context.Background(), token, "password2"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// The old password no longer works and the new one does.
	if _, err := s.Login(context.Background(), "ada@example.com", "password1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("AC2: old password still accepted, got %v", err)
	}
	if _, err := s.Login(context.Background(), "ada@example.com", "password2"); err != nil {
		t.Fatalf("AC2: new password rejected: %v", err)
	}

	// Every session issued before the reset is dead.
	for name, token := range map[string]string{"first": first, "second": second} {
		if _, ok := s.Authenticate(context.Background(), token); ok {
			t.Fatalf("AC2: %s session survived the reset", name)
		}
	}

	// The stored hash belongs to the new password, and to that user only.
	repo := s.repo.(*memoryRepository)
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	stored := repo.users[user.ID]
	if !verifyPassword(stored.PasswordHash, "password2") {
		t.Fatal("AC2: users.password_hash was not updated")
	}
}

// AC2 — the minimum length is enforced before the token is spent, so a typo in
// the new password does not burn the link.
func TestResetPasswordRejectsShortPasswordWithoutSpendingToken(t *testing.T) {
	s := newResetStore()
	registerForReset(t, s, "ada@example.com", "password1")

	token, err := s.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := s.ResetPassword(context.Background(), token, "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("AC2: short password got %v, want ErrInvalidInput", err)
	}

	// The same token must still work with a valid password.
	if err := s.ResetPassword(context.Background(), token, "password2"); err != nil {
		t.Fatalf("AC2: token was consumed by the rejected attempt: %v", err)
	}
}

// AC3 — an expired token is refused and changes nothing.
func TestResetPasswordRejectsExpiredToken(t *testing.T) {
	s := newResetStore()
	registerForReset(t, s, "ada@example.com", "password1")

	token, err := s.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	repo := s.repo.(*memoryRepository)
	repo.mu.Lock()
	reset := repo.resets[tokenHash(token)]
	reset.ExpiresAt = time.Now().Add(-time.Minute)
	repo.resets[tokenHash(token)] = reset
	repo.mu.Unlock()

	if err := s.ResetPassword(context.Background(), token, "password2"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("AC3: expired token got %v, want ErrResetTokenInvalid", err)
	}
	if _, err := s.Login(context.Background(), "ada@example.com", "password1"); err != nil {
		t.Fatalf("AC3: password changed despite the expired token: %v", err)
	}
}

// AC3 — a token is single-use: the second redemption fails and the password set
// by the first one stays.
func TestResetPasswordRejectsReusedToken(t *testing.T) {
	s := newResetStore()
	registerForReset(t, s, "ada@example.com", "password1")

	token, err := s.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := s.ResetPassword(context.Background(), token, "password2"); err != nil {
		t.Fatalf("first reset: %v", err)
	}
	if err := s.ResetPassword(context.Background(), token, "password3"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("AC3: reused token got %v, want ErrResetTokenInvalid", err)
	}
	if _, err := s.Login(context.Background(), "ada@example.com", "password2"); err != nil {
		t.Fatalf("AC3: the first reset's password was overwritten: %v", err)
	}
}

// An unknown token must be indistinguishable from an expired one.
func TestResetPasswordRejectsUnknownToken(t *testing.T) {
	s := newResetStore()
	registerForReset(t, s, "ada@example.com", "password1")

	if err := s.ResetPassword(context.Background(), "not-a-real-token", "password2"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("unknown token got %v, want ErrResetTokenInvalid", err)
	}
}
