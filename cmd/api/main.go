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
	"agentdeck/internal/board"
	"agentdeck/internal/config"
	"agentdeck/internal/migrate"
	"agentdeck/internal/notify"
	"agentdeck/internal/providerreg"
	"agentdeck/internal/skill"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authAPI struct {
	store *auth.Store
	// mailer and logger are only set by the real server. Handlers must tolerate
	// both being nil so a unit test can exercise them without a mail relay.
	mailer     notifier
	logger     *slog.Logger
	appBaseURL string
	// masterKey is the raw AGENTDECK_MASTER_KEY, decoded on use by internal/crypto.
	// It is never logged, never echoed, and never compared to anything a caller
	// sends. An empty value leaves the credential endpoints answering 500 rather
	// than storing a key in the clear.
	masterKey string
	// projects is nil in unit tests that do not exercise registration seeding,
	// so every use is nil-guarded.
	projects projectSeeder
}

// projectSeeder creates the starter project a brand-new workspace opens with.
//
// US-AD09 AC4 requires that creating a board never depends on the operator
// first creating a project ("tidak memerlukan pemilihan organisasi maupun
// project baru bila pengguna belum punya project"). Registration therefore
// seeds one project, and the board form only ever picks an existing one.
//
// The dependency lives here, at the composition root, because it spans two
// domains: auth owns registration, board owns projects. Neither package may
// import the other.
type projectSeeder interface {
	ListProjects(ctx context.Context, orgID string) ([]board.Project, error)
	CreateProject(ctx context.Context, orgID, slug, name string) (board.Project, error)
}

// The concrete implementation must satisfy the seam, or main.go would wire a
// service that silently fails to seed.
var _ projectSeeder = (*board.Service)(nil)

// starterProjectSlug is the slug every new workspace's starter project gets.
// Slug uniqueness is per org (projects_org_slug_key) and the org was created a
// moment earlier, so a constant cannot collide with anything the operator owns.
const starterProjectSlug = "getting-started"

// seedStarterProject gives a freshly registered workspace one project to hold
// its first board. A failure is logged and swallowed: the account and session
// are already valid, and the operator can still create a project by hand, so
// failing the signup over a convenience row would be the worse outcome.
func (a authAPI) seedStarterProject(ctx context.Context, workspace auth.Workspace) {
	if a.projects == nil {
		return
	}
	existing, err := a.projects.ListProjects(ctx, workspace.ID)
	if err != nil {
		a.logSeedFailure(workspace.ID, err)
		return
	}
	if len(existing) > 0 {
		// Register reuses a user's existing personal workspace on a retried
		// signup, so this path is reachable with the operator's own projects
		// already present. Seeding again would collide with or crowd them out.
		return
	}
	if _, err := a.projects.CreateProject(ctx, workspace.ID, starterProjectSlug, "Getting started"); err != nil {
		a.logSeedFailure(workspace.ID, err)
	}
}

func (a authAPI) logSeedFailure(workspaceID string, err error) {
	if a.logger == nil {
		return
	}
	a.logger.Warn("starter project not seeded", "workspace_id", workspaceID, "error", err)
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	OrgName  string `json:"org_name"`
}

// writeAuthError maps auth domain errors to the contract status codes so the
// same failure always yields the same HTTP code from every handler
// (US-AD01 AC2/AC4, US-AD02 AC2, US-AD03 AC1).
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailExists), errors.Is(err, auth.ErrSlugTaken):
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
	case errors.Is(err, auth.ErrResetTokenInvalid):
		// US-AD88 AC3: expired, already used, and unknown tokens all answer
		// 410 with the same body, so a probe cannot tell them apart.
		http.Error(w, err.Error(), http.StatusGone)
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

	// US-AD09 AC4: the starter project is what lets the very next screen create
	// a board without a project step.
	a.seedStarterProject(r.Context(), workspace)

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
	migrationPool, err := pgxpool.New(ctx, cfg.MigrationDatabaseURL)
	if err != nil {
		logger.Error("migration database connection failed", "error", err)
		return
	}
	if err := migrationPool.Ping(ctx); err != nil {
		migrationPool.Close()
		logger.Error("migration database ping failed", "error", err)
		return
	}
	if err := migrate.Apply(ctx, migrationPool); err != nil {
		migrationPool.Close()
		logger.Error("migration failed", "error", err)
		return
	}
	migrationPool.Close()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("runtime database connection failed", "error", err)
		return
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("runtime database ping failed", "error", err)
		return
	}

	store := auth.NewStore(auth.NewPgxRepository(pool))
	boardService := board.NewService(board.NewPgxRepository(pool))

	var mailer notifier = notify.LogMailer{Logger: logger}
	if cfg.SMTP.Host != "" {
		mailer = notify.SMTPMailer{Config: notify.SMTPConfig{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
		}}
	}
	// US-AD09 AC4: `projects` is the same board service the routes use, so a
	// new workspace opens with one project and the board form never has to ask
	// the operator to create one first.
	api := authAPI{store: store, mailer: mailer, logger: logger, appBaseURL: cfg.AppBaseURL, projects: boardService, masterKey: cfg.MasterKey}
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
	// US-AD89: the profile is the one resource a user may read and write
	// without a role, because it belongs to them and not to a workspace.
	mux.HandleFunc("PATCH /api/v1/auth/me", api.updateMe)
	// AC4: a foreign or unknown user id answers 404, never 403.
	mux.HandleFunc("GET /api/v1/users/{id}", api.userProfile)
	// US-AD88: both reset endpoints are public (AC4) — a locked-out operator
	// has no session to authenticate with.
	mux.HandleFunc("POST /api/v1/auth/password/reset-request", api.requestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password/reset", api.resetPassword)

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

	// US-AD109: the provider registry is org-scoped, so it rides the runtime
	// pool the same way the skill library does. It is wired with the upstream
	// probe and the credential decrypter because two of its seven endpoints
	// (verify, models) actually call the operator's endpoint.
	//
	// It is built before the board routes on purpose: phase 5 made the agent
	// form choose a provider, so the agent write path resolves that choice
	// through this service (AC6) and checks the model against the provider's
	// fetched list (AC10).
	providerSvc := newProviderService(pool, cfg.MasterKey)
	registerBoardRoutes(mux, api, boardService, providerSvc)
	// The agent registry and the skill library are their own route files, so
	// each owns its role table in one place (see registerAgentRoutes /
	// registerAgentSkillRoutes). Wiring them here is the one line that makes
	// them reachable — they were written but unmounted, which left every
	// agent-catalog, PATCH /agents/{id} and /agent-skills request a 404.
	registerAgentRoutes(mux, api, boardService, providerSvc)
	registerAgentCredentialRoutes(mux, api, boardService)
	registerAgentSkillRoutes(mux, api, skill.NewService(skill.NewPgxRepository(pool)))
	registerProviderRoutes(mux, api, providerSvc)

	// AC7's automatic half: refresh model lists older than 24 hours without
	// anyone pressing the button. It runs as nobody — no role required, no HTTP
	// surface — because the alternative (refresh on read) would let a Viewer
	// spend the workspace's credential. Stopped by the same ctx as the server.
	go providerreg.NewModelRefresher(providerSvc, logger).Run(ctx)

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
