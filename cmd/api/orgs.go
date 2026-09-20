package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

// memberResponse is one row of GET /api/v1/orgs/{id}/members.
//
// `created_at` is when the membership row was written, not when the user account
// was created: the SQL join selects `m.created_at` (see
// internal/store/queries/queries.sql ListMembers) and the domain field is
// populated all the way through, so the design's "Joined" column renders a real
// fact instead of a placeholder. There is deliberately no `status` field — the
// schema has no pending-invite state to report one from.
type memberResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
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
//
// The requested id is the {id} from the route when the pattern has one, and
// only falls back to the X-Org-ID header for routes without an id. The path
// parameter is the resource the caller named, so it is what the membership is
// checked against; honouring the header over the path would let a caller
// authenticate against their own org and then address another tenant's.
func (a authAPI) orgContextMiddleware(next http.Handler) http.Handler {
	return a.contextMiddleware(next, true)
}

// orgHeaderContextMiddleware resolves the tenant only from X-Org-ID. M1
// resource routes use {id} for a project, board, or task, so it must not be
// mistaken for an organization id.
func (a authAPI) orgHeaderContextMiddleware(next http.Handler) http.Handler {
	return a.contextMiddleware(next, false)
}

func (a authAPI) contextMiddleware(next http.Handler, pathID bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(a.store, r)
		if !ok {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		requestedID := strings.TrimSpace(r.Header.Get("X-Org-ID"))
		if pathID {
			requestedID = strings.TrimSpace(r.PathValue("id"))
			if requestedID == "" {
				requestedID = strings.TrimSpace(r.Header.Get("X-Org-ID"))
			}
		}
		workspace, role, err := a.store.ResolveWorkspace(r.Context(), user.Email, requestedID)
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
		if !a.store.Authorize(r.Context(), orgCtx.workspace.ID, orgCtx.email, minimum) {
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

	memberships, err := a.store.Workspaces(r.Context(), user.Email)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	orgs := make([]map[string]string, 0, len(memberships))
	for _, membership := range memberships {
		orgs = append(orgs, map[string]string{
			"id":   membership.WorkspaceID,
			"name": membership.Name,
			"slug": membership.Slug,
			"role": string(membership.Role),
			"kind": membership.Kind,
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

	workspace, err := a.store.CreateWorkspace(r.Context(), user.Email, input.Name, input.Slug)
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

	name := strings.TrimSpace(input.Name)
	if err := a.store.UpdateWorkspace(r.Context(), orgCtx.workspace.ID, orgCtx.email, name); err != nil {
		writeAuthError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":   orgCtx.workspace.ID,
		"name": name,
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

	members, err := a.store.Members(r.Context(), orgCtx.workspace.ID, orgCtx.email)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	rows := make([]memberResponse, 0, len(members))
	for _, member := range members {
		rows = append(rows, memberResponse{
			UserID:    member.UserID,
			Email:     member.Email,
			Name:      member.Name,
			Role:      string(member.Role),
			CreatedAt: member.CreatedAt.Format(time.RFC3339Nano),
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

	if err := a.store.AddMember(r.Context(), orgCtx.workspace.ID, orgCtx.email, input.Email, role); err != nil {
		writeAuthError(w, err)
		return
	}

	// AC1: the invite is emailed. The membership row is already written, so a
	// dead relay must not fail the request — the member exists either way, and
	// the failure is the operator's to see in the log. Both seams are optional:
	// a unit test builds an API with neither.
	if a.mailer != nil {
		to := strings.ToLower(strings.TrimSpace(input.Email))
		if err := a.mailer.SendInvite(r.Context(), to, orgCtx.workspace.Name, string(role), a.signInLink()); err != nil && a.logger != nil {
			a.logger.Error("member invite mail failed", "error", err)
		}
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
	if err := a.store.ChangeMemberRole(r.Context(), orgCtx.workspace.ID, orgCtx.email, target, role); err != nil {
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
	if err := a.store.RemoveMemberByID(r.Context(), orgCtx.workspace.ID, orgCtx.email, target); err != nil {
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
