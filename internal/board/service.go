package board

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"agentdeck/internal/pricing"
	"agentdeck/internal/providerreg"
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
	// ProviderOpenAICompatible is the BYO protocol name. Until phase 6 it also
	// meant "the provider that carries a base_url" (agents_base_url_chk); that
	// column is gone, so what is left is the name a BYO agent's `provider`
	// holds. Exported so the handler and the validator name the same string
	// instead of each carrying its own literal.
	ProviderOpenAICompatible = "openai_compatible"
	// ErrAgentHasRunningTask is US-AD20 AC4: an agent executing a task cannot be
	// deleted, because the run would lose the retry and runtime limits it reads
	// from the agent row. The operator must finish or fail the task first.
	ErrAgentHasRunningTask = errors.New("agent is still running a task; finish or fail it first")
	// ErrArchiveRequiresAdmin is US-AD73 AC4. It is checked in the handler
	// rather than at the route because PATCH /agents/{id} carries two floors:
	// Member for the field update (US-AD96) and owner/admin for retiring the
	// agent. The domain error exists so the reason is one string in one place.
	ErrArchiveRequiresAdmin = errors.New("archiving an agent requires the owner or admin role")
	// ErrArchiveRequiresAdminTask is US-AD59 AC4. Same authority question as
	// the agent sentinel above, different resource: retiring a task is an
	// admin decision, while ordinary column moves stay Member-level.
	ErrArchiveRequiresAdminTask = errors.New("archiving a task requires the owner or admin role")
	// ErrArchiveRequiresTerminal is US-AD59 AC2: only a task that has already
	// finished moving may be retired. Archiving `running` would take the row off
	// the board while a run still holds it, and archiving `blocked`/`ready`
	// would hide work that is still queued.
	ErrArchiveRequiresTerminal = errors.New("only a finished task can be archived")
	ErrLastOwner               = errors.New("cannot remove the last owner")
	// ErrUnknownProvider is US-AD86 AC3: a credential for a provider the price
	// table does not know is a 400, not a stored secret nobody can spend. It is
	// deliberately not ErrInvalidInput: the operator's fix is "pick a provider
	// this deployment supports", not "your payload is malformed".
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrUnknownModel is US-AD67 AC2: the model resolves to no price at all, so
	// the agent could never be costed or claimed.
	ErrUnknownModel = errors.New("unknown model")
	// ErrCredentialKeyUnavailable is returned when AGENTDECK_MASTER_KEY is
	// missing or malformed. It must never fall back to storing the key in the
	// clear — an unavailable master key is a 500 that stores nothing, which is
	// the only safe reading of "we cannot encrypt this".
	ErrCredentialKeyUnavailable = errors.New("provider credential encryption is not configured")
	// ErrNoProviderKey is the "no credential stored" case. It is a 400 and not a
	// 404 because the agent exists: what the caller asked for is a credential
	// check on an agent that has none.
	ErrNoProviderKey = errors.New("agent has no stored provider credential")
	// ErrProviderNotProbeable is returned when a handshake was asked for but
	// the agent has no endpoint in this deployment. Since US-AD109 that is the
	// agent with no provider of its own: the address lives on the provider row,
	// so an agent pointing at nothing has nothing to ping. The state itself is
	// legitimate — the message names the missing reference, not a malformed
	// agent.
	ErrProviderNotProbeable = errors.New("agent has no provider to test")
	// ErrProviderHandshakeFailed is the upstream's answer, not ours: the endpoint
	// was reachable and refused (401, 404, a timeout). It is a 502 so the UI can
	// tell "your key is wrong" from "we are broken".
	ErrProviderHandshakeFailed = errors.New("provider handshake failed")
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
	// claimLock identifies this process as the owner of a run. It exists so the
	// dispatcher's heartbeat (5.1 phase 1) only touches runs this instance
	// claimed: two instances sharing a board must not keep each other's dead runs
	// alive. Empty is allowed and means "unset" — see WithClaimLock.
	claimLock string
	// decrypt opens an AES-GCM sealed provider credential. Nil means no master key
	// was configured, which is reported rather than worked around.
	decrypt func([]byte) (string, error)
	// providers resolves an agent's provider_id to an endpoint. Nil means none was
	// wired in, which is reported rather than guessed.
	providers ProviderRegistry
}

// NewService builds the domain service over a Repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// WithClaimLock names this instance for run ownership. Set once at wiring time;
// returning the receiver keeps the constructor call a one-liner.
func (s *Service) WithClaimLock(host string) *Service {
	s.claimLock = host
	return s
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

	// Unfiltered on purpose: this asks "does any task still live in a column I
	// am removing", so narrowing the set would let a filtered-out task sit in a
	// deleted column.
	tasks, err := s.repo.ListBoardTasks(ctx, before.OrgID, before.ID, TaskFilter{})
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
	if err := validateAgent(a); err != nil {
		return Agent{}, err
	}
	if a.ID == "" {
		a.ID = Must()
	}
	// The insert binds these columns explicitly, so an omitted value must be
	// filled here — the DDL default only applies when the column is absent from
	// the statement, which it is not.
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

// validateAgent is the one place the agent field contract is checked, shared by
// create and update so the two paths cannot drift. Every rule here mirrors a
// DDL CHECK: validating first turns what Postgres would answer with a raw
// SQLSTATE (a 500) into a 400 the caller can act on.
func validateAgent(a Agent) error {
	if !validateName(a.Name) || !validateName(a.Provider) || !validateName(a.Model) {
		return ErrInvalidInput
	}
	if !AcceptableRetryPolicy(a.RetryPolicy) {
		return ErrInvalidInput
	}
	if a.MaxRuntimeSeconds < 1 || a.MaxRuntimeSeconds > 86400 {
		return ErrInvalidInput
	}
	if a.MaxAttempts < 1 || a.MaxAttempts > 10 {
		return ErrInvalidInput
	}
	// US-AD67 AC1/AC2: an agent must be registerable against a provider and a
	// model this deployment can actually price.
	//
	// Provider: ValidateProvider reads the price table, so the rule cannot drift
	// from the catalog the way a hand-written allowlist would.
	//
	// Model: the AC says a combination absent from the price table is a 400.
	// Read literally that would also reject `openai_compatible`, whose model
	// list comes from the operator's own endpoint (US-AD106 AC2) and is
	// therefore never in our catalog — enforcing it there would make BYO
	// impossible while US-AD106 is a Must in the same milestone. So AC2 applies
	// to the built-in providers only, and "known" means the name resolves to
	// something priced: an exact entry OR a pattern. `unpriced` is the 400.
	if err := ValidateProvider(a.Provider); err != nil {
		return err
	}
	if a.Provider != ProviderOpenAICompatible {
		if pricing.Resolve(a.Model, nil).Source == pricing.SourceUnpriced {
			return fmt.Errorf("%w: model %q is in no price table", ErrUnknownModel, a.Model)
		}
	}
	return nil
}

// GetAgent loads one agent scoped by org.
func (s *Service) GetAgent(ctx context.Context, id, orgID string) (Agent, error) {
	return s.repo.GetAgent(ctx, id, orgID)
}

// ListAgents returns the agents of one project.
func (s *Service) ListAgents(ctx context.Context, orgID, projectID string) ([]Agent, error) {
	return s.repo.ListAgents(ctx, orgID, projectID)
}

// AssignmentView is the agent detail screen's assignment section: which tasks the
// agent holds, and the picker as it would appear for one of its boards.
//
// It is one call rather than three because the section is meaningless unless the
// three agree: the count of hidden agents is only honest beside the list they were
// hidden from, and the enforce flag is only meaningful beside the running rows.
type AssignmentView struct {
	Tasks []AssignedTask
	// Picker is the active agents of BoardID's project. Empty when no board is
	// given, because the picker belongs to a board and there is nothing to
	// preview without one.
	Picker []AssignableAgent
	// HiddenAgents is how many archived agents the picker left out (AC2). The
	// section states it instead of quietly omitting rows.
	HiddenAgents int
	// BoardID is the board the picker was resolved for, empty when the agent
	// holds no task yet.
	BoardID string
}

// AgentAssignment resolves the assignment section for one agent (US-AD73 AC1/AC2).
//
// The board the picker is previewed for is the one holding the agent's most recent
// task — an agent belongs to a project, not a board, so there is no board to read
// off the agent itself. An agent with no tasks therefore gets an empty picker: the
// section has nothing to preview, which is the honest answer rather than picking an
// arbitrary board and showing a list that board's create-task modal would not show.
func (s *Service) AgentAssignment(ctx context.Context, orgID, agentID string) (AssignmentView, error) {
	if _, err := s.repo.GetAgent(ctx, agentID, orgID); err != nil {
		return AssignmentView{}, err
	}
	tasks, err := s.repo.ListAssignedTasks(ctx, orgID, agentID)
	if err != nil {
		return AssignmentView{}, err
	}
	view := AssignmentView{Tasks: tasks}
	if len(tasks) > 0 {
		view.BoardID = tasks[0].BoardID
		picker, err := s.repo.ListAssignableAgentsForBoard(ctx, orgID, view.BoardID)
		if err != nil {
			return AssignmentView{}, err
		}
		view.Picker = picker
	}
	hidden, err := s.repo.CountArchivedAgents(ctx, orgID)
	if err != nil {
		return AssignmentView{}, err
	}
	view.HiddenAgents = hidden
	return view, nil
}

// ListAssignableAgents is the create-task modal's picker for one board
// (US-AD11 AC1). It is the same repository call the agent detail screen makes,
// exposed on the board because that is the only scope a create-task modal has:
// `AgentAssignment` resolves a board from the agent's tasks, which cannot answer
// "who can I assign on *this* board" for an agent that holds none.
func (s *Service) ListAssignableAgents(ctx context.Context, orgID, boardID string) ([]AssignableAgent, error) {
	if _, err := s.repo.GetBoard(ctx, boardID, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListAssignableAgentsForBoard(ctx, orgID, boardID)
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

// UpdateAgent replaces every mutable field (US-AD96, US-AD106). It is a full
// update rather than a merge: the caller's request is the new state, so an
// omitted field lands on its default instead of silently keeping the old value.
// The handler is responsible for reading the current row and filling anything
// the request left out, which keeps that decision at the boundary where the
// request shape is known.
func (s *Service) UpdateAgent(ctx context.Context, a Agent) (Agent, error) {
	if err := validateAgent(a); err != nil {
		return Agent{}, err
	}
	return s.repo.UpdateAgent(ctx, a)
}

// ArchiveAgent retires an agent without deleting it (US-AD73). The running-task
// guard is the same one delete uses, and for the same reason (AC3): archiving
// an agent mid-run would leave the run without its retry and limit source. The
// run is NOT cancelled — the caller is told to finish it, which is what "409,
// not a severed run" means.
func (s *Service) ArchiveAgent(ctx context.Context, id, orgID string) (Agent, error) {
	running, err := s.repo.CountAgentRunningTasks(ctx, id, orgID)
	if err != nil {
		return Agent{}, err
	}
	if running > 0 {
		return Agent{}, ErrAgentHasRunningTask
	}
	return s.repo.ArchiveAgent(ctx, id, orgID)
}

// UnarchiveAgent returns an archived agent to service. No guard: putting an
// agent back can only add capacity, never strand a run.
func (s *Service) UnarchiveAgent(ctx context.Context, id, orgID string) (Agent, error) {
	return s.repo.UnarchiveAgent(ctx, id, orgID)
}

// ---- provider credentials (US-AD86) -----------------------------------------

// ValidateProvider rejects a `provider` that is not one of the three protocols
// the schema declares (the `providers_protocol_chk` CHECK), because that is what
// `agents.provider` holds: US-AD109 derives the value from `providers.protocol`
// and the request may only carry it for an agent that has no provider of its own.
//
// It used to accept the 18 vendor ids `pricing.Providers()` knows, and that was
// the wrong vocabulary. The price table is keyed by *vendor*; the schema is
// keyed by *protocol*; the two overlap only by accident (`anthropic` and
// `google` are both, `openai` is only the former). Accepting the vendor list
// meant `POST /projects/{id}/agents` stored `provider="deepseek"` — a value no
// other layer would accept, and one the form has not offered since the registry
// became the source of endpoint and credential. DECISIONS 6A.J records the
// mismatch; this is the fix.
//
// The model half of US-AD67 AC2 still reads the price table, which is what that
// table is for. Only the provider half changes vocabulary.
func ValidateProvider(provider string) error {
	if providerreg.AcceptableProtocol(provider) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUnknownProvider, provider)
}

// SetProviderKey stores an already-sealed credential and reports the agent's
// resulting has_provider_key. The sealing is the caller's job: internal/board
// must not hold the master key, because anything it holds it can also write to
// a log.
func (s *Service) SetProviderKey(ctx context.Context, id, orgID string, sealed []byte) (bool, error) {
	return s.repo.SetAgentProviderKey(ctx, id, orgID, sealed)
}

// ClearProviderKey revokes the credential, returning the agent to the
// deployment's environment key.
func (s *Service) ClearProviderKey(ctx context.Context, id, orgID string) (bool, error) {
	return s.repo.ClearAgentProviderKey(ctx, id, orgID)
}

// ProviderKey returns the sealed credential for an agent, or ErrNoProviderKey.
func (s *Service) ProviderKey(ctx context.Context, id, orgID string) ([]byte, error) {
	return s.repo.AgentProviderKey(ctx, id, orgID)
}

// CreateTask persists a new task on a board. The initial status is backlog
// unless the caller passes a status; ready is set by the dispatcher once the
// dependency gate passes, not by this method.
//
// `assigneeAgentID` is optional (US-AD11 AC5: a task with no agent is valid and
// still lands in backlog). When it is given it is resolved through the
// org-scoped `GetAgent` first: the foreign key only proves the id exists
// somewhere, so without this a member of one workspace could point their task at
// another workspace's agent by id.
func (s *Service) CreateTask(ctx context.Context, orgID, boardID, title, body, createdBy string, priority int, status TaskStatus, assigneeAgentID string) (Task, error) {
	if !validateName(title) {
		return Task{}, ErrInvalidInput
	}
	if status == "" {
		status = StatusBacklog
	}
	if priority < -10 || priority > 10 {
		return Task{}, ErrInvalidInput
	}
	if assigneeAgentID != "" {
		if _, err := s.repo.GetAgent(ctx, assigneeAgentID, orgID); err != nil {
			return Task{}, ErrInvalidInput
		}
	}
	t := Task{
		ID:              Must(),
		OrgID:           orgID,
		BoardID:         boardID,
		Title:           title,
		Body:            body,
		Status:          status,
		Priority:        priority,
		CreatedBy:       createdBy,
		AssigneeAgentID: assigneeAgentID,
		WorkspaceKind:   WorkspaceScratch,
		GoalMode:        "auto",
		GoalMaxTurns:    25,
	}
	return s.repo.CreateTask(ctx, t)
}

// GetTask loads a task scoped by org.
func (s *Service) GetTask(ctx context.Context, id, orgID string) (Task, error) {
	return s.repo.GetTask(ctx, id, orgID)
}

// ListBoardTasks lists the non-archived tasks of one board, narrowed by the
// contract's optional `status`, `assignee` and `search` filters.
func (s *Service) ListBoardTasks(ctx context.Context, orgID, boardID string, filter TaskFilter) ([]Task, error) {
	return s.repo.ListBoardTasks(ctx, orgID, boardID, filter)
}

// MoveTask applies one lifecycle transition with the optimistic-status guard:
// the caller states the status it believes the task is in, and the database
// only updates a row that still holds it. Two writers racing produce one winner
// and one ErrConflict, never a double move.
//
// Archiving is the one transition with rules of its own (US-AD59):
//
//	AC2 — only a terminal task may be archived. `blocked`, `ready`, `review`
//	      and the rest are still moving, and hiding a moving row takes work off
//	      the board that nobody finished. 409, because the payload is fine and
//	      the task's state is what refuses it.
//	AC3 — archiving an already-archived task is idempotent, and it needs no
//	      branch of its own: `archived` is itself terminal, so a retry passes
//	      AC2's check and the write below compares the row against the status
//	      just read — which matches. The caller's own `from` is deliberately not
//	      what the guard is given, which is the part that makes the retry a
//	      200 rather than a 409 on the caller's earlier write.
//
// AC4 (owner/admin) is enforced in the handler, not here: the role is not part
// of the domain state, and `MoveTask` is also reached by the dispatcher paths
// that hold no role at all.
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
		current, err := s.repo.GetTask(ctx, id, orgID)
		if err != nil {
			return Task{}, err
		}
		if !current.Status.IsTerminal() {
			return Task{}, ErrArchiveRequiresTerminal
		}
		// `current.Status`, not the caller's `from`: the guard's job here is to
		// catch a concurrent writer between the read above and this write, not
		// to re-check a status the caller stated before the read.
		return s.repo.UpdateTaskStatus(ctx, id, orgID, current.Status, to)
	}
	return s.repo.UpdateTaskStatus(ctx, id, orgID, from, to)
}

// CancelTask implements POST /tasks/{id}/cancel.
//
// Two halves, in this order, and the order matters:
//
//  1. Record the cancel against the live run, durably. This is what the
//     dispatcher reads. A cancel written only into this process's memory would
//     be a cancel that silently does nothing whenever the API and the dispatcher
//     run as different processes — which §66's `-role=api|dispatcher` allows.
//  2. Move the task to `cancelled`.
//
// When the task is already `cancelled`, step 2 is skipped: US-AD59 AC3's
// reasoning applies here too — asking to cancel something already cancelled is
// the caller's request already satisfied, not a conflict. When the task is
// terminal in another way (`done`, `failed`, `archived`), it is refused rather
// than silently overwritten: cancelling finished work would rewrite history.
func (s *Service) CancelTask(ctx context.Context, id, orgID string) (Task, error) {
	task, err := s.repo.GetTask(ctx, id, orgID)
	if err != nil {
		return Task{}, err
	}
	if task.Status == StatusCancelled {
		return task, nil
	}
	if task.Status.IsTerminal() {
		return Task{}, ErrConflict
	}

	// No live run is not a problem: a `backlog` or `blocked` task has nothing to
	// abort, and cancelling it is still meaningful. Only the task row moves.
	if task.CurrentRunID != "" {
		// The returned timestamp is deliberately discarded: the API reports the
		// task, not the run, because the run's outcome is the dispatcher's to
		// decide. Keeping the value would invite reporting it as if this handler
		// had observed the run end.
		if _, err := s.repo.RequestRunCancel(ctx, task.CurrentRunID, orgID); err != nil {
			// The run was closed between the read above and this write. The task
			// is still ours to cancel and the executor will not pick it up again,
			// so this is not a failure — it just means there was nothing to stop.
			if !errors.Is(err, ErrNotFound) {
				return Task{}, err
			}
		}
	}
	return s.repo.UpdateTaskStatus(ctx, id, orgID, task.Status, StatusCancelled)
}

// RetryTask implements POST /tasks/{id}/retry.
//
// This is the operator's override, not the dispatcher's automatic retry (§10.2).
// The distinction is the whole point of the endpoint: §10.1 sends `capability`
// and `policy` failures to "No-Retry" because retrying them unchanged spends
// money to learn nothing. But the human who has just fixed the credential or
// registered the missing tool is holding information the taxonomy does not have,
// and without this endpoint their only path is to move the task to `backlog` and
// back by hand.
//
// It therefore accepts any non-running status. `running` is refused: a live run
// holds the task, and returning it to `ready` would let it be claimed again
// while the first run is still writing steps.
func (s *Service) RetryTask(ctx context.Context, id, orgID string) (Task, error) {
	// The existence check is not redundant with the UPDATE's own WHERE clause.
	// The statement's guard refuses a `running` task by matching zero rows, so
	// without this a task that does not exist and a task that is mid-run are
	// indistinguishable — both would be a 409. That is an existence oracle: the
	// caller learns which ids exist from the status code. Read first, then act,
	// so 404 means "no such task here" and 409 means "this one is busy".
	if _, err := s.repo.GetTask(ctx, id, orgID); err != nil {
		return Task{}, err
	}
	return s.repo.RetryTask(ctx, id, orgID)
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
// Claim is the dispatcher's batch claim. Each claimed task gets its own run id:
// sharing one id across a batch left every task but the first without a run row,
// so a caller that passed a single id would half-claim the batch.
//
// runIDs is the caller's own list, sized to `limit`; the SQL takes the first
// len(rows) entries by row number. The returned tasks carry the run id that was
// written into each `current_run_id`, so the dispatcher never has to guess which
// id went to which row.
func (s *Service) Claim(ctx context.Context, orgID, boardID string, runIDs []string, limit int) ([]Task, error) {
	if limit <= 0 || len(runIDs) < limit {
		// A short list would leave rows with a NULL run id: claimed and
		// unrunnable at the same time.
		return nil, ErrInvalidInput
	}
	return s.repo.ClaimReadyTasks(ctx, orgID, boardID, runIDs, limit)
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
