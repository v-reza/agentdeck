package main

// US-AD08 AC3 — creating a project is an owner/admin action.
//
// The story's acceptance criterion is explicit ("Hanya `owner` dan `admin`
// dapat membuat project; `member`/`viewer` mendapat 403") and PRD section 12
// names US-AD08..AD15 as the stories whose permission AC is authoritative.
// ARCHITECTURE 6.2.5 lists the route's Role Min as `Member`; where the two
// disagree the PRD is the acceptance contract, and US-AD10 AC4 shows the same
// admin-for-structure pattern, so the PRD reading is the consistent one.
//
// The mux comes from registerBoardRoutes — the same function main.go calls —
// so this asserts the production role gate rather than a copy of it. A copy
// would keep passing after main.go was loosened, which is the whole failure
// mode being guarded.
//
// The denied branch never reaches the handler: the gate runs first, so the
// service is built with no repository at all. If a denied request did reach the
// handler it would panic instead of answering 403, which is exactly the bug.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

func TestCreateProjectRequiresAdmin(t *testing.T) {
	scenario := newRBACTestAPI(t)
	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(nil))

	// Every actor is a member of orgA with a distinct role, so a 403 here is the
	// role gate firing and nothing else. alice=owner, andre=admin, marta=member,
	// vera=viewer (rbac_test.go topology).
	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false},
		{actor: "andre", wantDenied: false},
		{actor: "marta", wantDenied: true},
		{actor: "vera", wantDenied: true},
	}

	for _, tc := range cases {
		t.Run(tc.actor, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
			req.Header.Set("Authorization", "Bearer "+scenario.tokens[tc.actor+"@x.test"])
			req.Header.Set("X-Org-ID", scenario.orgA)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s creating a project: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			// owner/admin pass the gate. The service has no repository, so the
			// handler fails *after* the gate; anything but 403 proves they were
			// let through, which is the only claim made here. The full 201 path
			// is covered by the live API check and the e2e suite.
			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s creating a project: status = 403, want the gate to allow admin+", tc.actor)
			}
		})
	}
}

// TestCreateProjectGateRejectsViewerBeforeTheBodyIsRead pins the ordering the
// gate depends on: a viewer with a malformed body must get 403, not 400. If the
// body were decoded first, a denied caller could probe request validation.
func TestCreateProjectGateRejectsViewerBeforeTheBodyIsRead(t *testing.T) {
	scenario := newRBACTestAPI(t)
	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", malformedJSON{})
	req.Header.Set("Authorization", "Bearer "+scenario.tokens["vera@x.test"])
	req.Header.Set("X-Org-ID", scenario.orgA)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer with a malformed body: status = %d, want 403 (the gate must run first)", recorder.Code)
	}
}

// malformedJSON is a body the decoder would reject, used only to prove the gate
// answers before the decoder runs.
type malformedJSON struct{}

func (malformedJSON) Read([]byte) (int, error) { return 0, errUnreadableBody }

var errUnreadableBody = errTestBody("body must not be read for a denied caller")

type errTestBody string

func (e errTestBody) Error() string { return string(e) }
