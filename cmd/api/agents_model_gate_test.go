package main

import (
	"fmt"
	"net/http"
	"testing"
)

// US-AD67 AC1/AC2 — the provider and model an agent is registered with must be
// ones the deployment can actually price.
//
// The rule has two halves, and the second one is deliberately narrow:
//
//	AC1: `provider` must be one the price table knows. The table is the source
//	     of truth rather than a hand-written allowlist, because a provider the
//	     catalog cannot price is the same 400 the AC is about.
//	AC2: for a built-in provider the `model` must resolve to *some* price —
//	     exact entry or pattern. A name that falls through to `unpriced` is a
//	     400.
//
// `openai_compatible` (US-AD106) is exempt from AC2. Its model list comes from
// the operator's own `GET {base_url}/models` (US-AD106 AC2), so every one of
// those names is by construction absent from our catalog. Enforcing AC2 there
// would make BYO impossible while US-AD106 is a Must in the same milestone.
//
// These tests are the reason the gate exists: before it, an unknown provider
// was accepted at 201 and only surfaced later as an agent that could never be
// priced or claimed.

func TestCreateAgentRejectsUnknownProvider(t *testing.T) {
	f := newAgentFixture(t)

	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-ghost","provider":"not-a-provider","model":"gpt-4o"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("US-AD67 AC1: unknown provider want 400, got %d — %s", w.Code, w.Body.String())
	}
}

func TestCreateAgentRejectsUnpricedModel(t *testing.T) {
	f := newAgentFixture(t)

	// "totally-made-up-model" resolves to SourceUnpriced: it is in no exact
	// entry and matches no pattern. That is the AC2 case.
	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-unpriced","provider":"openai","model":"totally-made-up-model"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("US-AD67 AC2: unpriced model want 400, got %d — %s", w.Code, w.Body.String())
	}
}

// TestCreateAgentAcceptsPatternPricedModel pins the other side of AC2: a model
// with no exact catalog entry but a matching pattern is priced, so it is
// allowed. Without this, a gate that demanded an exact hit would look correct
// while silently shrinking the catalog to the 220 exact rows.
func TestCreateAgentAcceptsPatternPricedModel(t *testing.T) {
	f := newAgentFixture(t)

	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-pattern","provider":"google","model":"gemini-2.0-flash"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("pattern-priced model want 201, got %d — %s", w.Code, w.Body.String())
	}
}

// TestCreateAgentAcceptsBYOModel is US-AD106 AC2 surviving US-AD67 AC2: a BYO
// model name is never in our catalog, and registering it must still work.
func TestCreateAgentAcceptsBYOModel(t *testing.T) {
	f := newAgentFixture(t)

	w := f.postAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-byo-model","provider":"openai_compatible",`+
			`"model":"my-own-llama-70b","base_url":"https://byo.test/v1"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("US-AD106 AC2: BYO model want 201, got %d — %s", w.Code, w.Body.String())
	}
}

// TestUpdateAgentRejectsUnknownProviderAndUnpricedModel is the update half of
// US-AD67 AC1/AC2. PATCH is a full update with merge semantics, so the gate has
// to run on the merged agent: a caller who changes only the provider, or only
// the model, must be caught the same way.
func TestUpdateAgentRejectsUnknownProviderAndUnpricedModel(t *testing.T) {
	f := newUpdateFixture(t)

	// Each subtest registers its own agent. The fake repo hands back the same
	// row it was given, so a shared agent would carry the previous subtest's
	// mutation into the next one — the second case would then 400 because of
	// the first case's bad provider, and the test would pass for a reason it
	// does not claim to test.
	cases := []struct {
		name string
		body string
	}{
		{"unknown provider", `{"provider":"not-a-provider"}`},
		{"unpriced model", `{"model":"totally-made-up-model"}`},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
				fmt.Sprintf(`{"name":"agent-gate-%d","provider":"openai","model":"gpt-4o"}`, i))

			w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("US-AD67 AC1/AC2: want 400, got %d — %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestUpdateAgentAcceptsBYOModel mirrors the create case on the update path.
func TestUpdateAgentAcceptsBYOModel(t *testing.T) {
	f := newUpdateFixture(t)
	created := f.createAgent(t, "alice", f.scenario.orgA, f.projectID,
		`{"name":"agent-byo-switch","provider":"openai","model":"gpt-4o"}`)

	w := f.patchAgent(t, "alice", f.scenario.orgA, created.ID,
		`{"provider":"openai_compatible","model":"my-own-llama-70b","base_url":"https://byo.test/v1"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("US-AD106 AC2: BYO model on update want 200, got %d — %s", w.Code, w.Body.String())
	}
}
