package main

// POST /runs/{id}/cancel dan GET /runs/{id}/summary — sisa §6.2.11.
//
// Yang diuji di sini adalah dua hal yang tidak kelihatan dari handler-nya:
// gerbang perannya, dan bahwa cancel-run benar-benar MENCATAT pembatalannya
// (bukan cuma menjawab 200).

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"agentdeck/internal/board"
)

// RequestRunCancel meniru statement aslinya: guard-nya `status = 'running'`,
// jadi run yang sudah ditutup menghasilkan nol baris (ErrNotFound), bukan
// pembatalan yang tercatat.
func (s *stubRunRepo) RequestRunCancel(_ context.Context, id, orgID string) (time.Time, error) {
	run, ok := s.runs[id]
	if !ok || run.OrgID != orgID {
		return time.Time{}, board.ErrNotFound
	}
	if run.Status != board.RunRunning {
		return time.Time{}, board.ErrNotFound
	}
	now := time.Now()
	run.CancelRequestedAt = &now
	s.runs[id] = run
	return now, nil
}

// TestCancelRunRecordsTheRequest — ARCHITECTURE 6.2.11.
//
// The endpoint's whole job is to make the cancellation visible to the
// dispatcher, which reads `cancel_requested_at`. A handler that answered 200
// without writing would pass a status-code-only test, so the assertion is on
// the field.
func TestCancelRunRecordsTheRequest(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/cancel",
		"marta", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("cancel run = %d, want 200: %s", w.Code, w.Body.String())
	}

	stored := repo.runs["run-1"]
	if stored.CancelRequestedAt == nil {
		t.Fatal("cancel returned 200 but cancel_requested_at was never written")
	}
	// And the response says so, rather than making the caller poll to find out.
	var body struct {
		CancelRequestedAt string `json:"cancel_requested_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.CancelRequestedAt == "" {
		t.Fatalf("response does not report the cancellation: %s", w.Body.String())
	}
}

// raceStubRunRepo models the narrow window CancelRun has to survive: the run was
// live when it was read, and closed by the executor before the cancel write
// landed. The write therefore reports zero rows (ErrNotFound) — and the caller
// asked for "this run stops", which is now true.
type raceStubRunRepo struct {
	*stubRunRepo
}

func (r *raceStubRunRepo) RequestRunCancel(_ context.Context, id, orgID string) (time.Time, error) {
	// The executor won the race: close the run, then report that there was
	// nothing left for the cancel to write.
	run, err := r.GetRun(context.Background(), id, orgID)
	if err != nil {
		return time.Time{}, err
	}
	run.Status = board.RunEnded
	run.Outcome = "succeeded"
	r.runs[id] = run
	return time.Time{}, board.ErrNotFound
}

// TestCancelRunSurvivesTheExecutorWinningTheRace — the ErrNotFound branch.
//
// This is the one path where the code could plausibly get it wrong in a way no
// happy-path test notices: the run closed between the read and the write, so the
// cancel has nothing to record, and reporting 404 would tell the caller their
// cancellation failed when the run in fact stopped.
func TestCancelRunSurvivesTheExecutorWinningTheRace(t *testing.T) {
	scenario := newRBACTestAPI(t)
	inner := &stubRunRepo{
		runs: map[string]board.Run{
			"run-1": {
				ID: "run-1", OrgID: scenario.orgA, TaskID: "task-1", AgentID: "agent-1",
				Attempt: 1, Status: board.RunRunning, MaxRuntimeSecs: 3600,
			},
		},
		tasks: map[string]board.Task{
			"task-1": {
				ID: "task-1", OrgID: scenario.orgA, BoardID: "board-a",
				Status: board.StatusRunning, AssigneeAgentID: "agent-1", CurrentRunID: "run-1",
			},
		},
	}
	repo := &raceStubRunRepo{stubRunRepo: inner}
	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(repo), nil)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/cancel",
		"marta", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("cancel that lost the race = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		Status  string `json:"status"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The run is reported as the executor left it, not as a cancellation.
	if body.Status != "ended" || body.Outcome != "succeeded" {
		t.Fatalf("response does not reflect the executor's outcome: %s", w.Body.String())
	}
}

// TestCancelRunOnAFinishedRunIsIdempotent — §6.2.11's "Ya" (idempotent).
//
// There is nothing left to abort, so the desired state already holds. Answering
// 409 would turn a retried request into a failure.
func TestCancelRunOnAFinishedRunIsIdempotent(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	finished := repo.runs["run-1"]
	finished.Status = board.RunEnded
	finished.Outcome = "succeeded"
	repo.runs["run-1"] = finished

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/cancel",
		"marta", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("cancel on a finished run = %d, want 200: %s", w.Code, w.Body.String())
	}
	// Crucially it did not get "reopened" into a cancel: the run keeps its own
	// outcome.
	if got := repo.runs["run-1"].Outcome; got != "succeeded" {
		t.Fatalf("a finished run's outcome was overwritten by cancel: %q", got)
	}
}

// TestCancelRunUnknownRunIs404 — §6.2.11 / US-AD41 AC3/AC4.
//
// An id from another tenant must be indistinguishable from one that does not
// exist; both are 404, never 500 and never 200.
func TestCancelRunUnknownRunIs404(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/nope/cancel",
		"marta", scenario.orgA, "").Code; got != http.StatusNotFound {
		t.Fatalf("cancel unknown run = %d, want 404", got)
	}
	// bella is a member of orgB, so the middleware lets her through and the
	// 404 comes from the run lookup. Using a non-member here would test the
	// middleware's 403 instead, which is a different guard.
	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/cancel",
		"bella", scenario.orgB, "").Code; got != http.StatusNotFound {
		t.Fatalf("cancel run from another tenant = %d, want 404", got)
	}
}

// TestCancelRunRequiresMember — the role gate: a viewer may read a run but may
// not stop one.
func TestCancelRunRequiresMember(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/runs/run-1/cancel",
		"vera", scenario.orgA, "").Code; got != http.StatusForbidden {
		t.Fatalf("viewer cancel = %d, want 403", got)
	}
	if repo.runs["run-1"].CancelRequestedAt != nil {
		t.Fatal("a forbidden cancel still wrote the request")
	}
}

// TestRunSummaryReportsDuration — US-AD41 AC1.
//
// AC1 lists "status, outcome, durasi, total biaya, total token, attempt
// number". All but the duration are already on GET /runs/{id}; the duration is
// the subtraction, and it is the reason this endpoint exists.
func TestRunSummaryReportsDuration(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	// Deliberately NOT "ended ≈ now()": if the end time is essentially the
	// present, then measuring a finished run against now() gives the same answer
	// and the test cannot tell the two apart. This run ended 210 seconds ago, so
	// a handler that ignores ended_at reports ~300s instead of 90s.
	started := time.Now().Add(-300 * time.Second)
	ended := started.Add(90 * time.Second)
	run := repo.runs["run-1"]
	run.StartedAt = started
	run.EndedAt = &ended
	run.Outcome = "succeeded"
	run.Status = board.RunEnded
	run.CostMicros = 1234
	run.TokensIn = 10
	run.TokensOut = 20
	run.Summary = "done"
	repo.runs["run-1"] = run

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1/summary",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d, want 200: %s", w.Code, w.Body.String())
	}

	var body struct {
		RunID           string `json:"run_id"`
		TaskID          string `json:"task_id"`
		Attempt         int    `json:"attempt"`
		Status          string `json:"status"`
		Outcome         string `json:"outcome"`
		DurationSeconds int64  `json:"duration_seconds"`
		CostMicros      int64  `json:"cost_micros"`
		TokensIn        int64  `json:"tokens_in"`
		TokensOut       int64  `json:"tokens_out"`
		Summary         string `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DurationSeconds != 90 {
		t.Fatalf("duration_seconds = %d, want 90", body.DurationSeconds)
	}
	// The rest of AC1, so a future refactor cannot drop a field unnoticed.
	if body.RunID != "run-1" || body.TaskID != "task-1" || body.Attempt != 1 {
		t.Fatalf("identity fields wrong: %+v", body)
	}
	if body.Status != "ended" || body.Outcome != "succeeded" {
		t.Fatalf("status/outcome wrong: %+v", body)
	}
	if body.CostMicros != 1234 || body.TokensIn != 10 || body.TokensOut != 20 {
		t.Fatalf("cost/tokens wrong: %+v", body)
	}
}

// TestRunSummaryOnALiveRunMeasuresSoFar — an unfinished run reports elapsed
// time, not zero.
func TestRunSummaryOnALiveRunMeasuresSoFar(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	run := repo.runs["run-1"]
	run.StartedAt = time.Now().Add(-30 * time.Second)
	repo.runs["run-1"] = run

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1/summary",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		DurationSeconds int64  `json:"duration_seconds"`
		EndedAt         string `json:"ended_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DurationSeconds < 29 {
		t.Fatalf("live run reported %ds; a run in progress must not read as 0", body.DurationSeconds)
	}
	if body.EndedAt != "" {
		t.Fatalf("live run reports an ended_at: %q", body.EndedAt)
	}
}

// TestRunSummaryClampsANegativeDuration — clock skew between the API and the
// database can put started_at in the future. A page showing "-60s" is worse
// than one showing 0, and a negative duration is not a fact about the run.
func TestRunSummaryClampsANegativeDuration(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	run := repo.runs["run-1"]
	run.StartedAt = time.Now().Add(60 * time.Second)
	repo.runs["run-1"] = run

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1/summary",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		DurationSeconds int64 `json:"duration_seconds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DurationSeconds != 0 {
		t.Fatalf("duration_seconds = %d, want 0 (negative durations are clamped)", body.DurationSeconds)
	}
}

// TestRunSummaryUnknownRunIs404 — US-AD41 AC3/AC4.
func TestRunSummaryUnknownRunIs404(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/nope/summary",
		"vera", scenario.orgA, "").Code; got != http.StatusNotFound {
		t.Fatalf("summary of an unknown run = %d, want 404", got)
	}
	// Same reasoning as the cancel case: bella is in orgB, so this reaches the
	// lookup rather than the middleware.
	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1/summary",
		"bella", scenario.orgB, "").Code; got != http.StatusNotFound {
		t.Fatalf("summary across tenants = %d, want 404", got)
	}
}

// TestRunSummaryIsReadableByViewer — reading is Viewer in §6.2.11.
func TestRunSummaryIsReadableByViewer(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/runs/run-1/summary",
		"vera", scenario.orgA, "").Code; got != http.StatusOK {
		t.Fatalf("viewer summary = %d, want 200", got)
	}
}

// compile-time reminder: stubRunRepo must still satisfy board.Repository, so a
// new interface method fails the build here rather than panicking at runtime.
var _ board.Repository = (*stubRunRepo)(nil)

// keep pgx imported for the stub's zero-row sentinel
var _ = pgx.ErrNoRows
