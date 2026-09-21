package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
	"agentdeck/internal/pricing"
	"agentdeck/internal/providerreg"
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
	// BaseURL is the BYO endpoint (DECISIONS 6A.F). A create may set it, which
	// is what US-AD106 AC1 describes ("mendaftarkan agent dengan provider =
	// 'openai_compatible' dan base_url yang valid berhasil").
	BaseURL string `json:"base_url"`
	// ProviderID points at a workspace provider (US-AD109). Omitted means the
	// agent keeps whatever it had, which on create is none: an agent with no
	// provider of its own is a valid, permanent state, not a missing backfill.
	ProviderID string `json:"provider_id"`
}

// agentUpdateRequest is the PATCH body (US-AD96, US-AD106, US-AD73). It carries
// the create field set plus `archived`.
//
// `archived` is not a column: it is the request's spelling of
// `agents.archived_at = now()` / NULL, which ARCHITECTURE 6.2.7 puts on this
// endpoint rather than behind a route of its own.
type agentUpdateRequest struct {
	agentRequest
	Archived *bool `json:"archived"`
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
	// BaseURL is present only for a BYO provider (openai_compatible). It is
	// omitted rather than sent as "" so the client cannot mistake an absent
	// endpoint for an empty one.
	BaseURL *string `json:"base_url,omitempty"`
	// ArchivedAt is US-AD73: set means retired. Every read path selects it, so
	// the registry can tell a live agent from a retired one instead of printing
	// a constant zero for the archive count.
	ArchivedAt *string `json:"archived_at,omitempty"`
	// HasProviderKey is the generated column agents.has_provider_key: true once
	// a credential is stored (DECISIONS 6A.I). It reports presence, never the
	// key — the ciphertext never enters board.Agent, so it cannot leak here.
	HasProviderKey bool `json:"has_provider_key"`
	// ProviderID is the workspace provider this agent draws its endpoint and
	// credential from (US-AD109). Omitted when the agent has none.
	ProviderID *string `json:"provider_id,omitempty"`
	CreatedAt  string  `json:"created_at"`
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
	resp := agentResponse{
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
		HasProviderKey:    a.HasProviderKey,
		CreatedAt:         a.CreatedAt.Format(time.RFC3339Nano),
	}
	if a.BaseURL != "" {
		baseURL := a.BaseURL
		resp.BaseURL = &baseURL
	}
	if a.ProviderID != "" {
		providerID := a.ProviderID
		resp.ProviderID = &providerID
	}
	if a.ArchivedAt != nil {
		archivedAt := a.ArchivedAt.Format(time.RFC3339Nano)
		resp.ArchivedAt = &archivedAt
	}
	return resp
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
		BaseURL:           strings.TrimSpace(req.BaseURL),
		ProviderID:        strings.TrimSpace(req.ProviderID),
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
	// The provider is resolved before the write: it is what decides `provider`
	// and `base_url` (US-AD109 AC6), and the model is checked against the list
	// that provider offers (AC10).
	if err := a.applyProvider(r.Context(), orgCtx.workspace.ID, &agent); err != nil {
		writeBoardError(w, err)
		return
	}
	models, err := a.providerModels(r.Context(), orgCtx.workspace.ID, agent)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	if err := validateModelAgainstProvider(agent.Model, models); err != nil {
		writeBoardError(w, err)
		return
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

// registerAgentRoutes mounts the agent-domain routes that are not already
// declared in registerBoardRoutes. It exists as its own function for the same
// reason registerBoardRoutes does: the role gate is declared once, next to the
// handler it guards, and the RBAC test drives this function rather than a copy.
//
//	GET    /api/v1/agents/{id}   Viewer   (registered in registerBoardRoutes)
//	PATCH  /api/v1/agents/{id}   Member   (here; archive raises it to Admin)
//	GET    /api/v1/agent-catalog Viewer   (here)
func registerAgentRoutes(mux *http.ServeMux, api authAPI, svc *board.Service, providers *providerreg.Service) {
	boardAPI := boardAPI{svc: svc, providers: providers}
	agentRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}
	// ARCHITECTURE 6.2.7 lists Member for PATCH /agents/{id}, and US-AD96 is a
	// Member-level edit. US-AD73 AC4 raises *archiving* to owner/admin, which is
	// enforced inside the handler because it is the same route: one endpoint,
	// two floors. Splitting it into a separate archive route would contradict
	// 6.2.7, and gating the whole PATCH at Admin would break the Member edit
	// US-AD96 asks for.
	agentRoute("PATCH /api/v1/agents/{id}", http.HandlerFunc(boardAPI.updateAgent), auth.Member)
	agentRoute("GET /api/v1/agent-catalog", http.HandlerFunc(boardAPI.agentCatalog), auth.Viewer)
}

// PATCH /api/v1/agents/{id} — full update plus archive/unarchive (US-AD96,
// US-AD106, US-AD73).
//
// Two role floors on one route. The gate already admitted Member+, so the only
// thing left to check here is the archive floor: US-AD73 AC4 says retiring an
// agent needs owner/admin. A member may rename or re-model an agent (US-AD96),
// but may not take it out of service — the two are different authorities.
func (a boardAPI) updateAgent(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.boardContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req agentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")

	// The archive branch is answered before the field update so a caller asking
	// to archive gets exactly one effect. A payload that both archives and
	// edits would otherwise apply the edit to a row it is simultaneously
	// retiring, and a 409 from the running-task guard would leave the caller
	// unsure whether the edit landed.
	if req.Archived != nil {
		// Both directions need owner/admin: US-AD73 AC4 is about the authority
		// over an agent's availability, and putting one back into service is
		// the same decision as taking it out. Gating only the archive direction
		// would let a member undo an admin's retirement.
		if orgCtx.role != auth.Owner && orgCtx.role != auth.Admin {
			http.Error(w, board.ErrArchiveRequiresAdmin.Error(), http.StatusForbidden)
			return
		}
		var updated board.Agent
		if *req.Archived {
			updated, err = a.svc.ArchiveAgent(r.Context(), id, orgCtx.workspace.ID)
		} else {
			updated, err = a.svc.UnarchiveAgent(r.Context(), id, orgCtx.workspace.ID)
		}
		if err != nil {
			writeBoardError(w, err)
			return
		}
		writeAgent(w, updated)
		return
	}

	// A full update needs the whole current row first: the sqlc statement writes
	// every mutable column, so an omitted field must land on the stored value,
	// not on a Go zero. Reading first also produces the 404 for an id outside
	// the caller's org, which is the tenant boundary.
	current, err := a.svc.GetAgent(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	merged := mergeAgent(current, req)
	// Same resolution as create: the provider decides `provider` and `base_url`
	// (US-AD109 AC6), and the model must be one that provider offers (AC10).
	if err := a.applyProvider(r.Context(), orgCtx.workspace.ID, &merged); err != nil {
		writeBoardError(w, err)
		return
	}
	models, err := a.providerModels(r.Context(), orgCtx.workspace.ID, merged)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	if err := validateModelAgainstProvider(merged.Model, models); err != nil {
		writeBoardError(w, err)
		return
	}
	updated, err := a.svc.UpdateAgent(r.Context(), merged)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeAgent(w, updated)
}

// mergeAgent overlays the request onto the stored row. Absent fields keep their
// stored value, which is what makes a PATCH-shaped body safe on an endpoint
// whose SQL is a full UPDATE.
//
// base_url follows the same rule as every other field, which it could not do
// until GetAgent's statement selected it: an omitted base_url now preserves the
// stored endpoint instead of clearing it. Clearing is expressed by switching the
// provider away from openai_compatible, which is the only state where a NULL
// base_url is legal anyway (agents_base_url_chk), so there is no case that needs
// an explicit "clear" spelling.
func mergeAgent(current board.Agent, req agentUpdateRequest) board.Agent {
	if req.Name != "" {
		current.Name = req.Name
	}
	if req.Provider != "" {
		current.Provider = req.Provider
	}
	if req.Model != "" {
		current.Model = req.Model
	}
	if req.ReasoningEffort != "" {
		current.ReasoningEffort = req.ReasoningEffort
	}
	if req.Skills != nil {
		if raw, err := json.Marshal(req.Skills); err == nil {
			current.SkillsJSON = raw
		}
	}
	if req.Tools != nil {
		if raw, err := json.Marshal(req.Tools); err == nil {
			current.ToolsJSON = raw
		}
	}
	if req.MaxRuntimeSeconds != nil {
		current.MaxRuntimeSeconds = *req.MaxRuntimeSeconds
	}
	if req.RetryPolicy != "" {
		current.RetryPolicy = req.RetryPolicy
	}
	if req.MaxAttempts != nil {
		current.MaxAttempts = *req.MaxAttempts
	}
	// provider_id is overlaid like every other field, so a PATCH that only
	// renames an agent keeps its provider. An omitted id is not a clear: the
	// registry reference is what carries the endpoint and the credential, and
	// silently dropping it on an unrelated edit would repoint the agent at the
	// deployment default (US-AD109 AC6).
	if req.ProviderID != "" {
		current.ProviderID = strings.TrimSpace(req.ProviderID)
	}
	// provider and base_url move together (agents_base_url_chk): switching to a
	// built-in provider clears the endpoint, and switching to openai_compatible
	// requires one. An omitted base_url is not a clear — it keeps the stored
	// endpoint, so a PATCH that only renames an agent no longer wipes its
	// provider URL.
	switch {
	case req.BaseURL != "":
		// An explicitly sent endpoint wins whatever the provider says. If the
		// two disagree, validateAgent answers 400 — dropping a host the
		// operator typed would silently send traffic somewhere else, which is
		// the exact failure agents_base_url_chk exists to prevent.
		current.BaseURL = strings.TrimSpace(req.BaseURL)
	case req.Provider != "" && req.Provider != board.ProviderOpenAICompatible:
		current.BaseURL = ""
	}
	return current
}

func writeAgent(w http.ResponseWriter, agent board.Agent) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAgentResponse(agent))
}

// applyProvider resolves the agent's provider_id into the three stored fields
// that describe where it runs (US-AD109 AC6, DECISIONS 6A.J).
//
// This is the hinge of phase 5. `agents.provider` used to be typed by the
// operator; it is now *derived* from `providers.protocol`, because the provider
// is what owns the endpoint and the credential. Letting the request also set
// them would mean two writers for one fact, and the row could describe an
// endpoint the credential does not belong to.
//
// An empty providerID is not an error: it clears the reference and leaves the
// agent on the deployment's environment default, which is the permanent state of
// every built-in agent the backfill deliberately skipped (DECISIONS 6A.J).
//
// The lookup carries the org id, so a provider id from another workspace is
// indistinguishable from an absent one — the same tenant boundary every other
// query in this file holds.
func (a boardAPI) applyProvider(ctx context.Context, orgID string, agent *board.Agent) error {
	if agent.ProviderID == "" {
		// Clearing keeps `provider` as-is: an agent that never had a provider
		// must not be rewritten to openai_compatible just because the field
		// went away, and base_url follows the stored provider rather than the
		// request.
		return nil
	}
	if a.providers == nil {
		// A deployment that mounted the agent routes without the registry (the
		// RBAC tests do exactly this) has nothing to resolve against. Answering
		// "unknown provider" beats a nil dereference.
		return board.ErrUnknownProvider
	}
	provider, err := a.providers.Get(ctx, orgID, agent.ProviderID)
	if err != nil {
		if errors.Is(err, providerreg.ErrProviderNotFound) {
			// US-AD07: a foreign provider id is not found, not forbidden.
			return board.ErrUnknownProvider
		}
		return err
	}
	agent.Provider = string(provider.Protocol)
	agent.BaseURL = provider.BaseURL
	return nil
}

// providerModels returns the model list the agent's provider offers, or nil when
// the agent has no provider.
//
// Nil is a distinct answer from an empty list: "this provider has fetched
// nothing yet" must not be enforced as "no model is allowed", because phase 3
// fetches lazily and a provider created minutes ago legitimately has none.
func (a boardAPI) providerModels(ctx context.Context, orgID string, agent board.Agent) ([]string, error) {
	if agent.ProviderID == "" || a.providers == nil {
		return nil, nil
	}
	provider, err := a.providers.Get(ctx, orgID, agent.ProviderID)
	if err != nil {
		if errors.Is(err, providerreg.ErrProviderNotFound) {
			return nil, board.ErrUnknownProvider
		}
		return nil, err
	}
	if len(provider.Models) == 0 {
		return nil, nil
	}
	return provider.Models, nil
}

// validateModelAgainstProvider implements US-AD109 AC10's other half: a model
// the selected provider does not offer is refused, because the provider is now
// the thing that knows which models exist.
//
// `models` is nil when nothing was fetched yet, and that is deliberately *not* a
// rejection: the allowlist is only authoritative once it exists.
//
// The models come from the provider's fetched list, which the operator controls
// by editing their own upstream — this is a usability check against a stale or
// wrong selection, not a security boundary. It cannot be one: the list is
// whatever the operator's endpoint answered with.
func validateModelAgainstProvider(model string, models []string) error {
	if models == nil {
		return nil
	}
	for _, known := range models {
		if known == model {
			return nil
		}
	}
	return fmt.Errorf("%w: model %q is not offered by this provider", board.ErrUnknownModel, model)
}

// ---- agent catalog (US-AD96, US-AD108) --------------------------------------

// catalogModel is one model's estimate. Rates are micro-USD per 1,000,000
// tokens — the same unit the ledger stores — so the UI never re-derives a price
// from a rounded float. The field names are the ARCHITECTURE 9.1 ones; `usd`
// is added because a dropdown that only has micros makes the operator do the
// division in their head.
type catalogModel struct {
	Model string `json:"model"`
	// Source is the resolution tier that produced these rates: manual, catalog,
	// pattern, or unpriced (DECISIONS 6A.C). `unpriced` means the model is not
	// in the table at all and every rate below is zero — the UI must render
	// "harga tidak diketahui" rather than "$0.00", because zero here is the
	// absence of a price, not a free model.
	Source       string      `json:"price_source"`
	PricingModel string      `json:"pricing_model"`
	Input        catalogRate `json:"input"`
	Output       catalogRate `json:"output"`
	Cached       catalogRate `json:"cached"`
	Reasoning    catalogRate `json:"reasoning"`
	CacheWrite   catalogRate `json:"cache_creation"`
	PriceVersion int         `json:"price_version"`
	Estimate     bool        `json:"estimate"`
	Disclaimer   string      `json:"disclaimer"`
}

// catalogRate carries one rate in both units so a client can use either without
// a second request and without a lossy conversion of its own.
type catalogRate struct {
	MicrosPer1M int64   `json:"micros_per_1m"`
	USDPer1M    float64 `json:"usd_per_1m"`
}

// The disclaimer is a contract string, not copy: US-AD108 AC1 requires every
// cost figure to be labelled an estimate, and the label has to come from the
// server so a client cannot quietly drop it. It is short enough to render
// inline under a price.
const catalogDisclaimer = "Angka ini estimasi dari tabel harga internal AgentDeck, bukan tagihan. Biaya sebenarnya ada di dashboard provider masing-masing."

// GET /api/v1/agent-catalog — the model list plus its estimated rates.
//
// The list is exactly the pricing table's surface: every exact entry, every
// pattern, and a probe of the unpriced tier. Nothing is invented for a model the
// table does not know — an unknown name is reported with price_source
// "unpriced" and zero rates, which is US-AD108 AC3.
func (a boardAPI) agentCatalog(w http.ResponseWriter, r *http.Request) {
	out := make([]catalogModel, 0, len(pricing.Catalog())+len(pricing.Patterns()))
	for _, m := range pricing.Catalog() {
		out = append(out, catalogEntry(m, pricing.Resolve(m, nil)))
	}
	for _, p := range pricing.Patterns() {
		// A pattern is not a model: it is the rule that prices models absent
		// from the exact table. It is listed with its own name and the pattern
		// tier, so the UI can show it as a catch-all rather than as a
		// selectable model.
		out = append(out, catalogEntry(p.Pattern, pricing.Resolution{
			Price:        p.Price,
			Source:       pricing.SourcePattern,
			PricingModel: p.Pattern,
		}))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"estimate":      true,
		"disclaimer":    catalogDisclaimer,
		"price_version": pricing.PriceVersion,
		"models":        out,
	})
}

func catalogEntry(name string, res pricing.Resolution) catalogModel {
	p := res.Price
	return catalogModel{
		Model:        name,
		Source:       string(res.Source),
		PricingModel: res.PricingModel,
		Input:        rate(p.InputMicrosPer1M),
		Output:       rate(p.OutputMicrosPer1M),
		Cached:       rate(p.CachedMicrosPer1M),
		Reasoning:    rate(p.ReasoningMicrosPer1M),
		CacheWrite:   rate(p.CacheWriteMicrosPer1M),
		PriceVersion: p.PriceVersion,
		Estimate:     true,
		Disclaimer:   catalogDisclaimer,
	}
}

// rate converts the stored micro-USD per 1M into both units the response
// carries. It is the only float in the path and it is display-only: every cost
// computation stays in integer micro-USD (DECISIONS 6A.D).
func rate(microsPer1M int64) catalogRate {
	return catalogRate{
		MicrosPer1M: microsPer1M,
		USDPer1M:    float64(microsPer1M) / 1_000_000,
	}
}
