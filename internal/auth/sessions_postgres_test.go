package auth

// Postgres-backed coverage for US-AD90 and US-AD98. These run against the real
// pgx path because the in-memory repository has already diverged from the SQL
// once in this phase (it kept accepting a revoked session, and it let a closed
// account log in) — the fake is a double, not a second implementation of the
// contract, and only the real statements prove the contract holds.
//
// Gated on AGENTDECK_TEST_DATABASE_URL, like the rest of the suite.

import (
	"context"
	"errors"
	"testing"
)

// pgSessionsStore builds a store plus one registered user with two devices.
func pgSessionsStore(t *testing.T, email string) (*Store, User, string, string) {
	t.Helper()
	store := pgTestStore(t)

	user, _, first, err := store.Register(context.Background(), email, "password1", "", "", SessionMeta{UserAgent: "device-one"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	second, err := store.Login(context.Background(), email, "password1", SessionMeta{UserAgent: "device-two"})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	return store, user, first, second
}

// TestPgSessionsListShowsRecordedClientContext — US-AD90 AC2.
//
// The columns existed but nothing wrote them, so AC2 could not be satisfied.
func TestPgSessionsListShowsRecordedClientContext(t *testing.T) {
	ctx := context.Background()
	store, user, _, _ := pgSessionsStore(t, uniqueEmail(t, "pg.sessions@example.com"))

	sessions, err := store.Sessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	agents := map[string]bool{}
	for _, s := range sessions {
		agents[s.UserAgent] = true
		if s.CreatedAt.IsZero() {
			t.Errorf("session %s has no created_at; the list needs it (AC2)", s.ID)
		}
		if s.LastSeenAt.IsZero() {
			t.Errorf("session %s has no last_seen_at; the list needs it (AC2)", s.ID)
		}
	}
	if !agents["device-one"] || !agents["device-two"] {
		t.Fatalf("recorded user agents are wrong: %+v", sessions)
	}
}

// TestPgRevokedSessionIsRefusedImmediately — US-AD05 AC1, US-AD90 AC4.
//
// The row-level guarantee: after the revoke, the token is dead on the very next
// call. This is the check the in-memory fake failed.
func TestPgRevokedSessionIsRefusedImmediately(t *testing.T) {
	ctx := context.Background()
	store, user, first, second := pgSessionsStore(t, uniqueEmail(t, "pg.revoke@example.com"))

	firstID, err := store.SessionIDForToken(ctx, first)
	if err != nil {
		t.Fatalf("session id for the first device: %v", err)
	}

	// Revoke the first session through the same call the route uses.
	if err := store.RevokeSession(ctx, user.ID, user.Email, "", firstID, true); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// AC1: the next request with that token is refused.
	if _, ok := store.Authenticate(ctx, first); ok {
		t.Fatal("revoked token still authenticates")
	}
	// The other device is untouched.
	if _, ok := store.Authenticate(ctx, second); !ok {
		t.Fatal("revoking one session killed the other")
	}
}

// TestPgChangePasswordKeepsOnlyTheCallersSession — US-AD90 AC1.
func TestPgChangePasswordKeepsOnlyTheCallersSession(t *testing.T) {
	ctx := context.Background()
	store, user, first, second := pgSessionsStore(t, uniqueEmail(t, "pg.changepw@example.com"))

	currentID, err := store.SessionIDForToken(ctx, second)
	if err != nil {
		t.Fatalf("session id: %v", err)
	}
	if err := store.ChangePassword(ctx, user.ID, currentID, "password1", "password2"); err != nil {
		t.Fatalf("change password: %v", err)
	}

	// AC1: the caller's own session survives...
	if _, ok := store.Authenticate(ctx, second); !ok {
		t.Fatal("the current session was revoked by its own password change")
	}
	// ...and the other one does not.
	if _, ok := store.Authenticate(ctx, first); ok {
		t.Fatal("the other session survived the password change")
	}
	// The new password works and the old one does not.
	if _, err := store.Login(ctx, user.Email, "password2", SessionMeta{}); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := store.Login(ctx, user.Email, "password1", SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
}

// TestPgChangePasswordRejectsTheWrongCurrentPassword — US-AD90 AC3.
func TestPgChangePasswordRejectsTheWrongCurrentPassword(t *testing.T) {
	ctx := context.Background()
	store, user, _, second := pgSessionsStore(t, uniqueEmail(t, "pg.wrongpw@example.com"))

	currentID, _ := store.SessionIDForToken(ctx, second)
	err := store.ChangePassword(ctx, user.ID, currentID, "not-it", "password2")
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("want ErrWrongPassword, got %v", err)
	}
	// AC3: nothing changed.
	if _, err := store.Login(ctx, user.Email, "password1", SessionMeta{}); err != nil {
		t.Fatalf("password changed despite the rejection: %v", err)
	}
}

// TestPgCloseAccountRemovesWorkspaceAndSessions — US-AD98 AC1/AC2/AC5.
func TestPgCloseAccountRemovesWorkspaceAndSessions(t *testing.T) {
	ctx := context.Background()
	store := pgTestStore(t)

	email := uniqueEmail(t, "pg.close@example.com")
	user, workspace, session, err := store.Register(ctx, email, "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// AC1: a mismatched confirmation changes nothing.
	if err := store.CloseAccount(ctx, user.ID, "someone-else@example.com"); !errors.Is(err, ErrAccountClosureConfirm) {
		t.Fatalf("want ErrAccountClosureConfirm, got %v", err)
	}
	if _, ok := store.Authenticate(ctx, session); !ok {
		t.Fatal("account was closed despite the mismatched confirmation")
	}

	if err := store.CloseAccount(ctx, user.ID, email); err != nil {
		t.Fatalf("close: %v", err)
	}
	// AC2: every session is gone.
	if _, ok := store.Authenticate(ctx, session); ok {
		t.Fatal("a session survived the account closure")
	}
	// The account no longer resolves, so it cannot log in.
	if _, err := store.Login(ctx, email, "password1", SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("closed account can log in: %v", err)
	}
	// AC2: the workspace went with it. Checked at the row level, NOT through
	// ResolveWorkspace: that call fails for a closed account whether or not the
	// org was flagged, so it would pass even with the org left wide open.
	var orgDeletedAt *string
	if err := pgPool(t).QueryRow(ctx, "SELECT deleted_at::text FROM orgs WHERE id = $1", workspace.ID).Scan(&orgDeletedAt); err != nil {
		t.Fatalf("read back org: %v", err)
	}
	if orgDeletedAt == nil {
		t.Fatal("AC2: the user's sole-member workspace was not closed with the account")
	}
	// AC5: the row is still there, just flagged — the closure is reversible.
	var deletedAt *string
	row := pgPool(t).QueryRow(ctx, "SELECT deleted_at::text FROM users WHERE id = $1", user.ID)
	if err := row.Scan(&deletedAt); err != nil {
		t.Fatalf("read back user: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("AC5 requires a soft closure, but users.deleted_at is NULL")
	}
}

// TestPgLastOwnerCannotCloseASharedWorkspace — US-AD98 AC3.
func TestPgLastOwnerCannotCloseASharedWorkspace(t *testing.T) {
	ctx := context.Background()
	store := pgTestStore(t)

	email := uniqueEmail(t, "pg.owner@example.com")
	owner, workspace, _, err := store.Register(ctx, email, "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if err := store.AddMember(ctx, workspace.ID, owner.Email, uniqueEmail(t, "pg.member@example.com"), Member); err != nil {
		t.Fatalf("add member: %v", err)
	}

	// The owner is now the last owner of a workspace with somebody else in it.
	err = store.CloseAccount(ctx, owner.ID, email)
	if !errors.Is(err, ErrLastOwner) {
		t.Fatalf("want ErrLastOwner, got %v", err)
	}
}

// TestPgCrossTenantSessionRevokeDenied — US-AD05 AC2.
//
// The service-level guard, against real rows: an admin of one workspace may not
// revoke a session belonging to someone outside it.
func TestPgCrossTenantSessionRevokeDenied(t *testing.T) {
	ctx := context.Background()
	store := pgTestStore(t)

	adminEmail := uniqueEmail(t, "pg.admin@example.com")
	outsiderEmail := uniqueEmail(t, "pg.outsider@example.com")
	_, workspaceA, _, err := store.Register(ctx, adminEmail, "password1", "", "Team A", SessionMeta{})
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	_, _, _, err = store.Register(ctx, outsiderEmail, "password1", "", "Team B", SessionMeta{})
	if err != nil {
		t.Fatalf("register outsider: %v", err)
	}
	outsiderSession, err := store.Login(ctx, outsiderEmail, "password1", SessionMeta{})
	if err != nil {
		t.Fatalf("outsider login: %v", err)
	}
	outsiderSessionID, err := store.SessionIDForToken(ctx, outsiderSession)
	if err != nil {
		t.Fatalf("outsider session id: %v", err)
	}

	err = store.RevokeSession(ctx, "someone-in-team-a", adminEmail, workspaceA.ID, outsiderSessionID, true)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("cross-tenant revoke: want ErrSessionNotFound, got %v", err)
	}
	// The outsider's session is untouched.
	if _, ok := store.Authenticate(ctx, outsiderSession); !ok {
		t.Fatal("the outsider's session was revoked across tenants")
	}
}
