package main

// US-AD73 AC1/AC2 — the assignment section of screen 28-agent-detail.
//
// The mock draws a section that is not decoration: it previews the assign picker
// (AC2, archived agents excluded) beside the tasks an agent holds (AC1, a running
// task is never cut off by archiving). The screen used to have no such section at
// all, so the mock's claims had no implementation behind them.
//
// The tests here assert the response the screen needs, through the route
// registerAgentRoutes registers — the same function main.go calls, so the
// production wiring and the role floor are what is exercised rather than a copy.
//
//	AC1 : the agent's tasks come back, running first, with no archived row.
//	AC2 : the picker holds only active agents of that board's project, and the
//	      count of what it hid is reported rather than dropped.
//
// plus the tenant boundary every read in this package holds: an agent from
// another workspace is not found, not forbidden (US-AD07).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentdeck/internal/board"
)

// assignmentResponse is the handler's own envelope. It is declared here rather
// than imported so a field rename in cmd/api has to be reflected twice — the test
// fails loudly instead of silently decoding a zero value.
type assignmentResponse struct {
	BoardID      string `json:"board_id"`
	RunningCount int    `json:"running_count"`
	HiddenAgents int    `json:"hidden_agents"`
	Tasks        []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		CostMicros int64  `json:"cost_micros"`
		Estimate   bool   `json:"estimate"`
	} `json:"tasks"`
	Picker []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"picker"`
}

// getAssignment drives GET /agents/{id}/tasks through the real route.
func (f phase5Fixture) getAssignment(t *testing.T, actor, orgID, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// seedAgent puts one agent row in the fake repo directly. The subject under test
// is a read, so the write path (already covered elsewhere) is skipped.
func (f phase5Fixture) seedAgent(t *testing.T, id, orgID, projectID, name string, archived bool) {
	t.Helper()
	a := board.Agent{ID: id, OrgID: orgID, ProjectID: projectID, Name: name, Provider: "openai_compatible", Model: "gpt-4o"}
	if archived {
		at := time.Now()
		a.ArchivedAt = &at
	}
	if _, err := f.repo.CreateAgent(context.Background(), a); err != nil {
		t.Fatalf("seed agent %s: %v", id, err)
	}
}

func (f phase5Fixture) seedTask(t *testing.T, id, orgID, boardID, agentID, title string, status board.TaskStatus, micros int64) {
	t.Helper()
	if _, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: id, OrgID: orgID, BoardID: boardID, Title: title,
		Status: status, AssigneeAgentID: agentID, CostMicros: micros,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

// TestAgentTasksReportsRunningFirstAndHidesArchivedTasks is AC1's two halves in
// one assertion: the row the guarantee is about is the row the operator sees
// first, and a task that was archived is not resurrected by this screen.
func TestAgentTasksReportsRunningFirstAndHidesArchivedTasks(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedAgent(t, "agent-1", f.scenario.orgA, f.projectID, "agent-one", false)

	f.seedTask(t, "task-done", f.scenario.orgA, f.boardA, "agent-1", "Finished job", board.StatusDone, 420_000)
	f.seedTask(t, "task-run", f.scenario.orgA, f.boardA, "agent-1", "Live job", board.StatusRunning, 318_000)
	f.seedTask(t, "task-gone", f.scenario.orgA, f.boardA, "agent-1", "Purged job", board.StatusArchived, 999_000)

	w := f.getAssignment(t, "andre", f.scenario.orgA, "agent-1")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}
	var got assignmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v — %s", err, w.Body.String())
	}

	if len(got.Tasks) != 2 {
		t.Fatalf("want the 2 non-archived tasks, got %d: %+v", len(got.Tasks), got.Tasks)
	}
	if got.Tasks[0].ID != "task-run" {
		t.Errorf("running task must sort first, got %q", got.Tasks[0].ID)
	}
	if got.RunningCount != 1 {
		t.Errorf("running_count = %d, want 1", got.RunningCount)
	}
	// US-AD108 AC1: the cost travels as an estimate, never as a bill.
	if !got.Tasks[0].Estimate {
		t.Error("task cost must be marked as an estimate")
	}
	if got.Tasks[0].CostMicros != 318_000 {
		t.Errorf("cost_micros = %d, want the raw integer 318000", got.Tasks[0].CostMicros)
	}
	// The board the picker is resolved for is the one holding the newest task.
	if got.BoardID != f.boardA {
		t.Errorf("board_id = %q, want %q", got.BoardID, f.boardA)
	}
}

// TestAgentTasksPickerOffersOnlyActiveAgentsOfTheProject is AC2. The archived
// agent must not be an option, and the fact that it was left out has to be
// visible — a picker that silently drops rows is how AC2 would regress without
// anyone noticing.
func TestAgentTasksPickerOffersOnlyActiveAgentsOfTheProject(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedAgent(t, "agent-live", f.scenario.orgA, f.projectID, "agent-live", false)
	f.seedAgent(t, "agent-retired", f.scenario.orgA, f.projectID, "agent-retired", true)
	// An active agent of *another* project must not appear in this board's picker.
	f.seedAgent(t, "agent-other-project", f.scenario.orgA, "proj-other", "agent-elsewhere", false)
	f.seedTask(t, "task-1", f.scenario.orgA, f.boardA, "agent-live", "Job", board.StatusRunning, 0)

	w := f.getAssignment(t, "vera", f.scenario.orgA, "agent-live")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}
	var got assignmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	names := make([]string, 0, len(got.Picker))
	for _, option := range got.Picker {
		names = append(names, option.Name)
	}
	if len(names) != 1 || names[0] != "agent-live" {
		t.Errorf("picker = %v, want only the active agent of this board's project", names)
	}
	if got.HiddenAgents != 1 {
		t.Errorf("hidden_agents = %d, want 1 (the archived agent the picker left out)", got.HiddenAgents)
	}
}

// TestAgentTasksIsTenantScoped: a foreign agent id is not found, not forbidden
// (US-AD07). The route is Viewer+, so the role gate must not be what answers —
// otherwise the 403 would confirm the agent exists in another workspace.
func TestAgentTasksIsTenantScoped(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedAgent(t, "agent-b", f.scenario.orgB, "proj-b", "agent-b", false)
	f.seedTask(t, "task-b", f.scenario.orgB, f.boardB, "agent-b", "Other tenant job", board.StatusRunning, 0)

	w := f.getAssignment(t, "andre", f.scenario.orgA, "agent-b")
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read must be 404, got %d — %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body == "" || json.Valid(w.Body.Bytes()) {
		t.Errorf("404 body should be the plain not-found text, got %q", body)
	}
}

// TestAgentTasksEmptyForAnAgentWithNoWork: an agent that has never been assigned
// anything answers an empty list and no picker board — not a 404 and not a board
// picked arbitrarily. The screen renders its empty state from this.
func TestAgentTasksEmptyForAnAgentWithNoWork(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedAgent(t, "agent-idle", f.scenario.orgA, f.projectID, "agent-idle", false)

	w := f.getAssignment(t, "andre", f.scenario.orgA, "agent-idle")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}
	var got assignmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Tasks) != 0 || got.BoardID != "" || len(got.Picker) != 0 {
		t.Errorf("idle agent must answer empty, got %+v", got)
	}
}

// TestAgentTasksRejectsAnonymous: the role floor is real. Without a session the
// gate answers 401 before the handler runs.
func TestAgentTasksRejectsAnonymous(t *testing.T) {
	f := newPhase5Fixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/tasks", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read must be 401, got %d", w.Code)
	}
}
