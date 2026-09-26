package main

// HTTP surface for the approval gate — ARCHITECTURE 6.2.14.
//
// The role gates here are the RBAC matrix (11.3), not the PRD's AC text. The
// conflict is documented in internal/board/approval.go; the short version is
// that `POST /tasks/{id}/approvals` is Member-gated, so a member who could also
// approve would be able to open and clear their own gate.
//
// `GET /approvals` and `GET /approvals/{id}` sit on the org HEADER middleware,
// not the path-id one: neither path carries an org id, and the tenant comes from
// the session plus X-Org-ID (11.1 step 5).

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"agentdeck/internal/board"
)

type approvalResponse struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	TaskTitle   string `json:"task_title,omitempty"`
	RunID       string `json:"run_id"`
	RequestedBy string `json:"requested_by"`
	DecidedBy   string `json:"decided_by,omitempty"`
	Decision    string `json:"decision"`
	GateMode    string `json:"gate_mode"`
	Reason      string `json:"reason,omitempty"`
	// PreviewJSON is echoed as stored, not re-encoded. A server that parses and
	// re-serialises a diff preview can show the approver something the worker did
	// not propose — key order and number formatting are not worth that risk.
	PreviewJSON json.RawMessage `json:"preview_json,omitempty"`
	ExpiresAt   string          `json:"expires_at"`
	DecidedAt   string          `json:"decided_at,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

func toApprovalResponse(a board.Approval) approvalResponse {
	out := approvalResponse{
		ID:          a.ID,
		TaskID:      a.TaskID,
		TaskTitle:   a.TaskTitle,
		RunID:       a.RunID,
		RequestedBy: a.RequestedBy,
		DecidedBy:   a.DecidedBy,
		Decision:    string(a.Decision),
		GateMode:    a.GateMode,
		Reason:      a.Reason,
		ExpiresAt:   a.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if len(a.PreviewJSON) > 0 {
		out.PreviewJSON = json.RawMessage(a.PreviewJSON)
	}
	if a.DecidedAt != nil {
		out.DecidedAt = a.DecidedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// GET /api/v1/approvals — the inbox: pending gates that have not expired.
func (a boardAPI) listApprovals(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	approvals, err := a.svc.ApprovalInbox(r.Context(), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]approvalResponse, 0, len(approvals))
	for _, approval := range approvals {
		out = append(out, toApprovalResponse(approval))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// GET /api/v1/approvals/{id} — one gate, with its preview.
func (a boardAPI) getApproval(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	approval, err := a.svc.GetApproval(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toApprovalResponse(approval))
}

// POST /api/v1/approvals/{id}/approve — US-AD34, owner/admin per 11.3.
func (a boardAPI) approveApproval(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	approval, err := a.svc.Approve(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, orgCtx.email)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toApprovalResponse(approval))
}

type rejectApprovalRequest struct {
	Reason string `json:"reason"`
}

// POST /api/v1/approvals/{id}/reject — US-AD35, owner/admin per 11.3.
//
// The reason is required (US-AD35 AC2) and enforced in the service, so a caller
// reaching it by any other path gets the same answer.
func (a boardAPI) rejectApproval(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req rejectApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	approval, err := a.svc.Reject(r.Context(), r.PathValue("id"), orgCtx.workspace.ID, orgCtx.email, req.Reason)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toApprovalResponse(approval))
}

type requestApprovalRequest struct {
	// PreviewJSON is `json.RawMessage`, not a decoded struct: the preview's shape
	// belongs to the worker that proposed the action, and the server's job is to
	// store it and hand it back unchanged.
	PreviewJSON json.RawMessage `json:"preview_json"`
	Reason      string          `json:"reason"`
}

// POST /api/v1/tasks/{id}/approvals — US-AD33, the worker's hold request.
func (a boardAPI) requestApproval(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req requestApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	approval, err := a.svc.RequestApproval(r.Context(), r.PathValue("id"), orgCtx.workspace.ID,
		orgCtx.email, req.PreviewJSON, req.Reason)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusCreated, toApprovalResponse(approval))
}
