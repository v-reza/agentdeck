package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
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
	CreatedAt    time.Time
}

type Workspace struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

type Store struct {
	mu             sync.RWMutex
	users          map[string]User
	workspaces     map[string]Workspace
	memberships    map[string]map[string]Role
	firstWorkspace map[string]string
	sessions       map[string]Session
	failures       map[string]loginFailure
}

type loginFailure struct {
	count    int
	lockedAt time.Time
}

func NewStore() *Store {
	return &Store{
		users:          make(map[string]User),
		workspaces:     make(map[string]Workspace),
		memberships:    make(map[string]map[string]Role),
		firstWorkspace: make(map[string]string),
		sessions:       make(map[string]Session),
		failures:       make(map[string]loginFailure),
	}
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

// Register implements US-AD01. A personal workspace is created automatically
// and the registering user becomes its owner; the caller never has to touch
// an org form (AC5). An absent name falls back to the email local part (AC6).
// Validation failures return ErrInvalidInput so the API answers 400, while a
// taken email returns ErrEmailExists for 409 (AC2/AC4).
func (s *Store) Register(email, password, name, orgName string) (User, Workspace, string, error) {
	normalized, err := validateRegistration(email, password)
	if err != nil {
		return User{}, Workspace{}, "", err
	}
	if name == "" {
		// AC6: name falls back to the local part of the email as typed, so
		// "Ada@Example.com" becomes "Ada", not the normalized "ada".
		name = strings.SplitN(strings.TrimSpace(email), "@", 2)[0]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// A pending invitation creates a shadow user row so the membership has
	// someone to point at (US-AD04 AC1). Real registration claims that row
	// instead of rejecting the email as taken: the invitee keeps their pending
	// memberships and gains their own personal workspace.
	shadow, invited := s.users[normalized]
	if invited {
		if shadow.PasswordHash != "" {
			return User{}, Workspace{}, "", ErrEmailExists
		}
		if name == "" {
			name = shadow.Name
		}
	}

	workspaceName := name + "'s workspace"
	if orgName != "" {
		workspaceName = orgName
	}
	workspace := Workspace{
		ID:   "ws-" + normalized,
		Name: workspaceName,
		Slug: "ws-" + normalized,
	}
	user := User{
		ID:           normalized,
		Email:        normalized,
		Name:         name,
		PasswordHash: hashPassword(password),
		CreatedAt:    time.Now(),
	}

	sessionToken, err := newToken()
	if err != nil {
		return User{}, Workspace{}, "", err
	}

	s.users[normalized] = user
	s.workspaces[workspace.ID] = workspace
	s.memberships[workspace.ID] = map[string]Role{normalized: Owner}
	s.firstWorkspace[normalized] = workspace.ID
	s.saveSession(normalized, sessionToken)

	return user, workspace, sessionToken, nil
}

func (s *Store) saveSession(userID, sessionToken string) {
	hash := tokenHash(sessionToken)
	now := time.Now()
	s.sessions[hash] = Session{
		ID:         hash,
		UserID:     userID,
		TokenHash:  hash,
		ExpiresAt:  now.Add(sessionDuration),
		LastSeenAt: now,
	}
}

// Login implements US-AD02. Five consecutive failures lock the account for
// 15 minutes (AC2); a locked account reports ErrAccountLocked so the API can
// surface the remaining lock time instead of a generic credential error.
func (s *Store) Login(email, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := strings.ToLower(strings.TrimSpace(email))
	failure := s.failures[normalized]
	if !failure.lockedAt.IsZero() && time.Since(failure.lockedAt) < lockDuration {
		return "", ErrAccountLocked
	}
	if !failure.lockedAt.IsZero() {
		failure = loginFailure{}
		s.failures[normalized] = failure
	}

	user, exists := s.users[normalized]
	if !exists || !verifyPassword(user.PasswordHash, password) {
		failure.count++
		if failure.count >= lockThreshold {
			failure.lockedAt = time.Now()
		}
		s.failures[normalized] = failure
		return "", ErrInvalidCredentials
	}
	s.failures[normalized] = loginFailure{}

	sessionToken, err := newToken()
	if err != nil {
		return "", err
	}
	s.saveSession(user.ID, sessionToken)
	return sessionToken, nil
}

// LockedUntil reports when a locked account unlocks; zero means not locked.
// It lets the login UI show the remaining lock time (US-AD02 AC6).
func (s *Store) LockedUntil(email string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	failure, exists := s.failures[strings.ToLower(strings.TrimSpace(email))]
	if !exists || failure.lockedAt.IsZero() {
		return time.Time{}
	}
	unlocksAt := failure.lockedAt.Add(lockDuration)
	if time.Now().After(unlocksAt) {
		return time.Time{}
	}
	return unlocksAt
}

func (s *Store) Logout(sessionToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, tokenHash(sessionToken))
}

// Authenticate validates an opaque session token and applies the US-AD02 AC4
// sliding window: last_seen_at refreshes on use, and a session idle longer
// than idleTimeout is revoked even before its absolute expiry. A token that
// fails every check returns ok=false so callers answer 401, never user data.
func (s *Store) Authenticate(sessionToken string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := tokenHash(sessionToken)
	session, exists := s.sessions[hash]
	now := time.Now()
	if !exists || now.After(session.ExpiresAt) {
		return User{}, false
	}
	if now.Sub(session.LastSeenAt) > idleTimeout {
		delete(s.sessions, hash)
		return User{}, false
	}

	session.LastSeenAt = now
	s.sessions[hash] = session

	user, exists := s.users[session.UserID]
	if !exists {
		return User{}, false
	}
	return user, true
}

// Workspaces lists every org the user belongs to with their role, backing the
// workspace switcher and GET /api/v1/orgs (ARCHITECTURE 6.2.4).
type Membership struct {
	WorkspaceID string
	Name        string
	Slug        string
	Role        Role
}

// WorkspaceMember is one row of a workspace roster. Email is the stable join
// key; UserID is exposed to the API path /api/v1/orgs/{id}/members/{user_id}.
type WorkspaceMember struct {
	UserID    string
	Email     string
	Name      string
	Role      Role
	CreatedAt time.Time
}

func (s *Store) Workspaces(email string) []Membership {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(email))

	var result []Membership
	for workspaceID, members := range s.memberships {
		if role, ok := members[normalized]; ok {
			workspace := s.workspaces[workspaceID]
			result = append(result, Membership{
				WorkspaceID: workspaceID,
				Name:        workspace.Name,
				Slug:        workspace.Slug,
				Role:        role,
			})
		}
	}
	return result
}

// Members lists the membership rows of one workspace (ARCHITECTURE 6.2.4
// GET /api/v1/orgs/{id}/members). Viewer and above can call it; an actor
// outside the workspace gets a zero list with ErrWorkspaceNotFound so no other
// tenant's roster is ever returned.
func (s *Store) Members(workspaceID, actorEmail string) ([]WorkspaceMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(actorEmail))
	members, exists := s.memberships[workspaceID]
	if !exists {
		return nil, ErrWorkspaceNotFound
	}
	if _, ok := members[normalized]; !ok {
		return nil, ErrForbidden
	}

	result := make([]WorkspaceMember, 0, len(members))
	for email, role := range members {
		result = append(result, WorkspaceMember{
			UserID:    s.users[email].ID,
			Email:     email,
			Name:      s.users[email].Name,
			Role:      role,
			CreatedAt: s.membershipCreatedAt(workspaceID, email),
		})
	}
	return result, nil
}

// membershipCreatedAt reports when the user joined this workspace. The store
// records one join time per user, so the membership inherits it.
func (s *Store) membershipCreatedAt(workspaceID, email string) time.Time {
	if s.users[email].ID == "" {
		return time.Time{}
	}
	if created := s.users[email].CreatedAt; !created.IsZero() {
		return created
	}
	return s.workspaces[workspaceID].CreatedAt
}

// CreateWorkspace adds a second org the caller owns. The owner of a brand-new
// org is the caller, so this path never accepts a role argument (ARCHITECTURE
// 6.2.4 POST /api/v1/orgs). Slug uniqueness is global: two orgs cannot share a
// slug, and a solo user never needs to choose one (US-AD03 AC4).
func (s *Store) CreateWorkspace(actorEmail, name, slug string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeEmail(actorEmail)
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	if _, exists := s.users[normalized]; !exists {
		return Workspace{}, ErrForbidden
	}
	if strings.TrimSpace(name) == "" {
		return Workspace{}, ErrInvalidInput
	}

	workspaceSlug := strings.ToLower(strings.TrimSpace(slug))
	if workspaceSlug == "" {
		workspaceSlug = workspaceSlugFrom(name)
	}
	if len(workspaceSlug) < 3 || len(workspaceSlug) > 40 {
		return Workspace{}, ErrInvalidInput
	}
	for _, existing := range s.workspaces {
		if existing.Slug == workspaceSlug {
			return Workspace{}, ErrSlugTaken
		}
	}

	workspace := Workspace{
		ID:        "org-" + workspaceSlug,
		Name:      strings.TrimSpace(name),
		Slug:      workspaceSlug,
		CreatedAt: time.Now(),
	}
	s.workspaces[workspace.ID] = workspace
	s.memberships[workspace.ID] = map[string]Role{normalized: Owner}

	return workspace, nil
}

// workspaceSlugFrom turns a name into a lowercase dashed slug.
func workspaceSlugFrom(name string) string {
	lowered := strings.ToLower(strings.TrimSpace(name))
	lowered = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, lowered)
	return strings.Trim(lowered, "-")
}

// ResolveWorkspace implements ARCHITECTURE 11.1 step 5, the two-priority org
// resolution. An explicit X-Org-ID wins; absence falls back to the actor's
// first workspace. A valid user who is not a member of the requested org gets
// ErrForbidden (403); an unknown org id gets ErrWorkspaceNotFound (404). The
// distinction keeps the tenant boundary observable by the caller, which is how
// US-AD07 AC1 (403 on X-Org-ID) and AC3 (404 on path id) are both satisfied.
func (s *Store) ResolveWorkspace(actorEmail, requestedID string) (Workspace, Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(actorEmail))

	if requestedID != "" {
		workspace, exists := s.workspaces[requestedID]
		if !exists {
			return Workspace{}, "", ErrWorkspaceNotFound
		}
		role, ok := s.memberships[requestedID][normalized]
		if !ok {
			return Workspace{}, "", ErrForbidden
		}
		return workspace, role, nil
	}

	firstID, ok := s.firstWorkspace[normalized]
	if !ok {
		return Workspace{}, "", ErrForbidden
	}
	return s.workspaces[firstID], s.memberships[firstID][normalized], nil
}

// UpdateWorkspace implements PATCH /api/v1/orgs/{id}: rename an org (US-AD03
// AC2). Only the owner may rename; admin and member get ErrForbidden. An
// unknown org is ErrWorkspaceNotFound so the failure is not distinguishable
// from a real one by status code alone.
func (s *Store) UpdateWorkspace(workspaceID, actorEmail, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.memberships[workspaceID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	if members[strings.ToLower(strings.TrimSpace(actorEmail))] != Owner {
		return ErrForbidden
	}
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInput
	}

	workspace := s.workspaces[workspaceID]
	workspace.Name = strings.TrimSpace(name)
	s.workspaces[workspaceID] = workspace

	return nil
}

// AddMember invites a user into one workspace by email (US-AD04 AC1). The actor
// must hold at least admin in that same workspace, so a user of org A can never
// mutate the membership of org B (US-AD07). The invitee is resolved by email —
// not by a client-supplied user id — so the caller cannot pick an arbitrary
// account to elevate. A user who has never logged in gets a shadow row, which
// the real registration later claims; a user who is already a member keeps the
// existing role unless the actor explicitly re-invites.
func (s *Store) AddMember(workspaceID, actorEmail, inviteeEmail string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.memberships[workspaceID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	if rank(members[strings.ToLower(actorEmail)]) < rank(Admin) {
		return ErrForbidden
	}
	if rank(role) < rank(Viewer) || rank(role) > rank(Admin) {
		return ErrInvalidRole
	}

	normalized, err := normalizeEmail(inviteeEmail)
	if err != nil {
		return ErrInvalidInput
	}
	if _, exists := s.users[normalized]; !exists {
		// Shadow row: no password, no session, no org of its own. It exists
		// only so the membership has a user to point at until the invitee
		// registers and claims it (Register matches on normalized email).
		s.users[normalized] = User{
			ID:        normalized,
			Email:     normalized,
			Name:      strings.SplitN(normalized, "@", 2)[0],
			CreatedAt: time.Now(),
		}
	}

	members[normalized] = role
	return nil
}

// ChangeRole mutates a role within one workspace only. Owner is never
// assignable or removable here — it is created at registration time — so no
// actor can escalate itself or anyone else past admin.
func (s *Store) ChangeRole(workspaceID, actorEmail, userEmail string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.memberships[workspaceID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	if rank(members[strings.ToLower(actorEmail)]) < rank(Admin) {
		return ErrForbidden
	}

	target := strings.ToLower(userEmail)
	oldRole, exists := members[target]
	if !exists {
		return ErrMemberNotFound
	}
	if oldRole == Owner {
		return ErrLastOwner
	}
	if role == Owner || rank(role) < rank(Viewer) || rank(role) > rank(Admin) {
		return ErrInvalidRole
	}

	members[target] = role
	return nil
}

// ChangeMemberRole is the by-user-id entry point used by
// PATCH /api/v1/orgs/{id}/members/{user_id}. It resolves the id to the stable
// email key, then delegates to ChangeRole, so the path parameter can never
// reach the membership map directly.
func (s *Store) ChangeMemberRole(workspaceID, actorEmail, userID string, role Role) error {
	return s.ChangeRole(workspaceID, actorEmail, s.emailForUserID(userID), role)
}

// RemoveMemberByID is the by-user-id entry point used by
// DELETE /api/v1/orgs/{id}/members/{user_id}.
func (s *Store) RemoveMemberByID(workspaceID, actorEmail, userID string) error {
	return s.RemoveMember(workspaceID, actorEmail, s.emailForUserID(userID))
}

// emailForUserID maps a public user id back to the membership key. An unknown
// id maps to an email that cannot be a member, so the caller sees
// ErrMemberNotFound (404) instead of a leak.
func (s *Store) emailForUserID(userID string) string {
	normalized := strings.ToLower(strings.TrimSpace(userID))
	s.mu.RLock()
	defer s.mu.RUnlock()

	if user, ok := s.users[normalized]; ok {
		return user.Email
	}
	return "unknown-" + normalized
}

// RemoveMember drops a membership in one workspace. The last remaining owner
// is protected: an org must never be left ownerless (US-AD04 AC3).
func (s *Store) RemoveMember(workspaceID, actorEmail, userEmail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members, exists := s.memberships[workspaceID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	if rank(members[strings.ToLower(actorEmail)]) < rank(Admin) {
		return ErrForbidden
	}

	target := strings.ToLower(userEmail)
	oldRole, exists := members[target]
	if !exists {
		return ErrMemberNotFound
	}
	if oldRole == Owner {
		owners := 0
		for _, role := range members {
			if role == Owner {
				owners++
			}
		}
		if owners == 1 {
			return ErrLastOwner
		}
	}

	delete(members, target)
	return nil
}

// Authorize is the permission gate every workspace-scoped handler must call.
// It resolves the role from the membership of that specific workspace, so a
// user's role in org A grants nothing in org B.
func (s *Store) Authorize(workspaceID, email string, minimum Role) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	role, exists := s.memberships[workspaceID][strings.ToLower(email)]
	return exists && rank(role) >= rank(minimum)
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
