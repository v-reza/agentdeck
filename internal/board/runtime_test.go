package board

import (
	"context"
	"errors"
	"testing"

	"agentdeck/internal/ulid"
)

// M4 runtime against a live Postgres. The unit-level decisions (`retryAllowed`)
// are pure functions and are tested without a database; everything here needs
// real rows, because what is being pinned is the *wiring*: that the outcome of a
// run actually moves its task, that `consecutive_failures` actually increments,
// and that a stale run writes nothing.
//
// Gate: AGENTDECK_TEST_DATABASE_URL, same as the rest of this package.

// runtimeFixture builds the smallest graph a run needs: org → project → board →
// agent → task, then claims the task and opens its run. Returning the ids as a
// struct keeps each test's setup to one line and makes the "which id is which"
// mistake impossible.
type runtimeFixture struct {
	orgID    string
	boardID  string
	taskID   string
	agentID  string
	runID    string
	svc      *Service
	repo     Repository
	maxTries int
}

func newRuntimeFixture(t *testing.T, retryPolicy string, maxAttempts int) runtimeFixture {
	t.Helper()
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	project, err := repo.CreateProject(ctx, Project{
		ID: ulid.Must(), OrgID: orgID, Slug: lower("rt-" + ulid.Must()[20:]), Name: "Runtime",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	boardID := ulid.Must()
	if _, err := repo.CreateBoard(ctx, Board{
		ID: boardID, OrgID: orgID, ProjectID: project.ID,
		Slug: lower("rt-" + ulid.Must()[20:]), Name: "Runtime Board", Columns: DefaultColumns,
		// The same cap the create endpoint defaults to (cmd/api/boards.go). Seeding
		// this repo call with the zero value would make the board's cap $0, which the
		// dispatcher reads as "already over budget".
		BudgetDailyMicros: DefaultBudgetDailyMicros,
	}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	agent, err := repo.CreateAgent(ctx, Agent{
		ID: ulid.Must(), OrgID: orgID, ProjectID: project.ID, Name: "runner",
		// A protocol, not a vendor: this is what the column holds since US-AD109.
		Provider: "openai_compatible", Model: "gpt-4o",
		MaxRuntimeSeconds: 3600, RetryPolicy: retryPolicy, MaxAttempts: maxAttempts,
		// skills_json/tools_json are NOT NULL with an array CHECK: the column is
		// JSONB, so the zero value (nil []byte) is SQL NULL, not '[]'.
		SkillsJSON: []byte("[]"), ToolsJSON: []byte("[]"),
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := repo.CreateTask(ctx, Task{
		ID: ulid.Must(), OrgID: orgID, BoardID: boardID, Title: "Run me",
		Status: StatusBacklog, CreatedBy: newUser(t, ctx),
		WorkspaceKind: WorkspaceScratch, GoalMode: "auto",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// backlog -> ready -> claimed (running). Claiming writes current_run_id, and
	// StartRun refuses a run id the task does not point at, so it has to happen
	// through the real claim path.
	if _, err := repo.UpdateTaskStatus(ctx, task.ID, orgID, StatusBacklog, StatusReady); err != nil {
		t.Fatalf("ready: %v", err)
	}
	runID := ulid.Must()
	claimed, err := repo.ClaimReadyTasks(ctx, orgID, boardID, []string{runID}, 1)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claim returned %d tasks, want 1", len(claimed))
	}
	return runtimeFixture{
		orgID: orgID, boardID: boardID, taskID: task.ID, agentID: agent.ID, runID: runID,
		// WithClaimLock matches production: the dispatcher always names its instance,
		// and a run with no owner cannot be heartbeated by anyone (see
		// HeartbeatOwned, which refuses an unnamed heartbeat rather than applying it
		// to every run in the org).
		svc: NewService(repo).WithClaimLock("test-host"), repo: repo, maxTries: maxAttempts,
	}
}

func (f runtimeFixture) start(t *testing.T) Run {
	t.Helper()
	run, err := f.svc.StartRun(context.Background(), f.orgID, f.taskID, f.agentID, f.runID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	return run
}

func (f runtimeFixture) task(t *testing.T) Task {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return task
}

// TestPgStartRunBindsClaimAndStartsAttemptOne pins the claim/run handshake: the
// run id the claim wrote is the one the run row carries, and a caller holding a
// different id is refused instead of forking the task's ownership.
func TestPgStartRunBindsClaimAndStartsAttemptOne(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	run := f.start(t)

	if run.ID != f.runID {
		t.Fatalf("run id = %q, want the claimed id %q", run.ID, f.runID)
	}
	if run.Attempt != 1 {
		t.Fatalf("first attempt = %d, want 1", run.Attempt)
	}
	if run.Status != RunRunning {
		t.Fatalf("status = %q, want %q", run.Status, RunRunning)
	}
	// 3600 is the agent's limit, copied at claim time. If this were read live
	// from the agent, editing the agent could extend a run already in flight.
	if run.MaxRuntimeSecs != 3600 {
		t.Fatalf("max runtime = %d, want 3600 (copied from the agent)", run.MaxRuntimeSecs)
	}

	// A second StartRun with a foreign run id must not create a row: the task
	// belongs to one run.
	if _, err := f.svc.StartRun(context.Background(), f.orgID, f.taskID, f.agentID, ulid.Must()); !errors.Is(err, ErrConflict) {
		t.Fatalf("StartRun with a foreign run id = %v, want ErrConflict", err)
	}
}

// TestPgEndRunSucceededMovesTaskToReview is US-AD22 AC2 and the J2 walkthrough:
// a successful run hands the task to a human, it does not close it. ARCHITECTURE
// 5.3 says `done` here; the PRD wins because `review` would otherwise be an
// unreachable state and "approve the result" would have nothing to approve.
func TestPgEndRunSucceededMovesTaskToReview(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)

	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "succeeded", Summary: "done",
	}); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	if got := f.task(t).Status; got != StatusReview {
		t.Fatalf("task after a successful run = %q, want %q", got, StatusReview)
	}
}

// TestPgEndRunRejectsAnUnknownOutcome keeps the topic out of the database's
// hands. `runs_outcome_chk` would reject a typo'd outcome too, but as a 500:
// the worker that sent it cannot tell a bad request from a broken server.
func TestPgEndRunRejectsAnUnknownOutcome(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)

	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "finished",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown outcome = %v, want ErrInvalidInput", err)
	}
}

// TestPgEndRunCountsAndCapsRetries is J4. Two things have to hold together:
// `consecutive_failures` increments (before this, nothing wrote it, so the
// ceiling was compared against a frozen number and a failing task retried
// forever), and the ceiling is the agent's `max_attempts`.
func TestPgEndRunCountsAndCapsRetries(t *testing.T) {
	f := newRuntimeFixture(t, "always", 2)
	f.start(t)

	// Attempt 1 fails: one failure so far, so a retry is allowed.
	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "failed", FailureKind: "transient",
	}); err != nil {
		t.Fatalf("EndRun attempt 1: %v", err)
	}
	task := f.task(t)
	if task.Status != StatusReady {
		t.Fatalf("after failure 1 status = %q, want %q (max_attempts=2)", task.Status, StatusReady)
	}
	if task.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d after one failure, want 1", task.ConsecutiveFailures)
	}

	// Attempt 2 fails as well: 2 failures against max_attempts=2 is the ceiling,
	// so the task stops instead of looping.
	runID2 := ulid.Must()
	if _, err := f.repo.ClaimReadyTasks(context.Background(), f.orgID, f.boardID, []string{runID2}, 1); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if _, err := f.svc.StartRun(context.Background(), f.orgID, f.taskID, f.agentID, runID2); err != nil {
		t.Fatalf("StartRun attempt 2: %v", err)
	}
	if _, err := f.svc.EndRun(context.Background(), runID2, f.orgID, RunSummary{
		Outcome: "failed", FailureKind: "transient",
	}); err != nil {
		t.Fatalf("EndRun attempt 2: %v", err)
	}
	task = f.task(t)
	if task.Status != StatusFailed {
		t.Fatalf("status at the ceiling = %q, want %q", task.Status, StatusFailed)
	}
	if task.ConsecutiveFailures != 2 {
		t.Fatalf("consecutive_failures = %d, want 2", task.ConsecutiveFailures)
	}
}

// TestPgEndRunTransientOnlyPolicyIgnoresACapabilityFailure is the reason the
// policy exists. A capability failure (missing credential, missing tool) will
// fail again identically; retrying it spends money to produce the same error.
func TestPgEndRunTransientOnlyPolicyIgnoresACapabilityFailure(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 5)
	f.start(t)

	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "failed", FailureKind: "capability",
	}); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	if got := f.task(t).Status; got != StatusFailed {
		t.Fatalf("capability failure under transient_only = %q, want %q", got, StatusFailed)
	}
}

// TestPgEndRunBudgetExceededBlocksTheTask is J5 step 3 cross-checked against the
// PRD: a run that hit the board cap waits for a human with block_kind=budget.
// Returning it to `ready` (ARCHITECTURE 5.3's version) would re-claim it
// immediately and spend the budget again.
func TestPgEndRunBudgetExceededBlocksTheTask(t *testing.T) {
	f := newRuntimeFixture(t, "always", 5)
	f.start(t)

	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "budget_exceeded",
	}); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	task := f.task(t)
	if task.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", task.Status, StatusBlocked)
	}
	if task.BlockKind != "budget" {
		t.Fatalf("block_kind = %q, want \"budget\" (a blocked task with no kind cannot be routed)",
			task.BlockKind)
	}
}

// TestPgEndRunStaleRunWritesNothing is US-AD22 AC3: a worker that lost its claim
// cannot close the run, and — the part that actually matters — cannot move the
// task either. A zombie worker dragging a task out of a retry another worker had
// already started is the failure this prevents.
func TestPgEndRunStaleRunWritesNothing(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)

	// The reclaim path closes the run first; then the original worker reports in.
	if _, err := f.repo.EndRun(context.Background(), f.runID, f.orgID, RunSummary{Outcome: "reclaimed"}); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	before := f.task(t)

	if _, err := f.svc.EndRun(context.Background(), f.runID, f.orgID, RunSummary{
		Outcome: "succeeded", Summary: "late",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale EndRun = %v, want ErrNotFound", err)
	}
	after := f.task(t)
	if after.Status != before.Status {
		t.Fatalf("stale EndRun moved the task: %q -> %q", before.Status, after.Status)
	}
}

// TestPgHeartbeatRefusesADeadRun is US-AD23 via the heartbeat surface: once the
// run is closed, a beat is answered 404 so the worker stops instead of working
// against a task that has moved on.
func TestPgHeartbeatRefusesADeadRun(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)

	if _, err := f.svc.Heartbeat(context.Background(), f.runID, f.orgID); err != nil {
		t.Fatalf("heartbeat on a live run: %v", err)
	}
	if _, err := f.repo.EndRun(context.Background(), f.runID, f.orgID, RunSummary{Outcome: "cancelled"}); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := f.svc.Heartbeat(context.Background(), f.runID, f.orgID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("heartbeat on a closed run = %v, want ErrNotFound", err)
	}
}

// TestPgLedgerEntriesRollIntoRunTotals pins the rollup being computed from the
// ledger in the same statement that closes the run. A rollup maintained
// separately by the executor would drift the moment a run ended without one, and
// `runs.cost_micros` is what the cost gate and the cost screen read.
func TestPgLedgerEntriesRollIntoRunTotals(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)
	ctx := context.Background()

	entries := []LedgerUsage{
		{Provider: "openai", Model: "gpt-4o", Kind: "llm", TokensIn: 100, TokensOut: 50,
			CostMicros: 1500, PriceVersion: 1, PriceSource: "catalog", PricingModel: "gpt-4o"},
		{Provider: "openai", Model: "gpt-4o", Kind: "llm", TokensIn: 200, TokensOut: 20,
			CostMicros: 2500, PriceVersion: 1, PriceSource: "catalog", PricingModel: "gpt-4o"},
	}
	for _, e := range entries {
		if _, err := f.svc.AppendLedger(ctx, f.orgID, f.runID, f.taskID, e); err != nil {
			t.Fatalf("AppendLedger: %v", err)
		}
	}
	// A price_version is mandatory: a cost with no version cannot be re-derived
	// after the price table changes, so it is an unverifiable number.
	if _, err := f.svc.AppendLedger(ctx, f.orgID, f.runID, f.taskID, LedgerUsage{
		Provider: "openai", Model: "gpt-4o", Kind: "llm", CostMicros: 10, PriceVersion: 0,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ledger entry without a price version = %v, want ErrInvalidInput", err)
	}

	run, err := f.svc.EndRun(ctx, f.runID, f.orgID, RunSummary{Outcome: "succeeded"})
	if err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	if run.CostMicros != 4000 {
		t.Fatalf("run cost = %d, want 4000 (summed from the ledger)", run.CostMicros)
	}
	if run.TokensIn != 300 || run.TokensOut != 70 {
		t.Fatalf("run tokens = %d/%d, want 300/70", run.TokensIn, run.TokensOut)
	}

	spend, err := f.svc.BoardSpendToday(ctx, f.boardID, f.orgID)
	if err != nil {
		t.Fatalf("BoardSpendToday: %v", err)
	}
	if spend != 4000 {
		t.Fatalf("board spend today = %d, want 4000", spend)
	}

	ledger, err := f.svc.RunLedger(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("RunLedger: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("ledger rows = %d, want 2 (the rejected entry must not be stored)", len(ledger))
	}
}

// TestPgRunIsScopedToItsOrg is the tenant boundary. Every runtime read carries
// org_id, so a run id from another workspace must be indistinguishable from one
// that does not exist.
func TestPgRunIsScopedToItsOrg(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)

	if _, err := f.svc.GetRun(context.Background(), f.runID, newOrg(t, context.Background())); err == nil {
		t.Fatal("GetRun returned a run from another org")
	}
}

// TestPgStepsAreDenseAndOrdered pins the trace: `seq` comes back in order, and
// the UNIQUE (run_id, seq) constraint stops two workers from claiming the same
// line — otherwise `ORDER BY seq` would look right while the trace was ambiguous.
func TestPgStepsAreDenseAndOrdered(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if _, err := f.svc.StartStep(ctx, f.orgID, f.runID, Step{
			Seq: i, Kind: "llm", Name: "call",
		}, map[string]any{"n": i, "step": "call"}); err != nil {
			t.Fatalf("StartStep %d: %v", i, err)
		}
	}
	steps, err := f.svc.RunSteps(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("RunSteps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(steps))
	}
	for i, s := range steps {
		if s.Seq != i+1 {
			t.Fatalf("step %d has seq %d, want %d (trace out of order)", i, s.Seq, i+1)
		}
	}

	// Reusing a seq is refused by the database, not silently accepted.
	if _, err := f.svc.StartStep(ctx, f.orgID, f.runID, Step{Seq: 1, Kind: "llm"}, nil); err == nil {
		t.Fatal("a duplicate (run_id, seq) was accepted")
	}

	// Finishing moves the line to a terminal status with its measured cost.
	finished, err := f.svc.FinishStep(ctx, f.orgID, f.runID, 1, Step{
		Status: "succeeded", TokensIn: 10, TokensOut: 5, CostMicros: 42,
	})
	if err != nil {
		t.Fatalf("FinishStep: %v", err)
	}
	if finished.Status != "succeeded" || finished.CostMicros != 42 {
		t.Fatalf("finished step = %q/%d, want succeeded/42", finished.Status, finished.CostMicros)
	}
}

// TestPgStepPayloadOver64KBIsRefused is N21. Truncating instead would make the
// trace lie about what the agent actually sent, which is the opposite of what a
// trace is for.
func TestPgStepPayloadOver64KBIsRefused(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)

	big := make([]byte, 70000)
	for i := range big {
		big[i] = 'x'
	}
	if _, err := f.svc.StartStep(context.Background(), f.orgID, f.runID, Step{
		Seq: 1, Kind: "llm",
	}, map[string]any{"blob": string(big)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized payload = %v, want ErrInvalidInput", err)
	}
}

// TestRetryAllowedIsTheAgentsPolicy is the pure half, so a reader can see the
// table without a database. `transient_only` is the default and the interesting
// case: retrying a capability failure reproduces the same error at full price.
func TestRetryAllowedIsTheAgentsPolicy(t *testing.T) {
	cases := []struct {
		policy, kind string
		want         bool
	}{
		{"never", "transient", false},
		{"always", "capability", true},
		{"transient_only", "transient", true},
		{"transient_only", "capability", false},
		{"transient_only", "budget", false},
		{"transient_only", "", false},
	}
	for _, c := range cases {
		if got := retryAllowed(c.policy, c.kind); got != c.want {
			t.Errorf("retryAllowed(%q, %q) = %v, want %v", c.policy, c.kind, got, c.want)
		}
	}
}
