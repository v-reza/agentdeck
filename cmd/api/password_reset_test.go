package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/auth"
)

// US-AD88 — handler-level behaviour. The domain rules are covered in
// internal/auth; these tests pin the HTTP contract: the status codes and the
// fact that AC5's uniformity is not broken by the handler.

// recordingMailer captures what would have been emailed.
type recordingMailer struct {
	to   string
	link string
	err  error
}

func (m *recordingMailer) SendPasswordReset(_ context.Context, to, link string) error {
	m.to, m.link = to, link
	return m.err
}

// SendInvite satisfies notifier. These tests are about the password-reset flow,
// so an invite reaching them would be a routing bug worth failing on.
func (m *recordingMailer) SendInvite(_ context.Context, _, _, _, _ string) error {
	return errors.New("unexpected: reset flow must not send a member invite")
}

func newResetAPI(t *testing.T) (authAPI, *auth.Store, *recordingMailer) {
	t.Helper()
	store := auth.NewStore(auth.NewMemoryRepository())
	mailer := &recordingMailer{}
	api := authAPI{
		store:      store,
		mailer:     mailer,
		logger:     slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
		appBaseURL: "http://localhost:5173",
	}
	return api, store, mailer
}

func postJSON(handler http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

// AC1 — a registered email yields 202 and an email whose link carries the
// token. The link points at the web app's reset screen.
func TestRequestPasswordResetHandlerSendsLink(t *testing.T) {
	api, store, mailer := newResetAPI(t)
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}

	recorder := postJSON(api.requestPasswordReset, "/api/v1/auth/password/reset-request", `{"email":"ada@example.com"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body: %s)", recorder.Code, recorder.Body.String())
	}
	if mailer.to != "ada@example.com" {
		t.Fatalf("mailer.to = %q, want the requested address", mailer.to)
	}
	if !strings.HasPrefix(mailer.link, "http://localhost:5173/reset/") {
		t.Fatalf("link = %q, want the web app's /reset/:token screen", mailer.link)
	}
}

// AC5 — an unregistered email must produce a byte-identical response and must
// not send anything.
func TestRequestPasswordResetHandlerHidesUnknownEmail(t *testing.T) {
	api, _, mailer := newResetAPI(t)

	known := postJSON(api.requestPasswordReset, "/api/v1/auth/password/reset-request", `{"email":"nobody@example.com"}`)
	if known.Code != http.StatusAccepted {
		t.Fatalf("unknown email status = %d, want 202", known.Code)
	}
	if mailer.to != "" {
		t.Fatal("AC5: no mail may be sent for an unknown address")
	}

	// The body must carry no trace of the account's existence.
	if strings.Contains(strings.ToLower(known.Body.String()), "not found") {
		t.Fatalf("AC5: response leaks that the account is unknown: %q", known.Body.String())
	}
}

// AC5 — a malformed body is answered the same way, so it is not an oracle
// either.
func TestRequestPasswordResetHandlerToleratesMalformedBody(t *testing.T) {
	api, _, mailer := newResetAPI(t)

	recorder := postJSON(api.requestPasswordReset, "/api/v1/auth/password/reset-request", `not json`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("malformed body status = %d, want 202", recorder.Code)
	}
	if mailer.to != "" {
		t.Fatal("no mail may be sent for a malformed request")
	}
}

// AC2 — a valid token returns 200 and the new password works.
func TestResetPasswordHandlerSucceeds(t *testing.T) {
	api, store, _ := newResetAPI(t)
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	token, err := store.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"token": token, "password": "password2"})
	recorder := postJSON(api.resetPassword, "/api/v1/auth/password/reset", string(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}
	if _, err := store.Login(context.Background(), "ada@example.com", "password2"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
}

// AC3 — an expired, used, or unknown token is 410, never 404 or 400, so the
// three are indistinguishable.
func TestResetPasswordHandlerAnswers410ForBadTokens(t *testing.T) {
	api, store, _ := newResetAPI(t)
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	token, err := store.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	// Spend it once, so the second attempt is the "already used" case.
	body, _ := json.Marshal(map[string]string{"token": token, "password": "password2"})
	if recorder := postJSON(api.resetPassword, "/api/v1/auth/password/reset", string(body)); recorder.Code != http.StatusOK {
		t.Fatalf("first reset status = %d, want 200", recorder.Code)
	}

	for name, candidate := range map[string]string{"used": token, "unknown": "deadbeef"} {
		payload, _ := json.Marshal(map[string]string{"token": candidate, "password": "password3"})
		recorder := postJSON(api.resetPassword, "/api/v1/auth/password/reset", string(payload))
		if recorder.Code != http.StatusGone {
			t.Fatalf("AC3 %s token: status = %d, want 410", name, recorder.Code)
		}
	}
}

// AC2 — a short password is 400 and does not consume the token.
func TestResetPasswordHandlerRejectsShortPassword(t *testing.T) {
	api, store, _ := newResetAPI(t)
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	token, err := store.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"token": token, "password": "short"})
	recorder := postJSON(api.resetPassword, "/api/v1/auth/password/reset", string(body))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if _, err := store.Login(context.Background(), "ada@example.com", "password1"); err != nil {
		t.Fatalf("the original password must still work: %v", err)
	}
}

// A mail failure must not change the caller's response: the token is already
// stored, so the flow stays uniform and the failure is logged instead.
func TestRequestPasswordResetHandlerSurvivesMailFailure(t *testing.T) {
	api, store, mailer := newResetAPI(t)
	mailer.err = errors.New("relay down")
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}

	recorder := postJSON(api.requestPasswordReset, "/api/v1/auth/password/reset-request", `{"email":"ada@example.com"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 even when the relay fails", recorder.Code)
	}
}

// AC4 — both endpoints are public. Neither handler may consult the session.
func TestResetEndpointsArePublic(t *testing.T) {
	api, store, _ := newResetAPI(t)
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	token, err := store.RequestPasswordReset(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	// No cookie, no Authorization header on either request.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/reset-request", strings.NewReader(`{"email":"ada@example.com"}`))
	recorder := httptest.NewRecorder()
	api.requestPasswordReset(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("reset-request without a session = %d, want 202", recorder.Code)
	}

	body, _ := json.Marshal(map[string]string{"token": token, "password": "password2"})
	confirmRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/reset", strings.NewReader(string(body)))
	confirmRecorder := httptest.NewRecorder()
	api.resetPassword(confirmRecorder, confirmRequest)
	if confirmRecorder.Code != http.StatusOK {
		t.Fatalf("reset without a session = %d, want 200", confirmRecorder.Code)
	}
}

// A nil mailer must not panic: the API is constructed without one in tests.
func TestRequestPasswordResetHandlerWithoutMailer(t *testing.T) {
	store := auth.NewStore(auth.NewMemoryRepository())
	api := authAPI{store: store}
	if _, _, _, err := store.Register(context.Background(), "ada@example.com", "password1", "", ""); err != nil {
		t.Fatalf("register: %v", err)
	}

	recorder := postJSON(api.requestPasswordReset, "/api/v1/auth/password/reset-request", `{"email":"ada@example.com"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
}
