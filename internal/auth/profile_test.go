package auth

import (
	"context"
	"errors"
	"testing"
)

// ptr is the pointer constructor the ProfileUpdate patch needs: a nil field
// means "absent", so every test value has to be addressable.
func ptr[T any](value T) *T { return &value }

// US-AD89 AC1: the payload the Profile screen renders carries the four fields
// the AC names — id, email, name, avatar_user. The avatar is derived, so it is
// checked here rather than through the HTTP layer.
func TestAvatarForDerivesMonogram(t *testing.T) {
	cases := []struct {
		name     string
		display  string
		email    string
		url      string
		kind     string
		initials string
	}{
		{"two words", "Reza Zulfi", "reza@example.com", "", "monogram", "RZ"},
		{"one word", "Reza", "reza@example.com", "", "monogram", "R"},
		{"empty name falls back to email", "", "reza@example.com", "", "monogram", "R"},
		{"hyphenated name", "ada-lovelace", "ada@example.com", "", "monogram", "AL"},
		{"uploaded image wins", "Reza", "reza@example.com", "https://cdn/a.png", "image", "R"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AvatarFor(tc.display, tc.email, tc.url)
			if got.Kind != tc.kind {
				t.Fatalf("kind = %q, want %q", got.Kind, tc.kind)
			}
			if got.Initials != tc.initials {
				t.Fatalf("initials = %q, want %q", got.Initials, tc.initials)
			}
			if got.SizePX != 26 || got.BgColor != "#101014" {
				t.Fatalf("avatar disc = %+v, want the design's 26px #101014 disc", got)
			}
			if tc.url != "" && got.URL != tc.url {
				t.Fatalf("url = %q, want %q", got.URL, tc.url)
			}
		})
	}
}

// US-AD89 AC2: changing the name returns the updated identity, so the topbar can
// re-render from the mutation's own response instead of a follow-up GET.
func TestUpdateProfileChangesName(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	user, _, _, err := store.Register(ctx, "reza@example.com", "password1", "Reza", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	updated, err := store.UpdateProfile(ctx, user.ID, ProfileUpdate{Name: ptr("Reza Zulfi")})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Name != "Reza Zulfi" {
		t.Fatalf("name = %q, want %q", updated.Name, "Reza Zulfi")
	}
	if updated.ID != user.ID {
		t.Fatalf("id changed: %q -> %q", user.ID, updated.ID)
	}

	// The change is durable, not just echoed back.
	reread, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if reread.Name != "Reza Zulfi" {
		t.Fatalf("stored name = %q, want %q", reread.Name, "Reza Zulfi")
	}
}

// US-AD89 AC3 (failure path): an email already owned by another account is a
// conflict, and the write must not land.
func TestUpdateProfileRejectsTakenEmail(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	first, _, _, err := store.Register(ctx, "ada@example.com", "password1", "Ada", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register first: %v", err)
	}
	second, _, _, err := store.Register(ctx, "grace@example.com", "password1", "Grace", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register second: %v", err)
	}

	// The conflicting patch also carries a name change: AC3 says the data does
	// not change, so a rejected email must not smuggle the name through.
	_, err = store.UpdateProfile(ctx, second.ID, ProfileUpdate{
		Email: ptr("ada@example.com"),
		Name:  ptr("Mallory"),
	})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("taken email: got %v, want ErrEmailExists", err)
	}

	unchanged, err := store.UserByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if unchanged.Email != "grace@example.com" {
		t.Fatalf("email = %q, want it unchanged after a rejected update", unchanged.Email)
	}
	if unchanged.Name != "Grace" {
		t.Fatalf("name = %q, want it unchanged after a rejected update", unchanged.Name)
	}
	if first.ID == "" {
		t.Fatal("first user vanished")
	}
}

// US-AD89 AC4 (permission): the update is scoped to the caller's own id. An
// unknown id is ErrUserNotFound — the 404 the AC asks for, never a 403 that
// would confirm the account exists.
func TestUpdateProfileScopesToOwnAccount(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	user, _, _, err := store.Register(ctx, "ada@example.com", "password1", "Ada", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	const foreign = "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	_, err = store.UpdateProfile(ctx, foreign, ProfileUpdate{Name: ptr("Mallory")})
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("foreign id: got %v, want ErrUserNotFound", err)
	}

	if _, err := store.UserByID(ctx, foreign); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("read foreign id: got %v, want ErrUserNotFound", err)
	}

	// An empty patch changes nothing but still returns the current identity.
	same, err := store.UpdateProfile(ctx, user.ID, ProfileUpdate{})
	if err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	if same.Name != "Ada" {
		t.Fatalf("empty patch changed the name to %q", same.Name)
	}
}

// A rejected update must leave every field alone, not just the offending one.
func TestUpdateProfileRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())
	user, _, _, err := store.Register(ctx, "ada@example.com", "password1", "Ada", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	cases := []struct {
		name  string
		patch ProfileUpdate
	}{
		{"blank name", ProfileUpdate{Name: ptr("   ")}},
		{"malformed email", ProfileUpdate{Email: ptr("not-an-email")}},
		{"non-http avatar", ProfileUpdate{AvatarURL: ptr("javascript:alert(1)")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.UpdateProfile(ctx, user.ID, tc.patch); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}

	unchanged, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if unchanged.Name != "Ada" || unchanged.Email != "ada@example.com" || unchanged.AvatarURL != "" {
		t.Fatalf("rejected patches mutated the row: %+v", unchanged)
	}
}
