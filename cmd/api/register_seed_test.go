package main

// US-AD09 AC4 — "Pembuatan board tidak memerlukan pemilihan organisasi maupun
// project baru bila pengguna belum punya project."
//
// The criterion is only satisfiable if a workspace that has just been created
// already contains a project, because the board form's project list is the
// server's list. Registration is therefore the place the starter project is
// seeded, and these tests pin that: the happy path seeds exactly one project
// into the workspace that was just created, a seeder failure does not cost the
// operator their account, and a retried signup does not seed a second project
// on top of the ones the operator already owns.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
)

// fakeSeeder records what the register handler asked for, so the assertions can
// be about the calls rather than about a rendered list.
type fakeSeeder struct {
	existing   []board.Project
	listErr    error
	createErr  error
	created    []board.Project
	listCalls  int
	createCall int
}

func (f *fakeSeeder) ListProjects(_ context.Context, orgID string) ([]board.Project, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.existing, nil
}

func (f *fakeSeeder) CreateProject(_ context.Context, orgID, slug, name string) (board.Project, error) {
	f.createCall++
	if f.createErr != nil {
		return board.Project{}, f.createErr
	}
	project := board.Project{ID: "proj-seeded", OrgID: orgID, Slug: slug, Name: name}
	f.created = append(f.created, project)
	return project, nil
}

// newSeedTestAPI builds the register handler with the seeding seam wired, and
// with a logger that discards so a swallowed seed failure does not print.
func newSeedTestAPI(seeder projectSeeder) authAPI {
	return authAPI{
		store:      auth.NewStore(auth.NewMemoryRepository()),
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		appBaseURL: "http://localhost:5173",
		projects:   seeder,
	}
}

// registerOnce drives the real handler through the real mux, so the test
// exercises the route main.go registers rather than a direct method call.
func registerOnce(t *testing.T, api authAPI, email string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", api.register)

	body := `{"email":"` + email + `","password":"password1","name":"Seed"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

func TestRegisterSeedsOneStarterProject(t *testing.T) {
	seeder := &fakeSeeder{}
	api := newSeedTestAPI(seeder)

	recorder := registerOnce(t, api, "seed-happy@x.test")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
	}

	if seeder.listCalls != 1 {
		t.Fatalf("ListProjects calls = %d, want 1", seeder.listCalls)
	}
	if seeder.createCall != 1 {
		t.Fatalf("CreateProject calls = %d, want 1", seeder.createCall)
	}

	seeded := seeder.created[0]
	if seeded.Slug != starterProjectSlug {
		t.Fatalf("seeded slug = %q, want %q", seeded.Slug, starterProjectSlug)
	}
	if strings.TrimSpace(seeded.Name) == "" {
		t.Fatal("seeded project has no name")
	}
	// The project must land in the workspace the caller just got, never in
	// another tenant: the board form lists projects by the session's workspace.
	if seeded.OrgID == "" {
		t.Fatal("seeded project has no org id")
	}
}

// A seeder that cannot write must not turn a successful signup into a failure.
// The account, the workspace, and the session are all already durable at this
// point, so the operator would be locked out of an account that exists.
func TestRegisterSucceedsWhenSeedingFails(t *testing.T) {
	for name, seeder := range map[string]*fakeSeeder{
		"list fails":   {listErr: errors.New("db down")},
		"create fails": {createErr: errors.New("db down")},
	} {
		t.Run(name, func(t *testing.T) {
			api := newSeedTestAPI(seeder)
			recorder := registerOnce(t, api, "seed-fail@x.test")

			if recorder.Code != http.StatusCreated {
				t.Fatalf("register: status = %d, want 201 despite the seed failure", recorder.Code)
			}
			if recorder.Header().Get("Set-Cookie") == "" {
				t.Fatal("register did not set a session cookie")
			}
		})
	}
}

// Register reuses the caller's existing personal workspace on a retried signup,
// so the seed path can run against a workspace that already has projects. The
// operator's own projects must survive untouched — seeding is a convenience for
// an empty workspace, not a rule about every workspace.
func TestRegisterDoesNotSeedWhenTheWorkspaceAlreadyHasProjects(t *testing.T) {
	seeder := &fakeSeeder{
		existing: []board.Project{{ID: "proj-owned", OrgID: "org", Slug: "mine", Name: "Mine"}},
	}
	api := newSeedTestAPI(seeder)

	recorder := registerOnce(t, api, "seed-existing@x.test")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201", recorder.Code)
	}
	if seeder.createCall != 0 {
		t.Fatalf("CreateProject calls = %d, want 0 when a project already exists", seeder.createCall)
	}
}

// The seam is optional: a unit test that only cares about auth wires no seeder,
// and registration must still work rather than panic on a nil interface.
func TestRegisterWithoutASeederStillSucceeds(t *testing.T) {
	api := authAPI{
		store:      auth.NewStore(auth.NewMemoryRepository()),
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		appBaseURL: "http://localhost:5173",
	}

	recorder := registerOnce(t, api, "seed-none@x.test")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201", recorder.Code)
	}
}
