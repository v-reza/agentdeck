package board

import (
	"context"
	"testing"

	"agentdeck/internal/ulid"
)

// The dispatcher's own persistence: budget aggregate, batch claim with one run id
// per task, the dependency gate, and stale reclaim. Each of these is a behaviour
// that only a real Postgres can confirm — a fake would have to reimplement the
// SQL to disagree with it.

func TestPgBudgetStartsAtZeroAndTracksSpend(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// A board with no spend yet must still report its cap: the guardrail cannot
	// depend on a row existing for today.
	b, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if b.SpentTodayMicros != 0 || b.RunCount != 0 {
		t.Errorf("fresh board: spent=%d runs=%d, want 0/0", b.SpentTodayMicros, b.RunCount)
	}
	if b.CapMicros != DefaultBudgetDailyMicros {
		t.Errorf("cap = %d, want the board's %d", b.CapMicros, DefaultBudgetDailyMicros)
	}
	if b.Exceeded() {
		t.Error("a board with no spend reports exceeded")
	}

	// Two steps, then the verdicts. N18 alerts at 80% of the cap: the fixture's board
	// is seeded with DefaultBudgetDailyMicros (N16), so 80% is 16_000_000 and the
	// cap itself is 20_000_000.
	if err := f.svc.BumpDailyCost(ctx, f.orgID, f.boardID, 16_000_000, 100, 10); err != nil {
		t.Fatalf("bump: %v", err)
	}
	if err := f.svc.BumpDailyRunCount(ctx, f.boardID); err != nil {
		t.Fatalf("run count: %v", err)
	}
	b, _ = f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if b.RunCount != 1 {
		t.Errorf("run_count = %d, want 1 (one run, not one per step)", b.RunCount)
	}
	if b.Status != "warning" {
		t.Errorf("at 80%%: status = %q, want warning", b.Status)
	}
	if b.Exceeded() {
		t.Error("the 80% alert was treated as the hard stop")
	}

	if err := f.svc.BumpDailyCost(ctx, f.orgID, f.boardID, 4_000_000, 0, 0); err != nil {
		t.Fatalf("bump: %v", err)
	}
	b, _ = f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if !b.Exceeded() || b.Status != "exceeded" {
		t.Errorf("at the cap: status = %q exceeded = %v", b.Status, b.Exceeded())
	}
	if b.SpentTodayMicros != 20_000_000 {
		t.Errorf("spent = %d, want the sum of both steps", b.SpentTodayMicros)
	}
}

// TestPgBatchClaimGivesEachTaskItsOwnRun is the regression guard for the shared
// run id: a batch used to write one `current_run_id` for every row it claimed,
// which left all but the first task pointing at a run that would never exist.
func TestPgBatchClaimGivesEachTaskItsOwnRun(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// Two more claimed-able tasks alongside the fixture's own (already running).
	for i := 0; i < 2; i++ {
		newReadyTask(t, ctx, f)
	}

	tasks, runIDs, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, RunBucketSize)
	if err != nil {
		t.Fatalf("claim batch: %v", err)
	}
	if len(tasks) != 2 || len(runIDs) != 2 {
		t.Fatalf("claimed %d tasks with %d run ids, want 2/2", len(tasks), len(runIDs))
	}
	if runIDs[0] == runIDs[1] {
		t.Fatal("two tasks share a run id")
	}
	for i, task := range tasks {
		if task.CurrentRunID != runIDs[i] {
			t.Errorf("task %d points at run %q, got %q", i, task.CurrentRunID, runIDs[i])
		}
	}

	// A short id list is refused rather than half-applied: it would write NULL into
	// current_run_id and leave a `running` task with no run behind it.
	if _, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, 0); err == nil {
		t.Error("a zero-size batch was accepted")
	}
}

// TestPgClaimRefusesATaskWhoseParentIsUnfinished covers 4a's dependency predicate.
// Without it a child task runs before the work it depends on.
func TestPgClaimRefusesATaskWhoseParentIsUnfinished(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	child := newReadyTask(t, ctx, f)
	// The fixture's task is the parent and is `running`, not done.
	if err := f.repo.CreateTaskLink(ctx, f.taskID, child.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	if _, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, RunBucketSize); err != nil {
		t.Fatalf("claim: %v", err)
	}
	after, err := f.repo.GetTask(ctx, child.ID, f.orgID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if after.Status == StatusRunning {
		t.Error("a task with an unfinished parent was claimed")
	}
}

// TestPgDependencyGateBlocksAndWakes covers both halves of 4e: a ready task whose
// parent is unfinished becomes `blocked`/`dependency`, and it returns to `ready`
// when that parent reaches `done`.
func TestPgDependencyGateBlocksAndWakes(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	child := newReadyTask(t, ctx, f)
	if err := f.repo.CreateTaskLink(ctx, f.taskID, child.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	moved, err := f.svc.BlockDependentTasks(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("dependency gate: %v", err)
	}
	if moved != 1 {
		t.Fatalf("blocked %d tasks, want 1", moved)
	}
	blocked, _ := f.repo.GetTask(ctx, child.ID, f.orgID)
	if blocked.Status != StatusBlocked || blocked.BlockKind != "dependency" {
		t.Fatalf("child = %s/%q, want blocked/dependency", blocked.Status, blocked.BlockKind)
	}

	// A blocked task with no kind cannot be routed, and `dependency` is the only
	// kind this path may set.
	if err := f.repo.WakeDependents(ctx, f.taskID); err != nil {
		t.Fatalf("wake: %v", err)
	}
	woken, _ := f.repo.GetTask(ctx, child.ID, f.orgID)
	if woken.Status != StatusReady {
		t.Errorf("after wake: %s, want ready", woken.Status)
	}
	if woken.BlockKind != "" {
		t.Errorf("block_kind = %q, want it cleared", woken.BlockKind)
	}
}

// TestPgReclaimReturnsStaleRunsToTheQueue covers 4b. It also pins the ceiling: a
// task that has used up its attempts must go to `failed`, not back to `ready`,
// or the queue would reclaim it forever.
func TestPgReclaimReturnsStaleRunsToTheQueue(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	if _, err := f.svc.StartRun(ctx, f.orgID, f.taskID, f.agentID, f.runID); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Age the run past N8 without waiting 15 minutes: the heartbeat is what the
	// query compares, so that is what moves.
	ageRun(t, ctx, f.runID, 20)

	moved, err := f.svc.ReclaimStale(ctx, f.orgID, 50, 3)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(moved) != 1 {
		t.Fatalf("reclaimed %d, want 1", len(moved))
	}
	if moved[0].Status != StatusReady {
		t.Errorf("task = %s, want ready (1 of 3 attempts used)", moved[0].Status)
	}
	run, err := f.svc.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != RunEnded || run.Outcome != "reclaimed" {
		t.Errorf("run = %s/%s, want ended/reclaimed", run.Status, run.Outcome)
	}

	// The reclaimed task is claimable again — this is the whole point, and it only
	// works because the reclaim clears current_run_id.
	if _, _, err := f.svc.ClaimBatch(ctx, f.orgID, f.boardID, 1); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
}

// TestPgReclaimFailsATaskAtItsAttemptCeiling is the other half of 4b: the same
// query must not hand back a task that has run out of attempts.
func TestPgReclaimFailsATaskAtItsAttemptCeiling(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	if _, err := f.svc.StartRun(ctx, f.orgID, f.taskID, f.agentID, f.runID); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Two failures already, so this reclaim is the third attempt.
	for i := 0; i < 2; i++ {
		if _, err := f.repo.IncrementTaskFailures(ctx, f.taskID, f.orgID); err != nil {
			t.Fatalf("bump failures: %v", err)
		}
	}
	ageRun(t, ctx, f.runID, 20)

	moved, err := f.svc.ReclaimStale(ctx, f.orgID, 50, 3)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(moved) != 1 {
		t.Fatalf("reclaimed %d, want 1", len(moved))
	}
	if moved[0].Status != StatusFailed {
		t.Errorf("task = %s, want failed at the attempt ceiling", moved[0].Status)
	}
}

// TestPgReleaseClaimFreesATaskWithNoRun is the guard for the claim-without-run
// state: `status='running'` and a run id but no `runs` row, which reclaim cannot
// see and the claim predicate will not touch.
func TestPgReleaseClaimFreesATaskWithNoRun(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// The fixture already claimed the task (running + current_run_id) without
	// starting a run, which is exactly the broken state.
	if err := f.svc.ReleaseClaim(ctx, f.taskID, f.orgID, f.runID, "needs_input", "no agent assigned"); err != nil {
		t.Fatalf("release: %v", err)
	}
	task, err := f.repo.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != StatusBlocked || task.BlockKind != "needs_input" {
		t.Errorf("task = %s/%q, want blocked/needs_input", task.Status, task.BlockKind)
	}
	if task.CurrentRunID != "" {
		t.Errorf("current_run_id = %q, want it cleared", task.CurrentRunID)
	}
	if task.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1", task.ConsecutiveFailures)
	}

	// A capability gap is terminal in §10.2, not blocked.
	f2 := newRuntimeFixture(t, "transient_only", 3)
	if err := f2.svc.ReleaseClaim(ctx, f2.taskID, f2.orgID, f2.runID, "capability", "no endpoint"); err != nil {
		t.Fatalf("release: %v", err)
	}
	task2, _ := f2.repo.GetTask(ctx, f2.taskID, f2.orgID)
	if task2.Status != StatusFailed {
		t.Errorf("capability release: %s, want failed", task2.Status)
	}
}

// TestPgHeartbeatIsScopedToItsOwner: two instances must not keep each other's
// dead runs alive (5.1 phase 1).
func TestPgHeartbeatIsScopedToItsOwner(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	if _, err := f.svc.StartRun(ctx, f.orgID, f.taskID, f.agentID, f.runID); err != nil {
		t.Fatalf("start: %v", err)
	}
	ageRun(t, ctx, f.runID, 20)

	// A different instance's heartbeat must not touch this run. The run was aged to
	// 20 minutes, so a STALE age is the pass condition — an age near zero would mean
	// someone refreshed it.
	other := NewService(f.repo).WithClaimLock("some-other-host")
	if err := other.HeartbeatOwned(ctx, f.orgID); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if age := lastHeartbeatAge(t, ctx, f.runID); age < 15*60 {
		t.Errorf("a foreign instance refreshed this run's heartbeat (age %.0fs)", age)
	}

	// The owner's own heartbeat does refresh it, which is what makes the scoping
	// above a real restriction rather than a heartbeat that never works.
	owner := NewService(f.repo).WithClaimLock(claimLockOf(t, ctx, f.runID))
	if err := owner.HeartbeatOwned(ctx, f.orgID); err != nil {
		t.Fatalf("owner heartbeat: %v", err)
	}
	if age := lastHeartbeatAge(t, ctx, f.runID); age > 5 {
		t.Errorf("the owning instance did not refresh its own run (age %.0fs)", age)
	}

	// The owner must not be able to heartbeat without a name at all: that would
	// either touch every run in the org or none.
	if err := NewService(f.repo).HeartbeatOwned(ctx, f.orgID); err == nil {
		t.Error("a heartbeat with no owner name was accepted")
	}
}

// newReadyTask seeds one more task in the fixture's board, moved to `ready` the
// way the API does it.
func newReadyTask(t *testing.T, ctx context.Context, f runtimeFixture) Task {
	t.Helper()
	task, err := f.repo.CreateTask(ctx, Task{
		ID: ulid.Must(), OrgID: f.orgID, BoardID: f.boardID, Title: "Next",
		Status: StatusBacklog, CreatedBy: newUser(t, ctx),
		WorkspaceKind: WorkspaceScratch, GoalMode: "auto",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := f.repo.UpdateTaskStatus(ctx, task.ID, f.orgID, StatusBacklog, StatusReady); err != nil {
		t.Fatalf("ready: %v", err)
	}
	return task
}

// ageRun moves a run's heartbeat back by `minutes`, so a stale-run path can be
// exercised without the test sleeping for N8's fifteen minutes. It writes
// `last_heartbeat_at` directly because that is the column the reclaim compares.
func ageRun(t *testing.T, ctx context.Context, runID string, minutes int) {
	t.Helper()
	// pgSuitePool is the same pool the repository writes through, so this touches
	// the row the production path reads.
	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE runs SET last_heartbeat_at = now() - make_interval(mins => $2) WHERE id = $1`,
		runID, minutes); err != nil {
		t.Fatalf("age run: %v", err)
	}
}

// lastHeartbeatAge reports how many seconds ago the run last heartbeat.
func lastHeartbeatAge(t *testing.T, ctx context.Context, runID string) float64 {
	t.Helper()
	var age float64
	if err := pgSuitePool.QueryRow(ctx,
		`SELECT EXTRACT(EPOCH FROM (now() - last_heartbeat_at)) FROM runs WHERE id = $1`,
		runID).Scan(&age); err != nil {
		t.Fatalf("heartbeat age: %v", err)
	}
	return age
}

// claimLockOf reads the owner name stored on a run. A run started through the
// service carries the Service's claimLock; when none was set the column is NULL,
// and a heartbeat with an empty owner is refused rather than applying to every run.
func claimLockOf(t *testing.T, ctx context.Context, runID string) string {
	t.Helper()
	var lock *string
	if err := pgSuitePool.QueryRow(ctx, `SELECT claim_lock FROM runs WHERE id = $1`, runID).Scan(&lock); err != nil {
		t.Fatalf("read claim_lock: %v", err)
	}
	if lock == nil {
		return ""
	}
	return *lock
}
