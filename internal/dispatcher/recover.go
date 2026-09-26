package dispatcher

import (
	"context"
	"errors"
	"time"

	"agentdeck/internal/board"
)

// The three ways a claimed task cannot start. Each maps onto a §10 failure kind
// rather than a retry: none of them is fixed by trying again unchanged.
var (
	errNoAgent           = errors.New("task has no assigned agent")
	errArchivedAgent     = errors.New("assigned agent is archived")
	errNoProviderAddress = errors.New("assigned agent has no provider address")
)

// startFailureKind maps a preparation failure onto the §10 taxonomy.
//
// The two `needs_input` cases are the ones a retry cannot fix: nobody assigned an
// agent, or the assignment points at an agent someone retired. §10.2 sends
// `needs_input` to `blocked`, which is what makes the board show "a human has to
// act" instead of silently burning attempts.
func startFailureKind(err error) string {
	switch {
	case errors.Is(err, errNoAgent), errors.Is(err, errArchivedAgent):
		return "needs_input"
	case errors.Is(err, errNoProviderAddress):
		// A capability gap in §10's sense: the agent cannot reach a model. §10.2
		// makes capability terminal (`failed`) rather than blocked.
		return "capability"
	}
	return "unknown"
}

// abortClaim releases a task that was claimed but could not start.
//
// Releasing matters more than reporting: the claim wrote `status='running'` and a
// run id, so a task that fails preparation would otherwise be `running` forever
// with no run row behind it — invisible to reclaim (nothing to reclaim) and
// invisible to the claim predicate (not `ready`). The task is closed as a failed
// run so the normal retry policy decides its fate.
func (d *Dispatcher) abortClaim(ctx context.Context, task board.Task, runID string, cause error) {
	kind := startFailureKind(cause)
	d.log.Warn("dispatcher: task cannot start",
		"task", task.ID, "board", task.BoardID, "kind", kind, "error", cause)

	if err := d.store.ReleaseClaim(ctx, task.ID, task.OrgID, runID, kind, cause.Error()); err != nil {
		d.log.Error("dispatcher: releasing an unstartable claim",
			"task", task.ID, "run", runID, "error", err)
	}
}

// reclaim closes runs that stopped heartbeating (4b) and wakes the dependents of
// anything that finished successfully while we were away.
func (d *Dispatcher) reclaim(ctx context.Context, boards []board.Board) error {
	for _, b := range boards {
		moved, err := d.store.ReclaimStale(ctx, b.OrgID, ReclaimBatch, 0)
		if err != nil {
			return err
		}
		if len(moved) > 0 {
			d.log.Info("dispatcher: reclaimed stale runs", "board", b.ID, "count", len(moved))
		}
	}
	return nil
}

// recoverOnBoot handles the state a crashed instance leaves behind.
//
// This is not the same job as reclaim. Reclaim waits 15 minutes for a heartbeat
// to go quiet, which is right for a *peer* instance that might still be alive, but
// wrong for our own previous process: it is definitively gone, its runs will never
// heartbeat again, and its tasks would sit `running` for a quarter of an hour
// before anything notices. Closing them immediately is the difference between a
// restart costing seconds and costing 15 minutes.
func (d *Dispatcher) recoverOnBoot(ctx context.Context) {
	boards, err := d.store.ClaimableBoards(ctx)
	if err != nil {
		d.log.Error("dispatcher: boot recovery could not list boards", "error", err)
		return
	}
	for _, b := range boards {
		moved, err := d.store.ReclaimStale(ctx, b.OrgID, ReclaimBatch, 0)
		if err != nil {
			d.log.Error("dispatcher: boot recovery", "board", b.ID, "error", err)
			continue
		}
		if len(moved) > 0 {
			d.log.Info("dispatcher: recovered runs left by the previous process",
				"board", b.ID, "count", len(moved))
		}
	}
	d.lastReclaim = time.Now()
}
