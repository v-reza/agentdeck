package main

// US-AD20 — agent registry CRUD. All five acceptance criteria, plus the tenant
// boundary the ACs imply but do not spell out.
//
//	AC1 : creating an agent answers 201 and echoes every configured field.
//	AC2 : a viewer is refused at the registry's mutating endpoints.
//	AC3 : a duplicate name inside one project is a 409, not a 500.
//	AC4 : deleting an agent that still holds a running task is a 409, and the
//	      row is left untouched.
//	AC5 : a first agent needs no provider credential (B2C path).
//
// The mux comes from registerBoardRoutes — the same function main.go calls — so
// these assert the production role gate and wiring, not a copy of either.
//
// The repository is fakeBoardRepo, extended with the agent methods below. It
// embeds board.Repository, so a method this story does not exercise panics on a
// nil dereference rather than silently returning a zero value: a test that
// "passed" against a repository which persisted nothing would be worse than no
// test.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// agentFixture reuses the column fixture's mux and scenario and adds a project
// to register agents into. The scenario is created once: newRBACTestAPI
// registers real users and mints real sessions, so a second call would build an
// unrelated tenant whose tokens do not match the project under test.
type agentFixture struct {
	columnFixture
	projectID string
}

func newAgentFixture(t *testing.T) agentFixture {
	t.Helper()
	f := columnEditAPI(t)
	return agentFixture{columnFixture: f, projectID: "proj-a"}
}

// asActor attaches the actor's session and the org header, mirroring what the
// browser sends.
func (f agentFixture) asActor(req *http.Request, actor, orgID string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	return req
}

// postAgent registers an agent through the real route.
func (f agentFixture) postAgent(t *testing.T, actor, orgID, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = f.asActor(req, actor, orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// do issues a bodyless request through the real route.
func (f agentFixture) do(t *testing.T, method, path, actor, orgID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req = f.asActor(req, actor, orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

func decodeAgent(t *testing.T, w *httptest.ResponseRecorder) agentResponse {
	t.Helper()
	var got agentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode agent %q: %v", w.Body.String(), err)
	}
	return got
}

// TestCreateAgentAC1 pins the full field set: a registry that silently dropped
// max_runtime_seconds would let a run exceed N9, so each configured value must
// survive the round trip.
func TestCreateAgentAC1(t *testing.T) {
	f := newAgentFixture(t)
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{
		"name":"agent-backend",
		"provider":"openai",
		"model":"gpt-4o",
		"reasoning_effort":"high",
		"skills":["go","sql"],
		"tools":["shell"],
		"max_runtime_seconds":7200,
		"retry_policy":"always",
		"max_attempts":5
	}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("AC1: want 201, got %d — %s", w.Code, w.Body.String())
	}
	got := decodeAgent(t, w)
	if got.Name != "agent-backend" || got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Errorf("AC1: identity fields not echoed: %+v", got)
	}
	if got.ReasoningEffort != "high" {
		t.Errorf("AC1: reasoning_effort want high, got %q", got.ReasoningEffort)
	}
	if got.MaxRuntimeSeconds != 7200 {
		t.Errorf("AC1: max_runtime_seconds want 7200, got %d", got.MaxRuntimeSeconds)
	}
	if got.RetryPolicy != "always" || got.MaxAttempts != 5 {
		t.Errorf("AC1: retry policy not echoed: %q / %d", got.RetryPolicy, got.MaxAttempts)
	}
	if len(got.Skills) != 2 || got.Skills[0] != "go" || got.Skills[1] != "sql" {
		t.Errorf("AC1: skills want [\"go\",\"sql\"], got %v", got.Skills)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "shell" {
		t.Errorf("AC1: tools want [\"shell\"], got %v", got.Tools)
	}
	if got.ID == "" {
		t.Error("AC1: agent has no id")
	}
}

// TestCreateAgentAC2ViewerForbidden is US-AD20 AC2 on the mutating endpoint.
func TestCreateAgentAC2ViewerForbidden(t *testing.T) {
	f := newAgentFixture(t)
	w := f.postAgent(t, "vera", f.scenario.orgA, f.projectID, `{"name":"agent-x","provider":"openai","model":"gpt-4o"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("AC2: viewer want 403, got %d — %s", w.Code, w.Body.String())
	}
}

// TestCreateAgentAC2MemberAllowed is the other half: the gate must not be a wall
// that also stops the roles the route table permits.
func TestCreateAgentAC2MemberAllowed(t *testing.T) {
	f := newAgentFixture(t)
	w := f.postAgent(t, "marta", f.scenario.orgA, f.projectID, `{"name":"agent-member","provider":"openai","model":"gpt-4o"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("member registering an agent want 201, got %d — %s", w.Code, w.Body.String())
	}
}

// TestDeleteAgentAC4RequiresAdmin covers the delete gate and the running-task
// guard together, because the AC names both the conflict and the role floor.
func TestDeleteAgentAC4RequiresAdmin(t *testing.T) {
	f := newAgentFixture(t)
	created := decodeAgent(t, f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{"name":"agent-del","provider":"openai","model":"gpt-4o"}`))
	// marta is a member: registering is allowed, deleting is not.
	w := f.do(t, http.MethodDelete, "/api/v1/agents/"+created.ID, "marta", f.scenario.orgA)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member deleting an agent want 403, got %d — %s", w.Code, w.Body.String())
	}
	if _, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA); err != nil {
		t.Fatalf("agent vanished after a denied delete: %v", err)
	}
}

// TestDeleteAgentAC4RunningTask is the criterion that needs the guard: the agent
// carries the retry policy and runtime ceiling of the run it is executing, so
// removing it mid-run would strand that run.
func TestDeleteAgentAC4RunningTask(t *testing.T) {
	f := newAgentFixture(t)
	created := decodeAgent(t, f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{"name":"agent-busy","provider":"openai","model":"gpt-4o"}`))

	// Seed exactly one running task for this agent. A task in any other status
	// must NOT block the delete, so the guard is about `running` specifically.
	if _, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: "task-running", OrgID: f.scenario.orgA, BoardID: f.boardA,
		Title: "held by the agent", Status: board.StatusRunning,
		AssigneeAgentID: created.ID,
	}); err != nil {
		t.Fatalf("seed running task: %v", err)
	}

	w := f.do(t, http.MethodDelete, "/api/v1/agents/"+created.ID, "alice", f.scenario.orgA)
	if w.Code != http.StatusConflict {
		t.Fatalf("AC4: agent holding a running task want 409, got %d — %s", w.Code, w.Body.String())
	}
	// "and does not change data" — the row must still be there.
	if _, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA); err != nil {
		t.Fatalf("AC4: agent was removed despite the 409: %v", err)
	}
}

// TestDeleteAgentAllowedWhenIdle is the control for the guard above: without it,
// a repository that always reported a running task would pass AC4 and break
// every legitimate delete.
func TestDeleteAgentAllowedWhenIdle(t *testing.T) {
	f := newAgentFixture(t)
	created := decodeAgent(t, f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{"name":"agent-idle","provider":"openai","model":"gpt-4o"}`))

	// A task in a non-running status must not block the delete.
	if _, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: "task-done", OrgID: f.scenario.orgA, BoardID: f.boardA,
		Title: "finished", Status: board.StatusDone,
		AssigneeAgentID: created.ID,
	}); err != nil {
		t.Fatalf("seed done task: %v", err)
	}

	w := f.do(t, http.MethodDelete, "/api/v1/agents/"+created.ID, "alice", f.scenario.orgA)
	if w.Code != http.StatusNoContent {
		t.Fatalf("idle agent delete want 204, got %d — %s", w.Code, w.Body.String())
	}
	if _, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA); err == nil {
		t.Fatal("idle agent still present after a 204 delete")
	}
}

// TestCreateAgentAC3DuplicateName pins the 409. Postgres owns uniqueness via
// agents_project_name_key; the fake mirrors that with a per-project check so the
// service's error mapping is what is under test.
func TestCreateAgentAC3DuplicateName(t *testing.T) {
	f := newAgentFixture(t)
	body := `{"name":"agent-dup","provider":"openai","model":"gpt-4o"}`
	if w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body); w.Code != http.StatusCreated {
		t.Fatalf("AC3 setup: first create want 201, got %d — %s", w.Code, w.Body.String())
	}
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusConflict {
		t.Fatalf("AC3: duplicate name want 409, got %d — %s", w.Code, w.Body.String())
	}
}

// TestCreateAgentAC3SameNameDifferentProject proves uniqueness is per project,
// not per org: two projects may each own an "agent-backend".
func TestCreateAgentAC3SameNameDifferentProject(t *testing.T) {
	f := newAgentFixture(t)
	if _, err := f.repo.CreateProject(context.Background(), board.Project{
		ID: "proj-a2", OrgID: f.scenario.orgA, Slug: "proj-a2", Name: "Second",
	}); err != nil {
		t.Fatalf("seed second project: %v", err)
	}
	body := `{"name":"agent-shared","provider":"openai","model":"gpt-4o"}`
	if w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body); w.Code != http.StatusCreated {
		t.Fatalf("first project: want 201, got %d — %s", w.Code, w.Body.String())
	}
	if w := f.postAgent(t, "alice", f.scenario.orgA, "proj-a2", body); w.Code != http.StatusCreated {
		t.Fatalf("second project with the same name: want 201, got %d — %s", w.Code, w.Body.String())
	}
}

// TestCreateAgentAC5NoCredentialRequired is the B2C path: name, provider and
// model are the only required fields, and the omitted ones must land on the
// DDL's defaults rather than on Go zero values.
func TestCreateAgentAC5NoCredentialRequired(t *testing.T) {
	f := newAgentFixture(t)
	w := f.postAgent(t, "bella", f.scenario.orgB, "proj-b", `{"name":"agent-solo","provider":"local","model":"llama3"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("AC5: minimal agent want 201, got %d — %s", w.Code, w.Body.String())
	}
	got := decodeAgent(t, w)
	if got.MaxRuntimeSeconds != 14400 {
		t.Errorf("AC5: default max_runtime_seconds want 14400 (N9), got %d", got.MaxRuntimeSeconds)
	}
	if got.RetryPolicy != "transient_only" {
		t.Errorf("AC5: default retry_policy want transient_only, got %q", got.RetryPolicy)
	}
	if got.MaxAttempts != 3 {
		t.Errorf("AC5: default max_attempts want 3, got %d", got.MaxAttempts)
	}
	if got.ReasoningEffort != "medium" {
		t.Errorf("AC5: default reasoning_effort want medium, got %q", got.ReasoningEffort)
	}
	if len(got.Skills) != 0 || len(got.Tools) != 0 {
		t.Errorf("AC5: default json columns want empty lists, got %v / %v", got.Skills, got.Tools)
	}
}

// TestAgentTenantBoundary is the boundary the ACs imply. A caller from another
// org must not read or delete an agent by id, and the answer is 404 rather than
// 403 so the id's existence stays unconfirmed.
func TestAgentTenantBoundary(t *testing.T) {
	f := newAgentFixture(t)
	created := decodeAgent(t, f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{"name":"agent-secret","provider":"openai","model":"gpt-4o"}`))

	// bella owns orgB and is a member of nothing in orgA. She names orgB as her
	// tenant while addressing orgA's agent: the lookup must miss.
	get := f.do(t, http.MethodGet, "/api/v1/agents/"+created.ID, "bella", f.scenario.orgB)
	if get.Code != http.StatusNotFound {
		t.Fatalf("cross-org GET want 404, got %d — %s", get.Code, get.Body.String())
	}

	del := f.do(t, http.MethodDelete, "/api/v1/agents/"+created.ID, "bella", f.scenario.orgB)
	if del.Code != http.StatusNotFound && del.Code != http.StatusForbidden {
		t.Fatalf("cross-org DELETE want 404/403, got %d — %s", del.Code, del.Body.String())
	}
	if _, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA); err != nil {
		t.Fatalf("cross-org delete removed the agent: %v", err)
	}
}

// TestListAgentsIsProjectScoped: the list route must not leak another project's
// agents, and a viewer may read it (Viewer is the floor in the route table).
func TestListAgentsIsProjectScoped(t *testing.T) {
	f := newAgentFixture(t)
	if _, err := f.repo.CreateProject(context.Background(), board.Project{
		ID: "proj-other", OrgID: f.scenario.orgA, Slug: "proj-other", Name: "Other",
	}); err != nil {
		t.Fatalf("seed other project: %v", err)
	}
	f.postAgent(t, "alice", f.scenario.orgA, f.projectID, `{"name":"agent-here","provider":"openai","model":"gpt-4o"}`)
	f.postAgent(t, "alice", f.scenario.orgA, "proj-other", `{"name":"agent-elsewhere","provider":"openai","model":"gpt-4o"}`)

	w := f.do(t, http.MethodGet, "/api/v1/projects/"+f.projectID+"/agents", "vera", f.scenario.orgA)
	if w.Code != http.StatusOK {
		t.Fatalf("viewer listing agents want 200, got %d — %s", w.Code, w.Body.String())
	}
	var got []agentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list %q: %v", w.Body.String(), err)
	}
	if len(got) != 1 || got[0].Name != "agent-here" {
		t.Fatalf("list must contain exactly this project's agent, got %+v", got)
	}
}

// TestCreateAgentRejectsBadInput keeps the 400 path honest: each of these would
// otherwise reach the DDL CHECK and surface as a 500.
func TestCreateAgentRejectsBadInput(t *testing.T) {
	f := newAgentFixture(t)
	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","provider":"openai","model":"gpt-4o"}`},
		{"blank name", `{"name":"   ","provider":"openai","model":"gpt-4o"}`},
		{"empty provider", `{"name":"a","provider":"","model":"gpt-4o"}`},
		{"empty model", `{"name":"a","provider":"openai","model":""}`},
		{"unknown retry policy", `{"name":"a","provider":"openai","model":"gpt-4o","retry_policy":"sometimes"}`},
		{"runtime above N9 cap", `{"name":"a","provider":"openai","model":"gpt-4o","max_runtime_seconds":90000}`},
		{"runtime below floor", `{"name":"a","provider":"openai","model":"gpt-4o","max_runtime_seconds":0}`},
		{"attempts above cap", `{"name":"a","provider":"openai","model":"gpt-4o","max_attempts":11}`},
		{"attempts below floor", `{"name":"a","provider":"openai","model":"gpt-4o","max_attempts":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d — %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestGetAgentUnknownIs404 keeps an unknown id from becoming a 500.
func TestGetAgentUnknownIs404(t *testing.T) {
	f := newAgentFixture(t)
	w := f.do(t, http.MethodGet, "/api/v1/agents/01JZZZZZZZZZZZZZZZZZZZZZZZ", "alice", f.scenario.orgA)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown agent want 404, got %d — %s", w.Code, w.Body.String())
	}
}
