package auth

import (
	"context"
	"sort"
	"strings"
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
	mu          sync.RWMutex
	users       map[string]User
	byEmail     map[string]string
	orgs        map[string]Workspace
	memberships map[string]map[string]Role
	// joinedAt is when a membership row was written, per (org, user). The
	// Postgres query selects `m.created_at`, so the in-memory double has to
	// carry its own timestamp too — falling back to the user's account age
	// would make GET /orgs/{id}/members report a different `created_at` in a
	// test than in production.
	joinedAt     map[string]map[string]time.Time
	createdOrder []string
	sessions     map[string]Session
	resets       map[string]PasswordResetRow
	audits       []AuditEntry
	auditErr     error
}

// NewMemoryRepository builds the in-memory Repository used by the unit tests.
func NewMemoryRepository() Repository { return newMemoryRepository() }

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		users:       map[string]User{},
		byEmail:     map[string]string{},
		orgs:        map[string]Workspace{},
		memberships: map[string]map[string]Role{},
		joinedAt:    map[string]map[string]time.Time{},
		sessions:    map[string]Session{},
		resets:      map[string]PasswordResetRow{},
		audits:      []AuditEntry{},
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
		// Sama seperti `deleted_at IS NULL` di SQL: akun yang sudah ditutup
		// (US-AD98) tidak resolve, jadi login dan Authenticate menolaknya.
		if user := m.users[id]; user.DeletedAt == nil {
			return user, nil
		}
	}
	return User{}, ErrUserNotFound
}

func (m *memoryRepository) GetUserByID(ctx context.Context, id string) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if user, ok := m.users[id]; ok && user.DeletedAt == nil {
		return user, nil
	}
	return User{}, ErrUserNotFound
}

// UpdateUserProfile mirrors the Postgres statement: the email index is
// case-folded and unique, so a second account claiming the same address is
// ErrEmailExists (US-AD89 AC3), and the whole write is abandoned rather than
// applied field by field. An unknown id is ErrUserNotFound (404, AC4).
func (m *memoryRepository) UpdateUserProfile(ctx context.Context, id, name, email, avatarURL string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	if owner, exists := m.byEmail[normalized]; exists && owner != id {
		return User{}, ErrEmailExists
	}

	delete(m.byEmail, user.Email)
	user.Name = name
	user.Email = normalized
	user.AvatarURL = avatarURL
	m.users[id] = user
	m.byEmail[normalized] = id
	return user, nil
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

	if workspace, ok := m.orgs[id]; ok && workspace.DeletedAt == nil {
		return workspace, nil
	}
	return Workspace{}, ErrWorkspaceNotFound
}

func (m *memoryRepository) GetOrgByIDIncludingDeleted(ctx context.Context, id string) (Workspace, error) {
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

func (m *memoryRepository) RenameOrgWithAudit(ctx context.Context, id, actorUserID, ip, beforeName, afterName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.auditErr != nil {
		return m.auditErr
	}
	workspace, ok := m.orgs[id]
	if !ok {
		return ErrWorkspaceNotFound
	}
	workspace.Name = afterName
	m.orgs[id] = workspace
	m.audits = append(m.audits, AuditEntry{
		OrgID: id, ActorUserID: actorUserID, Action: "org.rename", TargetType: "org", TargetID: id,
		Before: `{"name":"` + beforeName + `"}`, After: `{"name":"` + afterName + `"}`, IP: ip, CreatedAt: time.Now().UTC(),
	})
	return nil
}

func (m *memoryRepository) lastAudit() (AuditEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.audits) == 0 {
		return AuditEntry{}, false
	}
	return m.audits[len(m.audits)-1], true
}

func (m *memoryRepository) CreateMembership(ctx context.Context, orgID, userID string, role Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.memberships[orgID]; !exists {
		m.memberships[orgID] = map[string]Role{}
		m.joinedAt[orgID] = map[string]time.Time{}
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
	m.joinedAt[orgID][userID] = time.Now().UTC()
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
		if !ok || workspace.DeletedAt != nil {
			// Ruang kerja yang ditutup bersama akun pemiliknya (US-AD98 AC2)
			// berhenti muncul di switcher, sama seperti penyaring di SQL.
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
			CreatedAt: m.joinedAt[orgID][userID],
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

// CountOrgMembers mirrors the SQL's join on live users: an account that has
// been closed is not still in the workspace (US-AD98 AC2).
func (m *memoryRepository) CountOrgMembers(ctx context.Context, orgID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return 0, ErrWorkspaceNotFound
	}
	count := 0
	for userID := range members {
		if user, ok := m.users[userID]; ok && user.DeletedAt == nil {
			count++
		}
	}
	return count, nil
}

func (m *memoryRepository) SoftDeleteOrg(ctx context.Context, orgID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	org, ok := m.orgs[orgID]
	if !ok {
		return ErrWorkspaceNotFound
	}
	now := time.Now()
	org.DeletedAt = &now
	m.orgs[orgID] = org
	return nil
}

func (m *memoryRepository) SoftDeleteUser(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, ok := m.users[userID]
	if !ok {
		return ErrUserNotFound
	}
	now := time.Now()
	user.DeletedAt = &now
	m.users[userID] = user
	return nil
}

// findSessionLocked resolves a session by its row id. The map is keyed by
// token_hash — that is the only lookup the login path needs — so id lookups
// scan. Callers must hold at least a read lock.
func (m *memoryRepository) findSessionLocked(id string) (Session, string, bool) {
	for key, sess := range m.sessions {
		if sess.ID == id {
			return sess, key, true
		}
	}
	return Session{}, "", false
}

func (m *memoryRepository) ListSessionsForUser(ctx context.Context, userID string) ([]SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	out := []SessionInfo{}
	for _, sess := range m.sessions {
		if sess.UserID != userID || sess.DeletedAt != nil || !sess.ExpiresAt.After(now) {
			continue
		}
		out = append(out, SessionInfo{
			ID: sess.ID, UserID: sess.UserID, UserAgent: sess.UserAgent, IP: sess.IP,
			LastSeenAt: sess.LastSeenAt, CreatedAt: sess.CreatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt.After(out[j].LastSeenAt) })
	return out, nil
}

func (m *memoryRepository) GetSessionByID(ctx context.Context, id, userID string) (SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, _, ok := m.findSessionLocked(id)
	if !ok || sess.UserID != userID || sess.DeletedAt != nil {
		return SessionInfo{}, ErrSessionNotFound
	}
	return SessionInfo{
		ID: sess.ID, UserID: sess.UserID, UserAgent: sess.UserAgent, IP: sess.IP,
		LastSeenAt: sess.LastSeenAt, CreatedAt: sess.CreatedAt,
	}, nil
}

func (m *memoryRepository) GetSessionAnyUser(ctx context.Context, id string) (SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, _, ok := m.findSessionLocked(id)
	if !ok || sess.DeletedAt != nil || !sess.ExpiresAt.After(time.Now()) {
		return SessionInfo{}, ErrSessionNotFound
	}
	return SessionInfo{
		ID: sess.ID, UserID: sess.UserID, UserAgent: sess.UserAgent, IP: sess.IP,
		LastSeenAt: sess.LastSeenAt, CreatedAt: sess.CreatedAt,
	}, nil
}

func (m *memoryRepository) RevokeSessionByID(ctx context.Context, id, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, key, ok := m.findSessionLocked(id)
	if !ok || sess.UserID != userID || sess.DeletedAt != nil {
		return false, nil
	}
	now := time.Now()
	sess.DeletedAt = &now
	m.sessions[key] = sess
	return true, nil
}

func (m *memoryRepository) RevokeOtherSessions(ctx context.Context, userID, keepID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, sess := range m.sessions {
		if sess.UserID != userID || sess.ID == keepID || sess.DeletedAt != nil {
			continue
		}
		sess.DeletedAt = &now
		m.sessions[key] = sess
	}
	return nil
}

func (m *memoryRepository) IsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members, exists := m.memberships[orgID]
	if !exists {
		return false, nil
	}
	_, ok := members[userID]
	return ok, nil
}

func (m *memoryRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok || user.DeletedAt != nil {
		return ErrUserNotFound
	}
	user.PasswordHash = passwordHash
	m.users[userID] = user
	return nil
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
	// Sesi yang dicabut (US-AD90 AC4, US-AD98 AC2) adalah sesi mati, sama
	// seperti `deleted_at IS NULL` di SQL. Tanpa cek ini, pencabutan hanya
	// menghapus baris dari DAFTAR sementara tokennya tetap bisa dipakai —
	// endpoint yang melaporkan sukses tapi tidak mengubah apa pun.
	if session.DeletedAt != nil {
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

// CreatePasswordReset stores only the token hash (US-AD88 AC1).
func (m *memoryRepository) CreatePasswordReset(ctx context.Context, hash, userID string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.resets[hash] = PasswordResetRow{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

// ConsumePasswordReset enforces expiry and single-use in one step, mirroring
// the conditional UPDATE the Postgres repository runs (US-AD88 AC3): every
// failing case collapses into ErrResetTokenInvalid.
func (m *memoryRepository) ConsumePasswordReset(ctx context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	reset, exists := m.resets[hash]
	if !exists || reset.UsedAt != nil || time.Now().After(reset.ExpiresAt) {
		return ErrResetTokenInvalid
	}
	usedAt := time.Now()
	reset.UsedAt = &usedAt
	m.resets[hash] = reset
	return nil
}

func (m *memoryRepository) GetPasswordResetByTokenHash(ctx context.Context, hash string) (PasswordResetRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reset, exists := m.resets[hash]
	if !exists {
		return PasswordResetRow{}, ErrResetTokenInvalid
	}
	return reset, nil
}

func (m *memoryRepository) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[userID]
	if !exists {
		return ErrUserNotFound
	}
	user.PasswordHash = passwordHash
	m.users[userID] = user
	return nil
}

func (m *memoryRepository) DeleteUserSessions(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for hash, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, hash)
		}
	}
	return nil
}

// setSessionExpiry and setSessionLastSeen live in testing.go (test-only).
