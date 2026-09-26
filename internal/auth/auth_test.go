package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegisterLoginLogoutAndWorkspace(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	user, workspace, session, err := store.Register(ctx, "Ada@Example.com", "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Name != "Ada" || workspace.ID == "" || session == "" {
		t.Fatalf("register returned incomplete result: user=%#v workspace=%#v", user, workspace)
	}
	if workspace.Name != "Ada's workspace" {
		t.Fatalf("workspace name = %q, want %q", workspace.Name, "Ada's workspace")
	}

	if _, _, _, err = store.Register(ctx, "ada@example.com", "password1", "", "", SessionMeta{}); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate registration: got %v, want ErrEmailExists", err)
	}
	if _, ok := store.Authenticate(ctx, session); !ok {
		t.Fatal("session not authenticated")
	}

	store.Logout(ctx, session)
	if _, ok := store.Authenticate(ctx, session); ok {
		t.Fatal("revoked session accepted")
	}

	loginSession, err := store.Login(ctx, "ada@example.com", "password1", SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Authenticate(ctx, loginSession); !ok {
		t.Fatal("login session invalid")
	}
}

func TestRegisterValidation(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	cases := []struct {
		name     string
		email    string
		password string
	}{
		{"empty email", "", "password1"},
		{"missing at", "notanemail", "password1"},
		{"no local part", "@example.com", "password1"},
		{"no domain dot", "a@example", "password1"},
		{"short password", "a@example.com", "short"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := store.Register(ctx, tc.email, tc.password, "", "", SessionMeta{}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRegisterUsesEmailLocalPartWhenNameAbsent(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	user, _, _, err := store.Register(ctx, "john.doe@example.com", "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "john.doe" {
		t.Fatalf("name fallback = %q, want %q", user.Name, "john.doe")
	}
}

func TestRegisterKeepsExplicitOrgName(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	_, workspace, _, err := store.Register(ctx, "a@example.com", "password1", "Alice", "Acme Ops", SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Name != "Acme Ops" {
		t.Fatalf("workspace name = %q, want %q", workspace.Name, "Acme Ops")
	}

	workspaces, err := store.Workspaces(ctx, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(workspaces))
	}
	if workspaces[0].Role != Owner {
		t.Fatalf("role = %q, want owner", workspaces[0].Role)
	}
	if workspaces[0].Name != "Acme Ops" {
		t.Fatalf("workspace list name = %q, want %q", workspaces[0].Name, "Acme Ops")
	}
}

func TestRBACAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, workspaceA, _, _ := store.Register(ctx, "a@example.com", "password1", "", "", SessionMeta{})
	_, workspaceB, _, _ := store.Register(ctx, "b@example.com", "password1", "", "", SessionMeta{})

	if !store.Authorize(ctx, workspaceA.ID, "a@example.com", Owner) ||
		!store.Authorize(ctx, workspaceA.ID, "a@example.com", Admin) {
		t.Fatal("owner matrix failed")
	}
	if store.Authorize(ctx, workspaceA.ID, "b@example.com", Viewer) ||
		store.Authorize(ctx, workspaceB.ID, "a@example.com", Viewer) {
		t.Fatal("cross-tenant access leaked")
	}

	if err := store.AddMember(ctx, workspaceA.ID, "a@example.com", "b@example.com", Member); err != nil {
		t.Fatal(err)
	}
	if !store.Authorize(ctx, workspaceA.ID, "b@example.com", Member) ||
		store.Authorize(ctx, workspaceA.ID, "b@example.com", Admin) {
		t.Fatal("member matrix failed")
	}
	if err := store.AddMember(ctx, workspaceA.ID, "b@example.com", "a@example.com", Admin); err == nil {
		t.Fatal("member escalated role")
	}
	if err := store.ChangeRole(ctx, workspaceA.ID, "a@example.com", "b@example.com", Admin); err != nil ||
		!store.Authorize(ctx, workspaceA.ID, "b@example.com", Admin) {
		t.Fatal("admin role change failed")
	}
}

func TestChangeRoleGuardsOwner(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, workspace, _, _ := store.Register(ctx, "owner@example.com", "password1", "", "", SessionMeta{})
	if err := store.ChangeRole(ctx, workspace.ID, "owner@example.com", "owner@example.com", Admin); err == nil {
		t.Fatal("owner self-demotion accepted")
	}

	if err := store.AddMember(ctx, workspace.ID, "owner@example.com", "member@example.com", Member); err != nil {
		t.Fatal(err)
	}
	if err := store.ChangeRole(ctx, workspace.ID, "owner@example.com", "member@example.com", Owner); err == nil {
		t.Fatal("owner granted to non-owner")
	}
}

func TestRemoveMemberProtectsLastOwner(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, workspace, _, _ := store.Register(ctx, "owner@example.com", "password1", "", "", SessionMeta{})
	if err := store.RemoveMember(ctx, workspace.ID, "owner@example.com", "owner@example.com"); err == nil {
		t.Fatal("last owner removal accepted")
	}

	if err := store.AddMember(ctx, workspace.ID, "owner@example.com", "member@example.com", Member); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveMember(ctx, workspace.ID, "owner@example.com", "member@example.com"); err != nil {
		t.Fatalf("member removal failed: %v", err)
	}
	if store.Authorize(ctx, workspace.ID, "member@example.com", Viewer) {
		t.Fatal("removed member still authorized")
	}
}

// TestMemberByIDUsesPublicID is the regression test for the
// PATCH/DELETE /api/v1/orgs/{id}/members/{user_id} path: those endpoints hand
// the handler a public ULID, not an email. Lowercasing that id broke the
// Crockford-base-32 lookup and every member-management call returned 404 even
// for a membership that exists.
func TestMemberByIDUsesPublicID(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	owner, workspace, _, err := store.Register(ctx, "owner@example.com", "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, workspace.ID, "owner@example.com", "member@example.com", Member); err != nil {
		t.Fatal(err)
	}

	// The public id the API exposes is the ULID, so the by-id path must round
	// trip through it rather than the email.
	roster, err := store.Members(ctx, workspace.ID, "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var memberID string
	for _, m := range roster {
		if m.Email == "member@example.com" {
			memberID = m.UserID
		}
	}
	if memberID == "" {
		t.Fatal("invited member not listed")
	}

	if err := store.ChangeMemberRole(ctx, workspace.ID, "owner@example.com", memberID, Admin); err != nil {
		t.Fatalf("change role by id: %v", err)
	}
	if !store.Authorize(ctx, workspace.ID, "member@example.com", Admin) {
		t.Fatal("role change by id did not take effect")
	}

	if err := store.RemoveMemberByID(ctx, workspace.ID, "owner@example.com", memberID); err != nil {
		t.Fatalf("remove member by id: %v", err)
	}
	if store.Authorize(ctx, workspace.ID, "member@example.com", Viewer) {
		t.Fatal("removed member still authorized")
	}

	// An unknown id is 404 (ErrMemberNotFound), not a leak.
	if err := store.RemoveMemberByID(ctx, workspace.ID, "owner@example.com", owner.ID+"Z"); err == nil {
		t.Fatal("unknown member id accepted")
	}
}

func TestLoginLocksAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, _, err := store.Register(ctx, "locked@example.com", "password1", "", "", SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := store.Login(ctx, "locked@example.com", "wrong", SessionMeta{}); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if _, err := store.Login(ctx, "locked@example.com", "password1", SessionMeta{}); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected lockout, got %v", err)
	}
	if unlocksAt := store.LockedUntil("locked@example.com"); unlocksAt.IsZero() {
		t.Fatal("LockedUntil reported no lock on a locked account")
	}
}

func TestPasswordHashUsesArgon2id(t *testing.T) {
	hash := hashPassword("password1")
	if !strings.HasPrefix(hash, "$argon2id$") || !verifyPassword(hash, "password1") || verifyPassword(hash, "wrong") {
		t.Fatalf("invalid argon2id password hash: %q", hash)
	}
}

// TestCreateWorkspaceRejectsDuplicateSlug is the regression test for F3:
// CreateOrg's unique-violation branch has to surface as ErrSlugTaken so the
// API answers 409. Before the mapping was made context-aware, a slug clash was
// reported as ErrUserNotFound and the handler would have returned 404 for an
// org id the caller never sent.
func TestCreateWorkspaceRejectsDuplicateSlug(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	if _, _, _, err := store.Register(ctx, "owner@example.com", "password1", "", "", SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(ctx, "owner@example.com", "My Team", "my-team"); err != nil {
		t.Fatal(err)
	}
	_, err := store.CreateWorkspace(ctx, "owner@example.com", "Other Team", "my-team")
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug: got %v, want ErrSlugTaken", err)
	}
}

// TestTenantIsolationOnUnknownOrg covers the membership reads on an org the
// actor does not belong to (F5). A member-management call scoped to a foreign
// or nonexistent org must report not-found, never a role row.
func TestTenantIsolationOnUnknownOrg(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	if _, _, _, err := store.Register(ctx, "owner@example.com", "password1", "", "", SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	// An org id with no membership at all.
	if _, err := store.Members(ctx, "01ABCDEFGHILKJMNPRSTUVWXY", "owner@example.com"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("members of unknown org: got %v, want ErrWorkspaceNotFound", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "x@example.com", "password1", "", "", SessionMeta{})

	setSessionExpiry(t, store, sessionToken, time.Now().Add(-8*24*time.Hour))

	if _, ok := store.Authenticate(ctx, sessionToken); ok {
		t.Fatal("expired session accepted")
	}
}

func TestSessionIdleTimeout(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "idle@example.com", "password1", "", "", SessionMeta{})

	setSessionLastSeen(t, store, sessionToken, time.Now().Add(-25*time.Hour))

	if _, ok := store.Authenticate(ctx, sessionToken); ok {
		t.Fatal("idle session accepted past the 24h sliding window")
	}
}

func TestAuthenticateRefreshesLastSeen(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "active@example.com", "password1", "", "", SessionMeta{})

	setSessionLastSeen(t, store, sessionToken, time.Now().Add(-12*time.Hour))

	if _, ok := store.Authenticate(ctx, sessionToken); !ok {
		t.Fatal("session rejected inside the sliding window")
	}

	refreshed := sessionLastSeen(t, store, sessionToken)
	if time.Since(refreshed) > time.Minute {
		t.Fatal("Authenticate did not refresh last_seen_at")
	}
}

func TestWorkspacesListsOnlyOwnMemberships(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, workspaceA, _, _ := store.Register(ctx, "a@example.com", "password1", "", "", SessionMeta{})
	_, workspaceB, _, _ := store.Register(ctx, "b@example.com", "password1", "", "", SessionMeta{})
	if err := store.AddMember(ctx, workspaceA.ID, "a@example.com", "b@example.com", Viewer); err != nil {
		t.Fatal(err)
	}

	list, err := store.Workspaces(ctx, "b@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || len(list) != 2 {
		t.Fatalf("b memberships = %d, want 2", len(list))
	}
	list, err = store.Workspaces(ctx, "a@example.com")
	if err != nil || len(list) != 1 {
		t.Fatalf("a memberships = %d, want 1", len(list))
	}
	// Unknown users see no workspaces rather than an error.
	list, err = store.Workspaces(ctx, "ghost@example.com")
	if err != nil || len(list) != 0 {
		t.Fatalf("unknown user memberships = %d, want 0", len(list))
	}
	_ = workspaceB
}
