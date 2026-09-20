package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"agentdeck/internal/auth"
)

// US-AD04 AC1 — the invite is *emailed*, not silently written to memberships.
//
// The domain tests in internal/auth already prove the row is created with the
// requested role. What they cannot prove is that the transport was asked to
// carry it: nothing in the store's signature mentions mail, so a handler that
// forgot to notify would still pass every membership test. These tests pin the
// handler half of AC1.

// inviteMailer records the invite payload and can simulate a relay failure.
type inviteMailer struct {
	to, org, role, link string
	calls               int
	err                 error
}

func (m *inviteMailer) SendPasswordReset(_ context.Context, _, _ string) error {
	return errors.New("unexpected: member invite must not send a password reset")
}

func (m *inviteMailer) SendInvite(_ context.Context, to, org, role, link string) error {
	m.calls++
	m.to, m.org, m.role, m.link = to, org, role, link
	return m.err
}

func TestAddMemberSendsInvite(t *testing.T) {
	test := newRBACTestAPI(t)
	mailer := &inviteMailer{}
	// `do` re-reads test.api on every request, so swapping the mailer here is
	// enough — no need to stand up a second server.
	test.api.mailer = mailer

	org := test.createOrg("alice@x.test", "Invite Co", "invite-co")
	test.invite("alice@x.test", org, "bob@x.test", auth.Admin)

	if mailer.calls != 1 {
		t.Fatalf("invite mails sent = %d, want exactly 1", mailer.calls)
	}
	if mailer.to != "bob@x.test" {
		t.Fatalf("invite to = %q, want the invited address", mailer.to)
	}
	if mailer.org != "Invite Co" {
		t.Fatalf("invite org = %q, want the workspace name", mailer.org)
	}
	if mailer.role != string(auth.Admin) {
		t.Fatalf("invite role = %q, want %q", mailer.role, auth.Admin)
	}
	// The link carries no token: AC1 writes the membership up front, so the
	// invite is a notice, not a grant. A tokenised link here would mean the
	// recipient has to accept before they can sign in, which is a different
	// story than the one the PRD describes.
	if mailer.link != "http://localhost:5173/login" {
		t.Fatalf("invite link = %q, want the sign-in screen", mailer.link)
	}
}

// A relay that is down must not turn a successful invite into a 5xx. The
// membership row is already committed; failing the request would tell the
// operator the invite did not happen when it did.
func TestAddMemberSurvivesMailerFailure(t *testing.T) {
	test := newRBACTestAPI(t)
	test.api.mailer = &inviteMailer{err: errors.New("relay down")}

	org := test.createOrg("alice@x.test", "Flaky Co", "flaky-co")
	resp := test.do(t, http.MethodPost, "/api/v1/orgs/"+org+"/members",
		`{"email":"carol@x.test","role":"member"}`, test.tokens["alice@x.test"], org)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("invite with dead relay: got %d, want 201 (body: %s)", resp.StatusCode, resp.body)
	}

	// And the member is really there — the 201 is not covering for a rollback.
	roster := test.members("alice@x.test", org)
	found := false
	for _, row := range roster {
		if row.Email == "carol@x.test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("carol@x.test missing from roster after a failed invite mail: %+v", roster)
	}
}

// The roster must never leak the role as a free-form string the client has to
// interpret; AC3's guard is server-side, so the response is the contract.
func TestMembersResponseShape(t *testing.T) {
	test := newRBACTestAPI(t)
	org := test.createOrg("alice@x.test", "Shape Co", "shape-co")
	test.invite("alice@x.test", org, "bob@x.test", auth.Admin)

	resp := test.do(t, http.MethodGet, "/api/v1/orgs/"+org+"/members", "", test.tokens["alice@x.test"], org)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list members: %d %s", resp.StatusCode, resp.body)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(resp.body), &rows); err != nil {
		t.Fatalf("decode roster: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("roster size = %d, want 2 (owner + admin)", len(rows))
	}
	for _, row := range rows {
		// `created_at` is the "Joined" column: the roster must carry it, and it
		// must be a parseable RFC 3339 timestamp rather than an empty string,
		// which is what a dropped field would render as.
		for _, key := range []string{"user_id", "email", "name", "role", "created_at"} {
			if _, ok := row[key]; !ok {
				t.Fatalf("roster row missing %q: %+v", key, row)
			}
		}
		stamp, _ := row["created_at"].(string)
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			t.Fatalf("created_at = %q, want RFC 3339: %v", stamp, err)
		}
	}
}
