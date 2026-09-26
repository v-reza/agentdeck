package board

import (
	"context"
	"errors"
	"testing"
)

// POST /tasks/{id}/cancel, /retry, /archive against a live Postgres.
//
// The interesting behaviour is not the happy path — it is what happens when a
// run is mid-flight, when a task is already finished, and when two writers race.
// Those are the cases the state machine (5.4) and US-AD59 describe, and the ones
// a handler that only forwards a status string gets wrong.

// TestPgCancelRecordsTheRequestOnTheLiveRun is the durable half. Before this,
// the cancel lived in the dispatcher's process memory, so an API process asking
// the dispatcher to stop a run did nothing observable at all.
func TestPgCancelRecordsTheRequestOnTheLiveRun(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	if _, err := f.svc.CancelTask(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	requested, err := f.repo.RunCancelRequested(ctx, f.runID)
	if err != nil {
		t.Fatalf("RunCancelRequested: %v", err)
	}
	if !requested {
		t.Error("cancel was not recorded against the live run")
	}
	if got := f.task(t).Status; got != StatusCancelled {
		t.Errorf("task status = %q, want %q", got, StatusCancelled)
	}
}

// TestPgCancelIsIdempotentAndKeepsTheFirstTimestamp: a retried request must not
// move the stamp. "Since when was this cancelled" is the thing an operator reads
// off the screen, and a second click would otherwise reset it.
func TestPgCancelIsIdempotentAndKeepsTheFirstTimestamp(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	if _, err := f.svc.CancelTask(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	first, err := f.repo.RequestRunCancel(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	task, err := f.svc.CancelTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("second cancel must not error: %v", err)
	}
	if task.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", task.Status)
	}

	second, err := f.repo.RequestRunCancel(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("re-request: %v", err)
	}
	if first.IsZero() || second.IsZero() {
		t.Fatal("cancel returned a zero timestamp after a cancel")
	}
	if !first.Equal(second) {
		t.Errorf("timestamp moved: %v -> %v", first, second)
	}
}

// TestPgCancelRefusesAFinishedTask: cancelling completed work would rewrite
// history. A task that ended in `done` is not a task that was stopped.
func TestPgCancelRefusesAFinishedTask(t *testing.T) {
	ctx := context.Background()
	// One fixture per terminal state: `running -> failed` is a dispatcher-owned
	// transition that UpdateTaskStatus deliberately refuses (only the run's own
	// outcome may apply it), so each case is seeded from its own task.
	for _, terminal := range []TaskStatus{StatusDone, StatusFailed} {
		f := newRuntimeFixture(t, "transient_only", 3)
		f.start(t)
		if _, err := f.repo.UpdateTaskStatus(ctx, f.taskID, f.orgID, StatusRunning, terminal); err != nil {
			t.Fatalf("move to %s: %v", terminal, err)
		}
		if _, err := f.svc.CancelTask(ctx, f.taskID, f.orgID); !errors.Is(err, ErrConflict) {
			t.Errorf("cancelling a %s task: err = %v, want ErrConflict", terminal, err)
		}
		if got := f.task(t).Status; got != terminal {
			t.Errorf("status changed from %s to %s", terminal, got)
		}
	}
}

// TestPgCancelWithoutALiveRunStillCancelsTheTask: a `backlog` task has nothing
// to abort, and refusing to cancel it would make the endpoint useless for the
// only state a user can see before anything runs.
func TestPgCancelWithoutALiveRunStillCancelsTheTask(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// Move it out of the claimed state without ever opening a run.
	if _, err := f.repo.UpdateTaskStatus(ctx, f.taskID, f.orgID, StatusRunning, StatusReady); err != nil {
		t.Fatalf("back to ready: %v", err)
	}
	if err := f.repo.ClearTaskCurrentRun(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("clear run binding: %v", err)
	}

	task, err := f.svc.CancelTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if task.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", task.Status)
	}
}

// TestPgRetryResetsFailuresAndReturnsToReady is the endpoint's whole contract.
// The counter reset is the part that matters: without it the task returns to
// `ready` already at its ceiling and fails terminally on the very next error,
// which makes the retry look like it worked and then not work.
func TestPgRetryResetsFailuresAndReturnsToReady(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	// Burn two attempts, then park the task in `failed` the way the ceiling does.
	for range 2 {
		if _, err := f.repo.IncrementTaskFailures(ctx, f.taskID, f.orgID); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}
	if _, err := f.repo.UpdateTaskStatus(ctx, f.taskID, f.orgID, StatusRunning, StatusFailed); err != nil {
		t.Fatalf("to failed: %v", err)
	}
	if err := f.repo.ClearTaskCurrentRun(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("clear run binding: %v", err)
	}

	task, err := f.svc.RetryTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	if task.Status != StatusReady {
		t.Errorf("status = %q, want ready", task.Status)
	}
	if task.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0", task.ConsecutiveFailures)
	}
}

// TestPgRetryRefusesWhileARunHoldsTheTask. This is the guard that keeps the
// endpoint from creating two runs on one task: returning a `running` task to
// `ready` makes it claimable again while the first run is still writing steps.
func TestPgRetryRefusesWhileARunHoldsTheTask(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	if _, err := f.svc.RetryTask(ctx, f.taskID, f.orgID); !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict while running", err)
	}
	if got := f.task(t).Status; got != StatusRunning {
		t.Errorf("status = %q, want running (unchanged)", got)
	}
}

// TestPgRetryWorksFromEveryNonRunningStatus. §10.1 sends `capability` and
// `policy` to No-Retry, but that is the AUTOMATIC policy: the human who just
// fixed the credential or registered the missing tool holds information the
// taxonomy does not, and the endpoint exists for them. `blocked` is the state
// those failures land in, so refusing it would make the endpoint pointless.
func TestPgRetryWorksFromEveryNonRunningStatus(t *testing.T) {
	ctx := context.Background()
	for _, from := range []TaskStatus{
		StatusBacklog, StatusReady, StatusBlocked, StatusReview,
		StatusDone, StatusFailed, StatusCancelled,
	} {
		f := newRuntimeFixture(t, "never", 3)
		if _, err := f.repo.UpdateTaskStatus(ctx, f.taskID, f.orgID, StatusRunning, from); err != nil {
			t.Fatalf("seed %s: %v", from, err)
		}
		if err := f.repo.ClearTaskCurrentRun(ctx, f.taskID, f.orgID); err != nil {
			t.Fatalf("clear %s: %v", from, err)
		}
		task, err := f.svc.RetryTask(ctx, f.taskID, f.orgID)
		if err != nil {
			t.Errorf("retry from %s: %v", from, err)
			continue
		}
		if task.Status != StatusReady {
			t.Errorf("retry from %s left status %q, want ready", from, task.Status)
		}
	}
}

// TestPgArchiveRequiresTerminalAndIsIdempotent is US-AD59 AC2 + AC3. The AC4
// role floor is the route's gate and is tested at the HTTP layer.
func TestPgArchiveRequiresTerminalAndIsIdempotent(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	// AC2: still running, so refused.
	if _, err := f.svc.MoveTask(ctx, f.taskID, f.orgID, StatusRunning, StatusArchived); !errors.Is(err, ErrArchiveRequiresTerminal) {
		t.Errorf("archiving a running task: err = %v, want ErrArchiveRequiresTerminal", err)
	}

	// Reach a terminal state, then archive twice.
	if _, err := f.repo.UpdateTaskStatus(ctx, f.taskID, f.orgID, StatusRunning, StatusDone); err != nil {
		t.Fatalf("to done: %v", err)
	}
	task, err := f.svc.MoveTask(ctx, f.taskID, f.orgID, StatusDone, StatusArchived)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if task.Status != StatusArchived {
		t.Fatalf("status = %q, want archived", task.Status)
	}
	// AC3: the second call names `archived` as its own source, which is the
	// idempotent branch — a repeated request succeeds rather than 409-ing.
	again, err := f.svc.MoveTask(ctx, f.taskID, f.orgID, StatusArchived, StatusArchived)
	if err != nil {
		t.Fatalf("re-archive must not error: %v", err)
	}
	if again.Status != StatusArchived {
		t.Errorf("status = %q after re-archive", again.Status)
	}
}

// TestPgCancelAndRetryAreTenantScoped: the ids are the only thing separating two
// orgs, and an unscoped write here would let one tenant stop another's work.
func TestPgCancelAndRetryAreTenantScoped(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()
	otherOrg := newOrg(t, ctx)

	if _, err := f.svc.CancelTask(ctx, f.taskID, otherOrg); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org cancel: err = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.RetryTask(ctx, f.taskID, otherOrg); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConflict) {
		t.Errorf("cross-org retry: err = %v, want a refusal", err)
	}
	if got := f.task(t).Status; got != StatusRunning {
		t.Errorf("cross-org request changed the task to %q", got)
	}
}

// TestPgEndRunCancelledClosesAnEndedRun: the cancel can race the executor to
// closing the run. Zero rows then means "already closed", which is the state the
// caller wanted — reporting an error would turn a benign race into a 500.
func TestPgEndRunCancelledClosesAnEndedRun(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	// The executor gets there first.
	if _, err := f.svc.EndRun(ctx, f.runID, f.orgID, RunSummary{Outcome: "succeeded", Summary: "done"}); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	// Zero rows from the guarded UPDATE: the run is already closed, so there is
	// nothing left to cancel. Reported as ErrNotFound so the caller can tell
	// "I just closed it" from "it was already closed" — the dispatcher treats
	// the latter as the expected race, not as a failure.
	if _, err := f.svc.EndRunCancelled(ctx, f.runID, f.orgID, "cancelled too late"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for an already-ended run", err)
	}
	// And the original outcome is not rewritten: the work really did succeed.
	run, err := f.repo.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Outcome != "succeeded" {
		t.Errorf("outcome = %q, want the original succeeded", run.Outcome)
	}
}

// TestPgEndRunCancelledSetsTheOutcomeWhenTheRunIsStillOpen is the normal path.
func TestPgEndRunCancelledSetsTheOutcomeWhenTheRunIsStillOpen(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	run, err := f.svc.EndRunCancelled(ctx, f.runID, f.orgID, "cancelled before the first call")
	if err != nil {
		t.Fatalf("EndRunCancelled: %v", err)
	}
	if run.Outcome != "cancelled" {
		t.Errorf("outcome = %q, want cancelled", run.Outcome)
	}
	if run.Status != RunEnded {
		t.Errorf("status = %q, want ended", run.Status)
	}
}

// TestPgApplyOutcomeCancelledMovesTheTaskToCancelled pins the dispatcher's cancel
// route to the same task transition as a run that reported `cancelled` itself.
// Two routes to the same state, one assertion.
func TestPgApplyOutcomeCancelledMovesTheTaskToCancelled(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	task := f.task(t)
	if err := f.svc.ApplyOutcome(ctx, task, Run{Outcome: "cancelled", OrgID: f.orgID, TaskID: f.taskID}); err != nil {
		t.Fatalf("ApplyOutcome: %v", err)
	}
	if got := f.task(t).Status; got != StatusCancelled {
		t.Errorf("status = %q, want cancelled", got)
	}
}

// TestPgCancelDoesNotTouchAnotherRunsFlag: the cancel is addressed to one run id.
// Writing it org-wide would stop every run in the workspace on one click.
func TestPgCancelDoesNotTouchAnotherRunsFlag(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	// A second task in the same org, claimed with its own run id.
	b := newRuntimeFixture(t, "transient_only", 3)
	b.start(t)
	if b.runID == f.runID {
		t.Fatal("fixtures produced the same run id")
	}

	if _, err := f.svc.CancelTask(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	bystander, err := f.repo.RunCancelRequested(ctx, b.runID)
	if err != nil {
		t.Fatalf("read bystander: %v", err)
	}
	if bystander {
		t.Error("cancelling one task marked a different run for cancellation")
	}
	if got := b.task(t).Status; got != StatusRunning {
		t.Errorf("bystander task status = %q, want running (untouched)", got)
	}
}

// TestPgRetryOnAnUnknownTaskIsNotFound pins the distinction between "no such
// task" and "this task is busy". Both used to come back as a conflict, because
// the statement's guard refuses a `running` task by matching zero rows — which
// makes the status code an existence oracle. Found by probing the real API; the
// unit test below it passed either way because the double had the same flaw.
func TestPgRetryOnAnUnknownTaskIsNotFound(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	if _, err := f.svc.RetryTask(ctx, "01HZZZZZZZZZZZZZZZZZZZZZ", f.orgID); !errors.Is(err, ErrNotFound) {
		t.Errorf("retry an unknown task: err = %v, want ErrNotFound", err)
	}
	// And the busy case is still a conflict, so the two are distinguishable.
	if _, err := f.svc.RetryTask(ctx, f.taskID, f.orgID); !errors.Is(err, ErrConflict) {
		t.Errorf("retry a running task: err = %v, want ErrConflict", err)
	}
}
