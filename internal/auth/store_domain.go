package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"agentdeck/internal/ulid"
)

// Store is the M0 auth domain. It owns the security rules (validation, role
// escalation, lockout, session lifetime) and delegates every write to a
// Repository, so there is exactly one place the rules can be enforced and
// exactly one place the data is written.
type Store struct {
	repo Repository

	mu       sync.Mutex
	failures map[string]loginFailure
}

type loginFailure struct {
	count    int
	lockedAt time.Time
}

// NewStore wraps any Repository. The caller picks the implementation; the
// domain rules above do not change between them.
func NewStore(repo Repository) *Store {
	return &Store{
		repo:     repo,
		failures: make(map[string]loginFailure),
	}
}

// IsShadow reports whether a hash belongs to a pending-invitation row.
func IsShadow(hash string) bool { return hash == shadowPasswordHash }

// Register implements US-AD01. A personal workspace is created automatically
// and the registering user becomes its owner; the caller never has to touch
// an org form (AC5). An absent name falls back to the email local part (AC6).
// Validation failures return ErrInvalidInput so the API answers 400, while a
// taken email returns ErrEmailExists for 409 (AC2/AC4).
func (s *Store) Register(ctx context.Context, email, password, name, orgName string) (User, Workspace, string, error) {
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
	locked := s.lockedUntil(normalized)
	s.mu.Unlock()
	if !locked.IsZero() {
		return User{}, Workspace{}, "", ErrAccountLocked
	}

	existing, err := s.repo.GetUserByEmail(ctx, normalized)
	switch {
	case err == nil && !IsShadow(existing.PasswordHash):
		return User{}, Workspace{}, "", ErrEmailExists
	case err == nil:
		// A pending invitation: claim the shadow row instead of rejecting the
		// email as taken. The invitee keeps their memberships and gains their
		// own personal workspace (US-AD04 AC1).
		if err := s.repo.ClaimShadowUser(ctx, normalized, name, hashPassword(password)); err != nil {
			return User{}, Workspace{}, "", err
		}
		existing.Name = name
	case errors.Is(err, ErrUserNotFound):
		user, cerr := s.repo.CreateUser(ctx, normalized, name, hashPassword(password), false)
		if cerr != nil {
			return User{}, Workspace{}, "", cerr
		}
		existing = user
	default:
		return User{}, Workspace{}, "", err
	}

	workspaceName := name + "'s workspace"
	if orgName != "" {
		workspaceName = orgName
	}
	// orgs.id must be a ULID TEXT(26) (DECISIONS 202), so the personal
	// workspace cannot be keyed by a predictable value. Idempotency comes
	// from looking up the user's existing registration-kind workspace
	// instead, so a retried signup reuses it rather than orphaning orgs.
	if workspace, err := s.repo.PersonalWorkspace(ctx, existing.ID); err == nil {
		if err := s.repo.CreateMembership(ctx, workspace.ID, existing.ID, Owner); err != nil {
			return User{}, Workspace{}, "", err
		}
		return existing, workspace, "", nil
	} else if !errors.Is(err, ErrWorkspaceNotFound) {
		return User{}, Workspace{}, "", err
	}

	slug := workspaceSlugFrom(email)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	orgID, err := ulid.New()
	if err != nil {
		return User{}, Workspace{}, "", err
	}
	workspace, err := s.repo.CreateOrg(ctx, orgID, slug, workspaceName, personalOrgKind)
	if err != nil {
		return User{}, Workspace{}, "", err
	}
	if err := s.repo.CreateMembership(ctx, workspace.ID, existing.ID, Owner); err != nil {
		return User{}, Workspace{}, "", err
	}

	sessionToken, err := newToken()
	if err != nil {
		return User{}, Workspace{}, "", err
	}
	if err := s.saveSession(ctx, existing.ID, sessionToken); err != nil {
		return User{}, Workspace{}, "", err
	}

	return existing, workspace, sessionToken, nil
}

func (s *Store) saveSession(ctx context.Context, userID, sessionToken string) error {
	now := time.Now()
	return s.repo.CreateSession(ctx, Session{
		ID:         ulid.Must(),
		UserID:     userID,
		TokenHash:  tokenHash(sessionToken),
		ExpiresAt:  now.Add(sessionDuration),
		LastSeenAt: now,
	})
}

// Login implements US-AD02. Five consecutive failures lock the account for
// 15 minutes (AC2); a locked account reports ErrAccountLocked so the API can
// surface the remaining lock time instead of a generic credential error.
func (s *Store) Login(ctx context.Context, email, password string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))

	s.mu.Lock()
	if locked := s.lockedUntil(normalized); !locked.IsZero() {
		s.mu.Unlock()
		return "", ErrAccountLocked
	}
	s.mu.Unlock()

	user, err := s.repo.GetUserByEmail(ctx, normalized)
	switch {
	case errors.Is(err, ErrUserNotFound):
		// An unknown email has no secret to brute-force, and counting it
		// would let anyone lock out a victim's future registration with
		// five throwaway attempts. Report invalid credentials, but leave
		// the lockout counter untouched.
		return "", ErrInvalidCredentials
	case err != nil:
		return "", err
	case IsShadow(user.PasswordHash):
		// A pending invitation has no credentials yet; it is claimed via
		// Register. Counting these failures would let an attacker freeze a
		// pending invite, so the counter stays untouched here too.
		return "", ErrInvalidCredentials
	case !verifyPassword(user.PasswordHash, password):
		s.recordFailure(normalized)
		return "", ErrInvalidCredentials
	}

	s.mu.Lock()
	delete(s.failures, normalized)
	s.mu.Unlock()

	sessionToken, err := newToken()
	if err != nil {
		return "", err
	}
	if err := s.saveSession(ctx, user.ID, sessionToken); err != nil {
		return "", err
	}
	return sessionToken, nil
}

func (s *Store) recordFailure(normalized string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	failure := s.failures[normalized]
	if !failure.lockedAt.IsZero() && time.Since(failure.lockedAt) >= lockDuration {
		failure = loginFailure{}
	}
	failure.count++
	if failure.count >= lockThreshold {
		failure.lockedAt = time.Now()
	}
	s.failures[normalized] = failure
}

// lockedUntil reports when a locked account unlocks; zero means not locked.
// It lets the login UI show the remaining lock time (US-AD02 AC6). Caller
// must hold s.mu.
func (s *Store) lockedUntil(normalized string) time.Time {
	failure, exists := s.failures[normalized]
	if !exists || failure.lockedAt.IsZero() {
		return time.Time{}
	}
	unlocksAt := failure.lockedAt.Add(lockDuration)
	if time.Now().After(unlocksAt) {
		return time.Time{}
	}
	return unlocksAt
}

// LockedUntil is the exported wrapper for the login UI.
func (s *Store) LockedUntil(email string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockedUntil(strings.ToLower(strings.TrimSpace(email)))
}

func (s *Store) Logout(ctx context.Context, sessionToken string) error {
	return s.repo.DeleteSessionByTokenHash(ctx, tokenHash(sessionToken))
}

// Authenticate validates an opaque session token and applies the US-AD02 AC4
// sliding window: last_seen_at refreshes on use, and a session idle longer
// than idleTimeout is revoked even before its absolute expiry. A token that
// fails every check returns ok=false so callers answer 401, never user data.
func (s *Store) Authenticate(ctx context.Context, sessionToken string) (User, bool) {
	session, err := s.repo.GetSessionByTokenHash(ctx, tokenHash(sessionToken))
	if err != nil {
		return User{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.repo.DeleteSessionByTokenHash(ctx, session.TokenHash)
		return User{}, false
	}
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return User{}, false
	}
	if IsShadow(user.PasswordHash) {
		return User{}, false
	}
	return user, true
}

// Workspaces lists every org the user belongs to with their role, backing the
// workspace switcher and GET /api/v1/orgs (ARCHITECTURE 6.2.4).
func (s *Store) Workspaces(ctx context.Context, email string) ([]Membership, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		// An unknown user sees no workspaces rather than an error, so the
		// workspace switcher and GET /api/v1/orgs stay safe to call with a
		// stale or forged email (US-AD05 AC1).
		return nil, nil
	}
	rows, err := s.repo.ListOrgsForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	result := make([]Membership, 0, len(rows))
	for _, row := range rows {
		result = append(result, Membership{
			WorkspaceID: row.OrgID,
			Name:        row.Name,
			Slug:        row.Slug,
			Role:        row.Role,
		})
	}
	return result, nil
}

// firstOrg resolves an actor with no explicit X-Org-ID to their default org:
// the first one they joined, which is the personal workspace created at
// registration.
func (s *Store) firstOrg(ctx context.Context, actorEmail string) (Workspace, Role, error) {
	actor, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(actorEmail)))
	if err != nil {
		return Workspace{}, "", ErrForbidden
	}
	orgs, err := s.repo.ListOrgsForUser(ctx, actor.ID)
	if err != nil {
		return Workspace{}, "", ErrForbidden
	}
	if len(orgs) == 0 {
		return Workspace{}, "", ErrForbidden
	}
	first := orgs[0]
	return Workspace{
		ID:   first.OrgID,
		Name: first.Name,
		Slug: first.Slug,
		Kind: first.Kind,
	}, first.Role, nil
}

// Members lists the membership rows of one org (ARCHITECTURE 6.2.4
// GET /api/v1/orgs/{id}/members). Viewer and above can call it; an actor
// outside the org gets ErrForbidden so no other tenant's roster is returned.
func (s *Store) Members(ctx context.Context, workspaceID, actorEmail string) ([]WorkspaceMember, error) {
	if _, _, err := s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Viewer); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceMember, 0, len(rows))
	for _, row := range rows {
		result = append(result, WorkspaceMember{
			UserID:    row.UserID,
			Email:     row.Email,
			Name:      row.Name,
			Role:      row.Role,
			CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

// CreateWorkspace adds a second org the caller owns. The owner of a brand-new
// org is the caller, so this path never accepts a role argument (ARCHITECTURE
// 6.2.4 POST /api/v1/orgs). Slug uniqueness is global: two orgs cannot share a
// slug, and a solo user never needs to choose one (US-AD03 AC4).
func (s *Store) CreateWorkspace(ctx context.Context, actorEmail, name, slug string) (Workspace, error) {
	actor, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(actorEmail)))
	if err != nil {
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

	workspace, err := s.repo.CreateOrg(ctx, ulid.Must(), workspaceSlug, strings.TrimSpace(name), manualOrgKind)
	if err != nil {
		if errors.Is(err, ErrSlugTaken) {
			return Workspace{}, ErrSlugTaken
		}
		return Workspace{}, err
	}
	if err := s.repo.CreateMembership(ctx, workspace.ID, actor.ID, Owner); err != nil {
		return Workspace{}, err
	}
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
// first org. A valid user who is not a member of the requested org gets
// ErrForbidden (403); an unknown org id gets ErrWorkspaceNotFound (404). The
// distinction keeps the tenant boundary observable by the caller, which is how
// US-AD07 AC1 (403 on X-Org-ID) and AC3 (404 on path id) are both satisfied.
func (s *Store) ResolveWorkspace(ctx context.Context, actorEmail, requestedID string) (Workspace, Role, error) {
	return s.resolveAndAuthorize(ctx, requestedID, actorEmail, Viewer)
}

// resolveAndAuthorize is the single tenant-resolution path. An empty
// requestedID falls back to the actor's first org (the personal workspace
// created at registration).
//
// The failure code is part of the contract (US-AD07): a *known* user who is
// not a member of the requested org gets ErrForbidden (403), while an org id
// that does not exist at all gets ErrWorkspaceNotFound (404). The split keeps
// the tenant boundary observable — a foreign org exists, the caller just has
// no business in it — without leaking whether an arbitrary id is real.
func (s *Store) resolveAndAuthorize(ctx context.Context, requestedID, actorEmail string, minimum Role) (Workspace, Role, error) {
	actor, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(actorEmail)))
	if err != nil {
		return Workspace{}, "", ErrForbidden
	}

	if requestedID == "" {
		return s.firstOrg(ctx, actorEmail)
	}

	// The org must exist first: an unknown id is a 404 regardless of who asks.
	workspace, err := s.repo.GetOrgByID(ctx, requestedID)
	if err != nil {
		return Workspace{}, "", ErrWorkspaceNotFound
	}

	membership, err := s.repo.GetMembership(ctx, requestedID, actor.ID)
	if err != nil {
		// The org exists but this user is not part of it: 403, never 404,
		// because answering 404 would confirm the tenant's existence to a
		// caller who is not allowed to know it.
		return Workspace{}, "", ErrForbidden
	}
	if rank(membership.Role) < rank(minimum) {
		return Workspace{}, "", ErrForbidden
	}
	return workspace, membership.Role, nil
}

// UpdateWorkspace implements PATCH /api/v1/orgs/{id}: rename an org (US-AD03
// AC2). Only the owner may rename; admin and member get ErrForbidden. An
// unknown org is ErrWorkspaceNotFound so the failure is observable.
func (s *Store) UpdateWorkspace(ctx context.Context, workspaceID, actorEmail, name string) error {
	if _, _, err := s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Owner); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInput
	}
	return s.repo.UpdateOrgName(ctx, workspaceID, strings.TrimSpace(name))
}

// AddMember invites a user into one org by email (US-AD04 AC1). The actor
// must hold at least admin in that same org, so a user of org A can never
// mutate the membership of org B (US-AD07). The invitee is resolved by email
// — not by a client-supplied user id — so the caller cannot pick an arbitrary
// account to elevate. A user who has never logged in gets a shadow row, which
// the real registration later claims.
func (s *Store) AddMember(ctx context.Context, workspaceID, actorEmail, inviteeEmail string, role Role) error {
	if _, _, err := s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Admin); err != nil {
		return err
	}
	if rank(role) < rank(Viewer) || rank(role) > rank(Admin) {
		return ErrInvalidRole
	}

	normalized, err := normalizeEmail(inviteeEmail)
	if err != nil {
		return ErrInvalidInput
	}
	// The membership row keys on user id, not email: email is the stable
	// invite handle a human types, but memberships.user_id must point at a
	// real users.id. A shadow row gets the pending membership attached to its
	// id so registration claims both the row and the role.
	user, err := s.repo.GetUserByEmail(ctx, normalized)
	switch {
	case errors.Is(err, ErrUserNotFound):
		user, err = s.repo.CreateUser(ctx, normalized,
			strings.SplitN(normalized, "@", 2)[0], shadowPasswordHash, true)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	}
	return s.repo.CreateMembership(ctx, workspaceID, user.ID, role)
}

// ChangeRole mutates a role within one org only. Owner is never assignable or
// removable here — it is created at registration time — so no actor can
// escalate itself or anyone else past admin.
func (s *Store) ChangeRole(ctx context.Context, workspaceID, actorEmail, userEmail string, role Role) error {
	if _, _, err := s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Admin); err != nil {
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(userEmail)))
	if err != nil {
		return ErrMemberNotFound
	}
	membership, err := s.repo.GetMembership(ctx, workspaceID, user.ID)
	if err != nil {
		return ErrMemberNotFound
	}
	if membership.Role == Owner {
		return ErrLastOwner
	}
	if role == Owner || rank(role) < rank(Viewer) || rank(role) > rank(Admin) {
		return ErrInvalidRole
	}
	return s.repo.UpdateMembershipRole(ctx, workspaceID, user.ID, role)
}

// ChangeMemberRole is the by-user-id entry point used by
// PATCH /api/v1/orgs/{id}/members/{user_id}. It resolves the id to the stable
// email key, then delegates to ChangeRole, so the path parameter can never
// reach the membership directly.
func (s *Store) ChangeMemberRole(ctx context.Context, workspaceID, actorEmail, userID string, role Role) error {
	email, err := s.emailForUserID(ctx, userID)
	if err != nil {
		return ErrMemberNotFound
	}
	return s.ChangeRole(ctx, workspaceID, actorEmail, email, role)
}

// RemoveMemberByID is the by-user-id entry point used by
// DELETE /api/v1/orgs/{id}/members/{user_id}.
func (s *Store) RemoveMemberByID(ctx context.Context, workspaceID, actorEmail, userID string) error {
	email, err := s.emailForUserID(ctx, userID)
	if err != nil {
		return ErrMemberNotFound
	}
	return s.RemoveMember(ctx, workspaceID, actorEmail, email)
}

// emailForUserID maps a public user id back to the membership key. An unknown
// id reports ErrUserNotFound, so the caller sees ErrMemberNotFound (404)
// instead of a leak.
func (s *Store) emailForUserID(ctx context.Context, userID string) (string, error) {
	user, err := s.repo.GetUserByID(ctx, strings.ToLower(strings.TrimSpace(userID)))
	if err != nil {
		return "", err
	}
	return user.Email, nil
}

// RemoveMember drops a membership in one org. The last remaining owner is
// protected: an org must never be left ownerless (US-AD04 AC3).
func (s *Store) RemoveMember(ctx context.Context, workspaceID, actorEmail, userEmail string) error {
	if _, _, err := s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Admin); err != nil {
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(userEmail)))
	if err != nil {
		return ErrMemberNotFound
	}
	membership, err := s.repo.GetMembership(ctx, workspaceID, user.ID)
	if err != nil {
		return ErrMemberNotFound
	}
	if membership.Role == Owner {
		owners, cerr := s.repo.CountOrgOwners(ctx, workspaceID)
		if cerr != nil {
			return cerr
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	return s.repo.DeleteMembership(ctx, workspaceID, user.ID)
}

// Authorize is the permission gate every org-scoped handler must call. It
// resolves the role from the membership of that specific org, so a user's role
// in org A grants nothing in org B.
func (s *Store) Authorize(ctx context.Context, workspaceID, email string, minimum Role) bool {
	_, _, err := s.resolveAndAuthorize(ctx, workspaceID, email, minimum)
	return err == nil
}

// GetOrg is the read entry point for GET /api/v1/orgs/{id}: viewer and above.
func (s *Store) GetOrg(ctx context.Context, workspaceID, actorEmail string) (Workspace, Role, error) {
	return s.resolveAndAuthorize(ctx, workspaceID, actorEmail, Viewer)
}
