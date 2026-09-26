package board

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// M4 runtime — the run lifecycle and the ledger.
//
// What this file is and is not:
//
//   It reads and writes the rows a runtime produces: `runs`, `steps`,
//   `ledger_entries`, and the state each one implies for its task. It does *not*
//   run an LLM. Claiming happens through `Service.Claim` (M1), and whoever claims
//   drives the run through these transitions.
//
// The task transitions are the part worth being careful about, because two
// contract sections disagree and the disagreement is not cosmetic:
//
//   ARCHITECTURE 5.3/5.4 says a successful run sets the task to `done`.
//   PRD US-AD22 AC2 and the J2 walkthrough say a successful run sets it to
//   `review`, with `review -> done` left to a human.
//
// The PRD wins here, and the reason is that the alternative dead-ends: `review`
// is in the state machine (5.4) and in the J2 flow, so a runtime that jumped
// straight to `done` would leave a state no code path could ever reach — the
// review screen would be permanently empty and "approve result" would have
// nothing to approve. A contract section that describes an unreachable state is
// the one that yields.

// validOutcomes is the CHECK constraint on `runs.outcome`
// (runs_outcome_chk). It is duplicated here rather than trusted, because a
// typo would otherwise become a 500 from the database instead of a 400 from the
// API — and the worker that sent it cannot tell those apart from the message.
var validOutcomes = map[string]bool{
	"succeeded":       true,
	"failed":          true,
	"timed_out":       true,
	"cancelled":       true,
	"reclaimed":       true,
	"budget_exceeded": true,
}

// StartRun opens a new attempt for a task that has already been claimed.
//
// `runID` comes from the caller because the claim already used it: Claim/
// ClaimReadyTasks sets `tasks.current_run_id = runID` inside the locking
// transaction (ARCHITECTURE 4b), so generating a second id here would leave the
// task pointing at a run that does not exist. This method asserts the two agree
// instead of creating the binding itself — a second unbinding write would be a
// claim path with no row lock behind it.
//
// The attempt number is derived from the rows that already exist rather than
// sent by the caller: a worker retrying after a timeout would otherwise reuse
// attempt 1 and the retry accounting (US-AD22, J4) would silently stop advancing.
func (s *Service) StartRun(ctx context.Context, orgID, taskID, agentID, runID string) (Run, error) {
	if runID == "" {
		return Run{}, ErrInvalidInput
	}
	task, err := s.repo.GetTask(ctx, taskID, orgID)
	if err != nil {
		return Run{}, err
	}
	if task.Status != StatusRunning || task.CurrentRunID != runID {
		// Either the task is not claimed, or it is claimed by a different run.
		// Both mean this caller is not the holder, so it must not write.
		return Run{}, ErrConflict
	}
	agent, err := s.repo.GetAgent(ctx, agentID, orgID)
	if err != nil {
		return Run{}, err
	}
	attempt, err := s.repo.NextRunAttempt(ctx, taskID)
	if err != nil {
		return Run{}, err
	}
	run, err := s.repo.CreateRun(ctx, Run{
		ID:      runID,
		OrgID:   orgID,
		TaskID:  taskID,
		AgentID: agent.ID,
		Attempt: attempt,
		Status:  RunRunning,
		// Copied at claim time, exactly like the DDL comment says: raising an
		// agent's limit later must not extend a run already in flight, or a
		// runaway run could be kept alive by editing the agent.
		MaxRuntimeSecs: agent.MaxRuntimeSeconds,
		ClaimLock:      s.claimLock,
	})
	if err != nil {
		return Run{}, err
	}
	_, err = s.RecordRunEvent(ctx, orgID, task.BoardID, taskID, run.ID, "run.claimed", map[string]any{
		"attempt": run.Attempt,
		"agent":   agent.Name,
	})
	return run, err
}

// GetRun reads one run inside its org.
func (s *Service) GetRun(ctx context.Context, id, orgID string) (Run, error) {
	return s.repo.GetRun(ctx, id, orgID)
}

// TaskRuns lists every attempt on a task, newest first.
func (s *Service) TaskRuns(ctx context.Context, taskID, orgID string) ([]Run, error) {
	return s.repo.ListTaskRuns(ctx, taskID, orgID)
}

// Heartbeat marks a live run as still working (N7: every 60s). It returns
// ErrNotFound when the run is no longer `running`, which is how a worker whose
// run was reclaimed (N8) is told to stop rather than keep writing.
func (s *Service) Heartbeat(ctx context.Context, runID, orgID string) (Run, error) {
	run, err := s.repo.HeartbeatRun(ctx, runID, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return run, err
}

// ClaimAndStart is the manual force-claim (US-AD21, "bypass loop dispatcher"):
// claim one named task, then open its run.
//
// It exists because the only other way in is the batch claim, which claims
// whatever the priority order hands back — a caller naming a specific task needs
// that task or an error, not a different one. The two steps are separate machine
// operations on purpose: if opening the run fails, the claim has already happened
// and the task is `running` with a run id, which the reclaim path (J6) recovers.
// Rolling them into one transaction would instead leave the task claimable by a
// second dispatcher the moment the run insert failed.
func (s *Service) ClaimAndStart(ctx context.Context, orgID, taskID string) (Run, error) {
	task, err := s.repo.GetTask(ctx, taskID, orgID)
	if err != nil {
		return Run{}, err
	}
	if task.Status != StatusReady {
		return Run{}, ErrConflict
	}
	// The agent is resolved before the claim so a task with no agent is refused
	// while it is still `ready`, rather than claimed and then abandoned.
	if task.AssigneeAgentID == "" {
		return Run{}, ErrInvalidInput
	}
	agent, err := s.repo.GetAgent(ctx, task.AssigneeAgentID, orgID)
	if err != nil {
		return Run{}, err
	}
	runID := Must()
	claimed, err := s.repo.ClaimTask(ctx, taskID, orgID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Someone else claimed it between the read and the write.
		return Run{}, ErrConflict
	}
	if err != nil {
		return Run{}, err
	}
	attempt, err := s.repo.NextRunAttempt(ctx, taskID)
	if err != nil {
		return Run{}, err
	}
	run, err := s.repo.CreateRun(ctx, Run{
		ID: runID, OrgID: orgID, TaskID: claimed.ID, AgentID: agent.ID,
		Attempt: attempt, Status: RunRunning, MaxRuntimeSecs: agent.MaxRuntimeSeconds,
		ClaimLock: s.claimLock,
	})
	if err != nil {
		return Run{}, err
	}
	_, err = s.RecordRunEvent(ctx, orgID, claimed.BoardID, claimed.ID, run.ID, "run.claimed", map[string]any{
		"attempt": attempt, "agent": agent.Name, "manual": true,
	})
	return run, err
}

// EndRun closes a run and moves its task to whatever the outcome implies.
//
// The order matters: the run is closed first, and only a run that actually
// transitioned (status was `running`) moves the task. A worker that lost its
// claim gets ErrNotFound and changes nothing — US-AD22 AC3 asks for exactly
// that, and moving the task anyway would let a zombie worker drag a task out of
// a retry another worker had already started.
func (s *Service) EndRun(ctx context.Context, runID, orgID string, summary RunSummary) (Run, error) {
	if !validOutcomes[summary.Outcome] {
		return Run{}, ErrInvalidInput
	}
	run, err := s.repo.EndRun(ctx, runID, orgID, summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	task, err := s.repo.GetTask(ctx, run.TaskID, orgID)
	if err != nil {
		return Run{}, err
	}
	if err := s.applyOutcome(ctx, task, run); err != nil {
		return Run{}, err
	}
	// Release the claim binding. EndRun is the only place a run stops being the
	// task's live run, and the claim predicate is `current_run_id IS NULL`
	// (ARCHITECTURE 4b) — so without this, every task a run ever touched would be
	// unclaimable from then on, which is precisely the retry path (J4), the
	// reclaim path (J6), and review -> ready.
	if err := s.repo.ClearTaskCurrentRun(ctx, task.ID, orgID); err != nil {
		return Run{}, err
	}
	_, err = s.RecordRunEvent(ctx, orgID, task.BoardID, task.ID, run.ID, "run.finished", map[string]any{
		"outcome":     summary.Outcome,
		"cost_micros": run.CostMicros,
	})
	return run, err
}

// applyOutcome is ARCHITECTURE 5.3's updateTaskAfterRun with the PRD's
// correction applied to the success case (see this file's header).
//
// The retry decision needs three things the run does not carry: the agent's
// retry policy, its attempt ceiling, and a failure counter that survives the
// run. The first two come from the agent row (they belong to the agent, US-AD96),
// the third is incremented here — before this, `tasks.consecutive_failures` had
// no writer at all, so the ceiling was compared against a number that never
// moved and a failing task would have retried forever.
func (s *Service) applyOutcome(ctx context.Context, task Task, run Run) error {
	switch run.Outcome {
	case "succeeded":
		// US-AD22 AC2 / J2: a finished run goes to `review`, not `done`. A human
		// approving the result is what closes the task.
		if err := s.resetFailures(ctx, task); err != nil {
			return err
		}
		return s.moveTask(ctx, task, StatusReview)
	case "budget_exceeded":
		// J5 step 3: the board cap was hit, so the task waits on a human rather
		// than looping. ARCHITECTURE 5.3 says `ready`, PRD US-AD32 AC2 says
		// `blocked` with block_kind=budget; the PRD wins for the same reason
		// `review` does — `ready` would immediately re-claim a task that just
		// proved it cannot finish inside the budget.
		return s.blockTask(ctx, task, "budget")
	case "failed", "timed_out":
		return s.retryOrFail(ctx, task, run)
	case "reclaimed":
		// 5.3: the task was already returned to `ready` by the reclaim path.
		return nil
	case "cancelled":
		return s.moveTask(ctx, task, StatusCancelled)
	}
	return nil
}

// retryOrFail is J4 plus the §10.2 taxonomy. Three-way, not two-way: a failure
// kind can retry, can be terminal-recoverable, or can fail outright.
//
// `blocked` is the branch that used to be missing. §10.2 says needs_input,
// dependency, policy and budget end in `blocked` with their own block_kind, and
// that is not a softer `failed`: a policy refusal or a board out of budget will
// refuse again identically, so retrying it spends money to learn nothing, while
// `failed` would claim the work is impossible when a human can still unblock it.
func (s *Service) retryOrFail(ctx context.Context, task Task, run Run) error {
	if kind, blocked := BlockedKind(run.FailureKind); blocked {
		return s.blockTask(ctx, task, kind)
	}
	failures, err := s.repo.IncrementTaskFailures(ctx, task.ID, task.OrgID)
	if err != nil {
		return err
	}
	agent, err := s.repo.GetAgent(ctx, run.AgentID, task.OrgID)
	if err != nil {
		// The agent row is the only source of the policy; without it, refusing to
		// retry is the safe answer. Retrying blind could loop forever.
		return s.moveTask(ctx, task, StatusFailed)
	}
	if !retryAllowed(agent.RetryPolicy, run.FailureKind) || failures >= agent.MaxAttempts {
		return s.moveTask(ctx, task, StatusFailed)
	}
	return s.moveTask(ctx, task, StatusReady)
}

// retryAllowed reads the agent's policy (agents_retry_policy_chk: never,
// transient_only, always). `transient_only` is the default and the interesting
// one: a capability failure — no credential, missing tool — will fail again no
// matter how many times it is retried, so retrying it only spends money.
func retryAllowed(policy, failureKind string) bool {
	switch policy {
	case "never":
		return false
	case "always":
		return true
	default: // transient_only
		return failureKind == "transient"
	}
}

// moveTask applies one dispatcher-owned transition. The guard is the WHERE
// clause (see UpdateTaskStatus): a concurrent writer produces zero rows, which
// is reported as a conflict rather than silently losing the transition.
func (s *Service) moveTask(ctx context.Context, task Task, to TaskStatus) error {
	return s.moveTaskFrom(ctx, task, task.Status, to)
}

// moveTaskFrom pins the row's current status as the precondition, except for the
// dispatcher-owned moves out of a live state: a run can finish while the task
// row says `running`, and reading the status first would turn a legitimate
// transition into a conflict.
func (s *Service) moveTaskFrom(ctx context.Context, task Task, from, to TaskStatus) error {
	if _, err := s.repo.UpdateTaskStatus(ctx, task.ID, task.OrgID, from, to); err != nil {
		return err
	}
	return nil
}

// blockTask sets a blocked task together with its block_kind. The two have to
// move together: a `blocked` task with no kind cannot be routed — the UI shows
// what it waits on, and the state machine (5.4) makes each kind reach `ready` by
// a different action.
func (s *Service) blockTask(ctx context.Context, task Task, kind string) error {
	return s.repo.BlockTask(ctx, task.ID, task.OrgID, kind)
}

func (s *Service) resetFailures(ctx context.Context, task Task) error {
	return s.repo.ResetTaskFailures(ctx, task.ID, task.OrgID)
}

// AppendLedger writes one priced call and rolls the result into the run totals.
//
// The price is not computed here. `pricing.Resolve` needs the provider and the
// model, which the executor already has, and a second place that prices calls
// is a second place that can disagree about what a token costs. This layer
// validates the row and stores it.
func (s *Service) AppendLedger(ctx context.Context, orgID, runID, taskID string, usage LedgerUsage) (LedgerEntry, error) {
	if usage.PriceVersion <= 0 {
		// price_version is mandatory in the DDL and for a reason: a cost with no
		// version cannot be re-derived once the table changes, so it is an
		// unverifiable number pretending to be a fact.
		return LedgerEntry{}, ErrInvalidInput
	}
	if usage.CostMicros < 0 || usage.TokensIn < 0 || usage.TokensOut < 0 {
		return LedgerEntry{}, ErrInvalidInput
	}
	entry, err := s.repo.CreateLedgerEntry(ctx, LedgerEntry{
		OrgID:            orgID,
		RunID:            runID,
		TaskID:           taskID,
		Provider:         usage.Provider,
		Model:            usage.Model,
		Kind:             usage.Kind,
		TokensIn:         usage.TokensIn,
		TokensOut:        usage.TokensOut,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens:  usage.ReasoningTokens,
		CostMicros:       usage.CostMicros,
		PriceVersion:     usage.PriceVersion,
		PriceSource:      usage.PriceSource,
		PricingModel:     usage.PricingModel,
	})
	if err != nil {
		return LedgerEntry{}, err
	}
	if _, err := s.RecordRunEvent(ctx, orgID, "", taskID, runID, "ledger.entry", map[string]any{
		"model":       usage.Model,
		"cost_micros": usage.CostMicros,
	}); err != nil {
		return LedgerEntry{}, err
	}
	return entry, nil
}

// RunLedger lists the priced calls of one run, oldest first.
func (s *Service) RunLedger(ctx context.Context, runID, orgID string) ([]LedgerEntry, error) {
	return s.repo.ListRunLedger(ctx, runID, orgID)
}

// BoardLedger lists a board's most recent priced calls.
func (s *Service) BoardLedger(ctx context.Context, boardID, orgID string, limit int) ([]LedgerEntry, error) {
	if limit <= 0 || limit > 500 {
		// Bounded on purpose: an unbounded ledger read is a full table scan
		// dressed up as an API call.
		limit = 100
	}
	return s.repo.ListBoardLedger(ctx, boardID, orgID, limit)
}

// BoardSpendToday is the number the US-AD32 cost gate reads. It is computed
// from `ledger_entries` rather than from `runs`, so a run still in flight
// contributes what it has actually spent.
func (s *Service) BoardSpendToday(ctx context.Context, boardID, orgID string) (int64, error) {
	return s.repo.BoardSpendToday(ctx, boardID, orgID)
}

// StartStep opens one line of a run's trace. `seq` is sent by the caller because
// only the executor knows how many steps it has taken; the UNIQUE (run_id, seq)
// constraint is what stops two of them from claiming the same line.
//
// The payload is normalised, not preserved: `steps.payload_json` is JSONB
// (ARCHITECTURE 3.13), and Postgres reorders keys and rewrites whitespace on the
// way in. So the trace is faithful in *content* — which key held which value —
// and not in bytes. Anything that needs the original bytes (a signed body, an
// exact command line) has to travel as a string field inside the JSON, because
// no amount of care here survives the column type.
func (s *Service) StartStep(ctx context.Context, orgID, runID string, step Step, payload any) (Step, error) {
	if step.Seq < 1 || step.Seq > 5000 {
		return Step{}, ErrInvalidInput
	}
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return Step{}, ErrInvalidInput
		}
		if len(encoded) > 65536 {
			// N21: one step payload is capped at 64 KB. Truncating silently would
			// make the trace lie about what was sent, so this refuses instead.
			return Step{}, ErrInvalidInput
		}
		step.PayloadJSON = encoded
	}
	return s.repo.CreateStep(ctx, Step{
		OrgID:       orgID,
		RunID:       runID,
		Seq:         step.Seq,
		Kind:        step.Kind,
		Name:        step.Name,
		Status:      "running",
		PayloadJSON: step.PayloadJSON,
	})
}

// FinishStep closes one trace line with its measured cost.
func (s *Service) FinishStep(ctx context.Context, orgID, runID string, seq int, step Step) (Step, error) {
	if step.Status != "succeeded" && step.Status != "failed" {
		return Step{}, ErrInvalidInput
	}
	finished, err := s.repo.FinishStep(ctx, runID, seq, orgID, step)
	if errors.Is(err, pgx.ErrNoRows) {
		return Step{}, ErrNotFound
	}
	return finished, err
}

// RunSteps lists a run's trace in order.
func (s *Service) RunSteps(ctx context.Context, runID, orgID string) ([]Step, error) {
	return s.repo.ListRunSteps(ctx, runID, orgID)
}

// RecordRunEvent appends a run-scoped event. Same append-only rule as task
// events; `boardID` may be empty for events that are not board-scoped.
func (s *Service) RecordRunEvent(ctx context.Context, orgID, boardID, taskID, runID, kind string, payload any) (Event, error) {
	var encoded []byte
	if payload != nil {
		var err error
		encoded, err = json.Marshal(payload)
		if err != nil {
			return Event{}, ErrInvalidInput
		}
	}
	return s.repo.CreateEvent(ctx, Event{
		OrgID:       orgID,
		BoardID:     boardID,
		TaskID:      taskID,
		RunID:       runID,
		Kind:        kind,
		PayloadJSON: encoded,
	})
}
