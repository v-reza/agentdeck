package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account temporarily locked")
	ErrForbidden          = errors.New("forbidden")
	ErrWorkspaceNotFound  = errors.New("workspace not found")
	ErrMemberNotFound     = errors.New("member not found")
	ErrInvalidRole        = errors.New("invalid role")
	ErrLastOwner          = errors.New("last owner cannot be demoted")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrSlugTaken          = errors.New("slug already taken")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionNotFound    = errors.New("session not found")
)

const minPasswordLength = 8

// normalizeEmail lowercases and trims an address and rejects anything that is
// not shaped like one. Membership keys must be stable and unique per human, so
// "Ada@Example.com" and "ada@example.com" are always the same member.
func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	parts := strings.SplitN(normalized, "@", 2)
	if len(parts) != 2 || parts[0] == "" || !strings.Contains(parts[1], ".") {
		return "", ErrInvalidInput
	}
	return normalized, nil
}

// validateRegistration trims, normalizes, and validates email/password. It
// never touches the store: callers distinguish 400 (invalid input) from 409
// (email taken) instead of reading a single generic error.
func validateRegistration(email, password string) (string, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return "", ErrInvalidInput
	}
	if len(password) < minPasswordLength {
		return "", ErrInvalidInput
	}
	return normalized, nil
}

// tokenHash stores only SHA-256 of the opaque 64-byte session token; the raw
// token only ever exists in the cookie (ARCHITECTURE 3.19).
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ParseRole accepts only the four frozen values (DECISIONS section 4). Anything
// else — including "superuser", "root", or a stray owner — is ErrInvalidRole,
// never silently coerced.
func ParseRole(value string) (Role, error) {
	switch Role(value) {
	case Owner:
		return Owner, nil
	case Admin:
		return Admin, nil
	case Member:
		return Member, nil
	case Viewer:
		return Viewer, nil
	}
	return "", ErrInvalidRole
}
