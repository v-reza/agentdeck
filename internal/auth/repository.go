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

	// CreateOrg persists a tenant and records its kind. The personal workspace
	// of a registering user is kind 'personal'; every other org is 'manual'.
	CreateOrg(ctx context.Context, id, slug, name, kind string) (Workspace, error)

	// GetOrgByID loads one tenant by id; ErrWorkspaceNotFound if unknown.
	GetOrgByID(ctx context.Context, id string) (Workspace, error)

	// UpdateOrgName renames a tenant. Slug collisions are ErrSlugTaken.
	UpdateOrgName(ctx context.Context, id, name string) error

	// CreateMembership links a user to an org with one role. Re-adding an
	// existing membership updates the role rather than erroring, so an invite
	// to an existing member is idempotent.
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
