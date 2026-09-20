package board

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// Domain errors. Handlers map these to stable HTTP codes; the same failure must
// always yield the same code from every path.
var (
	ErrNotFound  = errors.New("not found in this workspace")
	ErrSlugTaken = errors.New("slug is already taken")
	// ErrNameTaken is the sibling of ErrSlugTaken: a board name is unique inside
	// one project (US-AD09 AC2), so a second "Sprint 24" is a 409 on the name and
	// not on the slug. Kept separate because the operator's fix differs.
	ErrNameTaken      = errors.New("a board with this name already exists in this project")
	ErrInvalidInput   = errors.New("invalid input")
	ErrInvalidStatus  = errors.New("unsupported task status")
	ErrCycleDetected  = errors.New("dependency cycle detected")
	ErrColumnsInvalid = errors.New("columns are not valid")
	// ErrColumnNameTaken is US-AD10 AC3: renaming a column to a name another
	// column already uses is a 409, not a 400. It is separate from
	// ErrColumnsInvalid because the operator's fix differs — one is "pick a
	// different name", the other is "your payload is malformed".
	ErrColumnNameTaken = errors.New("a column with this name already exists on this board")
	// ErrColumnHasTasks is US-AD10 AC2: a column holding tasks cannot be removed
	// until those tasks are moved. The error carries no count because the HTTP
	// body must not leak how much work sits in another tenant's board — the
	// count is read from the caller's own board before this is returned.
	ErrColumnHasTasks = errors.New("column still holds tasks; move them before removing it")
	ErrBudgetInvalid  = errors.New("budget must be zero or positive")
	// ErrAgentNameTaken is US-AD20 AC3: an agent name is unique inside one
	// project (the DDL enforces it with agents_project_name_key), so a second
	// "agent-backend" is a 409 rather than a 500 from the constraint.
	ErrAgentNameTaken = errors.New("an agent with this name already exists in this project")
	// ErrAgentHasRunningTask is US-AD20 AC4: an agent executing a task cannot be
	// deleted, because the run would lose the retry and runtime limits it reads
	// from the agent row. The operator must finish or fail the task first.
	ErrAgentHasRunningTask = errors.New("agent is still running a task; finish or fail it first")
	ErrLastOwner           = errors.New("cannot remove the last owner")
)

// slugMin/slugMax and the allowed characters mirror the DDL CHECK
// slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$', so a slug the API accepts is a
// slug the database accepts. Validating here means a 400 before any write.
const (
	slugMin = 3
	slugMax = 40
)

// validSlug reports whether a string matches the DDL slug CHECK. Length bounds
// come from the regex (2..39 body characters plus the two edge characters).
func validSlug(s string) bool {
	runes := []rune(s)
	if len(runes) < slugMin || len(runes) > slugMax {
		return false
	}
	for i, r := range runes {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			continue
		case r == '-' && i > 0 && i < len(runes)-1:
			continue
		}
		return false
	}
	return true
}

// validateName rejects an empty or whitespace-only name. A name made of only
// spaces would otherwise pass the NOT NULL constraint and render as blank.
func validateName(name string) bool {
	return strings.TrimSpace(name) != ""
}

// validColumns checks the board column contract: at least one column, unique
// keys, and non-empty labels. The DDL CHECK only verifies the value is a JSON
// array, so the semantic contract lives here — one place that both the create
// and the update path pass through.
func validColumns(cols []Column) bool {
	if len(cols) == 0 {
		return false
	}
	seen := make(map[string]bool, len(cols))
	for _, c := range cols {
		if strings.TrimSpace(c.Key) == "" || strings.TrimSpace(c.Name) == "" {
			return false
		}
		if seen[c.Key] {
			return false
		}
		seen[c.Key] = true
	}
	return true
}

// checkColumnNamesUnique is US-AD10 AC3. `validColumns` already rejects a
// duplicate *key*, but the key is internal — the operator reads and edits the
// display *name*, so two columns both called "Review" would be
// indistinguishable on the board while carrying different keys. Comparison is
// trimmed and case-insensitive because "Done" and "done " are the same column
// to a human, and a 409 the operator cannot see the reason for is worse than
// the 400 it replaced.
func checkColumnNamesUnique(cols []Column) error {
	seen := make(map[string]bool, len(cols))
	for _, c := range cols {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if seen[name] {
			return ErrColumnNameTaken
		}
		seen[name] = true
	}
	return nil
}

// Service holds the invariants of the board domain. It owns validation and the
// lifecycle rules so handlers stay thin: parse, call a service method, map the
// error to a status code.
type Service struct {
	repo Repository
}

// NewService builds the domain service over a Repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateProject persists a new project inside an org. Slug uniqueness is per
// org (projects_org_slug_key); a collision reports ErrSlugTaken as a 409.
func (s *Service) CreateProject(ctx context.Context, orgID, slug, name string) (Project, error) {
	if !validSlug(slug) || !validateName(name) {
		return Project{}, ErrInvalidInput
	}
	p := Project{
		ID:    Must(),
		OrgID: orgID,
		Slug:  slug,
		Name:  name,
	}
	return s.repo.CreateProject(ctx, p)
}

// GetProject loads a project, scoped by org so a caller cannot read another
// tenant's project by id.
func (s *Service) GetProject(ctx context.Context, id, orgID string) (Project, error) {
	return s.repo.GetProject(ctx, id, orgID)
}

// ListProjects lists the projects of one org.
func (s *Service) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	return s.repo.ListProjects(ctx, orgID)
}

// UpdateProjectName renames a project. The slug is the stable identifier, so a
// rename never changes URLs.
func (s *Service) UpdateProjectName(ctx context.Context, id, orgID, name string) error {
	if !validateName(name) {
		return ErrInvalidInput
	}
	return s.repo.UpdateProjectName(ctx, id, orgID, name)
}

// DeleteProject removes a project and, by cascade, its boards and tasks.
func (s *Service) DeleteProject(ctx context.Context, id, orgID string) error {
	return s.repo.DeleteProject(ctx, id, orgID)
}

// CreateBoard persists a board under a project. Empty columns fall back to the
// contract default so a board always has a renderable layout.
func (s *Service) CreateBoard(ctx context.Context, orgID, projectID, slug, name string, cols []Column, budgetMicros int64) (Board, error) {
	if !validSlug(slug) || !validateName(name) {
		return Board{}, ErrInvalidInput
	}
	// The parent project must belong to the caller's org. boards_project_fk keys
	// only on project_id, so Postgres alone accepts a board whose project lives in
	// another tenant: the probe returned 201 with a foreign project_id, and the
	// board was then invisible in that org's own list (US-AD07/US-AD09 AC4).
	// Resolving the project through the org-scoped read makes the tenant the
	// authority, and a project that is absent *or* foreign is the same 404.
	if _, err := s.repo.GetProject(ctx, projectID, orgID); err != nil {
		return Board{}, err
	}
	if cols == nil {
		cols = DefaultColumns
	}
	if !validColumns(cols) {
		return Board{}, ErrColumnsInvalid
	}
	if budgetMicros < 0 {
		return Board{}, ErrBudgetInvalid
	}
	b := Board{
		ID:                Must(),
		OrgID:             orgID,
		ProjectID:         projectID,
		Slug:              slug,
		Name:              name,
		Columns:           cols,
		BudgetDailyMicros: budgetMicros,
	}
	return s.repo.CreateBoard(ctx, b)
}

// GetBoard loads one board scoped by org.
func (s *Service) GetBoard(ctx context.Context, id, orgID string) (Board, error) {
	return s.repo.GetBoard(ctx, id, orgID)
}

// ListBoards lists boards under one project.
func (s *Service) ListBoards(ctx context.Context, orgID, projectID string) ([]Board, error) {
	return s.repo.ListBoards(ctx, orgID, projectID)
}

// UpdateBoardColumns replaces the board layout. A task's status maps to a
// column key (DECISIONS 3), so the layout is a view of status and never a status
// of its own.
//
// Two acceptance criteria live here rather than in the handler, because both
// need the board's *current* layout to decide:
//
//   - AC3: the new names must be unique on this board.
//   - AC2: a column that still holds tasks may not be dropped. The check reads
//     the caller's own board through the org-scoped repository, so a board in
//     another tenant is a 404 before any of this runs.
func (s *Service) UpdateBoardColumns(ctx context.Context, id, orgID string, cols []Column) error {
	if !validColumns(cols) {
		return ErrColumnsInvalid
	}
	if err := checkColumnNamesUnique(cols); err != nil {
		return err
	}
	before, err := s.repo.GetBoard(ctx, id, orgID)
	if err != nil {
		return err
	}
	if err := s.guardRemovedColumns(ctx, before, cols); err != nil {
		return err
	}
	return s.repo.UpdateBoardColumns(ctx, id, orgID, cols)
}

// guardRemovedColumns enforces US-AD10 AC2: a column that still holds tasks may
// not be removed until they are moved.
//
// "Holds tasks" is decided by the same mapping the board screen renders with —
// TaskStatus.ColumnKey() — rather than by comparing status strings to column
// keys. The two are not the same set: `awaiting_approval` renders in `running`,
// and `failed`/`cancelled` render in `done`, so a status-equality check would
// report those columns as empty and delete work out from under the operator.
//
// The task list is reused rather than a dedicated COUNT query: the repository
// already answers it org-scoped and excluding archived rows, and a second
// aggregation would be a second place for the column mapping to drift.
func (s *Service) guardRemovedColumns(ctx context.Context, before Board, after []Column) error {
	kept := make(map[string]bool, len(after))
	for _, c := range after {
		kept[c.Key] = true
	}

	removed := make(map[string]bool)
	for _, c := range before.Columns {
		if !kept[c.Key] {
			removed[c.Key] = true
		}
	}
	if len(removed) == 0 {
		return nil
	}

	tasks, err := s.repo.ListBoardTasks(ctx, before.OrgID, before.ID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if removed[t.Status.ColumnKey()] {
			return ErrColumnHasTasks
		}
	}
	return nil
}

// UpdateBoardName renames a board. It is Member-level (US-AD83), the same gate
// as the budget update, which is why both are reachable through PATCH
// /boards/{id} while the layout has its own Admin-gated route.
func (s *Service) UpdateBoardName(ctx context.Context, id, orgID, name string) error {
	if !validateName(name) {
		return ErrInvalidInput
	}
	return s.repo.UpdateBoardName(ctx, id, orgID, name)
}

// UpdateBoardBudget sets the board's daily cap in micro-USD (N16).
func (s *Service) UpdateBoardBudget(ctx context.Context, id, orgID string, budgetMicros int64) error {
	if budgetMicros < 0 {
		return ErrBudgetInvalid
	}
	return s.repo.UpdateBoardBudget(ctx, id, orgID, budgetMicros)
}

// DeleteBoard removes a board and its tasks.
func (s *Service) DeleteBoard(ctx context.Context, id, orgID string) error {
	return s.repo.DeleteBoard(ctx, id, orgID)
}

// ---- agents (US-AD20) -------------------------------------------------------

// CreateAgent persists a worker profile. Provider and model are required and
// stored verbatim: the model string is the pricing key, so normalising it here
// would silently change what the ledger looks up later. A credential is NOT
// required (AC5) — an agent registered without one is valid and simply not
// ready to execute, which is what lets a fleet be described before its secrets
// are distributed.
func (s *Service) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	if !validateName(a.Name) || !validateName(a.Provider) || !validateName(a.Model) {
		return Agent{}, ErrInvalidInput
	}
	if !AcceptableRetryPolicy(a.RetryPolicy) {
		return Agent{}, ErrInvalidInput
	}
	if a.MaxRuntimeSeconds < 1 || a.MaxRuntimeSeconds > 86400 {
		return Agent{}, ErrInvalidInput
	}
	if a.MaxAttempts < 1 || a.MaxAttempts > 10 {
		return Agent{}, ErrInvalidInput
	}
	if a.ID == "" {
		a.ID = Must()
	}
	if a.ReasoningEffort == "" {
		a.ReasoningEffort = "medium"
	}
	if len(a.SkillsJSON) == 0 {
		a.SkillsJSON = []byte("[]")
	}
	if len(a.ToolsJSON) == 0 {
		a.ToolsJSON = []byte("[]")
	}
	return s.repo.CreateAgent(ctx, a)
}

// GetAgent loads one agent scoped by org.
func (s *Service) GetAgent(ctx context.Context, id, orgID string) (Agent, error) {
	return s.repo.GetAgent(ctx, id, orgID)
}

// ListAgents returns the agents of one project.
func (s *Service) ListAgents(ctx context.Context, orgID, projectID string) ([]Agent, error) {
	return s.repo.ListAgents(ctx, orgID, projectID)
}

// DeleteAgent removes an agent, refusing while it still holds a running task
// (AC4). The check and the delete are not one transaction: a task could be
// claimed between them. That race is acceptable here because the tasks_agent_fk
// constraint is ON DELETE SET NULL — the worst case is a running task whose
// assignee is cleared, which is exactly the state the FK already allows, and
// the dispatcher reads limits from the run it created, not from a live lookup.
// Closing it fully would need a lock on the tasks table for every agent delete.
func (s *Service) DeleteAgent(ctx context.Context, id, orgID string) error {
	running, err := s.repo.CountAgentRunningTasks(ctx, id, orgID)
	if err != nil {
		return err
	}
	if running > 0 {
		return ErrAgentHasRunningTask
	}
	return s.repo.DeleteAgent(ctx, id, orgID)
}

// CreateTask persists a new task on a board. The initial status is backlog
// unless the caller passes a status; ready is set by the dispatcher once the
// dependency gate passes, not by this method.
func (s *Service) CreateTask(ctx context.Context, orgID, boardID, title, body, createdBy string, priority int, status TaskStatus) (Task, error) {
	if !validateName(title) {
		return Task{}, ErrInvalidInput
	}
	if status == "" {
		status = StatusBacklog
	}
	if priority < -10 || priority > 10 {
		return Task{}, ErrInvalidInput
	}
	t := Task{
		ID:            Must(),
		OrgID:         orgID,
		BoardID:       boardID,
		Title:         title,
		Body:          body,
		Status:        status,
		Priority:      priority,
		CreatedBy:     createdBy,
		WorkspaceKind: WorkspaceScratch,
		GoalMode:      "auto",
		GoalMaxTurns:  25,
	}
	return s.repo.CreateTask(ctx, t)
}

// GetTask loads a task scoped by org.
func (s *Service) GetTask(ctx context.Context, id, orgID string) (Task, error) {
	return s.repo.GetTask(ctx, id, orgID)
}

// ListBoardTasks lists the non-archived tasks of one board.
func (s *Service) ListBoardTasks(ctx context.Context, orgID, boardID string) ([]Task, error) {
	return s.repo.ListBoardTasks(ctx, orgID, boardID)
}

// MoveTask applies one lifecycle transition with the optimistic-status guard:
// the caller states the status it believes the task is in, and the database
// only updates a row that still holds it. Two writers racing produce one winner
// and one ErrConflict, never a double move.
func (s *Service) MoveTask(ctx context.Context, id, orgID string, from, to TaskStatus) (Task, error) {
	if from == "" || to == "" {
		return Task{}, ErrInvalidStatus
	}
	if from == to {
		// An idempotent move is not an error: return the current task so a
		// retried request succeeds instead of 409-ing its own earlier write.
		return s.repo.GetTask(ctx, id, orgID)
	}
	if to == StatusArchived {
		return s.repo.UpdateTaskStatus(ctx, id, orgID, from, to)
	}
	return s.repo.UpdateTaskStatus(ctx, id, orgID, from, to)
}

// AssignTask sets the agent that will run the task. An empty agent id clears
// the assignment.
func (s *Service) AssignTask(ctx context.Context, id, orgID, agentID string) (Task, error) {
	return s.repo.AssignTask(ctx, id, orgID, agentID)
}

// UpdateTaskFields edits a task's text and priority.
func (s *Service) UpdateTaskFields(ctx context.Context, id, orgID, title, body string, priority int) (Task, error) {
	if !validateName(title) {
		return Task{}, ErrInvalidInput
	}
	if priority < -10 || priority > 10 {
		return Task{}, ErrInvalidInput
	}
	return s.repo.UpdateTaskFields(ctx, id, orgID, title, body, priority)
}

// DeleteTask removes a task. Its event history is deleted by the tasks cascade
// through board, so the timeline goes with it.
func (s *Service) DeleteTask(ctx context.Context, id, orgID string) error {
	return s.repo.DeleteTask(ctx, id, orgID)
}

// CreateLink adds one dependency edge and returns whether the child became
// ready. A cycle would deadlock the dispatcher, so the walk refuses to add an
// edge that closes one before the row is written (ARCHITECTURE 4e).
func (s *Service) CreateLink(ctx context.Context, parentID, childID string) (ready bool, err error) {
	if parentID == childID {
		return false, ErrInvalidInput
	}
	cyclic, err := s.wouldCycle(ctx, parentID, childID)
	if err != nil {
		return false, err
	}
	if cyclic {
		return false, ErrCycleDetected
	}
	if err := s.repo.CreateTaskLink(ctx, parentID, childID); err != nil {
		return false, err
	}
	unfinished, err := s.repo.CountUnfinishedParents(ctx, childID)
	if err != nil {
		return false, err
	}
	return unfinished == 0, nil
}

// DeleteLink removes one edge and re-checks the child's dependency gate.
func (s *Service) DeleteLink(ctx context.Context, parentID, childID string) (ready bool, err error) {
	if err := s.repo.DeleteTaskLink(ctx, parentID, childID); err != nil {
		return false, err
	}
	unfinished, err := s.repo.CountUnfinishedParents(ctx, childID)
	if err != nil {
		return false, err
	}
	return unfinished == 0, nil
}

// wouldCycle reports whether adding parentID -> childID would close a cycle:
// it walks the ancestor chain of parentID and asks whether childID is already
// an ancestor. The walk is bounded by the depth of the DAG, and a task is its
// own ancestor is impossible because the table CHECK rejects self-links.
func (s *Service) wouldCycle(ctx context.Context, parentID, childID string) (bool, error) {
	// If childID is an ancestor of parentID, then parentID -> childID closes a
	// loop. Walk parents of parentID upward.
	seen := make(map[string]bool)
	frontier := []string{parentID}
	for len(frontier) > 0 {
		current := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if seen[current] {
			continue
		}
		seen[current] = true
		links, err := s.repo.ListTaskParents(ctx, current)
		if err != nil {
			return false, err
		}
		for _, link := range links {
			if link.ParentID == childID {
				return true, nil
			}
			if !seen[link.ParentID] {
				frontier = append(frontier, link.ParentID)
			}
		}
	}
	return false, nil
}

// TaskLinks returns both sides of a task's dependency edges.
func (s *Service) TaskLinks(ctx context.Context, taskID string) (parents, children []TaskLink, err error) {
	parents, err = s.repo.ListTaskParents(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	children, err = s.repo.ListTaskChildren(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	return parents, children, nil
}

// TaskParents returns the direct prerequisites of one task.
func (s *Service) TaskParents(ctx context.Context, taskID string) ([]TaskLink, error) {
	return s.repo.ListTaskParents(ctx, taskID)
}

// RecordTaskEvent appends one lifecycle event to the board timeline. Events are
// append-only: no code path may update or delete them (ARCHITECTURE 3.12).
func (s *Service) RecordTaskEvent(ctx context.Context, orgID, boardID, taskID, kind string, payload []byte) (Event, error) {
	return s.repo.CreateEvent(ctx, Event{
		OrgID:       orgID,
		BoardID:     boardID,
		TaskID:      taskID,
		Kind:        kind,
		PayloadJSON: payload,
	})
}

// TaskHistory returns the append-only timeline for one task.
func (s *Service) TaskHistory(ctx context.Context, taskID string) ([]Event, error) {
	return s.repo.ListTaskEvents(ctx, taskID)
}

// Claim applies the dispatcher's atomic claim over the board's ready tasks.
// Two dispatchers running concurrently claim disjoint sets because the CTE
// holds FOR UPDATE SKIP LOCKED inside one transaction (ARCHITECTURE 4b).
func (s *Service) Claim(ctx context.Context, orgID, boardID, runID string, limit int) ([]Task, error) {
	if limit <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repo.ClaimReadyTasks(ctx, orgID, boardID, runID, limit)
}

// AcceptableStatus reports whether a status string is one the lifecycle allows
// a caller to set directly. Internal transitions (running, awaiting_approval)
// are set by the dispatcher only.
func AcceptableStatus(s string) bool {
	switch TaskStatus(s) {
	case StatusBacklog, StatusReady, StatusReview, StatusDone, StatusFailed,
		StatusCancelled, StatusArchived, StatusBlocked:
		return true
	}
	return false
}

// isAlpha reports whether a rune is an ASCII letter — used to reject slugs that
// contain uppercase, which the DDL CHECK already forbids.
func isAlpha(r rune) bool {
	return unicode.IsLetter(r)
}
