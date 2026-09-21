package main

// Provider registry HTTP surface (US-AD109, ARCHITECTURE 6.2.8, DECISIONS 6A.J).
//
//	GET    /api/v1/providers          Viewer
//	POST   /api/v1/providers          Admin
//	GET    /api/v1/providers/{id}     Viewer
//	PATCH  /api/v1/providers/{id}     Admin
//	DELETE /api/v1/providers/{id}     Admin
//	POST   /api/v1/providers/{id}/verify   Admin   (AC3)
//	POST   /api/v1/providers/{id}/models   Admin   (AC7)
//
// The last two reach the operator's upstream endpoint. They are the only routes
// in the repo that do: the runtime has never called an LLM, so the probe built
// here (internal/provider.ProbeInference) is the first inference call in the
// codebase. It asks for one token, because authenticating is the whole point
// and the cheapest call that authenticates is the right one (AC3).
//
// The credential is write-only. It is sealed with AES-256-GCM on the way in
// (internal/crypto) and no endpoint here ever returns it: the reads answer
// `has_key`, and the masked form appears only in the response to the one write
// that supplied it — the same rule US-AD86 applies to an agent's credential
// (AC2, "mengikuti aturan US-AD96 AC2/AC4").
//
// Every mutating route is Admin. A provider's key is shared by every agent in
// the workspace, so editing it is a security action, not a member-level edit.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/crypto"
	"agentdeck/internal/providerreg"
)

// providerBodyMaxBytes bounds the request body. A name, a protocol, a base URL
// capped at provider.MaxBaseURLLength, and a key all fit well inside this.
const providerBodyMaxBytes = 64 << 10

// providerRequest is the create body. APIKey is a plaintext credential that
// exists only for the length of the request: it is sealed before it reaches the
// database and is never stored, logged, or echoed back.
type providerRequest struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	BaseURL  string `json:"base_url"`
	// APIKey is optional. A local endpoint that checks nothing (Ollama) has no
	// credential, and the DDL comment names that case (api_key_enc NULL).
	APIKey string `json:"api_key"`
	// IsDefault is optional: nil means "the workspace decides", which is true
	// for its first provider (AC9).
	IsDefault *bool `json:"is_default"`
}

// providerPatchRequest is a partial update, so every field is a pointer and
// "absent" stays distinguishable from "set to the zero value". That distinction
// is what makes `{"is_default": false}` clear the default while omitting the
// field leaves it alone.
type providerPatchRequest struct {
	Name      *string `json:"name"`
	Protocol  *string `json:"protocol"`
	BaseURL   *string `json:"base_url"`
	APIKey    *string `json:"api_key"`
	IsDefault *bool   `json:"is_default"`
}

// providerResponse is the read shape. It carries no credential and no
// ciphertext: HasKey reports presence, MaskedKey is set only on the write that
// supplied a key, and neither can be used to recover the secret.
type providerResponse struct {
	ID       string `json:"id"`
	OrgID    string `json:"org_id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	BaseURL  string `json:"base_url"`
	// Models is the last fetched list (AC7). It is empty until phase 3 fetches
	// one, and ModelsFetchedAt stays null to say why — the UI reads null as
	// "never fetched" rather than "fetched, nothing to refresh".
	Models          []string `json:"models"`
	ModelsFetchedAt *string  `json:"models_fetched_at,omitempty"`
	LastVerifiedAt  *string  `json:"last_verified_at,omitempty"`
	IsDefault       bool     `json:"is_default"`
	HasKey          bool     `json:"has_key"`
	// MaskedKey is `sk-...XXXX`, present only in the response to a request that
	// supplied a key. It is never read back from storage.
	MaskedKey string `json:"masked_key,omitempty"`
	CreatedAt string `json:"created_at"`
}

// providerInUseResponse is AC5's 409 body. The contract says the refusal names
// the agents using the provider, so this is an object rather than http.Error's
// bare string.
type providerInUseResponse struct {
	Error string `json:"error"`
	// Agents is capped at providerInUseListLimit: the operator needs to see what
	// to repoint, not to page through a thousand rows in an error message. Total
	// reports the real count so a truncated list is not read as the whole truth.
	Agents []agentRefResponse `json:"agents"`
	Total  int                `json:"total"`
}

// providerInUseListLimit caps the named agents in a 409 body.
const providerInUseListLimit = 20

func toProviderResponse(p providerreg.Provider, maskedKey string) providerResponse {
	out := providerResponse{
		ID:        p.ID,
		OrgID:     p.OrgID,
		Name:      p.Name,
		Protocol:  string(p.Protocol),
		BaseURL:   p.BaseURL,
		Models:    p.Models,
		IsDefault: p.IsDefault,
		HasKey:    p.HasKey,
		MaskedKey: maskedKey,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if out.Models == nil {
		// AC1 says a provider carries a model list; an empty one is a list, not
		// null, so a client can iterate without a nil check.
		out.Models = []string{}
	}
	if p.ModelsFetchedAt != nil {
		s := p.ModelsFetchedAt.UTC().Format(time.RFC3339Nano)
		out.ModelsFetchedAt = &s
	}
	if p.LastVerifiedAt != nil {
		s := p.LastVerifiedAt.UTC().Format(time.RFC3339Nano)
		out.LastVerifiedAt = &s
	}
	return out
}

// writeProviderError maps a domain error to its stable HTTP code so the same
// failure always yields the same response from every path. A foreign provider id
// is ErrProviderNotFound -> 404, consistent with the rest of the repo (US-AD07):
// the tenant boundary must not be observable as a 403.
//
// ErrProviderInUse is handled before this is called, because its 409 body needs
// the agents the sentinel alone does not carry.
func writeProviderError(w http.ResponseWriter, err error) {
	var probeErr *providerreg.ProbeError
	switch {
	case errors.Is(err, providerreg.ErrProviderNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, providerreg.ErrNameTaken):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, providerreg.ErrInvalidInput),
		errors.Is(err, providerreg.ErrNoKeyForProbe),
		errors.Is(err, providerreg.ErrNoModelsToProbe):
		// The caller can act on all three: fix the payload, add a credential,
		// or refresh the model list first. None is a server fault.
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.As(err, &probeErr):
		// The request was well-formed and the credential may be perfectly good;
		// the upstream refused us or could not be reached. That is a bad
		// gateway, not a bad request — the same split POST /provider/models
		// already makes (400 user-error / 502 upstream).
		http.Error(w, err.Error(), http.StatusBadGateway)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// providerAPI wires the registry to HTTP. It rides the same authAPI as the
// auth/org/board routes so tenant resolution and the role gate stay in one
// place.
type providerAPI struct {
	svc *providerreg.Service
	// keyRaw is the raw AGENTDECK_MASTER_KEY. An empty value leaves every write
	// that carries a credential answering 500 rather than storing it in the
	// clear — the same rule the agent credential endpoints follow.
	keyRaw string
}

func (a providerAPI) context(r *http.Request) (orgContext, error) {
	return currentOrgContext(r)
}

// sealCredential encrypts a plaintext key for storage. It is called only when
// the request actually carried one.
func (a providerAPI) sealCredential(plaintext string) ([]byte, error) {
	key, err := crypto.LoadKey(a.keyRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errCredentialKeyUnavailable, err)
	}
	return crypto.Seal(key, plaintext)
}

// errCredentialKeyUnavailable is the 500 for a write that carries a key while no
// master key is configured. It is not a caller mistake, so it is not a 400: the
// deployment is missing AGENTDECK_MASTER_KEY.
var errCredentialKeyUnavailable = errors.New("provider credential encryption is not configured")

// GET /api/v1/providers
func (a providerAPI) listProviders(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	providers, err := a.svc.List(r.Context(), orgCtx.workspace.ID)
	if err != nil {
		writeProviderError(w, err)
		return
	}
	// A non-nil empty slice: an org with no providers gets `[]`, not `null`, so
	// the client can map over it directly.
	out := make([]providerResponse, 0, len(providers))
	for _, p := range providers {
		out = append(out, toProviderResponse(p, ""))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// POST /api/v1/providers
func (a providerAPI) createProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req providerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, providerBodyMaxBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	in := providerreg.CreateInput{
		Name:     req.Name,
		Protocol: req.Protocol,
		BaseURL:  strings.TrimSpace(req.BaseURL),
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey != "" {
		sealed, err := a.sealCredential(apiKey)
		if err != nil {
			writeProviderError(w, err)
			return
		}
		in.SealedKey = sealed
	}
	if req.IsDefault != nil {
		in.IsDefault = req.IsDefault
	}

	created, err := a.svc.Create(r.Context(), orgCtx.workspace.ID, in)
	if err != nil {
		writeProviderError(w, err)
		return
	}

	// The masked key is computed from the plaintext of this request and shown
	// once. It is never derived from storage, because storage only holds the
	// ciphertext.
	masked := ""
	if apiKey != "" {
		masked = maskCredential(apiKey)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toProviderResponse(created, masked))
}

// GET /api/v1/providers/{id}
func (a providerAPI) getProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	p, err := a.svc.Get(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeProviderError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProviderResponse(p, ""))
}

// PATCH /api/v1/providers/{id}
func (a providerAPI) updateProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req providerPatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, providerBodyMaxBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	in := providerreg.UpdateInput{
		Name:      req.Name,
		Protocol:  req.Protocol,
		IsDefault: req.IsDefault,
	}
	if req.BaseURL != nil {
		trimmed := strings.TrimSpace(*req.BaseURL)
		in.BaseURL = &trimmed
	}
	// An explicit empty key is the caller's mistake rather than a request to
	// remove the credential: there is no endpoint in the contract that removes
	// one, so silently treating "" as "leave it" would hide the error.
	masked := ""
	if req.APIKey != nil {
		apiKey := strings.TrimSpace(*req.APIKey)
		if apiKey == "" {
			http.Error(w, "api_key must not be empty", http.StatusBadRequest)
			return
		}
		sealed, err := a.sealCredential(apiKey)
		if err != nil {
			writeProviderError(w, err)
			return
		}
		in.SealedKey = sealed
		masked = maskCredential(apiKey)
	}

	updated, err := a.svc.Update(r.Context(), orgCtx.workspace.ID, r.PathValue("id"), in)
	if err != nil {
		writeProviderError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProviderResponse(updated, masked))
}

// DELETE /api/v1/providers/{id}
//
// AC5's refusal is a 409 whose body names the agents pinning the provider, so it
// is written here rather than through writeProviderError: the sentinel says why,
// only the error value says who.
func (a providerAPI) deleteProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	err = a.svc.Delete(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		var inUse *providerreg.InUseError
		if errors.As(err, &inUse) {
			body := providerInUseResponse{
				Error:  err.Error(),
				Agents: make([]agentRefResponse, 0, len(inUse.Agents)),
				Total:  len(inUse.Agents),
			}
			for i, ref := range inUse.Agents {
				if i == providerInUseListLimit {
					break
				}
				body.Agents = append(body.Agents, agentRefResponse{ID: ref.ID, Name: ref.Name})
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		writeProviderError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/providers/{id}/verify — AC3.
//
// The response is the whole provider, so the caller sees the new
// last_verified_at without a second request. There is no "verified: true" field:
// the timestamp is the evidence, and a boolean beside it would be a second
// source of truth that could disagree.
//
// A refusal here is not a 400. A wrong credential is the upstream saying 401,
// which is a 502 (the payload was fine). What *is* a 400 is a provider with no
// credential or no model list to probe with — both are states the operator can
// fix in the UI, and both are mapped in writeProviderError.
func (a providerAPI) verifyProvider(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	p, err := a.svc.Verify(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeProviderError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProviderResponse(p, ""))
}

// POST /api/v1/providers/{id}/models — AC7's manual refresh.
//
// It does not consult ModelsStale: the whole point of the manual button is to
// fetch now. The 24-hour rule is what a *page load* uses to decide whether to
// offer the refresh, and that decision belongs to the UI reading
// models_fetched_at, not to this handler.
func (a providerAPI) refreshProviderModels(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	p, err := a.svc.RefreshModels(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		writeProviderError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toProviderResponse(p, ""))
}

// registerProviderRoutes mounts the seven registry routes of §6.2.8.
//
// It is its own function for the same reason registerAgentRoutes is: the role
// table is declared exactly once, next to the handlers it guards, and the RBAC
// test drives this function rather than a copy of it.
//
// The two endpoints that are not mounted here — POST /providers/{id}/verify
// (AC3) and POST /providers/{id}/models (AC7) — need a protocol-aware upstream
// call and are scheduled for phase 3. Their §6.2.8 rows stay ⬜, which is what
// keeps the status column honest.
func registerProviderRoutes(mux *http.ServeMux, api authAPI, svc *providerreg.Service) {
	provAPI := providerAPI{svc: svc, keyRaw: api.masterKey}
	providerRoute := func(pattern string, handler http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(handler, minimum)))
	}
	// Reading a provider is Viewer, but reading never yields a credential: the
	// response carries `has_key` and nothing that can be turned back into a
	// secret. AC8 is what fixes the write floor at Admin — the key is shared by
	// every agent in the workspace, so changing it is a security action.
	providerRoute("GET /api/v1/providers", http.HandlerFunc(provAPI.listProviders), auth.Viewer)
	providerRoute("POST /api/v1/providers", http.HandlerFunc(provAPI.createProvider), auth.Admin)
	providerRoute("GET /api/v1/providers/{id}", http.HandlerFunc(provAPI.getProvider), auth.Viewer)
	providerRoute("PATCH /api/v1/providers/{id}", http.HandlerFunc(provAPI.updateProvider), auth.Admin)
	providerRoute("DELETE /api/v1/providers/{id}", http.HandlerFunc(provAPI.deleteProvider), auth.Admin)

	// Phase 3. Both call the operator's upstream, so both are Admin for the same
	// reason the writes are: they spend the workspace's credential and its
	// quota. Neither is idempotent in the strict sense — /models overwrites the
	// cached list and /verify moves a timestamp — but both are safe to repeat,
	// which is what the contract's Idempotent column is about.
	providerRoute("POST /api/v1/providers/{id}/verify", http.HandlerFunc(provAPI.verifyProvider), auth.Admin)
	providerRoute("POST /api/v1/providers/{id}/models", http.HandlerFunc(provAPI.refreshProviderModels), auth.Admin)
}
