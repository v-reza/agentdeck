package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
)

// M4 runtime HTTP surface: the run lifecycle, the trace, and the ledger.
//
// The write routes are `Internal/Key`, role Worker in the contract's table.
// That column is a description of *who calls them*, not a role this codebase
// enforces — there is no Worker role in `memberships` (role is
// owner/admin/member/viewer). The org-header gate is what actually authorises
// them, and the holder check that matters is the claim: a worker can only write
// to the run its task points at (see board.Service.StartRun / EndRun).
//
// ponytail: no separate worker credential check beyond the org header, because
// the executor does not exist yet. Add it when a worker binary is issued its own
// key — the seam is `runs.claim_lock`, which is already written by the claim.

// registerRunRoutes is called from registerBoardRoutes with the same api and
// service, so the role gate on each run route is declared once, next to the
// board routes it belongs to.
func registerRunRoutes(mux *http.ServeMux, api authAPI, svc *board.Service, bAPI boardAPI) {
	boardAPI := bAPI
	boardRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}

	// Run lifecycle. Reading is Viewer; the worker-facing writes are Member,
	// the lowest role that is still a member of the workspace.
	// US-AD21 manual force claim: the entry point that starts a run without the
	// (not yet built) dispatcher loop.
	boardRoute("POST /api/v1/tasks/{id}/claim", http.HandlerFunc(boardAPI.claimTask), auth.Member)
	boardRoute("GET /api/v1/tasks/{id}/runs", http.HandlerFunc(boardAPI.listTaskRuns), auth.Viewer)
	boardRoute("GET /api/v1/runs/{id}", http.HandlerFunc(boardAPI.getRun), auth.Viewer)
	boardRoute("POST /api/v1/runs/{id}/heartbeat", http.HandlerFunc(boardAPI.heartbeatRun), auth.Member)
	boardRoute("POST /api/v1/runs/{id}/end", http.HandlerFunc(boardAPI.endRun), auth.Member)

	// Trace.
	boardRoute("GET /api/v1/runs/{id}/steps", http.HandlerFunc(boardAPI.listRunSteps), auth.Viewer)
	boardRoute("POST /api/v1/runs/{id}/steps", http.HandlerFunc(boardAPI.createRunStep), auth.Member)
	boardRoute("PATCH /api/v1/runs/{id}/steps/{seq}", http.HandlerFunc(boardAPI.finishRunStep), auth.Member)

	// Ledger (US-AD32 cost reporting).
	boardRoute("GET /api/v1/runs/{id}/ledger", http.HandlerFunc(boardAPI.listRunLedger), auth.Viewer)
	boardRoute("GET /api/v1/boards/{id}/ledger", http.HandlerFunc(boardAPI.listBoardLedger), auth.Viewer)
}

// writeJSONResponse is the encoder every run route uses. It exists because
// there are ten of them: writing the header and the encoder inline ten times is
// ten chances to forget the Content-Type, which a client sees as a text/plain
// body it cannot parse.
func writeJSONResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// pathInt reads a numeric path segment. A non-numeric `seq` is the caller's
// mistake, so it is a 400 from the handler rather than a parse error surfacing
// as a 500.
func pathInt(r *http.Request, name string) (int, error) {
	return strconv.Atoi(r.PathValue(name))
}

type runResponse struct {
	ID              string `json:"id"`
	OrgID           string `json:"org_id"`
	TaskID          string `json:"task_id"`
	AgentID         string `json:"agent_id"`
	Attempt         int    `json:"attempt"`
	Status          string `json:"status"`
	Outcome         string `json:"outcome"`
	FailureKind     string `json:"failure_kind"`
	LastHeartbeatAt string `json:"last_heartbeat_at"`
	MaxRuntimeSecs  int    `json:"max_runtime_seconds"`
	CostMicros      int64  `json:"cost_micros"`
	TokensIn        int64  `json:"tokens_in"`
	TokensOut       int64  `json:"tokens_out"`
	Summary         string `json:"summary"`
	Error           string `json:"error"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
}

func toRunResponse(run board.Run) runResponse {
	beat := ""
	if run.LastHeartbeatAt != nil {
		beat = run.LastHeartbeatAt.Format(time.RFC3339Nano)
	}
	ended := ""
	if run.EndedAt != nil {
		ended = run.EndedAt.Format(time.RFC3339Nano)
	}
	return runResponse{
		ID:              run.ID,
		OrgID:           run.OrgID,
		TaskID:          run.TaskID,
		AgentID:         run.AgentID,
		Attempt:         run.Attempt,
		Status:          string(run.Status),
		Outcome:         run.Outcome,
		FailureKind:     run.FailureKind,
		LastHeartbeatAt: beat,
		MaxRuntimeSecs:  run.MaxRuntimeSecs,
		CostMicros:      run.CostMicros,
		TokensIn:        run.TokensIn,
		TokensOut:       run.TokensOut,
		Summary:         run.Summary,
		Error:           run.Error,
		StartedAt:       run.StartedAt.Format(time.RFC3339Nano),
		EndedAt:         ended,
	}
}

type stepResponse struct {
	Seq         int     `json:"seq"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	TokensIn    int64   `json:"tokens_in"`
	TokensOut   int64   `json:"tokens_out"`
	CostMicros  int64   `json:"cost_micros"`
	StartedAt   string  `json:"started_at"`
	EndedAt     string  `json:"ended_at"`
	PayloadJSON *string `json:"payload_json"`
}

func toStepResponse(s board.Step) stepResponse {
	ended := ""
	if s.EndedAt != nil {
		ended = s.EndedAt.Format(time.RFC3339Nano)
	}
	// The payload is echoed as the JSON that came back from `payload_json`. It is
	// already normalised by the column type (JSONB reorders keys and rewrites
	// whitespace), so the handler must not normalise it *again* — but it also
	// must not claim byte fidelity it cannot deliver.
	var payload *string
	if len(s.PayloadJSON) > 0 {
		raw := string(s.PayloadJSON)
		payload = &raw
	}
	return stepResponse{
		Seq:         s.Seq,
		Kind:        s.Kind,
		Name:        s.Name,
		Status:      s.Status,
		TokensIn:    s.TokensIn,
		TokensOut:   s.TokensOut,
		CostMicros:  s.CostMicros,
		StartedAt:   s.StartedAt.Format(time.RFC3339Nano),
		EndedAt:     ended,
		PayloadJSON: payload,
	}
}

type ledgerEntryResponse struct {
	ID               int64  `json:"id"`
	RunID            string `json:"run_id"`
	TaskID           string `json:"task_id"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Kind             string `json:"kind"`
	TokensIn         int64  `json:"tokens_in"`
	TokensOut        int64  `json:"tokens_out"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	CostMicros       int64  `json:"cost_micros"`
	PriceVersion     int    `json:"price_version"`
	PriceSource      string `json:"price_source"`
	PricingModel     string `json:"pricing_model"`
	CreatedAt        string `json:"created_at"`
}

func toLedgerResponse(e board.LedgerEntry) ledgerEntryResponse {
	return ledgerEntryResponse{
		ID:               e.ID,
		RunID:            e.RunID,
		TaskID:           e.TaskID,
		Provider:         e.Provider,
		Model:            e.Model,
		Kind:             e.Kind,
		TokensIn:         e.TokensIn,
		TokensOut:        e.TokensOut,
		CacheReadTokens:  e.CacheReadTokens,
		CacheWriteTokens: e.CacheWriteTokens,
		ReasoningTokens:  e.ReasoningTokens,
		CostMicros:       e.CostMicros,
		PriceVersion:     e.PriceVersion,
		PriceSource:      e.PriceSource,
		PricingModel:     e.PricingModel,
		CreatedAt:        e.CreatedAt.Format(time.RFC3339Nano),
	}
}

// GET /api/v1/tasks/{id}/runs — every attempt on a task, newest first.
func (a boardAPI) listTaskRuns(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	// The task is read first so a task id from another workspace is a 404 rather
	// than an empty list, which would leak that the id exists somewhere.
	if _, err := a.svc.GetTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	runs, err := a.svc.TaskRuns(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]runResponse, 0, len(runs))
	for _, run := range runs {
		out = append(out, toRunResponse(run))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// GET /api/v1/runs/{id} — one run.
func (a boardAPI) getRun(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	run, err := a.svc.GetRun(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toRunResponse(run))
}

// POST /api/v1/runs/{id}/heartbeat — N7, every 60s. A run that is no longer
// live answers 404, which is how a reclaimed worker is told to stop.
func (a boardAPI) heartbeatRun(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	run, err := a.svc.Heartbeat(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toRunResponse(run))
}

type endRunRequest struct {
	Outcome     string `json:"outcome"`
	FailureKind string `json:"failure_kind"`
	Summary     string `json:"summary"`
	Error       string `json:"error"`
}

// POST /api/v1/runs/{id}/end — the worker reports its result. The task's own
// transition is decided by the service from the outcome, not sent by the worker:
// a worker that could name its task's next status could move it straight to
// `done` and erase the human review step (US-AD22 AC2).
func (a boardAPI) endRun(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req endRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	run, err := a.svc.EndRun(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, board.RunSummary{
		Outcome:     req.Outcome,
		FailureKind: req.FailureKind,
		Summary:     req.Summary,
		Error:       req.Error,
	})
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toRunResponse(run))
}

// GET /api/v1/runs/{id}/steps — the trace, in `seq` order.
func (a boardAPI) listRunSteps(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if _, err := a.svc.GetRun(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	steps, err := a.svc.RunSteps(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]stepResponse, 0, len(steps))
	for _, s := range steps {
		out = append(out, toStepResponse(s))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

type stepRequest struct {
	Seq     int             `json:"seq"`
	Kind    string          `json:"kind"`
	Name    string          `json:"name"`
	Status  string          `json:"status"`
	Tokens  *stepTokens     `json:"tokens"`
	Cost    int64           `json:"cost_micros"`
	Payload json.RawMessage `json:"payload"`
}

type stepTokens struct {
	In  int64 `json:"in"`
	Out int64 `json:"out"`
}

// POST /api/v1/runs/{id}/steps — open one trace line.
func (a boardAPI) createRunStep(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var payload any
	if len(req.Payload) > 0 {
		payload = req.Payload
	}
	step, err := a.svc.StartStep(r.Context(), orgCtx.workspace.ID, r.PathValue("id"), board.Step{
		Seq: req.Seq, Kind: req.Kind, Name: req.Name,
	}, payload)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusCreated, toStepResponse(step))
}

// PATCH /api/v1/runs/{id}/steps/{seq} — close one trace line with its cost.
func (a boardAPI) finishRunStep(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	seq, err := pathInt(r, "seq")
	if err != nil {
		http.Error(w, "invalid step sequence", http.StatusBadRequest)
		return
	}
	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	step := board.Step{Status: req.Status, CostMicros: req.Cost}
	if req.Tokens != nil {
		step.TokensIn = req.Tokens.In
		step.TokensOut = req.Tokens.Out
	}
	finished, err := a.svc.FinishStep(r.Context(), orgCtx.workspace.ID, r.PathValue("id"), seq, step)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toStepResponse(finished))
}

// GET /api/v1/runs/{id}/ledger — the priced calls of one run.
func (a boardAPI) listRunLedger(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if _, err := a.svc.GetRun(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	entries, err := a.svc.RunLedger(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]ledgerEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toLedgerResponse(e))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

type boardLedgerResponse struct {
	// SpendTodayMicros is the number the US-AD32 cost gate reads, served next to
	// the rows so the screen and the gate cannot show different totals.
	SpendTodayMicros int64                 `json:"spend_today_micros"`
	BudgetMicros     int64                 `json:"budget_micros"`
	Entries          []ledgerEntryResponse `json:"entries"`
}

// GET /api/v1/boards/{id}/ledger — a board's recent priced calls plus what it
// has spent today. The board row is read for its budget so the figure the UI
// compares against is the one the gate uses.
func (a boardAPI) listBoardLedger(w http.ResponseWriter, r *http.Request) {
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
	entries, err := a.svc.BoardLedger(r.Context(), b.ID, orgCtx.workspace.ID, 100)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	spend, err := a.svc.BoardSpendToday(r.Context(), b.ID, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := boardLedgerResponse{
		SpendTodayMicros: spend,
		BudgetMicros:     b.BudgetDailyMicros,
		Entries:          make([]ledgerEntryResponse, 0, len(entries)),
	}
	for _, e := range entries {
		out.Entries = append(out.Entries, toLedgerResponse(e))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// POST /api/v1/tasks/{id}/claim — manual force claim (US-AD21). Claims the named
// task and opens run attempt N+1. Refused with 409 when the task is not `ready`
// or another claimer won the race; 400 when it has no agent to run it.
func (a boardAPI) claimTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	run, err := a.svc.ClaimAndStart(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusCreated, toRunResponse(run))
}
