package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentdeck/internal/auth"
)

// Sesi aktif, ganti password, dan tutup akun — US-AD90, US-AD98, US-AD05.
//
// Semua route di sini butuh identitas pemanggil yang sudah diresolusi, jadi
// handler-nya membaca orgContext. Sesi yang tidak bisa diresolusi (API key tanpa
// baris `sessions`) ditolak 401 di sini, bukan di middleware: sebagian besar
// route lain sah dipanggil dengan API key, dan menolaknya di middleware akan
// mematikan mereka juga.

// GET /api/v1/auth/sessions — US-AD90 AC2.
func (a authAPI) listSessions(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	sessions, err := a.store.Sessions(r.Context(), orgCtx.userID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionResponse{
			ID:         s.ID,
			UserAgent:  s.UserAgent,
			IP:         s.IP,
			LastSeenAt: s.LastSeenAt.UTC().Format(time.RFC3339),
			CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339),
			// AC2's "perangkat ini". Computed rather than stored: it is one
			// equality against the caller's own session, and a stored column
			// would have to be rewritten whenever anyone opens the page.
			Current: s.ID == orgCtx.sessionID,
		})
	}
	writeJSONResponse(w, http.StatusOK, out)
}

type sessionResponse struct {
	ID         string `json:"id"`
	UserAgent  string `json:"user_agent"`
	IP         string `json:"ip"`
	LastSeenAt string `json:"last_seen_at"`
	CreatedAt  string `json:"created_at"`
	Current    bool   `json:"current"`
}

// DELETE /api/v1/auth/sessions/{id} — US-AD90 AC4, US-AD05 AC2.
//
// The floor is Viewer on the route because revoking your OWN session is a Viewer
// action; the owner/admin requirement for someone else's session is enforced in
// the service, where the target's owner is known. A route-level Admin gate would
// lock users out of their own session list.
func (a authAPI) revokeSession(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	isAdmin := orgCtx.role == auth.Owner || orgCtx.role == auth.Admin
	err = a.store.RevokeSession(r.Context(), orgCtx.userID, orgCtx.email,
		orgCtx.workspace.ID, r.PathValue("id"), isAdmin)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	// 204 with no body: the resource is gone and there is nothing useful to say
	// about it. The client's own list is refetched.
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/auth/password/change — US-AD90 AC1/AC3.
func (a authAPI) changePassword(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if orgCtx.sessionID == "" {
		// AC1 says the CURRENT session survives. Without a session row there is
		// no "current session" to keep, and revoking the others would be a
		// guess. Refusing is the honest answer for an API-key caller.
		http.Error(w, "password change requires a session", http.StatusUnauthorized)
		return
	}
	var input passwordChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	err = a.store.ChangePassword(r.Context(), orgCtx.userID, orgCtx.sessionID,
		input.OldPassword, input.NewPassword)
	if err != nil {
		// AC3 is 401: the old password was wrong. Deliberately NOT the generic
		// writeAuthError default, which would make it a 400.
		if errors.Is(err, auth.ErrWrongPassword) {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type passwordChangeRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// DELETE /api/v1/auth/me — US-AD98.
//
// There is no `{id}` in the path, so AC4 ("only your own account") is structural:
// a caller has no way to name another account.
func (a authAPI) closeAccount(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var input closeAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := a.store.CloseAccount(r.Context(), orgCtx.userID, input.ConfirmEmail); err != nil {
		writeAuthError(w, err)
		return
	}
	// AC2: the account is closed, so the cookie that carried the caller here is
	// cleared. 202, not 204: the closure is accepted and reversible for 30 days
	// (AC5), so it is not yet "done" in the sense a 204 claims.
	http.SetCookie(w, auth.ExpiredSessionCookie())
	w.WriteHeader(http.StatusAccepted)
}

type closeAccountRequest struct {
	ConfirmEmail string `json:"confirm_email"`
}
