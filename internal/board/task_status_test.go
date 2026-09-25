package board

import (
	"context"
	"errors"
	"testing"

	"agentdeck/internal/ulid"
)

// newUser inserts a bare user row: `tasks.created_by` is NOT NULL and the board
// domain never reads the row, so registration is out of scope here. The hash
// just has to satisfy users_password_chk.
func newUser(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := ulid.Must()
	if _, err := pgSuitePool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)",
		id, "mv-"+lower(id[20:])+"@example.test", "Move Suite",
		"$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$0000000000000000000000000000000000000000000"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// TestPgUpdateTaskStatusMovesTheTask pins the parameter order of
// `UpdateTaskStatus` (US-AD12, the board's drag-and-drop).
//
// The generated params are `Status` and `Status_2`, and the SQL is
//
//	SET status = $4 WHERE id = $1 AND org_id = $2 AND status = $3
//
// so `Status` is the GUARD and `Status_2` is the new value — the reverse of what
// the names suggest. Filling them in the intuitive order made `WHERE status =
// <target>` match nothing, so every single move returned zero rows and the
// repository translated that to ErrConflict: a 409 on every column change, with
// no test to notice because this suite had no move coverage at all.
//
// This test exists to make the swap impossible to reintroduce. It needs a live
// database because the bug lives in the query's parameter binding, which a fake
// repository replaces wholesale.
func TestPgUpdateTaskStatusMovesTheTask(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	// A board needs its project (FK) and its columns (boards_columns_chk is in
	// the DDL but the semantic contract lives in validColumns).
	project, err := repo.CreateProject(ctx, Project{
		ID: ulid.Must(), OrgID: orgID, Slug: lower("mvp-" + ulid.Must()[19:]), Name: "Move Project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	boardID := ulid.Must()
	if _, err := repo.CreateBoard(ctx, Board{
		ID: boardID, OrgID: orgID, ProjectID: project.ID,
		Slug: lower("mv-" + ulid.Must()[20:]), Name: "Move Board", Columns: DefaultColumns,
	}); err != nil {
		t.Fatalf("create board: %v", err)
	}

	task, err := repo.CreateTask(ctx, Task{
		ID: ulid.Must(), OrgID: orgID, BoardID: boardID, Title: "Drag me", Status: StatusBacklog,
		CreatedBy: newUser(t, ctx), WorkspaceKind: WorkspaceScratch, GoalMode: "auto",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	moved, err := repo.UpdateTaskStatus(ctx, task.ID, orgID, StatusBacklog, StatusReady)
	if err != nil {
		t.Fatalf("move backlog -> ready: %v (every move returning ErrConflict means the "+
			"guard parameter is carrying the target status)", err)
	}
	if moved.Status != StatusReady {
		t.Fatalf("status after move = %q, want %q", moved.Status, StatusReady)
	}

	// The guard still has to reject a stale move: the task is `ready` now, so
	// claiming it is still `backlog` must conflict rather than overwrite.
	if _, err := repo.UpdateTaskStatus(ctx, task.ID, orgID, StatusBacklog, StatusRunning); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale move error = %v, want ErrConflict", err)
	}

	// The optimistic guard reports a conflict; it must not have written anything.
	after, err := repo.GetTask(ctx, task.ID, orgID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if after.Status != StatusReady {
		t.Fatalf("status after rejected move = %q, want %q (the failed move wrote anyway)",
			after.Status, StatusReady)
	}
}
