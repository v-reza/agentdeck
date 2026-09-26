package board

import (
	"context"
	"errors"

	"agentdeck/internal/store"

	"github.com/jackc/pgx/v5"
)

// This file holds the dispatcher's own reads and writes (ARCHITECTURE 5.1). They
// live here rather than in runtime.go because runtime.go serves the HTTP surface —
// one run, addressed by id — while everything below serves the tick loop, which
// walks a board.

// RunBucketSize is the N20 batch ceiling: one tick claims at most this many tasks
// per board.
const RunBucketSize = 20

// StaleRunAge is N8: a run with no heartbeat for this long is reclaimed.
const StaleRunAgeMinutes = 15

// ClaimBatch claims up to `limit` ready tasks for a board, returning each with the
// run id that was written into its `current_run_id`.
//
// The run ids are generated here, one per row, because the SQL takes an array: a
// single id shared across a batch was the bug that left every task but the first
// without a run row.
func (s *Service) ClaimBatch(ctx context.Context, orgID, boardID string, limit int) ([]Task, []string, error) {
	if limit <= 0 {
		return nil, nil, ErrInvalidInput
	}
	if limit > RunBucketSize {
		limit = RunBucketSize
	}
	runIDs := make([]string, limit)
	for i := range runIDs {
		runIDs[i] = Must()
	}
	tasks, err := s.repo.ClaimReadyTasks(ctx, orgID, boardID, runIDs, limit)
	if err != nil {
		return nil, nil, err
	}
	// Hand back exactly the ids that landed on rows, in claim order: the SQL
	// assigns by row number, so this is the same order the query returned.
	return tasks, runIDs[:len(tasks)], nil
}

// BlockDependentTasks moves ready tasks whose parents are unfinished into
// `blocked`/`dependency` (5.2 phase 3d). Returning them to `blocked` rather than
// leaving them `ready` matters for more than tidiness: a `ready` task that can
// never be claimed is exactly what the claim predicate now refuses, and the board
// should show why it is waiting instead of showing it as work in the queue.
func (s *Service) BlockDependentTasks(ctx context.Context, orgID, boardID string) (int64, error) {
	return s.repo.BlockDependentTasks(ctx, orgID, boardID)
}

// ReclaimStale closes runs that stopped heartbeating and returns their tasks to
// the queue (4b). Tasks that have exhausted their attempts become `failed`
// instead, which is the same ceiling retryOrFail applies on the normal path.
func (s *Service) ReclaimStale(ctx context.Context, orgID string, limit, maxAttempts int) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	return s.repo.ReclaimStaleRuns(ctx, orgID, limit, maxAttempts)
}

// HeartbeatOwned refreshes the heartbeat of the runs this instance holds. The
// instance name comes from the Service, set once at wiring time, so the tick loop
// does not have to carry it through every call.
func (s *Service) HeartbeatOwned(ctx context.Context, orgID string) error {
	if s.claimLock == "" {
		// Without an owner name the heartbeat would either touch every run in the
		// org (keeping other instances' dead runs alive) or nothing at all.
		// Refusing is the honest answer: see NewDispatcher, which requires it.
		return ErrInvalidInput
	}
	return s.repo.HeartbeatOwnedRuns(ctx, orgID, s.claimLock)
}

// WakeDependents unblocks the children of a task that just reached `done` (4e.1).
func (s *Service) WakeDependents(ctx context.Context, taskID string) error {
	return s.repo.WakeDependents(ctx, taskID)
}

// BoardBudgetToday reads the N16 cap and today's spend from the aggregate rather
// than summing the ledger (4c).
func (s *Service) BoardBudgetToday(ctx context.Context, orgID, boardID string) (BoardBudget, error) {
	return s.repo.BoardBudgetToday(ctx, orgID, boardID)
}

// BumpDailyRunCount counts one run against the day, once per run rather than per
// step.
func (s *Service) BumpDailyRunCount(ctx context.Context, boardID string) error {
	return s.repo.BumpDailyRunCount(ctx, boardID)
}

// BumpDailyCost adds one step's spend and tokens to the board's day (5.3).
func (s *Service) BumpDailyCost(ctx context.Context, orgID, boardID string, micros, tokensIn, tokensOut int64) error {
	return s.repo.UpsertDailyBoardCost(ctx, orgID, boardID, micros, tokensIn, tokensOut)
}

// RecordRunUsage rolls a step's usage into the run's own totals. Without it
// `runs.cost_micros` stays zero and the per-run figure the UI shows is wrong even
// though the ledger is right.
func (s *Service) RecordRunUsage(ctx context.Context, runID string, micros, tokensIn, tokensOut int64) error {
	return s.repo.RecordRunUsage(ctx, runID, micros, tokensIn, tokensOut)
}

// ReleaseClaim drops a claim whose run never started. See the SQL for why this is
// not the same operation as EndRun: there is no run row to close.
func (s *Service) ReleaseClaim(ctx context.Context, taskID, orgID, runID, failureKind, detail string) error {
	return s.repo.ReleaseClaim(ctx, taskID, orgID, runID, failureKind, detail)
}

// BlockedKind reports which block_kind a failure implies, and whether the task
// should be blocked instead of retried or failed.
//
// ARCHITECTURE §10.2 splits the taxonomy in three: some kinds retry, three are
// terminal-but-recoverable and belong in `blocked` (`needs_input`, `dependency`,
// `policy`, plus `budget`), and the rest fail. The distinction is the contract's,
// not this function's invention — retrying a `policy` refusal spends money to be
// refused again.
func BlockedKind(failureKind string) (string, bool) {
	switch failureKind {
	case "needs_input", "dependency", "policy", "budget":
		return failureKind, true
	}
	return "", false
}

// ------------------------------------------------------------------ repo ----

// The adapter methods below are thin: they exist so the service and the
// repository interface share the vocabulary of the tick loop.

// BlockDependentTasks counts the rows it moved, which is the only signal that a
// board has work stuck behind an unfinished parent.
func (r *pgxRepository) BlockDependentTasks(ctx context.Context, orgID, boardID string) (int64, error) {
	return r.q.ClaimReadyTaskDepsBlocked(ctx, store.ClaimReadyTaskDepsBlockedParams{OrgID: orgID, BoardID: boardID})
}

func (r *pgxRepository) HeartbeatOwnedRuns(ctx context.Context, orgID, lock string) error {
	return r.q.HeartbeatOwnedRuns(ctx, store.HeartbeatOwnedRunsParams{OrgID: orgID, ClaimLock: &lock})
}

// ReclaimStaleRuns returns the task rows that were moved, so the dispatcher can
// log or event them. A reclaim that moved nothing is not an error.
func (r *pgxRepository) ReclaimStaleRuns(ctx context.Context, orgID string, limit, maxAttempts int) ([]Task, error) {
	rows, err := r.q.ReclaimStaleRuns(ctx, store.ReclaimStaleRunsParams{
		OrgID: orgID, Limit: int32(limit), MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, Task{ID: row.ID, BoardID: row.BoardID, Status: TaskStatus(row.Status)})
	}
	return out, nil
}

func (r *pgxRepository) WakeDependents(ctx context.Context, taskID string) error {
	return r.q.WakeDependents(ctx, taskID)
}

func (r *pgxRepository) UpsertDailyBoardCost(ctx context.Context, orgID, boardID string, micros, tokensIn, tokensOut int64) error {
	return r.q.UpsertDailyBoardCost(ctx, store.UpsertDailyBoardCostParams{
		OrgID: orgID, BoardID: boardID, TotalMicros: micros, TokensIn: tokensIn, TokensOut: tokensOut,
	})
}

func (r *pgxRepository) BumpDailyRunCount(ctx context.Context, boardID string) error {
	return r.q.BumpDailyRunCount(ctx, boardID)
}

// BoardBudgetToday maps the row, telling "no board" apart from "no spend".
func (r *pgxRepository) BoardBudgetToday(ctx context.Context, orgID, boardID string) (BoardBudget, error) {
	row, err := r.q.BoardBudgetToday(ctx, store.BoardBudgetTodayParams{ID: boardID, OrgID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardBudget{}, ErrNotFound
	}
	if err != nil {
		return BoardBudget{}, err
	}
	return BoardBudget{
		BoardID:          row.BoardID,
		CapMicros:        row.Cap,
		SpentTodayMicros: row.SpentToday,
		RunCount:         int(row.RunCount),
		Status:           row.BudgetStatus,
	}, nil
}

func (r *pgxRepository) RecordRunUsage(ctx context.Context, runID string, micros, tokensIn, tokensOut int64) error {
	return r.q.RecordRunUsage(ctx, store.RecordRunUsageParams{
		ID: runID, CostMicros: micros, TokensIn: tokensIn, TokensOut: tokensOut,
	})
}

// ReleaseClaim lowers a claim whose run could not start. `runID` is recorded in
// the same shape EndRun uses so the two paths cannot disagree about the outcome;
// the SQL applies the §10.2 verdict itself (needs_input -> blocked, else failed).
func (r *pgxRepository) ReleaseClaim(ctx context.Context, taskID, orgID, runID, failureKind, detail string) error {
	// ReleaseClaim is :one, and a zero-row result is the expected outcome when the
	// task already moved (a peer closed it first). Only a real error is returned.
	_, err := r.q.ReleaseClaim(ctx, store.ReleaseClaimParams{
		TaskID: taskID, OrgID: orgID, FailureKind: failureKind,
	})
	return err
}
