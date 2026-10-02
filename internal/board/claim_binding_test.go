package board

import (
	"context"
	"testing"
)

// A task that returns to `ready` must not keep its run binding.
//
// ClaimReadyTasks requires `current_run_id IS NULL` (ARCHITECTURE 4b), and the
// column is set by claiming and cleared by EndRun/ReleaseClaim/reclaim. The two
// paths that move a task BACK to `ready` from outside the tick wrote the status
// without clearing the column, so the task carried a run id that belonged to a
// run that had already ended:
//
//	RetryTask  (POST /tasks/{id}/retry)  — status = 'ready', binding untouched
//	MoveTask   (PATCH /tasks/{id})       — status = 'ready', binding untouched
//
// The result is a task that is visible in the Ready column, has an agent, and
// can never be claimed again — by the dispatcher or by POST /tasks/{id}/claim.
// It is not `running`, so reclaim ignores it; it is not `blocked`, so the board
// shows it as queued work. The only way out was editing the database.
//
// Found by watching a real board: a task retried after a `policy` failure sat in
// Ready for seven minutes with a live dispatcher and was never picked up.
//
// These tests fail against the unfixed statements, which is the point: the bug
// is a missing clause, and only a real Postgres can prove the clause is there.

// The retry path. The fixture's task is `running` with a run bound, so the retry
// has to land it in a state that is claimable again.
func TestPgRetryClearsTheRunBindingSoTheTaskCanBeClaimed(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// End the run first, the way the executor does. A task with a live run is
	// refused by the retry guard, and that refusal is correct — the stale binding
	// this test is about is the one a FINISHED run leaves behind.
	// The fixture claims through the repository, which writes current_run_id but
	// NOT a `runs` row — StartRun is what inserts that, and it refuses a task
	// whose binding is not the run id it was given. So the run row is created the
	// way production creates it, through StartRun itself.
	if _, err := f.svc.StartRun(ctx, f.orgID, f.taskID, f.agentID, f.runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := f.svc.EndRun(ctx, f.runID, f.orgID, RunSummary{
		Outcome: "failed", FailureKind: "policy",
	}); err != nil {
		t.Fatalf("end run: %v", err)
	}

	if _, err := f.svc.RetryTask(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("retry: %v", err)
	}

	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if task.Status != StatusReady {
		t.Fatalf("status = %q, want ready", task.Status)
	}
	if task.CurrentRunID != "" {
		t.Errorf("current_run_id = %q after a retry, want empty: "+
			"ClaimReadyTasks requires NULL, so this task can never be claimed again",
			task.CurrentRunID)
	}

	// The claim is the assertion that matters. A status check alone passes while
	// the task is still unclaimable.
	claimed, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, RunBucketSize)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	found := false
	for _, c := range claimed {
		if c.ID == f.taskID {
			found = true
		}
	}
	if !found {
		t.Errorf("a retried task was not claimed by the next tick: it is ready, "+
			"visible, and stuck (%d tasks claimed)", len(claimed))
	}
}

// The PATCH path. `backlog -> ready` is a documented transition (ARCHITECTURE
// 4a) and the one a user drives from the board.
func TestPgPatchToReadyClearsTheRunBinding(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// Move the running task back to backlog first, then to ready — the shape a
	// user creates by dragging a card, and the shape that used to strand it.
	if _, err := f.svc.MoveTask(ctx, f.taskID, f.orgID, StatusRunning, StatusBacklog); err != nil {
		t.Fatalf("running -> backlog: %v", err)
	}
	if _, err := f.svc.MoveTask(ctx, f.taskID, f.orgID, StatusBacklog, StatusReady); err != nil {
		t.Fatalf("backlog -> ready: %v", err)
	}

	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if task.CurrentRunID != "" {
		t.Errorf("current_run_id = %q after moving to ready, want empty", task.CurrentRunID)
	}

	claimed, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, RunBucketSize)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	found := false
	for _, c := range claimed {
		if c.ID == f.taskID {
			found = true
		}
	}
	if !found {
		t.Error("a task moved back to ready was not claimed by the next tick")
	}
}

// The binding must NOT be cleared while the task is genuinely running: the
// dispatcher needs it to address the run, and StartRun refuses a task whose
// current_run_id is not the run id it was called with.
func TestPgRunBindingSurvivesWhileRunning(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if task.Status != StatusRunning || task.CurrentRunID != f.runID {
		t.Errorf("fixture is not running with its run bound: status=%q run=%q want %q",
			task.Status, task.CurrentRunID, f.runID)
	}
}

// The other half of the same invariant: a task can be stuck `running` with no run
// behind it at all. The process died between ClaimReadyTasks — which writes the
// status and the binding — and StartRun, which inserts the `runs` row. There is
// no row for ReclaimStaleRuns to close, so that sweep cannot see it, and the task
// is not `ready` either, so no claim will ever pick it up.
//
// Found on a real board: two tasks `running` since 2026-09-23 in an org that had
// never had a dispatcher running.
func TestPgReclaimOrphanedClaimsReturnsTheTaskToReady(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// The fixture claims through the repository, which writes the binding without
	// inserting a run — precisely the orphaned shape. Age it past the grace period
	// so it is not read as a claim mid-handoff.
	ageClaim(t, ctx, f.taskID, 20)

	moved, err := f.svc.ReclaimOrphanedClaims(ctx, f.orgID, 50, 3)
	if err != nil {
		t.Fatalf("reclaim orphaned: %v", err)
	}
	found := false
	for _, m := range moved {
		if m.ID == f.taskID {
			found = true
		}
	}
	if !found {
		t.Fatalf("an orphaned claim was not reclaimed (%d moved)", len(moved))
	}

	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if task.Status != StatusReady {
		t.Errorf("status = %q, want ready", task.Status)
	}
	if task.CurrentRunID != "" {
		t.Errorf("current_run_id = %q, want empty", task.CurrentRunID)
	}

	// Claimable again is the assertion that matters, not the status.
	claimed, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, RunBucketSize)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	back := false
	for _, c := range claimed {
		if c.ID == f.taskID {
			back = true
		}
	}
	if !back {
		t.Error("a reclaimed orphan was not claimable again")
	}
}

// The grace period is load-bearing: between claiming a task and inserting its run
// row there is a real window where the task is `running` with no run behind it.
// A sweep that did not respect that window would steal claims out from under a
// healthy dispatcher and hand the same task to two workers.
func TestPgReclaimOrphanedClaimsLeavesFreshClaimsAlone(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// The fixture's claim is seconds old, which is the mid-handoff state.
	moved, err := f.svc.ReclaimOrphanedClaims(ctx, f.orgID, 50, 3)
	if err != nil {
		t.Fatalf("reclaim orphaned: %v", err)
	}
	if len(moved) != 0 {
		t.Errorf("reclaimed %d claim(s) made seconds ago: a healthy handoff was "+
			"treated as an orphan", len(moved))
	}

	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if task.Status != StatusRunning || task.CurrentRunID != f.runID {
		t.Errorf("a fresh claim was disturbed: status=%q run=%q want running/%q",
			task.Status, task.CurrentRunID, f.runID)
	}
}

// ageClaim backdates a task's claim so the orphan sweep's grace period has
// elapsed. `started_at` is the column that records when the claim happened.
func ageClaim(t *testing.T, ctx context.Context, taskID string, minutes int) {
	t.Helper()
	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE tasks SET started_at = now() - make_interval(mins => $2) WHERE id = $1`,
		taskID, minutes); err != nil {
		t.Fatalf("age claim: %v", err)
	}
}
