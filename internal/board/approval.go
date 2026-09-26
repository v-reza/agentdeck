package board

// Approval gate — ARCHITECTURE 3.13, 5.2 phase 3e, 5.3, 8.3/8.4, 6.2.14.
//
// The gate exists so a human sees what an agent proposes before the workspace
// pays for it. Three contract conflicts had to be settled to write this, and
// each is decided here rather than deferred:
//
//  1. **Where a decided gate sends the task.** PRD US-AD34 AC1 says `running`,
//     US-AD35 AC1 says `blocked(needs_input)`. ARCHITECTURE's state machine
//     (5.4 line 1598) and 5.3 line 201 both say approve -> `ready`, reject ->
//     `blocked(policy)`. ARCHITECTURE wins: it is the newer document, it is the
//     one the transition table is generated from, and the PRD's `running` is
//     unreachable — the run was already ended with `outcome=failed` when the
//     gate was raised, so nothing is running to go back to. `needs_input` is
//     also the wrong block kind: the approver did not ask for more information,
//     they refused.
//
//  2. **Who may decide.** PRD US-AD34 AC3 allows `member`; ARCHITECTURE 11.3's
//     RBAC matrix allows owner/admin only. The matrix wins, and not merely
//     because it is newer: `POST /tasks/{id}/approvals` is Member-gated, so
//     letting members decide would let one person raise a gate and clear it
//     themselves. A gate a member can open and close alone is not a gate.
//     US-AD35 AC4 already says reject is owner/admin, so the PRD contradicts
//     itself between the two ACs.
//
//  3. **Which run an approval belongs to.** `approvals.run_id` is NOT NULL and
//     FK-bound to `runs`. A task whose run was never started (claimed manually
//     but not yet executed) has no run to point at, so the request is refused
//     rather than stored against a fabricated run id.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentdeck/internal/ulid"
)

// ApprovalGateRequire is the only gate mode this module creates. `auto` means
// "run without asking" (nothing to store) and `deny` means "never run" (the
// dispatcher fails the task); both are dispatcher decisions, not approval rows.
const ApprovalGateRequire = "require"

// ApprovalWindow is N23. Kept as a named constant so the test that proves the
// expiry sweep works does not have to re-derive "24 hours" from prose.
const ApprovalWindow = 24 * time.Hour

// RequestApproval implements POST /tasks/{id}/approvals (US-AD33).
//
// `requestedBy` is the caller's identity, recorded so an approver can see who
// asked. It is not used for authorization: the route's role gate is.
func (s *Service) RequestApproval(ctx context.Context, taskID, orgID, requestedBy string, previewJSON []byte, reason string) (Approval, error) {
	task, err := s.repo.GetTask(ctx, taskID, orgID)
	if err != nil {
		return Approval{}, err
	}
	// US-AD33 AC2. Checked on the parsed bytes, not on `len(raw)`: `{}` and
	// `null` are both non-empty payloads that show the approver nothing, and a
	// gate whose preview is empty is a button with no label.
	if len(previewJSON) == 0 || !hasContent(previewJSON) {
		return Approval{}, ErrApprovalPreviewRequired
	}

	// A gate is raised against the run the work is happening in. Without one
	// there is no work to pause, and `approvals.run_id` would have to be
	// invented — a row pointing at a run that does not exist.
	// `approvals.run_id` is NOT NULL and FK-bound to `runs`, so a gate without a
	// live run cannot be stored. This happens legitimately: a task already parked
	// in `awaiting_approval` has had its run closed, so a second gate has nothing
	// to attach to until the task is claimed again. Refused as invalid input
	// rather than failing later on an empty run id (which would read as a 404 for
	// a task that plainly exists).
	if task.CurrentRunID == "" {
		return Approval{}, ErrInvalidInput
	}
	run, err := s.repo.GetRun(ctx, task.CurrentRunID, orgID)
	if err != nil {
		return Approval{}, err
	}

	approval := Approval{
		ID:          ulid.Must(),
		OrgID:       orgID,
		TaskID:      task.ID,
		RunID:       run.ID,
		RequestedBy: requestedBy,
		Decision:    ApprovalPending,
		GateMode:    ApprovalGateRequire,
		Reason:      strings.TrimSpace(reason),
		PreviewJSON: previewJSON,
		ExpiresAt:   time.Now().Add(ApprovalWindow),
	}
	stored, err := s.repo.CreateApproval(ctx, approval)
	if err != nil {
		return Approval{}, err
	}

	// Park the task, then close the run — in that order, and without going
	// through `Service.EndRun`.
	//
	// `Service.EndRun` calls `applyOutcome`, which decides the task's next status
	// from the run's outcome (5.3): `failed` consults the retry policy and can
	// re-queue the task, `budget_exceeded` blocks it, `succeeded` sends it to
	// `review`. None of those is the right destination here. The destination for
	// a gated task is the gate, and letting `applyOutcome` pick first means
	// fighting it afterwards — whichever order is chosen, one of the two writes
	// wins and the other is silently discarded.
	//
	// So the approval path owns this transition and closes the run with the
	// repository call, which is what `EndRun` uses underneath anyway.
	current, err := s.repo.GetTask(ctx, task.ID, orgID)
	if err != nil {
		return Approval{}, err
	}
	if current.Status != StatusAwaitingApproval {
		// `from` is the status just read, so this is a compare-and-set: a
		// concurrent writer loses here instead of being overwritten.
		if _, err := s.MoveTask(ctx, task.ID, orgID, current.Status, StatusAwaitingApproval); err != nil {
			return Approval{}, err
		}
	}

	// A gate is a stop, so the run is closed rather than left `running`: a run
	// nothing is executing would be reclaimed as stale 15 minutes later (N8) and
	// retried — spending money on work a human has not approved.
	//
	// `budget_exceeded` is the outcome, and the choice is deliberate. The run
	// stopped because a human has to decide, which is the same shape as the cap
	// case: work that must not continue until something outside the run changes.
	// `failed` would be read as "the work is broken", and a reader of
	// `runs.outcome` cannot see the gate from there.
	if run.Status == RunRunning {
		summary := RunSummary{
			Outcome:     "budget_exceeded",
			FailureKind: "budget",
			Error:       "run paused for human approval",
		}
		if _, err := s.repo.EndRun(ctx, run.ID, orgID, summary); err != nil {
			return Approval{}, err
		}
	}
	// Release the claim binding in every case. The claim predicate is
	// `current_run_id IS NULL` (4b), so a task still pointing at this run could
	// never be claimed again — and approving the gate would send it to `ready`
	// where the dispatcher would silently never pick it up.
	if err := s.repo.ClearTaskCurrentRun(ctx, task.ID, orgID); err != nil {
		return Approval{}, err
	}

	if _, err := s.RecordRunEvent(ctx, orgID, task.BoardID, task.ID, run.ID, "approval.requested", map[string]any{
		"approval_id": stored.ID,
		"gate_mode":   stored.GateMode,
		"expires_at":  stored.ExpiresAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return Approval{}, err
	}
	return stored, nil
}

// hasContent reports whether a JSON payload says anything. `{}`, `null`, `[]`
// and a bare string are all "nothing to preview": they parse, so a length check
// passes them, and an approver looking at `{}` has no more information than one
// looking at an empty box.
func hasContent(raw []byte) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		// Not valid JSON. The handler rejects that before calling here; if it
		// somehow arrives, it is not a preview.
		return false
	}
	switch typed := value.(type) {
	case nil:
		return false
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

// ApprovalInbox implements GET /approvals (6.2.14): pending and not yet expired.
func (s *Service) ApprovalInbox(ctx context.Context, orgID string) ([]Approval, error) {
	return s.repo.ListPendingApprovals(ctx, orgID)
}

// GetApproval implements GET /approvals/{id}.
func (s *Service) GetApproval(ctx context.Context, id, orgID string) (Approval, error) {
	return s.repo.GetApproval(ctx, id, orgID)
}

// TaskApprovals implements the per-task history used by the run trace.
func (s *Service) TaskApprovals(ctx context.Context, taskID, orgID string) ([]Approval, error) {
	return s.repo.ListTaskApprovals(ctx, taskID, orgID)
}

// Approve implements POST /approvals/{id}/approve (US-AD34).
//
// Idempotent by design at the storage layer (8.4): the UPDATE only fires on
// `decision = 'pending'`, so two approvers racing produce one winner and one
// zero-row result. A second call on an already-decided gate is a 409
// (ErrApprovalDecided) — the decision is a fact about what a human approved, and
// overwriting it would rewrite the audit trail.
func (s *Service) Approve(ctx context.Context, id, orgID, decidedBy string) (Approval, error) {
	return s.decide(ctx, id, orgID, decidedBy, ApprovalApproved, "", StatusReady)
}

// Reject implements POST /approvals/{id}/reject (US-AD35).
//
// US-AD35 AC2: a reason is mandatory. A rejection with no explanation leaves the
// worker — and whoever retries the task — with nothing to act on.
func (s *Service) Reject(ctx context.Context, id, orgID, decidedBy, reason string) (Approval, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Approval{}, ErrInvalidInput
	}
	return s.decide(ctx, id, orgID, decidedBy, ApprovalRejected, reason, StatusBlocked)
}

// decide is the shared half of approve/reject: one transition, one event, one
// task move. The only differences are the decision value, the reason, and where
// the task lands.
func (s *Service) decide(ctx context.Context, id, orgID, decidedBy string, decision ApprovalDecision, reason string, to TaskStatus) (Approval, error) {
	approval, err := s.repo.GetApproval(ctx, id, orgID)
	if err != nil {
		return Approval{}, err
	}
	// Read first, then compare: this is what lets "already decided" (409) be
	// told apart from "does not exist" (404). The write below still carries its
	// own `decision = 'pending'` predicate, so the read is not the guard — it is
	// only the classification.
	if approval.Decision != ApprovalPending {
		return Approval{}, ErrApprovalDecided
	}

	moved, err := s.repo.DecideApproval(ctx, id, orgID, decision, decidedBy, reason)
	if err != nil {
		return Approval{}, err
	}
	if !moved {
		// Lost the race between the read above and the write. Same fact, same
		// answer: somebody decided this first.
		return Approval{}, ErrApprovalDecided
	}

	task, err := s.repo.GetTask(ctx, approval.TaskID, orgID)
	if err != nil {
		return Approval{}, err
	}

	if _, err := s.RecordRunEvent(ctx, orgID, task.BoardID, task.ID, approval.RunID, "approval.decided", map[string]any{
		"approval_id": approval.ID,
		"decision":    string(decision),
		"decided_by":  decidedBy,
	}); err != nil {
		return Approval{}, err
	}

	// US-AD35 AC1 (reject -> blocked) needs a block kind, and the kind is
	// `policy` per 5.4: the work was refused by a rule the workspace set, which
	// is exactly what a rejected gate is. Approve sends the task back to `ready`
	// so the dispatcher claims it again with a fresh run — the approved action is
	// not resumed in place, because the run that proposed it has ended.
	// `BlockTask` writes the status and the kind in one statement: two writes
	// would leave a window where the task is `blocked` with the previous kind
	// still attached, and the board renders a blocked task by its kind.
	if to == StatusBlocked {
		if err := s.repo.BlockTask(ctx, task.ID, orgID, "policy"); err != nil {
			return Approval{}, err
		}
	} else if task.Status != to {
		if _, err := s.MoveTask(ctx, task.ID, orgID, task.Status, to); err != nil {
			return Approval{}, err
		}
	}

	// Re-read so the caller sees what was stored (decided_by, decided_at,
	// decision) rather than the pre-decision snapshot it was handed above.
	return s.repo.GetApproval(ctx, id, orgID)
}

// ExpireDueApprovals implements the N23 sweep from 8.3. The dispatcher calls it
// each tick; it returns the gates it closed so the caller can log them.
//
// This is a system action, not a user one: there is no handler for it, because
// "expire what is due" is not something a client should be able to trigger for
// an arbitrary moment.
func (s *Service) ExpireDueApprovals(ctx context.Context, orgID string) ([]Approval, error) {
	expired, err := s.repo.ExpireApprovals(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, approval := range expired {
		task, err := s.repo.GetTask(ctx, approval.TaskID, orgID)
		if err != nil {
			// The task is gone (archived and cleaned). The approval is already
			// marked expired, which is the part that matters; there is nothing
			// left to move.
			continue
		}
		if _, err := s.RecordRunEvent(ctx, orgID, task.BoardID, task.ID, approval.RunID, "approval.expired", map[string]any{
			"approval_id": approval.ID,
		}); err != nil {
			return nil, err
		}
		// 5.4 / 8.3: an unanswered gate parks the task as `blocked(policy)`.
		if task.Status == StatusAwaitingApproval {
			if err := s.repo.BlockTask(ctx, task.ID, orgID, "policy"); err != nil {
				return nil, err
			}
		}
	}
	return expired, nil
}
