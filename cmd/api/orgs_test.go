package main

import (
	"net/http"
	"testing"
)

// TestCreateOrgDuplicateSlugIsConflict pins the status code for a slug that is
// already taken. ARCHITECTURE 6.1 reserves 409 for a request that conflicts with
// the current state of the resource; the slug unique index (orgs_slug_key) is
// exactly that. Without an explicit mapping the domain error fell through
// writeAuthError's default arm and the API answered 400, which reads as a
// malformed request and sends the client looking for a validation bug that is
// not there.
func TestCreateOrgDuplicateSlugIsConflict(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodPost, "/api/v1/orgs",
		`{"name":"Acme Again","slug":"acme"}`, test.tokens["alice@x.test"], "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate slug: got %d, want 409 (body: %s)", resp.StatusCode, resp.body)
	}

	// A slug only collides with itself: the same request under a free slug must
	// still create the org, so the 409 above is about the conflict and not a
	// blanket rejection of the endpoint.
	fresh := test.do(t, http.MethodPost, "/api/v1/orgs",
		`{"name":"Acme Three","slug":"acme-three"}`, test.tokens["alice@x.test"], "")
	if fresh.StatusCode != http.StatusCreated {
		t.Fatalf("free slug: got %d, want 201 (body: %s)", fresh.StatusCode, fresh.body)
	}
}
