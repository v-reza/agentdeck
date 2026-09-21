package main

// US-AD96 / US-AD106 / US-AD73 — PATCH /api/v1/agents/{id} and
// GET /api/v1/agent-catalog.
//
//	US-AD96 AC1 : the catalog the form is built from is served by the API.
//	US-AD109: the agent's endpoint and credential belong to its provider; a
//	              pairing is a 400, not a CHECK violation surfacing as a 500.
//	US-AD73 AC1 : an archived agent keeps its row but leaves the assign list.
//	US-AD73 AC3 : archiving an agent that still holds a running task is a 409
//	              and the run is NOT severed.
//	US-AD73 AC4 : archiving needs owner/admin; a member may still edit fields.
//	US-AD108 AC1: every rate is labelled an estimate, by the server.
//	US-AD108 AC3: a model the table does not price is reported unpriced with
//	              zero rates — never filled with an invented number.
//
// The mux comes from registerAgentRoutes, the same function main.go will call,
// so the role gates asserted here are the production ones. A copy of the route
// table in a test would keep passing after main.go was loosened.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentdeck/internal/board"
	"agentdeck/internal/pricing"
)

// updateFixture is the column fixture's mux plus the agent routes, which is the
// wiring main.go will have. The scenario is created once: newRBACTestAPI
// registers real users and mints real sessions, so a second call would build an
// unrelated tenant whose tokens do not match the project under test.
type updateFixture struct {
	columnFixture
	projectID string
}

func newUpdateFixture(t *testing.T) updateFixture {
	t.Helper()
	f := columnEditAPI(t)
	registerAgentRoutes(f.mux, f.scenario.api, board.NewService(f.repo), nil)
	return updateFixture{columnFixture: f, projectID: "proj-a"}
}

// patchAgent drives PATCH through the real route.
func (f updateFixture) patchAgent(t *testing.T, actor, orgID, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// createAgent posts through the real route and returns the created agent.
func (f updateFixture) createAgent(t *testing.T, actor, orgID, projectID, body string) agentResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup create agent: want 201, got %d — %s", w.Code, w.Body.String())
	}
	return decodeAgent(t, w)
}

// ---- agents (US-AD96, US-AD106, US-AD73) ------------------------------------
//
// fakeBoardRepo already mirrors the two behaviours Postgres owns for this table
// (name unique per project, every read org-scoped). The three methods below
// extend that to the update/archive paths so the service's merge and guard logic
// is what is under test, not the fake's bookkeeping.
//
// UpdateAgent reproduces agents_project_name_key, because that index is the
// reason a rename onto a taken name is a 409 and not a 500.
func (r *fakeBoardRepo) UpdateAgent(_ context.Context, a board.Agent) (board.Agent, error) {
	stored, ok := r.agents[a.ID]
	if !ok || stored.OrgID != a.OrgID {
		return board.Agent{}, board.ErrNotFound
	}
	for id, existing := range r.agents {
		if id != a.ID && existing.ProjectID == stored.ProjectID && existing.Name == a.Name {
			return board.Agent{}, board.ErrAgentNameTaken
		}
	}
	r.agents[a.ID] = a
	return a, nil
}

// ArchiveAgent/UnarchiveAgent mirror the only thing Postgres does here: set or
// clear archived_at on a row the org owns. The running-task guard lives in the
// service, so it is deliberately absent from the fake.
func (r *fakeBoardRepo) ArchiveAgent(_ context.Context, id, orgID string) (board.Agent, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return board.Agent{}, board.ErrNotFound
	}
	now := time.Now()
	a.ArchivedAt = &now
	r.agents[id] = a
	return a, nil
}

func (r *fakeBoardRepo) UnarchiveAgent(_ context.Context, id, orgID string) (board.Agent, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return board.Agent{}, board.ErrNotFound
	}
	a.ArchivedAt = nil
	r.agents[id] = a
	return a, nil
}

// ---- PATCH: field update ----------------------------------------------------

// TestUpdateAgentReplacesEveryField is US-AD96 on the edit path. Each field the
// form can change is asserted, because a registry that silently dropped
// max_runtime_seconds would let a run exceed N9.
func TestUpdateAgentReplacesEveryField(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-before","provider":"openai","model":"gpt-4o"}`)

	w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{
		"name":"agent-after",
		"provider":"anthropic",
		"model":"claude-opus-4-6",
		"reasoning_effort":"high",
		"skills":["go","sql"],
		"tools":["bash"],
		"max_runtime_seconds":7200,
		"retry_policy":"always",
		"max_attempts":5
	}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}
	got := decodeAgent(t, w)
	if got.Name != "agent-after" || got.Provider != "anthropic" || got.Model != "claude-opus-4-6" {
		t.Errorf("identity fields not updated: %+v", got)
	}
	if got.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort want high, got %q", got.ReasoningEffort)
	}
	if got.MaxRuntimeSeconds != 7200 {
		t.Errorf("max_runtime_seconds want 7200, got %d", got.MaxRuntimeSeconds)
	}
	if got.RetryPolicy != "always" || got.MaxAttempts != 5 {
		t.Errorf("retry policy not updated: %q / %d", got.RetryPolicy, got.MaxAttempts)
	}
	if len(got.Skills) != 2 || got.Skills[0] != "go" || got.Skills[1] != "sql" {
		t.Errorf("skills want [go sql], got %v", got.Skills)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "bash" {
		t.Errorf("tools want [bash], got %v", got.Tools)
	}
	if got.ID != created.ID {
		t.Errorf("id changed on update: %q -> %q", created.ID, got.ID)
	}
}

// TestUpdateAgentKeepsOmittedFields pins the merge. The sqlc statement is a full
// UPDATE, so an omitted field must land on the stored value rather than on a Go
// zero — otherwise renaming an agent would silently reset its runtime to 0 and
// its retry policy to "", both of which the DDL CHECK would then reject as 500.
func TestUpdateAgentKeepsOmittedFields(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-keep","provider":"openai","model":"gpt-4o","max_runtime_seconds":7200,"retry_policy":"always","max_attempts":5,"skills":["go"]}`)

	w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{"name":"agent-renamed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rename-only patch want 200, got %d — %s", w.Code, w.Body.String())
	}
	got := decodeAgent(t, w)
	if got.Name != "agent-renamed" {
		t.Errorf("name want agent-renamed, got %q", got.Name)
	}
	if got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Errorf("omitted provider/model were reset: %q / %q", got.Provider, got.Model)
	}
	if got.MaxRuntimeSeconds != 7200 || got.RetryPolicy != "always" || got.MaxAttempts != 5 {
		t.Errorf("omitted limits were reset: %d / %q / %d",
			got.MaxRuntimeSeconds, got.RetryPolicy, got.MaxAttempts)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "go" {
		t.Errorf("omitted skills were reset: %v", got.Skills)
	}
}

// ---- PATCH: the endpoint is no longer the agent's (US-AD109 fase 6) ---------

// TestUpdateAgentIgnoresABaseURL is what replaced US-AD106 AC1's pairing rule on
// this endpoint.
//
// That rule — `openai_compatible` iff base_url is set — existed because the agent
// owned its address. Phase 6 dropped the column, so there is no longer a pairing
// to validate: a request that sends a base_url is not refused, it is *ignored*,
// because honouring it would put an endpoint back on a row whose whole point is
// that it holds only a reference (AC6). Silently accepting it is the failure
// mode this test exists to catch: an operator who thinks they redirected an agent
// would see no error and no effect.
func TestUpdateAgentIgnoresABaseURL(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-byo","provider":"openai","model":"gpt-4o"}`)

	// Every shape the old contract refused or acted on: all of them are now a
	// no-op on the address, and none of them is an error.
	for _, body := range []string{
		`{"base_url":"https://byo.test/v1"}`,
		`{"provider":"openai_compatible","base_url":"https://byo.test/v1"}`,
		`{"provider":"anthropic","base_url":"https://byo.test/v1"}`,
	} {
		w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, body)
		if w.Code != http.StatusOK {
			t.Fatalf("body %s: want 200 (a base_url is ignored, not refused), got %d — %s",
				body, w.Code, w.Body.String())
		}
		// The request may still set `provider`; it is a protocol label and is
		// stored as sent when no provider_id is given. What must never appear is
		// an address on the agent.
		if bytes.Contains(w.Body.Bytes(), []byte("byo.test")) {
			t.Errorf("body %s: the agent echoed the base_url it was sent: %s", body, w.Body.String())
		}
	}

	// The last body above set provider=anthropic, and that is a protocol label
	// the agent legitimately stores — what it must never store is the address
	// that travelled with it. An unrelated PATCH proves the row is intact and
	// still carries no endpoint.
	after := decodeAgent(t, f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{"model":"gpt-4o-mini"}`))
	if after.Provider != "anthropic" {
		t.Errorf("provider must round-trip, got %q", after.Provider)
	}
	if after.Model != "gpt-4o-mini" {
		t.Errorf("model must round-trip, got %q", after.Model)
	}
}

// TestUpdateAgentDuplicateNameIs409 mirrors US-AD20 AC3 on the update path: the
// unique index agents_project_name_key is what makes the name unique per
// project, and a rename onto an existing name is a conflict the operator fixes
// by choosing another name — not a 500 from the constraint.
func TestUpdateAgentDuplicateNameIs409(t *testing.T) {
	f := newUpdateFixture(t)
	f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-taken","provider":"openai","model":"gpt-4o"}`)
	other := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-other","provider":"openai","model":"gpt-4o"}`)

	w := f.patchAgent(t, "alice", f.scenario.orgA, other.ID, `{"name":"agent-taken"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate name on update want 409, got %d — %s", w.Code, w.Body.String())
	}
}

// ---- PATCH: archive (US-AD73) ----------------------------------------------

// TestArchiveAgentRequiresAdmin is US-AD73 AC4. PATCH carries two floors: a
// member may edit fields (US-AD96) but may not retire an agent, and a viewer is
// stopped by the route gate before the handler. This is the criterion that makes
// one endpoint with two authorities worth testing in both directions — the
// member case is the one a single Admin gate on the route would have hidden.
func TestArchiveAgentRequiresAdmin(t *testing.T) {
	f := newUpdateFixture(t)

	cases := []struct {
		actor  string
		status int
	}{
		{actor: "alice", status: http.StatusOK},        // owner
		{actor: "andre", status: http.StatusOK},        // admin
		{actor: "marta", status: http.StatusForbidden}, // member
		{actor: "vera", status: http.StatusForbidden},  // viewer
	}
	for _, tc := range cases {
		t.Run(tc.actor, func(t *testing.T) {
			// Each actor gets its own agent so the owner/admin cases do not
			// archive the row the member case is about to address.
			agent := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
				`{"name":"agent-retire-`+tc.actor+`","provider":"openai","model":"gpt-4o"}`)
			w := f.patchAgent(t, tc.actor, f.scenario.orgA, agent.ID, `{"archived":true}`)
			if w.Code != tc.status {
				t.Fatalf("%s archiving: want %d, got %d — %s", tc.actor, tc.status, w.Code, w.Body.String())
			}
			if tc.status != http.StatusOK {
				return
			}
			// AC1: the row survives and carries archived_at, which is what
			// lets every read path filter it out of the assign list.
			got := decodeAgent(t, w)
			if got.ArchivedAt == nil {
				t.Error("archived_at is null after archiving")
			}
			if got.ID != agent.ID || got.Name != agent.Name {
				t.Errorf("archived response lost identity: %+v", got)
			}
			if _, err := f.repo.GetAgent(context.Background(), agent.ID, f.scenario.orgA); err != nil {
				t.Errorf("archiving deleted the row: %v", err)
			}
		})
	}
}

// TestArchiveAgentWithRunningTaskIs409 is US-AD73 AC3: an agent holding a
// running task cannot be retired, and the run must NOT be severed. The second
// half is the one worth asserting — a handler that cancelled the task and
// answered 409 would satisfy the status code and defeat the criterion.
func TestArchiveAgentWithRunningTaskIs409(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-busy","provider":"openai","model":"gpt-4o"}`)

	task, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: "task-running", OrgID: f.scenario.orgA, BoardID: f.boardA,
		Title: "Held by agent-busy", Status: board.StatusRunning,
		AssigneeAgentID: created.ID,
	})
	if err != nil {
		t.Fatalf("seed running task: %v", err)
	}

	w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{"archived":true}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("AC3: archiving while running want 409, got %d — %s", w.Code, w.Body.String())
	}

	// The run is untouched: still running, still assigned to the same agent.
	after, err := f.repo.GetTask(context.Background(), task.ID, f.scenario.orgA)
	if err != nil {
		t.Fatalf("read task after refused archive: %v", err)
	}
	if after.Status != board.StatusRunning {
		t.Errorf("AC3: the run was severed — status %q, want running", after.Status)
	}
	if after.AssigneeAgentID != created.ID {
		t.Errorf("AC3: the run lost its assignee — %q, want %q", after.AssigneeAgentID, created.ID)
	}

	// And the agent was not archived on the way out.
	got, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA)
	if err != nil {
		t.Fatalf("read agent after refused archive: %v", err)
	}
	if got.ArchivedAt != nil {
		t.Error("AC3: the refused archive still set archived_at")
	}
}

// TestUnarchiveAgentSucceeds is the other half of US-AD73: retiring an agent is
// reversible, and unarchiving needs no running-task guard because putting an
// agent back can only add capacity.
func TestUnarchiveAgentSucceeds(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-return","provider":"openai","model":"gpt-4o"}`)

	archived := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{"archived":true}`)
	if archived.Code != http.StatusOK {
		t.Fatalf("setup archive: want 200, got %d — %s", archived.Code, archived.Body.String())
	}

	// A member may not unarchive either: it is the same authority as archiving.
	denied := f.patchAgent(t, "marta", f.scenario.orgA, created.ID, `{"archived":false}`)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member unarchiving want 403, got %d — %s", denied.Code, denied.Body.String())
	}

	w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, `{"archived":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("unarchive want 200, got %d — %s", w.Code, w.Body.String())
	}
	if got := decodeAgent(t, w); got.ArchivedAt != nil {
		t.Errorf("archived_at want null after unarchive, got %v", *got.ArchivedAt)
	}
}

// TestUpdateAgentTenantBoundaryIs404 is the boundary the ACs imply but do not
// spell out. bella owns orgB and is a member of nothing in orgA; she addresses
// orgA's agent while naming orgB as her tenant. The answer is 404 rather than
// 403 so the id's existence stays unconfirmed.
func TestUpdateAgentTenantBoundaryIs404(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-secret","provider":"openai","model":"gpt-4o"}`)

	for _, body := range []string{`{"name":"hijacked"}`, `{"archived":true}`} {
		w := f.patchAgent(t, "bella", f.scenario.orgB, created.ID, body)
		if w.Code != http.StatusNotFound {
			t.Fatalf("cross-org PATCH %s want 404, got %d — %s", body, w.Code, w.Body.String())
		}
	}
	// The row is untouched.
	got, err := f.repo.GetAgent(context.Background(), created.ID, f.scenario.orgA)
	if err != nil {
		t.Fatalf("cross-org patch removed the agent: %v", err)
	}
	if got.Name != "agent-secret" || got.ArchivedAt != nil {
		t.Errorf("cross-org patch mutated the row: %+v", got)
	}
}

// TestUpdateAgentUnknownIDIs404 keeps a stale id from becoming a 500.
func TestUpdateAgentUnknownIDIs404(t *testing.T) {
	f := newUpdateFixture(t)
	w := f.patchAgent(t, "alice", f.scenario.orgA, "01JZZZZZZZZZZZZZZZZZZZZZZZ", `{"name":"x"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown agent want 404, got %d — %s", w.Code, w.Body.String())
	}
}

// ---- GET /api/v1/agent-catalog (US-AD96 AC1, US-AD108) ---------------------

type catalogResponse struct {
	Estimate     bool           `json:"estimate"`
	Disclaimer   string         `json:"disclaimer"`
	PriceVersion int            `json:"price_version"`
	Models       []catalogModel `json:"models"`
}

func (f updateFixture) getCatalog(t *testing.T, actor, orgID string) (*httptest.ResponseRecorder, catalogResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-catalog", nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	var got catalogResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode catalog %q: %v", w.Body.String(), err)
		}
	}
	return w, got
}

// TestAgentCatalogServesEveryPricedModel is US-AD96 AC1: the form's model list
// is served by the API, not hardcoded in the client. The counts are the ones
// DECISIONS 6A.A states, so a table that silently shrank fails here.
func TestAgentCatalogServesEveryPricedModel(t *testing.T) {
	f := newUpdateFixture(t)
	w, got := f.getCatalog(t, "vera", f.scenario.orgA)
	if w.Code != http.StatusOK {
		t.Fatalf("viewer reading the catalog want 200, got %d — %s", w.Code, w.Body.String())
	}

	// 220 exact + 51 pattern: the catalog is the table's surface, nothing more.
	if len(got.Models) != len(pricing.Catalog())+len(pricing.Patterns()) {
		t.Errorf("catalog has %d models, want %d exact + %d pattern",
			len(got.Models), len(pricing.Catalog()), len(pricing.Patterns()))
	}
	if len(got.Models) != 220+51 {
		t.Errorf("catalog has %d models, want 220 exact + 51 pattern (DECISIONS 6A.A)", len(got.Models))
	}
}

// TestAgentCatalogPricesKnownModel is US-AD108 AC1/AC5 with the numbers from
// docs/PRICING.md: claude-opus-4-6 is input 5, output 25, reasoning 25 per 1M
// tokens. reasoning is asserted separately from output because collapsing the
// two is exactly the bug the 5-component formula exists to prevent.
func TestAgentCatalogPricesKnownModel(t *testing.T) {
	f := newUpdateFixture(t)
	_, got := f.getCatalog(t, "vera", f.scenario.orgA)

	var found *catalogModel
	for i := range got.Models {
		if got.Models[i].Model == "claude-opus-4-6" {
			found = &got.Models[i]
			break
		}
	}
	if found == nil {
		t.Fatal("claude-opus-4-6 is missing from the catalog")
	}
	if found.Source != string(pricing.SourceCatalog) {
		t.Errorf("price_source want catalog, got %q", found.Source)
	}
	if found.Input.USDPer1M != 5 {
		t.Errorf("input want 5 USD/1M, got %v", found.Input.USDPer1M)
	}
	if found.Output.USDPer1M != 25 {
		t.Errorf("output want 25 USD/1M, got %v", found.Output.USDPer1M)
	}
	if found.Reasoning.USDPer1M != 25 {
		t.Errorf("reasoning want 25 USD/1M, got %v", found.Reasoning.USDPer1M)
	}
	// The internal unit is micro-USD per 1M, and it is the one the ledger uses.
	if found.Input.MicrosPer1M != 5_000_000 {
		t.Errorf("input want 5000000 micros/1M, got %d", found.Input.MicrosPer1M)
	}
}

// TestAgentCatalogMarksEveryPriceAsEstimate is US-AD108 AC1. The label comes
// from the server so a client cannot quietly drop it, and it is repeated per
// entry because a client that renders a single row must still carry it.
func TestAgentCatalogMarksEveryPriceAsEstimate(t *testing.T) {
	f := newUpdateFixture(t)
	_, got := f.getCatalog(t, "vera", f.scenario.orgA)

	if !got.Estimate {
		t.Error("top-level estimate flag is false")
	}
	if got.Disclaimer == "" {
		t.Error("top-level disclaimer is empty")
	}
	if got.PriceVersion != pricing.PriceVersion {
		t.Errorf("price_version want %d, got %d", pricing.PriceVersion, got.PriceVersion)
	}
	for _, m := range got.Models {
		if !m.Estimate || m.Disclaimer == "" {
			t.Fatalf("model %q is not labelled an estimate", m.Model)
		}
	}
}

// TestAgentCatalogNeverInventsAPrice is US-AD108 AC3. A model the table does
// not price must be reported unpriced with zero rates — the UI renders "harga
// tidak diketahui" rather than "$0.00", and the difference is the whole point.
// Nothing is added to the catalog to cover an unknown name.
func TestAgentCatalogNeverInventsAPrice(t *testing.T) {
	f := newUpdateFixture(t)
	_, got := f.getCatalog(t, "vera", f.scenario.orgA)

	const unknown = "totally-unknown-model-xyz"
	for _, m := range got.Models {
		if m.Model == unknown {
			t.Fatal("the catalog invented an entry for a model that is not in the price table")
		}
	}

	// The resolution path for that name is unpriced with zero rates, which is
	// what the UI must be able to distinguish from a free model.
	res := pricing.Resolve(unknown, nil)
	if res.Source != pricing.SourceUnpriced {
		t.Fatalf("unknown model resolved as %q, want unpriced", res.Source)
	}
	if res.Price.InputMicrosPer1M != 0 || res.Price.OutputMicrosPer1M != 0 {
		t.Errorf("unpriced model carries rates: %+v", res.Price)
	}
}

// ---- known gap: the list response cannot report archived_at yet -------------

// TestListAgentsReportsArchivedAt is the US-AD73 AC1/AC2 contract on the read
// path.
//
// The criterion needs an archived agent to disappear from the assign dropdown,
// and the client can only do that if the list it reads says which agents are
// archived. This test used to pin the opposite — a gap, where ListAgents'
// statement selected the pre-0008 column set and could not report `archived_at`
// at all. The statement is now widened, so the gap is closed and this asserts
// the behaviour that replaced it.
//
// The archived row must still be LISTED. Archiving is reversible, and the
// registry is where a user finds the agent again to unarchive it; filtering it
// out of the list would strand it. What must not happen is the row reading as
// assignable — that is what `archived_at` on the wire lets the UI enforce.
func TestListAgentsReportsArchivedAt(t *testing.T) {
	f := newUpdateFixture(t)
	f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-active","provider":"openai","model":"gpt-4o"}`)
	retired := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-retired","provider":"openai","model":"gpt-4o"}`)
	if w := f.patchAgent(t, "alice", f.scenario.orgA, retired.ID, `{"archived":true}`); w.Code != http.StatusOK {
		t.Fatalf("setup archive: want 200, got %d — %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+f.projectID+"/agents", nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens["alice@x.test"])
	req.Header.Set("X-Org-ID", f.scenario.orgA)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list agents want 200, got %d — %s", w.Code, w.Body.String())
	}

	// Raw JSON, not agentResponse: the point is what the wire carries, and a
	// struct field would report the zero value as though it were a real absence.
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode list %q: %v", w.Body.String(), err)
	}
	if len(rows) != 2 {
		t.Fatalf("want both agents listed (archiving keeps the row), got %d", len(rows))
	}

	seen := map[string]bool{}
	for _, row := range rows {
		name, _ := row["name"].(string)
		_, hasArchived := row["archived_at"]
		seen[name] = hasArchived
	}
	if !seen["agent-retired"] {
		t.Fatal("agent-retired is listed without archived_at: the UI cannot tell it " +
			"apart from a live agent, so US-AD73 AC2 (excluded from assign) is unenforceable")
	}
	if seen["agent-active"] {
		t.Fatal("agent-active carries archived_at: `omitempty` must drop it for a live " +
			"agent, otherwise the UI sees every row as retired")
	}
}

// TestAgentCatalogRequiresViewer pins the floor from ARCHITECTURE 6.2.7: reading
// the catalog is Viewer, so every role gets it and an anonymous caller does not.
func TestAgentCatalogRequiresViewer(t *testing.T) {
	f := newUpdateFixture(t)

	for _, actor := range []string{"vera", "marta", "andre", "alice"} {
		if w, _ := f.getCatalog(t, actor, f.scenario.orgA); w.Code != http.StatusOK {
			t.Errorf("%s reading the catalog: want 200, got %d", actor, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-catalog", nil)
	req.Header.Set("X-Org-ID", f.scenario.orgA)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous catalog read want 401, got %d", w.Code)
	}
}
