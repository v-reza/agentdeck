package main

// HTTP surface for task comments — ARCHITECTURE 6.2.17, US-AD42.
//
// Two shapes of path, so two middlewares. `GET`/`POST /tasks/{id}/comments`
// carry a task id and use the task-owner middleware; `PATCH`/`DELETE
// /comments/{id}` carry only the comment id, so they sit on the org HEADER
// middleware like the approval routes do — the tenant comes from the session
// plus X-Org-ID, and the comment id is scoped to that org in SQL.
//
// Reading is Viewer and writing is Member (US-AD42 AC2). That floor is on the
// route table in runs.go, not here.

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"agentdeck/internal/board"
)

type commentResponse struct {
	ID            int64  `json:"id"`
	TaskID        string `json:"task_id"`
	Body          string `json:"body"`
	AuthorUserID  string `json:"author_user_id,omitempty"`
	AuthorAgentID string `json:"author_agent_id,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func toCommentResponse(c board.Comment) commentResponse {
	return commentResponse{
		ID:            c.ID,
		TaskID:        c.TaskID,
		Body:          c.Body,
		AuthorUserID:  c.AuthorUserID,
		AuthorAgentID: c.AuthorAgentID,
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type commentRequest struct {
	Body string `json:"body"`
}

// commentID parses the path id. A non-numeric id is 400, not 500: the route
// matched, so the caller reached a comment endpoint and asked for something that
// cannot be a comment. Answering 500 would make a typo look like a server fault.
func commentID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// GET /api/v1/tasks/{id}/comments
func (a boardAPI) listComments(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	orgID := orgCtx.workspace.ID
	comments, err := a.svc.ListComments(r.Context(), r.PathValue("id"), orgID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]commentResponse, 0, len(comments))
	for _, c := range comments {
		out = append(out, toCommentResponse(c))
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"comments": out})
}

// POST /api/v1/tasks/{id}/comments
func (a boardAPI) createComment(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	orgID := orgCtx.workspace.ID
	userID := orgCtx.userID
	var req commentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	// Only the user branch is reachable from an HTTP session. The agent branch
	// exists in the domain because comments_author_chk allows it and the
	// worker-facing route (§6.2.17's Internal/Key surface) will need it once
	// `api_keys` exists — see docs/OPEN-ISSUES.md.
	comment, err := a.svc.CreateComment(r.Context(), r.PathValue("id"), orgID, req.Body, board.CommentAuthor{UserID: userID})
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusCreated, toCommentResponse(comment))
}

// PATCH /api/v1/comments/{id}
func (a boardAPI) editComment(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	orgID := orgCtx.workspace.ID
	userID := orgCtx.userID
	id, ok := commentID(r)
	if !ok {
		http.Error(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	var req commentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	comment, err := a.svc.EditComment(r.Context(), id, orgID, userID, req.Body)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, toCommentResponse(comment))
}

// DELETE /api/v1/comments/{id}
func (a boardAPI) deleteComment(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	orgID := orgCtx.workspace.ID
	userID := orgCtx.userID
	id, ok := commentID(r)
	if !ok {
		http.Error(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	if err := a.svc.DeleteComment(r.Context(), id, orgID, userID); err != nil {
		writeBoardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
