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

	user, workspace, session, err := store.Register(ctx, "Ada@Example.com", "password1", "", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Name != "Ada" || workspace.ID == "" || session == "" {
		t.Fatalf("register returned incomplete result: user=%#v workspace=%#v", user, workspace)
	}
	if workspace.Name != "Ada's workspace" {
		t.Fatalf("workspace name = %q, want %q", workspace.Name, "Ada's workspace")
	}

	if _, _, _, err = store.Register(ctx, "ada@example.com", "password1", "", ""); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate registration: got %v, want ErrEmailExists", err)
	}
	if _, ok := store.Authenticate(ctx, session); !ok {
		t.Fatal("session not authenticated")
	}

	store.Logout(ctx, session)
	if _, ok := store.Authenticate(ctx, session); ok {
		t.Fatal("revoked session accepted")
	}

	loginSession, err := store.Login(ctx, "ada@example.com", "password1")
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
			if _, _, _, err := store.Register(ctx, tc.email, tc.password, "", ""); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRegisterUsesEmailLocalPartWhenNameAbsent(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	user, _, _, err := store.Register(ctx, "john.doe@example.com", "password1", "", "")
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

	_, workspace, _, err := store.Register(ctx, "a@example.com", "password1", "Alice", "Acme Ops")
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
	_, workspaceA, _, _ := store.Register(ctx, "a@example.com", "password1", "", "")
	_, workspaceB, _, _ := store.Register(ctx, "b@example.com", "password1", "", "")

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
	_, workspace, _, _ := store.Register(ctx, "owner@example.com", "password1", "", "")
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
	_, workspace, _, _ := store.Register(ctx, "owner@example.com", "password1", "", "")
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

func TestLoginLocksAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, _, err := store.Register(ctx, "locked@example.com", "password1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := store.Login(ctx, "locked@example.com", "wrong"); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if _, err := store.Login(ctx, "locked@example.com", "password1"); !errors.Is(err, ErrAccountLocked) {
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

func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "x@example.com", "password1", "", "")

	setSessionExpiry(t, store, sessionToken, time.Now().Add(-8*24*time.Hour))

	if _, ok := store.Authenticate(ctx, sessionToken); ok {
		t.Fatal("expired session accepted")
	}
}

func TestSessionIdleTimeout(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "idle@example.com", "password1", "", "")

	setSessionLastSeen(t, store, sessionToken, time.Now().Add(-25*time.Hour))

	if _, ok := store.Authenticate(ctx, sessionToken); ok {
		t.Fatal("idle session accepted past the 24h sliding window")
	}
}

func TestAuthenticateRefreshesLastSeen(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	_, _, sessionToken, _ := store.Register(ctx, "active@example.com", "password1", "", "")

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
	_, workspaceA, _, _ := store.Register(ctx, "a@example.com", "password1", "", "")
	_, workspaceB, _, _ := store.Register(ctx, "b@example.com", "password1", "", "")
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
