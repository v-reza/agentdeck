package main

// POST /tasks/{id}/cancel, /retry, /archive — the last three routes of
// ARCHITECTURE 6.2.9.
//
// The point of these tests is the gate, not the handler body. A handler that
// forwards to the service is hard to get wrong; the role floor and the
// tenant scoping are what silently fail, because a route registered at Member
// answers 200 for a member and nothing anywhere reports that it should not have.
//
// The mux comes from registerBoardRoutes — the same function main.go calls — so
// the gates exercised here are the production gates.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// postTask drives one of the three lifecycle routes through the real mux.
func (f columnFixture) postTask(t *testing.T, actor, orgID, taskID, action string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/"+action, nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

func decodeTask(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}

// TestArchiveEndpointRequiresAdmin is US-AD59 AC4 on the endpoint the contract
// names. The `/move` route carries the same floor, but that is the board's
// drag-and-drop path; this is the one a client calls to retire a task, and a
// member answering 200 here would be the criterion failing while everything
// looked green.
func TestArchiveEndpointRequiresAdmin(t *testing.T) {
	cases := []struct {
		actor string
		role  string
		want  int
	}{
		{"vera", "viewer", http.StatusForbidden},
		{"marta", "member", http.StatusForbidden},
		{"andre", "admin", http.StatusOK},
		{"alice", "owner", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			f := columnEditAPI(t)
			// One task per actor: a shared row would make the later actors read
			// the idempotent path instead of the gate.
			id := "arch-" + tc.role
			f.seedTask(t, id, f.scenario.orgA, board.StatusDone)
			rec := f.postTask(t, tc.actor, f.scenario.orgA, id, "archive")
			if rec.Code != tc.want {
				t.Errorf("%s (%s): status = %d, want %d (body %q)",
					tc.actor, tc.role, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestArchiveEndpointRefusesANonTerminalTask is AC2 through the endpoint: the
// board would lose the row while a run still holds it.
func TestArchiveEndpointRefusesANonTerminalTask(t *testing.T) {
	f := columnEditAPI(t)
	for _, status := range []board.TaskStatus{
		board.StatusBacklog, board.StatusReady, board.StatusRunning, board.StatusReview,
	} {
		id := "arch-nonterm-" + string(status)
		f.seedTask(t, id, f.scenario.orgA, status)
		if rec := f.postTask(t, "andre", f.scenario.orgA, id, "archive"); rec.Code != http.StatusConflict {
			t.Errorf("archive from %s: status = %d, want 409 (body %q)", status, rec.Code, rec.Body.String())
		}
	}
}

// TestArchiveEndpointIsIdempotent is AC3 on the second call.
func TestArchiveEndpointIsIdempotent(t *testing.T) {
	f := columnEditAPI(t)
	id := "arch-twice"
	f.seedTask(t, id, f.scenario.orgA, board.StatusDone)

	first := f.postTask(t, "andre", f.scenario.orgA, id, "archive")
	if first.Code != http.StatusOK {
		t.Fatalf("first archive: status = %d (body %q)", first.Code, first.Body.String())
	}
	second := f.postTask(t, "andre", f.scenario.orgA, id, "archive")
	if second.Code != http.StatusOK {
		t.Errorf("re-archive: status = %d, want 200 (body %q)", second.Code, second.Body.String())
	}
	if got := decodeTask(t, second)["status"]; got != "archived" {
		t.Errorf("status = %v, want archived", got)
	}
}

// TestArchiveEndpointHidesTheTaskFromTheBoard is AC1: the row leaves the default
// listing. Asserted through the HTTP list route rather than the repo, because
// "tidak muncul di board default" is a claim about what a client sees.
func TestArchiveEndpointHidesTheTaskFromTheBoard(t *testing.T) {
	f := columnEditAPI(t)
	id := "arch-hidden"
	f.seedTask(t, id, f.scenario.orgA, board.StatusDone)

	before := f.listTasks(t, "vera", f.scenario.orgA, "")
	if !containsTaskID(t, before, id) {
		t.Fatalf("task %s missing from the board before archiving", id)
	}
	if rec := f.postTask(t, "andre", f.scenario.orgA, id, "archive"); rec.Code != http.StatusOK {
		t.Fatalf("archive: status = %d (body %q)", rec.Code, rec.Body.String())
	}
	after := f.listTasks(t, "vera", f.scenario.orgA, "")
	if containsTaskID(t, after, id) {
		t.Errorf("archived task %s is still on the board", id)
	}
}

// TestCancelEndpointIsOpenToMembers: cancelling is a Member action per the
// contract's Role Min column, and a viewer must not be able to stop work.
func TestCancelEndpointIsOpenToMembers(t *testing.T) {
	cases := []struct {
		actor string
		role  string
		want  int
	}{
		{"vera", "viewer", http.StatusForbidden},
		{"marta", "member", http.StatusOK},
		{"andre", "admin", http.StatusOK},
		{"alice", "owner", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			f := columnEditAPI(t)
			id := "cancel-" + tc.role
			f.seedTask(t, id, f.scenario.orgA, board.StatusReady)
			rec := f.postTask(t, tc.actor, f.scenario.orgA, id, "cancel")
			if rec.Code != tc.want {
				t.Errorf("%s (%s): status = %d, want %d (body %q)",
					tc.actor, tc.role, rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusOK {
				if got := decodeTask(t, rec)["status"]; got != "cancelled" {
					t.Errorf("status = %v, want cancelled", got)
				}
			}
		})
	}
}

// TestRetryEndpointResetsAFailedTask is the endpoint's contract end to end.
func TestRetryEndpointResetsAFailedTask(t *testing.T) {
	f := columnEditAPI(t)
	id := "retry-me"
	f.seedTask(t, id, f.scenario.orgA, board.StatusFailed)

	rec := f.postTask(t, "marta", f.scenario.orgA, id, "retry")
	if rec.Code != http.StatusOK {
		t.Fatalf("retry: status = %d (body %q)", rec.Code, rec.Body.String())
	}
	body := decodeTask(t, rec)
	if body["status"] != "ready" {
		t.Errorf("status = %v, want ready", body["status"])
	}
	if got := body["consecutive_failures"]; got != float64(0) {
		t.Errorf("consecutive_failures = %v, want 0", got)
	}
}

// TestRetryEndpointRefusesARunningTask: the guard that keeps one task from
// having two live runs.
func TestRetryEndpointRefusesARunningTask(t *testing.T) {
	f := columnEditAPI(t)
	id := "retry-running"
	f.seedTask(t, id, f.scenario.orgA, board.StatusRunning)

	if rec := f.postTask(t, "marta", f.scenario.orgA, id, "retry"); rec.Code != http.StatusConflict {
		t.Errorf("retry a running task: status = %d, want 409 (body %q)", rec.Code, rec.Body.String())
	}
}

// TestLifecycleEndpointsAreTenantScoped. The ids are the only thing separating
// two orgs, and an unscoped write would let one tenant stop or restart another's
// work. `other-org` is a real org in the fixture with the same actor in it.
func TestLifecycleEndpointsAreTenantScoped(t *testing.T) {
	f := columnEditAPI(t)
	id := "scope-me"
	f.seedTask(t, id, f.scenario.orgA, board.StatusFailed)

	for _, action := range []string{"cancel", "retry", "archive"} {
		// bella is an admin in orgB, so she clears both the membership check and
		// the role gate: the only thing left to refuse her is the tenant scope.
		// (alice is not a member of orgB at all, and the middleware would answer
		// 403 before the handler ever ran — that would test the wrong gate.)
		rec := f.postTask(t, "bella", f.scenario.orgB, id, action)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s across orgs: status = %d, want 404 (body %q)", action, rec.Code, rec.Body.String())
		}
	}
	task, err := f.repo.GetTask(context.Background(), id, f.scenario.orgA)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != board.StatusFailed {
		t.Errorf("cross-org request changed the task to %q", task.Status)
	}
}

// TestCancelEndpointOnAnUnknownTaskIsNotFound pins the 404 mapping. Before the
// GetTask fix this was a 500, which is both the wrong status and an existence
// oracle: "no such task" and "not your task" answered differently.
func TestCancelEndpointOnAnUnknownTaskIsNotFound(t *testing.T) {
	f := columnEditAPI(t)
	for _, action := range []string{"cancel", "retry", "archive"} {
		rec := f.postTask(t, "alice", f.scenario.orgA, "task-does-not-exist", action)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s on an unknown task: status = %d, want 404 (body %q)", action, rec.Code, rec.Body.String())
		}
	}
}

// containsTaskID reports whether a task list response carries the id. The list
// route encodes a bare array (see boardAPI.listTasks), not an object with a
// `tasks` key — decoding the wrong shape here would make every assertion pass
// for the wrong reason, so a decode failure is fatal rather than false.
func containsTaskID(t *testing.T, rec *httptest.ResponseRecorder, id string) bool {
	t.Helper()
	var body []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode task list %q: %v", rec.Body.String(), err)
	}
	for _, task := range body {
		if task.ID == id {
			return true
		}
	}
	return false
}
