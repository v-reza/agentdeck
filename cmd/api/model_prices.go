// Harga manual per model — tingkat 1 resolusi harga (DECISIONS 6A.C).
//
//	GET    /api/v1/model-prices           Viewer
//	PUT    /api/v1/model-prices/{model}   Admin
//	DELETE /api/v1/model-prices/{model}   Admin
//
// Endpoint ini menutup lubang yang terukur: nama model BYO tidak akan pernah
// cocok dengan tabel katalog maupun pattern mana pun, jadi `pricing.Resolve`
// menjatuhkannya ke tingkat 4 `unpriced` dengan biaya nol — dan nol di
// `ledger_entries` tidak bisa dibedakan dari "gratis". Tanpa rute ini, tidak ada
// satu pun cara mengisi harga untuk model seperti itu.
//
// Baca = Viewer, tulis = Admin. Itu pembagian yang sama dengan registry
// provider (AC8): harga yang dipakai bersama oleh seluruh ruang kerja adalah
// tindakan administratif, sedangkan membacanya tidak mengungkap rahasia apa pun.
//
// `PUT`, bukan `POST`: baris di sini diidentifikasi oleh nama model, tidak ada
// koleksi yang tumbuh. Menaruh namanya di path membuat rute itu mengekspresikan
// kunci aslinya, dan `ON CONFLICT (org_id, model)` di query membuat pengulangan
// aman (Idempotent: ya).
package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agentdeck/internal/auth"
	"agentdeck/internal/modelprice"
	"agentdeck/internal/pricing"
)

// modelPriceAPI is the handler group. It holds the service and nothing else.
type modelPriceAPI struct {
	svc *modelprice.Service
}

// modelPriceRequest is a write payload. The three optional rates are pointers so
// "absent" and "zero" are different requests: absent falls back to another rate,
// zero means free, and only the caller knows which one they mean.
type modelPriceRequest struct {
	InputMicrosPer1m      *int64 `json:"input_micros_per_1m"`
	OutputMicrosPer1m     *int64 `json:"output_micros_per_1m"`
	CachedMicrosPer1m     *int64 `json:"cached_micros_per_1m"`
	ReasoningMicrosPer1m  *int64 `json:"reasoning_micros_per_1m"`
	CacheWriteMicrosPer1m *int64 `json:"cache_write_micros_per_1m"`
}

// modelPriceResponse reports the rates in both units, for the same reason the
// agent catalog does: a client that wants dollars should not do its own lossy
// conversion, and one that wants the exact integer should not have to recover it
// from a float.
type modelPriceResponse struct {
	Model      string       `json:"model"`
	Input      catalogRate  `json:"input"`
	Output     catalogRate  `json:"output"`
	Cached     *catalogRate `json:"cached"`
	Reasoning  *catalogRate `json:"reasoning"`
	CacheWrite *catalogRate `json:"cache_creation"`
	Source     string       `json:"price_source"`
	UpdatedAt  string       `json:"updated_at"`
}

func toModelPriceResponse(o modelprice.Override) modelPriceResponse {
	return modelPriceResponse{
		Model:      o.Model,
		Input:      rate(o.InputMicrosPer1M),
		Output:     rate(o.OutputMicrosPer1M),
		Cached:     optionalRate(o.CachedMicrosPer1M),
		Reasoning:  optionalRate(o.ReasoningMicrosPer1M),
		CacheWrite: optionalRate(o.CacheWriteMicrosPer1M),
		// The source is a constant of this endpoint rather than a column: a row
		// in this table IS the manual tier. Storing the word "manual" next to
		// every row would be a second copy of the same fact.
		Source:    string(pricing.SourceManual),
		UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// optionalRate is nil for an absent rate so the response says "this field is not
// part of the price" instead of reporting a fallback value as if it were stored.
func optionalRate(v *int64) *catalogRate {
	if v == nil {
		return nil
	}
	r := rate(*v)
	return &r
}

// GET /api/v1/model-prices
func (a modelPriceAPI) listModelPrices(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	overrides, err := a.svc.List(r.Context(), orgCtx.workspace.ID)
	if err != nil {
		a.writeError(w, err)
		return
	}
	out := make([]modelPriceResponse, 0, len(overrides))
	for _, o := range overrides {
		out = append(out, toModelPriceResponse(o))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"prices": out})
}

// PUT /api/v1/model-prices/{model}
func (a modelPriceAPI) setModelPrice(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	model, err := pathModel(r)
	if err != nil {
		http.Error(w, modelprice.ErrInvalidInput.Error(), http.StatusBadRequest)
		return
	}

	var req modelPriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed JSON body", http.StatusBadRequest)
		return
	}

	saved, err := a.svc.Set(r.Context(), orgCtx.workspace.ID, orgCtx.userID, modelprice.SetInput{
		Model:                 model,
		InputMicrosPer1M:      req.InputMicrosPer1m,
		OutputMicrosPer1M:     req.OutputMicrosPer1m,
		CachedMicrosPer1M:     req.CachedMicrosPer1m,
		ReasoningMicrosPer1M:  req.ReasoningMicrosPer1m,
		CacheWriteMicrosPer1M: req.CacheWriteMicrosPer1m,
	})
	if err != nil {
		a.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toModelPriceResponse(saved))
}

// DELETE /api/v1/model-prices/{model}
func (a modelPriceAPI) deleteModelPrice(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := a.context(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	model, err := pathModel(r)
	if err != nil {
		http.Error(w, modelprice.ErrInvalidInput.Error(), http.StatusBadRequest)
		return
	}
	if err := a.svc.Remove(r.Context(), orgCtx.workspace.ID, model); err != nil {
		a.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a modelPriceAPI) context(r *http.Request) (orgContext, error) {
	return currentOrgContext(r)
}

// writeError maps the two domain sentinels. A model with no override is a 404
// (nothing to delete, nothing to report), and a bad payload is a 400.
func (a modelPriceAPI) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, modelprice.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, modelprice.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// pathModel reads the model name out of the path.
//
// The name is URL-escaped in the path because real model names carry slashes
// ("anthropic/claude-opus-4-6"), and a raw slash would be read as another path
// segment and never match the route. Unescaped, trimmed, and refused when empty
// — an empty model name would write a row no lookup could ever find.
func pathModel(r *http.Request) (string, error) {
	raw := r.PathValue("model")
	if unescaped, err := url.PathUnescape(raw); err == nil {
		raw = unescaped
	}
	model := strings.TrimSpace(raw)
	if model == "" {
		return "", modelprice.ErrInvalidInput
	}
	return model, nil
}

// registerModelPriceRoutes mounts the three routes. Like the provider and agent
// registrations, the role gate is declared in one place and the RBAC test drives
// this same table.
func registerModelPriceRoutes(mux *http.ServeMux, api authAPI, svc *modelprice.Service) {
	handler := modelPriceAPI{svc: svc}
	apiRoute := func(pattern string, h http.Handler, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(h, minimum)))
	}
	apiRoute("GET /api/v1/model-prices", http.HandlerFunc(handler.listModelPrices), auth.Viewer)
	apiRoute("PUT /api/v1/model-prices/{model}", http.HandlerFunc(handler.setModelPrice), auth.Admin)
	apiRoute("DELETE /api/v1/model-prices/{model}", http.HandlerFunc(handler.deleteModelPrice), auth.Admin)
}
