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
	// ARCHITECTURE 6.2.9's last three task routes. cancel and retry are Member;
	// archive is Admin because US-AD59 AC4 sets that floor, and it lives here
	// rather than in the board routes above so the task-lifecycle writes are
	// declared in one place.
	boardRoute("POST /api/v1/tasks/{id}/cancel", http.HandlerFunc(boardAPI.cancelTask), auth.Member)
	boardRoute("POST /api/v1/tasks/{id}/retry", http.HandlerFunc(boardAPI.retryTask), auth.Member)
	boardRoute("POST /api/v1/tasks/{id}/archive", http.HandlerFunc(boardAPI.archiveTask), auth.Admin)
	boardRoute("GET /api/v1/tasks/{id}/runs", http.HandlerFunc(boardAPI.listTaskRuns), auth.Viewer)
	boardRoute("GET /api/v1/runs/{id}", http.HandlerFunc(boardAPI.getRun), auth.Viewer)
	boardRoute("POST /api/v1/runs/{id}/heartbeat", http.HandlerFunc(boardAPI.heartbeatRun), auth.Member)
	boardRoute("POST /api/v1/runs/{id}/end", http.HandlerFunc(boardAPI.endRun), auth.Member)
	// 6.2.11's two remaining routes. Cancelling a run is Member (it stops work
	// the workspace is paying for); reading its summary is Viewer.
	boardRoute("POST /api/v1/runs/{id}/cancel", http.HandlerFunc(boardAPI.cancelRun), auth.Member)
	boardRoute("GET /api/v1/runs/{id}/summary", http.HandlerFunc(boardAPI.getRunSummary), auth.Viewer)

	// Trace.
	boardRoute("GET /api/v1/runs/{id}/steps", http.HandlerFunc(boardAPI.listRunSteps), auth.Viewer)
	boardRoute("POST /api/v1/runs/{id}/steps", http.HandlerFunc(boardAPI.createRunStep), auth.Member)
	boardRoute("PATCH /api/v1/runs/{id}/steps/{seq}", http.HandlerFunc(boardAPI.finishRunStep), auth.Member)

	// Ledger (US-AD32 cost reporting).
	boardRoute("GET /api/v1/runs/{id}/ledger", http.HandlerFunc(boardAPI.listRunLedger), auth.Viewer)
	boardRoute("GET /api/v1/boards/{id}/ledger", http.HandlerFunc(boardAPI.listBoardLedger), auth.Viewer)

	// Budget (6.2.15). Reading is Viewer — the cost rail is on every board screen
	// and a viewer seeing the number is US-AD32 AC4. Raising the cap is Admin:
	// it is the control that decides how much the workspace may spend.
	boardRoute("GET /api/v1/boards/{id}/budget", http.HandlerFunc(boardAPI.getBoardBudget), auth.Viewer)
	boardRoute("PATCH /api/v1/boards/{id}/budget", http.HandlerFunc(boardAPI.updateBoardBudget), auth.Admin)

	// Cost summary. {id} di sini adalah id ORG, jadi middleware-nya yang membaca
	// path parameter — bukan yang header. Route board di atas memakai
	// orgHeaderContextMiddleware karena {id}-nya board.
	mux.Handle("GET /api/v1/orgs/{id}/cost-summary",
		api.orgContextMiddleware(api.requireRole(http.HandlerFunc(boardAPI.getCostSummary), auth.Admin)))
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
	// CancelRequestedAt is "" when nobody has asked this run to stop. Exposed
	// because it is the only way a caller can tell "cancellation requested but
	// the executor has not stopped yet" from "nothing happened" — the run is
	// still `running` in both cases.
	CancelRequestedAt string `json:"cancel_requested_at"`
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
	cancelled := ""
	if run.CancelRequestedAt != nil {
		cancelled = run.CancelRequestedAt.Format(time.RFC3339Nano)
	}
	return runResponse{
		ID:                run.ID,
		OrgID:             run.OrgID,
		TaskID:            run.TaskID,
		AgentID:           run.AgentID,
		Attempt:           run.Attempt,
		Status:            string(run.Status),
		Outcome:           run.Outcome,
		FailureKind:       run.FailureKind,
		LastHeartbeatAt:   beat,
		MaxRuntimeSecs:    run.MaxRuntimeSecs,
		CostMicros:        run.CostMicros,
		TokensIn:          run.TokensIn,
		TokensOut:         run.TokensOut,
		Summary:           run.Summary,
		Error:             run.Error,
		StartedAt:         run.StartedAt.Format(time.RFC3339Nano),
		EndedAt:           ended,
		CancelRequestedAt: cancelled,
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

// POST /api/v1/runs/{id}/cancel — stop a live run (ARCHITECTURE 6.2.11).
//
// The response is the run, not a bare 204, so the caller can see the
// cancellation actually landed (`cancel_requested_at` set) rather than having to
// poll and guess. A run that had already ended comes back unchanged and still
// 200: the requested state holds.
func (a boardAPI) cancelRun(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	run, err := a.svc.CancelRun(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toRunResponse(run))
}

type runSummaryResponse struct {
	RunID       string `json:"run_id"`
	TaskID      string `json:"task_id"`
	Attempt     int    `json:"attempt"`
	Status      string `json:"status"`
	Outcome     string `json:"outcome"`
	FailureKind string `json:"failure_kind"`
	// DurationSeconds is computed from started_at/ended_at, which GET /runs/{id}
	// already exposes. US-AD41 AC1 asks the run page to show "durasi", and the
	// subtraction is a fact about the run rather than about this handler, so it
	// is done once here instead of in every client. A live run reports the
	// elapsed time so far.
	DurationSeconds int64  `json:"duration_seconds"`
	CostMicros      int64  `json:"cost_micros"`
	TokensIn        int64  `json:"tokens_in"`
	TokensOut       int64  `json:"tokens_out"`
	Summary         string `json:"summary"`
	Error           string `json:"error"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
}

// GET /api/v1/runs/{id}/summary — US-AD41 AC1.
func (a boardAPI) getRunSummary(w http.ResponseWriter, r *http.Request) {
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

	// An unfinished run is measured against now(), not against zero: a page
	// that shows "0s" for a run that has been going for ten minutes is worse
	// than one that shows nothing.
	end := time.Now()
	if run.EndedAt != nil {
		end = *run.EndedAt
	}
	duration := end.Sub(run.StartedAt)
	if duration < 0 {
		// Clock skew between the API and the database, or a started_at in the
		// future. Report nothing rather than a negative duration.
		duration = 0
	}

	writeJSONResponse(w, http.StatusOK, runSummaryResponse{
		RunID:           run.ID,
		TaskID:          run.TaskID,
		Attempt:         run.Attempt,
		Status:          string(run.Status),
		Outcome:         run.Outcome,
		FailureKind:     run.FailureKind,
		DurationSeconds: int64(duration.Seconds()),
		CostMicros:      run.CostMicros,
		TokensIn:        run.TokensIn,
		TokensOut:       run.TokensOut,
		Summary:         run.Summary,
		Error:           run.Error,
		StartedAt:       run.StartedAt.Format(time.RFC3339Nano),
		EndedAt:         endedAtString(run),
	})
}

func endedAtString(run board.Run) string {
	if run.EndedAt == nil {
		return ""
	}
	return run.EndedAt.Format(time.RFC3339Nano)
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

type costSummaryResponse struct {
	TotalMicros int64                 `json:"total_micros"`
	ByModel     []costByModelResponse `json:"by_model"`
	ByBoard     []costByBoardResponse `json:"by_board"`
	// WindowDays is stated rather than implied: "the last 30 days" is part of
	// what the number means, and a client that hardcodes 30 would silently
	// mislabel the figure if the window ever changes.
	WindowDays int `json:"window_days"`
}

type costByModelResponse struct {
	Model      string `json:"model"`
	Provider   string `json:"provider"`
	CostMicros int64  `json:"cost_micros"`
	TokensIn   int64  `json:"tokens_in"`
	TokensOut  int64  `json:"tokens_out"`
	Runs       int    `json:"runs"`
}

type costByBoardResponse struct {
	BoardID    string `json:"board_id"`
	Name       string `json:"name"`
	CostMicros int64  `json:"cost_micros"`
	TokensIn   int64  `json:"tokens_in"`
	TokensOut  int64  `json:"tokens_out"`
	Runs       int    `json:"runs"`
}

// GET /api/v1/orgs/{id}/cost-summary — 30-day totals by model and board.
//
// US-AD32 reporting. Admin-gated per §6.2.15's Role Min column: this is the
// workspace's whole spend history, not one board's running total.
//
// The empty case is an empty report with a zero total, never a 404 or an error:
// an org that has not spent anything yet is a normal org, and the screen shows
// its empty state from a well-formed answer (US-AD32 AC3's "nol, bukan error").
func (a boardAPI) getCostSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := a.svc.OrgCostSummary(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := costSummaryResponse{
		TotalMicros: summary.TotalMicros,
		WindowDays:  30,
		ByModel:     make([]costByModelResponse, 0, len(summary.ByModel)),
		ByBoard:     make([]costByBoardResponse, 0, len(summary.ByBoard)),
	}
	for _, m := range summary.ByModel {
		out.ByModel = append(out.ByModel, costByModelResponse{
			Model: m.Model, Provider: m.Provider, CostMicros: m.CostMicros,
			TokensIn: m.TokensIn, TokensOut: m.TokensOut, Runs: m.Runs,
		})
	}
	for _, b := range summary.ByBoard {
		out.ByBoard = append(out.ByBoard, costByBoardResponse{
			BoardID: b.BoardID, Name: b.BoardName, CostMicros: b.CostMicros,
			TokensIn: b.TokensIn, TokensOut: b.TokensOut, Runs: b.Runs,
		})
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// boardBudgetResponse is the shape the finops screen expects (frontend
// src/lib/domain.ts `BoardBudget`): every money field is micro-USD, and
// `threshold_crossed` is reported by the server rather than recomputed by each
// client from a magic 0.8.
type boardBudgetResponse struct {
	BoardID           string `json:"board_id"`
	Day               string `json:"day"`
	BudgetDailyMicros int64  `json:"budget_daily_micros"`
	SpentMicros       int64  `json:"spent_micros"`
	RunCount          int    `json:"run_count"`
	TokensIn          int64  `json:"tokens_in"`
	TokensOut         int64  `json:"tokens_out"`
	ThresholdCrossed  bool   `json:"threshold_crossed"`
}

func toBoardBudgetResponse(b board.BoardBudget) boardBudgetResponse {
	return boardBudgetResponse{
		BoardID:           b.BoardID,
		Day:               b.Day,
		BudgetDailyMicros: b.CapMicros,
		SpentMicros:       b.SpentTodayMicros,
		RunCount:          b.RunCount,
		TokensIn:          b.TokensIn,
		TokensOut:         b.TokensOut,
		// N18: the 80% alert. `warning` is exactly "at or past 80%, not yet at
		// the cap", so the flag is the status rather than a second threshold.
		ThresholdCrossed: b.Status == "warning" || b.Status == "exceeded",
	}
}

// GET /api/v1/boards/{id}/budget — realtime usage against the N16 daily cap.
//
// Reads the aggregate (daily_board_costs), which is the same row the dispatcher's
// cost gate reads. The screen and the guardrail therefore cannot disagree about
// whether the board is out of budget.
func (a boardAPI) getBoardBudget(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	budget, err := a.svc.BoardBudgetToday(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toBoardBudgetResponse(budget))
}

type boardBudgetRequest struct {
	BudgetDailyMicros *int64 `json:"budget_daily_micros"`
}

// PATCH /api/v1/boards/{id}/budget — set the daily cap (Admin, §6.2.15).
//
// A pointer, so "not sent" is a 400 rather than a silent zero. Zero is a
// legitimate value (it means "stop spending"), and a request that omits the
// field must not be indistinguishable from one that asks for it.
//
// The response is the budget after the write, read back from the aggregate, so
// the caller sees the cap and today's spend together rather than having to
// re-query to learn what the new cap applies to.
func (a boardAPI) updateBoardBudget(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req boardBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.BudgetDailyMicros == nil {
		http.Error(w, "budget_daily_micros is required", http.StatusBadRequest)
		return
	}
	if err := a.svc.UpdateBoardBudget(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, *req.BudgetDailyMicros); err != nil {
		writeBoardError(w, err)
		return
	}
	budget, err := a.svc.BoardBudgetToday(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toBoardBudgetResponse(budget))
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

// POST /api/v1/tasks/{id}/cancel — stop a task and abort its run if one is live.
//
// The cancel is recorded durably and the dispatcher applies it: the API never
// writes run state itself (§5.1 makes the dispatcher the only writer). That is
// why this returns 200 with the task rather than a run — the run's outcome is
// the dispatcher's to decide, and reporting it here would be reporting something
// this handler did not observe.
func (a boardAPI) cancelTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	task, err := a.svc.CancelTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toTaskResponse(task))
}

// POST /api/v1/tasks/{id}/retry — the operator's override of the retry policy.
// Resets the failure counter and returns the task to `ready`; 409 while a run
// still holds it.
func (a boardAPI) retryTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	task, err := a.svc.RetryTask(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toTaskResponse(task))
}

// POST /api/v1/tasks/{id}/archive — US-AD59.
//
// Delegates to the same MoveTask the board's drag-and-drop uses rather than
// writing the status here: the terminal-only rule (AC2) and the idempotent
// re-archive (AC3) already live there, and a second path to `archived` would be
// a second place for those rules to drift. The role floor (AC4) is this route's
// gate, which is why the endpoint exists at all — the board route accepts the
// archive target too, and one of the two had to be the enforced one.
func (a boardAPI) archiveTask(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	task, err := a.svc.GetTask(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	task, err = a.svc.MoveTask(r.Context(), id, orgCtx.workspace.ID, task.Status, board.StatusArchived)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toTaskResponse(task))
}
