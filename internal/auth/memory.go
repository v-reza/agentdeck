package auth

import (
	"context"
	"sync"
	"time"

	"agentdeck/internal/ulid"
)

// memoryRepository is an in-memory Repository used by the unit tests. It is
// deliberately not the production implementation: an in-memory store cannot
// survive a restart, and the M0 contract (ARCHITECTURE P4) requires every
// committed write to land in Postgres. It exists so the domain rules can be
// tested without a database; the Postgres-backed repository is what the API
// actually runs against.
//
// Users are keyed by their public ULID, with a secondary email index, so the
// in-memory store has the same lookup semantics as the Postgres one: an email
// lookup is case-folded, an id lookup is exact (Crockford base 32 is already
// canonical). Keying by email would make ids and emails interchangeable and
// mask id/email confusion bugs that the Postgres path exposes.
type memoryRepository struct {
	mu           sync.RWMutex
	users        map[string]User
	byEmail      map[string]string
	orgs         map[string]Workspace
	memberships  map[string]map[string]Role
	createdOrder []string
	sessions     map[string]Session
}

// NewMemoryRepository builds the in-memory Repository used by the unit tests.
func NewMemoryRepository() Repository { return newMemoryRepository() }

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		users:       map[string]User{},
		byEmail:     map[string]string{},
		orgs:        map[string]Workspace{},
		memberships: map[string]map[string]Role{},
		sessions:    map[string]Session{},
	}
}

func (m *memoryRepository) CreateUser(ctx context.Context, email, name, passwordHash string, isShadow bool) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byEmail[email]; exists && !isShadow {
		return User{}, ErrEmailExists
	}
	user := User{
		ID:           ulid.Must(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		IsShadow:     isShadow,
		CreatedAt:    time.Now(),
	}
	m.users[user.ID] = user
	m.byEmail[user.Email] = user.ID
	return user, nil
}

func (m *memoryRepository) ClaimShadowUser(ctx context.Context, email, name, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, exists := m.byEmail[email]
	if !exists {
		return ErrEmailExists
	}
	user := m.users[id]
	if !user.IsShadow {
		return ErrEmailExists
	}
	user.Name = name
	user.PasswordHash = passwordHash
	user.IsShadow = false
	m.users[id] = user
	return nil
}

func (m *memoryRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if id, ok := m.byEmail[email]; ok {
		return m.users[id], nil
	}
	return User{}, ErrUserNotFound
}

func (m *memoryRepository) GetUserByID(ctx context.Context, id string) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if user, ok := m.users[id]; ok {
		return user, nil
	}
	return User{}, ErrUserNotFound
}

func (m *memoryRepository) CreateOrg(ctx context.Context, id, slug, name, kind string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.orgs {
		if existing.Slug == slug {
			return Workspace{}, ErrSlugTaken
		}
	}
	workspace := Workspace{ID: id, Slug: slug, Name: name, Kind: kind, CreatedAt: time.Now()}
	m.orgs[id] = workspace
	m.createdOrder = append(m.createdOrder, id)
	return workspace, nil
}

func (m *memoryRepository) GetOrgByID(ctx context.Context, id string) (Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if workspace, ok := m.orgs[id]; ok {
		return workspace, nil
	}
	return Workspace{}, ErrWorkspaceNotFound
}

func (m *memoryRepository) UpdateOrgName(ctx context.Context, id, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspace, ok := m.orgs[id]
	if !ok {
		return ErrWorkspaceNotFound
	}
	workspace.Name = name
	m.orgs[id] = workspace
	return nil
}

func (m *memoryRepository) CreateMembership(ctx context.Context, orgID, userID string, role Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.memberships[orgID]; !exists {
		m.memberships[orgID] = map[string]Role{}
	}
	// A re-invite must not re-rank an existing member: the inviter asked to
	// add the user, not to change their role, and otherwise an owner's own
	// signup would silently demote them.
	if existing, exists := m.memberships[orgID][userID]; exists {
		if existing == role {
			return nil
		}
		return ErrMemberExists
	}
	m.memberships[orgID][userID] = role
	return nil
}

func (m *memoryRepository) GetMembership(ctx context.Context, orgID, userID string) (MembershipRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return MembershipRow{}, ErrWorkspaceNotFound
	}
	role, ok := members[userID]
	if !ok {
		return MembershipRow{}, ErrMemberNotFound
	}
	return MembershipRow{OrgID: orgID, UserID: userID, Role: role}, nil
}

// PersonalWorkspace mirrors the registration-kind lookup the Postgres
// repository does through org_kinds: the org whose kind is 'registration' and
// which the user owns. Ownership is the test — an invitee is a member of
// someone else's registration-kind org, and returning it here would hand them
// a workspace that is not theirs. A retried registration reuses their own
// instead of orphaning a second one.
func (m *memoryRepository) PersonalWorkspace(ctx context.Context, userID string) (Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, orgID := range m.createdOrder {
		if m.orgs[orgID].Kind != personalOrgKind {
			continue
		}
		if m.memberships[orgID][userID] != Owner {
			continue
		}
		return m.orgs[orgID], nil
	}
	return Workspace{}, ErrWorkspaceNotFound
}

// ListOrgsForUser returns orgs in created order, so the personal workspace
// created first wins resolution when no X-Org-ID is supplied.
func (m *memoryRepository) ListOrgsForUser(ctx context.Context, userID string) ([]OrgMembershipRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := []OrgMembershipRow{}
	for _, orgID := range m.createdOrder {
		role, ok := m.memberships[orgID][userID]
		if !ok {
			continue
		}
		workspace, ok := m.orgs[orgID]
		if !ok {
			continue
		}
		rows = append(rows, OrgMembershipRow{
			OrgID: orgID,
			Slug:  workspace.Slug,
			Name:  workspace.Name,
			Kind:  workspace.Kind,
			Role:  role,
		})
	}
	return rows, nil
}

func (m *memoryRepository) ListMembers(ctx context.Context, orgID string) ([]MemberRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return nil, ErrWorkspaceNotFound
	}
	rows := make([]MemberRow, 0, len(members))
	for userID, role := range members {
		user := m.users[userID]
		rows = append(rows, MemberRow{
			UserID:    user.ID,
			Email:     user.Email,
			Name:      user.Name,
			Role:      role,
			CreatedAt: user.CreatedAt,
		})
	}
	return rows, nil
}

func (m *memoryRepository) UpdateMembershipRole(ctx context.Context, orgID, userID string, role Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	if _, ok := members[userID]; !ok {
		return ErrMemberNotFound
	}
	members[userID] = role
	return nil
}

func (m *memoryRepository) DeleteMembership(ctx context.Context, orgID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return ErrWorkspaceNotFound
	}
	delete(members, userID)
	return nil
}

func (m *memoryRepository) CountOrgOwners(ctx context.Context, orgID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return 0, ErrWorkspaceNotFound
	}
	owners := 0
	for _, role := range members {
		if role == Owner {
			owners++
		}
	}
	return owners, nil
}

func (m *memoryRepository) CreateSession(ctx context.Context, sess Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[sess.TokenHash] = sess
	return nil
}

// GetSessionByTokenHash applies the sliding idle window (US-AD02 AC4): a
// session idle past idleTimeout is treated as expired even before its
// absolute expiry, and last_seen_at refreshes on every successful use.
func (m *memoryRepository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[tokenHash]
	if !exists {
		return Session{}, ErrSessionNotFound
	}
	now := time.Now()
	if now.After(session.ExpiresAt) || now.Sub(session.LastSeenAt) > idleTimeout {
		delete(m.sessions, tokenHash)
		return Session{}, ErrSessionNotFound
	}
	session.LastSeenAt = now
	m.sessions[tokenHash] = session
	return session, nil
}

func (m *memoryRepository) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, tokenHash)
	return nil
}

func (m *memoryRepository) DeleteExpiredSessions(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for hash, session := range m.sessions {
		if now.After(session.ExpiresAt) || now.Sub(session.LastSeenAt) > idleTimeout {
			delete(m.sessions, hash)
		}
	}
	return nil
}

// setSessionExpiry and setSessionLastSeen live in testing.go (test-only).
