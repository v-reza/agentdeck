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

// TaskFilter narrows a board listing. Every field is optional: nil means "no
// constraint on this axis", which is why they are pointers rather than zero
// values — an empty search term is a real search ("match nothing") and must not
// be indistinguishable from "no search".
//
// The three axes are the ones §6.2.16 advertises for
// `GET /api/v1/boards/{board_id}/tasks`, applied in SQL rather than by the
// caller filtering the response.
type TaskFilter struct {
	// Statuses is a set: the board's filter chips are multi-select, and a scalar
	// would mean dropping every selection but the first. Empty = no constraint,
	// which is indistinguishable from "no status matches" only if a caller passes
	// an empty non-nil slice — `AcceptableStatus` rejects that at the edge.
	Statuses []TaskStatus
	Assignee *string
	Search   *string
}

// Empty reports whether the filter constrains anything.
func (f TaskFilter) Empty() bool {
	return len(f.Statuses) == 0 && f.Assignee == nil && f.Search == nil
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
	// ProviderID is the registry row this agent draws its endpoint and
	// credential from (US-AD109, DECISIONS 6A.J). Empty means the agent has no
	// provider of its own — it is a valid, permanent state (an agent using the
	// deployment's environment default), not a missing backfill. It is empty
	// rather than *string because every read path treats "none" and "unset"
	// identically, and the sqlc params are *string at the boundary.
	ProviderID string
	// ArchivedAt is US-AD73: non-nil while the agent is retired. Nil is active.
	ArchivedAt *time.Time
	CreatedAt  time.Time
	// HasProviderKey reports whether provider_api_key_enc is set. It is the
	// generated column agents.has_provider_key (DECISIONS 6A.I), not a second
	// source of truth: the ciphertext itself never reaches this struct, so it
	// cannot leak through a log line or a JSON tag on a list response.
	HasProviderKey bool
}

// AssignedTask is one row of the assignment table on the agent detail screen
// (US-AD73 AC1). It is deliberately narrow: the screen needs to name the task,
// say where it stands, and show what it has spent — not the whole Task struct,
// which the board screens own.
type AssignedTask struct {
	ID         string
	BoardID    string
	Title      string
	Status     TaskStatus
	CostMicros int64
	CreatedAt  time.Time
}

// AssignableAgent is one option of the assign picker (US-AD73 AC2). HasProviderKey
// is carried because the picker marks the rows that are not ready to claim yet.
type AssignableAgent struct {
	ID             string
	Name           string
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

// ---- M4 runtime: runs, steps, ledger -----------------------------------------

// Run is one execution of one task by one agent. It is the unit the dispatcher
// claims, the executor heartbeats, and the ledger rolls up into.
type Run struct {
	ID              string
	OrgID           string
	TaskID          string
	AgentID         string
	Attempt         int
	Status          RunStatus
	Outcome         string
	FailureKind     string
	LastHeartbeatAt *time.Time
	MaxRuntimeSecs  int
	CostMicros      int64
	TokensIn        int64
	TokensOut       int64
	Summary         string
	Error           string
	StartedAt       time.Time
	EndedAt         *time.Time
}

// RunStatus is `runs.status`. The three live states are distinct on purpose:
// `claiming` is the window between the row lock being released and the executor
// reporting in, and a reclaim has to tell that window apart from a live run.
type RunStatus string

const (
	RunPending  RunStatus = "pending"
	RunClaiming RunStatus = "claiming"
	RunRunning  RunStatus = "running"
	RunEnded    RunStatus = "ended"
)

// Step is one line of a run's trace. `Seq` is dense and starts at 1, which is
// what makes the trace replayable in order.
type Step struct {
	ID          int64
	OrgID       string
	RunID       string
	Seq         int
	Kind        string
	Name        string
	Status      string
	TokensIn    int64
	TokensOut   int64
	CostMicros  int64
	StartedAt   time.Time
	EndedAt     *time.Time
	PayloadJSON []byte
}

// LedgerEntry is one priced LLM (or tool) call. `CostMicros` is the computed
// price × quantity, never a unit price: the row has to stay readable after the
// price table changes, which is why `PriceVersion` is mandatory and `PriceSource`
// records which of the four resolution tiers produced the number.
type LedgerEntry struct {
	ID               int64
	OrgID            string
	RunID            string
	TaskID           string
	Provider         string
	Model            string
	Kind             string
	TokensIn         int64
	TokensOut        int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostMicros       int64
	PriceVersion     int
	PriceSource      string
	PricingModel     string
	CreatedAt        time.Time
}

// LedgerUsage is the usage a worker reports for one call. The executor resolves
// the price (it owns the provider and the model) and hands the ledger a number
// that is already final; the board service never prices anything itself.
type LedgerUsage struct {
	Provider         string
	Model            string
	Kind             string
	TokensIn         int64
	TokensOut        int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostMicros       int64
	PriceVersion     int
	PriceSource      string
	PricingModel     string
}

// RunSummary is what a worker reports when it finishes a run. `FailureKind` is
// the worker's classification (ARCHITECTURE 10); it decides whether a failure is
// retryable, so a worker that leaves it empty is treated as `unknown`.
type RunSummary struct {
	Outcome     string
	FailureKind string
	Summary     string
	Error       string
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
	// ListAssignedTasks returns the tasks an agent holds, running first
	// (US-AD73 AC1: the detail screen states that archiving does not cut a
	// running task off, and that is only meaningful beside the rows it is
	// about). Archived tasks are excluded, like ListBoardTasks.
	ListAssignedTasks(ctx context.Context, orgID, agentID string) ([]AssignedTask, error)
	// ListAssignableAgentsForBoard is the real assign picker for one board:
	// active agents of the board's project (US-AD73 AC2). It is what the detail
	// screen previews, so the preview cannot drift from the picker itself.
	ListAssignableAgentsForBoard(ctx context.Context, orgID, boardID string) ([]AssignableAgent, error)
	// CountArchivedAgents is how the picker reports what it hid instead of
	// omitting rows silently.
	CountArchivedAgents(ctx context.Context, orgID string) (int, error)

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
	ListBoardTasks(ctx context.Context, orgID, boardID string, filter TaskFilter) ([]Task, error)
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
	// ClaimTask claims one named task (POST /tasks/{id}/claim).
	ClaimTask(ctx context.Context, taskID, orgID, runID string) (Task, error)

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

	// ---- runs (M4) -------------------------------------------------------
	CreateRun(ctx context.Context, r Run) (Run, error)
	GetRun(ctx context.Context, id, orgID string) (Run, error)
	ListTaskRuns(ctx context.Context, taskID, orgID string) ([]Run, error)
	NextRunAttempt(ctx context.Context, taskID string) (int, error)
	// HeartbeatRun returns ErrNotFound when the run is no longer live, so a
	// worker whose run was reclaimed hears "stop" instead of writing further.
	HeartbeatRun(ctx context.Context, id, orgID string) (Run, error)
	EndRun(ctx context.Context, id, orgID string, summary RunSummary) (Run, error)

	// ---- steps (M4) ------------------------------------------------------
	CreateStep(ctx context.Context, s Step) (Step, error)
	FinishStep(ctx context.Context, runID string, seq int, orgID string, s Step) (Step, error)
	ListRunSteps(ctx context.Context, runID, orgID string) ([]Step, error)

	// ---- ledger (M4) -----------------------------------------------------
	CreateLedgerEntry(ctx context.Context, e LedgerEntry) (LedgerEntry, error)
	ListRunLedger(ctx context.Context, runID, orgID string) ([]LedgerEntry, error)
	ListBoardLedger(ctx context.Context, boardID, orgID string, limit int) ([]LedgerEntry, error)
	BoardSpendToday(ctx context.Context, boardID, orgID string) (int64, error)

	// IncrementTaskFailures/ResetTaskFailures own the retry counter. It has to
	// be a counter in the database rather than a value the caller passes: the
	// ceiling comparison happens on the next attempt, possibly on another
	// worker, so the number must outlive the run that produced it.
	IncrementTaskFailures(ctx context.Context, taskID, orgID string) (int, error)
	ResetTaskFailures(ctx context.Context, taskID, orgID string) error
	// BlockTask sets `blocked` with its kind in one write; SetTaskCurrentRun
	// binds a claimed task to its run.
	BlockTask(ctx context.Context, taskID, orgID, kind string) error
	SetTaskCurrentRun(ctx context.Context, taskID, orgID, runID string) (Task, error)
	// ClearTaskCurrentRun releases the binding a claim created. It is not
	// optional bookkeeping: the claim predicate is `current_run_id IS NULL`, so a
	// finished run that kept its id would make the task unclaimable forever.
	ClearTaskCurrentRun(ctx context.Context, taskID, orgID string) error
}
