package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"agentdeck/internal/auth"
)

// US-AD89 — the HTTP contract of the profile screen. The domain rules live in
// internal/auth/profile_test.go; these tests pin the status codes and the exact
// payload the screen renders, because AC1 is a field list and AC3/AC4 are about
// which code comes back, not about what the store decided internally.

// profileResponse mirrors the body of GET/PATCH /api/v1/auth/me.
type profileResponse struct {
	ID         string              `json:"id"`
	Email      string              `json:"email"`
	Name       string              `json:"name"`
	AvatarUser auth.Avatar         `json:"avatar_user"`
	Workspaces []map[string]string `json:"workspaces"`
}

func decodeProfile(t *testing.T, body string) profileResponse {
	t.Helper()
	var parsed profileResponse
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("decode profile: %v (body: %s)", err, body)
	}
	return parsed
}

// AC1 — the four attributes the AC names come back on the session endpoint, and
// `avatar_user` carries the 26px #101014 monogram the rail draws.
func TestMeReturnsTheFourAC1Attributes(t *testing.T) {
	harness := newRBACTestAPI(t)

	resp := harness.do(t, http.MethodGet, "/api/v1/auth/me", "", harness.tokens["alice@x.test"], "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, resp.body)
	}

	profile := decodeProfile(t, resp.body)
	if profile.ID != harness.userIDs["alice@x.test"] {
		t.Fatalf("id = %q, want the session's user id", profile.ID)
	}
	if profile.Email != "alice@x.test" {
		t.Fatalf("email = %q, want alice@x.test", profile.Email)
	}
	if profile.Name != "alice" {
		t.Fatalf("name = %q, want the registered display name", profile.Name)
	}
	if profile.AvatarUser.Kind != "monogram" {
		t.Fatalf("avatar_user.kind = %q, want monogram for an account with no uploaded image", profile.AvatarUser.Kind)
	}
	if profile.AvatarUser.Initials != "A" {
		t.Fatalf("avatar_user.initials = %q, want %q", profile.AvatarUser.Initials, "A")
	}
	if profile.AvatarUser.SizePX != 26 || profile.AvatarUser.BgColor != "#101014" {
		t.Fatalf("avatar_user = %+v, want the design's 26px #101014 disc", profile.AvatarUser)
	}
}

// AC2 — renaming answers 200 with the new identity in the body. The screen
// re-renders the topbar from this response, so a 204 or an empty body would
// force a follow-up GET and break "without a full reload".
func TestPatchMeRenamesAndEchoesTheNewIdentity(t *testing.T) {
	harness := newRBACTestAPI(t)

	resp := harness.do(t, http.MethodPatch, "/api/v1/auth/me", `{"name":"Reza Zulfi"}`,
		harness.tokens["alice@x.test"], "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, resp.body)
	}

	renamed := decodeProfile(t, resp.body)
	if renamed.Name != "Reza Zulfi" {
		t.Fatalf("name = %q, want the value just written", renamed.Name)
	}
	if renamed.AvatarUser.Initials != "RZ" {
		t.Fatalf("avatar_user.initials = %q, want the monogram to follow the new name", renamed.AvatarUser.Initials)
	}

	// Durable, not merely echoed: a fresh GET agrees.
	reread := harness.do(t, http.MethodGet, "/api/v1/auth/me", "", harness.tokens["alice@x.test"], "")
	if got := decodeProfile(t, reread.body).Name; got != "Reza Zulfi" {
		t.Fatalf("reread name = %q, want the stored value", got)
	}
}

// AC3 (failure path) — an address owned by another account is a 409, and the
// name in the same patch must not land either.
func TestPatchMeRejectsTakenEmailWithoutPartialWrite(t *testing.T) {
	harness := newRBACTestAPI(t)

	resp := harness.do(t, http.MethodPatch, "/api/v1/auth/me",
		`{"email":"alice@x.test","name":"Mallory"}`, harness.tokens["bella@x.test"], "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body: %s)", resp.StatusCode, resp.body)
	}

	after := harness.do(t, http.MethodGet, "/api/v1/auth/me", "", harness.tokens["bella@x.test"], "")
	profile := decodeProfile(t, after.body)
	if profile.Email != "bella@x.test" {
		t.Fatalf("email = %q, want it unchanged after a rejected patch", profile.Email)
	}
	if profile.Name == "Mallory" {
		t.Fatal("AC3: the rejected patch smuggled the name through")
	}
}

// AC3 also covers the shape the API refuses outright: a blank name is a 400,
// never a silent no-op that leaves the caller believing the write landed.
func TestPatchMeRejectsBlankName(t *testing.T) {
	harness := newRBACTestAPI(t)

	resp := harness.do(t, http.MethodPatch, "/api/v1/auth/me", `{"name":"   "}`,
		harness.tokens["alice@x.test"], "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", resp.StatusCode, resp.body)
	}
}

// AC4 (permission) — a foreign id is 404, never 403: a 403 confirms the account
// exists. The caller's own id resolves, which stops the endpoint from being a
// blanket 404 that would pass the AC vacuously.
func TestUserProfileHidesForeignAccounts(t *testing.T) {
	harness := newRBACTestAPI(t)

	own := harness.do(t, http.MethodGet, "/api/v1/users/"+harness.userIDs["alice@x.test"], "",
		harness.tokens["alice@x.test"], "")
	if own.StatusCode != http.StatusOK {
		t.Fatalf("own id: status = %d, want 200 (body: %s)", own.StatusCode, own.body)
	}

	foreign := harness.do(t, http.MethodGet, "/api/v1/users/"+harness.userIDs["bella@x.test"], "",
		harness.tokens["alice@x.test"], "")
	if foreign.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign id: status = %d, want 404", foreign.StatusCode)
	}
	if strings.Contains(foreign.body, "bella") {
		t.Fatal("AC4: the 404 leaks the other account's identity")
	}

	// There is no route that names another account for a write: the patch
	// endpoint carries no {id} at all, so the only mutable subject is the
	// session's own user. A method the router does not register must not
	// quietly reach a handler.
	writeForeign := harness.do(t, http.MethodPatch, "/api/v1/users/"+harness.userIDs["bella@x.test"],
		`{"name":"Mallory"}`, harness.tokens["alice@x.test"], "")
	if writeForeign.StatusCode == http.StatusOK {
		t.Fatalf("AC4: PATCH /users/{id} answered 200 — a caller can name another account")
	}
}

// AC5 (B2C) — the screen is reachable and writable with no role at all in the
// active workspace. `vera` is a plain viewer in orgA, and the profile endpoints
// are deliberately outside the org-scoped middleware.
func TestProfileNeedsNoOwnerOrAdminRole(t *testing.T) {
	harness := newRBACTestAPI(t)

	viewer := harness.tokens["vera@x.test"]
	read := harness.do(t, http.MethodGet, "/api/v1/auth/me", "", viewer, "")
	if read.StatusCode != http.StatusOK {
		t.Fatalf("viewer read: status = %d, want 200 (body: %s)", read.StatusCode, read.body)
	}

	write := harness.do(t, http.MethodPatch, "/api/v1/auth/me", `{"name":"Vera Viewer"}`, viewer, "")
	if write.StatusCode != http.StatusOK {
		t.Fatalf("viewer write: status = %d, want 200 (body: %s)", write.StatusCode, write.body)
	}
	if got := decodeProfile(t, write.body).Name; got != "Vera Viewer" {
		t.Fatalf("name = %q, want the viewer's own rename to land", got)
	}
}

// An anonymous caller gets 401 on both verbs — the profile is not a public
// resource just because it carries no tenant.
func TestProfileRequiresASession(t *testing.T) {
	harness := newRBACTestAPI(t)

	read := harness.do(t, http.MethodGet, "/api/v1/auth/me", "", "", "")
	if read.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous read: status = %d, want 401", read.StatusCode)
	}

	write := harness.do(t, http.MethodPatch, "/api/v1/auth/me", `{"name":"Nobody"}`, "", "")
	if write.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous write: status = %d, want 401", write.StatusCode)
	}
}
