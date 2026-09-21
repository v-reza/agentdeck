package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/skill"
)

// Agent skill library HTTP surface (US-AD107, ARCHITECTURE 6.2.7).
//
//	GET    /api/v1/agent-skills              Viewer
//	POST   /api/v1/agent-skills              Admin
//	PATCH  /api/v1/agent-skills/{id}         Admin
//	DELETE /api/v1/agent-skills/{id}         Admin
//	GET    /api/v1/agent-skills/{id}/agents  Viewer   (added: the "dipakai oleh"
//	                                                   list ARCHITECTURE 6.2.9 + US-AD107 needs)
//
// Every mutating route is Admin. There is no route — and no handler function —
// that accepts an agent identity: the only authentication on this surface is a
// user session (orgHeaderContextMiddleware -> currentUser), so an agent cannot
// write a skill even if it holds a board credential. That is the mechanism
// behind US-AD107 AC1; the role tests below pin it.
//
// Skill is per *org*, not per user (user decision, DECISIONS 6A.G): the tenant
// comes from X-Org-ID, and a skill owned by another org is a 404, never a 403.

type skillRequest struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	BodyMD string `json:"body_md"`
}

// skillPatchRequest omits Slug deliberately: the slug is the key agents store
// in skills_json, so renaming it would silently detach every agent using the
// skill. Changing it is not an edit, it is a different skill.
type skillPatchRequest struct {
	Name   string `json:"name"`
	BodyMD string `json:"body_md"`
}

type skillResponse struct {
	ID       string `json:"id"`
	OrgID    string `json:"org_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	BodyMD   string `json:"body_md"`
	Version  int    `json:"version"`
	IsSystem bool   `json:"is_system"`
	// UsedBy is the "dipakai N agent" counter of ARCHITECTURE 6.2.9 + US-AD107, present on
	// every row of the list so the UI never has to ask per skill.
	UsedBy    int    `json:"used_by"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// agentRefResponse is one row of "dipakai oleh": the id routes to the agent
// detail page, the name is what the user reads.
type agentRefResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func toSkillResponse(s skill.Skill, usedBy int) skillResponse {
	createdBy := ""
	if s.CreatedBy != nil {
		createdBy = *s.CreatedBy
	}
	return skillResponse{
		ID:        s.ID,
		OrgID:     s.OrgID,
		Slug:      s.Slug,
		Name:      s.Name,
		BodyMD:    s.BodyMD,
		Version:   s.Version,
		IsSystem:  s.IsSystem,
		UsedBy:    usedBy,
		CreatedBy: createdBy,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// writeSkillError maps a domain error to its stable HTTP code so the same
// failure always yields the same response from every path. A foreign skill id
// is ErrSkillNotFound -> 404, consistent with the rest of the repo (US-AD07):
// the tenant boundary must not be observable as a 403.
func writeSkillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skill.ErrSkillNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, skill.ErrSlugTaken), errors.Is(err, skill.ErrSystemSkill):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, skill.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// skillAPI wires the skill domain to HTTP. It rides the same authAPI as the
// auth/org/board routes so tenant resolution and the role gate stay in one
// place.
type skillAPI struct {
	svc *skill.Service
}

func (a skillAPI) context(r *http.Request) (orgContext, error) {
	return currentOrgContext(r)
}

// GET /api/v1/agent-skills
func (a skillAPI) listSkills(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	rows, err := a.svc.List(r.Context(), orgCtx.workspace.ID)
	if err != nil {
		writeSkillError(w, err)
		return
	}
	// An empty library is an empty array, never null: the UI's empty state is
	// driven by length, and a null would make it handle a second shape.
	out := make([]skillResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSkillResponse(row.Skill, row.UsedBy))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// POST /api/v1/agent-skills
func (a skillAPI) createSkill(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req skillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := a.svc.Create(r.Context(), orgCtx.workspace.ID, orgCtx.userID, req.Slug, req.Name, req.BodyMD)
	if err != nil {
		writeSkillError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toSkillResponse(created, 0))
}

// PATCH /api/v1/agent-skills/{id}
func (a skillAPI) updateSkill(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req skillPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	updated, err := a.svc.Update(r.Context(), orgCtx.workspace.ID, r.PathValue("id"), req.Name, req.BodyMD)
	if err != nil {
		writeSkillError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toSkillResponse(updated, 0))
}

// DELETE /api/v1/agent-skills/{id}
func (a skillAPI) deleteSkill(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.svc.Delete(r.Context(), orgCtx.workspace.ID, r.PathValue("id")); err != nil {
		writeSkillError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/agent-skills/{id}/agents — "dipakai oleh" (ARCHITECTURE 6.2.9 + US-AD107).
func (a skillAPI) listSkillAgents(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	found, err := a.svc.Get(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeSkillError(w, err)
		return
	}
	refs, err := a.svc.AgentsUsing(r.Context(), orgCtx.workspace.ID, found.Slug)
	if err != nil {
		writeSkillError(w, err)
		return
	}
	out := make([]agentRefResponse, 0, len(refs))
	for _, ref := range refs {
		out = append(out, agentRefResponse{ID: ref.ID, Name: ref.Name})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// seedWorkspaceSkills adapts the skill service to the auth store's
// workspace-created hook. A seed failure is logged and swallowed: the user, the
// org, and the owner membership are already durable when the hook runs, so
// failing registration over eight convenience rows would be the worse outcome —
// and the seed is idempotent, so the next call converges.
func seedWorkspaceSkills(ctx context.Context, api authAPI, svc *skill.Service, orgID string) {
	if err := svc.SeedSystemSkills(ctx, orgID); err != nil && api.logger != nil {
		api.logger.Warn("system skills not seeded", "org_id", orgID, "error", err)
	}
}

// registerAgentSkillRoutes mounts every skill-library route and, in the same
// call, wires the workspace-created seed hook.
//
// The seeding hook is installed here on purpose: this function is the one line
// the composition root has to add for the skill library to exist at all, so
// making it also the line that enables seeding means a workspace cannot get the
// endpoints without getting its eight defaults (US-AD107 AC5). Splitting them
// would make "skills exist but the library is empty" a reachable state.
//
// Each route chains authentication + tenant resolution + the role gate, so no
// skill handler can be registered without all three. The gate is what makes an
// agent unable to write: authentication here is a user session only.
func registerAgentSkillRoutes(mux *http.ServeMux, api authAPI, svc *skill.Service) {
	if api.store != nil {
		api.store.OnWorkspaceCreated(func(ctx context.Context, orgID string) {
			seedWorkspaceSkills(ctx, api, svc, orgID)
		})
	}

	skillSvc := skillAPI{svc: svc}
	skillRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}
	// US-AD107 AC4: member and viewer only read. Every write is Admin, and
	// nothing on this surface is Member-gated — there is no such thing as a
	// member who may edit the org's skills.
	skillRoute("GET /api/v1/agent-skills", http.HandlerFunc(skillSvc.listSkills), auth.Viewer)
	skillRoute("POST /api/v1/agent-skills", http.HandlerFunc(skillSvc.createSkill), auth.Admin)
	skillRoute("PATCH /api/v1/agent-skills/{id}", http.HandlerFunc(skillSvc.updateSkill), auth.Admin)
	skillRoute("DELETE /api/v1/agent-skills/{id}", http.HandlerFunc(skillSvc.deleteSkill), auth.Admin)
	// "Dipakai oleh" is a read of agent names the caller can already list, so
	// it is Viewer: raising it would hide the one signal that stops a user
	// from editing a skill agents are running.
	skillRoute("GET /api/v1/agent-skills/{id}/agents", http.HandlerFunc(skillSvc.listSkillAgents), auth.Viewer)
}
