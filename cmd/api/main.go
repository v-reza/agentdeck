package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"agentdeck/internal/auth"
	"agentdeck/internal/config"
	"agentdeck/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authAPI struct {
	store *auth.Store
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	OrgName  string `json:"org_name"`
}

// writeAuthError maps auth domain errors to the contract status codes so the
// same failure always yields the same HTTP code from every handler
// (US-AD01 AC2/AC4, US-AD02 AC2).
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, auth.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, auth.ErrAccountLocked):
		w.Header().Set("Retry-After", "900")
		http.Error(w, err.Error(), http.StatusTooManyRequests)
	case errors.Is(err, auth.ErrInvalidCredentials):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrLastOwner):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, auth.ErrMemberNotFound), errors.Is(err, auth.ErrWorkspaceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (a authAPI) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	user, workspace, sessionToken, err := a.store.Register(
		r.Context(),
		input.Email,
		input.Password,
		input.Name,
		input.OrgName,
	)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	http.SetCookie(w, auth.SessionCookie(sessionToken))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id":      user.ID,
		"workspace_id": workspace.ID,
	})
}

func (a authAPI) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	sessionToken, err := a.store.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	http.SetCookie(w, auth.SessionCookie(sessionToken))
	w.WriteHeader(http.StatusOK)
}

// currentUser resolves the session from the cookie or the
// Authorization: Bearer *** header (ARCHITECTURE 11.1), returning the
// authenticated user. ok=false means 401 for every protected handler.
func currentUser(store *auth.Store, r *http.Request) (auth.User, bool) {
	token := sessionToken(r)
	if token == "" {
		return auth.User{}, false
	}
	return store.Authenticate(r.Context(), token)
}

// sessionToken reads the opaque token from the cookie or the
// Authorization: Bearer <token> header (ARCHITECTURE 11.1).
func sessionToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (a authAPI) me(w http.ResponseWriter, r *http.Request) {
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
	workspaces := make([]map[string]string, 0, len(memberships))
	for _, membership := range memberships {
		workspaces = append(workspaces, map[string]string{
			"id":   membership.WorkspaceID,
			"name": membership.Name,
			"slug": membership.Slug,
			"role": string(membership.Role),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         user.ID,
		"email":      user.Email,
		"name":       user.Name,
		"workspaces": workspaces,
	})
}

func (a authAPI) logout(w http.ResponseWriter, r *http.Request) {
	token := sessionToken(r)
	if token == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if _, ok := a.store.Authenticate(r.Context(), token); !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	if err := a.store.Logout(r.Context(), token); err != nil {
		writeAuthError(w, err)
		return
	}
	http.SetCookie(w, auth.ExpiredSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.FromEnvironment()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("database ping failed", "error", err)
		return
	}
	if err := migrate.Apply(ctx, pool); err != nil {
		logger.Error("migration failed", "error", err)
		return
	}

	store := auth.NewStore(auth.NewPgxRepository(pool))
	api := authAPI{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintln(w, "ready")
	})
	mux.HandleFunc("POST /api/v1/auth/register", api.register)
	mux.HandleFunc("POST /api/v1/auth/login", api.login)
	mux.HandleFunc("POST /api/v1/auth/logout", api.logout)
	mux.HandleFunc("GET /api/v1/auth/me", api.me)

	// Org and membership routes. The middleware resolves the tenant from the
	// session plus X-Org-ID (never from the path), so the {id} in the path is
	// only a pointer: a foreign org id fails resolution, not the lookup.
	mux.HandleFunc("GET /api/v1/orgs", api.listOrgs)
	mux.HandleFunc("POST /api/v1/orgs", api.createOrg)

	// orgRoute chains authentication + tenant resolution + the role gate in
	// one place, so no org-scoped handler can be registered without all three.
	orgRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgContextMiddleware(api.requireRole(handler, minimum)))
	}

	orgRoute("GET /api/v1/orgs/{id}", http.HandlerFunc(api.getOrg), auth.Viewer)
	orgRoute("PATCH /api/v1/orgs/{id}", http.HandlerFunc(api.updateOrg), auth.Owner)
	orgRoute("GET /api/v1/orgs/{id}/members", http.HandlerFunc(api.listMembers), auth.Viewer)
	orgRoute("POST /api/v1/orgs/{id}/members", http.HandlerFunc(api.addMember), auth.Admin)
	orgRoute("PATCH /api/v1/orgs/{id}/members/{user_id}", http.HandlerFunc(api.updateMember), auth.Admin)
	orgRoute("DELETE /api/v1/orgs/{id}/members/{user_id}", http.HandlerFunc(api.removeMember), auth.Admin)

	server := &http.Server{Addr: cfg.Addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Shutdown)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	logger.Info("api listening", "addr", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
	}
}
