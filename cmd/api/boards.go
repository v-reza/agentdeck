package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
	"agentdeck/internal/modelprice"
	"agentdeck/internal/providerreg"
)

// boardAPI wires the M1 board domain to HTTP. It rides the same authAPI as the
// auth and org routes so tenant resolution and the role gate stay in one place:
// every board route is registered through boardRoute, which chains
// authentication + tenant resolution + role gate exactly like orgRoute.
type boardAPI struct {
	svc *board.Service
	// providers is the workspace provider registry (US-AD109). Phase 5 made the
	// agent form choose a provider instead of typing an endpoint, so the write
	// path has to resolve that choice into the protocol and base URL the row
	// stores — and refuse a provider id from another workspace. It is optional
	// so the RBAC tests, which mount these routes without a registry, still
	// exercise the role gates.
	providers *providerreg.Service
	// modelPrices is tier 1 of the price resolution (DECISIONS 6A.C): the
	// workspace's manual overrides. The catalog handler reads it so the rate it
	// reports is the rate that will actually be billed — without it the endpoint
	// would quote the table price for a model whose real cost is an override.
	// Optional for the same reason as providers: the RBAC tests mount the routes
	// without one and still have to exercise the gates.
	modelPrices *modelprice.Service
}

// writeBoardError maps a domain error to its stable HTTP code so the same
// failure always yields the same response from every path.
func writeBoardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, board.ErrSlugTaken):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, board.ErrNameTaken):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, board.ErrColumnNameTaken), errors.Is(err, board.ErrColumnHasTasks):
		http.Error(w, err.Error(), http.StatusConflict)
	// US-AD20: a duplicate agent name and a delete-while-running are both
	// conflicts the operator resolves by changing what they sent, not by
	// fixing a malformed payload.
	case errors.Is(err, board.ErrAgentNameTaken), errors.Is(err, board.ErrAgentHasRunningTask):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, board.ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, board.ErrCycleDetected):
		http.Error(w, err.Error(), http.StatusConflict)
	// providerreg.ErrInvalidInput is the provider registry's counterpart to the
	// board sentinel above: a malformed name, an unknown protocol, or — the case
	// the models probe relies on — a base URL the SSRF guard refused. The fix is
	// a different address, so it is a 400 on every route that reaches it, not a
	// 500 that reads as "the server broke".
	case errors.Is(err, board.ErrColumnsInvalid), errors.Is(err, board.ErrBudgetInvalid),
		errors.Is(err, board.ErrInvalidInput), errors.Is(err, board.ErrInvalidStatus),
		errors.Is(err, providerreg.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, board.ErrArchiveRequiresAdmin):
		http.Error(w, err.Error(), http.StatusForbidden)
	// US-AD86 AC3: an unknown provider is the caller's mistake and the fix is a
	// different provider, so it is a 400 rather than a 409. Same for US-AD67
	// AC2's unpriced model — the fix is a different model.
	case errors.Is(err, board.ErrUnknownProvider), errors.Is(err, board.ErrUnknownModel),
		errors.Is(err, board.ErrNoProviderKey),
		errors.Is(err, board.ErrProviderNotProbeable):
		http.Error(w, err.Error(), http.StatusBadRequest)
	// The upstream provider refused us (US-AD86 validate). 502, not 500: our
	// service is fine, the thing we called is not.
	case errors.Is(err, board.ErrProviderHandshakeFailed):
		http.Error(w, err.Error(), http.StatusBadGateway)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a boardAPI) boardContext(r *http.Request) (orgContext, error) {
	return currentOrgContext(r)
}

// ---- projects --------------------------------------------------------------

type projectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type projectResponse struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func toProjectResponse(p board.Project) projectResponse {
	return projectResponse{
		ID:        p.ID,
		OrgID:     p.OrgID,
		Slug:      p.Slug,
		Name:      p.Name,
		CreatedAt: p.CreatedAt.Format(time.RFC3339Nano),
	}
}

// registerBoardRoutes mounts every board-domain route. It exists as one
// function so the role gate on each route is declared exactly once, in the same
// place main.go wires it and in the same place the RBAC test drives it: a copy
// of the table in a test would keep passing after main.go was loosened.
//
// Each route chains authentication + tenant resolution + the role gate, so no
// board handler can be registered without all three.
func registerBoardRoutes(mux *http.ServeMux, api authAPI, svc *board.Service, providers *providerreg.Service) {
	boardAPI := boardAPI{svc: svc, providers: providers}
	boardRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}
	// US-AD08 AC3: creating a project is owner/admin. ARCHITECTURE 6.2.5 lists
	// Member for this route; the PRD acceptance criterion is the binding one and
	// PRD 12 names US-AD08..AD15 as the stories whose permission AC governs.
	boardRoute("POST /api/v1/projects", http.HandlerFunc(boardAPI.createProject), auth.Admin)
	boardRoute("GET /api/v1/projects", http.HandlerFunc(boardAPI.listProjects), auth.Viewer)
	boardRoute("GET /api/v1/projects/{id}", http.HandlerFunc(boardAPI.getProject), auth.Viewer)
	boardRoute("PATCH /api/v1/projects/{id}", http.HandlerFunc(boardAPI.updateProject), auth.Member)
	boardRoute("DELETE /api/v1/projects/{id}", http.HandlerFunc(boardAPI.deleteProject), auth.Admin)
	// US-AD09 AC3: creating a board is owner/admin, the same rule US-AD08 AC3
	// applies one level up. ARCHITECTURE 6.2.6 lists Member here; PRD section 12
	// names US-AD08..AD15 as the stories whose permission AC is authoritative, so
	// the PRD reading wins — and it is the consistent one, since a member may not
	// create the project that would hold the board.
	boardRoute("POST /api/v1/projects/{project_id}/boards", http.HandlerFunc(boardAPI.createBoard), auth.Admin)
	boardRoute("GET /api/v1/projects/{project_id}/boards", http.HandlerFunc(boardAPI.listBoards), auth.Viewer)
	boardRoute("GET /api/v1/boards/{id}", http.HandlerFunc(boardAPI.getBoard), auth.Viewer)
	// PATCH /boards/{id} stays Member: it carries name/slug/budget, and neither
	// US-AD83 (rename) nor US-AD84 (budget) raises the role, so nothing here
	// justifies the change ARCHITECTURE 6.2.6 does not ask for either.
	boardRoute("PATCH /api/v1/boards/{id}", http.HandlerFunc(boardAPI.updateBoard), auth.Member)
	boardRoute("DELETE /api/v1/boards/{id}", http.HandlerFunc(boardAPI.deleteBoard), auth.Admin)
	// US-AD10 AC4: editing the board layout is owner/admin. This is a separate
	// route from PATCH /boards/{id} exactly as ARCHITECTURE 6.2.6 lists it,
	// which is what lets the layout be admin-gated without dragging the rename
	// and budget update up with it. ARCHITECTURE lists Member for this path;
	// PRD section 12 names US-AD08..AD15 as the stories whose permission AC is
	// authoritative, and US-AD10 AC4 is explicit, so the AC wins — the same
	// reading already applied to POST /projects and POST /boards.
	boardRoute("GET /api/v1/boards/{id}/columns", http.HandlerFunc(boardAPI.getBoardColumns), auth.Viewer)
	boardRoute("PATCH /api/v1/boards/{id}/columns", http.HandlerFunc(boardAPI.updateBoardColumns), auth.Admin)
	boardRoute("POST /api/v1/boards/{board_id}/tasks", http.HandlerFunc(boardAPI.createTask), auth.Member)
	boardRoute("GET /api/v1/boards/{board_id}/tasks", http.HandlerFunc(boardAPI.listTasks), auth.Viewer)
	boardRoute("GET /api/v1/tasks/{id}", http.HandlerFunc(boardAPI.getTask), auth.Viewer)
	boardRoute("PATCH /api/v1/tasks/{id}", http.HandlerFunc(boardAPI.updateTask), auth.Member)
	// US-AD80 AC2: deleting a task is owner/admin, and ARCHITECTURE 6.2.6's
	// Role Min column says Admin too — this one line was the only disagreement.
	boardRoute("DELETE /api/v1/tasks/{id}", http.HandlerFunc(boardAPI.deleteTask), auth.Admin)
	boardRoute("POST /api/v1/tasks/{id}/move", http.HandlerFunc(boardAPI.moveTask), auth.Member)
	boardRoute("POST /api/v1/tasks/{id}/assign", http.HandlerFunc(boardAPI.assignTask), auth.Member)
	boardRoute("POST /api/v1/tasks/{id}/links", http.HandlerFunc(boardAPI.createLink), auth.Member)
	boardRoute("DELETE /api/v1/tasks/{id}/links/{parent_id}", http.HandlerFunc(boardAPI.deleteLink), auth.Member)
	boardRoute("GET /api/v1/tasks/{id}/links", http.HandlerFunc(boardAPI.listLinks), auth.Viewer)
	boardRoute("GET /api/v1/tasks/{id}/dag", http.HandlerFunc(boardAPI.taskDag), auth.Viewer)

	// US-AD20 agent registry. Roles come from the ARCHITECTURE route table:
	// registering is Member, reading is Viewer, deleting is Admin. The create
	// route hangs off the project because an agent name is unique per project.
	boardRoute("GET /api/v1/projects/{project_id}/agents", http.HandlerFunc(boardAPI.listAgents), auth.Viewer)
	boardRoute("POST /api/v1/projects/{project_id}/agents", http.HandlerFunc(boardAPI.createAgent), auth.Member)
	boardRoute("GET /api/v1/agents/{id}", http.HandlerFunc(boardAPI.getAgent), auth.Viewer)
	boardRoute("DELETE /api/v1/agents/{id}", http.HandlerFunc(boardAPI.deleteAgent), auth.Admin)
}

// POST /api/v1/projects — create a project in the caller's active org.
// The org comes from the resolved tenant context, never from the body, so a
// caller cannot create a project in another tenant's org.
func (a boardAPI) createProject(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	p, err := a.svc.CreateProject(r.Context(), orgCtx.workspace.ID, req.Slug, req.Name)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toProjectResponse(p))
}

// GET /api/v1/projects — list the projects of the caller's active org.
func (a boardAPI) listProjects(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	projects, err := a.svc.ListProjects(r.Context(), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GET /api/v1/projects/{id}
func (a boardAPI) getProject(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	p, err := a.svc.GetProject(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProjectResponse(p))
}

// PATCH /api/v1/projects/{id}
func (a boardAPI) updateProject(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := a.svc.UpdateProjectName(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, req.Name); err != nil {
		writeBoardError(w, err)
		return
	}
	p, err := a.svc.GetProject(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProjectResponse(p))
}

// DELETE /api/v1/projects/{id}
func (a boardAPI) deleteProject(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.svc.DeleteProject(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- boards ----------------------------------------------------------------

type boardRequest struct {
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Columns      []board.Column `json:"columns"`
	BudgetMicros *int64         `json:"budget_daily_micros"`
}

type boardResponse struct {
	ID                string         `json:"id"`
	OrgID             string         `json:"org_id"`
	ProjectID         string         `json:"project_id"`
	Slug              string         `json:"slug"`
	Name              string         `json:"name"`
	Columns           []board.Column `json:"columns"`
	BudgetDailyMicros int64          `json:"budget_daily_micros"`
	CreatedAt         string         `json:"created_at"`
}

func toBoardResponse(b board.Board) boardResponse {
	return boardResponse{
		ID:                b.ID,
		OrgID:             b.OrgID,
		ProjectID:         b.ProjectID,
		Slug:              b.Slug,
		Name:              b.Name,
		Columns:           b.Columns,
		BudgetDailyMicros: b.BudgetDailyMicros,
		CreatedAt:         b.CreatedAt.Format(time.RFC3339Nano),
	}
}

// POST /api/v1/projects/{project_id}/boards
func (a boardAPI) createBoard(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req boardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	budget := int64(board.DefaultBudgetDailyMicros)
	if req.BudgetMicros != nil {
		budget = *req.BudgetMicros
	}
	b, err := a.svc.CreateBoard(
		r.Context(),
		orgCtx.workspace.ID,
		r.PathValue("project_id"),
		req.Slug,
		req.Name,
		req.Columns,
		budget,
	)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toBoardResponse(b))
}

// GET /api/v1/projects/{project_id}/boards
func (a boardAPI) listBoards(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	boards, err := a.svc.ListBoards(r.Context(), orgCtx.workspace.ID, r.PathValue("project_id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]boardResponse, 0, len(boards))
	for _, b := range boards {
		out = append(out, toBoardResponse(b))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GET /api/v1/boards/{id}
func (a boardAPI) getBoard(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	b, err := a.svc.GetBoard(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toBoardResponse(b))
}

// PATCH /api/v1/boards/{id} — name and budget. Only the fields that are present
// are applied, so a budget change does not require re-sending the name.
//
// It deliberately does NOT accept `columns`. The layout has its own route with
// its own role gate (US-AD10 AC4: Admin), and leaving a second write path here
// would let a Member edit the layout through the Member-gated route — the gate
// would be decorative. ARCHITECTURE 6.2.6 lists this route as
// `{name, slug, budget_daily_micros}` for the same reason.
func (a boardAPI) updateBoard(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req boardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	orgID := orgCtx.workspace.ID
	if req.Name != "" {
		if err := a.svc.UpdateBoardName(r.Context(), id, orgID, req.Name); err != nil {
			writeBoardError(w, err)
			return
		}
	}
	if req.BudgetMicros != nil {
		if err := a.svc.UpdateBoardBudget(r.Context(), id, orgID, *req.BudgetMicros); err != nil {
			writeBoardError(w, err)
			return
		}
	}
	b, err := a.svc.GetBoard(r.Context(), id, orgID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toBoardResponse(b))
}

// DELETE /api/v1/boards/{id}
func (a boardAPI) deleteBoard(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := a.svc.DeleteBoard(r.Context(), id, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- columns (US-AD10) -------------------------------------------------------

type columnsRequest struct {
	Columns []board.Column `json:"columns"`
}

// GET /api/v1/boards/{id}/columns — the board layout on its own.
//
// It exists as its own route because the layout has a different role gate from
// the rest of the board record (ARCHITECTURE 6.2.6): reading it is Viewer,
// editing it is Admin. A single PATCH /boards/{id} could not express that split.
func (a boardAPI) getBoardColumns(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	b, err := a.svc.GetBoard(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	if b.Columns == nil {
		b.Columns = []board.Column{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(b.Columns)
}

// PATCH /api/v1/boards/{id}/columns — replace the layout (US-AD10 AC1/AC2/AC3).
//
// The request must carry the *whole* layout, not a diff: the layout is an
// ordered list, so "the column at index 2" is only meaningful against the list
// the caller was looking at. A partial update would make a concurrent reorder
// silently drop a column instead of losing an optimistic race.
func (a boardAPI) updateBoardColumns(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req columnsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	orgID := orgCtx.workspace.ID
	if err := a.svc.UpdateBoardColumns(r.Context(), id, orgID, req.Columns); err != nil {
		writeBoardError(w, err)
		return
	}
	b, err := a.svc.GetBoard(r.Context(), id, orgID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	if b.Columns == nil {
		b.Columns = []board.Column{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(b.Columns)
}

// ---- tasks -----------------------------------------------------------------

type taskRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Priority *int   `json:"priority"`
	Status   string `json:"status"`
}

type taskResponse struct {
	ID                  string `json:"id"`
	OrgID               string `json:"org_id"`
	BoardID             string `json:"board_id"`
	Title               string `json:"title"`
	Body                string `json:"body"`
	Status              string `json:"status"`
	Priority            int    `json:"priority"`
	AssigneeAgentID     string `json:"assignee_agent_id"`
	CreatedBy           string `json:"created_by"`
	IdempotencyKey      string `json:"idempotency_key"`
	BlockKind           string `json:"block_kind"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	WorkspaceKind       string `json:"workspace_kind"`
	GoalMode            string `json:"goal_mode"`
	GoalMaxTurns        int    `json:"goal_max_turns"`
	CurrentRunID        string `json:"current_run_id"`
	CostMicros          int64  `json:"cost_micros"`
	TokensIn            int64  `json:"tokens_in"`
	TokensOut           int64  `json:"tokens_out"`
	CreatedAt           string `json:"created_at"`
	StartedAt           string `json:"started_at"`
	CompletedAt         string `json:"completed_at"`
	ArchivedAt          string `json:"archived_at"`
}

func toTaskResponse(t board.Task) taskResponse {
	started := ""
	if t.StartedAt != nil {
		started = t.StartedAt.Format(time.RFC3339Nano)
	}
	completed := ""
	if t.CompletedAt != nil {
		completed = t.CompletedAt.Format(time.RFC3339Nano)
	}
	archived := ""
	if t.ArchivedAt != nil {
		archived = t.ArchivedAt.Format(time.RFC3339Nano)
	}
	return taskResponse{
		ID:                  t.ID,
		OrgID:               t.OrgID,
		BoardID:             t.BoardID,
		Title:               t.Title,
		Body:                t.Body,
		Status:              string(t.Status),
		Priority:            t.Priority,
		AssigneeAgentID:     t.AssigneeAgentID,
		CreatedBy:           t.CreatedBy,
		IdempotencyKey:      t.IdempotencyKey,
		BlockKind:           t.BlockKind,
		ConsecutiveFailures: t.ConsecutiveFailures,
		WorkspaceKind:       string(t.WorkspaceKind),
		GoalMode:            t.GoalMode,
		GoalMaxTurns:        t.GoalMaxTurns,
		CurrentRunID:        t.CurrentRunID,
		CostMicros:          t.CostMicros,
		TokensIn:            t.TokensIn,
		TokensOut:           t.TokensOut,
		CreatedAt:           t.CreatedAt.Format(time.RFC3339Nano),
		StartedAt:           started,
		CompletedAt:         completed,
		ArchivedAt:          archived,
	}
}

// POST /api/v1/boards/{board_id}/tasks
func (a boardAPI) createTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	status := board.StatusBacklog
	if req.Status != "" {
		if !board.AcceptableStatus(req.Status) {
			http.Error(w, "unsupported task status", http.StatusBadRequest)
			return
		}
		status = board.TaskStatus(req.Status)
	}
	t, err := a.svc.CreateTask(
		r.Context(),
		orgCtx.workspace.ID,
		r.PathValue("board_id"),
		req.Title,
		req.Body,
		orgCtx.email,
		priority,
		status,
	)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toTaskResponse(t))
}

// GET /api/v1/boards/{board_id}/tasks
func (a boardAPI) listTasks(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	tasks, err := a.svc.ListBoardTasks(r.Context(), orgCtx.workspace.ID, r.PathValue("board_id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toTaskResponse(t))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GET /api/v1/tasks/{id}
func (a boardAPI) getTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	t, err := a.svc.GetTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toTaskResponse(t))
}

// PATCH /api/v1/tasks/{id}
func (a boardAPI) updateTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	t, err := a.svc.UpdateTaskFields(
		r.Context(), r.PathValue("id"), orgCtx.workspace.ID,
		req.Title, req.Body, priority,
	)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toTaskResponse(t))
}

// DELETE /api/v1/tasks/{id}
func (a boardAPI) deleteTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.svc.DeleteTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// moveRequest carries one lifecycle transition. The caller states the status it
// believes the task holds so the optimistic guard in the repository can reject
// a race instead of applying a double move.
type moveRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// POST /api/v1/tasks/{id}/move
func (a boardAPI) moveTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.To == "" {
		http.Error(w, "target status is required", http.StatusBadRequest)
		return
	}
	if !board.AcceptableStatus(req.To) {
		http.Error(w, "unsupported task status", http.StatusBadRequest)
		return
	}
	from := board.StatusBacklog
	if req.From != "" {
		from = board.TaskStatus(req.From)
	}
	t, err := a.svc.MoveTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, from, board.TaskStatus(req.To))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toTaskResponse(t))
}

// assignRequest sets or clears the agent that will run the task.
type assignRequest struct {
	AgentID string `json:"agent_id"`
}

// POST /api/v1/tasks/{id}/assign
func (a boardAPI) assignTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req assignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	t, err := a.svc.AssignTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, req.AgentID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toTaskResponse(t))
}

// ---- dependency links --------------------------------------------------------

type linkRequest struct {
	ParentID string `json:"parent_id"`
}

// POST /api/v1/tasks/{id}/links — add one dependency edge. The child is the
// task in the path; the parent is the task it waits on.
func (a boardAPI) createLink(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	childID := r.PathValue("id")
	if _, err := a.svc.GetTask(r.Context(), childID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	if _, err := a.svc.GetTask(r.Context(), req.ParentID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	ready, err := a.svc.CreateLink(r.Context(), req.ParentID, childID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"parent_id": req.ParentID,
		"child_id":  childID,
		"ready":     ready,
	})
}

// DELETE /api/v1/tasks/{id}/links/{parent_id}
func (a boardAPI) deleteLink(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	childID := r.PathValue("id")
	parentID := r.PathValue("parent_id")
	if _, err := a.svc.GetTask(r.Context(), childID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	if _, err := a.svc.GetTask(r.Context(), parentID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	ready, err := a.svc.DeleteLink(r.Context(), parentID, childID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"parent_id": r.PathValue("parent_id"),
		"child_id":  childID,
		"ready":     ready,
	})
}

// GET /api/v1/tasks/{id}/links — the task's dependency edges.
func (a boardAPI) listLinks(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	taskID := r.PathValue("id")
	if _, err := a.svc.GetTask(r.Context(), taskID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	parents, children, err := a.svc.TaskLinks(r.Context(), taskID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"parents":  parents,
		"children": children,
	})
}

// GET /api/v1/tasks/{id}/dag — the dependency closure for this task.
func (a boardAPI) taskDag(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	t, err := a.svc.GetTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	parents, err := a.svc.TaskParents(r.Context(), t.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task":    toTaskResponse(t),
		"parents": parents,
	})
}
