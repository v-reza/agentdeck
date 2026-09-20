package board

import (
	"context"
	"time"
)

// Column is one lane of a board. A column is a VIEW of task status, never a new
// status: the board's task lifecycle is the tasks.status CHECK list, and
// columns_json only decides how those statuses are grouped on screen
// (DECISIONS §3). That is why a custom column may not introduce a status.
type Column struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// DefaultColumns is the contract §3 default board layout. The DDL embeds the
// same five columns as the boards.columns_json default, so this is the one place
// to change both — never hand-write a column list in a handler.
// DefaultBudgetDailyMicros is the boards.budget_daily_micros default, the
// N16 daily cap of $20 expressed in micro-USD. It mirrors the DDL default so
// the API and the database agree on what a board with no budget means.
const DefaultBudgetDailyMicros = 20000000

var DefaultColumns = []Column{
	{Key: "backlog", Name: "Backlog"},
	{Key: "ready", Name: "Ready"},
	{Key: "running", Name: "Running"},
	{Key: "review", Name: "Review"},
	{Key: "done", Name: "Done"},
}

// Project groups boards inside an org (ARCHITECTURE 3.5). Slug is unique per org.
type Project struct {
	ID        string
	OrgID     string
	Slug      string
	Name      string
	CreatedAt time.Time
}

// Board is one kanban board plus its daily budget cap in micro-USD (N16).
type Board struct {
	ID                string
	OrgID             string
	ProjectID         string
	Slug              string
	Name              string
	Columns           []Column
	BudgetDailyMicros int64
	CreatedAt         time.Time
}

// TaskStatus is the lifecycle a task may hold. It mirrors the tasks.status CHECK
// list in ARCHITECTURE 3.8 exactly; keeping it here means the compiler, not a
// comment, enforces which statuses exist.
type TaskStatus string

const (
	StatusBacklog          TaskStatus = "backlog"
	StatusReady            TaskStatus = "ready"
	StatusRunning          TaskStatus = "running"
	StatusAwaitingApproval TaskStatus = "awaiting_approval"
	StatusBlocked          TaskStatus = "blocked"
	StatusReview           TaskStatus = "review"
	StatusDone             TaskStatus = "done"
	StatusFailed           TaskStatus = "failed"
	StatusCancelled        TaskStatus = "cancelled"
	StatusArchived         TaskStatus = "archived"
)

// IsTerminal reports whether the status will never change again. A terminal task
// is the only state from which the dispatcher stops retrying it, so the retry
// and reclaim logic keys off this instead of repeating the list.
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case StatusDone, StatusFailed, StatusCancelled, StatusArchived:
		return true
	}
	return false
}

// ColumnKey is the board-column key this status renders under. It is derived,
// never stored: if a board renames or reorders its columns, statuses stay valid
// because they are not copied into column definitions.
func (s TaskStatus) ColumnKey() string {
	switch s {
	case StatusBacklog:
		return "backlog"
	case StatusReady:
		return "ready"
	case StatusRunning, StatusAwaitingApproval:
		return "running"
	case StatusReview:
		return "review"
	case StatusDone, StatusFailed, StatusCancelled:
		return "done"
	case StatusBlocked:
		return "backlog"
	}
	return "backlog"
}

// WorkspaceKind mirrors the tasks.workspace_kind CHECK (ARCHITECTURE 3.8).
type WorkspaceKind string

const (
	WorkspaceScratch   WorkspaceKind = "scratch"
	WorkspaceDir       WorkspaceKind = "dir"
	WorkspaceWorktree  WorkspaceKind = "worktree"
	WorkspaceContainer WorkspaceKind = "container"
)

// Task is the unit of work (ARCHITECTURE 3.8).
type Task struct {
	ID                  string
	OrgID               string
	BoardID             string
	Title               string
	Body                string
	Status              TaskStatus
	Priority            int
	AssigneeAgentID     string
	CreatedBy           string
	IdempotencyKey      string
	BlockKind           string
	ConsecutiveFailures int
	WorkspaceKind       WorkspaceKind
	WorkspacePath       string
	BranchName          string
	CompletionContract  string
	GoalMode            string
	GoalMaxTurns        int
	CurrentRunID        string
	CostMicros          int64
	TokensIn            int64
	TokensOut           int64
	CreatedAt           time.Time
	StartedAt           *time.Time
	CompletedAt         *time.Time
	ArchivedAt          *time.Time
}

// Agent is a worker profile (ARCHITECTURE 3.7). It is the retry and limit
// source for every Run it executes, so the fields that bound a run —
// MaxRuntimeSeconds, RetryPolicy, MaxAttempts — live here rather than on the
// task. The provider credential is deliberately absent: it is M2 scope
// (§16) and US-AD20 AC5 says an agent without one is valid.
type Agent struct {
	ID                string
	OrgID             string
	ProjectID         string
	Name              string
	Provider          string
	Model             string
	ReasoningEffort   string
	SkillsJSON        []byte
	ToolsJSON         []byte
	MaxRuntimeSeconds int
	RetryPolicy       string
	MaxAttempts       int
	// BaseURL is the BYO endpoint (DECISIONS 6A.F). The DDL pairs it with
	// Provider — 'openai_compatible' iff it is set (agents_base_url_chk) — and
	// validateAgent enforces the same pairing so a mismatch is a 400 rather
	// than a CHECK violation surfacing as a 500.
	BaseURL string
	// ArchivedAt is US-AD73: non-nil while the agent is retired. Nil is active.
	ArchivedAt *time.Time
	CreatedAt  time.Time
	// HasProviderKey reports whether provider_api_key_enc is set. It is the
	// generated column agents.has_provider_key (DECISIONS 6A.I), not a second
	// source of truth: the ciphertext itself never reaches this struct, so it
	// cannot leak through a log line or a JSON tag on a list response.
	HasProviderKey bool
}

// RetryPolicy mirrors the agents.retry_policy CHECK (DECISIONS §4).
func AcceptableRetryPolicy(s string) bool {
	switch s {
	case "never", "transient_only", "always":
		return true
	}
	return false
}

// TaskLink is one edge of the dependency DAG. The table's CHECK rejects
// parent_id = child_id; the service additionally rejects cycles before insert.
type TaskLink struct {
	ParentID string
	ChildID  string
}

// Event is one append-only row of the board timeline (ARCHITECTURE 3.12). No
// code path may UPDATE or DELETE an event; the only writer is CreateEvent.
type Event struct {
	ID          int64
	OrgID       string
	BoardID     string
	TaskID      string
	RunID       string
	Kind        string
	PayloadJSON []byte
	CreatedAt   time.Time
}

// Repository is the persistence boundary for the M1 board domain. As in auth,
// every method takes a context so a slow database is bounded by the request,
// and every org-scoped method carries org_id explicitly: there is no second code
// path that could forget the tenant scope.
type Repository interface {
	// ---- projects --------------------------------------------------------
	CreateProject(ctx context.Context, p Project) (Project, error)
	GetProject(ctx context.Context, id, orgID string) (Project, error)
	ListProjects(ctx context.Context, orgID string) ([]Project, error)
	UpdateProjectName(ctx context.Context, id, orgID, name string) error
	DeleteProject(ctx context.Context, id, orgID string) error

	// ---- boards ----------------------------------------------------------
	CreateBoard(ctx context.Context, b Board) (Board, error)
	GetBoard(ctx context.Context, id, orgID string) (Board, error)
	ListBoards(ctx context.Context, orgID, projectID string) ([]Board, error)
	UpdateBoardColumns(ctx context.Context, id, orgID string, columns []Column) error
	UpdateBoardName(ctx context.Context, id, orgID, name string) error
	UpdateBoardBudget(ctx context.Context, id, orgID string, budgetMicros int64) error
	DeleteBoard(ctx context.Context, id, orgID string) error
	CountBoardsInProject(ctx context.Context, orgID, projectID string) (int, error)

	// ---- agents ----------------------------------------------------------
	CreateAgent(ctx context.Context, a Agent) (Agent, error)
	GetAgent(ctx context.Context, id, orgID string) (Agent, error)
	ListAgents(ctx context.Context, orgID, projectID string) ([]Agent, error)
	DeleteAgent(ctx context.Context, id, orgID string) error
	// UpdateAgent replaces every mutable field at once (US-AD96/US-AD106). It
	// is a full update, not a partial patch: the sqlc statement writes all ten
	// columns, so an omitted field lands on its default rather than keeping the
	// stored value. Returns ErrNotFound when the id is absent from the org.
	UpdateAgent(ctx context.Context, a Agent) (Agent, error)
	// ArchiveAgent/UnarchiveAgent toggle agents.archived_at (US-AD73). The row
	// survives because a running task still reads its retry and runtime limits
	// from it; only the assign dropdown stops offering it.
	ArchiveAgent(ctx context.Context, id, orgID string) (Agent, error)
	UnarchiveAgent(ctx context.Context, id, orgID string) (Agent, error)
	// CountAgentRunningTasks is the guard for US-AD20 AC4 and US-AD73 AC3:
	// deleting or archiving an agent that still holds a running task would
	// strand that run without its retry and limit source, so the count is
	// checked before the write.
	CountAgentRunningTasks(ctx context.Context, id, orgID string) (int, error)

	// ---- provider credentials (US-AD86) ----------------------------------
	//
	// These three carry sealed bytes only: encryption happens in the handler
	// layer where the master key lives, so internal/board never sees a
	// plaintext credential and cannot log one by accident. The bools are the
	// generated column agents.has_provider_key, read straight off the
	// statement's RETURNING rather than recomputed in Go (DECISIONS 6A.I).
	//
	// SetAgentProviderKey is also the rotation path: it overwrites the column,
	// so no credential history exists to leak (US-AD86 AC4).
	SetAgentProviderKey(ctx context.Context, id, orgID string, sealed []byte) (bool, error)
	// ClearAgentProviderKey revokes the credential. The agent falls back to the
	// deployment's environment key, which is why this is a 200 and not a delete.
	ClearAgentProviderKey(ctx context.Context, id, orgID string) (bool, error)
	// AgentProviderKey is the single reader of the ciphertext column. It returns
	// ErrNoProviderKey when the column is NULL, so "no credential" is a 400 the
	// caller can act on rather than an empty string that decrypts to nothing.
	AgentProviderKey(ctx context.Context, id, orgID string) ([]byte, error)

	// ---- tasks -----------------------------------------------------------
	CreateTask(ctx context.Context, t Task) (Task, error)
	GetTask(ctx context.Context, id, orgID string) (Task, error)
	ListBoardTasks(ctx context.Context, orgID, boardID string) ([]Task, error)
	// UpdateTaskStatus applies one lifecycle transition atomically: the WHERE
	// clause includes the expected current status, so two writers racing on the
	// same task produce one winner and one ErrConflict instead of a double move.
	UpdateTaskStatus(ctx context.Context, id, orgID string, from, to TaskStatus) (Task, error)
	AssignTask(ctx context.Context, id, orgID, agentID string) (Task, error)
	UpdateTaskFields(ctx context.Context, id, orgID, title, body string, priority int) (Task, error)
	DeleteTask(ctx context.Context, id, orgID string) error
	// ClaimReadyTasks is the dispatcher's atomic claim: it locks a bounded batch
	// of ready tasks with FOR UPDATE SKIP LOCKED and flips them to running in one
	// statement, so concurrent dispatchers claim disjoint sets (ARCHITECTURE 4b).
	ClaimReadyTasks(ctx context.Context, orgID, boardID, runID string, limit int) ([]Task, error)

	// ---- dependency DAG --------------------------------------------------
	CreateTaskLink(ctx context.Context, parentID, childID string) error
	DeleteTaskLink(ctx context.Context, parentID, childID string) error
	ListTaskParents(ctx context.Context, taskID string) ([]TaskLink, error)
	ListTaskChildren(ctx context.Context, taskID string) ([]TaskLink, error)
	// CountUnfinishedParents is the gate a child must pass before it becomes
	// ready: zero unfinished parents (ARCHITECTURE 4e).
	CountUnfinishedParents(ctx context.Context, taskID string) (int, error)

	// ---- events ----------------------------------------------------------
	CreateEvent(ctx context.Context, e Event) (Event, error)
	ListTaskEvents(ctx context.Context, taskID string) ([]Event, error)
	ListBoardEventsAfter(ctx context.Context, boardID, orgID string, afterID int64, limit int) ([]Event, error)
}
