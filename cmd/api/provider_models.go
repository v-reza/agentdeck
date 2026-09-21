package main

// POST /api/v1/provider/models — the stateless probe the agent form's model
// dropdown needs.
//
// It exists because neither credential endpoint can be used before the agent
// does: PUT /agents/{id}/provider-key needs an id, and POST /agents/{id}/validate
// reads the credential back out of the database. The form has a provider and a
// pasted key and nothing else, so the request carries both and this endpoint
// stores nothing — no agent row, no sealed key, no cache. The credential lives
// for the length of one request.
//
// The role floor is Admin, the same as the credential writes: this endpoint
// accepts a raw key. See registerAgentCredentialRoutes.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"agentdeck/internal/board"
	"agentdeck/internal/provider"
	"agentdeck/internal/providerreg"
)

// probeBodyMaxBytes bounds the request body. Both fields are short (a base URL
// capped at provider.MaxBaseURLLength and a key), so anything larger is a
// mistake or a probe for an unbounded read.
const probeBodyMaxBytes = 64 << 10

// providerModelsRequest is the probe body: a base URL and a raw credential, both
// supplied by the caller because the agent they describe does not exist yet.
type providerModelsRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// providerModelsResponse is the model list and nothing else. No echo of the base
// URL, no masked key: the caller already knows both, and every extra field is
// one more place a credential could leak.
type providerModelsResponse struct {
	Models []string `json:"models"`
}

// POST /api/v1/provider/models — list the models an upstream advertises.
//
// Auth and the Admin floor are already applied by the middleware chain
// (registerAgentCredentialRoutes); there is no tenant to resolve because nothing
// is read or written per org.
func (a credentialAPI) providerModels(w http.ResponseWriter, r *http.Request) {
	var req providerModelsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, probeBodyMaxBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.BaseURL == "" || req.APIKey == "" {
		http.Error(w, "base_url and api_key are required", http.StatusBadRequest)
		return
	}

	// The guard runs here as well as inside ListModels, which is the only reason
	// the two failures below can be told apart at all: an address the guard
	// refuses is the caller's mistake (400), an upstream that refuses us is not
	// (502). Reimplementing the rule here instead would be the bug this avoids —
	// two tables that can disagree about what is reachable. The cost is one extra
	// DNS lookup, on a request that then makes an HTTP call anyway.
	if _, err := provider.ValidateOperatorBaseURL(r.Context(), req.BaseURL); err != nil {
		writeBoardError(w, fmt.Errorf("%w: %s", providerreg.ErrInvalidInput, err))
		return
	}

	// ListModels validates again on the way in and redacts the key from whatever
	// it returns, so this error text is safe to hand back.
	models, err := provider.ListModels(r.Context(), a.client, req.BaseURL, req.APIKey)
	if err != nil {
		writeBoardError(w, fmt.Errorf("%w: %s", board.ErrProviderHandshakeFailed, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providerModelsResponse{Models: models})
}
