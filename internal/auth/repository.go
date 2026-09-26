package auth

import (
	"context"
	"time"
)

// Repository is the persistence boundary. Every method takes a context so a
// slow or dead database can be bounded by the request, and every write is
// durable: an implementation that loses committed data on process restart
// fails this contract and the tenant-isolation gate.
type Repository interface {
	// CreateUser persists a new identity. Email uniqueness is case-folded;
	// isShadow marks a row created by a pending invitation that registration
	// later claims. It returns the stored user with its server-generated ID.
	CreateUser(ctx context.Context, email, name, passwordHash string, isShadow bool) (User, error)

	// ClaimShadowUser turns a pending-invitation row into a real account by
	// setting its password hash. A row that already has credentials is not a
	// shadow, so the claim must report ErrEmailExists instead of overwriting.
	ClaimShadowUser(ctx context.Context, email, name, passwordHash string) error

	// GetUserByEmail loads a live (non-deleted) identity by case-insensitive
	// email. A missing email reports ErrUserNotFound.
	GetUserByEmail(ctx context.Context, email string) (User, error)

	// GetUserByID loads a live (non-deleted) identity by id. Used to resolve
	// a session to a user and /members/{user_id} to an email. A missing id
	// reports ErrUserNotFound.
	GetUserByID(ctx context.Context, id string) (User, error)

	// UpdateUserProfile writes the caller's own profile fields (US-AD89 AC2).
	// It is one statement, so a taken email (ErrEmailExists, AC3) rolls the
	// whole write back rather than leaving a half-applied name change. An
	// unknown id is ErrUserNotFound — 404, never 403 (AC4).
	UpdateUserProfile(ctx context.Context, id, name, email, avatarURL string) (User, error)

	// ---- sessions (US-AD90, US-AD05) -------------------------------------

	// ListSessionsForUser returns the caller's live sessions, most recently used
	// first (US-AD90 AC2). Revoked and expired rows are excluded: the screen
	// exists to spot a device you do not recognise, and dead rows on it are
	// noise exactly where the signal matters.
	ListSessionsForUser(ctx context.Context, userID string) ([]SessionInfo, error)

	// GetSessionByID loads one live session owned by userID. Scoping by user is
	// what makes a foreign session id a 404 rather than a read.
	GetSessionByID(ctx context.Context, id, userID string) (SessionInfo, error)

	// GetSessionAnyUser loads one live session without an owner filter. Only the
	// owner/admin revocation path (US-AD05 AC2) may use it, and it exists so the
	// membership check can happen before any decision about the session.
	GetSessionAnyUser(ctx context.Context, id string) (SessionInfo, error)

	// RevokeSessionByID revokes one session. It reports whether a row changed, so
	// a repeat request is "already revoked" rather than a failure.
	RevokeSessionByID(ctx context.Context, id, userID string) (bool, error)

	// RevokeOtherSessions revokes every session of userID except keepID — the
	// caller's own (US-AD90 AC1). One statement, so there is no window in which
	// the caller has no session at all.
	RevokeOtherSessions(ctx context.Context, userID, keepID string) error

	// IsOrgMember reports whether userID belongs to orgID. It is the tenant bound
	// on revoking someone else's session: an admin of one workspace is not an
	// admin of the installation.
	IsOrgMember(ctx context.Context, orgID, userID string) (bool, error)

	// SoftDeleteOrg and SoftDeleteUser are US-AD98 AC5's soft closure. Hard
	// deletes would make the 30-day recovery window impossible to honour.
	SoftDeleteOrg(ctx context.Context, orgID string) error
	SoftDeleteUser(ctx context.Context, userID string) error

	// CountOrgMembers answers US-AD98 AC3's "workspace that still has other
	// people". CountOrgOwners already exists above.
	CountOrgMembers(ctx context.Context, orgID string) (int, error)

	// UpdatePassword replaces the stored hash (US-AD90 AC1). It is one statement
	// with `deleted_at IS NULL`, so a closed account cannot rotate its password.
	UpdatePassword(ctx context.Context, userID, passwordHash string) error

	// CreateOrg persists a tenant and records its kind. The personal workspace
	// of a registering user is kind 'personal'; every other org is 'manual'.
	CreateOrg(ctx context.Context, id, slug, name, kind string) (Workspace, error)

	// GetOrgByID loads one tenant by id; ErrWorkspaceNotFound if unknown.
	GetOrgByID(ctx context.Context, id string) (Workspace, error)

	// UpdateOrgName renames a tenant. Slug collisions are ErrSlugTaken.
	UpdateOrgName(ctx context.Context, id, name string) error

	// RenameOrgWithAudit commits the rename and its audit record atomically. A
	// failed audit write must leave the org name unchanged (US-AD77 AC2).
	RenameOrgWithAudit(ctx context.Context, id, actorUserID, ip, beforeName, afterName string) error

	// CreateMembership links a user to an org with one role. An invitation to an
	// existing member leaves the role alone: the inviter asked to add the user,
	// not to re-rank them, and re-adding must stay idempotent so a retried invite
	// cannot silently demote an owner.
	CreateMembership(ctx context.Context, orgID, userID string, role Role) error

	// GetMembership answers "is this user in this org, and as what". It is the
	// single query every org-scoped handler resolves its tenant through, so
	// there is no second code path that could forget the org_id scope.
	GetMembership(ctx context.Context, orgID, userID string) (MembershipRow, error)

	// PersonalWorkspace returns the registration-kind workspace created for
	// the user at signup, so a retried registration reuses it instead of
	// creating a second one.
	PersonalWorkspace(ctx context.Context, userID string) (Workspace, error)

	// ListOrgsForUser returns every org a user belongs to with their role. It
	// is scoped by membership, never by an org id the caller supplied.
	ListOrgsForUser(ctx context.Context, userID string) ([]OrgMembershipRow, error)

	// ListMembers returns the roster of one org.
	ListMembers(ctx context.Context, orgID string) ([]MemberRow, error)

	// UpdateMembershipRole changes a single role.
	UpdateMembershipRole(ctx context.Context, orgID, userID string, role Role) error

	// DeleteMembership removes a user from an org.
	DeleteMembership(ctx context.Context, orgID, userID string) error

	// CountOrgOwners is the guard for the last-owner rule.
	CountOrgOwners(ctx context.Context, orgID string) (int, error)

	// CreateSession stores the SHA-256 of a 64-byte opaque token. The raw
	// token only ever exists in the Set-Cookie, so a store dump leaks nothing.
	CreateSession(ctx context.Context, sess Session) error

	// GetSessionByTokenHash loads a live session by token hash and bumps
	// last_seen_at (sliding idle window, US-AD02 AC4). An expired or unknown
	// hash reports ErrSessionNotFound.
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)

	// DeleteSessionByTokenHash ends one session (logout).
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error

	// DeleteExpiredSessions is the reaper; call it on a timer.
	DeleteExpiredSessions(ctx context.Context) error

	// CreatePasswordReset stores one reset token hash for a user (US-AD88
	// AC1). Only the hash is persisted; the raw token is emailed.
	CreatePasswordReset(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error

	// ConsumePasswordReset marks a token used in the same statement that
	// checks it, so two concurrent redemptions cannot both succeed. A token
	// that is unknown, already used, or past its window reports
	// ErrResetTokenInvalid (AC3).
	ConsumePasswordReset(ctx context.Context, tokenHash string) error

	// GetPasswordResetByTokenHash loads the reset row a token points at, so
	// the redemption updates the right user. It is only called after
	// ConsumePasswordReset has claimed the token.
	GetPasswordResetByTokenHash(ctx context.Context, tokenHash string) (PasswordResetRow, error)

	// UpdateUserPassword replaces the password hash of one user (AC2).
	UpdateUserPassword(ctx context.Context, userID, passwordHash string) error

	// DeleteUserSessions revokes every live session of a user. A reset
	// revokes all of them: the operator has no trusted device (AC2).
	DeleteUserSessions(ctx context.Context, userID string) error
}

// MembershipRow is the persisted (org, user, role) triple.
type MembershipRow struct {
	OrgID     string
	UserID    string
	Role      Role
	CreatedAt time.Time
}

// OrgMembershipRow is one row of a user's org roster.
type OrgMembershipRow struct {
	OrgID string
	Slug  string
	Name  string
	Kind  string
	Role  Role
}

// MemberRow is one row of an org's roster.
type MemberRow struct {
	UserID    string
	Email     string
	Name      string
	Role      Role
	CreatedAt time.Time
}

// PasswordResetRow is one outstanding reset token. Only the hash is stored;
// UsedAt marks a spent token, which is how single-use is enforced (AC3).
type PasswordResetRow struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
