// Package auth holds the M0 auth domain: registration, opaque server-side
// sessions, and the four-role membership matrix. Domain rules (hashing,
// validation, lockout, escalation) live in Store; persistence is delegated to
// a Repository so the in-memory fake and the Postgres implementation are
// interchangeable without touching the HTTP layer.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	SessionCookieName = "agentdeck_session"

	// sessionDuration is the absolute session lifetime (US-AD02 AC4).
	sessionDuration = 7 * 24 * time.Hour
	// idleTimeout is the sliding window: a session unused for longer than
	// this is rejected even before its absolute expiry (US-AD02 AC4).
	idleTimeout = 24 * time.Hour

	lockThreshold = 5
	lockDuration  = 15 * time.Minute

	// shadowPasswordHash stands in for a user created by a pending invitation
	// who has not set a password yet. It satisfies the schema's argon2id CHECK
	// while never verifying against any input, so a shadow row cannot log in.
	// Registration replaces it with a real hash (ClaimShadowUser).
	shadowPasswordHash = "$argon2id$v=19$m=65536,t=1,p=4$$"

	personalOrgKind = "registration"
	manualOrgKind   = "manual"
)

type Role string

const (
	Owner  Role = "owner"
	Admin  Role = "admin"
	Member Role = "member"
	Viewer Role = "viewer"
)

type User struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	IsShadow     bool
	CreatedAt    time.Time
}

type Workspace struct {
	ID        string
	Name      string
	Slug      string
	Kind      string
	CreatedAt time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

// Membership is one row of a user's org roster (the workspace switcher and
// GET /api/v1/orgs).
type Membership struct {
	WorkspaceID string
	Name        string
	Slug        string
	Role        Role
}

// WorkspaceMember is one row of an org's roster. UserID is the public key used
// by /api/v1/orgs/{id}/members/{user_id}; email is the stable invite key.
type WorkspaceMember struct {
	UserID    string
	Email     string
	Name      string
	Role      Role
	CreatedAt time.Time
}

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func hashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return "$argon2id$v=19$m=65536,t=1,p=4$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(hash)
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations, threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[5])
	if err != nil {
		return false
	}
	candidate := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(threads), uint32(len(expected)))
	return subtle.ConstantTimeCompare(candidate, expected) == 1
}

// newToken returns a 64-byte opaque random session token, hex encoded. Only
// its SHA-256 hash is ever stored (ARCHITECTURE 3.19).
func newToken() (string, error) {
	bytes := make([]byte, 64)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func rank(role Role) int {
	switch role {
	case Viewer:
		return 1
	case Member:
		return 2
	case Admin:
		return 3
	case Owner:
		return 4
	default:
		return 0
	}
}

func SessionCookie(sessionToken string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionToken,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   int(sessionDuration / time.Second),
	}
}

func ExpiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   -1,
	}
}
