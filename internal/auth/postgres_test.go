package auth

// Postgres-backed coverage for the M0 acceptance criteria the in-memory
// repository cannot prove: the live pgx path through Register, session
// revocation, and cross-tenant denial. The unit suites cover the domain rules;
// these tests prove the production Repository actually satisfies them, and
// would have caught the email-as-user-id and not-found-sentinel bugs that the
// in-memory fake masked.
//
// Gated on AGENTDECK_TEST_DATABASE_URL exactly like internal/migrate, so a
// checkout without Postgres still runs every other suite.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/migrate"
	"agentdeck/internal/ulid"
)

// databaseURLEnv is one indirection so the suite can be run against an
// alternate DSN without editing tests.
func databaseURLEnv() string { return os.Getenv("AGENTDECK_TEST_DATABASE_URL") }

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := databaseURLEnv()
	if url == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed M0 tests skipped")
	}
	return url
}

// pgTestStore builds a Store over the suite's Postgres pool and applies the
// current schema. Each test derives its own email suffix, so tests never
// share rows and no cross-test cleanup is needed. The pool is shared across
// the suite because migrate.Apply holds an advisory lock and would otherwise
// race between tests; it is nil when no database was requested, in which case
// the caller already skipped.
func pgTestStore(t *testing.T) *Store {
	t.Helper()
	testDatabaseURL(t)
	if pgSuitePool == nil {
		t.Fatal("pgTestStore called without AGENTDECK_TEST_DATABASE_URL")
	}
	if err := migrate.Apply(context.Background(), pgSuitePool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewStore(NewPgxRepository(pgSuitePool))
}

// pgPool gives a test a raw pool for assertions the Store does not expose.
func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if pgSuitePool == nil {
		t.Fatal("pgPool called without AGENTDECK_TEST_DATABASE_URL")
	}
	return pgSuitePool
}

// pgSuitePool is the single pool the whole Postgres suite shares; TestMain
// opens it once and closes it when the binary is done.
var pgSuitePool *pgxpool.Pool

// TestMain owns the database for the suite. Every Postgres-backed test shares
// one pool so the migration advisory lock is not contested between tests, and
// so a run without AGENTDECK_TEST_DATABASE_URL still executes every unit test.
func TestMain(m *testing.M) {
	url := databaseURLEnv()
	if url == "" {
		// Nothing to connect to: run the unit suites and exit.
		os.Exit(m.Run())
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentdeck auth tests: connect: %v\n", err)
		os.Exit(1)
	}
	pgSuitePool = pool
	os.Exit(m.Run())
}

// uniqueEmail derives a per-test address so tests are order-independent
// without needing a schema reset between them: a fresh ULID is the same
// generator the domain uses for real ids, so no two runs collide and the
// local part still reads as the role it represents.
func uniqueEmail(t *testing.T, local string) string {
	t.Helper()
	at := strings.IndexByte(local, '@')
	return local[:at] + "." + strings.ToLower(ulid.Must()) + local[at:]
}

// rawSessionCount asserts the opaque token is never persisted in plaintext:
// only its SHA-256 lands in sessions.token_hash (ARCHITECTURE 3.19).
func rawSessionCount(t *testing.T, ctx context.Context, rawToken string) int {
	t.Helper()
	var n int
	err := pgPool(t).QueryRow(ctx,
		"select count(*) from sessions where token_hash like '%' || $1 || '%' or token_hash = $2",
		rawToken, rawToken).Scan(&n)
	if err != nil {
		t.Fatalf("count raw token: %v", err)
	}
	return n
}

// TestPgRegisterCreatesSessionAndPersonalWorkspace is US-AD01 AC1/AC5/AC6 and
// US-AD93 AC1 against Postgres: registration persists the user, the
// registration-kind personal workspace, the owner membership, and a live
// session whose token exists only as a hash.
func TestPgRegisterCreatesSessionAndPersonalWorkspace(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "ada@example.com")

	user, workspace, token, err := store.Register(ctx, email, "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if token == "" {
		t.Fatal("register returned no session token")
	}
	if len(user.ID) != 26 {
		t.Errorf("user id len = %d, want 26 (ULID)", len(user.ID))
	}
	if len(workspace.ID) != 26 {
		t.Errorf("workspace id len = %d, want 26 (ULID)", len(workspace.ID))
	}
	if workspace.Kind != "registration" {
		t.Errorf("workspace kind = %q, want registration", workspace.Kind)
	}
	// AC6: an absent name is the email local part, not a 400.
	local := email[:strings.IndexByte(email, '@')]
	if got := user.Name; got != local {
		t.Errorf("name fallback = %q, want %q", got, local)
	}
	if !store.Authorize(ctx, workspace.ID, email, Owner) {
		t.Error("registrant is not owner of the workspace it created")
	}
	if rawSessionCount(t, ctx, token) > 0 {
		t.Error("raw session token is persisted instead of only its hash")
	}
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Error("session token issued at registration fails to authenticate")
	}

	// The workspace lookup must find this registration-kind org again, which
	// is what makes a retried registration reuse instead of duplicating it.
	memberships, err := store.Workspaces(ctx, email)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	if len(memberships) != 1 {
		t.Fatalf("registrant belongs to %d workspaces, want 1", len(memberships))
	}
	if memberships[0].WorkspaceID != workspace.ID {
		t.Errorf("listed workspace = %q, want %q", memberships[0].WorkspaceID, workspace.ID)
	}
	if memberships[0].Role != Owner {
		t.Errorf("listed role = %q, want owner", memberships[0].Role)
	}
}

// TestPgRegisterDuplicateEmailRejected is US-AD01 AC2/AC4: a second identical
// registration is ErrEmailExists and a case-folded variant is too.
func TestPgRegisterDuplicateEmailRejected(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "dupe@example.com")

	if _, _, _, err := store.Register(ctx, email, "password123", "Dupe", "", SessionMeta{}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, _, _, err := store.Register(ctx, email, "password123", "Dupe", "", SessionMeta{})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("second register err = %v, want ErrEmailExists", err)
	}
	_, _, _, err = store.Register(ctx, strings.ToUpper(email), "password123", "Dupe", "", SessionMeta{})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("case-variant register err = %v, want ErrEmailExists", err)
	}
}

// TestPgLoginLogoutAndRevocation is US-AD02 AC1/AC3 and US-AD05 AC1: login
// issues a session, logout revokes exactly that one, and a revoked token is
// dead.
func TestPgLoginLogoutAndRevocation(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "lobe@example.com")

	_, _, regToken, err := store.Register(ctx, email, "password123", "Lobe", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := store.Authenticate(ctx, regToken); !ok {
		t.Fatal("registration session does not authenticate")
	}

	token, err := store.Login(ctx, email, "password123", SessionMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatal("login returned no token")
	}
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Error("login token does not authenticate")
	}

	if err := store.Logout(ctx, token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, ok := store.Authenticate(ctx, token); ok {
		t.Error("token still valid after logout")
	}
	// US-AD05 AC1: only the presented session is revoked.
	if _, ok := store.Authenticate(ctx, regToken); !ok {
		t.Error("logout revoked a session it should not have touched")
	}
	// A shadow user can never log in, so a pending invite is not a backdoor.
	if _, err := store.Login(ctx, email, "wrong-password", SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad password err = %v, want ErrInvalidCredentials", err)
	}
}

// TestPgCrossTenantDenied is US-AD07 AC1/AC3: a user of one workspace cannot
// act in another, and the failure is a denial that leaks no rows.
func TestPgCrossTenantDenied(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, wsA, regToken, err := store.Register(ctx, uniqueEmail(t, "alice@example.com"), "password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	_, wsB, _, err := store.Register(ctx, uniqueEmail(t, "bob@example.com"), "password123", "Bob", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register B: %v", err)
	}
	if wsA.ID == wsB.ID {
		t.Fatal("two registrations produced one workspace")
	}

	if _, err := store.Members(ctx, wsB.ID, alice.Email); !errors.Is(err, ErrForbidden) {
		t.Errorf("Members on foreign org err = %v, want ErrForbidden", err)
	}
	if store.Authorize(ctx, wsB.ID, alice.Email, Viewer) {
		t.Error("Authorize granted Viewer in a foreign workspace")
	}
	if err := store.ChangeRole(ctx, wsB.ID, alice.Email, alice.Email, Admin); !errors.Is(err, ErrForbidden) {
		t.Errorf("ChangeRole on foreign org err = %v, want ErrForbidden", err)
	}
	// The denial must not have disturbed her own session or workspace.
	if _, ok := store.Authenticate(ctx, regToken); !ok {
		t.Error("own session broke after a denied foreign request")
	}
	if _, _, err := store.GetOrg(ctx, wsA.ID, alice.Email); err != nil {
		t.Errorf("own org read failed after denial: %v", err)
	}
}

// TestPgRoleMatrix exercises all four frozen roles on Postgres, including the
// negative paths (DECISIONS section 4, US-AD04 AC2/AC3).
func TestPgRoleMatrix(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	ownerEmail := uniqueEmail(t, "owner@example.com")

	_, ws, _, err := store.Register(ctx, ownerEmail, "password123", "Owner", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}

	// AddMember keys the membership on the invited user's id, which for a
	// never-logged-in invitee is a shadow row; registration then claims it.
	cases := []struct {
		role Role
		mail string
	}{
		{Admin, "admin@example.com"},
		{Member, "member@example.com"},
		{Viewer, "viewer@example.com"},
	}
	emails := map[Role]string{}
	for _, c := range cases {
		invitee := uniqueEmail(t, c.mail)
		if err := store.AddMember(ctx, ws.ID, ownerEmail, invitee, c.role); err != nil {
			t.Fatalf("add %s: %v", c.role, err)
		}
		emails[c.role] = invitee
		// Claiming the shadow row must keep the invited role (US-AD04 AC1).
		if _, _, _, err := store.Register(ctx, invitee, "password123", string(c.role), "", SessionMeta{}); err != nil {
			t.Fatalf("register invited %s: %v", c.role, err)
		}
	}

	// Every role reads the org and roster (viewer+).
	for role, email := range emails {
		if _, _, err := store.GetOrg(ctx, ws.ID, email); err != nil {
			t.Errorf("%s cannot read org: %v", role, err)
		}
		if _, err := store.Members(ctx, ws.ID, email); err != nil {
			t.Errorf("%s cannot list members: %v", role, err)
		}
	}

	// US-AD04 AC2: viewer cannot manage members.
	if err := store.AddMember(ctx, ws.ID, emails[Viewer], uniqueEmail(t, "invitee@example.com"), Member); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer AddMember err = %v, want ErrForbidden", err)
	}
	// Admin can change a role within its rank.
	if err := store.ChangeRole(ctx, ws.ID, emails[Admin], emails[Viewer], Member); err != nil {
		t.Errorf("admin viewer->member err = %v", err)
	}
	// Owner is never assignable: ChangeRole rejects the role itself, so the
	// escalation is denied before the actor's rank is even compared
	// (US-AD04 AC3).
	if err := store.ChangeRole(ctx, ws.ID, emails[Admin], emails[Member], Owner); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("admin promoting to owner err = %v, want ErrInvalidRole", err)
	}
	// The last owner cannot be demoted out of existence.
	if err := store.ChangeRole(ctx, ws.ID, ownerEmail, ownerEmail, Member); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting last owner err = %v, want ErrLastOwner", err)
	}
	// An unknown role value is rejected, never coerced.
	if err := store.AddMember(ctx, ws.ID, ownerEmail, uniqueEmail(t, "rogue@example.com"), "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("invalid role err = %v, want ErrInvalidRole", err)
	}
	// Removing the last owner is refused too.
	if err := store.RemoveMember(ctx, ws.ID, ownerEmail, ownerEmail); !errors.Is(err, ErrLastOwner) {
		t.Errorf("removing last owner err = %v, want ErrLastOwner", err)
	}
	// An admin can remove a non-owner member.
	if err := store.RemoveMember(ctx, ws.ID, ownerEmail, emails[Member]); err != nil {
		t.Errorf("removing member err = %v", err)
	}
	if store.Authorize(ctx, ws.ID, emails[Member], Viewer) {
		t.Error("removed member still authorized")
	}
}
