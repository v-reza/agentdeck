package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// US-AD109 AC4 — the SSRF guard runs on the provider's base URL, which is where
// the address lives now.
//
// This file used to drive `POST /agents` with a `base_url`. US-AD109 moved the
// endpoint onto the provider (DECISIONS 6A.J) and fase 6 dropped the agent's
// column, so the same rule is exercised through the route that still accepts an
// address. The guard itself never changed: `provider.ValidateAddressOnly` plus
// the two loopback spellings DECISIONS 6A.F names.
//
// The cases below are deliberately the ones the provider suite does NOT already
// carry. `TestProviderBaseURLPassesSSRFGuard` covers the link-local metadata
// endpoint, the RFC1918 ranges, `[::1]`, a non-http scheme and the empty string.
// What is unique here is the *spelling* family — a decimal or hex literal that
// resolves to loopback, a bare non-URL, a public host over plain http, and
// credentials embedded in the URL — because each of those is a way to smuggle an
// address past a guard that only looked at the obvious form.
//
// The local-host relaxation is asserted in both directions: `localhost` and
// `127.0.0.1` are admitted (a self-hosted inference server has no other
// address), while every other spelling of loopback stays refused.
func TestProviderBaseURLRejectsSmuggledLoopbackSpellings(t *testing.T) {
	f := newProviderFixture(t)

	rejected := []struct {
		name string
		url  string
		why  string
	}{
		{"loopback as decimal", "https://2130706433/v1", "not on the allowlist"},
		{"loopback as hex", "https://0x7f000001/v1", "not on the allowlist"},
		{"loopback ipv6", "https://[::1]:8080/v1", "not on the allowlist"},
		{"plain http to a public host", "http://api.example.com/v1", "https only"},
		{"not a URL", "not-a-url", "unparseable"},
		{"credentials in the URL", "https://user:pass@api.example.com/v1", "embedded credentials"},
	}
	for _, tc := range rejected {
		t.Run("rejected: "+tc.name, func(t *testing.T) {
			// A unique name per case: providers_org_name_key rejects a repeat
			// with 409, so a shared name would make every case after the first
			// fail for the wrong reason.
			payload, err := json.Marshal(map[string]string{
				"name": "SSRF-" + tc.name, "protocol": "openai_compatible", "base_url": tc.url,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			recorder := f.create(t, "alice", f.scenario.orgA, string(payload))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("want 400 for %s (%s), got %d — %s",
					tc.url, tc.why, recorder.Code, recorder.Body.String())
			}
		})
	}

	// The relaxation. These must be *accepted*, which is what makes the refusal
	// list above meaningful rather than a blanket ban on anything that is not a
	// well-known public host.
	allowed := []struct {
		name string
		url  string
	}{
		{"localhost with a port", "http://localhost:11434/v1"},
		{"loopback literal", "http://127.0.0.1:8080/v1"},
	}
	for _, tc := range allowed {
		t.Run("allowed: "+tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"name": "Local-" + tc.name, "protocol": "openai_compatible", "base_url": tc.url,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if recorder := f.create(t, "alice", f.scenario.orgA, string(payload)); recorder.Code != http.StatusCreated {
				t.Fatalf("want 201 for %s, got %d — %s", tc.url, recorder.Code, recorder.Body.String())
			}
		})
	}
}
