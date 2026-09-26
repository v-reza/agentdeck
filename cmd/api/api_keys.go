package main

// HTTP surface untuk 6.2.3 API Keys.
//
// Lima endpoint, semuanya di bawah sesi pengguna — bukan key itu sendiri. Key
// dipakai untuk mengakses endpoint lain (11.1); yang di sini adalah cara
// membuat, melihat, mencabut, dan menghapusnya.
//
// Dua aturan yang menentukan bentuk handler ini:
//
//   - Plaintext `adk_...` hanya ada di response 201. Tidak ada endpoint yang
//     bisa mengembalikannya lagi, jadi `GET /api-keys/{id}` tidak punya field
//     itu — dan tidak boleh, karena menyimpannya berarti menyimpannya.
//   - Semua operasi ter-scope ke pemiliknya. Key milik anggota lain di
//     workspace yang sama menjawab 404, bukan 403: keberadaannya bukan urusan
//     pemanggil.

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentdeck/internal/auth"
)

type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// createAPIKeyResponse adds the one and only sighting of the plaintext token.
type createAPIKeyResponse struct {
	apiKeyResponse
	// Key is the full `adk_...`. Shown once, never retrievable again.
	Key string `json:"key"`
}

func toAPIKeyResponse(k auth.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix,
		LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt, CreatedAt: k.CreatedAt,
	}
}

func writeAPIKeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrAPIKeyNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, auth.ErrAPIKeyNameInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		writeAuthError(w, err)
	}
}

// GET /api/v1/api-keys
func (a authAPI) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	keys, err := a.store.APIKeys(r.Context(), orgCtx.workspace.ID, orgCtx.userID)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	out := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKeyResponse(k))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// POST /api/v1/api-keys
func (a authAPI) createAPIKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	key, token, err := a.store.CreateAPIKey(r.Context(), orgCtx.workspace.ID, orgCtx.userID, req.Name)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(createAPIKeyResponse{
		apiKeyResponse: toAPIKeyResponse(key),
		Key:            token,
	})
}

// GET /api/v1/api-keys/{id}
func (a authAPI) getAPIKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	key, err := a.store.APIKey(r.Context(), orgCtx.workspace.ID, orgCtx.userID, r.PathValue("id"))
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAPIKeyResponse(key))
}

// DELETE /api/v1/api-keys/{id}
func (a authAPI) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.store.DeleteAPIKey(r.Context(), orgCtx.workspace.ID, orgCtx.userID, r.PathValue("id")); err != nil {
		writeAPIKeyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/api-keys/{id}/revoke
//
// Idempoten dari sisi hasil: mencabut key yang sudah dicabut menjawab 200,
// bukan 404 atau 409. Yang tidak ada memang 404 — dan itu dibedakan dengan
// membaca dulu, karena `RevokeAPIKey` mengubah nol baris di kedua kasus.
func (a authAPI) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	revoked, err := a.store.RevokeAPIKey(r.Context(), orgCtx.workspace.ID, orgCtx.userID, r.PathValue("id"))
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"revoked": revoked})
}
