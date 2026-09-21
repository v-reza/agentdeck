package main

// US-AD86 — per-agent LLM provider credentials. Three endpoints, per
// ARCHITECTURE 6.2.7:
//
//	POST   /api/v1/agents/{id}/validate        Member  — handshake against the provider
//	PUT    /api/v1/agents/{id}/provider-key    Admin   — store / rotate the key
//	DELETE /api/v1/agents/{id}/provider-key    Admin   — revoke it
//
// The credential is write-only. It is sealed with AES-256-GCM (internal/crypto)
// before it touches the database, and no path in this file puts it in a response
// body, a log line, or an error string — `redact` in internal/provider covers the
// one place it travels through net/http.
//
// Rotation and revocation are the same statement with a different value
// (US-AD86 AC4), so there is deliberately no history table: the old ciphertext is
// overwritten, not versioned.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"agentdeck/internal/auth"
	"agentdeck/internal/board"
	"agentdeck/internal/crypto"
	"agentdeck/internal/provider"
	"agentdeck/internal/providerreg"
)

// providerKeyRequest is the PUT body. `api_key` is the only required field.
//
// The optional provider/model/base_url exist for US-AD96 AC5's "test before
// save" flow and for rotating a key on an agent whose endpoint the caller is
// re-stating in the same request. They are applied only when the agent does not
// carry one yet.
type providerKeyRequest struct {
	APIKey   string `json:"api_key"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
}

// providerKeyResponse reports presence, never value. `masked_key` is a shape the
// operator can recognise (US-AD96 AC2) and nothing more: it is derived from the
// plaintext in this one request and immediately discarded, so a later GET cannot
// reproduce it.
type providerKeyResponse struct {
	ID             string `json:"id"`
	HasProviderKey bool   `json:"has_provider_key"`
	MaskedKey      string `json:"masked_key,omitempty"`
}

// validateResponse is the handshake result. `ok` is the machine-readable half and
// `detail` the human one. A refused key is answered 200 with ok=false: "the
// provider said 401" is a successful test with a negative result, not a failure
// of this API. A 4xx/5xx from here means the test could not be run at all.
type validateResponse struct {
	OK     bool     `json:"ok"`
	Detail string   `json:"detail"`
	Models []string `json:"models,omitempty"`
}

// credentialAPI holds what the credential endpoints need beyond the board
// service: the master key, and the HTTP client used for the handshake.
//
// The client is a field rather than a package-level default so a test can point
// the handshake at an httptest server. provider.NewClient() is the production
// value and refuses redirects to private addresses (US-AD106 AC4).
type credentialAPI struct {
	svc    *board.Service
	keyRaw string
	client *http.Client
	// providers resolves the agent's endpoint. Until phase 6 the handshake read
	// `agents.base_url`; that column is gone, and the address belongs to the
	// provider (US-AD109 AC6), so this is the only place left that can answer
	// "where do I send this request".
	providers *providerreg.Service
}

// registerAgentCredentialRoutes mounts the three US-AD86 routes.
//
// It is separate from registerAgentRoutes because these routes hang off a
// different authority: the registry is Member-writable, the credential is
// owner/admin. One function, one role table, so the floor is declared exactly
// once — the same shape registerAgentRoutes uses, and the reason the RBAC tests
// drive the production mux instead of a copy of it.
func registerAgentCredentialRoutes(mux *http.ServeMux, api authAPI, svc *board.Service, providers *providerreg.Service) {
	credAPI := credentialAPI{svc: svc, keyRaw: api.masterKey, client: provider.NewClient(), providers: providers}
	agentRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}
	// US-AD86 AC2: reading or writing a credential is owner/admin. `member` and
	// `viewer` are refused at the gate, before any handler runs.
	agentRoute("PUT /api/v1/agents/{id}/provider-key", http.HandlerFunc(credAPI.putProviderKey), auth.Admin)
	agentRoute("DELETE /api/v1/agents/{id}/provider-key", http.HandlerFunc(credAPI.deleteProviderKey), auth.Admin)
	// ARCHITECTURE 6.2.7 puts the handshake at Member: it is a diagnostic, not a
	// credential write, and it never returns the key.
	agentRoute("POST /api/v1/agents/{id}/validate", http.HandlerFunc(credAPI.validateProvider), auth.Member)
	// The stateless probe takes a raw credential, so it carries the credential
	// floor (Admin), not the diagnostic one. It is deliberately not mounted under
	// /agents/{id}: the caller has no id yet — that is the whole reason it exists.
	agentRoute("POST /api/v1/provider/models", http.HandlerFunc(credAPI.providerModels), auth.Admin)
}

// masterKey decodes AGENTDECK_MASTER_KEY. An unusable key is a 500 that stores
// nothing: falling back to plaintext would be a silent downgrade of every
// credential this deployment holds, which is worse than the feature being down.
func (a credentialAPI) masterKey(w http.ResponseWriter) ([]byte, bool) {
	key, err := crypto.LoadKey(a.keyRaw)
	if err != nil {
		http.Error(w, board.ErrCredentialKeyUnavailable.Error(), http.StatusInternalServerError)
		return nil, false
	}
	return key, true
}

// PUT /api/v1/agents/{id}/provider-key — store or rotate (US-AD86 AC1/AC2/AC4).
func (a credentialAPI) putProviderKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")

	var req providerKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		http.Error(w, "api_key is required", http.StatusBadRequest)
		return
	}

	// The stored row is read first: the provider is what AC3's 400 is about, and
	// the request may only override it with a provider the table knows.
	agent, err := a.svc.GetAgent(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	if err := board.ValidateProvider(agent.Provider); err != nil {
		writeBoardError(w, err)
		return
	}

	key, ok := a.masterKey(w)
	if !ok {
		return
	}
	sealed, err := crypto.Seal(key, apiKey)
	if err != nil {
		// Seal's error text carries no key material, but it is an internal
		// failure rather than the caller's mistake.
		http.Error(w, "could not encrypt the credential", http.StatusInternalServerError)
		return
	}

	hasKey, err := a.svc.SetProviderKey(r.Context(), id, orgCtx.workspace.ID, sealed)
	if err != nil {
		writeBoardError(w, err)
		return
	}

	// The masked shape is computed here, from the plaintext in this request, and
	// never persisted. A GET cannot reproduce it because nothing stores it.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providerKeyResponse{
		ID:             id,
		HasProviderKey: hasKey,
		MaskedKey:      maskCredential(apiKey),
	})
}

// DELETE /api/v1/agents/{id}/provider-key — revoke (US-AD86 AC2).
//
// The agent is not deleted, and having no credential is not an error: the
// operation is idempotent and its postcondition ("this agent has no stored
// credential") holds either way.
func (a credentialAPI) deleteProviderKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")

	hasKey, err := a.svc.ClearProviderKey(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providerKeyResponse{ID: id, HasProviderKey: hasKey})
}

// agentEndpoint resolves where an agent's inference goes, from the provider it
// points at (US-AD109 AC6).
//
// Three answers, and they are distinct on purpose:
//   - an agent with no provider has no address this deployment knows: the
//     deployment's environment default is the runtime's business, not a URL
//     this endpoint may invent, so it is ErrProviderNotProbeable.
//   - an agent whose provider row is gone is the same situation as a foreign
//     provider id (US-AD07): ErrUnknownProvider, not a nil dereference.
//   - a provider with an empty base_url cannot exist (providers_base_url_chk),
//     so a blank one here is a stored row that violates its own constraint.
func (a credentialAPI) agentEndpoint(ctx context.Context, orgID string, agent board.Agent) (string, error) {
	if agent.ProviderID == "" || a.providers == nil {
		return "", board.ErrProviderNotProbeable
	}
	provider, err := a.providers.Get(ctx, orgID, agent.ProviderID)
	if err != nil {
		if errors.Is(err, providerreg.ErrProviderNotFound) {
			return "", board.ErrUnknownProvider
		}
		return "", err
	}
	return provider.BaseURL, nil
}

// POST /api/v1/agents/{id}/validate — handshake with the provider.
//
// US-AD86 is silent on this endpoint's body and ARCHITECTURE 6.2.7 only says
// "test ping", so the contract is deliberately narrow: the agent's own stored
// credential is used, and no URL is accepted from the caller. A caller-supplied
// base_url would be an SSRF vector dressed as a test; the stored one is already
// guarded by US-AD106 AC3.
//
// The endpoint comes from the agent's provider (US-AD109 AC6). An agent with no
// provider of its own has no address in this deployment, so there is nothing to
// ping: answering 200 with ok=true would be a fabricated success, and a 400 says
// exactly that.
func (a credentialAPI) validateProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")

	agent, err := a.svc.GetAgent(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	baseURL, err := a.agentEndpoint(r.Context(), orgCtx.workspace.ID, agent)
	if err != nil {
		writeBoardError(w, err)
		return
	}

	key, ok := a.masterKey(w)
	if !ok {
		return
	}
	sealed, err := a.svc.ProviderKey(r.Context(), id, orgCtx.workspace.ID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	apiKey, err := crypto.Open(key, sealed)
	if err != nil {
		// A credential we cannot open is a server-side fault: the master key
		// changed, or the column was tampered with. GCM authentication fails
		// closed, so there is no plaintext to fall back to.
		http.Error(w, "stored credential could not be decrypted", http.StatusInternalServerError)
		return
	}

	models, err := provider.ListModels(r.Context(), a.client, baseURL, apiKey)
	if err != nil {
		// The upstream refused or was unreachable. internal/provider has already
		// redacted the key from the message; report ok=false, because a wrong key
		// is the expected outcome of pressing this button.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(validateResponse{
			OK:     false,
			Detail: "provider rejected the handshake: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(validateResponse{
		OK:     true,
		Detail: "provider accepted the credential",
		Models: models,
	})
}

// maskCredential is the `sk-...XXXX` shape US-AD96 AC2 asks for. It keeps the
// first four characters (the provider's own prefix, which is not secret) and the
// last four, never the middle. A short key is masked entirely rather than
// half-revealed.
func maskCredential(key string) string {
	const edge = 4
	if len(key) <= edge*2 {
		return strings.Repeat("*", len(key))
	}
	return key[:edge] + "..." + key[len(key)-edge:]
}
