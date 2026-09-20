package main

import (
	"context"
	"encoding/json"
	"net/http"
)

// notifier is the seam between the auth domain and the transport. The domain
// decides *what* happens (single-use token, 30-minute window, a role someone was
// given); the notifier only carries the message, so the API can run with SMTP
// configured or with nothing but a log line.
//
// Both methods return an error the caller logs rather than returns: a member who
// was successfully added must not see a failure because a relay was down, and
// AC5's uniform response must not depend on the mailer.
type notifier interface {
	SendPasswordReset(ctx context.Context, to, link string) error
	SendInvite(ctx context.Context, to, orgName, role, signInLink string) error
}

// resetRequest is the payload of POST /api/v1/auth/password/reset-request.
type resetRequest struct {
	Email string `json:"email"`
}

// resetConfirm is the payload of POST /api/v1/auth/password/reset.
type resetConfirm struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// requestPasswordReset implements US-AD88 AC1/AC5.
//
// It always answers 202, for a registered email, an unknown one, and a
// malformed one alike. That is the whole point of AC5: any other status would
// turn this endpoint into an account-existence oracle. The mailer is only
// invoked when the domain actually issued a token.
func (a authAPI) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input resetRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// Still 202: a malformed body is the caller's problem, but the
		// response must not distinguish it from a valid unknown address.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	token, err := a.store.RequestPasswordReset(r.Context(), input.Email)
	if err != nil {
		// A real failure (database down) is a 500 — the caller retries. It
		// says nothing about whether the account exists.
		writeAuthError(w, err)
		return
	}
	if token != "" && a.mailer != nil {
		link := a.resetLink(token)
		if err := a.mailer.SendPasswordReset(r.Context(), input.Email, link); err != nil {
			// The token is already persisted, so the operator can still use
			// the link if it reaches them; surface the failure to the log
			// rather than to the caller, whose response must stay uniform.
			a.logger.Error("password reset mail failed", "error", err)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

// resetLink builds the URL the recipient clicks. It points at the web app's
// reset screen, not at the API.
func (a authAPI) resetLink(token string) string {
	return a.appBaseURL + "/reset/" + token
}

// signInLink is where an invited member goes to act on the notice. Unlike
// resetLink it carries no token: AC1 records the membership up front, so the
// invite tells the recipient what happened rather than granting them anything.
func (a authAPI) signInLink() string {
	return a.appBaseURL + "/login"
}

// resetPassword implements US-AD88 AC2/AC3.
func (a authAPI) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input resetConfirm
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if err := a.store.ResetPassword(r.Context(), input.Token, input.Password); err != nil {
		writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
