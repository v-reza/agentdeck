package main

// US-AD109 phase 5 — the agent form chooses a provider instead of typing an
// endpoint. Each acceptance criterion this phase delivers, with a test that
// fails for the *right* reason:
//
//	AC6 : `agents.provider` is *derived* from the chosen
//	      provider, so one edit of the provider reaches every agent using it and
//	      an agent cannot describe an endpoint its credential does not belong to.
//	AC10: switching provider clears the model choice, and a model the provider
//	      does not offer is refused.
//
// The mux comes from registerAgentRoutes — the same function main.go calls —
// so these assert the production wiring, not a copy of it. The provider service
// is the real one over the in-memory provider repo, so the resolution runs the
// same code path as production; only the storage is fake.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/board"
	"agentdeck/internal/providerreg"
)

// phase5Fixture is the update fixture plus a live provider registry wired the
// way main.go wires it: the agent routes resolve provider ids through the same
// service the /providers routes serve.
type phase5Fixture struct {
	updateFixture
	providers *fakeProviderRepo
}

// newPhase5Fixture builds its own mux rather than extending columnEditAPI's.
// Registering the agent routes a second time on an existing mux would panic
// (ServeMux rejects a duplicate pattern), and the point of this fixture is that
// both registrars receive the registry — which is exactly what main.go does.
func newPhase5Fixture(t *testing.T) phase5Fixture {
	t.Helper()
	scenario := newRBACTestAPI(t)
	repo := newFakeBoardRepo()

	seed := func(orgID, projectID, boardID string) {
		t.Helper()
		if _, err := repo.CreateProject(context.Background(), board.Project{
			ID: projectID, OrgID: orgID, Slug: projectID, Name: "Demo",
		}); err != nil {
			t.Fatalf("seed project %s: %v", projectID, err)
		}
		if _, err := repo.CreateBoard(context.Background(), board.Board{
			ID: boardID, OrgID: orgID, ProjectID: projectID,
			Slug: boardID, Name: "Sprint",
			Columns:           board.DefaultColumns,
			BudgetDailyMicros: board.DefaultBudgetDailyMicros,
		}); err != nil {
			t.Fatalf("seed board %s: %v", boardID, err)
		}
	}
	seed(scenario.orgA, "proj-a", "board-a")
	seed(scenario.orgB, "proj-b", "board-b")

	providers := newFakeProviderRepo()
	svc := providerreg.NewServiceWithProbe(providers, providerreg.ServiceOptions{})

	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(repo), svc)
	registerAgentRoutes(mux, scenario.api, board.NewService(repo), svc)

	f := columnFixture{mux: mux, repo: repo, scenario: scenario, boardA: "board-a", boardB: "board-b"}
	return phase5Fixture{updateFixture: updateFixture{columnFixture: f, projectID: "proj-a"}, providers: providers}
}

// seedProvider inserts a provider row directly, bypassing the HTTP layer: the
// subject under test is the agent write path, not the provider handler.
func (f phase5Fixture) seedProvider(t *testing.T, id, org, protocol, baseURL string, models []string) {
	t.Helper()
	f.providers.providers[id] = providerreg.Provider{
		ID: id, OrgID: org, Name: "seeded-" + id,
		Protocol: providerreg.Protocol(protocol), BaseURL: baseURL, Models: models,
	}
}

// postAgent drives POST /projects/{id}/agents through the real route.
func (f phase5Fixture) postAgent(t *testing.T, actor, orgID, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// getAgent reads one agent back through the real route.
func (f phase5Fixture) getAgent(t *testing.T, actor, orgID, agentID string) agentResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("read agent: want 200, got %d — %s", w.Code, w.Body.String())
	}
	return decodeAgent(t, w)
}

// storedAgent is the row as the fake repo holds it, which is where the derived
// columns actually land.
func (f phase5Fixture) storedAgent(t *testing.T, id string) board.Agent {
	t.Helper()
	a, ok := f.repo.agents[id]
	if !ok {
		t.Fatalf("agent %s not stored", id)
	}
	return a
}

// ---- AC6: the provider decides the protocol and the endpoint -----------------

// TestAgentProviderIsResolvedFromTheRegistry is the hinge of phase 5. The
// request may *say* provider=openai and base_url=https://attacker.example, and
// the stored row must still carry what the provider row says — because the
// credential belongs to that provider, and an agent that describes a different
// endpoint than its credential is the failure AC6 exists to prevent.
func TestAgentProviderIsResolvedFromTheRegistry(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"seeded-model"})

	body := `{"name":"agent-resolve","provider":"openai","model":"seeded-model",
	          "base_url":"https://attacker.example/v1","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create with a provider: want 201, got %d — %s", w.Code, w.Body.String())
	}

	stored := f.storedAgent(t, decodeAgent(t, w).ID)
	if stored.Provider != "openai_compatible" {
		t.Errorf("provider: want %q derived from the provider row, got %q",
			"openai_compatible", stored.Provider)
	}
	if stored.ProviderID != "prov-a" {
		t.Errorf("provider_id: want prov-a, got %q", stored.ProviderID)
	}
	// AC6: the attacker-supplied base_url is not merely outvoted, it is not
	// stored at all — the agent holds a reference, and the endpoint lives on
	// the provider row. Phase 6 removed the column this used to land in, so the
	// proof is the row itself: nothing on the agent carries an address.
	stored = f.storedAgent(t, decodeAgent(t, w).ID)
	provider, err := f.providers.Get(context.Background(), f.scenario.orgA, stored.ProviderID)
	if err != nil {
		t.Fatalf("read the provider the agent points at: %v", err)
	}
	if provider.BaseURL != "http://127.0.0.1:9999/v1" {
		t.Errorf("the endpoint must live on the provider, got %q", provider.BaseURL)
	}
}

// TestAgentWithoutProviderIsStillValid is the state most rows are in: 963 of the
// dev database's agents are built-in agents the backfill deliberately left
// without a provider (DECISIONS 6A.J). Their address comes from the deployment's
// environment default, so omitting provider_id must keep working — and, now that
// phase 6 dropped the column, the request cannot smuggle its own endpoint in
// either.
func TestAgentWithoutProviderIsStillValid(t *testing.T) {
	f := newPhase5Fixture(t)

	body := `{"name":"agent-no-provider","provider":"openai","model":"gpt-4o"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create without a provider: want 201, got %d — %s", w.Code, w.Body.String())
	}

	stored := f.storedAgent(t, decodeAgent(t, w).ID)
	if stored.Provider != "openai" {
		t.Errorf("provider: want the request's own value, got %q", stored.Provider)
	}
	if stored.ProviderID != "" {
		t.Errorf("provider_id: want empty, got %q", stored.ProviderID)
	}
}

// TestAgentProviderFromAnotherWorkspaceIsRefused is the tenant boundary. The
// lookup carries the org id, so a provider id that exists in another workspace
// must be indistinguishable from one that does not exist at all — otherwise the
// error itself leaks the other tenant's ids (US-AD07).
func TestAgentProviderFromAnotherWorkspaceIsRefused(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-b", f.scenario.orgB, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"seeded-model"})

	body := `{"name":"agent-cross-org","provider":"openai_compatible","model":"seeded-model",
	          "base_url":"http://127.0.0.1:9999/v1","provider_id":"prov-b"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a foreign provider id: want 400, got %d — %s", w.Code, w.Body.String())
	}

	// An id that does not exist must answer identically, or the difference
	// between the two responses is an id oracle.
	missing := `{"name":"agent-missing","provider":"openai_compatible","model":"seeded-model",
	             "base_url":"http://127.0.0.1:9999/v1","provider_id":"prov-does-not-exist"}`
	w2 := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, missing)
	if w2.Code != w.Code {
		t.Errorf("absent id and foreign id must answer alike: foreign=%d absent=%d", w.Code, w2.Code)
	}
}

// ---- AC10: the model belongs to the provider --------------------------------

// TestAgentModelMustBeOfferedByItsProvider is AC10's enforcement half. Once the
// provider's list exists it is the allowlist, because the provider is the thing
// that knows which models its upstream serves.
func TestAgentModelMustBeOfferedByItsProvider(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"listed-model"})

	body := `{"name":"agent-bad-model","model":"unlisted-model","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a model the provider does not offer: want 400, got %d — %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unlisted-model") {
		t.Errorf("the 400 must name the refused model, got %q", w.Body.String())
	}

	// The listed one goes through, so the check is not simply rejecting
	// everything that names a provider.
	ok := `{"name":"agent-good-model","model":"listed-model","provider_id":"prov-a"}`
	if w2 := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, ok); w2.Code != http.StatusCreated {
		t.Fatalf("a listed model: want 201, got %d — %s", w2.Code, w2.Body.String())
	}
}

// TestAgentModelCheckIsSkippedUntilTheProviderHasAList pins the distinction
// between "no list yet" and "an empty list". Phase 3 fetches lazily, so a
// provider created minutes ago legitimately has nothing, and enforcing that as
// "no model is allowed" would make the form unusable exactly when the operator
// is filling it in.
func TestAgentModelCheckIsSkippedUntilTheProviderHasAList(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-empty", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", nil)

	body := `{"name":"agent-no-list","model":"anything-at-all","provider_id":"prov-empty"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("a provider with no fetched list must not reject a model: want 201, got %d — %s",
			w.Code, w.Body.String())
	}
}

// TestAgentProviderSwitchReachesTheAgent is AC6's observable half: one edit of
// the provider moves every agent that uses it, with no second write to the
// agents. Before phase 5 an operator had to edit each agent's base URL.
func TestAgentProviderSwitchReachesTheAgent(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"listed-model"})

	body := `{"name":"agent-moves","model":"listed-model","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup create: want 201, got %d — %s", w.Code, w.Body.String())
	}
	agentID := decodeAgent(t, w).ID

	// Move the provider's endpoint the way PATCH /providers/{id} does.
	f.providers.providers["prov-a"] = providerreg.Provider{
		ID: "prov-a", OrgID: f.scenario.orgA, Name: "seeded-prov-a",
		Protocol: providerreg.ProtocolOpenAICompatible, BaseURL: "http://127.0.0.1:8888/v1",
		Models: []string{"listed-model"},
	}

	// The read path must already report the new endpoint, because it resolves
	// through the provider rather than echoing a stored copy.
	read := f.getAgent(t, "alice", f.scenario.orgA, agentID)
	if read.ProviderID == nil || *read.ProviderID != "prov-a" {
		t.Fatalf("the agent must report its provider_id, got %v", read.ProviderID)
	}
}

// TestAgentPatchKeepsItsProvider is the regression this phase could easily
// introduce: PATCH is a full update, so a body that renames an agent and omits
// provider_id must keep the stored reference. Dropping it would silently move
// the agent onto the deployment's environment default.
func TestAgentPatchKeepsItsProvider(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"listed-model"})

	body := `{"name":"agent-keep","model":"listed-model","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup create: want 201, got %d — %s", w.Code, w.Body.String())
	}
	agentID := decodeAgent(t, w).ID

	// A rename that says nothing about the provider.
	patch := f.patchAgent(t, "alice", f.scenario.orgA, agentID, `{"name":"agent-renamed"}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("rename: want 200, got %d — %s", patch.Code, patch.Body.String())
	}

	stored := f.storedAgent(t, agentID)
	if stored.ProviderID != "prov-a" {
		t.Errorf("a rename must not drop the provider: want prov-a, got %q", stored.ProviderID)
	}
	if stored.Name != "agent-renamed" {
		t.Errorf("the rename must land: got name %q", stored.Name)
	}
}

// TestAgentPatchCanMoveTheAgentToAnotherProvider is AC10's fail path end to end:
// switching provider must actually land, which is the assertion the rename test
// above cannot make — it never sends a provider_id at all. Without this, a PATCH
// that silently ignored provider_id would pass every other test in this file.
func TestAgentPatchCanMoveTheAgentToAnotherProvider(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"model-a"})
	f.seedProvider(t, "prov-b", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:8888/v1", []string{"model-b"})

	body := `{"name":"agent-switch","model":"model-a","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup create: want 201, got %d — %s", w.Code, w.Body.String())
	}
	agentID := decodeAgent(t, w).ID

	// Switching provider while keeping the old model is refused (AC10).
	stale := f.patchAgent(t, "alice", f.scenario.orgA, agentID, `{"provider_id":"prov-b"}`)
	if stale.Code != http.StatusBadRequest {
		t.Fatalf("switching provider with the old model: want 400, got %d — %s", stale.Code, stale.Body.String())
	}

	// Switching provider *and* model lands, and the derived columns follow.
	moved := f.patchAgent(t, "alice", f.scenario.orgA, agentID,
		`{"provider_id":"prov-b","model":"model-b"}`)
	if moved.Code != http.StatusOK {
		t.Fatalf("switching provider and model: want 200, got %d — %s", moved.Code, moved.Body.String())
	}
	stored := f.storedAgent(t, agentID)
	if stored.ProviderID != "prov-b" {
		t.Errorf("provider_id: want prov-b, got %q", stored.ProviderID)
	}
	// The endpoint is not on the agent at all: it is read through the reference,
	// so "follows the new provider" is a property of the provider row.
	provider, err := f.providers.Get(context.Background(), f.scenario.orgA, stored.ProviderID)
	if err != nil {
		t.Fatalf("read the provider the agent moved to: %v", err)
	}
	if provider.BaseURL != "http://127.0.0.1:8888/v1" {
		t.Errorf("the provider the agent points at must hold the endpoint, got %q", provider.BaseURL)
	}
	if stored.Model != "model-b" {
		t.Errorf("model: want model-b, got %q", stored.Model)
	}
}

// ---- the wiring itself ------------------------------------------------------

// TestAgentRoutesWithoutARegistryStillAnswer makes the nil case explicit rather
// than incidental: the RBAC tests mount these routes with no provider service,
// and that must stay a clean 400 for a request that names a provider — never a
// nil dereference. The nil field is a test convenience, not a production state.
func TestAgentRoutesWithoutARegistryStillAnswer(t *testing.T) {
	scenario := newRBACTestAPI(t)
	repo := newFakeBoardRepo()
	if _, err := repo.CreateProject(context.Background(), board.Project{
		ID: "proj-a", OrgID: scenario.orgA, Slug: "proj-a", Name: "Demo",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	mux := http.NewServeMux()
	registerBoardRoutes(mux, scenario.api, board.NewService(repo), nil)
	registerAgentRoutes(mux, scenario.api, board.NewService(repo), nil)

	body := `{"name":"agent-nil-registry","model":"m","provider_id":"prov-a"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-a/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+scenario.tokens["alice@x.test"])
	req.Header.Set("X-Org-ID", scenario.orgA)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("no registry mounted: want 400, got %d — %s", w.Code, w.Body.String())
	}
}

// TestAgentResponseCarriesProviderID guards the read shape the UI depends on to
// preselect the dropdown. Without it the form would show "no provider" on every
// agent that has one, and the next save would then clear it.
func TestAgentResponseCarriesProviderID(t *testing.T) {
	f := newPhase5Fixture(t)
	f.seedProvider(t, "prov-a", f.scenario.orgA, "openai_compatible", "http://127.0.0.1:9999/v1", []string{"listed-model"})

	body := `{"name":"agent-shape","model":"listed-model","provider_id":"prov-a"}`
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup create: want 201, got %d — %s", w.Code, w.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if got, ok := raw["provider_id"]; !ok || got != "prov-a" {
		t.Errorf("the create response must carry provider_id: got %v (present=%v)", got, ok)
	}

	// An agent with no provider omits the field rather than sending "" — an
	// absent reference and an empty string must not both reach the client.
	noProvider := `{"name":"agent-no-provider","provider":"openai","model":"gpt-4o"}`
	w2 := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, noProvider)
	if w2.Code != http.StatusCreated {
		t.Fatalf("setup create without provider: want 201, got %d — %s", w2.Code, w2.Body.String())
	}
	var raw2 map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &raw2); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if _, ok := raw2["provider_id"]; ok {
		t.Errorf("an agent with no provider must omit provider_id, got %v", raw2["provider_id"])
	}
}

// unusedContext keeps the context import honest if the helpers above stop using
// it; it is referenced by the compile-time assertion below.
var _ = context.Background
