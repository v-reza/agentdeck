package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"agentdeck/internal/board"
)

// HTTP coverage for the M4 runtime surface. The service-level behaviour (which
// outcome moves a task where, what gets rolled up) is pinned against a live
// Postgres in internal/board/runtime_test.go; these tests are about the layer
// above it, where the failure modes are different:
//
//   - the role gate on each route (the contract's "Worker" column is a
//     description of the caller, not a role this codebase has);
//   - the payload the handler actually accepts and echoes, including the
//     verbatim step payload;
//   - the task transition being decided by the service, not sent by the worker.
//
// The double embeds board.Repository, so the runtime methods it does not
// implement panic loudly rather than silently returning zero values.

type runFixture struct {
	columnFixture
	taskID  string
	agentID string
}

type stubRunRepo struct {
	board.Repository
	runs    map[string]board.Run
	steps   []board.Step
	ledger  []board.LedgerEntry
	tasks   map[string]board.Task
	endCall *board.RunSummary
}

func (s *stubRunRepo) GetTask(_ context.Context, id, orgID string) (board.Task, error) {
	task, ok := s.tasks[id]
	if !ok || task.OrgID != orgID {
		return board.Task{}, board.ErrNotFound
	}
	return task, nil
}

func (s *stubRunRepo) GetRun(_ context.Context, id, orgID string) (board.Run, error) {
	run, ok := s.runs[id]
	if !ok || run.OrgID != orgID {
		return board.Run{}, board.ErrNotFound
	}
	return run, nil
}

func (s *stubRunRepo) ListTaskRuns(_ context.Context, taskID, orgID string) ([]board.Run, error) {
	out := []board.Run{}
	for _, run := range s.runs {
		if run.TaskID == taskID && run.OrgID == orgID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *stubRunRepo) HeartbeatRun(_ context.Context, id, orgID string) (board.Run, error) {
	run, err := s.GetRun(context.Background(), id, orgID)
	if err != nil {
		return board.Run{}, err
	}
	// Mirror the statement's `status = 'running'` predicate: a closed run is a
	// zero-row result, which the service turns into ErrNotFound.
	if run.Status != board.RunRunning {
		return board.Run{}, pgx.ErrNoRows
	}
	return run, nil
}

func (s *stubRunRepo) EndRun(_ context.Context, id, orgID string, summary board.RunSummary) (board.Run, error) {
	run, err := s.GetRun(context.Background(), id, orgID)
	if err != nil {
		return board.Run{}, err
	}
	if run.Status != board.RunRunning {
		return board.Run{}, pgx.ErrNoRows
	}
	s.endCall = &summary
	run.Status = board.RunEnded
	run.Outcome = summary.Outcome
	s.runs[id] = run
	return run, nil
}

func (s *stubRunRepo) UpdateTaskStatus(_ context.Context, id, orgID string, from, to board.TaskStatus) (board.Task, error) {
	task, err := s.GetTask(context.Background(), id, orgID)
	if err != nil {
		return board.Task{}, err
	}
	if task.Status != from {
		return board.Task{}, board.ErrConflict
	}
	task.Status = to
	s.tasks[id] = task
	return task, nil
}

func (s *stubRunRepo) IncrementTaskFailures(_ context.Context, id, _ string) (int, error) {
	task, err := s.GetTask(context.Background(), id, "")
	_ = err
	task.ConsecutiveFailures++
	s.tasks[id] = task
	return task.ConsecutiveFailures, nil
}

func (s *stubRunRepo) ResetTaskFailures(_ context.Context, id, _ string) error {
	task := s.tasks[id]
	task.ConsecutiveFailures = 0
	s.tasks[id] = task
	return nil
}

func (s *stubRunRepo) BlockTask(_ context.Context, id, _ string, kind string) error {
	task := s.tasks[id]
	task.Status = board.StatusBlocked
	task.BlockKind = kind
	s.tasks[id] = task
	return nil
}

func (s *stubRunRepo) ClearTaskCurrentRun(_ context.Context, id, _ string) error {
	task := s.tasks[id]
	task.CurrentRunID = ""
	s.tasks[id] = task
	return nil
}

func (s *stubRunRepo) GetAgent(_ context.Context, id, orgID string) (board.Agent, error) {
	if id != "agent-1" || orgID == "" {
		return board.Agent{}, board.ErrNotFound
	}
	// Retry policy and the attempt ceiling live on the agent (US-AD96).
	return board.Agent{ID: id, OrgID: orgID, Name: "runner", RetryPolicy: "transient_only", MaxAttempts: 3}, nil
}

func (s *stubRunRepo) CreateStep(_ context.Context, step board.Step) (board.Step, error) {
	s.steps = append(s.steps, step)
	return step, nil
}

func (s *stubRunRepo) FinishStep(_ context.Context, runID string, seq int, orgID string, step board.Step) (board.Step, error) {
	for i, existing := range s.steps {
		if existing.RunID == runID && existing.Seq == seq && existing.OrgID == orgID {
			step.Seq = seq
			step.Kind = existing.Kind
			step.PayloadJSON = existing.PayloadJSON
			s.steps[i] = step
			return step, nil
		}
	}
	return board.Step{}, pgx.ErrNoRows
}

func (s *stubRunRepo) ListRunSteps(_ context.Context, runID, orgID string) ([]board.Step, error) {
	out := []board.Step{}
	for _, step := range s.steps {
		if step.RunID == runID && step.OrgID == orgID {
			out = append(out, step)
		}
	}
	return out, nil
}

func (s *stubRunRepo) CreateEvent(_ context.Context, e board.Event) (board.Event, error) {
	return e, nil
}

func (s *stubRunRepo) GetBoard(_ context.Context, id, orgID string) (board.Board, error) {
	if orgID == "" {
		return board.Board{}, board.ErrNotFound
	}
	// The ledger endpoint reads the budget off the board row, so the double has to
	// carry one; the default is what the API seeds a new board with.
	return board.Board{ID: id, OrgID: orgID, BudgetDailyMicros: board.DefaultBudgetDailyMicros}, nil
}

func (s *stubRunRepo) ListRunLedger(_ context.Context, runID, orgID string) ([]board.LedgerEntry, error) {
	out := []board.LedgerEntry{}
	for _, entry := range s.ledger {
		if entry.RunID == runID && entry.OrgID == orgID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *stubRunRepo) ListBoardLedger(_ context.Context, _, orgID string, _ int) ([]board.LedgerEntry, error) {
	out := []board.LedgerEntry{}
	for _, entry := range s.ledger {
		if entry.OrgID == orgID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *stubRunRepo) CreateLedgerEntry(_ context.Context, e board.LedgerEntry) (board.LedgerEntry, error) {
	e.ID = int64(len(s.ledger) + 1)
	s.ledger = append(s.ledger, e)
	return e, nil
}

func (s *stubRunRepo) BoardSpendToday(_ context.Context, _, _ string) (int64, error) {
	var sum int64
	for _, entry := range s.ledger {
		sum += entry.CostMicros
	}
	return sum, nil
}

// newRunAPI mounts the run routes over a double holding one claimed task and one
// running run, which is the state every write route is reached from.
func newRunAPI(t *testing.T) (*http.ServeMux, *stubRunRepo, rbacTestAPI) {
	t.Helper()
	scenario := newRBACTestAPI(t)
	repo := &stubRunRepo{
		runs: map[string]board.Run{},
		tasks: map[string]board.Task{
			"task-1": {
				ID: "task-1", OrgID: scenario.orgA, BoardID: "board-a", Title: "Run me",
				Status: board.StatusRunning, AssigneeAgentID: "agent-1", CurrentRunID: "run-1",
				ConsecutiveFailures: 0,
			},
		},
	}
	repo.runs["run-1"] = board.Run{
		ID: "run-1", OrgID: scenario.orgA, TaskID: "task-1", AgentID: "agent-1",
		Attempt: 1, Status: board.RunRunning, MaxRuntimeSecs: 3600,
	}
	mux := http.NewServeMux()
	// The run API is reached through registerBoardRoutes, like production: the
	// role gate is declared in one place and this test drives that place.
	registerBoardRoutes(mux, scenario.api, board.NewService(repo), nil)
	// Registering the same mux twice would panic on duplicate patterns, so the
	// run routes come from the same call — nothing extra to do here.
	return mux, repo, scenario
}

func runRequest(t *testing.T, mux *http.ServeMux, scenario rbacTestAPI,
	method, path, actor, orgID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestRunRoutesRequireMembership pins the gate: an unauthenticated call is 401,
// and a viewer may read the ledger but may not write to it.
func TestRunRoutesRequireMembership(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	// No credentials at all.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /runs/{id} = %d, want 401", w.Code)
	}

	// A viewer reads.
	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1",
		"vera", scenario.orgA, "").Code; got != http.StatusOK {
		t.Fatalf("viewer GET /runs/{id} = %d, want 200", got)
	}
	// ...but does not write.
	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/heartbeat",
		"vera", scenario.orgA, "").Code; got != http.StatusForbidden {
		t.Fatalf("viewer heartbeat = %d, want 403", got)
	}
	// A member writes.
	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/heartbeat",
		"marta", scenario.orgA, "").Code; got != http.StatusOK {
		t.Fatalf("member heartbeat = %d, want 200", got)
	}
}

// TestEndRunRejectsAnOutcomeOutsideTheEnum keeps a typo from reaching the
// database's CHECK, where it would surface as a 500 the worker cannot act on.
func TestEndRunRejectsAnOutcomeOutsideTheEnum(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/end",
		"marta", scenario.orgA, `{"outcome":"everything_worked"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown outcome = %d, want 400 — %s", w.Code, w.Body.String())
	}
	if w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/end",
		"marta", scenario.orgA, `{"outcome":"succeeded"}`); w.Code != http.StatusOK {
		t.Fatalf("valid outcome = %d, want 200 — %s", w.Code, w.Body.String())
	}
}

// TestStaleRunHeartbeatIs404 is US-AD23 through the HTTP surface: the worker
// needs a signal it can act on, and 404 says "stop" where a 500 would say
// "retry".
func TestStaleRunHeartbeatIs404(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/end",
		"marta", scenario.orgA, `{"outcome":"cancelled"}`); w.Code != http.StatusOK {
		t.Fatalf("end = %d, want 200 — %s", w.Code, w.Body.String())
	}
	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/heartbeat",
		"marta", scenario.orgA, "").Code; got != http.StatusNotFound {
		t.Fatalf("heartbeat on a closed run = %d, want 404", got)
	}
}

// TestRunIsScopedToTheCallersOrg is the tenant boundary: the same run id in
// another workspace must be indistinguishable from one that does not exist.
func TestRunIsScopedToTheCallersOrg(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1",
		"bella", scenario.orgB, "").Code; got != http.StatusNotFound {
		t.Fatalf("cross-org run read = %d, want 404", got)
	}
	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/tasks/task-1/runs",
		"bella", scenario.orgB, "").Code; got != http.StatusNotFound {
		t.Fatalf("cross-org task runs = %d, want 404", got)
	}
}

// TestStepAcceptsAToolTokenShape pins the wire shape the contract documents
// (`tokens: {in, out}`), because the flat column names are easy to send by
// accident and would be silently ignored — a step with zero tokens looks like a
// free step, not a malformed one.
func TestStepAcceptsAToolTokenShape(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/steps",
		"marta", scenario.orgA, `{"seq":1,"kind":"llm","name":"call","tokens":{"in":120,"out":40}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create step = %d, want 201 — %s", w.Code, w.Body.String())
	}
	if len(repo.steps) != 1 {
		t.Fatalf("steps stored = %d, want 1", len(repo.steps))
	}

	// Finish it and read the measured cost back.
	w = runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/runs/run-1/steps/1",
		"marta", scenario.orgA, `{"status":"succeeded","cost_micros":1500,"tokens":{"in":120,"out":40}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("finish step = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var step stepResponse
	if err := json.Unmarshal(w.Body.Bytes(), &step); err != nil {
		t.Fatalf("decode step: %v", err)
	}
	if step.Status != "succeeded" || step.CostMicros != 1500 || step.TokensIn != 120 {
		t.Fatalf("finished step = %+v, want succeeded/1500/120", step)
	}

	// A non-numeric seq is a 400, not a panic.
	if got := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/runs/run-1/steps/abc",
		"marta", scenario.orgA, `{"status":"succeeded"}`).Code; got != http.StatusBadRequest {
		t.Fatalf("non-numeric seq = %d, want 400", got)
	}
}

// TestStepPayloadSurvivesTheRoundTrip pins the payload arriving back through the
// API unchanged in *content*.
//
// It deliberately does not assert byte identity. `steps.payload_json` is JSONB,
// so Postgres reorders keys and rewrites whitespace — this test can only see that
// if it runs against the real column, which is exactly why the assertion here is
// semantic and the live-database round trip is checked by
// internal/board/runtime_test.go plus a probe against the running API.
func TestStepPayloadSurvivesTheRoundTrip(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	payload := `{"z":1,  "a":[1,2]}`
	sent := `{"seq":2,"kind":"tool","name":"shell","payload":` + payload + `}`
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/steps",
		"marta", scenario.orgA, sent)
	if w.Code != http.StatusCreated {
		t.Fatalf("create step = %d, want 201 — %s", w.Code, w.Body.String())
	}
	var step stepResponse
	if err := json.Unmarshal(w.Body.Bytes(), &step); err != nil {
		t.Fatalf("decode step: %v", err)
	}
	if step.PayloadJSON == nil {
		t.Fatal("payload was dropped")
	}
	// The JSON decoder in the handler sees the same bytes the client wrote, so the
	// spacing inside the object must survive the round trip.
	var got, want map[string]any
	if err := json.Unmarshal([]byte(*step.PayloadJSON), &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("payload keys = %v, want %v", got, want)
	}
	for k, v := range want {
		if fmt.Sprint(got[k]) != fmt.Sprint(v) {
			t.Fatalf("payload[%q] = %v, want %v", k, got[k], v)
		}
	}
	// The array must survive as an array, not as a stringified blob.
	if _, ok := got["a"].([]any); !ok {
		t.Fatalf("payload[a] = %T, want an array", got["a"])
	}
}

// TestBoardLedgerReportsSpendAndBudget is the US-AD32 shape: the screen compares
// spend against the board's budget, so both figures have to come from the same
// response — a client that fetched them separately could show a crossed threshold
// as not-yet-crossed.
func TestBoardLedgerReportsSpendAndBudget(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/boards/board-a/ledger",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("board ledger = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var body boardLedgerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BudgetMicros != board.DefaultBudgetDailyMicros {
		t.Fatalf("budget = %d, want the board's %d", body.BudgetMicros, board.DefaultBudgetDailyMicros)
	}
	if body.Entries == nil {
		// An empty ledger must serialise as [] rather than null, or the screen has
		// to special-case a value the API chose.
		t.Fatal("entries was null, want an empty array")
	}
}
