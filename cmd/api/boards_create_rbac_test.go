package main

// US-AD09 AC3 — creating a board is an owner/admin action, mirroring US-AD08 AC3
// one level up.
//
// The story is explicit: "Hanya `owner` dan `admin` dapat membuat board;
// `member`/`viewer` mendapat 403", and PRD section 12 names US-AD08..AD15 as the
// stories whose permission AC is authoritative. ARCHITECTURE 6.2.6 lists the
// route's Role Min as `Member`; where the two disagree the PRD is the acceptance
// contract, and the consistent reading is the PRD's — a member who cannot create
// the project should not be able to create the board inside it either.
//
// The live probe found `member` answering 201, which is what this pins. The mux
// comes from registerBoardRoutes — the same function main.go calls — so this
// asserts the production role gate rather than a copy of it.
//
// The denied branch never reaches the handler: the gate runs first, so the
// service is built with no repository. An allowed request stops at the handler's
// own validation (an empty body is ErrInvalidInput → 400) without touching the
// repository, so a 403 here can only be the role gate.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

func TestCreateBoardRequiresAdmin(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/boards", nil)
			req.Header.Set("Authorization", "Bearer "+scenario.tokens[tc.actor+"@x.test"])
			req.Header.Set("X-Org-ID", scenario.orgA)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s creating a board: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			// owner/admin pass the gate. The service has no repository, so the
			// handler fails *after* the gate; anything but 403 proves they were
			// let through, which is the only claim made here. The full 201 path
			// is covered by the live API check and the Postgres-backed suite.
			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s creating a board: status = 403, want the gate to allow admin+", tc.actor)
			}
		})
	}
}

// TestCreateBoardGateRejectsViewerBeforeTheBodyIsRead pins the ordering the gate
// depends on: a viewer with a malformed body must get 403, not 400. If the body
// were decoded first, a denied caller could probe request validation.
func TestCreateBoardGateRejectsViewerBeforeTheBodyIsRead(t *testing.T) {
	scenario := newRBACTestAPI(t)
	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/boards", malformedJSON{})
	req.Header.Set("Authorization", "Bearer "+scenario.tokens["vera@x.test"])
	req.Header.Set("X-Org-ID", scenario.orgA)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer with a malformed body: status = %d, want 403 (the gate must run first)", recorder.Code)
	}
}
