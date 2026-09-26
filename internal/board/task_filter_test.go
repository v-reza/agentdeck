package board

import (
	"context"
	"testing"

	"agentdeck/internal/ulid"
)

// US-AD11 / §6.2.16 — the board list's advertised filters.
//
// The contract has always said `GET /boards/{id}/tasks` accepts `status`,
// `assignee` and `search`, and the implementation has always ignored all three:
// the handler called `ListBoardTasks(ctx, org, board)` with no filter at all and
// the UI filtered the returned slice in the browser. That made the documented
// filter a client-side accident — the same board loaded through curl and
// through the app disagreed.
//
// These run against real Postgres because the filter is a WHERE clause. A fake
// repository cannot tell `ILIKE '%x%'` from `= 'x'`, and that distinction is the
// whole feature.
func TestPgListBoardTasksFilters(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)
	boardID, projectID := newBoard(t, ctx, repo, orgID)
	agentID := newAgent(t, ctx, orgID, projectID, "agent-filter")

	// Three tasks that differ along exactly the axes under test.
	alpha := newTask(t, ctx, repo, orgID, boardID, "Fix flaky reclaim", "backlog")
	newTask(t, ctx, repo, orgID, boardID, "Upgrade Postgres schema", "ready")
	gamma := newTask(t, ctx, repo, orgID, boardID, "Reclaim orphaned leases", "ready")
	if _, err := repo.AssignTask(ctx, gamma, orgID, agentID); err != nil {
		t.Fatalf("assign: %v", err)
	}

	t.Run("no filter returns the whole board", func(t *testing.T) {
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("unfiltered = %d tasks, want 3", len(got))
		}
	})

	t.Run("status narrows to one column", func(t *testing.T) {
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Statuses: []TaskStatus{StatusReady}})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("status=ready = %d tasks, want 2 (beta, gamma)", len(got))
		}
		for _, task := range got {
			if task.Status != StatusReady {
				t.Fatalf("status=ready returned a %q task", task.Status)
			}
		}
	})

	t.Run("assignee narrows to one agent", func(t *testing.T) {
		agent := agentID
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Assignee: &agent})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].ID != gamma {
			t.Fatalf("assignee=%s = %v, want just gamma (%s)", agentID, ids(got), gamma)
		}
	})

	t.Run("search matches a substring, not the whole title", func(t *testing.T) {
		// "reclaim" appears in alpha's title and gamma's; a `=` comparison or an
		// anchored match would find neither.
		term := "reclaim"
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Search: &term})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("search=reclaim = %v, want alpha and gamma", ids(got))
		}
	})

	t.Run("search is case insensitive", func(t *testing.T) {
		term := "FLaky"
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Search: &term})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].ID != alpha {
			t.Fatalf("search=FLaky = %v, want just alpha (%s)", ids(got), alpha)
		}
	})

	t.Run("filters compose", func(t *testing.T) {
		term := "reclaim"
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Statuses: []TaskStatus{StatusReady}, Search: &term})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].ID != gamma {
			t.Fatalf("ready+reclaim = %v, want just gamma (%s)", ids(got), gamma)
		}
	})

	t.Run("an unmatched search returns nothing, not everything", func(t *testing.T) {
		// The failure mode this guards is the classic optional-filter bug: a
		// filter that falls through to "no constraint" and quietly returns the
		// whole board instead of an empty one.
		term := "no-such-task-anywhere"
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Search: &term})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("unmatched search = %v, want empty", ids(got))
		}
	})

	t.Run("filters stay inside the tenant", func(t *testing.T) {
		// The filter must not become a way to read across orgs: a second org's
		// board must be invisible even when the search term matches it.
		otherOrg := newOrg(t, ctx)
		otherBoard, _ := newBoard(t, ctx, repo, otherOrg)
		newTask(t, ctx, repo, otherOrg, otherBoard, "Reclaim someone else's leases", "ready")

		term := "reclaim"
		got, err := repo.ListBoardTasks(ctx, orgID, boardID, TaskFilter{Search: &term})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("cross-org search = %v, want only this org's two", ids(got))
		}
	})
}

// ids renders a task list as ids, so a failure names what came back instead of
// just a count.
func ids(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}

// newBoard inserts a project and a board with the default columns.
func newBoard(t *testing.T, ctx context.Context, repo Repository, orgID string) (boardID, projectID string) {
	t.Helper()
	project, err := repo.CreateProject(ctx, Project{
		ID: ulid.Must(), OrgID: orgID, Slug: lower("p-" + ulid.Must()[20:]), Name: "Filter Project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	board, err := repo.CreateBoard(ctx, Board{
		ID: ulid.Must(), OrgID: orgID, ProjectID: project.ID,
		Slug: lower("b-" + ulid.Must()[20:]), Name: "Filter Board", Columns: DefaultColumns,
	})
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	return board.ID, project.ID
}

// newAgent inserts an agent row. A real row is needed rather than a made-up id
// because `tasks_agent_fk` rejects an assignee that does not exist — the same
// constraint that makes the service's org check worth having.
func newAgent(t *testing.T, ctx context.Context, orgID, projectID, name string) string {
	t.Helper()
	id := ulid.Must()
	if _, err := pgSuitePool.Exec(ctx,
		`INSERT INTO agents (id, org_id, project_id, name, provider, model)
		 VALUES ($1, $2, $3, $4, 'openai_compatible', 'gpt-4o')`,
		id, orgID, projectID, name); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	return id
}

// newTask inserts one task and returns its id.
func newTask(t *testing.T, ctx context.Context, repo Repository, orgID, boardID, title string, status TaskStatus) string {
	t.Helper()
	task, err := repo.CreateTask(ctx, Task{
		ID: ulid.Must(), OrgID: orgID, BoardID: boardID, Title: title,
		Status: status, WorkspaceKind: WorkspaceScratch, GoalMode: "auto",
	})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task.ID
}
