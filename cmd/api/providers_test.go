package main

// US-AD109 — provider registry. Each acceptance criterion phase 2 delivers, with
// a test that fails for the *right* reason:
//
//	AC1 : a provider carries name, protocol, base URL, a sealed credential, and
//	      a model list; protocol is one of the three the DDL allows.
//	AC2 : the credential is write-only — sealed at rest, masked once, and never
//	      returned by any read.
//	AC4 : the base URL passes the same SSRF guard as US-AD106 AC3.
//	AC5 : deleting a provider agents still use is a 409 that names them.
//	AC6 : editing a provider reaches every agent using it.
//	AC8 : owner/admin write; member/viewer read.
//	AC9 : one default per workspace; the first provider is it; deleting it
//	      leaves none rather than an error.
//
// AC3 and AC7 are not here because their endpoints are not mounted: both need a
// protocol-aware upstream call and the contract schedules them for phase 3. Their
// §6.2.8 rows stay ⬜, which tools/verify_suite.py enforces against cmd/api.
//
// The mux comes from registerProviderRoutes — the same function main.go calls —
// so these assert the production role gate and the production handler wiring
// rather than a copy of either. A copy would keep passing after main.go was
// loosened, which is the exact failure mode being guarded.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"agentdeck/internal/crypto"
	"agentdeck/internal/providerreg"
)

// fakeProviderRepo is the in-memory providerreg.Repo these tests drive. It
// reproduces the behaviours Postgres owns for this table — the per-workspace
// name uniqueness (providers_org_name_key), at most one default
// (providers_org_default_key), and org-scoped reads — so the handler's error
// mapping is the thing under test rather than the database's.
//
// The embedded interface is deliberate: an unimplemented method panics with a
// nil-pointer dereference if a test ever reaches it, where a silent zero value
// would let a test "pass" against a repository that persisted nothing.
type fakeProviderRepo struct {
	providerreg.Repo

	mu        sync.Mutex
	providers map[string]providerreg.Provider
	// sealed is the ciphertext as stored: orgID -> providerID -> bytes. Held
	// apart from Provider because the domain type never carries it, and the
	// point of AC2 is that no read path can produce it.
	sealed map[string]map[string][]byte
	// using is the agents.provider_id side of AC5 and AC6:
	// orgID -> providerID -> agent id -> agent name.
	using map[string]map[string]map[string]string
	// baseURLs is the agents.base_url column AC6 reaches before phase 6 drops
	// it: orgID -> agentID -> address.
	baseURLs map[string]map[string]string
}

func newFakeProviderRepo() *fakeProviderRepo {
	return &fakeProviderRepo{
		providers: map[string]providerreg.Provider{},
		sealed:    map[string]map[string][]byte{},
		using:     map[string]map[string]map[string]string{},
		baseURLs:  map[string]map[string]string{},
	}
}

func (r *fakeProviderRepo) List(_ context.Context, orgID string) ([]providerreg.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []providerreg.Provider{}
	for _, p := range r.providers {
		if p.OrgID == orgID {
			out = append(out, p)
		}
	}
	// Mirrors the query's ORDER BY is_default DESC, name.
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *fakeProviderRepo) Get(_ context.Context, orgID, id string) (providerreg.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return providerreg.Provider{}, providerreg.ErrProviderNotFound
	}
	return p, nil
}

func (r *fakeProviderRepo) Count(_ context.Context, orgID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.providers {
		if p.OrgID == orgID {
			n++
		}
	}
	return n, nil
}

func (r *fakeProviderRepo) Create(_ context.Context, orgID, id string, in providerreg.CreateInput) (providerreg.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.providers {
		if existing.OrgID == orgID && existing.Name == in.Name {
			return providerreg.Provider{}, providerreg.ErrNameTaken
		}
	}
	wantDefault := in.IsDefault != nil && *in.IsDefault
	// providers_org_default_key is a partial unique index: at most one default
	// per workspace. Reproduced here because it is an invariant Postgres owns,
	// and a fake that ignored it would let a missing ClearDefault pass — the
	// production failure is a 23505, which is not a 500 this handler maps.
	if wantDefault {
		for _, existing := range r.providers {
			if existing.OrgID == orgID && existing.IsDefault {
				return providerreg.Provider{}, errors.New("providers_org_default_key: a default already exists")
			}
		}
	}
	p := providerreg.Provider{
		ID:        id,
		OrgID:     orgID,
		Name:      in.Name,
		Protocol:  providerreg.Protocol(in.Protocol),
		BaseURL:   in.BaseURL,
		Models:    []string{},
		IsDefault: wantDefault,
		HasKey:    len(in.SealedKey) > 0,
	}
	r.providers[id] = p
	if len(in.SealedKey) > 0 {
		if r.sealed[orgID] == nil {
			r.sealed[orgID] = map[string][]byte{}
		}
		r.sealed[orgID][id] = in.SealedKey
	}
	return p, nil
}

func (r *fakeProviderRepo) Update(_ context.Context, orgID, id string, in providerreg.UpdateInput) (providerreg.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return providerreg.Provider{}, providerreg.ErrProviderNotFound
	}
	if in.Name != nil {
		for otherID, other := range r.providers {
			if otherID != id && other.OrgID == orgID && other.Name == *in.Name {
				return providerreg.Provider{}, providerreg.ErrNameTaken
			}
		}
		p.Name = *in.Name
	}
	if in.Protocol != nil {
		p.Protocol = providerreg.Protocol(*in.Protocol)
	}
	if in.BaseURL != nil {
		p.BaseURL = *in.BaseURL
	}
	if in.IsDefault != nil {
		p.IsDefault = *in.IsDefault
	}
	if len(in.SealedKey) > 0 {
		p.HasKey = true
		if r.sealed[orgID] == nil {
			r.sealed[orgID] = map[string][]byte{}
		}
		r.sealed[orgID][id] = in.SealedKey
	}
	if in.IsDefault != nil && *in.IsDefault && !p.IsDefault {
		// Same partial unique index as Create: promoting to default while another
		// row still holds it is a 23505 in production.
		for otherID, other := range r.providers {
			if otherID != id && other.OrgID == orgID && other.IsDefault {
				return providerreg.Provider{}, errors.New("providers_org_default_key: a default already exists")
			}
		}
	}
	r.providers[id] = p
	return p, nil
}

func (r *fakeProviderRepo) ClearDefault(_ context.Context, orgID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.providers {
		if p.OrgID == orgID && p.IsDefault {
			p.IsDefault = false
			r.providers[id] = p
		}
	}
	return nil
}

func (r *fakeProviderRepo) Delete(_ context.Context, orgID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return providerreg.ErrProviderNotFound
	}
	delete(r.providers, id)
	delete(r.sealed[orgID], id)
	return nil
}

func (r *fakeProviderRepo) AgentsUsing(_ context.Context, orgID, providerID string) ([]providerreg.AgentRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	refs := make([]providerreg.AgentRef, 0, len(r.using[orgID][providerID]))
	for id, name := range r.using[orgID][providerID] {
		refs = append(refs, providerreg.AgentRef{ID: id, Name: name})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs, nil
}

// SyncAgentBaseURL carries a provider's address to the agents using it, which is
// how AC6 is observed before phase 6 drops agents.base_url.
func (r *fakeProviderRepo) SyncAgentBaseURL(_ context.Context, orgID, providerID, baseURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Only openai_compatible agents carry an address: agents_base_url_chk allows
	// one exactly when the agent's own provider is that value, so the real query
	// filters on it too. Reproduced here so an AC6 test cannot pass by writing an
	// address the database would have refused.
	if r.baseURLs[orgID] == nil {
		r.baseURLs[orgID] = map[string]string{}
	}
	for agentID := range r.using[orgID][providerID] {
		r.baseURLs[orgID][agentID] = baseURL
	}
	return nil
}

// assign records that agent `id`/`name` uses `providerID` in `orgID`. The map is
// keyed by agent id, so both the AC5 list and the AC6 sync read it the same way
// the generated queries do.
func (r *fakeProviderRepo) assign(orgID, providerID, id, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.using[orgID] == nil {
		r.using[orgID] = map[string]map[string]string{}
	}
	if r.using[orgID][providerID] == nil {
		r.using[orgID][providerID] = map[string]string{}
	}
	r.using[orgID][providerID][id] = name
}

// storedCiphertext is the sealed bytes the repository was handed, for the AC2
// assertions that the plaintext never reached it.
func (r *fakeProviderRepo) storedCiphertext(orgID, providerID string) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sealed[orgID][providerID]
}

// GetEncryptedKey mirrors the production query: it is the only way to reach the
// ciphertext, which is what makes "no read path returns a credential" a property
// of the interface rather than a promise.
func (r *fakeProviderRepo) GetEncryptedKey(_ context.Context, orgID, id string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return nil, providerreg.ErrProviderNotFound
	}
	return r.sealed[orgID][id], nil
}

// SetModels writes the fetched list and its timestamp together, like the query.
func (r *fakeProviderRepo) SetModels(_ context.Context, orgID, id string, models []string, fetchedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return providerreg.ErrProviderNotFound
	}
	p.Models = append([]string{}, models...)
	at := fetchedAt
	p.ModelsFetchedAt = &at
	r.providers[id] = p
	return nil
}

// SetVerifiedAt stamps the verification instant after a passing probe (AC3).
func (r *fakeProviderRepo) SetVerifiedAt(_ context.Context, orgID, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return providerreg.ErrProviderNotFound
	}
	stamp := at
	p.LastVerifiedAt = &stamp
	r.providers[id] = p
	return nil
}

// StaleProviders mirrors the cross-org query: rows whose list was never fetched
// or is older than before, oldest first, capped at limit.
func (r *fakeProviderRepo) StaleProviders(_ context.Context, before time.Time, limit int) ([]providerreg.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []providerreg.Provider{}
	for _, p := range r.providers {
		if p.ModelsFetchedAt == nil || p.ModelsFetchedAt.Before(before) {
			out = append(out, p)
		}
	}
	// ORDER BY models_fetched_at NULLS FIRST: never-fetched rows come first.
	sort.Slice(out, func(i, j int) bool {
		if (out[i].ModelsFetchedAt == nil) != (out[j].ModelsFetchedAt == nil) {
			return out[i].ModelsFetchedAt == nil
		}
		if out[i].ModelsFetchedAt == nil {
			return out[i].Name < out[j].Name
		}
		return out[i].ModelsFetchedAt.Before(*out[j].ModelsFetchedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// probeCalls records what the fake probe was asked to do, so a test can assert
// the upstream was never touched — the assertion that separates "refused before
// dialing" from "dialed and failed".
type probeCalls struct {
	mu             sync.Mutex
	listModels     int
	probeInference int
	models         []string
	lastModel      string
	lastKey        string
	lastBaseURL    string
}

func (c *probeCalls) snapshot() (listModels, probeInference int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listModels, c.probeInference
}

// fakeProbe is the in-memory providerreg.Probe. It lets each AC drive the
// upstream's answer without a live endpoint, and counts calls so "zero upstream
// traffic" is testable.
type fakeProbe struct {
	calls *probeCalls
	// modelsErr / probeErr are the upstream failures to return, if any.
	modelsErr error
	probeErr  error
	// models is the list ListModels answers with.
	models []string
}

func (p *fakeProbe) ListModels(_ context.Context, baseURL, apiKey string) ([]string, error) {
	p.calls.mu.Lock()
	p.calls.listModels++
	p.calls.lastBaseURL = baseURL
	p.calls.lastKey = apiKey
	p.calls.models = append([]string{}, p.models...)
	p.calls.mu.Unlock()
	if p.modelsErr != nil {
		return nil, p.modelsErr
	}
	return p.models, nil
}

func (p *fakeProbe) ProbeInference(_ context.Context, baseURL, apiKey, model string) error {
	p.calls.mu.Lock()
	p.calls.probeInference++
	p.calls.lastBaseURL = baseURL
	p.calls.lastKey = apiKey
	p.calls.lastModel = model
	p.calls.mu.Unlock()
	return p.probeErr
}

// agentBaseURL is what an agent's address column holds after a provider edit,
// which is how AC6 is observed before phase 6 drops that column.
func (r *fakeProviderRepo) agentBaseURL(orgID, agentID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.baseURLs[orgID][agentID]
}

// ageModels backdates a provider's models_fetched_at so a test can make it stale
// without sleeping 24 hours. It also seeds a list, so "was refreshed" is
// distinguishable from "was empty".
func (r *fakeProviderRepo) ageModels(_ *testing.T, orgID, id string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return
	}
	stamp := at
	p.ModelsFetchedAt = &stamp
	p.Models = []string{"seed"}
	r.providers[id] = p
}

// models reads back the stored list, which is how AC7 is observed.
func (r *fakeProviderRepo) models(orgID, id string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok || p.OrgID != orgID {
		return nil
	}
	return p.Models
}

// providerFixture is the shared setup: one mux carrying the real provider routes,
// one in-memory repository, and the RBAC scenario (orgA with one actor per role,
// plus bella's separate tenant).
type providerFixture struct {
	mux      *http.ServeMux
	repo     *fakeProviderRepo
	scenario rbacTestAPI
	key      []byte
	// probe is the fake upstream, and calls records what it was asked to do.
	// Both are shared with the service, so a test can make the upstream fail or
	// count the requests that never happened.
	probe *fakeProbe
	calls *probeCalls
	// svc is the same service the routes use, so a test can drive the refresher
	// through the production path instead of a second, parallel one.
	svc *providerreg.Service
}

func newProviderFixture(t *testing.T) providerFixture {
	t.Helper()
	return newProviderFixtureWithKey(t, testMasterKeyHex)
}

// newProviderFixtureWithKey lets a test pick the master key *before* the routes
// are registered. That order is forced, not stylistic: registerProviderRoutes
// captures the key into the handler at registration time, so a test that assigns
// `masterKey` afterwards is ignored, and re-registering to pick it up panics on a
// duplicate ServeMux pattern.
func newProviderFixtureWithKey(t *testing.T, keyRaw string) providerFixture {
	t.Helper()
	scenario := newRBACTestAPI(t)
	repo := newFakeProviderRepo()
	var key []byte
	if keyRaw != "" {
		k, err := crypto.LoadKey(keyRaw)
		if err != nil {
			t.Fatalf("load test master key: %v", err)
		}
		key = k
	}
	scenario.api.masterKey = keyRaw

	calls := &probeCalls{}
	probe := &fakeProbe{calls: calls}

	mux := http.NewServeMux()
	// The service is built the same way main.go builds it — with a probe and a
	// real decrypter — so the two phase-3 endpoints exercise the production
	// wiring rather than a stubbed-out path. Only the *transport* is fake.
	svc := providerreg.NewServiceWithProbe(repo, providerreg.ServiceOptions{
		Probe:   probe,
		Decrypt: credentialDecrypter(keyRaw),
	})
	registerProviderRoutes(mux, scenario.api, svc)
	return providerFixture{mux: mux, repo: repo, scenario: scenario, key: key, probe: probe, calls: calls, svc: svc}
}

// doProvider performs one request against the provider mux. An empty actor sends
// no credential at all; an empty org omits X-Org-ID, which resolves to the
// actor's personal workspace.
func (f providerFixture) doProvider(t *testing.T, method, path, body, actor, org string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if actor != "" {
		req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	}
	if org != "" {
		req.Header.Set("X-Org-ID", org)
	}
	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, req)
	return recorder
}

func (f providerFixture) create(t *testing.T, actor, org, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.doProvider(t, http.MethodPost, "/api/v1/providers", body, actor, org)
}

func (f providerFixture) patch(t *testing.T, actor, org, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.doProvider(t, http.MethodPatch, "/api/v1/providers/"+id, body, actor, org)
}

// mustCreate is the setup helper: it creates a provider as the owner and fails
// the test if the request did not answer 201.
func (f providerFixture) mustCreate(t *testing.T, org, name, protocol, baseURL, apiKey string) providerResponse {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"name": name, "protocol": protocol, "base_url": baseURL, "api_key": apiKey,
	})
	if err != nil {
		t.Fatalf("marshal provider body: %v", err)
	}
	recorder := f.create(t, "alice", org, string(payload))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("setup create provider %s: want 201, got %d — %s", name, recorder.Code, recorder.Body.String())
	}
	return decodeProvider(t, recorder)
}

func decodeProvider(t *testing.T, recorder *httptest.ResponseRecorder) providerResponse {
	t.Helper()
	var got providerResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode provider response %q: %v", recorder.Body.String(), err)
	}
	return got
}

func decodeProviderList(t *testing.T, recorder *httptest.ResponseRecorder) []providerResponse {
	t.Helper()
	var got []providerResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode provider list %q: %v", recorder.Body.String(), err)
	}
	return got
}

// ---- AC8: role floors -------------------------------------------------------

// TestProviderWriteRequiresAdmin is AC8. A provider's credential is shared by
// every agent in the workspace, so changing it is a security action: member and
// viewer are refused at the gate, before any handler runs.
func TestProviderWriteRequiresAdmin(t *testing.T) {
	f := newProviderFixture(t)

	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false}, // owner
		{actor: "andre", wantDenied: false}, // admin
		{actor: "marta", wantDenied: true},  // member
		{actor: "vera", wantDenied: true},   // viewer
	}

	createBody := `{"name":"Acme","protocol":"openai_compatible","base_url":"https://api.example.com/v1","api_key":"sk-test-1234567890"}`
	for _, tc := range cases {
		t.Run("POST/"+tc.actor, func(t *testing.T) {
			// A distinct name per actor so a successful create does not make the
			// next actor's request a 409 instead of the 201 it expects.
			body := strings.Replace(createBody, `"Acme"`, `"Acme-`+tc.actor+`"`, 1)
			recorder := f.create(t, tc.actor, f.scenario.orgA, body)
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s creating a provider: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code != http.StatusCreated {
				t.Fatalf("%s creating a provider: status = %d, want 201 (%s)", tc.actor, recorder.Code, recorder.Body.String())
			}
		})
	}

	// PATCH and DELETE need a real row to aim at; created by an admin, then
	// driven by each role.
	target := f.mustCreate(t, f.scenario.orgA, "Target", "openai_compatible", "https://api.example.com/v1", "sk-test-1234567890")

	for _, tc := range cases {
		t.Run("PATCH/"+tc.actor, func(t *testing.T) {
			recorder := f.patch(t, tc.actor, f.scenario.orgA, target.ID, `{"name":"Target"}`)
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s editing a provider: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s editing a provider: status = 403, want the gate to allow admin+", tc.actor)
			}
		})
	}

	for _, tc := range cases {
		t.Run("DELETE/"+tc.actor, func(t *testing.T) {
			// A fresh row per actor: a successful delete would otherwise leave
			// the next subtest aiming at nothing.
			victim := f.mustCreate(t, f.scenario.orgA, "Victim-"+tc.actor, "openai_compatible", "https://api.example.com/v1", "")
			recorder := f.doProvider(t, http.MethodDelete, "/api/v1/providers/"+victim.ID, "", tc.actor, f.scenario.orgA)
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s deleting a provider: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("%s deleting a provider: status = %d, want 204 (%s)", tc.actor, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestProviderReadsAreViewerPlus is the other half of AC8: member and viewer
// read the registry without a credential. Denying the read would break the
// screen AC1/AC9 specify without protecting anything — the response carries no
// secret, only `has_key`.
func TestProviderReadsAreViewerPlus(t *testing.T) {
	f := newProviderFixture(t)
	created := f.mustCreate(t, f.scenario.orgA, "Readable", "openai_compatible", "https://api.example.com/v1", "sk-test-1234567890")

	for _, actor := range []string{"alice", "andre", "marta", "vera"} {
		t.Run("GET/"+actor, func(t *testing.T) {
			if recorder := f.doProvider(t, http.MethodGet, "/api/v1/providers", "", actor, f.scenario.orgA); recorder.Code != http.StatusOK {
				t.Fatalf("%s listing providers: status = %d, want 200", actor, recorder.Code)
			}
			if recorder := f.doProvider(t, http.MethodGet, "/api/v1/providers/"+created.ID, "", actor, f.scenario.orgA); recorder.Code != http.StatusOK {
				t.Fatalf("%s reading a provider: status = %d, want 200", actor, recorder.Code)
			}
		})
	}
}

// TestProviderRoutesRejectAnonymous pins that the registry is not reachable
// without a session: authentication runs before the role gate, so an
// unauthenticated request is a 401 rather than a 403.
func TestProviderRoutesRejectAnonymous(t *testing.T) {
	f := newProviderFixture(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/providers"},
		{http.MethodPost, "/api/v1/providers"},
		{http.MethodGet, "/api/v1/providers/some-id"},
		{http.MethodPatch, "/api/v1/providers/some-id"},
		{http.MethodDelete, "/api/v1/providers/some-id"},
	} {
		t.Run(route.method, func(t *testing.T) {
			recorder := f.doProvider(t, route.method, route.path, `{}`, "", f.scenario.orgA)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s without a session: status = %d, want 401", route.method, route.path, recorder.Code)
			}
		})
	}
}

// ---- AC2: the credential is write-only -------------------------------------

// TestProviderCredentialIsSealedAndNeverReturned is AC2. Three separate claims,
// each with its own failure: the plaintext never reaches storage, no read
// returns it, and the masked form is offered exactly once — in the response to
// the write that supplied it.
func TestProviderCredentialIsSealedAndNeverReturned(t *testing.T) {
	f := newProviderFixture(t)
	const apiKey = "sk-test-abcdefghijklmnop"

	created := f.mustCreate(t, f.scenario.orgA, "Acme", "openai_compatible", "https://api.example.com/v1", apiKey)

	// 1. What storage received is ciphertext that decrypts back to the key, and
	//    it is not the key itself.
	sealed := f.repo.storedCiphertext(f.scenario.orgA, created.ID)
	if len(sealed) == 0 {
		t.Fatal("create stored no credential at all")
	}
	if bytes.Contains(sealed, []byte(apiKey)) {
		t.Fatal("stored credential contains the plaintext key")
	}
	got, err := crypto.Open(f.key, sealed)
	if err != nil {
		t.Fatalf("stored credential does not decrypt: %v", err)
	}
	if got != apiKey {
		t.Fatalf("stored credential decrypts to %q, want the submitted key", got)
	}

	// 2. The create response masks it, and the mask is not the key.
	if created.MaskedKey == "" {
		t.Fatal("create response carried no masked_key")
	}
	if strings.Contains(created.MaskedKey, apiKey) || created.MaskedKey == apiKey {
		t.Fatalf("masked_key %q reveals the key", created.MaskedKey)
	}
	if !created.HasKey {
		t.Fatal("create response says has_key = false after a key was submitted")
	}

	// 3. Every read path answers has_key and nothing else — no masked key, and
	//    obviously no plaintext.
	reads := map[string]*httptest.ResponseRecorder{
		"GET list":   f.doProvider(t, http.MethodGet, "/api/v1/providers", "", "vera", f.scenario.orgA),
		"GET detail": f.doProvider(t, http.MethodGet, "/api/v1/providers/"+created.ID, "", "vera", f.scenario.orgA),
	}
	for name, recorder := range reads {
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (%s)", name, recorder.Code, recorder.Body.String())
		}
		if body := recorder.Body.String(); strings.Contains(body, apiKey) {
			t.Fatalf("%s leaked the plaintext credential", name)
		}
		if body := recorder.Body.String(); strings.Contains(body, "masked_key") {
			t.Fatalf("%s returned masked_key; only the write that supplied a key may", name)
		}
		if body := recorder.Body.String(); !strings.Contains(body, `"has_key":true`) {
			t.Fatalf("%s does not report has_key = true (%s)", name, body)
		}
	}

	// 4. A provider created without a credential reports has_key = false rather
	//    than pretending to have one.
	bare := f.mustCreate(t, f.scenario.orgA, "Local", "openai_compatible", "http://localhost:11434/v1", "")
	if bare.HasKey {
		t.Fatal("a provider created with no api_key reports has_key = true")
	}
	if bare.MaskedKey != "" {
		t.Fatalf("a provider created with no api_key got masked_key %q", bare.MaskedKey)
	}
}

// TestProviderWriteWithoutMasterKeyIs500 is the guard that stops a misconfigured
// deployment from storing a credential in the clear: with no
// AGENTDECK_MASTER_KEY, a write carrying a key fails rather than succeeding
// unencrypted.
func TestProviderWriteWithoutMasterKeyIs500(t *testing.T) {
	f := newProviderFixtureWithKey(t, "")

	body := `{"name":"Acme","protocol":"openai_compatible","base_url":"https://api.example.com/v1","api_key":"sk-test-1234567890"}`
	recorder := f.create(t, "alice", f.scenario.orgA, body)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("create with no master key: status = %d, want 500 (%s)", recorder.Code, recorder.Body.String())
	}
	if len(f.repo.providers) != 0 {
		t.Fatal("a provider row was created even though the credential could not be sealed")
	}

	// A provider with no credential needs no key at all, so it still works.
	bare := f.create(t, "alice", f.scenario.orgA, `{"name":"Local","protocol":"openai_compatible","base_url":"http://localhost:11434/v1"}`)
	if bare.Code != http.StatusCreated {
		t.Fatalf("create without a credential: status = %d, want 201 (%s)", bare.Code, bare.Body.String())
	}
}

// ---- AC4: the SSRF guard ----------------------------------------------------

// TestProviderBaseURLPassesSSRFGuard is AC4. The write path uses the
// address-only guard, so what is asserted is the address rule itself: private
// and link-local targets are refused, and the two literal forms DECISIONS 6A.F
// names are allowed.
func TestProviderBaseURLPassesSSRFGuard(t *testing.T) {
	f := newProviderFixture(t)

	refused := []string{
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/v1",
		"http://192.168.1.10:8080/v1",
		"http://172.16.4.4/v1",
		"http://[::1]:8080/v1",
		"ftp://api.example.com/v1",
		"",
	}
	for _, baseURL := range refused {
		t.Run("refused/"+baseURL, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"name": "N", "protocol": "openai_compatible", "base_url": baseURL})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			recorder := f.create(t, "alice", f.scenario.orgA, string(payload))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("base_url %q: status = %d, want 400 (%s)", baseURL, recorder.Code, recorder.Body.String())
			}
		})
	}

	allowed := []string{
		"http://localhost:11434/v1",
		"http://127.0.0.1:8080/v1",
		"https://api.example.com/v1",
	}
	for _, baseURL := range allowed {
		t.Run("allowed/"+baseURL, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"name": "N-" + baseURL, "protocol": "openai_compatible", "base_url": baseURL})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			recorder := f.create(t, "alice", f.scenario.orgA, string(payload))
			if recorder.Code != http.StatusCreated {
				t.Fatalf("base_url %q: status = %d, want 201 (%s)", baseURL, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// ---- AC1: protocol is the three-value enum ---------------------------------

// TestProviderProtocolIsTheThreeValueEnum is AC1. The DDL CHECK
// (providers_protocol_chk) accepts exactly three values, so the API accepts the
// same three and refuses anything else with a 400 rather than letting it reach
// the database and surface as a 500.
func TestProviderProtocolIsTheThreeValueEnum(t *testing.T) {
	f := newProviderFixture(t)

	for _, protocol := range []string{"openai_compatible", "anthropic", "google"} {
		t.Run("accepted/"+protocol, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"name": "P-" + protocol, "protocol": protocol, "base_url": "https://api.example.com/v1"})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if recorder := f.create(t, "alice", f.scenario.orgA, string(payload)); recorder.Code != http.StatusCreated {
				t.Fatalf("protocol %q: status = %d, want 201 (%s)", protocol, recorder.Code, recorder.Body.String())
			}
		})
	}

	for _, protocol := range []string{"openai", "OpenAI_Compatible", "azure", ""} {
		t.Run("refused/"+protocol, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"name": "R-" + protocol, "protocol": protocol, "base_url": "https://api.example.com/v1"})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if recorder := f.create(t, "alice", f.scenario.orgA, string(payload)); recorder.Code != http.StatusBadRequest {
				t.Fatalf("protocol %q: status = %d, want 400 (%s)", protocol, recorder.Code, recorder.Body.String())
			}
		})
	}

	// A name that is empty or only whitespace would pass providers_name_chk's
	// btrim test only after trimming, so it is refused before the write.
	for _, name := range []string{"", "   "} {
		t.Run("refused-name", func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"name": name, "protocol": "openai_compatible", "base_url": "https://api.example.com/v1"})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if recorder := f.create(t, "alice", f.scenario.orgA, string(payload)); recorder.Code != http.StatusBadRequest {
				t.Fatalf("name %q: status = %d, want 400 (%s)", name, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestProviderNameIsUniquePerWorkspace is the other half of AC1's 409: two
// providers in one workspace cannot share a name (providers_org_name_key), while
// the same name in another workspace is fine — the registry is org-scoped.
func TestProviderNameIsUniquePerWorkspace(t *testing.T) {
	f := newProviderFixture(t)
	f.mustCreate(t, f.scenario.orgA, "Acme", "openai_compatible", "https://api.example.com/v1", "")

	dup := f.create(t, "alice", f.scenario.orgA, `{"name":"Acme","protocol":"openai_compatible","base_url":"https://other.example.com/v1"}`)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate name in one workspace: status = %d, want 409 (%s)", dup.Code, dup.Body.String())
	}

	// bella's workspace is a different tenant, so the same name is available.
	personal := f.scenario.personalOrg("bella@x.test")
	same := f.create(t, "bella", personal, `{"name":"Acme","protocol":"openai_compatible","base_url":"https://api.example.com/v1"}`)
	if same.Code != http.StatusCreated {
		t.Fatalf("same name in another workspace: status = %d, want 201 (%s)", same.Code, same.Body.String())
	}
}

// ---- tenant isolation (US-AD07) --------------------------------------------

// TestProviderIsOrgScoped pins the tenant boundary on every route: another
// workspace's provider id is a 404, never a 403, so the existence of the id does
// not leak.
func TestProviderIsOrgScoped(t *testing.T) {
	f := newProviderFixture(t)
	foreign := f.mustCreate(t, f.scenario.orgA, "Foreign", "openai_compatible", "https://api.example.com/v1", "sk-test-1234567890")

	personal := f.scenario.personalOrg("bella@x.test")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"read", http.MethodGet, "/api/v1/providers/" + foreign.ID, "", http.StatusNotFound},
		{"patch", http.MethodPatch, "/api/v1/providers/" + foreign.ID, `{"name":"Stolen"}`, http.StatusNotFound},
		{"delete", http.MethodDelete, "/api/v1/providers/" + foreign.ID, "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := f.doProvider(t, tc.method, tc.path, tc.body, "bella", personal)
			if recorder.Code != tc.want {
				t.Fatalf("%s a foreign provider: status = %d, want %d (%s)", tc.name, recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}

	// The list is scoped too: bella's workspace must not see orgA's provider.
	list := decodeProviderList(t, f.doProvider(t, http.MethodGet, "/api/v1/providers", "", "bella", personal))
	if len(list) != 0 {
		t.Fatalf("a foreign workspace listed %d providers, want 0", len(list))
	}

	// And the row survived every attempt.
	if _, err := f.repo.Get(context.Background(), f.scenario.orgA, foreign.ID); err != nil {
		t.Fatalf("the provider did not survive the foreign requests: %v", err)
	}
}

// ---- AC5: delete is refused while agents use the provider ------------------

// TestDeleteProviderInUseIs409 is AC5. The refusal has to name the agents
// pinning the provider, so it is a JSON body rather than http.Error's bare
// string: an operator reading "provider is still used by agents" without knowing
// which ones cannot act on it.
func TestDeleteProviderInUseIs409(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.mustCreate(t, f.scenario.orgA, "Acme", "openai_compatible", "https://api.example.com/v1", "")
	f.repo.assign(f.scenario.orgA, provider.ID, "agent-1", "Reviewer")
	f.repo.assign(f.scenario.orgA, provider.ID, "agent-2", "Builder")

	recorder := f.doProvider(t, http.MethodDelete, "/api/v1/providers/"+provider.ID, "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("deleting a provider in use: status = %d, want 409 (%s)", recorder.Code, recorder.Body.String())
	}
	var body providerInUseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 409 body %q: %v", recorder.Body.String(), err)
	}
	if body.Total != 2 {
		t.Fatalf("409 body reports total = %d, want 2", body.Total)
	}
	names := map[string]bool{}
	for _, ref := range body.Agents {
		names[ref.Name] = true
	}
	for _, want := range []string{"Reviewer", "Builder"} {
		if !names[want] {
			t.Fatalf("409 body does not name agent %q; got %v", want, names)
		}
	}

	// The row is still there: a refused delete must not have deleted anything.
	if _, err := f.repo.Get(context.Background(), f.scenario.orgA, provider.ID); err != nil {
		t.Fatalf("the provider was removed despite the 409: %v", err)
	}

	// Once the agents are repointed, the delete goes through.
	if err := f.repo.SyncAgentBaseURL(context.Background(), f.scenario.orgA, provider.ID, ""); err != nil {
		t.Fatalf("clear base urls: %v", err)
	}
	f.repo.mu.Lock()
	f.repo.using[f.scenario.orgA][provider.ID] = map[string]string{}
	f.repo.mu.Unlock()

	if recorder := f.doProvider(t, http.MethodDelete, "/api/v1/providers/"+provider.ID, "", "alice", f.scenario.orgA); recorder.Code != http.StatusNoContent {
		t.Fatalf("deleting an unused provider: status = %d, want 204 (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestDeleteProviderMissingIs404 pins the other delete path: an id that does not
// exist is a 404, not a 204 and not a 500.
func TestDeleteProviderMissingIs404(t *testing.T) {
	f := newProviderFixture(t)
	recorder := f.doProvider(t, http.MethodDelete, "/api/v1/providers/does-not-exist", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("deleting a missing provider: status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}
}

// ---- AC6: an edit reaches every agent using the provider -------------------

// TestProviderEditReachesItsAgents is AC6. The observable form before phase 6 is
// the agents' own address column: an agent's screens still render
// agents.base_url, so leaving it behind after a provider edit would make the
// change invisible exactly where the operator looks for it.
func TestProviderEditReachesItsAgents(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.mustCreate(t, f.scenario.orgA, "Acme", "openai_compatible", "https://old.example.com/v1", "")
	f.repo.assign(f.scenario.orgA, provider.ID, "agent-1", "Reviewer")
	f.repo.assign(f.scenario.orgA, provider.ID, "agent-2", "Builder")

	const newURL = "https://new.example.com/v1"
	recorder := f.patch(t, "alice", f.scenario.orgA, provider.ID, `{"base_url":"`+newURL+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("editing a provider's base_url: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	if got := decodeProvider(t, recorder).BaseURL; got != newURL {
		t.Fatalf("provider base_url = %q, want %q", got, newURL)
	}

	for _, agentID := range []string{"agent-1", "agent-2"} {
		if got := f.repo.agentBaseURL(f.scenario.orgA, agentID); got != newURL {
			t.Fatalf("agent %s base_url = %q, want %q — the edit did not reach it", agentID, got, newURL)
		}
	}

	// An edit that does not touch the address must not rewrite it: the sync is
	// driven by the presence of base_url in the request, not by every PATCH.
	f.repo.mu.Lock()
	f.repo.baseURLs[f.scenario.orgA]["agent-1"] = "untouched"
	f.repo.mu.Unlock()
	if recorder := f.patch(t, "alice", f.scenario.orgA, provider.ID, `{"name":"Renamed"}`); recorder.Code != http.StatusOK {
		t.Fatalf("renaming a provider: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	if got := f.repo.agentBaseURL(f.scenario.orgA, "agent-1"); got != "untouched" {
		t.Fatalf("a rename rewrote the agents' base_url to %q; only a base_url edit may", got)
	}
}

// TestProviderPatchIsPartial is the guard on the update shape. A full-replace
// update — the shape UpdateAgent uses — would blank every column the request
// omitted, and three of them (models_json, models_fetched_at, last_verified_at)
// belong to phase 3: renaming a provider would silently erase its model list and
// its verification timestamp.
func TestProviderPatchIsPartial(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.mustCreate(t, f.scenario.orgA, "Acme", "openai_compatible", "https://api.example.com/v1", "sk-test-1234567890")

	// Give the row the phase-3 fields the way phase 3 will, then rename it.
	f.repo.mu.Lock()
	stored := f.repo.providers[provider.ID]
	stored.Models = []string{"gpt-4o", "gpt-4o-mini"}
	f.repo.providers[provider.ID] = stored
	f.repo.mu.Unlock()

	recorder := f.patch(t, "alice", f.scenario.orgA, provider.ID, `{"name":"Renamed"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("renaming: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	got := decodeProvider(t, recorder)
	if got.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", got.Name)
	}
	if len(got.Models) != 2 {
		t.Fatalf("models = %v, want the two stored entries — a partial update must not blank them", got.Models)
	}
	if got.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base_url = %q, want it unchanged", got.BaseURL)
	}
	if got.Protocol != "openai_compatible" {
		t.Fatalf("protocol = %q, want it unchanged", got.Protocol)
	}
	if !got.HasKey {
		t.Fatal("has_key = false after a rename; the credential was dropped")
	}

	// An explicit empty key is refused rather than read as "remove it": no
	// endpoint in the contract removes a credential, so accepting "" would hide
	// the caller's mistake.
	empty := f.patch(t, "alice", f.scenario.orgA, provider.ID, `{"api_key":""}`)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("patching api_key to empty: status = %d, want 400 (%s)", empty.Code, empty.Body.String())
	}
}

// ---- AC9: the default provider ---------------------------------------------

// TestProviderDefaultLifecycle is AC9. Three claims: the workspace's first
// provider becomes its default so a single-provider workspace never has to pick,
// setting a new default clears the old one, and deleting the default leaves the
// workspace with none rather than an error.
func TestProviderDefaultLifecycle(t *testing.T) {
	f := newProviderFixture(t)
	org := f.scenario.orgA

	// The first provider is the default without being asked.
	first := f.mustCreate(t, org, "First", "openai_compatible", "https://first.example.com/v1", "")
	if !first.IsDefault {
		t.Fatal("the workspace's first provider is not the default")
	}

	// The second is not: with more than one there is no obvious default.
	second := f.mustCreate(t, org, "Second", "openai_compatible", "https://second.example.com/v1", "")
	if second.IsDefault {
		t.Fatal("the workspace's second provider became the default on its own")
	}

	// Moving the default clears the old one — the unique index allows at most
	// one, so a missed clear would be a 409 or a constraint violation.
	if recorder := f.patch(t, "alice", org, second.ID, `{"is_default":true}`); recorder.Code != http.StatusOK {
		t.Fatalf("promoting the second provider: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	list := decodeProviderList(t, f.doProvider(t, http.MethodGet, "/api/v1/providers", "", "vera", org))
	if len(list) != 2 {
		t.Fatalf("list returned %d providers, want 2", len(list))
	}
	if !list[0].IsDefault {
		t.Fatalf("the default is not listed first; got %q first", list[0].Name)
	}
	defaults := 0
	for _, p := range list {
		if p.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("workspace has %d defaults, want exactly 1", defaults)
	}
	if list[0].ID != second.ID {
		t.Fatalf("the default is %q, want the promoted one", list[0].Name)
	}

	// Deleting the default leaves none. AC9 calls that legal, not an error.
	if recorder := f.doProvider(t, http.MethodDelete, "/api/v1/providers/"+second.ID, "", "alice", org); recorder.Code != http.StatusNoContent {
		t.Fatalf("deleting the default provider: status = %d, want 204 (%s)", recorder.Code, recorder.Body.String())
	}
	after := decodeProviderList(t, f.doProvider(t, http.MethodGet, "/api/v1/providers", "", "vera", org))
	for _, p := range after {
		if p.IsDefault {
			t.Fatalf("provider %q is still the default after the default was deleted", p.Name)
		}
	}
	if len(after) != 1 {
		t.Fatalf("list returned %d providers after the delete, want 1", len(after))
	}

	// A create that explicitly asks not to be the default is honoured even when
	// it would otherwise have been the first.
	other := f.scenario.personalOrg("bella@x.test")
	no := false
	payload, err := json.Marshal(map[string]any{
		"name": "Explicit", "protocol": "openai_compatible",
		"base_url": "https://explicit.example.com/v1", "is_default": no,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if recorder := f.create(t, "bella", other, string(payload)); recorder.Code != http.StatusCreated {
		t.Fatalf("explicit is_default=false: status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
	}
	if decodeProvider(t, f.create(t, "bella", other, `{"name":"Third","protocol":"openai_compatible","base_url":"https://third.example.com/v1"}`)).IsDefault {
		t.Fatal("a second provider in a workspace with no default became one on its own")
	}
}

// ---- AC7: automatic refresh ------------------------------------------------

// TestRefresherRefetchesOnlyStaleProviders is AC7's automatic half. A list older
// than 24 hours is refetched with nobody pressing anything; a fresh one is left
// alone. The second half is what keeps the feature from becoming a poll: without
// it, every tick would hit every provider.
func TestRefresherRefetchesOnlyStaleProviders(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"gpt-4o-mini", "gpt-4o"}

	stale := f.mustCreate(t, f.scenario.orgA, "Stale", "openai_compatible", "https://stale.example.com/v1", "sk-tes...0000")
	fresh := f.mustCreate(t, f.scenario.orgA, "Fresh", "openai_compatible", "https://fresh.example.com/v1", "sk-tes...0000")

	// Age one row past the window and leave the other inside it. Both get a
	// model list so the refresher has something to compare against.
	now := time.Now().UTC()
	f.repo.ageModels(t, f.scenario.orgA, stale.ID, now.Add(-25*time.Hour))
	f.repo.ageModels(t, f.scenario.orgA, fresh.ID, now.Add(-1*time.Hour))

	refresher := providerreg.NewModelRefresher(f.svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	refreshed := refresher.RefreshOnce(context.Background())

	if refreshed != 1 {
		t.Fatalf("refreshed %d providers, want exactly 1 (only the stale one)", refreshed)
	}
	if got := f.repo.models(f.scenario.orgA, stale.ID); !reflect.DeepEqual(got, f.probe.models) {
		t.Fatalf("stale provider has %v, want the fetched list %v", got, f.probe.models)
	}
	// The fresh row still carries its seed list: it was not touched.
	if got := f.repo.models(f.scenario.orgA, fresh.ID); !reflect.DeepEqual(got, []string{"seed"}) {
		t.Fatalf("fresh provider was refetched: models = %v, want the untouched seed", got)
	}
}

// TestRefresherTreatsNeverFetchedAsStale is the case that makes the feature
// useful on day one: a provider registered a minute ago has no
// models_fetched_at at all, and reading NULL as "nothing to do" would leave it
// permanently without a model list.
func TestRefresherTreatsNeverFetchedAsStale(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"llama3"}

	created := f.mustCreate(t, f.scenario.orgA, "Brand new", "openai_compatible", "https://new.example.com/v1", "sk-tes...0000")

	refresher := providerreg.NewModelRefresher(f.svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if refreshed := refresher.RefreshOnce(context.Background()); refreshed != 1 {
		t.Fatalf("refreshed %d providers, want 1 (a never-fetched list is stale)", refreshed)
	}
	if got := f.repo.models(f.scenario.orgA, created.ID); len(got) != 1 {
		t.Fatalf("models = %v, want the fetched list", got)
	}
}

// TestRefresherKeepsGoingAfterAFailure: one broken provider must not stop the
// rest of the batch. The failing upstream is the ordinary case — a provider the
// operator has not finished configuring — so the loop's job is to continue.
func TestRefresherKeepsGoingAfterAFailure(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"m"}

	f.mustCreate(t, f.scenario.orgA, "A", "openai_compatible", "https://a.example.com/v1", "sk-tes...0000")
	f.mustCreate(t, f.scenario.orgA, "B", "openai_compatible", "https://b.example.com/v1", "sk-tes...0000")

	// Every fetch fails.
	f.probe.modelsErr = errors.New("upstream refused")

	refresher := providerreg.NewModelRefresher(f.svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if refreshed := refresher.RefreshOnce(context.Background()); refreshed != 0 {
		t.Fatalf("refreshed %d providers, want 0 when every upstream fails", refreshed)
	}
	// Both were attempted, not just the first: that is the property under test.
	if listCalls, _ := f.calls.snapshot(); listCalls != 2 {
		t.Fatalf("upstream was called %d times, want 2 (the loop must not stop at the first failure)", listCalls)
	}
}

// TestRefresherIsNotTriggeredByARead is the authority argument, asserted rather
// than described. Refreshing on read would let a Viewer — whose floor on GET
// /providers is Viewer — spend the workspace's credential against the operator's
// upstream. The refresher runs as nobody instead, so a Viewer's GET must produce
// zero upstream traffic.
func TestRefresherIsNotTriggeredByARead(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"m"}
	f.mustCreate(t, f.scenario.orgA, "P", "openai_compatible", "https://p.example.com/v1", "sk-tes...0000")

	// vera is a Viewer in orgA.
	if recorder := f.doProvider(t, http.MethodGet, "/api/v1/providers", "", "vera", f.scenario.orgA); recorder.Code != http.StatusOK {
		t.Fatalf("viewer list: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	listCalls, probeCalls := f.calls.snapshot()
	if listCalls != 0 {
		t.Fatalf("a viewer's read caused %d upstream model fetches, want 0", listCalls)
	}
	if probeCalls != 0 {
		t.Fatalf("a viewer's read caused %d upstream inference probes, want 0", probeCalls)
	}
}

// TestRefresherRunsAPassBeforeTheFirstTick: a provider that has never been fetched
// is stale from the moment it is created, so the loop must look before it waits.
// Without the up-front pass a workspace that registers a provider just after a
// tick waits a full interval for its first model list.
func TestRefresherRunsAPassBeforeTheFirstTick(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"m"}
	f.mustCreate(t, f.scenario.orgA, "P", "openai_compatible", "https://p.example.com/v1", "sk-tes...0000")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refresher := providerreg.NewModelRefresher(f.svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	go func() {
		defer close(done)
		refresher.Run(ctx)
	}()

	// The interval is 15 minutes, so a call inside the first second can only come
	// from the up-front pass.
	deadline := time.After(3 * time.Second)
	for {
		if listCalls, _ := f.calls.snapshot(); listCalls > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Run did not perform a pass before the first tick")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestProviderInUseErrorIsTheSentinel keeps the error contract honest: the
// service returns a value that satisfies errors.Is(err, ErrProviderInUse), so a
// caller can branch on the sentinel and read the agents only when it needs them.
func TestProviderInUseErrorIsTheSentinel(t *testing.T) {
	err := error(&providerreg.InUseError{Agents: []providerreg.AgentRef{{ID: "a", Name: "A"}}})
	if !errors.Is(err, providerreg.ErrProviderInUse) {
		t.Fatal("InUseError does not satisfy errors.Is(err, ErrProviderInUse)")
	}
	var target *providerreg.InUseError
	if !errors.As(err, &target) {
		t.Fatal("InUseError is not reachable through errors.As")
	}
	if len(target.Agents) != 1 || target.Agents[0].Name != "A" {
		t.Fatalf("errors.As lost the agents: %+v", target.Agents)
	}
}

// ---- AC7: the model list ----------------------------------------------------

// TestProviderModelRefreshStoresTheFetchedList is AC7's manual path. It asserts
// the list came from the upstream *and* that the timestamp moved, because a
// response showing models while models_fetched_at stayed null would leave the
// freshness check unable to tell a fetch from a cache.
func TestProviderModelRefreshStoresTheFetchedList(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"gpt-4o-mini", "llama3.1:8b"}

	created := f.mustCreate(t, f.scenario.orgA, "Local", "openai_compatible", "https://local.example.com/v1", "sk-tes...0000")
	if created.ModelsFetchedAt != nil {
		t.Fatalf("a freshly created provider reports models_fetched_at = %v, want null (never fetched)", *created.ModelsFetchedAt)
	}

	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh models: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	got := decodeProvider(t, recorder)
	if len(got.Models) != 2 || got.Models[0] != "gpt-4o-mini" {
		t.Fatalf("models = %v, want the two the upstream advertised", got.Models)
	}
	if got.ModelsFetchedAt == nil {
		t.Fatal("models_fetched_at is still null after a successful fetch")
	}
	if got.HasKey != true {
		t.Fatal("refreshing models dropped the credential flag")
	}

	// The stored row is what the next read serves, not just this response.
	read := decodeProvider(t, f.doProvider(t, http.MethodGet, "/api/v1/providers/"+created.ID, "", "alice", f.scenario.orgA))
	if len(read.Models) != 2 || read.ModelsFetchedAt == nil {
		t.Fatalf("GET after refresh = %+v, want the fetched list and its timestamp", read)
	}
}

// TestProviderModelRefreshSendsTheStoredCredential proves the probe runs with the
// credential the operator saved, decrypted — not with an empty string and not
// with the ciphertext. A refresh that sent nothing would be a silent 401 on
// every real provider.
func TestProviderModelRefreshSendsTheStoredCredential(t *testing.T) {
	f := newProviderFixture(t)
	const apiKey = "sk-tes...cdef"
	created := f.mustCreate(t, f.scenario.orgA, "Keyed", "openai_compatible", "https://keyed.example.com/v1", apiKey)

	if recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA); recorder.Code != http.StatusOK {
		t.Fatalf("refresh: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}

	f.calls.mu.Lock()
	defer f.calls.mu.Unlock()
	if f.calls.lastKey != apiKey {
		t.Fatalf("upstream received key %q, want the stored plaintext credential", f.calls.lastKey)
	}
	if f.calls.lastBaseURL != "https://keyed.example.com/v1" {
		t.Fatalf("upstream received base URL %q, want the provider's address", f.calls.lastBaseURL)
	}
}

// TestProviderModelRefreshFailureIs502 is AC7's failure path. An upstream that
// refuses is not the caller's mistake: the body was well-formed and the id
// exists. 502 keeps it distinct from a 400, which the next test pins.
func TestProviderModelRefreshFailureIs502(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.modelsErr = errors.New("provider: local.example.com returned HTTP 500")

	created := f.mustCreate(t, f.scenario.orgA, "Broken", "openai_compatible", "https://broken.example.com/v1", "sk-tes...0000")
	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("failed refresh: status = %d, want 502 (%s)", recorder.Code, recorder.Body.String())
	}

	// A failed fetch must not stamp the timestamp: a null is what tells the UI
	// to offer the button again.
	read := decodeProvider(t, f.doProvider(t, http.MethodGet, "/api/v1/providers/"+created.ID, "", "alice", f.scenario.orgA))
	if read.ModelsFetchedAt != nil {
		t.Fatalf("a failed fetch stamped models_fetched_at = %v", *read.ModelsFetchedAt)
	}
}

// TestProviderModelRefreshUnknownIs404 keeps the tenant boundary invisible: a
// provider of another workspace is answered exactly like one that does not
// exist, and the upstream is never contacted (US-AD07).
func TestProviderModelRefreshUnknownIs404(t *testing.T) {
	f := newProviderFixture(t)
	created := f.mustCreate(t, f.scenario.orgA, "Acme only", "openai_compatible", "https://acme.example.com/v1", "sk-tes...0000")

	before, _ := f.calls.snapshot()
	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "bella", f.scenario.orgB)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("foreign provider refresh: status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}
	after, _ := f.calls.snapshot()
	if after != before {
		t.Fatal("a 404 refresh still called the upstream")
	}
}

// TestModelsStaleTreatsNeverFetchedAsStale is AC7's automatic half at the unit
// level. The distinction it pins is the one that matters: null means "never
// fetched", so it is stale. Reading null as "nothing to refresh" would leave a
// provider that has never been polled permanently empty.
func TestModelsStaleTreatsNeverFetchedAsStale(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time {
		t := now.Add(-d)
		return &t
	}

	cases := []struct {
		name      string
		fetchedAt *time.Time
		wantStale bool
	}{
		{"never fetched", nil, true},
		{"fetched one hour ago", at(time.Hour), false},
		{"fetched 23 hours ago", at(23 * time.Hour), false},
		{"fetched exactly 24 hours ago", at(providerreg.ModelsStaleAfter), false},
		{"fetched 25 hours ago", at(25 * time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := providerreg.ModelsStale(providerreg.Provider{ModelsFetchedAt: tc.fetchedAt}, now)
			if got != tc.wantStale {
				t.Fatalf("ModelsStale(%s) = %v, want %v", tc.name, got, tc.wantStale)
			}
		})
	}
}

// ---- AC3: the inference probe ----------------------------------------------

// TestProviderVerifyUsesInferenceNotTheModelList is AC3, and it is the test the
// whole endpoint exists for.
//
// DECISIONS 6A.J measured a gateway that answers /models with no credential at
// all and 401 on inference. So a verify that only listed models would report a
// good key as verified and a wrong one as verified too. The assertion is
// therefore negative as well as positive: the probe must have called
// ProbeInference, and the stored list must not be what decided the answer.
func TestProviderVerifyUsesInferenceNotTheModelList(t *testing.T) {
	f := newProviderFixture(t)
	created := f.mustCreate(t, f.scenario.orgA, "Probe me", "openai_compatible", "https://probe.example.com/v1", "sk-tes...0000")

	// A verify with no model list yet is refused before any upstream call: the
	// probe has to name a model, and inventing one would test a model the
	// operator does not serve.
	before, _ := f.calls.snapshot()
	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/verify", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("verify without a model list: status = %d, want 400 (%s)", recorder.Code, recorder.Body.String())
	}
	after, _ := f.calls.snapshot()
	if after != before {
		t.Fatal("verify called the upstream despite having no model list to probe with")
	}

	// With a list, the probe runs and names a model from that list.
	f.probe.models = []string{"gpt-4o-mini"}
	if recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA); recorder.Code != http.StatusOK {
		t.Fatalf("refresh: status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	_, probesBefore := f.calls.snapshot()
	recorder = f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/verify", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify: status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	_, probesAfter := f.calls.snapshot()
	if probesAfter != probesBefore+1 {
		t.Fatalf("verify made %d inference probes, want exactly 1", probesAfter-probesBefore)
	}

	f.calls.mu.Lock()
	model := f.calls.lastModel
	f.calls.mu.Unlock()
	if model != "gpt-4o-mini" {
		t.Fatalf("probe used model %q, want a name from the provider's own list", model)
	}

	got := decodeProvider(t, recorder)
	if got.LastVerifiedAt == nil {
		t.Fatal("a passing probe did not stamp last_verified_at")
	}
}

// TestProviderVerifyFailureIs502AndLeavesNoStamp is AC3's failure path: a
// credential the upstream rejects must not produce a "verified" badge. The
// timestamp stays null, which is what makes the badge a fact rather than a
// decoration.
func TestProviderVerifyFailureIs502AndLeavesNoStamp(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"gpt-4o-mini"}
	created := f.mustCreate(t, f.scenario.orgA, "Bad key", "openai_compatible", "https://bad.example.com/v1", "sk-wrong")
	if recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA); recorder.Code != http.StatusOK {
		t.Fatalf("refresh: status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	f.probe.probeErr = errors.New("provider: bad.example.com rejected the inference probe with HTTP 401")
	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/verify", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("failed verify: status = %d, want 502 (%s)", recorder.Code, recorder.Body.String())
	}

	read := decodeProvider(t, f.doProvider(t, http.MethodGet, "/api/v1/providers/"+created.ID, "", "alice", f.scenario.orgA))
	if read.LastVerifiedAt != nil {
		t.Fatalf("a failed probe stamped last_verified_at = %v", *read.LastVerifiedAt)
	}
}

// TestProviderVerifyWithoutCredentialIs400 pins the refusal to probe a provider
// that stores no key. Reporting success would be a badge backed by no evidence,
// and reporting a 502 would blame the upstream for a state it never saw.
func TestProviderVerifyWithoutCredentialIs400(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"llama3.1:8b"}
	// No api_key: a local endpoint that checks nothing.
	created := f.mustCreate(t, f.scenario.orgA, "No key", "openai_compatible", "https://nokey.example.com/v1", "")
	if created.HasKey {
		t.Fatal("a provider created without a key reports has_key = true")
	}
	if recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/models", "", "alice", f.scenario.orgA); recorder.Code != http.StatusOK {
		t.Fatalf("refresh: status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	before, _ := f.calls.snapshot()
	recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+"/verify", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("verify without a credential: status = %d, want 400 (%s)", recorder.Code, recorder.Body.String())
	}
	after, _ := f.calls.snapshot()
	if after != before {
		t.Fatal("verify called the upstream for a provider with no credential")
	}
}

// ---- AC8: the two new routes obey the same floors ---------------------------

// TestProviderPhase3RoutesRequireAdmin is AC8 applied to the endpoints phase 3
// adds. They spend the workspace's credential and its quota, so the floor is the
// same as the writes: member and viewer are refused at the gate.
func TestProviderPhase3RoutesRequireAdmin(t *testing.T) {
	f := newProviderFixture(t)
	f.probe.models = []string{"gpt-4o-mini"}
	created := f.mustCreate(t, f.scenario.orgA, "Floors", "openai_compatible", "https://floors.example.com/v1", "sk-tes...0000")

	for _, path := range []string{"/verify", "/models"} {
		for _, actor := range []string{"marta", "vera"} {
			before, _ := f.calls.snapshot()
			recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+path, "", actor, f.scenario.orgA)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("%s as %s: status = %d, want 403 (%s)", path, actor, recorder.Code, recorder.Body.String())
			}
			after, _ := f.calls.snapshot()
			if after != before {
				t.Fatalf("%s as %s reached the upstream despite being refused", path, actor)
			}
		}
		// An unauthenticated call is refused too, and never dials.
		before, _ := f.calls.snapshot()
		recorder := f.doProvider(t, http.MethodPost, "/api/v1/providers/"+created.ID+path, "", "", f.scenario.orgA)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s anonymous: status = %d, want 401 (%s)", path, recorder.Code, recorder.Body.String())
		}
		after, _ := f.calls.snapshot()
		if after != before {
			t.Fatalf("%s anonymous reached the upstream", path)
		}
	}
}
