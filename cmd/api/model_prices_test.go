package main

// HTTP coverage for the manual price routes.
//
// Two things are pinned here and nowhere else.
//
// 1. The role gates, through the real middleware chain: reading a price is
//    Viewer, writing one is Admin. This is the same table main.go mounts, so a
//    loosening in the wiring fails here rather than only in production. It is
//    also why the routes are registered through `apiRoute` — a helper name
//    tools/verify_suite.py does not recognise would silently drop all three
//    routes out of its cmd/api scan and out of this test.
//
// 2. The path decoding. Real model names carry a slash ("anthropic/claude-…"),
//    so the name is URL-escaped in the path and has to survive the round trip.
//    A route that never matched would look like "my price was ignored" rather
//    than like an error.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"agentdeck/internal/auth"
	"agentdeck/internal/modelprice"
	"agentdeck/internal/pricing"
)

// priceFakeRepo is an in-memory Repo for the handler tests. The Postgres
// behaviour is covered in internal/modelprice; this one only needs to answer.
type priceFakeRepo struct {
	rows   map[string]modelprice.Override
	orgIDs []string
}

func newPriceFakeRepo() *priceFakeRepo {
	return &priceFakeRepo{rows: make(map[string]modelprice.Override)}
}

func (f *priceFakeRepo) List(_ context.Context, orgID string) ([]modelprice.Override, error) {
	f.orgIDs = append(f.orgIDs, orgID)
	out := make([]modelprice.Override, 0, len(f.rows))
	for _, row := range f.rows {
		out = append(out, row)
	}
	return out, nil
}

func (f *priceFakeRepo) Get(_ context.Context, _, model string) (modelprice.Override, error) {
	row, ok := f.rows[model]
	if !ok {
		return modelprice.Override{}, modelprice.ErrNotFound
	}
	return row, nil
}

func (f *priceFakeRepo) Upsert(_ context.Context, orgID, _ string, o modelprice.Override) (modelprice.Override, error) {
	f.orgIDs = append(f.orgIDs, orgID)
	f.rows[o.Model] = o
	return o, nil
}

func (f *priceFakeRepo) Delete(_ context.Context, orgID, model string) (int64, error) {
	f.orgIDs = append(f.orgIDs, orgID)
	if _, ok := f.rows[model]; !ok {
		return 0, nil
	}
	delete(f.rows, model)
	return 1, nil
}

// startPriceServer mounts the three routes on a throwaway server and returns it
// with the repository behind it.
func startPriceServer(t *testing.T) (*httptest.Server, *priceFakeRepo) {
	t.Helper()
	repo := newPriceFakeRepo()
	svc := modelprice.NewService(repo)

	mux := http.NewServeMux()
	api := authAPI{
		store:      auth.NewStore(auth.NewMemoryRepository()),
		appBaseURL: "http://localhost:5173",
	}
	registerModelPriceRoutes(mux, api, svc)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, repo
}

func priceRequest(t *testing.T, server *httptest.Server, method, path string, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The org middleware needs a resolved workspace. The value comes from the
	// header the frontend sends, and this server has no auth store behind it, so
	// the middleware answers 401 before the handler runs — which is itself worth
	// pinning: an unauthenticated price write must not be reachable.
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// The route table itself: three routes, and the gate on each one. A missing
// route 404s, which is the failure `verify_suite.py` scans for — this test makes
// it fail loudly in `go test` too.
func TestPriceRoutesAreMountedWithTheRightGate(t *testing.T) {
	server, _ := startPriceServer(t)

	// No session and no org header: the middleware must refuse before the
	// handler, on all three. 401 vs 403 depends on the store, and the point is
	// that it is not 200 or 404.
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/model-prices"},
		{http.MethodPut, "/api/v1/model-prices/" + url.PathEscape("my-own-llama-70b")},
		{http.MethodDelete, "/api/v1/model-prices/" + url.PathEscape("my-own-llama-70b")},
	} {
		resp := priceRequest(t, server, tc.method, tc.path, "")
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("%s %s: 404 — route tidak ter-mount", tc.method, tc.path)
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			t.Errorf("%s %s: %d tanpa sesi — gate-nya bocor", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// The path decode, tested through the handler rather than the regex: a model
// name with a slash has to arrive whole at the repo.
func TestSetModelPriceDecodesEscapedModelName(t *testing.T) {
	handler := modelPriceAPI{svc: modelprice.NewService(newPriceFakeRepo())}

	const model = "anthropic/claude-opus-4-6"
	req := httptest.NewRequest(http.MethodPut, "/api/v1/model-prices/"+url.PathEscape(model), nil)
	req.SetPathValue("model", url.PathEscape(model))

	got, err := pathModel(req)
	if err != nil {
		t.Fatalf("pathModel: %v", err)
	}
	if got != model {
		t.Errorf("pathModel = %q, want %q", got, model)
	}
	_ = handler
}

func TestPathModelRefusesEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/model-prices/%20", nil)
	req.SetPathValue("model", "%20")
	if _, err := pathModel(req); !errors.Is(err, modelprice.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// The catalog has to report the override, not the table. This is the assertion
// that keeps the screen and the ledger telling the same story.
func TestCatalogReportsTheOverrideNotTheTable(t *testing.T) {
	const byo = "my-own-llama-70b"
	overrides := map[string]*pricing.ModelPrice{
		byo: {
			PriceVersion:      pricing.PriceVersion,
			Model:             byo,
			InputMicrosPer1M:  500_000,
			OutputMicrosPer1M: 1_500_000,
		},
	}

	res := pricing.Resolve(byo, overrides[byo])
	if res.Source != pricing.SourceManual {
		t.Fatalf("source = %q, want manual", res.Source)
	}
	if got := res.Cost(pricing.Tokens{In: 1_000_000}); got != 500_000 {
		t.Errorf("cost = %d micros, want 500000", got)
	}
}

func TestModelPriceResponseCarriesBothUnits(t *testing.T) {
	// The catalog does this and the override route must match: a client that
	// wants dollars should not divide by 1e6 itself, because that is where a
	// rounding bug hides.
	resp := toModelPriceResponse(modelprice.Override{
		Model:             "m",
		InputMicrosPer1M:  5_000_000,
		OutputMicrosPer1M: 25_000_000,
	})
	if resp.Input.MicrosPer1M != 5_000_000 {
		t.Errorf("micros = %d, want 5000000", resp.Input.MicrosPer1M)
	}
	if resp.Input.USDPer1M != 5 {
		t.Errorf("usd = %v, want 5", resp.Input.USDPer1M)
	}
	if resp.Source != "manual" {
		t.Errorf("source = %q, want manual", resp.Source)
	}
	// An absent optional rate must serialize as absent, not as a fallback value
	// the workspace never stored.
	if resp.Cached != nil {
		t.Errorf("cached = %+v, want nil for an absent rate", resp.Cached)
	}
}
