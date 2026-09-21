package main

// POST /api/v1/provider/models — the stateless probe behind the agent form's
// model dropdown.
//
// The form cannot use the two endpoints that already exist: PUT
// /agents/{id}/provider-key needs an agent id, and POST /agents/{id}/validate
// reads the credential back out of the database. So the request carries the base
// URL and the key, and the endpoint stores nothing at all.
//
// The mux comes from registerAgentCredentialRoutes — the same function main.go
// calls — so the role floor asserted here is the production one. A copy of the
// route table in this file would keep passing after main.go was loosened.
//
// The fake repository is fakeBoardRepo. It embeds the Repository interface
// rather than implementing every method, so any write this endpoint attempted
// would panic on a nil dereference instead of quietly succeeding: "stateless" is
// asserted against what the handler actually did, not against a promise.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// probeKey is a fake credential. Nothing in this file ever talks to a real
// provider; the value only has to be recognisable in a response body.
const probeKey = "sk-test-not-real-0000"

// modelUpstream is a stand-in for the operator's own LLM server. It counts the
// requests it receives, which is what turns "the guard refused this" into a real
// assertion: a URL that would otherwise reach this server must produce zero hits.
type modelUpstream struct {
	srv    *httptest.Server
	status int
	body   string

	hits  atomic.Int64
	mu    sync.Mutex
	auths []string
}

func newModelUpstream(t *testing.T, status int, body string) *modelUpstream {
	t.Helper()
	u := &modelUpstream{status: status, body: body}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		u.mu.Lock()
		u.auths = append(u.auths, r.Header.Get("Authorization"))
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(u.status)
		_, _ = io.WriteString(w, u.body)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

// baseURL is the OpenAI-compatible shape: the endpoint root, no /models.
func (u *modelUpstream) baseURL() string { return u.srv.URL + "/v1" }

func (u *modelUpstream) authorization() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.auths) == 0 {
		return ""
	}
	return u.auths[0]
}

// probeBody pins the wire field names. Marshalling providerModelsRequest here
// would keep passing after a rename, which is exactly the change a client would
// break on.
func probeBody(baseURL, apiKey string) string {
	return fmt.Sprintf(`{"base_url":%q,"api_key":%q}`, baseURL, apiKey)
}

func (f credentialFixture) probe(t *testing.T, actor, orgID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.credential(t, http.MethodPost, "/api/v1/provider/models", actor, orgID, body)
}

func decodeModels(t *testing.T, w *httptest.ResponseRecorder) providerModelsResponse {
	t.Helper()
	var resp providerModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return resp
}

// TestProviderModelsProbeReturnsTheAdvertisedModels is the happy path: the
// upstream's ids come back in order, under `models`, and the key travels as a
// bearer token rather than in a URL.
func TestProviderModelsProbeReturnsTheAdvertisedModels(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"m1"},{"id":"m2"}]}`)

	w := f.probe(t, "alice", f.scenario.orgA, probeBody(up.baseURL(), probeKey))
	if w.Code != http.StatusOK {
		t.Fatalf("probe want 200, got %d — %s", w.Code, w.Body.String())
	}

	got := decodeModels(t, w).Models
	if len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Fatalf("models = %v, want [m1 m2]", got)
	}
	if hit := up.authorization(); hit != "Bearer "+probeKey {
		t.Errorf("upstream saw Authorization %q, want the key as a bearer token", hit)
	}
	if bytes.Contains(w.Body.Bytes(), []byte(probeKey)) {
		t.Errorf("the API key appears in the response body: %s", w.Body.String())
	}
}

// TestProviderModelsProbeRefusesBlockedBaseURLs is the SSRF half.
//
// The third case is the load-bearing one. `[::ffff:127.0.0.1]` means loopback
// but is not one of the two hosts the operator allowlist carries, so the guard
// must refuse it — and unlike 169.254.169.254 it points at a server that is
// really listening, this very httptest one, on the very port in the URL. If the
// guard were dropped the request would land there and the hit counter would say
// so. For the other two the counter is a formality; for this one it is the
// assertion.
func TestProviderModelsProbeRefusesBlockedBaseURLs(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"m1"}]}`)
	reachable := strings.Replace(up.srv.URL, "127.0.0.1", "[::ffff:127.0.0.1]", 1)

	cases := []struct {
		name    string
		baseURL string
	}{
		{name: "link-local metadata", baseURL: "https://169.254.169.254/v1"},
		{name: "decimal loopback", baseURL: "https://2130706433/v1"},
		{name: "loopback spelling the allowlist does not carry", baseURL: reachable + "/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := f.probe(t, "alice", f.scenario.orgA, probeBody(tc.baseURL, probeKey))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("blocked base_url %q: status = %d, want 400 — %s", tc.baseURL, w.Code, w.Body.String())
			}
		})
	}

	if hits := up.hits.Load(); hits != 0 {
		t.Fatalf("a refused base_url still reached the upstream %d time(s)", hits)
	}
}

// TestProviderModelsProbeAllowsTheOperatorLoopbackHosts pins the relaxation the
// operator asked for: `localhost` and `127.0.0.1`, plain http included, because
// that is where a self-hosted inference server runs. Without a positive test here
// the relaxation could be reverted by accident and every other test would still
// pass — they only ever assert refusals.
func TestProviderModelsProbeAllowsTheOperatorLoopbackHosts(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"local-model"}]}`)

	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			baseURL := strings.Replace(up.srv.URL, "127.0.0.1", host, 1) + "/v1"
			w := f.probe(t, "alice", f.scenario.orgA, probeBody(baseURL, probeKey))
			if w.Code != http.StatusOK {
				t.Fatalf("base_url %q: status = %d, want 200 — %s", baseURL, w.Code, w.Body.String())
			}
			if got := decodeModels(t, w).Models; len(got) != 1 || got[0] != "local-model" {
				t.Fatalf("base_url %q: models = %v, want [local-model]", baseURL, got)
			}
		})
	}
}

// TestProviderModelsProbeRejectsAnEmptyRequest is the 400 half that needs no
// upstream: the request is refused before anything is dialled. The counter is
// checked on the live upstream for the api_key cases, where a missing check
// would otherwise turn into a real request.
func TestProviderModelsProbeRejectsAnEmptyRequest(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"m1"}]}`)

	cases := []struct {
		name string
		body string
	}{
		{name: "api_key missing", body: probeBody(up.baseURL(), "")},
		{name: "api_key blank", body: `{"base_url":"` + up.baseURL() + `","api_key":"   "}`},
		{name: "base_url missing", body: `{"api_key":"` + probeKey + `"}`},
		{name: "base_url blank", body: probeBody("   ", probeKey)},
		{name: "base_url without a scheme", body: probeBody("provider.test/v1", probeKey)},
		{name: "malformed body", body: `{"base_url":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := f.probe(t, "alice", f.scenario.orgA, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 — %s", w.Code, w.Body.String())
			}
		})
	}

	if hits := up.hits.Load(); hits != 0 {
		t.Fatalf("a refused request reached the upstream %d time(s)", hits)
	}
}

// TestProviderModelsProbeMapsUpstreamRefusalTo502 is the other half of the error
// contract: the endpoint was reachable and said no, which is not our failure and
// not the caller's malformed payload either. 502 Bad Gateway is the one status
// that says "the thing we called is broken" rather than "we are".
//
// The key must not survive into that body. internal/provider redacts it, and the
// assertion is here because the 502 path is the one that echoes upstream text.
func TestProviderModelsProbeMapsUpstreamRefusalTo502(t *testing.T) {
	f := newCredentialFixture(t)

	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			up := newModelUpstream(t, status, `{"error":{"message":"bad key"}}`)
			w := f.probe(t, "alice", f.scenario.orgA, probeBody(up.baseURL(), probeKey))
			if w.Code != http.StatusBadGateway {
				t.Fatalf("upstream %d: status = %d, want 502 — %s", status, w.Code, w.Body.String())
			}
			if up.hits.Load() != 1 {
				t.Errorf("upstream %d: hits = %d, want 1", status, up.hits.Load())
			}
			if bytes.Contains(w.Body.Bytes(), []byte(probeKey)) {
				t.Fatalf("the API key appears in the 502 body: %s", w.Body.String())
			}
		})
	}
}

// TestProviderModelsProbeIsStateless: the whole point of the endpoint. No agent
// row, no sealed credential, no cache — the key is used once and dropped. A
// handler that quietly called SetProviderKey, or that registered the agent as a
// side effect, would fail here and nowhere else.
func TestProviderModelsProbeIsStateless(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"m1"}]}`)

	if w := f.probe(t, "alice", f.scenario.orgA, probeBody(up.baseURL(), probeKey)); w.Code != http.StatusOK {
		t.Fatalf("probe: %d — %s", w.Code, w.Body.String())
	}

	if n := len(f.repo.agents); n != 0 {
		t.Errorf("the probe created %d agent row(s), want 0", n)
	}
	if n := len(f.repo.providerKeys); n != 0 {
		t.Errorf("the probe stored %d provider credential(s), want 0", n)
	}
}

// TestProviderModelsProbeRoleFloorIsAdmin: the endpoint takes a raw credential,
// so it sits behind the same floor as PUT/DELETE provider-key (ARCHITECTURE
// 6.2.7). member and viewer are refused at the gate, before the handler runs, and
// — the part that matters — before the upstream is touched at all.
func TestProviderModelsProbeRoleFloorIsAdmin(t *testing.T) {
	f := newCredentialFixture(t)
	up := newModelUpstream(t, http.StatusOK, `{"data":[{"id":"m1"}]}`)

	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false}, // owner
		{actor: "andre", wantDenied: false}, // admin
		{actor: "marta", wantDenied: true},  // member
		{actor: "vera", wantDenied: true},   // viewer
	}
	allowed := 0
	for _, tc := range cases {
		t.Run(tc.actor, func(t *testing.T) {
			w := f.probe(t, tc.actor, f.scenario.orgA, probeBody(up.baseURL(), probeKey))
			if tc.wantDenied {
				if w.Code != http.StatusForbidden {
					t.Fatalf("%s probing a provider: status = %d, want 403", tc.actor, w.Code)
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("%s probing a provider: status = %d, want 200 — %s", tc.actor, w.Code, w.Body.String())
			}
			allowed++
		})
	}

	if hits := up.hits.Load(); int(hits) != allowed {
		t.Fatalf("upstream saw %d request(s), want %d — a refused role reached the provider", hits, allowed)
	}
}
