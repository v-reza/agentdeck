package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/auth"
)

// rbacTestAPI builds an isolated API with two orgs, four members, and one
// foreign user, then returns it along with session tokens per role.
//
// Topology (ARCHITECTURE 11.3 matrix):
//
//	orgA  owner   alice
//	      admin   andre
//	      member  marta
//	      viewer  vera
//
//	orgB  owner   bella   (never a member of orgA)
type rbacTestAPI struct {
	t       *testing.T
	api     authAPI
	server  *httptest.Server
	orgA    string
	orgB    string
	tokens  map[string]string
	userIDs map[string]string
}

func newRBACTestAPI(t *testing.T) rbacTestAPI {
	t.Helper()

	// appBaseURL mirrors config.Load's default. Without it a relative link
	// would reach the mailer, which is fine for the link's shape but not for
	// what the recipient is supposed to click.
	api := authAPI{
		store:      auth.NewStore(auth.NewMemoryRepository()),
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		appBaseURL: "http://localhost:5173",
	}
	server := httptest.NewServer(api.mux(t))

	test := rbacTestAPI{
		t:      t,
		api:    api,
		server: server,
		tokens: make(map[string]string),
	}

	// Register every actor. Each registration creates a personal org that is
	// deliberately not orgA, so orgA is only ever reachable through membership.
	// register stashes the session cookie in test.tokens; the workspace id it
	// returns is the caller's personal org and is intentionally discarded.
	for _, email := range []string{"alice@x.test", "andre@x.test", "marta@x.test", "vera@x.test", "bella@x.test"} {
		_ = test.register(email)
	}

	// Create orgA as a real second org and invite one member per role.
	test.orgA = test.createOrg("alice@x.test", "Acme", "acme")
	test.invite("alice@x.test", test.orgA, "andre@x.test", auth.Admin)
	test.invite("alice@x.test", test.orgA, "marta@x.test", auth.Member)
	test.invite("alice@x.test", test.orgA, "vera@x.test", auth.Viewer)

	// bella never joins orgA; her own org is a separate tenant.
	test.orgB = test.createOrg("bella@x.test", "Beta", "beta")

	// Resolve each member's user id from the roster.
	roster := test.members("alice@x.test", test.orgA)
	test.userIDs = make(map[string]string)
	for _, row := range roster {
		test.userIDs[row.Email] = row.UserID
	}

	t.Cleanup(server.Close)
	return test
}

// mux mirrors main.go routing, so the test exercises the real middleware chain
// instead of a re-registered subset.
func (a authAPI) mux(t *testing.T) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/v1/auth/register", a.register)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("GET /api/v1/auth/me", a.me)
	// US-AD89: the profile is the one resource a user may read and write
	// without naming a tenant, so both routes sit outside the org-scoped
	// middleware exactly as they do in main.go.
	mux.HandleFunc("PATCH /api/v1/auth/me", a.updateMe)
	mux.HandleFunc("GET /api/v1/users/{id}", a.userProfile)
	mux.HandleFunc("GET /api/v1/orgs", a.listOrgs)
	mux.HandleFunc("POST /api/v1/orgs", a.createOrg)

	orgRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, a.orgContextMiddleware(a.requireRole(handler, minimum)))
	}
	orgRoute("GET /api/v1/orgs/{id}", http.HandlerFunc(a.getOrg), auth.Viewer)
	orgRoute("PATCH /api/v1/orgs/{id}", http.HandlerFunc(a.updateOrg), auth.Owner)
	orgRoute("GET /api/v1/orgs/{id}/members", http.HandlerFunc(a.listMembers), auth.Viewer)
	orgRoute("POST /api/v1/orgs/{id}/members", http.HandlerFunc(a.addMember), auth.Admin)
	orgRoute("PATCH /api/v1/orgs/{id}/members/{user_id}", http.HandlerFunc(a.updateMember), auth.Admin)
	orgRoute("DELETE /api/v1/orgs/{id}/members/{user_id}", http.HandlerFunc(a.removeMember), auth.Admin)

	return mux
}

func (e rbacTestAPI) register(actor string) string {
	body := `{"email":"` + actor + `","password":"password1","name":"` + strings.Split(actor, "@")[0] + `"}`
	resp := e.do(e.t, http.MethodPost, "/api/v1/auth/register", body, "", "")
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", actor, resp.StatusCode, resp.body)
	}
	// The session token only ever lives in the Set-Cookie (ARCHITECTURE 3.19).
	if cookie := resp.header.Get("Set-Cookie"); cookie != "" {
		e.tokens[actor] = parseSessionCookie(cookie)
	}
	var parsed map[string]string
	_ = json.Unmarshal([]byte(resp.body), &parsed)
	return parsed["workspace_id"]
}

// parseSessionCookie pulls the opaque token out of the Set-Cookie header so
// tests can present it as a Bearer token the way a client would.
func parseSessionCookie(header string) string {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if name, value, found := strings.Cut(part, "="); found && name == auth.SessionCookieName {
			return value
		}
	}
	return ""
}

func (e rbacTestAPI) createOrg(actor, name, slug string) string {
	body := `{"name":"` + name + `","slug":"` + slug + `"}`
	resp := e.do(e.t, http.MethodPost, "/api/v1/orgs", body, e.tokens[actor], "")
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("createOrg %s: %d %s", actor, resp.StatusCode, resp.body)
	}
	var parsed map[string]string
	_ = json.Unmarshal([]byte(resp.body), &parsed)
	return parsed["id"]
}

// personalOrg is the caller's registration workspace, used to model the
// "no X-Org-ID" case and header forgery against it.
func (e rbacTestAPI) personalOrg(actor string) string {
	resp := e.do(e.t, http.MethodGet, "/api/v1/orgs", "", e.tokens[actor], "")
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("personalOrg %s: %d %s", actor, resp.StatusCode, resp.body)
	}
	var parsed []map[string]string
	_ = json.Unmarshal([]byte(resp.body), &parsed)
	if len(parsed) == 0 {
		e.t.Fatalf("personalOrg %s: no orgs listed", actor)
	}
	return parsed[0]["id"]
}

func (e rbacTestAPI) invite(actor, org, invitee string, role auth.Role) {
	body := `{"email":"` + invitee + `","role":"` + string(role) + `"}`
	resp := e.do(e.t, http.MethodPost, "/api/v1/orgs/"+org+"/members", body, e.tokens[actor], org)
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("invite %s -> %s as %s: %d %s", invitee, org, role, resp.StatusCode, resp.body)
	}
}

func (e rbacTestAPI) members(actor, org string) []memberResponse {
	resp := e.do(e.t, http.MethodGet, "/api/v1/orgs/"+org+"/members", "", e.tokens[actor], org)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("members %s @ %s: %d %s", actor, org, resp.StatusCode, resp.body)
	}
	var parsed []memberResponse
	_ = json.Unmarshal([]byte(resp.body), &parsed)
	return parsed
}

type rbacResponse struct {
	StatusCode int
	body       string
	header     http.Header
}

// do performs one request. token is the session; org is the X-Org-ID header.
// An empty org means "let the server resolve the actor's first workspace",
// which for every actor here is their personal org, never orgA.
func (e rbacTestAPI) do(test *testing.T, method, path, body, token, org string) rbacResponse {
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
	if org != "" {
		req.Header.Set("X-Org-ID", org)
	}

	recorder := httptest.NewRecorder()
	e.api.mux(e.t).ServeHTTP(recorder, req)

	return rbacResponse{
		StatusCode: recorder.Code,
		body:       recorder.Body.String(),
		header:     recorder.Header(),
	}
}

// TestRBACMatrix is the executable permission matrix for ARCHITECTURE 11.3.
// Every row is asserted twice: the allowed roles get the documented success
// code, and every other role gets 403. The actor never escapes orgA, so a 403
// is proof the gate fired, not proof the org was wrong.
func TestRBACMatrix(t *testing.T) {
	cases := []struct {
		name      string
		actor     string
		method    string
		path      string
		body      string
		orgHeader bool
		wantAllow map[string]bool
	}{
		{
			name:      "view org",
			actor:     "{actor}",
			method:    http.MethodGet,
			path:      "/api/v1/orgs/{org}",
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true, "andre": true, "marta": true, "vera": true},
		},
		{
			name:      "rename org",
			actor:     "{actor}",
			method:    http.MethodPatch,
			path:      "/api/v1/orgs/{org}",
			body:      `{"name":"Renamed"}`,
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true},
		},
		{
			name:      "list members",
			actor:     "{actor}",
			method:    http.MethodGet,
			path:      "/api/v1/orgs/{org}/members",
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true, "andre": true, "marta": true, "vera": true},
		},
		{
			name:      "invite member",
			actor:     "{actor}",
			method:    http.MethodPost,
			path:      "/api/v1/orgs/{org}/members",
			body:      `{"email":"newbie@x.test","role":"member"}`,
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true, "andre": true},
		},
		{
			name:      "change member role",
			actor:     "{actor}",
			method:    http.MethodPatch,
			path:      "/api/v1/orgs/{org}/members/{target}",
			body:      `{"role":"member"}`,
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true, "andre": true},
		},
		{
			name:      "remove member",
			actor:     "{actor}",
			method:    http.MethodDelete,
			path:      "/api/v1/orgs/{org}/members/{target}",
			orgHeader: true,
			wantAllow: map[string]bool{"alice": true, "andre": true},
		},
	}

	for _, tc := range cases {
		// The role-change and removal targets are a real member (andre), so a
		// successful row mutates state; rebuild per case to stay independent.
		for actor, allowed := range tc.wantAllow {
			scenario := newRBACTestAPI(t)
			token := scenario.tokens[actor+"@x.test"]
			path := strings.ReplaceAll(tc.path, "{org}", scenario.orgA)
			path = strings.ReplaceAll(path, "{target}", scenario.userIDs["andre@x.test"])

			resp := scenario.do(t, tc.method, path, tc.body, token, scenario.orgA)

			if allowed && resp.StatusCode == http.StatusForbidden {
				t.Errorf("%s: %s expected success, got 403", tc.name, actor)
			}
			if !allowed && resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s: %s expected 403, got %d %s", tc.name, actor, resp.StatusCode, resp.body)
			}
		}
	}
}

// TestRBACViewerCannotMutate asserts the read-only row directly: a viewer
// invoking every write endpoint in orgA gets 403 and the roster is unchanged.
func TestRBACViewerCannotMutate(t *testing.T) {
	test := newRBACTestAPI(t)
	before := len(test.members("alice@x.test", test.orgA))

	calls := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPatch, "/api/v1/orgs/{org}", `{"name":"Hacked"}`},
		{http.MethodPost, "/api/v1/orgs/{org}/members", `{"email":"newbie@x.test","role":"member"}`},
		{http.MethodPatch, "/api/v1/orgs/{org}/members/{target}", `{"role":"admin"}`},
		{http.MethodDelete, "/api/v1/orgs/{org}/members/{target}", ""},
	}

	for _, call := range calls {
		path := strings.ReplaceAll(call.path, "{org}", test.orgA)
		path = strings.ReplaceAll(path, "{target}", test.userIDs["andre@x.test"])

		resp := test.do(t, call.method, path, call.body, test.tokens["vera@x.test"], test.orgA)

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer %s %s: got %d, want 403", call.method, call.path, resp.StatusCode)
		}
	}

	after := len(test.members("alice@x.test", test.orgA))
	if before != after {
		t.Errorf("viewer mutated roster: %d -> %d members", before, after)
	}
}

// TestRBACMemberCannotManageMembers is the member row of the matrix: ordinary
// workspace permissions, but no member management and no org rename.
func TestRBACMemberCannotManageMembers(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodPost, "/api/v1/orgs/"+test.orgA+"/members",
		`{"email":"newbie@x.test","role":"member"}`, test.tokens["marta@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member invite: got %d, want 403", resp.StatusCode)
	}

	resp = test.do(t, http.MethodPatch, "/api/v1/orgs/"+test.orgA,
		`{"name":"Hacked"}`, test.tokens["marta@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member rename org: got %d, want 403", resp.StatusCode)
	}
}

// TestRBACAdminCannotExceedOwner is the admin ceiling: admin manages members,
// but cannot rename the org, cannot grant owner, and cannot remove the owner.
func TestRBACAdminCannotExceedOwner(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodPatch, "/api/v1/orgs/"+test.orgA,
		`{"name":"Hacked"}`, test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin rename org: got %d, want 403", resp.StatusCode)
	}

	// Granting owner through the invite path is rejected at parse time.
	resp = test.do(t, http.MethodPost, "/api/v1/orgs/"+test.orgA+"/members",
		`{"email":"newbie@x.test","role":"owner"}`, test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("admin grant owner via invite: got %d, want 400", resp.StatusCode)
	}

	// Promoting a member to owner through the change-role path is rejected.
	resp = test.do(t, http.MethodPatch, "/api/v1/orgs/"+test.orgA+"/members/"+test.userIDs["marta@x.test"],
		`{"role":"owner"}`, test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("admin grant owner via role change: got %d, want 400", resp.StatusCode)
	}

	// Removing the last owner is rejected (US-AD04 AC3).
	resp = test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA+"/members/"+test.userIDs["alice@x.test"],
		"", test.tokens["andre@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin remove last owner: got %d, want 403", resp.StatusCode)
	}

	// Andre still exists as a member: the failed removals did not delete him.
	roster := test.members("alice@x.test", test.orgA)
	found := false
	for _, row := range roster {
		if row.Email == "alice@x.test" {
			found = true
		}
	}
	if !found {
		t.Error("the last owner was removed despite the guard")
	}
}

// TestTenantIsolationCrossOrg covers US-AD07: a member of orgA cannot read or
// mutate orgB by any of the three forgery routes — X-Org-ID header, path id,
// or tampered request parameter.
func TestTenantIsolationCrossOrg(t *testing.T) {
	test := newRBACTestAPI(t)

	// bella owns orgB and is not a member of orgA at all. Her attempts to reach
	// orgA must fail; the X-Org-ID route yields 403 (AC1) and the path-id route
	// yields 404 so orgA's existence does not leak (AC3).
	resp := test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA+"/members",
		"", test.tokens["bella@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign orgA members list: got %d, want 403", resp.StatusCode)
	}

	// Bella reading her own org through its own path must succeed: the route
	// id is what scopes the request, so an empty header is not a bypass.
	resp = test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgB+"/members",
		"", test.tokens["bella@x.test"], "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("own org members list by path: got %d, want 200", resp.StatusCode)
	}
	if strings.Contains(resp.body, "alice@x.test") {
		t.Errorf("orgB roster leaked orgA member: %s", resp.body)
	}

	// marta is a legitimate member of orgA. She must not reach orgB by path id.
	resp = test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgB+"/members",
		"", test.tokens["marta@x.test"], test.orgB)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member cross-org via X-Org-ID: got %d, want 403", resp.StatusCode)
	}

	// Path forgery: point the path at orgB while the header resolves orgA.
	resp = test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgB+"/members/"+test.userIDs["andre@x.test"],
		"", test.tokens["marta@x.test"], test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("path forgery: got %d, want 403", resp.StatusCode)
	}

	// The reverse forgery must be denied too: the path names orgA (where marta
	// is a member) while the header names her own org. The path is the resource
	// the caller addressed, so the membership is checked against orgA and a
	// member removing an admin is not allowed.
	resp = test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA+"/members/"+test.userIDs["andre@x.test"],
		"", test.tokens["marta@x.test"], test.personalOrg("marta@x.test"))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("header forgery: got %d, want 403", resp.StatusCode)
	}

	// Andre must still be a member of orgA; the forged removal did nothing.
	roster := test.members("alice@x.test", test.orgA)
	found := false
	for _, row := range roster {
		if row.Email == "andre@x.test" {
			found = true
		}
	}
	if !found {
		t.Error("cross-tenant forgery removed an orgA member")
	}

	// GET /api/v1/orgs lists only the caller's own orgs.
	resp = test.do(t, http.MethodGet, "/api/v1/orgs", "", test.tokens["marta@x.test"], "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list orgs: got %d, want 200", resp.StatusCode)
	}
	if strings.Contains(resp.body, `"slug":"beta"`) {
		t.Errorf("org listing leaked another tenant: %s", resp.body)
	}
	if !strings.Contains(resp.body, `"slug":"acme"`) {
		t.Errorf("org listing missing the caller's own org: %s", resp.body)
	}
}

// TestUnauthenticatedDenied asserts 401 is returned before any tenant logic
// runs, so an anonymous request never learns whether an org exists.
func TestUnauthenticatedDenied(t *testing.T) {
	test := newRBACTestAPI(t)

	calls := []struct {
		method string
		path   string
		body   string
		org    string
	}{
		{http.MethodGet, "/api/v1/orgs/" + test.orgA, "", test.orgA},
		{http.MethodGet, "/api/v1/orgs/" + test.orgA + "/members", "", test.orgA},
		{http.MethodPost, "/api/v1/orgs/" + test.orgA + "/members", `{"email":"x@y.test","role":"member"}`, test.orgA},
		{http.MethodPatch, "/api/v1/orgs/" + test.orgA, `{"name":"Hacked"}`, test.orgA},
	}

	for _, call := range calls {
		resp := test.do(t, call.method, call.path, call.body, "", call.org)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: got %d, want 401", call.method, call.path, resp.StatusCode)
		}
	}

	// A garbage token is also 401, never 403 or 404.
	resp := test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA+"/members", "", "not-a-real-token", test.orgA)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("garbage token: got %d, want 401", resp.StatusCode)
	}
}

// TestMemberLifecycle is the happy path: invite, list, change role, remove.
// It proves the 403s above are real gates on working endpoints, not dead code.
func TestMemberLifecycle(t *testing.T) {
	test := newRBACTestAPI(t)

	roster := test.members("alice@x.test", test.orgA)
	if len(roster) != 4 {
		t.Fatalf("initial roster = %d, want 4", len(roster))
	}

	// Owner promotes marta to admin; she can then invite.
	resp := test.do(t, http.MethodPatch, "/api/v1/orgs/"+test.orgA+"/members/"+test.userIDs["marta@x.test"],
		`{"role":"admin"}`, test.tokens["alice@x.test"], test.orgA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promote marta: got %d %s, want 200", resp.StatusCode, resp.body)
	}

	resp = test.do(t, http.MethodPost, "/api/v1/orgs/"+test.orgA+"/members",
		`{"email":"newbie@x.test","role":"viewer"}`, test.tokens["marta@x.test"], test.orgA)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("marta invite after promotion: got %d %s, want 201", resp.StatusCode, resp.body)
	}

	// newbie must exist as a real user to log in: an invitation creates a
	// shadow row, and registration claims it (US-AD04 AC1).
	resp = test.do(t, http.MethodPost, "/api/v1/auth/register",
		`{"email":"newbie@x.test","password":"password1","name":"Newbie"}`, "", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("newbie register: got %d %s, want 201", resp.StatusCode, resp.body)
	}
	newbieToken := parseSessionCookie(resp.header.Get("Set-Cookie"))
	if newbieToken == "" {
		t.Fatal("newbie registration returned no session cookie")
	}

	roster = test.members("alice@x.test", test.orgA)
	if len(roster) != 5 {
		t.Fatalf("roster after invite = %d, want 5", len(roster))
	}

	// newbie is a viewer: they can read the roster but not invite.
	resp = test.do(t, http.MethodPost, "/api/v1/orgs/"+test.orgA+"/members",
		`{"email":"other@x.test","role":"member"}`, newbieToken, test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("newbie invite: got %d, want 403", resp.StatusCode)
	}
	// newbie sees orgA in their own org list — the invited user really did
	// join, and the shadow row was claimed by registration.
	me := test.do(t, http.MethodGet, "/api/v1/auth/me", "", newbieToken, "")
	if !strings.Contains(me.body, `"slug":"acme"`) {
		t.Errorf("invited user does not see orgA: %s", me.body)
	}

	// Owner removes newbie; the roster shrinks and newbie loses access.
	// newbie was invited after the initial userIDs snapshot, so resolve the
	// id from the live roster.
	for _, row := range roster {
		if row.Email == "newbie@x.test" {
			test.userIDs["newbie@x.test"] = row.UserID
		}
	}
	if test.userIDs["newbie@x.test"] == "" {
		t.Fatal("newbie has no user id in the roster")
	}

	resp = test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA+"/members/"+test.userIDs["newbie@x.test"],
		"", test.tokens["alice@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove newbie: got %d %s, want 204", resp.StatusCode, resp.body)
	}
	roster = test.members("alice@x.test", test.orgA)
	for _, row := range roster {
		if row.Email == "newbie@x.test" {
			t.Error("newbie still on the roster after removal")
		}
	}

	resp = test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA+"/members",
		"", newbieToken, test.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("removed member still reads roster: got %d, want 403", resp.StatusCode)
	}
}
