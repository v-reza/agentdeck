package main

// US-AD86 — provider credential endpoints. Every acceptance criterion that is
// in scope for this workstream, driven through the real mux:
//
//	AC1: the key is stored sealed (nonce||ciphertext||tag), never in the clear.
//	AC2: owner/admin may write it; member and viewer get 403.
//	AC3: an unknown provider is a 400.
//	AC4: an update overwrites the ciphertext; no credential history exists.
//
// Plus the two things the ACs imply but do not spell out: the plaintext must not
// appear in any response body, and `has_provider_key` must report the truth
// rather than the hardcoded false it used to.
//
// The mux comes from registerAgentCredentialRoutes — the same function main.go
// calls — so the role floor asserted here is the production one. A copy of the
// route table in this file would keep passing after main.go was loosened.
//
// The repository is fakeBoardRepo, extended with the three credential methods.
// It stores the sealed bytes verbatim, so "the database holds ciphertext" is
// asserted against what the handler actually passed down, not against a
// re-derivation inside the fake.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/board"
	"agentdeck/internal/crypto"
)

// testMasterKeyHex is a fixed 32-byte key in hex, i.e. what an operator would put
// in AGENTDECK_MASTER_KEY. It is a test constant, not a secret: nothing here ever
// runs against a real deployment.
const testMasterKeyHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// credentialFixture is the agent fixture plus the credential routes, which is the
// wiring main.go has. The scenario is created once (newRBACTestAPI mints real
// sessions), so a second call would build an unrelated tenant.
type credentialFixture struct {
	updateFixture
	key []byte
}

func newCredentialFixture(t *testing.T) credentialFixture {
	t.Helper()
	return newCredentialFixtureWithKey(t, testMasterKeyHex)
}

// newCredentialFixtureWithKey lets a test pick the master key *before* the routes
// are registered. That order is forced, not stylistic: registerAgentCredentialRoutes
// captures the key into the handler at registration time, so a test that assigns
// `masterKey` afterwards is ignored, and re-registering to pick it up panics on a
// duplicate ServeMux pattern.
func newCredentialFixtureWithKey(t *testing.T, keyRaw string) credentialFixture {
	t.Helper()
	f := newUpdateFixture(t)
	var key []byte
	if keyRaw != "" {
		k, err := crypto.LoadKey(keyRaw)
		if err != nil {
			t.Fatalf("load test master key: %v", err)
		}
		key = k
	}
	f.scenario.api.masterKey = keyRaw
	registerAgentCredentialRoutes(f.mux, f.scenario.api, board.NewService(f.repo), nil)
	return credentialFixture{updateFixture: f, key: key}
}

// do issues a request with a body through the real credential routes.
func (f credentialFixture) credential(t *testing.T, method, path, actor, orgID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

func (f credentialFixture) putKey(t *testing.T, actor, orgID, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.credential(t, http.MethodPut, "/api/v1/agents/"+agentID+"/provider-key", actor, orgID, body)
}

func (f credentialFixture) deleteKey(t *testing.T, actor, orgID, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	return f.credential(t, http.MethodDelete, "/api/v1/agents/"+agentID+"/provider-key", actor, orgID, "")
}

func (f credentialFixture) validate(t *testing.T, actor, orgID, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	return f.credential(t, http.MethodPost, "/api/v1/agents/"+agentID+"/validate", actor, orgID, "")
}

// storedCiphertext is the sealed bytes the fake repository was handed.
func (f credentialFixture) storedCiphertext(t *testing.T, agentID string) []byte {
	t.Helper()
	return f.repo.providerKeys[agentID]
}

// createBYOAgent registers an openai_compatible agent with a base URL, which is
// the only shape whose provider the credential endpoints can act on.
func (f credentialFixture) createBYOAgent(t *testing.T, name string) agentResponse {
	t.Helper()
	return f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"`+name+`","provider":"openai_compatible","model":"gpt-4o","base_url":"https://byo.test/v1"}`)
}

// TestPutProviderKeySealsAndReportsPresence is US-AD86 AC1 + AC4 in one path: the
// stored bytes are not the plaintext, they decrypt back to it, and the response
// says the agent now has a key.
func TestPutProviderKeySealsAndReportsPresence(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-sealed")
	const plaintext = "sk-live-abcdef0123456789"

	if got := f.repo.agents[agent.ID].HasProviderKey; got {
		t.Fatalf("agent reports a key before any PUT: has_provider_key = %v", got)
	}

	w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"`+plaintext+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT provider-key want 200, got %d — %s", w.Code, w.Body.String())
	}

	var resp providerKeyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !resp.HasProviderKey {
		t.Errorf("AC1: has_provider_key = false after a successful PUT")
	}

	sealed := f.storedCiphertext(t, agent.ID)
	if len(sealed) == 0 {
		t.Fatal("AC1: nothing was persisted to provider_api_key_enc")
	}
	// The whole point of the column: a database dump must not yield the key.
	if bytes.Contains(sealed, []byte(plaintext)) {
		t.Fatal("AC1: stored bytes contain the plaintext API key")
	}
	opened, err := crypto.Open(f.key, sealed)
	if err != nil {
		t.Fatalf("AC1: stored value does not decrypt: %v", err)
	}
	if opened != plaintext {
		t.Errorf("AC1: decrypted %q, want the key that was sent", opened)
	}
	// nonce(12) + at least one byte of ciphertext + tag(16).
	if len(sealed) < 12+1+16 {
		t.Errorf("AC1: sealed value is %d bytes, want at least nonce+ct+tag = 29", len(sealed))
	}

	// And the flag is readable through the ordinary agent read, which is where
	// the registry gets it from.
	got := decodeAgent(t, f.credential(t, http.MethodGet, "/api/v1/agents/"+agent.ID, "vera", f.scenario.orgA, ""))
	if !got.HasProviderKey {
		t.Errorf("GET /agents/{id} still reports has_provider_key = false after a PUT")
	}
}

// TestPutProviderKeyRefusesMemberAndViewer is US-AD86 AC2. Both denied roles are
// asserted: a gate that admitted member would still refuse viewer, and the AC
// names both.
func TestPutProviderKeyRefusesMemberAndViewer(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-roles")

	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false}, // owner
		{actor: "andre", wantDenied: false}, // admin
		{actor: "marta", wantDenied: true},  // member
		{actor: "vera", wantDenied: true},   // viewer
	}
	for _, tc := range cases {
		t.Run(tc.actor, func(t *testing.T) {
			// Snapshot before the attempt. Asserting "nothing is stored" would be
			// order-dependent: the owner subtest above legitimately stores a key,
			// so a later refused writer would see someone else's ciphertext and
			// fail a gate that actually held. What AC2 requires is that a refused
			// write changes nothing.
			before := f.storedCiphertext(t, agent.ID)
			w := f.putKey(t, tc.actor, f.scenario.orgA, agent.ID, `{"api_key":"sk-role-test-0000"}`)
			if tc.wantDenied {
				if w.Code != http.StatusForbidden {
					t.Fatalf("AC2: %s storing a credential: status = %d, want 403", tc.actor, w.Code)
				}
				if !bytes.Equal(f.storedCiphertext(t, agent.ID), before) {
					t.Fatalf("AC2: %s was refused but the stored credential changed", tc.actor)
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("%s storing a credential: status = %d, want 200 — %s", tc.actor, w.Code, w.Body.String())
			}
		})
	}
}

// TestDeleteProviderKeyRefusesMemberAndViewer is the same floor on revocation.
// AC2 says "read or update the credential"; removing it is the strongest update
// there is, so it cannot sit behind a weaker gate.
func TestDeleteProviderKeyRefusesMemberAndViewer(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-revoke")
	if w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"sk-revoke-test-000"}`); w.Code != http.StatusOK {
		t.Fatalf("setup PUT: %d — %s", w.Code, w.Body.String())
	}

	for _, actor := range []string{"marta", "vera"} {
		t.Run(actor, func(t *testing.T) {
			w := f.deleteKey(t, actor, f.scenario.orgA, agent.ID)
			if w.Code != http.StatusForbidden {
				t.Fatalf("AC2: %s revoking a credential: status = %d, want 403", actor, w.Code)
			}
			if len(f.storedCiphertext(t, agent.ID)) == 0 {
				t.Fatal("AC2: a refused DELETE still cleared the credential")
			}
		})
	}
}

// TestPutProviderKeyRejectsUnknownProvider is US-AD86 AC3.
//
// The provider is taken from the stored agent, so the way to exercise this is an
// agent whose provider the price table does not know. The fake repository accepts
// it (the CHECK is Postgres's job); the endpoint is what must refuse.
func TestPutProviderKeyRejectsUnknownProvider(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-unknown-provider")

	// Seed an agent row the way a hand-edited database or an older schema would
	// have it: a provider id that is in no price table.
	stored := f.repo.agents[agent.ID]
	stored.Provider = "totally-made-up-provider"
	f.repo.agents[agent.ID] = stored

	w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"sk-unknown-provider"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("AC3: want 400, got %d — %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("totally-made-up-provider")) {
		t.Errorf("AC3: 400 body does not name the provider: %s", w.Body.String())
	}
	if len(f.storedCiphertext(t, agent.ID)) != 0 {
		t.Fatal("AC3: an unknown provider was rejected but the credential was stored anyway")
	}
}

// TestPutProviderKeyAcceptsKnownProviders is the accepting half of AC3: every
// protocol the schema declares must pass.
//
// It used to accept the 18 *vendor* ids `pricing.Providers()` knows — `openai`,
// `deepseek`, `qwen` — which is a different vocabulary from the one
// `agents.provider` holds (US-AD109 AC6: the value is derived from
// `providers.protocol`). That test was not merely stale, it was the thing
// keeping the mismatch in place: it asserted the wrong answer, so the validator
// could not be corrected without a failure that looked like a regression.
// DECISIONS 6A.J records the mismatch; the vendor half now lives in
// `TestValidateProviderAcceptsOnlyProtocols`, which asserts the refusal.
func TestPutProviderKeyAcceptsKnownProviders(t *testing.T) {
	for _, protocol := range []string{"openai_compatible", "anthropic", "google"} {
		t.Run(protocol, func(t *testing.T) {
			if err := board.ValidateProvider(protocol); err != nil {
				t.Fatalf("protocol %q the schema declares was rejected: %v", protocol, err)
			}
		})
	}
	if err := board.ValidateProvider("not-a-provider"); err == nil {
		t.Fatal("ValidateProvider accepted an id that is in no vocabulary at all")
	}
}

// TestDeleteProviderKeyClearsTheColumn is the revoke half of AC2 + AC4: the flag
// goes back to false and the bytes are gone.
func TestDeleteProviderKeyClearsTheColumn(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-cleared")
	if w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"sk-clear-me-123456"}`); w.Code != http.StatusOK {
		t.Fatalf("setup PUT: %d — %s", w.Code, w.Body.String())
	}

	w := f.deleteKey(t, "alice", f.scenario.orgA, agent.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE provider-key want 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp providerKeyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if resp.HasProviderKey {
		t.Error("DELETE: has_provider_key still true")
	}
	if len(f.storedCiphertext(t, agent.ID)) != 0 {
		t.Error("DELETE: provider_api_key_enc is not NULL")
	}

	got := decodeAgent(t, f.credential(t, http.MethodGet, "/api/v1/agents/"+agent.ID, "vera", f.scenario.orgA, ""))
	if got.HasProviderKey {
		t.Error("DELETE: GET /agents/{id} still reports has_provider_key = true")
	}
}

// TestProviderKeyRotationOverwritesCiphertext is US-AD86 AC4. Two PUTs of
// different keys must leave one ciphertext, different bytes each time (the nonce
// is random per Seal), and only the newest plaintext must open.
//
// The "different bytes" assertion is the load-bearing one: a store that
// appended, or that reused a nonce, would produce identical bytes and this would
// catch it.
func TestProviderKeyRotationOverwritesCiphertext(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-rotate")
	const first, second = "sk-first-000000000000", "sk-second-11111111111"

	if w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"`+first+`"}`); w.Code != http.StatusOK {
		t.Fatalf("first PUT: %d — %s", w.Code, w.Body.String())
	}
	before := append([]byte(nil), f.storedCiphertext(t, agent.ID)...)

	if w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"`+second+`"}`); w.Code != http.StatusOK {
		t.Fatalf("second PUT: %d — %s", w.Code, w.Body.String())
	}
	after := f.storedCiphertext(t, agent.ID)

	if bytes.Equal(before, after) {
		t.Error("AC4: the ciphertext is byte-identical after a rotation; the nonce is not fresh or the write was skipped")
	}
	opened, err := crypto.Open(f.key, after)
	if err != nil {
		t.Fatalf("AC4: rotated value does not decrypt: %v", err)
	}
	if opened != second {
		t.Errorf("AC4: decrypted %q, want the newest key", opened)
	}
	if opened == first {
		t.Error("AC4: the previous key is still recoverable")
	}
}

// TestProviderKeyNeverAppearsInAResponse is AC1's other half: the key is
// write-only. Every response the three endpoints can produce is checked, because
// a leak through the one path nobody asserted is exactly how this fails.
func TestProviderKeyNeverAppearsInAResponse(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-secret")
	const plaintext = "sk-must-never-echo-9876543210"

	responses := map[string]*httptest.ResponseRecorder{
		"PUT":    f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"`+plaintext+`"}`),
		"GET":    f.credential(t, http.MethodGet, "/api/v1/agents/"+agent.ID, "vera", f.scenario.orgA, ""),
		"LIST":   f.credential(t, http.MethodGet, "/api/v1/projects/"+f.projectID+"/agents", "vera", f.scenario.orgA, ""),
		"DELETE": f.deleteKey(t, "alice", f.scenario.orgA, agent.ID),
	}
	for name, w := range responses {
		if bytes.Contains(w.Body.Bytes(), []byte(plaintext)) {
			t.Errorf("AC1: the plaintext API key appears in the %s response body: %s", name, w.Body.String())
		}
		if bytes.Contains(w.Body.Bytes(), []byte(testMasterKeyHex)) {
			t.Errorf("AC1: the master key appears in the %s response body", name)
		}
	}
}

// TestValidateProviderAllowsMember is ARCHITECTURE 6.2.7's floor for the
// handshake: Member, not Admin. A member must get past the gate — whatever the
// handler then answers — because the diagnostic is not a credential write.
//
// The agent here is BYO with no stored key, so the handler stops at "no stored
// credential" (400). That is the point: the status proves the gate let a member
// through, and it is asserted to be anything but 403.
func TestValidateProviderAllowsMember(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-probe-member")

	for _, actor := range []string{"marta", "alice", "andre"} {
		t.Run(actor, func(t *testing.T) {
			w := f.validate(t, actor, f.scenario.orgA, agent.ID)
			if w.Code == http.StatusForbidden {
				t.Fatalf("member-level handshake refused %s with 403", actor)
			}
			if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
				t.Fatalf("route is not mounted: %d", w.Code)
			}
		})
	}
}

// TestValidateProviderRefusesViewer is the other side of the same floor: viewer
// is below Member, so it is 403 at the gate.
func TestValidateProviderRefusesViewer(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-probe-viewer")

	if w := f.validate(t, "vera", f.scenario.orgA, agent.ID); w.Code != http.StatusForbidden {
		t.Fatalf("viewer handshake: status = %d, want 403", w.Code)
	}
}

// TestValidateProviderWithoutStoredKeyIs400 pins the "nothing to test with" case.
// Answering 200 with ok=true would be a fabricated success, which is the failure
// mode this endpoint is most likely to have.
func TestValidateProviderWithoutStoredKeyIs400(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-probe-empty")

	w := f.validate(t, "alice", f.scenario.orgA, agent.ID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("handshake with no stored credential: status = %d, want 400 — %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatal("handshake with no stored credential reported success")
	}
}

// TestValidateProviderWithoutProviderIs400 is the phase-6 shape of the old
// "built-in provider" case.
//
// US-AD109 moved the address onto the provider row, so the agent that has no
// endpoint in this deployment is the one with no provider — a legitimate
// permanent state, not a malformed agent. The endpoint must still answer 400:
// with nothing to ping, a 200 `ok: true` would be a fabricated success.
//
// The message is asserted, not just the status. Both this case and the
// missing-credential case answer 400, so a handler that returned the *wrong*
// 400 — or one that invented a URL and only failed later on the absent key —
// would pass a status-only check. The wording is what tells them apart.
func TestValidateProviderWithoutProviderIs400(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-probe-noprovider","provider":"openai_compatible","model":"gpt-4o"}`)

	w := f.validate(t, "alice", f.scenario.orgA, agent.ID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("handshake on an agent with no provider: status = %d, want 400 — %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), board.ErrProviderNotProbeable.Error()) {
		t.Fatalf("the 400 must name the missing provider, got %q", strings.TrimSpace(w.Body.String()))
	}
}

// TestProviderKeyTenantBoundary: a credential belongs to one org. Another tenant
// must not be able to write one onto an agent it does not own, and the answer is
// 404 rather than 403 so the existence of the id is not confirmed (US-AD07).
func TestProviderKeyTenantBoundary(t *testing.T) {
	f := newCredentialFixture(t)
	agent := f.createBYOAgent(t, "agent-tenant")

	// bella owns orgB and is an admin there, so the role gate admits her.
	w := f.putKey(t, "bella", f.scenario.orgB, agent.ID, `{"api_key":"sk-cross-tenant-0000"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant PUT: status = %d, want 404 — %s", w.Code, w.Body.String())
	}
	if len(f.storedCiphertext(t, agent.ID)) != 0 {
		t.Fatal("cross-tenant PUT persisted a credential")
	}
}

// TestPutProviderKeyWithoutMasterKeyIs500 is the guard that stops a misconfigured
// deployment from storing credentials in the clear. With no AGENTDECK_MASTER_KEY
// the endpoint must fail and write nothing — never fall back to plaintext.
func TestPutProviderKeyWithoutMasterKeyIs500(t *testing.T) {
	// The empty key is passed at fixture time, not assigned afterwards: the route
	// registration captures the key into the handler, so a post-hoc assignment is
	// ignored — the test would then assert against a live credential endpoint and
	// fail for the wrong reason.
	f := newCredentialFixtureWithKey(t, "")
	agent := f.createBYOAgent(t, "agent-nokey")

	w := f.putKey(t, "alice", f.scenario.orgA, agent.ID, `{"api_key":"sk-no-master-key"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("PUT without a master key: status = %d, want 500 — %s", w.Code, w.Body.String())
	}
	if len(f.storedCiphertext(t, agent.ID)) != 0 {
		t.Fatal("PUT without a master key stored something: the plaintext fallback is live")
	}
}

// TestMaskCredential pins the masked shape US-AD96 AC2 asks for: recognisable
// ends, nothing in between, and a short key fully hidden rather than half-shown.
func TestMaskCredential(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "sk-live-abcdef0123456789", want: "sk-l...6789"},
		{in: "sk-12345678", want: "sk-1...5678"},
		{in: "short", want: "*****"},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		if got := maskCredential(tc.in); got != tc.want {
			t.Errorf("maskCredential(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// The middle of a long key must never survive masking.
	const long = "sk-live-0123456789abcdefghij"
	if masked := maskCredential(long); strings.Contains(masked, "456789abc") {
		t.Errorf("masked value leaks the middle of the key: %q", masked)
	}
}

// ---- fake repository: the credential methods ---------------------------------
//
// The fake mirrors what Postgres owns for these three statements: the write is an
// overwrite (not an append), NULL is the absence of a credential, and
// has_provider_key is derived from the column rather than passed in.

func (r *fakeBoardRepo) SetAgentProviderKey(_ context.Context, id, orgID string, sealed []byte) (bool, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return false, board.ErrNotFound
	}
	r.providerKeys[id] = append([]byte(nil), sealed...)
	a.HasProviderKey = true
	r.agents[id] = a
	return true, nil
}

func (r *fakeBoardRepo) ClearAgentProviderKey(_ context.Context, id, orgID string) (bool, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return false, board.ErrNotFound
	}
	delete(r.providerKeys, id)
	a.HasProviderKey = false
	r.agents[id] = a
	return false, nil
}

func (r *fakeBoardRepo) AgentProviderKey(_ context.Context, id, orgID string) ([]byte, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return nil, board.ErrNotFound
	}
	sealed, ok := r.providerKeys[id]
	if !ok || len(sealed) == 0 {
		return nil, board.ErrNoProviderKey
	}
	return sealed, nil
}
