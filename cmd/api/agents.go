package main

import (
	"encoding/json"
	"net/http"
	"time"

	"agentdeck/internal/board"
)

// Agent registry HTTP surface (US-AD20).
//
// Routes and their minimum roles come from the ARCHITECTURE route table:
//
//	GET    /api/v1/projects/{project_id}/agents   Viewer
//	POST   /api/v1/projects/{project_id}/agents   Member
//	GET    /api/v1/agents/{id}                    Viewer
//	DELETE /api/v1/agents/{id}                    Admin
//
// US-AD20 AC2 says "viewer gets 403 at the agent registry endpoints", which the
// PRD's own note resolves as the *mutating* endpoints: GET is Viewer by the
// table above, and the binding permission for reads is the tenant boundary, not
// a role floor. PATCH /agents/{id} is deliberately absent — it is US-AD67 AC4
// (owner/admin for provider and model), a different criterion, and registering
// a route with the wrong gate here would silently satisfy neither story.

type agentRequest struct {
	Name              string   `json:"name"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	ReasoningEffort   string   `json:"reasoning_effort"`
	Skills            []string `json:"skills"`
	Tools             []string `json:"tools"`
	MaxRuntimeSeconds *int     `json:"max_runtime_seconds"`
	RetryPolicy       string   `json:"retry_policy"`
	MaxAttempts       *int     `json:"max_attempts"`
}

type agentResponse struct {
	ID                string   `json:"id"`
	OrgID             string   `json:"org_id"`
	ProjectID         string   `json:"project_id"`
	Name              string   `json:"name"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	ReasoningEffort   string   `json:"reasoning_effort"`
	Skills            []string `json:"skills"`
	Tools             []string `json:"tools"`
	MaxRuntimeSeconds int      `json:"max_runtime_seconds"`
	RetryPolicy       string   `json:"retry_policy"`
	MaxAttempts       int      `json:"max_attempts"`
	// HasProviderKey is always false until M2 stores an encrypted credential
	// (§16). It is reported rather than omitted so the client's `Agent` shape is
	// stable across that milestone, and it is never a key — only its presence.
	HasProviderKey bool   `json:"has_provider_key"`
	CreatedAt      string `json:"created_at"`
}

// The DDL defaults, mirrored so a request that omits a field lands on the same
// value the database would have chosen (N9 = 4 hours).
const (
	defaultAgentMaxRuntimeSeconds = 14400
	defaultAgentRetryPolicy       = "transient_only"
	defaultAgentMaxAttempts       = 3
)

func toAgentResponse(a board.Agent) agentResponse {
	// The stored columns are jsonb arrays by CHECK constraint, so they always
	// decode; an absent value becomes an empty list rather than null, which
	// keeps the client's `string[]` honest instead of making it handle null.
	skills := []string{}
	if len(a.SkillsJSON) > 0 {
		_ = json.Unmarshal(a.SkillsJSON, &skills)
	}
	tools := []string{}
	if len(a.ToolsJSON) > 0 {
		_ = json.Unmarshal(a.ToolsJSON, &tools)
	}
	return agentResponse{
		ID:                a.ID,
		OrgID:             a.OrgID,
		ProjectID:         a.ProjectID,
		Name:              a.Name,
		Provider:          a.Provider,
		Model:             a.Model,
		ReasoningEffort:   a.ReasoningEffort,
		Skills:            skills,
		Tools:             tools,
		MaxRuntimeSeconds: a.MaxRuntimeSeconds,
		RetryPolicy:       a.RetryPolicy,
		MaxAttempts:       a.MaxAttempts,
		HasProviderKey:    false,
		CreatedAt:         a.CreatedAt.Format(time.RFC3339Nano),
	}
}

// POST /api/v1/projects/{project_id}/agents
func (a boardAPI) createAgent(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	agent := board.Agent{
		OrgID:             orgCtx.workspace.ID,
		ProjectID:         r.PathValue("project_id"),
		Name:              req.Name,
		Provider:          req.Provider,
		Model:             req.Model,
		ReasoningEffort:   req.ReasoningEffort,
		MaxRuntimeSeconds: defaultAgentMaxRuntimeSeconds,
		RetryPolicy:       defaultAgentRetryPolicy,
		MaxAttempts:       defaultAgentMaxAttempts,
	}
	if req.Skills != nil {
		if raw, err := json.Marshal(req.Skills); err == nil {
			agent.SkillsJSON = raw
		}
	}
	if req.Tools != nil {
		if raw, err := json.Marshal(req.Tools); err == nil {
			agent.ToolsJSON = raw
		}
	}
	if req.MaxRuntimeSeconds != nil {
		agent.MaxRuntimeSeconds = *req.MaxRuntimeSeconds
	}
	if req.RetryPolicy != "" {
		agent.RetryPolicy = req.RetryPolicy
	}
	if req.MaxAttempts != nil {
		agent.MaxAttempts = *req.MaxAttempts
	}
	created, err := a.svc.CreateAgent(r.Context(), agent)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toAgentResponse(created))
}

// GET /api/v1/projects/{project_id}/agents
func (a boardAPI) listAgents(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	agents, err := a.svc.ListAgents(r.Context(), orgCtx.workspace.ID, r.PathValue("project_id"))
	if err != nil {
		writeBoardError(w, err)
		return
	}
	out := make([]agentResponse, 0, len(agents))
	for _, agent := range agents {
		out = append(out, toAgentResponse(agent))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GET /api/v1/agents/{id}
func (a boardAPI) getAgent(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	agent, err := a.svc.GetAgent(r.Context(), r.PathValue("id"), orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAgentResponse(agent))
}

// DELETE /api/v1/agents/{id}
func (a boardAPI) deleteAgent(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.svc.DeleteAgent(r.Context(), r.PathValue("id"), orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
