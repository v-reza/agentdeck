-- Invariant: a task that is not `running` must not hold a run binding.
--
-- The bug this closes (ARCHITECTURE 4b). `ClaimReadyTasks` requires
-- `tasks.current_run_id IS NULL`. The column is written by claiming and cleared
-- by EndRun, ReleaseClaim, ReclaimStaleRuns, and the approval path. One writer
-- did not clear it: `UpdateTaskStatus`, which serves PATCH status,
-- `backlog -> ready`, and `awaiting_approval -> ready` (the approve path).
--
-- A task moved back to `ready` kept the id of a run that had already ended, and
-- from then on matched zero rows in the claim predicate forever. It is not
-- `running`, so reclaim never looks at it; it is not `blocked`, so the board
-- shows it as queued work; and the API has no way to clear the column. The only
-- exit was editing the database. Found on a live board: a task retried after a
-- `policy` failure sat in Ready with a running dispatcher and was never picked
-- up.
--
-- The SQL is fixed in queries.sql. This constraint is the part that makes the
-- invariant hold for writers that do not exist yet — the next `SET status = ...`
-- statement added by someone who has not read that comment is refused by
-- Postgres instead of silently stranding a task. The repo's gate for this class
-- is the database, not a review.
--
-- Existing rows are repaired first. On a fresh database this updates nothing,
-- but a deployment that already hit the bug carries tasks that are otherwise
-- unreachable by any API call.
UPDATE tasks
SET current_run_id = NULL
WHERE status <> 'running' AND current_run_id IS NOT NULL;

-- DROP then ADD, not ADD alone: `Apply` repeats migrations over an already-populated
-- schema on the repair path (TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema),
-- and Postgres has no `ADD CONSTRAINT IF NOT EXISTS`. Re-adding the same constraint
-- is a no-op, and the UPDATE above has already repaired every row the old one
-- would reject.
ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_run_binding_chk;

ALTER TABLE tasks
    ADD CONSTRAINT tasks_run_binding_chk
    CHECK (status = 'running' OR current_run_id IS NULL);
