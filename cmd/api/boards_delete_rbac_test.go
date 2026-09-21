package main

// US-AD80 AC2 — deleting a task is an owner/admin action.
//
// The story is explicit: "Hanya `owner` dan `admin` yang bisa menghapus task."
// ARCHITECTURE 6.2.6 lists the route's Role Min as `Admin` as well, so the PRD
// acceptance criterion and the route table agree; only the wiring disagreed,
// answering 201 for `member`. PRD section 12 names US-AD08..AD15 as the stories
// whose permission AC is authoritative, and the same admin-for-structure rule
// already governs POST /projects and POST /boards.
//
// The mux comes from registerBoardRoutes — the same function main.go calls — so
// this asserts the production role gate rather than a copy of it. A copy would
// keep passing after main.go was loosened, which is the whole failure mode.
//
// DELETE carries no body, so unlike the create/project gates the handler walks
// straight into the service the moment it is let through. The service is built
// with no repository (`board.NewService(nil)`), so an allowed caller panics
// dereferencing it. That panic is the proof the caller was allowed: the gate
// answers 403 *instead of* reaching the handler. The test recovers it and says
// so, rather than asserting on a status an unreachable handler could never set.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// reachedHandler reports whether the request got past the role gate and into
// the handler. A nil-repository service panics on first use, so a recovered
// panic means the gate let the caller through.
func reachedHandler(mux *http.ServeMux, r *http.Request) (code int, reached bool) {
	defer func() {
		if rec := recover(); rec != nil {
			reached = true
		}
	}()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, r)
	return recorder.Code, false
}

func TestDeleteTaskRequiresAdmin(t *testing.T) {
	scenario := newRBACTestAPI(t)
	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(nil), nil)

	// alice=owner, andre=admin, marta=member, vera=viewer (rbac_test.go topology).
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
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/task-1", nil)
			req.Header.Set("Authorization", "Bearer "+scenario.tokens[tc.actor+"@x.test"])
			req.Header.Set("X-Org-ID", scenario.orgA)

			code, reached := reachedHandler(mux, req)

			if tc.wantDenied {
				if reached || code != http.StatusForbidden {
					t.Fatalf("%s deleting a task: reached handler = %v, status = %d, want 403 at the gate",
						tc.actor, reached, code)
				}
				return
			}
			if !reached {
				t.Fatalf("%s deleting a task: status = %d, want the gate to allow owner/admin through", tc.actor, code)
			}
		})
	}
}
