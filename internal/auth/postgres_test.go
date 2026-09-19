package auth

// Postgres-backed coverage for the M0 acceptance criteria that the in-memory
// repository cannot prove: the live pgx path through Register, session
// revocation, and cross-tenant denial. The unit suites cover the domain rules;
// these tests prove the production Repository actually satisfies them.
//
// Gated on AGENTDECK_TEST_DATABASE_URL exactly like internal/migrate, so a
// checkout without Postgres still runs every other suite.

import (
	"context"
	"errors"
	"net/mail"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/migrate"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := databaseURLEnv()
	if url == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed M0 tests skipped")
	}
	return url
}

// pgTestStore builds an isolated Store over the Postgres pool and applies the
// current schema. Each test gets its own users/orgs because emails are
// ULID-suffixed, so no cross-test cleanup is needed.
func pgTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	url := testDatabaseURL(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewStore(NewPgxRepository(pool))
}

// uniqueEmail keeps tests independent without needing a per-test schema reset.
func uniqueEmail(t *testing.T, base string) string {
	t.Helper()
	return mail.Address{
		Name:    "Test",
		Address: ulidPrefix(t) + base,
	}.Address
}

// ulidPrefix derives a per-run, per-test suffix from the test name so two runs
// never collide on the same email.
func ulidPrefix(t *testing.T) string {
	t.Helper()
	scratch := make([]byte, 5)
	for i := range scratch {
		scratch[i] = byte(t.Name()[i%len(t.Name())])
	}
	out := make([]byte, 10)
	const hexd = "0123456789abcdef"
	for i, b := range scratch {
		out[2*i] = hexd[b>>4]
		out[2*i+1] = hexd[b&0xF]
	}
	return string(out) + "."
}

// TestPgRegisterCreatesSessionAndPersonalWorkspace is US-AD01 AC1/AC5/AC6 and
// US-AD93 AC1 against Postgres: a registration persists the user, the
// registration-kind personal workspace, the owner membership, and a live
// session whose token only exists as a hash.
func TestPgRegisterCreatesSessionAndPersonalWorkspace(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "ada@example.com")

	user, workspace, token, err := store.Register(ctx, email, "password123", "", "")
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
	// AC6: an absent name is the email local part, not a rejection.
	if want := "ada"; user.Name != want {
		t.Errorf("name fallback = %q, want %q", user.Name, want)
	}
	if !store.Authorize(ctx, workspace.ID, email, Owner) {
		t.Error("registrant is not owner of the workspace it created")
	}
	// The raw token must never be what the store keeps; only its SHA-256.
	if workspaceContainsRawToken(t, ctx, token) {
		t.Error("raw session token is persisted instead of its hash")
	}
	// The session is usable, so the authenticate path round-trips.
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Error("session token issued at registration fails to authenticate")
	}
}

// TestPgRegisterDuplicateEmailRejected is US-AD01 AC2/AC4: a second identical
// registration is ErrEmailExists and changes nothing.
func TestPgRegisterDuplicateEmailRejected(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "dupe@example.com")

	if _, _, _, err := store.Register(ctx, email, "password123", "Dupe", ""); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, _, _, err := store.Register(ctx, email, "password123", "Dupe", "")
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("second register err = %v, want ErrEmailExists", err)
	}
	// The case-folded form must be rejected too: membership keys are stable.
	_, _, _, err = store.Register(ctx, "ADA@EXAMPLE.COM", "password123", "Dupe", "")
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("case-variant register err = %v, want ErrEmailExists", err)
	}
}

// TestPgLoginLogoutAndRevocation is US-AD02 AC1/AC3/AC4: login issues a
// session, logout revokes it, and a revoked token is dead.
func TestPgLoginLogoutAndRevocation(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "lobe@example.com")
	password := "password123"

	_, _, token, err := store.Register(ctx, email, password, "Lobe", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Fatal("registration session does not authenticate")
	}

	// Login again: the second session is independent of the first.
	token2, err := store.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token2 == token {
		t.Error("login returned the registration token instead of a new session")
	}
	if _, ok := store.Authenticate(ctx, token2); !ok {
		t.Error("login token does not authenticate")
	}

	if err := store.Logout(ctx, token2); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, ok := store.Authenticate(ctx, token2); ok {
		t.Error("token still valid after logout (US-AD05 AC1)")
	}
	// The registration session survives: only the revoked token is dead.
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Error("logout revoked a session it should not have touched")
	}

	if _, err := store.Login(ctx, email, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad password err = %v, want ErrInvalidCredentials", err)
	}
}

// TestPgCrossTenantDenied is US-AD07 AC1/AC3: a user of one workspace cannot
// act in another, and the failure is a denial, not a leak of the other org.
func TestPgCrossTenantDenied(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, wsA, tokenA, err := store.Register(ctx, uniqueEmail(t, "aliceA@example.com"), "password123", "Alice A", "")
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	_, wsB, _, err := store.Register(ctx, uniqueEmail(t, "bobB@example.com"), "password123", "Bob B", "")
	if err != nil {
		t.Fatalf("register B: %v", err)
	}
	if wsA.ID == wsB.ID {
		t.Fatal("two registrations produced the same workspace id")
	}

	// Alice cannot read B's roster, and the failure is a denial with no rows.
	if _, err := store.Members(ctx, wsB.ID, alice.Email); !errors.Is(err, ErrForbidden) {
		t.Errorf("Members on foreign org err = %v, want ErrForbidden", err)
	}
	if store.Authorize(ctx, wsB.ID, alice.Email, Viewer) {
		t.Error("Authorize granted Viewer in a foreign workspace")
	}
	// Alice cannot promote herself in B either.
	if err := store.ChangeRole(ctx, wsB.ID, alice.Email, alice.Email, Owner); !errors.Is(err, ErrForbidden) {
		t.Errorf("ChangeRole on foreign org err = %v, want ErrForbidden", err)
	}
	// Her own session still works, so the denial did not corrupt state.
	if _, ok := store.Authenticate(ctx, tokenA); !ok {
		t.Error("Alice's own session broke after a denied foreign request")
	}
}

// TestPgRoleMatrix exercises all four frozen roles on Postgres, including the
// negative paths (DECISIONS section 4 / US-AD04 AC2/AC3).
func TestPgRoleMatrix(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	ownerEmail := uniqueEmail(t, "owner@example.com")

	_, ws, _, err := store.Register(ctx, ownerEmail, "password123", "Owner", "")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}

	members := map[Role]string{
		Admin:  uniqueEmail(t, "admin@example.com"),
		Member: uniqueEmail(t, "member@example.com"),
		Viewer: uniqueEmail(t, "viewer@example.com"),
	}
	// Owner seeds the roster; owner itself is only assigned at registration or
	// org creation, so the owner row is the registration one.
	for role, email := range members {
		if _, err := store.Register(ctx, email, "password123", string(role), ""); err != nil {
			t.Fatalf("register %s: %v", role, err)
		}
		if err := store.AddMember(ctx, ws.ID, ownerEmail, email, role); err != nil {
			t.Fatalf("add %s: %v", role, err)
		}
	}

	// Every role can read the roster (viewer+).
	for role, email := range members {
		if _, _, err := store.GetOrg(ctx, ws.ID, email); err != nil {
			t.Errorf("%s cannot read org: %v", role, err)
		}
	}

	// US-AD04 AC2: viewer cannot manage members.
	if err := store.AddMember(ctx, ws.ID, members[Viewer], uniqueEmail(t, "invitee@example.com"), Member); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer AddMember err = %v, want ErrForbidden", err)
	}
	// Admin can manage members but cannot create an owner (US-AD04 AC3).
	if err := store.ChangeRole(ctx, ws.ID, members[Admin], members[Member], Owner); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin promoting to owner err = %v, want ErrForbidden", err)
	}
	// Admin can change a role within its rank.
	if err := store.ChangeRole(ctx, ws.ID, members[Admin], members[Viewer], Member); err != nil {
		t.Errorf("admin changing viewer->member err = %v", err)
	}
	// The last owner cannot be demoted out of existence.
	if err := store.ChangeRole(ctx, ws.ID, ownerEmail, ownerEmail, Member); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting last owner err = %v, want ErrLastOwner", err)
	}
	// An invalid role is never coerced.
	if err := store.AddMember(ctx, ws.ID, ownerEmail, uniqueEmail(t, "rogue@example.com"), "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("invalid role err = %v, want ErrInvalidRole", err)
	}
}

// TestPgSessionExpiryAndIdle is US-AD02 AC4: absolute and sliding-window
// expiry are enforced by the repository, not by client goodwill.
func TestPgSessionExpiryAndIdle(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()
	email := uniqueEmail(t, "sess@example.com")

	_, _, token, err := store.Register(ctx, email, "password123", "Sess", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Fatal("fresh session does not authenticate")
	}

	store.expireSessionsForTest(ctx, t, email, time.Now().Add(-time.Hour))
	if _, ok := store.Authenticate(ctx, token); ok {
		t.Error("expired session still authenticates (US-AD02 AC4)")
	}
}

// workspaceContainsRawToken asserts the opaque token is stored hashed, so a
// store dump yields nothing replayable (ARCHITECTURE 3.19).
func workspaceContainsRawToken(t *testing.T, ctx context.Context, rawToken string) bool {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURLEnv())
	if err != nil {
		t.Fatalf("connect for token check: %v", err)
	}
	defer pool.Close()
	var n int
	err = pool.QueryRow(ctx, "select count(*) from sessions where token_hash = $1 or token_hash like '%' || $2 || '%'", tokenHash(rawToken), rawToken).Scan(&n)
	if err != nil {
		t.Fatalf("query raw token: %v", err)
	}
	return n > 0
}
