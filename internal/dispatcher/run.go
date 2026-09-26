package dispatcher

import (
	"context"
	"errors"

	"agentdeck/internal/board"
)

// errBudgetReached ends a run whose board hit its daily cap (N16/N17). It is not
// a failure of the work: the spend it was allowed to make is already made, and
// §10.2 routes `budget` to `blocked` so a human decides whether to raise the cap.
var errBudgetReached = errors.New("board daily budget reached")

// errRunCancelled ends a run a human asked to stop. Deliberately NOT
// errBudgetReached: §5.5 gives "cancelled" its own outcome, and §5.3 routes it to
// the task's `cancelled` status. Reusing the budget error would report a
// cancelled run as `budget_exceeded` and block the task on a budget that is fine.
var errRunCancelled = errors.New("run cancelled by request")

// stepSink records each step the executor reports. It is the only place that
// turns executor output into rows, which keeps the storage decisions out of the
// executor (5.3).
type stepSink struct {
	d     *Dispatcher
	ctx   context.Context
	runID string
	task  board.Task
	// budgetExceeded is set when this step pushed the board past its daily cap,
	// so the run ends as budget_exceeded rather than as a plain success.
	budgetExceeded bool
	// cancelled is set when a human asked for this run to stop, either because
	// the guard above fired or because the check before the run found it already
	// requested. It is separate from budgetExceeded because the two produce
	// different outcomes and different task states.
	cancelled bool
}

// Step implements Sink. Order matters: the step row is written first, then the
// ledger row, then the rollups. If the process dies between them the ledger is
// missing a row but the step still explains what happened — the reverse order
// loses the only record of the spend.
func (s *stepSink) Step(seq int, kind, status string, usage board.LedgerUsage,
	costMicros int64, priceVersion int, priceSource, pricingModel string, payload any) error {

	// Cancellation is checked here, not only before the run starts: the executor
	// makes its provider call before reporting the step, so by the time this runs
	// the money is spent. What this protects is everything after — a run whose
	// first step lands after a cancel must not keep emitting steps, and must end
	// as `cancelled` rather than as a success.
	if s.d.cancelRequested(s.ctx, s.runID) {
		s.cancelled = true
		return errRunCancelled
	}

	if _, err := s.d.store.StartStep(s.ctx, s.task.OrgID, s.runID, board.Step{
		RunID: s.runID, Seq: seq, Kind: kind, Status: "running",
	}, payload); err != nil {
		return err
	}

	// Cache, reasoning and price fields are absent on purpose: the `steps` table
	// carries only tokens_in/out and cost_micros, while the money detail
	// (cache_read_tokens, price_version, price_source, pricing_model) lives on the
	// ledger row. The step is the work record; the ledger is the money record.
	step := board.Step{
		RunID: s.runID, Seq: seq, Kind: kind, Status: status,
		TokensIn: usage.TokensIn, TokensOut: usage.TokensOut,
		CostMicros: costMicros,
	}
	if _, err := s.d.store.FinishStep(s.ctx, s.task.OrgID, s.runID, seq, step); err != nil {
		return err
	}

	usage.Kind = kind
	usage.CostMicros = costMicros
	usage.PriceVersion = priceVersion
	usage.PriceSource = priceSource
	usage.PricingModel = pricingModel
	if _, err := s.d.store.AppendLedger(s.ctx, s.task.OrgID, s.runID, s.task.ID, usage); err != nil {
		return err
	}

	// Three rollups, each for a different reader: the run's own totals (run detail
	// screen), the board's day (N16 cap the next tick reads), and the run counter.
	if err := s.d.store.RecordRunUsage(s.ctx, s.runID, costMicros, usage.TokensIn, usage.TokensOut); err != nil {
		return err
	}
	if err := s.d.store.BumpDailyCost(s.ctx, s.task.OrgID, s.task.BoardID,
		costMicros, usage.TokensIn, usage.TokensOut); err != nil {
		return err
	}

	// N17: a single run may not spend past the per-run hard stop. The cap is read
	// after the spend is recorded, so the run that crosses it is the one that ends.
	if err := s.checkRunBudget(); err != nil {
		return err
	}
	return nil
}

// checkRunBudget ends the run when the board's day has reached its cap.
//
// It cancels a flag rather than the context because the executor is mid-call: the
// spend that pushed the board over is already paid for, so the honest outcome is
// "this run stopped because the board ran out", not "this run failed".
func (s *stepSink) checkRunBudget() error {
	budget, err := s.d.store.BoardBudgetToday(s.ctx, s.task.OrgID, s.task.BoardID)
	if err != nil {
		// A budget read that fails must not silently allow unbounded spend: stop
		// this run. Returning the error propagates to the executor, which aborts.
		return err
	}
	if budget.Exceeded() {
		s.budgetExceeded = true
		s.d.markCancelled(s.runID)
		return errBudgetReached
	}
	return nil
}

// runOne executes a started run and closes it with the right outcome.
func (d *Dispatcher) runOne(ctx context.Context, b board.Board, task board.Task, agent board.ResolvedAgent, runID string) {
	sink := &stepSink{d: d, ctx: ctx, runID: runID, task: task}

	output, failureKind := d.runner.Run(ctx, task, agent, runID, sink)
	// Re-check at the end: the executor emits one step, so a cancel that lands
	// after that step would otherwise never be noticed.
	cancelled := d.cancelRequested(ctx, runID)

	summary := board.RunSummary{Summary: output}
	switch {
	case sink.budgetExceeded || d.wasCancelled(runID):
		// J5 step 3 / N17: the cap was reached. `budget_exceeded` is its own
		// outcome because §10.2 routes it to `blocked`, not to a retry.
		summary.Outcome = "budget_exceeded"
		summary.FailureKind = "budget"
	case cancelled:
		// A human asked for this. Checked BEFORE the budget branch because a run
		// can be cancelled while the board is also over its cap, and the human's
		// reason is the true one — `budget_exceeded` would block the task on a
		// budget that is not the problem.
		summary.Outcome = "cancelled"
	case failureKind != "":
		summary.Outcome = "failed"
		summary.FailureKind = failureKind
		summary.Error = output
	case ctx.Err() != nil:
		// The process is shutting down or the parent context ended. §5.3 gives this
		// its own outcome: a cancelled run is not a failed one, and retrying it
		// would be a policy decision a human has not made.
		summary.Outcome = "cancelled"
	default:
		summary.Outcome = "succeeded"
	}

	if err := d.store.BumpDailyRunCount(ctx, task.BoardID); err != nil {
		d.log.Warn("dispatcher: run count rollup", "board", task.BoardID, "error", err)
	}

	run, err := d.store.EndRun(ctx, runID, task.OrgID, summary)
	if err != nil {
		d.log.Error("dispatcher: ending run", "run", runID, "task", task.ID, "error", err)
		return
	}
	d.clearCancelled(runID)

	// 5.3 / 4e.1: a task that reached its end wakes whatever was waiting on it. The
	// wake is unconditional on `done` in the SQL, so a run that ended in `review`
	// does not release dependents prematurely.
	if run.Status == board.RunEnded && summary.Outcome == "succeeded" {
		if err := d.store.WakeDependents(ctx, task.ID); err != nil {
			d.log.Warn("dispatcher: waking dependents", "task", task.ID, "error", err)
		}
	}
}

// cancelRequested reads the durable cancel flag written by
// POST /tasks/{id}/cancel. It fails open: a read error reports "not cancelled",
// because the alternative is aborting runs whenever the database hiccups, and
// the budget guard (which fails closed) is the one that protects money.
func (d *Dispatcher) cancelRequested(ctx context.Context, runID string) bool {
	requested, err := d.store.RunCancelRequested(ctx, runID)
	if err != nil {
		d.log.Warn("dispatcher: cancel check", "run", runID, "error", err)
		return false
	}
	return requested
}

// closeCancelled ends a run that was cancelled before it made any call, and
// moves its task to `cancelled`.
//
// EndRunCancelled is used rather than EndRun because the cancel may have raced
// the executor to closing the run; zero rows then means "already closed", which
// is the outcome the caller wanted either way. The task transition runs on the
// same path as a cancelled run that did start, so both routes to `cancelled`
// leave the same state behind.
func (d *Dispatcher) closeCancelled(ctx context.Context, task board.Task, runID string) {
	run, err := d.store.EndRunCancelled(ctx, runID, task.OrgID, "cancelled before the first provider call")
	if err != nil && !errors.Is(err, board.ErrNotFound) {
		// ErrNotFound is the expected race: the executor closed the run between
		// the cancel being recorded and this line. Everything else is a real
		// failure worth a log.
		d.log.Warn("dispatcher: closing cancelled run", "run", runID, "error", err)
	}
	if err := d.store.ClearTaskCurrentRun(ctx, task.ID, task.OrgID); err != nil {
		d.log.Warn("dispatcher: releasing cancelled task", "task", task.ID, "error", err)
		return
	}
	if err := d.store.ApplyOutcome(ctx, task, run); err != nil {
		d.log.Warn("dispatcher: applying cancelled outcome", "task", task.ID, "error", err)
	}
}

func (d *Dispatcher) markCancelled(runID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancelled[runID] = true
}

func (d *Dispatcher) wasCancelled(runID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancelled[runID]
}

func (d *Dispatcher) clearCancelled(runID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cancelled, runID)
}
