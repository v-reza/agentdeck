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
	// AvatarURL is the uploaded avatar (users.avatar_url, ARCHITECTURE 3.3).
	// Empty means the shell draws the derived monogram (US-AD89 AC1).
	AvatarURL string
	IsShadow  bool
	CreatedAt time.Time
	// DeletedAt is US-AD98's soft closure: nil means live. The column has always
	// existed and every user query filters on it; the domain value did not carry
	// it, so nothing in Go could tell a closed account from an open one.
	DeletedAt *time.Time
}

type Workspace struct {
	ID        string
	Name      string
	Slug      string
	Kind      string
	CreatedAt time.Time
	// DeletedAt is the org side of US-AD98 AC2/AC5: a workspace closed with its
	// owner's account keeps its rows for 30 days but stops resolving.
	DeletedAt *time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
	// UserAgent and IP are what the session list shows (US-AD90 AC2: "perangkat,
	// IP"). Empty means the client sent nothing — the column is NULL, and the
	// difference between "unknown" and "sent an empty header" is kept.
	UserAgent string
	IP        string
	CreatedAt time.Time
	// DeletedAt is a revoked session (the row is kept, so a revoked token is
	// distinguishable from a token that never existed).
	DeletedAt *time.Time
}

// SessionInfo is one row of the session list (US-AD90 AC2): what a user needs
// to recognise a device, and nothing about the token. TokenHash is deliberately
// absent — a screen that lists sessions has no use for a credential, and a
// struct that carries one is a struct that can leak it.
type SessionInfo struct {
	ID         string
	UserID     string
	UserAgent  string
	IP         string
	LastSeenAt time.Time
	CreatedAt  time.Time
}

// SessionMeta is the client context recorded when a session is created
// (US-AD90 AC2 shows it back as "perangkat, IP"). A struct rather than two
// adjacent string parameters, because Register and Login both take it and two
// same-typed neighbours are the kind of pair a call site swaps without the
// compiler noticing.
type SessionMeta struct {
	UserAgent string
	IP        string
}

// Membership is one row of a user's org roster (the workspace switcher and
// GET /api/v1/orgs).
type Membership struct {
	WorkspaceID string
	Name        string
	Slug        string
	Kind        string
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

type AuditEntry struct {
	OrgID       string
	ActorUserID string
	Action      string
	TargetType  string
	TargetID    string
	Before      string
	After       string
	IP          string
	CreatedAt   time.Time
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
	if parts[4] == "" || parts[5] == "" {
		// A shadow row's sentinel has an empty salt and hash: hex.DecodeString
		// turns that into a zero-length slice, and argon2.IDKey panics on one
		// instead of returning false, so a pending invitation's verify would
		// take the process down instead of denying login. Reject it here: a
		// shadow row holds no credentials and must never log in (F2).
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
