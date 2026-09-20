package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"agentdeck/internal/auth"
)

// profilePatch is the PATCH /api/v1/auth/me body (ARCHITECTURE 6.2.2:
// `{name, email, avatar}`). Every field is a pointer so the handler can tell an
// absent field ("leave it alone") from an explicitly empty one ("reject it"),
// which is what makes `{"name": ""}` a 400 instead of a silent no-op.
type profilePatch struct {
	Name   *string `json:"name"`
	Email  *string `json:"email"`
	Avatar *string `json:"avatar"`
}

// profileBody is the shared response shape of GET and PATCH /auth/me. Keeping
// one shape means the screen re-renders from the PATCH answer alone instead of
// issuing a follow-up GET, which is what AC2's "without a full reload" asks for.
//
// `avatar_user` is the AC1 field name, verbatim. It carries the monogram the
// shell draws (initials, colour, diameter) or, once an avatar is uploaded, the
// image URL — so the screen never has to compute a fallback that could disagree
// with the API.
type profileBody struct {
	ID         string              `json:"id"`
	Email      string              `json:"email"`
	Name       string              `json:"name"`
	AvatarUser auth.Avatar         `json:"avatar_user"`
	Workspaces []map[string]string `json:"workspaces"`
}

func (a authAPI) me(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(a.store, r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	a.writeProfile(w, r, user)
}

// updateMe implements PATCH /api/v1/auth/me (US-AD89 AC2/AC3/AC4).
//
// The subject is always the session's user. There is no `{id}` in the path, so
// AC4 ("only your own profile") is structural: a caller has no way to name
// another account. A taken address is a 409 and the write is abandoned in the
// same statement that would have applied it (AC3).
func (a authAPI) updateMe(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(a.store, r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var input profilePatch
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateProfile(r.Context(), user.ID, auth.ProfileUpdate{
		Name:      input.Name,
		Email:     input.Email,
		AvatarURL: input.Avatar,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}
	a.writeProfile(w, r, updated)
}

// userProfile implements GET /api/v1/users/{id} (US-AD89 AC4).
//
// An id that is not the caller's own answers 404 — never 403. A 403 would
// confirm that the account exists, which is exactly the leak the AC forbids, so
// a foreign id and a made-up id are deliberately indistinguishable. The caller's
// own id resolves normally, which keeps the endpoint from being a blanket 404
// that would pass the AC vacuously.
func (a authAPI) userProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(a.store, r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	requested := strings.TrimSpace(r.PathValue("id"))
	if requested == "" || requested != user.ID {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	target, err := a.store.UserByID(r.Context(), requested)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		writeAuthError(w, err)
		return
	}
	a.writeProfile(w, r, target)
}

// writeProfile renders the shared profile body. A membership lookup that fails
// still returns the identity: the topbar needs the name and avatar to render at
// all, and dropping them because the roster query stumbled would turn a partial
// outage into a blank shell.
func (a authAPI) writeProfile(w http.ResponseWriter, r *http.Request, user auth.User) {
	workspaces := make([]map[string]string, 0, 4)
	if memberships, err := a.store.Workspaces(r.Context(), user.Email); err == nil {
		for _, membership := range memberships {
			workspaces = append(workspaces, map[string]string{
				"id":   membership.WorkspaceID,
				"name": membership.Name,
				"slug": membership.Slug,
				"role": string(membership.Role),
				"kind": membership.Kind,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profileBody{
		ID:         user.ID,
		Email:      user.Email,
		Name:       user.Name,
		AvatarUser: auth.AvatarFor(user.Name, user.Email, user.AvatarURL),
		Workspaces: workspaces,
	})
}
