package main

import (
	"fmt"
	"net/http"
	"testing"
)

// US-AD106 AC3 — a BYO endpoint is checked before it is stored, not only when
// the operator presses "test credential".
//
// The guard used to have exactly one caller: `POST /agents/{id}/validate`. So an
// arbitrary string could be saved as `base_url` and the mistake only surfaced
// later, at handshake time, as a 502 — an address the API had already accepted.
// These cases drive the write path itself.
//
// The local-host relaxation is the operator's decision and is asserted here in
// both directions: `localhost` and `127.0.0.1` are admitted (a self-hosted
// inference server has no other address), while every other spelling of loopback
// stays refused.
func TestAgentBaseURLIsValidatedOnWrite(t *testing.T) {
	f := newAgentFixture(t)

	rejected := []struct {
		name string
		url  string
		why  string
	}{
		{"metadata endpoint", "https://169.254.169.254/latest", "link-local"},
		{"loopback as decimal", "https://2130706433/v1", "not on the allowlist"},
		{"loopback as hex", "https://0x7f000001/v1", "not on the allowlist"},
		{"loopback ipv6", "https://[::1]:8080/v1", "not on the allowlist"},
		{"private range", "https://10.0.0.1/v1", "rfc1918"},
		{"plain http to a public host", "http://api.example.com/v1", "https only"},
		{"not a URL", "not-a-url", "unparseable"},
		{"credentials in the URL", "https://user:pass@api.example.com/v1", "embedded credentials"},
	}
	for index, tc := range rejected {
		t.Run("rejected: "+tc.name, func(t *testing.T) {
			// A unique name per case: the fixture rejects a repeated agent name
			// with 409, so a shared name would make every case after the first
			// answer 409 and the assertion below would pass for the wrong
			// reason (or fail for one).
			name := fmt.Sprintf("byo-bad-%d", index)
			body := `{"name":"` + name + `","provider":"openai_compatible","model":"any-model",` +
				`"base_url":"` + tc.url + `"}`
			w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400 for %s (%s), got %d — %s", tc.url, tc.why, w.Code, w.Body.String())
			}
		})
	}

	// The relaxation. These must be *accepted* by the guard, which is what makes
	// the refusal list above meaningful rather than a blanket ban on everything
	// that is not a well-known public host.
	allowed := []string{
		"http://localhost:11434/v1",
		"https://127.0.0.1:8080/v1",
	}
	for index, raw := range allowed {
		t.Run("allowed: "+raw, func(t *testing.T) {
			body := fmt.Sprintf(`{"name":"byo-local-%d","provider":"openai_compatible",`+
				`"model":"any-model","base_url":"%s"}`, index, raw)
			w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
			if w.Code != http.StatusCreated {
				t.Fatalf("want 201 for a local endpoint %s, got %d — %s", raw, w.Code, w.Body.String())
			}
		})
	}
}

// Saving an agent must not require a working resolver.
//
// The first version of the write-path guard called the *operator* validator,
// which resolves the host to prove it is not loopback. That made `POST /agents`
// a DNS operation: registering an agent for a host that is down, behind a VPN,
// or not deployed yet was refused — which is the ordinary state of a form the
// operator is still filling in. It also made the same body succeed or fail
// depending on what the resolver returned that second.
//
// Both names below are deliberately unresolvable or resolve to a blocked
// address. They must be *stored*, because the write path decides about an
// address, not about liveness; liveness is what the operator's explicit
// "test credential" press is for. If someone reintroduces resolution here, the
// first case turns into a 400 and this test fails.
func TestSavingAnAgentDoesNotResolveDNS(t *testing.T) {
	f := newAgentFixture(t)

	unresolvable := []struct {
		name string
		url  string
		why  string
	}{
		{
			"host that does not exist",
			"https://this-host-does-not-exist.invalid/v1",
			"a hostname is not an address until something dials it",
		},
		{
			"host that resolves to loopback",
			"https://localhost.example.com/v1",
			"resolving would make the verdict depend on live DNS",
		},
	}
	for index, tc := range unresolvable {
		t.Run("stored: "+tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"name":"byo-nodns-%d","provider":"openai_compatible",`+
				`"model":"any-model","base_url":"%s"}`, index, tc.url)
			w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID, body)
			if w.Code != http.StatusCreated {
				t.Fatalf("want 201 for %s (%s), got %d — %s: the write path must not "+
					"resolve DNS; that check belongs to POST /agents/{id}/validate",
					tc.url, tc.why, w.Code, w.Body.String())
			}
		})
	}
}
