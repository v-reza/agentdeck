package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"agentdeck/internal/auth"
)

// orgRequest is the body of POST /api/v1/orgs and PATCH /api/v1/orgs/{id}.
// Only name is ever honored from a client; slug and role are never accepted
// from a request body because both are server-derived (ARCHITECTURE 11.3).
type orgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type memberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type memberResponse struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// orgContext is the resolved tenant for one request. It is built once by
// orgContextMiddleware from the session and the X-Org-ID header, never from a
// path parameter or a request body, so a caller cannot shop for a tenant by
// editing the URL (ARCHITECTURE 11.1 step 5, US-AD07).
type orgContext struct {
	email     string
	workspace auth.Workspace
	role      auth.Role
	resolved  bool
}

// orgContextMiddleware authenticates the request and resolves the active org.
// A missing or invalid session is 401; a valid session with no membership in
// the requested org is 403; an unknown org id is 404 (US-AD07 AC1/AC3).
func (a authAPI) orgContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(a.store, r)
		if !ok {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		requestedID := strings.TrimSpace(r.Header.Get("X-Org-ID"))
		workspace, role, err := a.store.ResolveWorkspace(user.Email, requestedID)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		ctx := withOrgContext(r.Context(), orgContext{
			email:     user.Email,
			workspace: workspace,
			role:      role,
			resolved:  true,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a authAPI) requireRole(next http.Handler, minimum auth.Role) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgCtx, err := currentOrgContext(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !a.store.Authorize(orgCtx.workspace.ID, orgCtx.email, minimum) {
			http.Error(w, "insufficient role", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /api/v1/orgs — list the orgs the caller belongs to (ARCHITECTURE 6.2.4).
// It is scoped to the caller's own memberships, so it cannot leak another
// tenant's org even without an X-Org-ID.
func (a authAPI) listOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(a.store, r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	memberships := a.store.Workspaces(user.Email)
	orgs := make([]map[string]string, 0, len(memberships))
	for _, membership := range memberships {
		orgs = append(orgs, map[string]string{
			"id":   membership.WorkspaceID,
			"name": membership.Name,
			"slug": membership.Slug,
			"role": string(membership.Role),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orgs)
}

// POST /api/v1/orgs — create a second org; the caller becomes its owner
// (US-AD03 AC1). Slug and role are never taken from the request body.
func (a authAPI) createOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(a.store, r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var input orgRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	workspace, err := a.store.CreateWorkspace(user.Email, input.Name, input.Slug)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":   workspace.ID,
		"name": workspace.Name,
		"slug": workspace.Slug,
	})
}

// GET /api/v1/orgs/{id} — org detail. The id is resolved against the caller's
// memberships, never trusted: a foreign id yields 404, not the org itself.
func (a authAPI) getOrg(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":   orgCtx.workspace.ID,
		"name": orgCtx.workspace.Name,
		"slug": orgCtx.workspace.Slug,
		"role": string(orgCtx.role),
	})
}

// PATCH /api/v1/orgs/{id} — rename an org. Owner only (US-AD03 AC2).
func (a authAPI) updateOrg(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var input orgRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if err := a.store.UpdateWorkspace(orgCtx.workspace.ID, orgCtx.email, input.Name); err != nil {
		writeAuthError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":   orgCtx.workspace.ID,
		"name": input.Name,
		"slug": orgCtx.workspace.Slug,
	})
}

// GET /api/v1/orgs/{id}/members — list the roster. Viewer and above
// (ARCHITECTURE 6.2.4, 11.3); a non-member gets 403 from the middleware.
func (a authAPI) listMembers(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	members, err := a.store.Members(orgCtx.workspace.ID, orgCtx.email)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	rows := make([]memberResponse, 0, len(members))
	for _, member := range members {
		rows = append(rows, memberResponse{
			UserID: member.UserID,
			Email:  member.Email,
			Name:   member.Name,
			Role:   string(member.Role),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

// POST /api/v1/orgs/{id}/members — invite by email (US-AD04 AC1). Admin and
// owner; member and viewer get 403 (AC2). Owner is never assignable, so an
// invite cannot escalate anyone past admin.
func (a authAPI) addMember(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var input memberRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	role, err := auth.ParseRole(input.Role)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	if err := a.store.AddMember(orgCtx.workspace.ID, orgCtx.email, input.Email, role); err != nil {
		writeAuthError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"email": strings.ToLower(strings.TrimSpace(input.Email)),
		"role":  string(role),
	})
}

// PATCH /api/v1/orgs/{id}/members/{user_id} — change a member's role. Admin
// and owner; the last owner is protected and owner is never granted
// (US-AD04 AC3).
func (a authAPI) updateMember(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var input memberRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	role, err := auth.ParseRole(input.Role)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	target := r.PathValue("user_id")
	if err := a.store.ChangeMemberRole(orgCtx.workspace.ID, orgCtx.email, target, role); err != nil {
		writeAuthError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id": target,
		"role":    string(role),
	})
}

// DELETE /api/v1/orgs/{id}/members/{user_id} — remove a member.
func (a authAPI) removeMember(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	target := r.PathValue("user_id")
	if err := a.store.RemoveMemberByID(orgCtx.workspace.ID, orgCtx.email, target); err != nil {
		writeAuthError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// withOrgContext stores the resolved tenant on the request context. Handlers
// read it through currentOrgContext and never re-derive it from input.
func withOrgContext(ctx context.Context, resolved orgContext) context.Context {
	return context.WithValue(ctx, orgContextKey{}, resolved)
}

func currentOrgContext(r *http.Request) (orgContext, error) {
	resolved, ok := r.Context().Value(orgContextKey{}).(orgContext)
	if !ok || !resolved.resolved {
		return orgContext{}, errors.New("no resolved org context")
	}
	return resolved, nil
}

type orgContextKey struct{}
