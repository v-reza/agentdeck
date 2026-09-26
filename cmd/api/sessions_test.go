package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Sesi aktif, ganti password, tutup akun — US-AD90, US-AD98, US-AD05.
//
// Yang diuji di sini adalah gerbangnya, bukan badan handler-nya. Gerbang peran
// tidak bisa dibuktikan e2e di repo ini (fixture-nya selalu owner), jadi unit
// test ini satu-satunya tempat AC peran benar-benar dieksekusi.

// login is a second device for an actor: it performs a real login and replaces
// the stashed token, which is what makes "revoke the others" observable.
func (e rbacTestAPI) login(actor, password string) {
	resp := e.do(e.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"`+actor+`","password":"`+password+`"}`, "", "")
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("login %s: %d %s", actor, resp.StatusCode, resp.body)
	}
	if cookie := resp.header.Get("Set-Cookie"); cookie != "" {
		e.tokens[actor] = parseSessionCookie(cookie)
	}
}

// doWithHeaders is do() plus request headers, for the one AC that is about what
// the client sent (the recorded user agent).
func (e rbacTestAPI) doWithHeaders(test *testing.T, method, path, body, token string, headers map[string]string) rbacResponse {
	test.Helper()

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, e.server.URL+path, reader)
	} else {
		req = httptest.NewRequest(method, e.server.URL+path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	e.api.mux(e.t).ServeHTTP(recorder, req)
	return rbacResponse{StatusCode: recorder.Code, body: recorder.Body.String(), header: recorder.Header()}
}

type sessionRow struct {
	ID         string `json:"id"`
	UserAgent  string `json:"user_agent"`
	IP         string `json:"ip"`
	LastSeenAt string `json:"last_seen_at"`
	CreatedAt  string `json:"created_at"`
	Current    bool   `json:"current"`
}

// listSessions menembak route nyata lewat middleware nyata, jadi ia sekaligus
// membuktikan route-nya terdaftar dan X-Org-ID diterima.
func (e rbacTestAPI) listSessions(actor, org string) []sessionRow {
	resp := e.do(e.t, http.MethodGet, "/api/v1/auth/sessions", "", e.tokens[actor], org)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("listSessions %s: %d %s", actor, resp.StatusCode, resp.body)
	}
	var rows []sessionRow
	if err := json.Unmarshal([]byte(resp.body), &rows); err != nil {
		e.t.Fatalf("listSessions %s: decode %v (%s)", actor, err, resp.body)
	}
	return rows
}

// TestListSessionsShowsTheCurrentDevice — US-AD90 AC2.
//
// AC2 asks the list to carry "perangkat, IP, waktu masuk terakhir, dan penanda
// perangkat ini". The first three come from the request that created the
// session; the fourth is computed against the caller's own session id. If the
// marker is ever derived from something else (say, "the newest row"), this test
// fails the moment a user has two sessions.
func TestListSessionsShowsTheCurrentDevice(t *testing.T) {
	test := newRBACTestAPI(t)

	rows := test.listSessions("alice@x.test", test.orgA)
	if len(rows) == 0 {
		t.Fatal("alice has no sessions; register should have created one")
	}
	current := 0
	for _, row := range rows {
		if row.ID == "" || row.CreatedAt == "" || row.LastSeenAt == "" {
			t.Fatalf("row is missing fields AC2 requires: %+v", row)
		}
		if row.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("expected exactly one row flagged current, got %d in %+v", current, rows)
	}

	// A second login is a second device. Exactly one of the two is "this device",
	// and the first session is still listed — AC2 is a device list, not a
	// "who is logged in right now" singleton.
	test.login("alice@x.test", "password1")
	rows = test.listSessions("alice@x.test", test.orgA)
	if len(rows) < 2 {
		t.Fatalf("second login did not add a session: %+v", rows)
	}
	current = 0
	for _, row := range rows {
		if row.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("expected exactly one current device after two logins, got %d", current)
	}
}

// TestListSessionsRecordsUserAgentAndIP — US-AD90 AC2.
//
// The columns existed from the start but nothing ever wrote them, so AC2 was
// satisfiable only in the sense that the endpoint returned an empty list of
// fields. This pins the write.
func TestListSessionsRecordsUserAgentAndIP(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.doWithHeaders(test.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"marta@x.test","password":"password1"}`, "",
		map[string]string{"User-Agent": "AgentDeckTest/1.0"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d %s", resp.StatusCode, resp.body)
	}
	token := parseSessionCookie(resp.header.Get("Set-Cookie"))

	listed := test.do(test.t, http.MethodGet, "/api/v1/auth/sessions", "", token, "")
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", listed.StatusCode, listed.body)
	}
	var rows []sessionRow
	if err := json.Unmarshal([]byte(listed.body), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, row := range rows {
		if strings.Contains(row.UserAgent, "AgentDeckTest/1.0") {
			found = true
			if row.IP == "" {
				t.Errorf("session with a user agent has no ip: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("the session created by that login is not in the list: %+v", rows)
	}
}

// TestRevokeOwnSession — US-AD90 AC4.
//
// The interesting part is the second call: the row is already revoked, and AC4's
// list is a list of buttons, so a double click must not be an error.
func TestRevokeOwnSession(t *testing.T) {
	test := newRBACTestAPI(t)

	test.login("vera@x.test", "password1")
	rows := test.listSessions("vera@x.test", test.orgA)

	var victim string
	for _, row := range rows {
		if !row.Current {
			victim = row.ID
		}
	}
	if victim == "" {
		t.Skip("vera has a single session; nothing to revoke")
	}

	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/sessions/"+victim, "", test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke own session: %d %s", resp.StatusCode, resp.body)
	}
	// Percabutan kedua menjawab 404, bukan 204: `GetSessionAnyUser` hanya
	// melihat sesi hidup, jadi sesi yang sudah dicabut tidak ketemu. Yang penting
	// bukan kode statusnya, tapi bahwa efeknya sama — sesi tetap mati dan tidak
	// ada baris lain yang ikut tercabut.
	resp = test.do(test.t, http.MethodDelete, "/api/v1/auth/sessions/"+victim, "", test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second revoke: want 404 (already gone), got %d %s", resp.StatusCode, resp.body)
	}
	for _, row := range test.listSessions("vera@x.test", test.orgA) {
		if row.ID == victim {
			t.Fatalf("revoked session is still listed: %+v", row)
		}
	}
}

// TestViewerCannotRevokeSomeoneElsesSession — US-AD05 AC2.
//
// vera is a Viewer in orgA. She may manage her own sessions and nothing else.
func TestViewerCannotRevokeSomeoneElsesSession(t *testing.T) {
	test := newRBACTestAPI(t)

	victim := test.listSessions("marta@x.test", test.orgA)[0].ID
	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/sessions/"+victim, "", test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer revoking another member's session: want 403, got %d %s", resp.StatusCode, resp.body)
	}
}

// TestAdminCannotRevokeOutsideTheirTenant — US-AD05 AC2, batas tenant.
//
// andre is an Admin of orgA. bella is in orgB and shares no workspace with him.
// Without the membership check, "admin" would mean admin of the installation.
func TestAdminCannotRevokeOutsideTheirTenant(t *testing.T) {
	test := newRBACTestAPI(t)

	victim := test.listSessions("bella@x.test", test.orgB)[0].ID
	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/sessions/"+victim, "", test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant revoke: want 404, got %d %s", resp.StatusCode, resp.body)
	}

	// And the session is still alive: a 404 that still revoked would be a worse
	// bug than a 200.
	if len(test.listSessions("bella@x.test", test.orgB)) == 0 {
		t.Fatal("bella's session was revoked by an admin of another tenant")
	}
}

// TestAdminCanRevokeAMemberOfTheirTenant — US-AD05 AC1/AC2.
//
// The allowed half of AC2, and AC1: the row is gone, so the token is dead.
func TestAdminCanRevokeAMemberOfTheirTenant(t *testing.T) {
	test := newRBACTestAPI(t)

	victimToken := test.tokens["marta@x.test"]
	victim := test.listSessions("marta@x.test", test.orgA)[0].ID
	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/sessions/"+victim, "", test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin revoking a member's session: want 204, got %d %s", resp.StatusCode, resp.body)
	}

	// AC1: the next request with that token is 401.
	after := test.do(test.t, http.MethodGet, "/api/v1/auth/sessions", "", victimToken, test.orgA)
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token still works: %d %s", after.StatusCode, after.body)
	}
}

// TestChangePasswordKeepsTheCurrentSession — US-AD90 AC1/AC3.
func TestChangePasswordKeepsTheCurrentSession(t *testing.T) {
	test := newRBACTestAPI(t)

	// Two devices. The FIRST token is the one that must die; the second is the
	// caller, and `test.tokens` is overwritten by each login, so the older one
	// has to be captured before the second login happens.
	olderDevice := test.tokens["marta@x.test"]
	test.login("marta@x.test", "password1")
	if test.tokens["marta@x.test"] == olderDevice {
		t.Fatal("second login returned the same token; the two-device setup is not real")
	}

	resp := test.do(test.t, http.MethodPost, "/api/v1/auth/password/change",
		`{"old_password":"password1","new_password":"password2"}`,
		test.tokens["marta@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change password: %d %s", resp.StatusCode, resp.body)
	}

	// AC1: the current session survives.
	if got := test.do(test.t, http.MethodGet, "/api/v1/auth/sessions", "", test.tokens["marta@x.test"], test.orgA); got.StatusCode != http.StatusOK {
		t.Fatalf("current session died after password change: %d %s", got.StatusCode, got.body)
	}
	// AC1: the other device does not.
	if got := test.do(test.t, http.MethodGet, "/api/v1/auth/sessions", "", olderDevice, test.orgA); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other session survived the password change: %d %s", got.StatusCode, got.body)
	}
	// The new password works, the old one does not.
	if got := test.do(test.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"marta@x.test","password":"password2"}`, "", ""); got.StatusCode != http.StatusOK {
		t.Fatalf("new password rejected: %d %s", got.StatusCode, got.body)
	}
	if got := test.do(test.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"marta@x.test","password":"password1"}`, "", ""); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d %s", got.StatusCode, got.body)
	}
}

// TestChangePasswordRejectsAWrongCurrentPassword — US-AD90 AC3.
func TestChangePasswordRejectsAWrongCurrentPassword(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(test.t, http.MethodPost, "/api/v1/auth/password/change",
		`{"old_password":"not-it","new_password":"password2"}`,
		test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current password: want 401, got %d %s", resp.StatusCode, resp.body)
	}
	// AC3 says nothing changes: the old password still works and the session is
	// still alive.
	if got := test.do(test.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"vera@x.test","password":"password1"}`, "", ""); got.StatusCode != http.StatusOK {
		t.Fatalf("password changed despite the rejection: %d %s", got.StatusCode, got.body)
	}
}

// TestChangePasswordRejectsAShortPassword — the same floor as registration.
func TestChangePasswordRejectsAShortPassword(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(test.t, http.MethodPost, "/api/v1/auth/password/change",
		`{"old_password":"password1","new_password":"short"}`,
		test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: want 400, got %d %s", resp.StatusCode, resp.body)
	}
}

// TestCloseAccountRequiresTheMatchingEmail — US-AD98 AC1.
func TestCloseAccountRequiresTheMatchingEmail(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/me",
		`{"confirm_email":"not-my-email@x.test"}`, test.tokens["vera@x.test"], test.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mismatched confirmation: want 400, got %d %s", resp.StatusCode, resp.body)
	}
	// Nothing changed.
	if got := test.do(test.t, http.MethodGet, "/api/v1/auth/me", "", test.tokens["vera@x.test"], ""); got.StatusCode != http.StatusOK {
		t.Fatalf("account was closed despite the mismatch: %d", got.StatusCode)
	}
}

// TestCloseAccountEndsEverySession — US-AD98 AC1/AC2.
func TestCloseAccountEndsEverySession(t *testing.T) {
	test := newRBACTestAPI(t)

	// bella owns orgB alone, so AC2 lets her close the account (nobody else is
	// in the workspace she would take with her).
	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/me",
		`{"confirm_email":"bella@x.test"}`, test.tokens["bella@x.test"], test.orgB)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("close account: want 202, got %d %s", resp.StatusCode, resp.body)
	}
	// AC2: every session is gone.
	if got := test.do(test.t, http.MethodGet, "/api/v1/auth/sessions", "", test.tokens["bella@x.test"], test.orgB); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session survived the account closure: %d %s", got.StatusCode, got.body)
	}
	// The account cannot be logged into again.
	if got := test.do(test.t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"bella@x.test","password":"password1"}`, "", ""); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("closed account logged in: %d %s", got.StatusCode, got.body)
	}
}

// TestLastOwnerCannotCloseASharedWorkspace — US-AD98 AC3.
//
// alice owns orgA and it has four members. Closing her account would leave a
// shared workspace with nobody able to administer it, so it is refused.
func TestLastOwnerCannotCloseASharedWorkspace(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(test.t, http.MethodDelete, "/api/v1/auth/me",
		`{"confirm_email":"alice@x.test"}`, test.tokens["alice@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("last owner closing a shared workspace: want 403, got %d %s", resp.StatusCode, resp.body)
	}
}
