package main

// HTTP surface for 6.2.19: audit log, in-app notifications, search, system info.
//
// Three different tenancy shapes live in this file, and they are not
// interchangeable:
//
//   - `/audit-log` and `/notifications` carry no id at all, so they resolve the
//     tenant from the session plus X-Org-ID (the header middleware), like the
//     approval inbox does.
//   - `/search/*` also carries no id, same middleware.
//   - `/system/info` is public and touches no database: it reports what is
//     running, which is the one thing an operator needs when a deploy is
//     suspected of being stale.

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
)

// ---- audit log (US-AD95) ----------------------------------------------------

type auditEntryResponse struct {
	ID           int64           `json:"id"`
	ActorUserID  string          `json:"actor_user_id,omitempty"`
	ActorAgentID string          `json:"actor_agent_id,omitempty"`
	Action       string          `json:"action"`
	TargetType   string          `json:"target_type"`
	TargetID     string          `json:"target_id"`
	BeforeJSON   json.RawMessage `json:"before_json,omitempty"`
	AfterJSON    json.RawMessage `json:"after_json,omitempty"`
	IP           string          `json:"ip,omitempty"`
	CreatedAt    string          `json:"created_at"`
}

// GET /api/v1/audit-log — US-AD95, Admin+ (11.3 "Lihat Audit Log").
//
// `id` is carried in the response so the caller can use it as the next `cursor`.
// Without it the cursor pagination this endpoint advertises would be unusable:
// there would be nothing to pass back.
func (a authAPI) listAuditLog(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()

	filter := auth.AuditFilter{}
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		cursor, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		filter.Cursor = cursor
	}
	if raw := strings.TrimSpace(q.Get("actor")); raw != "" {
		filter.ActorUserID = &raw
	}
	if raw := strings.TrimSpace(q.Get("action")); raw != "" {
		filter.Action = &raw
	}
	// US-AD95 AC2's date range. RFC3339 only: a looser parse would accept
	// "2026-09" and silently mean the first instant of that month.
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid from (want RFC3339)", http.StatusBadRequest)
			return
		}
		filter.From = t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid to (want RFC3339)", http.StatusBadRequest)
			return
		}
		filter.To = t
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || limit < 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		filter.Limit = int32(limit)
	}

	entries, err := a.store.AuditLog(r.Context(), orgCtx.workspace.ID, filter)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]auditEntryResponse, 0, len(entries))
	for _, e := range entries {
		row := auditEntryResponse{
			ActorUserID:  e.ActorUserID,
			ActorAgentID: e.ActorAgentID,
			Action:       e.Action,
			TargetType:   e.TargetType,
			TargetID:     e.TargetID,
			IP:           e.IP,
			CreatedAt:    e.CreatedAt.UTC().Format(time.RFC3339),
		}
		if len(e.Before) > 0 {
			row.BeforeJSON = json.RawMessage(e.Before)
		}
		if len(e.After) > 0 {
			row.AfterJSON = json.RawMessage(e.After)
		}
		out = append(out, row)
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"entries": out})
}

// ---- notifications (US-AD61) ------------------------------------------------

type notificationResponse struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	ReadAt     string `json:"read_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func toNotificationResponse(n auth.Notification) notificationResponse {
	out := notificationResponse{
		ID:         n.ID,
		Kind:       n.Kind,
		Title:      n.Title,
		Body:       n.Body,
		TargetType: n.TargetType,
		TargetID:   n.TargetID,
		CreatedAt:  n.CreatedAt.UTC().Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		out.ReadAt = n.ReadAt.UTC().Format(time.RFC3339)
	}
	return out
}

// GET /api/v1/notifications — US-AD61. Viewer, and only the caller's own rows:
// the user id comes from the session, never from a query parameter. That is
// AC4 — a caller cannot ask for somebody else's inbox by naming them.
func (a authAPI) listNotifications(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	limit := int32(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = int32(parsed)
	}
	items, unread, err := a.store.Notifications(r.Context(), orgCtx.userID, orgCtx.workspace.ID, limit)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]notificationResponse, 0, len(items))
	for _, n := range items {
		out = append(out, toNotificationResponse(n))
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"notifications": out,
		"unread_count":  unread,
	})
}

type markReadRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

// POST /api/v1/notifications/read — US-AD61 AC1.
//
// `{"ids":[...]}` or `{"all":true}`. Both absent is 400 rather than "mark
// everything": a client that forgot to send `all` would otherwise clear the
// badge it was trying to preserve.
func (a authAPI) markNotificationsRead(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req markReadRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	marked, err := a.store.MarkNotificationsRead(r.Context(), orgCtx.userID, orgCtx.workspace.ID, req.IDs, req.All)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"marked": marked})
}

// ---- search (6.2.19) --------------------------------------------------------

// GET /api/v1/search/tasks — Viewer.
func (a boardAPI) searchTasks(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	limit, ok := searchLimit(w, q.Get("limit"))
	if !ok {
		return
	}
	tasks, err := a.svc.SearchTasks(r.Context(), orgCtx.workspace.ID,
		q.Get("q"), strings.TrimSpace(q.Get("board_id")), limit)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toTaskResponse(t))
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"tasks": out})
}

// GET /api/v1/search/runs — Viewer.
func (a boardAPI) searchRuns(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	limit, ok := searchLimit(w, q.Get("limit"))
	if !ok {
		return
	}
	runs, err := a.svc.SearchRuns(r.Context(), orgCtx.workspace.ID, board.RunSearchFilter{
		TaskID:      q.Get("task_id"),
		FailureKind: q.Get("failure_kind"),
		Outcome:     q.Get("outcome"),
		Q:           q.Get("q"),
		Limit:       limit,
	})
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]runResponse, 0, len(runs))
	for _, run := range runs {
		out = append(out, toRunResponse(run))
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"runs": out})
}

// searchLimit parses `?limit=`. It writes the 400 itself and reports false so
// the two search handlers share one parse instead of two copies.
func searchLimit(w http.ResponseWriter, raw string) (int32, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || parsed < 0 {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return 0, false
	}
	return int32(parsed), true
}

// ---- system info ------------------------------------------------------------

type systemInfoResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
	StartedAt string `json:"started_at"`
}

// buildVersion and buildCommit are set with -ldflags at build time. They are
// variables rather than constants so a linker can overwrite them; an unset build
// reports "dev", which is honest — an empty version would read as a bug.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
)

// processStartedAt is when this process came up. Read once at init: recomputing
// it per request would answer a different question ("how long has this handler
// been up") and a restarted container would look like it had never restarted.
var processStartedAt = time.Now().UTC()

// GET /api/v1/system/info — Public.
//
// Deliberately public and deliberately thin. It answers the one question a
// deployment needs from the outside — "which build is actually serving?" — and
// nothing else. No database handle, no configuration, no feature flags: those
// would turn an unauthenticated endpoint into a map of the deployment.
func systemInfo(w http.ResponseWriter, r *http.Request) {
	commit := buildCommit
	if env := strings.TrimSpace(os.Getenv("AGENTDECK_COMMIT")); env != "" {
		// The container gets the SHA from its build args, which is more reliable
		// than remembering to pass ldflags through every local `go build`.
		commit = env
	}
	writeJSONResponse(w, http.StatusOK, systemInfoResponse{
		Version:   buildVersion,
		Commit:    commit,
		GoVersion: runtime.Version(),
		StartedAt: processStartedAt.Format(time.RFC3339),
	})
}
