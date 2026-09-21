package board

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"agentdeck/internal/store"
	"agentdeck/internal/ulid"
)

// ErrConflict is returned when a lifecycle transition does not match the
// expected current status, which is the dispatcher's race signal: two writers
// racing on one task produce one winner and one ErrConflict, not a double move.
var ErrConflict = errors.New("task is not in the expected state")

// pgxRepository is the production Repository for the board domain. Every write
// lands in Postgres and survives a restart (ARCHITECTURE P4); queries are the
// sqlc-generated bindings (P2), and every org-scoped query carries its org_id
// parameter explicitly so no code path can forget the tenant scope.
type pgxRepository struct {
	q *store.Queries
}

// NewPgxRepository builds the Postgres-backed Repository.
func NewPgxRepository(pool store.DBTX) Repository {
	return &pgxRepository{q: store.New(pool)}
}

// nullString maps a Go "" to SQL NULL for optional text. The column is NULL
// when unset, so a task with no idempotency key stores NULL, not an empty
// string that would collide with the tasks_idempotency_uniq constraint.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ts converts a nullable Postgres timestamp to a pointer, the shape the domain
// uses so "no value" is unambiguous instead of a zero time.
// boolOf flattens the nullable generated column agents.has_provider_key into a
// plain bool. It is NULL only for a row that predates the column, which the
// migration backfills; false is the honest answer for "we do not know".
func boolOf(b *bool) bool {
	return b != nil && *b
}

func ts(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// columns decodes a board's columns_json. A malformed array is impossible: the
// DDL CHECKs jsonb_typeof(columns_json) = 'array', and the only writer is
// UpdateBoardColumns, which re-encodes the []Column it just validated. A decode
// failure here means the row is corrupted, so it is an error, not a fallback to
// the default columns, which would silently hide a corrupt board.
func columns(raw []byte) ([]Column, error) {
	var cols []Column
	if err := json.Unmarshal(raw, &cols); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return DefaultColumns, nil
	}
	return cols, nil
}

func encodeColumns(cols []Column) ([]byte, error) {
	return json.Marshal(cols)
}

func projectRow(r store.Project) Project {
	return Project{
		ID:        r.ID,
		OrgID:     r.OrgID,
		Slug:      r.Slug,
		Name:      r.Name,
		CreatedAt: r.CreatedAt.Time,
	}
}

func boardRow(r store.Board) (Board, error) {
	cols, err := columns(r.ColumnsJson)
	if err != nil {
		return Board{}, err
	}
	return Board{
		ID:                r.ID,
		OrgID:             r.OrgID,
		ProjectID:         r.ProjectID,
		Slug:              r.Slug,
		Name:              r.Name,
		Columns:           cols,
		BudgetDailyMicros: r.BudgetDailyMicros,
		CreatedAt:         r.CreatedAt.Time,
	}, nil
}

func taskRow(r store.Task) Task {
	return Task{
		ID:                  r.ID,
		OrgID:               r.OrgID,
		BoardID:             r.BoardID,
		Title:               r.Title,
		Body:                r.Body,
		Status:              TaskStatus(r.Status),
		Priority:            int(r.Priority),
		AssigneeAgentID:     str(r.AssigneeAgentID),
		CreatedBy:           r.CreatedBy,
		IdempotencyKey:      str(r.IdempotencyKey),
		BlockKind:           str(r.BlockKind),
		ConsecutiveFailures: int(r.ConsecutiveFailures),
		WorkspaceKind:       WorkspaceKind(r.WorkspaceKind),
		WorkspacePath:       str(r.WorkspacePath),
		BranchName:          str(r.BranchName),
		CompletionContract:  str(r.CompletionContract),
		GoalMode:            r.GoalMode,
		GoalMaxTurns:        int(r.GoalMaxTurns),
		CurrentRunID:        str(r.CurrentRunID),
		CostMicros:          r.CostMicros,
		TokensIn:            r.TokensIn,
		TokensOut:           r.TokensOut,
		CreatedAt:           r.CreatedAt.Time,
		StartedAt:           ts(r.StartedAt),
		CompletedAt:         ts(r.CompletedAt),
		ArchivedAt:          ts(r.ArchivedAt),
	}
}

func eventRow(r store.Event) Event {
	return Event{
		ID:          r.ID,
		OrgID:       r.OrgID,
		BoardID:     str(r.BoardID),
		TaskID:      str(r.TaskID),
		RunID:       str(r.RunID),
		Kind:        r.Kind,
		PayloadJSON: r.PayloadJson,
		CreatedAt:   r.CreatedAt.Time,
	}
}

// uniqueViolation reports whether err is a Postgres unique-violation
// (SQLSTATE 23505) raised by the named constraint. The constraint name matters:
// a slug collision and, say, a board-name collision are different facts, and
// mapping the wrong one would report "slug is already taken" for a constraint
// the caller never touched.
func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// projectSlugTakenConstraint is the DDL index that makes a slug unique per org
// (ARCHITECTURE 3.5). Uniqueness is per org on purpose: two tenants may each
// own a project called "control-plane".
const projectSlugTakenConstraint = "projects_org_slug_key"

// boardSlugTakenConstraint is the same idea one level down: boards are unique
// per project, not per org (ARCHITECTURE 3.6).
const boardSlugTakenConstraint = "boards_project_slug_key"

// slugTakenError maps a unique violation on projects_org_slug_key to
// ErrSlugTaken, which writeBoardError answers as 409 (US-AD08 AC2). Postgres is
// the arbiter of uniqueness — a pre-flight SELECT would still race two
// concurrent creates — so the mapping happens on the way out of the write.
// Every other error, including nil, is returned unchanged.
func slugTakenError(err error) error {
	if uniqueViolation(err, projectSlugTakenConstraint) {
		return ErrSlugTaken
	}
	return err
}

// boardSlugTakenError is the same mapping one level down (US-AD09 AC2).
func boardSlugTakenError(err error) error {
	if uniqueViolation(err, boardSlugTakenConstraint) {
		return ErrSlugTaken
	}
	return err
}

// boardNameTakenConstraint is the DDL index added in migration 0007. US-AD09 AC2
// names the *name*, not the slug: "Nama board duplikat dalam satu project
// mengembalikan 409". agents already carried the equivalent index for its own
// duplicate-name criterion; boards were missing it, so a second "Sprint 24" with
// a fresh slug returned 201 until 0007 landed.
const boardNameTakenConstraint = "boards_project_name_key"

// boardNameTakenError maps a unique violation on boards_project_name_key to
// ErrNameTaken. Postgres is the arbiter for the same reason as the slug: a
// pre-flight SELECT races two concurrent creates.
func boardNameTakenError(err error) error {
	if uniqueViolation(err, boardNameTakenConstraint) {
		return ErrNameTaken
	}
	return err
}

// missingParentConstraint is the foreign key a board carries to its project.
// boards_project_fk also has no org_id in its key, so without an explicit check
// it permits a board whose project lives in another tenant (see
// CreateBoardWithProjectCheck).
const missingParentConstraint = "boards_project_fk"

// missingParentError maps a foreign-key violation on boards_project_fk to
// ErrNotFound, which writeBoardError answers as 404. A caller naming a project
// that does not exist is a user mistake; the handler's default branch reported it
// as `500 ERROR: insert or update on table "boards" violates foreign key
// constraint "boards_project_fk"`, which is both the wrong status and a raw
// SQLSTATE leaked to the client. Another foreign key is left untouched: it would
// be a different bug and must not be disguised as "not found".
func missingParentError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == missingParentConstraint {
		return ErrNotFound
	}
	return err
}

// noRowsError maps pgx.ErrNoRows to ErrNotFound. A `SELECT ... WHERE id = $1 AND
// org_id = $2` returns no rows for an absent id *and* for an id owned by another
// tenant, and US-AD07 requires those two to be indistinguishable — both are 404.
// Without this mapping the handler's default branch turns both into a 500, so a
// stale link or a cross-tenant probe reports a server fault. The sentinel arrives
// wrapped from the query helpers, so the check is errors.Is rather than ==.
func noRowsError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ---- projects -------------------------------------------------------------

func (r *pgxRepository) CreateProject(ctx context.Context, p Project) (Project, error) {
	row, err := r.q.CreateProject(ctx, store.CreateProjectParams{
		ID:    p.ID,
		OrgID: p.OrgID,
		Slug:  p.Slug,
		Name:  p.Name,
	})
	if err != nil {
		return Project{}, slugTakenError(err)
	}
	return projectRow(row), nil
}

func (r *pgxRepository) GetProject(ctx context.Context, id, orgID string) (Project, error) {
	row, err := r.q.GetProject(ctx, store.GetProjectParams{ID: id, OrgID: orgID})
	if err != nil {
		return Project{}, noRowsError(err)
	}
	return projectRow(row), nil
}

func (r *pgxRepository) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	rows, err := r.q.ListProjects(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectRow(row))
	}
	return out, nil
}

func (r *pgxRepository) UpdateProjectName(ctx context.Context, id, orgID, name string) error {
	return r.q.UpdateProjectName(ctx, store.UpdateProjectNameParams{ID: id, OrgID: orgID, Name: name})
}

func (r *pgxRepository) DeleteProject(ctx context.Context, id, orgID string) error {
	return r.q.DeleteProject(ctx, store.DeleteProjectParams{ID: id, OrgID: orgID})
}

// ---- boards ---------------------------------------------------------------

func (r *pgxRepository) CreateBoard(ctx context.Context, b Board) (Board, error) {
	cols, err := encodeColumns(b.Columns)
	if err != nil {
		return Board{}, err
	}
	row, err := r.q.CreateBoard(ctx, store.CreateBoardParams{
		ID:                b.ID,
		OrgID:             b.OrgID,
		ProjectID:         b.ProjectID,
		Slug:              b.Slug,
		Name:              b.Name,
		ColumnsJson:       cols,
		BudgetDailyMicros: b.BudgetDailyMicros,
	})
	if err != nil {
		return Board{}, missingParentError(boardNameTakenError(boardSlugTakenError(err)))
	}
	return boardRow(row)
}

func (r *pgxRepository) GetBoard(ctx context.Context, id, orgID string) (Board, error) {
	row, err := r.q.GetBoard(ctx, store.GetBoardParams{ID: id, OrgID: orgID})
	if err != nil {
		return Board{}, noRowsError(err)
	}
	return boardRow(row)
}

func (r *pgxRepository) ListBoards(ctx context.Context, orgID, projectID string) ([]Board, error) {
	rows, err := r.q.ListBoards(ctx, store.ListBoardsParams{OrgID: orgID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, row := range rows {
		board, err := boardRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, board)
	}
	return out, nil
}

func (r *pgxRepository) UpdateBoardColumns(ctx context.Context, id, orgID string, cols []Column) error {
	raw, err := encodeColumns(cols)
	if err != nil {
		return err
	}
	return r.q.UpdateBoardColumns(ctx, store.UpdateBoardColumnsParams{ID: id, OrgID: orgID, ColumnsJson: raw})
}

func (r *pgxRepository) UpdateBoardName(ctx context.Context, id, orgID, name string) error {
	return r.q.UpdateBoardName(ctx, store.UpdateBoardNameParams{ID: id, OrgID: orgID, Name: name})
}

func (r *pgxRepository) UpdateBoardBudget(ctx context.Context, id, orgID string, budgetMicros int64) error {
	return r.q.UpdateBoardBudget(ctx, store.UpdateBoardBudgetParams{ID: id, OrgID: orgID, BudgetDailyMicros: budgetMicros})
}

func (r *pgxRepository) DeleteBoard(ctx context.Context, id, orgID string) error {
	return r.q.DeleteBoard(ctx, store.DeleteBoardParams{ID: id, OrgID: orgID})
}

func (r *pgxRepository) CountBoardsInProject(ctx context.Context, orgID, projectID string) (int, error) {
	c, err := r.q.CountBoardsInProject(ctx, store.CountBoardsInProjectParams{OrgID: orgID, ProjectID: projectID})
	return int(c), err
}

// ---- tasks ----------------------------------------------------------------

func (r *pgxRepository) CreateTask(ctx context.Context, t Task) (Task, error) {
	row, err := r.q.CreateTask(ctx, store.CreateTaskParams{
		ID:              t.ID,
		OrgID:           t.OrgID,
		BoardID:         t.BoardID,
		Title:           t.Title,
		Body:            t.Body,
		Status:          string(t.Status),
		Priority:        int16(t.Priority),
		AssigneeAgentID: nullString(t.AssigneeAgentID),
		CreatedBy:       t.CreatedBy,
		IdempotencyKey:  nullString(t.IdempotencyKey),
		WorkspaceKind:   string(t.WorkspaceKind),
		GoalMode:        t.GoalMode,
		GoalMaxTurns:    int32(t.GoalMaxTurns),
	})
	if err != nil {
		return Task{}, err
	}
	return taskRow(row), nil
}

func (r *pgxRepository) GetTask(ctx context.Context, id, orgID string) (Task, error) {
	row, err := r.q.GetTask(ctx, store.GetTaskParams{ID: id, OrgID: orgID})
	if err != nil {
		return Task{}, err
	}
	return taskRow(row), nil
}

func (r *pgxRepository) ListBoardTasks(ctx context.Context, orgID, boardID string) ([]Task, error) {
	rows, err := r.q.ListBoardTasks(ctx, store.ListBoardTasksParams{OrgID: orgID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskRow(row))
	}
	return out, nil
}

// UpdateTaskStatus applies one lifecycle transition. The WHERE clause includes
// the expected current status, so a row that already moved returns no row and
// this reports ErrConflict instead of reporting a stale task as current.
func (r *pgxRepository) UpdateTaskStatus(ctx context.Context, id, orgID string, from, to TaskStatus) (Task, error) {
	row, err := r.q.UpdateTaskStatus(ctx, store.UpdateTaskStatusParams{
		ID:       id,
		OrgID:    orgID,
		Status:   string(to),
		Status_2: string(from),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrConflict
		}
		return Task{}, err
	}
	return taskRow(row), nil
}

func (r *pgxRepository) AssignTask(ctx context.Context, id, orgID, agentID string) (Task, error) {
	row, err := r.q.AssignTask(ctx, store.AssignTaskParams{
		ID:              id,
		OrgID:           orgID,
		AssigneeAgentID: nullString(agentID),
	})
	if err != nil {
		return Task{}, err
	}
	return taskRow(row), nil
}

func (r *pgxRepository) UpdateTaskFields(ctx context.Context, id, orgID, title, body string, priority int) (Task, error) {
	row, err := r.q.UpdateTaskFields(ctx, store.UpdateTaskFieldsParams{
		ID:       id,
		OrgID:    orgID,
		Title:    title,
		Body:     body,
		Priority: int16(priority),
	})
	if err != nil {
		return Task{}, err
	}
	return taskRow(row), nil
}

func (r *pgxRepository) DeleteTask(ctx context.Context, id, orgID string) error {
	return r.q.DeleteTask(ctx, store.DeleteTaskParams{ID: id, OrgID: orgID})
}

// ClaimReadyTasks is the atomic dispatcher claim. The CTE locks a bounded batch
// of ready tasks with FOR UPDATE SKIP LOCKED then flips them to running in the
// same statement, so concurrent dispatchers claim disjoint sets and a task is
// never claimed twice (ARCHITECTURE 4b).
func (r *pgxRepository) ClaimReadyTasks(ctx context.Context, orgID, boardID, runID string, limit int) ([]Task, error) {
	rows, err := r.q.ClaimReadyTasks(ctx, store.ClaimReadyTasksParams{
		OrgID:        orgID,
		BoardID:      boardID,
		Limit:        int32(limit),
		CurrentRunID: nullString(runID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskRow(row))
	}
	return out, nil
}

// ---- dependency DAG --------------------------------------------------------

func (r *pgxRepository) CreateTaskLink(ctx context.Context, parentID, childID string) error {
	return r.q.CreateTaskLink(ctx, store.CreateTaskLinkParams{ParentID: parentID, ChildID: childID})
}

func (r *pgxRepository) DeleteTaskLink(ctx context.Context, parentID, childID string) error {
	return r.q.DeleteTaskLink(ctx, store.DeleteTaskLinkParams{ParentID: parentID, ChildID: childID})
}

func (r *pgxRepository) ListTaskParents(ctx context.Context, taskID string) ([]TaskLink, error) {
	rows, err := r.q.ListTaskParents(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskLink, 0, len(rows))
	for _, row := range rows {
		out = append(out, TaskLink{ParentID: row.ParentID})
	}
	return out, nil
}

func (r *pgxRepository) ListTaskChildren(ctx context.Context, taskID string) ([]TaskLink, error) {
	rows, err := r.q.ListTaskChildren(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskLink, 0, len(rows))
	for _, row := range rows {
		out = append(out, TaskLink{ChildID: row.ChildID})
	}
	return out, nil
}

func (r *pgxRepository) CountUnfinishedParents(ctx context.Context, taskID string) (int, error) {
	c, err := r.q.CountUnfinishedParents(ctx, taskID)
	return int(c), err
}

// ---- events ----------------------------------------------------------------

func (r *pgxRepository) CreateEvent(ctx context.Context, e Event) (Event, error) {
	row, err := r.q.CreateEvent(ctx, store.CreateEventParams{
		OrgID:       e.OrgID,
		BoardID:     nullString(e.BoardID),
		TaskID:      nullString(e.TaskID),
		RunID:       nullString(e.RunID),
		Kind:        e.Kind,
		PayloadJson: e.PayloadJSON,
	})
	if err != nil {
		return Event{}, err
	}
	return eventRow(row), nil
}

func (r *pgxRepository) ListTaskEvents(ctx context.Context, taskID string) ([]Event, error) {
	rows, err := r.q.ListTaskEvents(ctx, nullString(taskID))
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventRow(row))
	}
	return out, nil
}

func (r *pgxRepository) ListBoardEventsAfter(ctx context.Context, boardID, orgID string, afterID int64, limit int) ([]Event, error) {
	rows, err := r.q.ListBoardEventsAfter(ctx, store.ListBoardEventsAfterParams{
		BoardID: nullString(boardID),
		OrgID:   orgID,
		ID:      afterID,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventRow(row))
	}
	return out, nil
}

// Compile-time assertion: the pgx repository satisfies the interface.
var _ Repository = (*pgxRepository)(nil)

// ---- agents (US-AD20) -------------------------------------------------------

// agentNameTakenConstraint is the DDL index that makes an agent name unique
// inside one project (ARCHITECTURE 3.7). Postgres is the arbiter, as with
// boards: a pre-flight SELECT would race two concurrent creates.
const agentNameTakenConstraint = "agents_project_name_key"

// agentNameTakenError maps a unique violation on agents_project_name_key to
// ErrAgentNameTaken, which writeBoardError answers as 409 (US-AD20 AC3).
func agentNameTakenError(err error) error {
	if uniqueViolation(err, agentNameTakenConstraint) {
		return ErrAgentNameTaken
	}
	return err
}

func (r *pgxRepository) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	row, err := r.q.CreateAgent(ctx, store.CreateAgentParams{
		ID:                a.ID,
		OrgID:             a.OrgID,
		ProjectID:         a.ProjectID,
		Name:              a.Name,
		Provider:          a.Provider,
		Model:             a.Model,
		ReasoningEffort:   a.ReasoningEffort,
		SkillsJson:        a.SkillsJSON,
		ToolsJson:         a.ToolsJSON,
		MaxRuntimeSeconds: int32(a.MaxRuntimeSeconds),
		RetryPolicy:       a.RetryPolicy,
		MaxAttempts:       int32(a.MaxAttempts),
		BaseUrl:           nullString(a.BaseURL),
	})
	if err != nil {
		return Agent{}, agentNameTakenError(err)
	}
	return agentFromCreate(row), nil
}

func (r *pgxRepository) GetAgent(ctx context.Context, id, orgID string) (Agent, error) {
	row, err := r.q.GetAgent(ctx, store.GetAgentParams{ID: id, OrgID: orgID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, ErrNotFound
		}
		return Agent{}, err
	}
	return agentFromGet(row), nil
}

func (r *pgxRepository) ListAgents(ctx context.Context, orgID, projectID string) ([]Agent, error) {
	rows, err := r.q.ListAgents(ctx, store.ListAgentsParams{OrgID: orgID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentFromList(row))
	}
	return out, nil
}

func (r *pgxRepository) DeleteAgent(ctx context.Context, id, orgID string) error {
	return r.q.DeleteAgent(ctx, store.DeleteAgentParams{ID: id, OrgID: orgID})
}

func (r *pgxRepository) CountAgentRunningTasks(ctx context.Context, id, orgID string) (int, error) {
	n, err := r.q.CountAgentRunningTasks(ctx, store.CountAgentRunningTasksParams{AssigneeAgentID: nullString(id), OrgID: orgID})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// UpdateAgent writes every mutable column at once (US-AD96, US-AD106). Postgres
// is the arbiter of the name uniqueness it owns: agents_project_name_key, mapped
// to ErrAgentNameTaken -> 409, exactly as on create. The provider/base_url pair
// is pre-checked in the service so a mismatch is a 400 instead of a raw CHECK
// violation surfacing as a 500.
func (r *pgxRepository) UpdateAgent(ctx context.Context, a Agent) (Agent, error) {
	row, err := r.q.UpdateAgent(ctx, store.UpdateAgentParams{
		ID:                a.ID,
		OrgID:             a.OrgID,
		Name:              a.Name,
		Provider:          a.Provider,
		Model:             a.Model,
		ReasoningEffort:   a.ReasoningEffort,
		SkillsJson:        a.SkillsJSON,
		ToolsJson:         a.ToolsJSON,
		MaxRuntimeSeconds: int32(a.MaxRuntimeSeconds),
		RetryPolicy:       a.RetryPolicy,
		MaxAttempts:       int32(a.MaxAttempts),
		BaseUrl:           nullString(a.BaseURL),
	})
	if err != nil {
		return Agent{}, agentNameTakenError(noRowsError(err))
	}
	return agentFromUpdate(row), nil
}

// ArchiveAgent sets archived_at, retiring the agent from every assign dropdown
// while leaving the row (US-AD73 AC1). The running-task guard lives in the
// service beside the identical guard on delete.
func (r *pgxRepository) ArchiveAgent(ctx context.Context, id, orgID string) (Agent, error) {
	row, err := r.q.ArchiveAgent(ctx, store.ArchiveAgentParams{ID: id, OrgID: orgID})
	if err != nil {
		return Agent{}, noRowsError(err)
	}
	return agentFromArchive(row), nil
}

// UnarchiveAgent clears archived_at, returning the agent to service.
func (r *pgxRepository) UnarchiveAgent(ctx context.Context, id, orgID string) (Agent, error) {
	row, err := r.q.UnarchiveAgent(ctx, store.UnarchiveAgentParams{ID: id, OrgID: orgID})
	if err != nil {
		return Agent{}, noRowsError(err)
	}
	return agentFromUnarchive(row), nil
}

// ---- provider credentials (US-AD86) -----------------------------------------

// SetAgentProviderKey writes the sealed credential (US-AD86 AC1) and returns the
// generated has_provider_key the statement echoed. Rotation is this same call:
// the column is overwritten, so the previous ciphertext is gone rather than
// versioned (AC4).
func (r *pgxRepository) SetAgentProviderKey(ctx context.Context, id, orgID string, sealed []byte) (bool, error) {
	row, err := r.q.SetAgentProviderKey(ctx, store.SetAgentProviderKeyParams{
		ID:                id,
		OrgID:             orgID,
		ProviderApiKeyEnc: sealed,
	})
	if err != nil {
		return false, noRowsError(err)
	}
	return row.HasProviderKey != nil && *row.HasProviderKey, nil
}

// ClearAgentProviderKey sets the column to NULL. It is one statement, which is
// what makes the generated flag trustworthy: there is no second write that could
// be skipped and leave a stale "true".
func (r *pgxRepository) ClearAgentProviderKey(ctx context.Context, id, orgID string) (bool, error) {
	row, err := r.q.ClearAgentProviderKey(ctx, store.ClearAgentProviderKeyParams{ID: id, OrgID: orgID})
	if err != nil {
		return false, noRowsError(err)
	}
	return row.HasProviderKey != nil && *row.HasProviderKey, nil
}

// AgentProviderKey reads the sealed credential. NULL maps to ErrNoProviderKey
// rather than an empty slice: pgx scans NULL into a nil []byte without error, so
// without this check an absent credential would travel as "empty string" and the
// handler would try to decrypt nothing.
func (r *pgxRepository) AgentProviderKey(ctx context.Context, id, orgID string) ([]byte, error) {
	sealed, err := r.q.GetAgentProviderKey(ctx, store.GetAgentProviderKeyParams{ID: id, OrgID: orgID})
	if err != nil {
		return nil, noRowsError(err)
	}
	if len(sealed) == 0 {
		return nil, ErrNoProviderKey
	}
	return sealed, nil
}

// The agent row shapes differ only by generated type, so each shares one mapper.
//
// Every shape carries base_url and archived_at, including create/get/list: the
// read statements were widened alongside the update/archive ones, because the
// registry cannot tell an active agent from a retired one without archived_at,
// and it would print a hardcoded zero for the archive count forever.
//
// ReasoningEffort and the JSON columns are the ones worth naming: the DDL
// defaults skills/tools to '[]', so a row always decodes to valid JSON.
func agentFromCreate(r store.CreateAgentRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl), ArchivedAt: ts(r.ArchivedAt),
		HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

func agentFromGet(r store.GetAgentRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl), ArchivedAt: ts(r.ArchivedAt),
		HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

func agentFromList(r store.ListAgentsRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl), ArchivedAt: ts(r.ArchivedAt),
		HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

// agentFromUpdate / agentFromArchive / agentFromUnarchive share one shape: the
// three statements RETURN the same column list, which is why one struct-like
// mapper each is enough. They are separate only because sqlc generates a
// distinct named type per statement.
func agentFromUpdate(r store.UpdateAgentRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl),
		ArchivedAt: ts(r.ArchivedAt), HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

func agentFromArchive(r store.ArchiveAgentRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl),
		ArchivedAt: ts(r.ArchivedAt), HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

func agentFromUnarchive(r store.UnarchiveAgentRow) Agent {
	return Agent{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, Name: r.Name,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort,
		SkillsJSON: r.SkillsJson, ToolsJSON: r.ToolsJson,
		MaxRuntimeSeconds: int(r.MaxRuntimeSeconds), RetryPolicy: r.RetryPolicy,
		MaxAttempts: int(r.MaxAttempts), BaseURL: str(r.BaseUrl),
		ArchivedAt: ts(r.ArchivedAt), HasProviderKey: boolOf(r.HasProviderKey), CreatedAt: r.CreatedAt.Time,
	}
}

// Must is the package-level ULID helper for board IDs; kept here so callers do
// not each reach into the ulid package with a second import surface.
func Must() string { return ulid.Must() }
