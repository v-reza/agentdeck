package main

// Handler 6.2.3 — API keys.
//
// Empat hal diukur di sini, dan tiga di antaranya tidak bisa dilihat dari CRUD:
//
//  1. plaintext `adk_...` muncul SEKALI di 201, dan tidak pernah lagi — bukan
//     di daftar, bukan di detail;
//  2. key milik orang lain 404, bukan 403: keberadaannya bukan urusan pemanggil;
//  3. **bearer `adk_...` benar-benar mengautentikasi** (11.1). Ini inti
//     modulnya — tanpa ini, tabel `api_keys` cuma CRUD dan endpoint Worker
//     tetap terpaksa didaftarkan `auth.Member`;
//  4. key yang dicabut langsung mati, dan prefix-nya tidak lagi cocok.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type apiKeyJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	LastUsedAt string `json:"last_used_at"`
	RevokedAt  string `json:"revoked_at"`
	Key        string `json:"key"`
}

func decodeAPIKey(t *testing.T, resp rbacResponse) apiKeyJSON {
	t.Helper()
	var out apiKeyJSON
	if err := json.Unmarshal([]byte(resp.body), &out); err != nil {
		t.Fatalf("decode api key: %v (%s)", err, resp.body)
	}
	return out
}

// mintAPIKey creates a key for the actor and returns the one-time plaintext.
func mintAPIKey(t *testing.T, e rbacTestAPI, actor, org, name string) apiKeyJSON {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/v1/api-keys",
		`{"name":"`+name+`"}`, e.tokens[actor], org)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create api key: %d %s", resp.StatusCode, resp.body)
	}
	return decodeAPIKey(t, resp)
}

// TestAPIKeyPlaintextIsShownOnceAndNeverAgain is US-AD's shape for 6.2.3: the
// token is returned by the create call and by nothing else.
func TestAPIKeyPlaintextIsShownOnceAndNeverAgain(t *testing.T) {
	e := newRBACTestAPI(t)
	owner := "marta@x.test" // Member orgA — cukup untuk membuat key
	org := e.orgA

	created := mintAPIKey(t, e, owner, org, "ci")
	if !strings.HasPrefix(created.Key, "adk_") {
		t.Fatalf("key = %q, want an adk_ prefix", created.Key)
	}
	// Prefix yang disimpan adalah 8 karakter pertama — `adk_` + 4.
	if created.Prefix != created.Key[:8] {
		t.Fatalf("prefix = %q, want the first 8 chars of %q", created.Prefix, created.Key)
	}
	if len(created.Prefix) != 8 {
		t.Fatalf("prefix len = %d, want 8", len(created.Prefix))
	}

	// Detail dan daftar tidak boleh membawa plaintext-nya.
	detail := e.do(t, http.MethodGet, "/api/v1/api-keys/"+created.ID, "", e.tokens[owner], org)
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("get api key: %d %s", detail.StatusCode, detail.body)
	}
	if strings.Contains(detail.body, created.Key) {
		t.Fatalf("detail membocorkan plaintext: %s", detail.body)
	}
	list := e.do(t, http.MethodGet, "/api/v1/api-keys", "", e.tokens[owner], org)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list api keys: %d %s", list.StatusCode, list.body)
	}
	if strings.Contains(list.body, created.Key) {
		t.Fatalf("daftar membocorkan plaintext: %s", list.body)
	}
	// Hash-nya juga tidak: yang tersimpan SHA-256, bukan token.
	if strings.Contains(list.body, "token_hash") {
		t.Fatalf("daftar membocorkan field hash: %s", list.body)
	}
}

// TestAPIKeyOfAnotherMemberIsNotFound pins the ownership boundary: a key is the
// caller's own resource, and its existence is not disclosed to anyone else.
func TestAPIKeyOfAnotherMemberIsNotFound(t *testing.T) {
	e := newRBACTestAPI(t)
	// Keduanya anggota orgA, jadi permintaan benar-benar sampai ke handler.
	// Kalau `other` bukan anggota, middleware menolak lebih dulu dan tesnya
	// lulus karena alasan yang salah.
	owner := "marta@x.test"
	other := "andre@x.test"
	org := e.orgA

	created := mintAPIKey(t, e, owner, org, "mine")

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/api/v1/api-keys/" + created.ID
		resp := e.do(t, method, path, "", e.tokens[other], org)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s key milik orang lain = %d, want 404", method, resp.StatusCode)
		}
	}
	revoke := e.do(t, http.MethodPost, "/api/v1/api-keys/"+created.ID+"/revoke", "", e.tokens[other], org)
	if revoke.StatusCode != http.StatusNotFound {
		t.Errorf("revoke key milik orang lain = %d, want 404", revoke.StatusCode)
	}
}

// TestRevokeIsIdempotentAndKillsTheKey: revoking twice is not an error, and the
// key stops authenticating immediately (2292).
func TestRevokeIsIdempotentAndKillsTheKey(t *testing.T) {
	e := newRBACTestAPI(t)
	owner := "marta@x.test"
	org := e.orgA
	created := mintAPIKey(t, e, owner, org, "deploy")

	// Sebelum dicabut, key-nya bisa dipakai.
	before := e.do(t, http.MethodGet, "/api/v1/api-keys", "", created.Key, "")
	if before.StatusCode != http.StatusOK {
		t.Fatalf("key aktif ditolak = %d %s", before.StatusCode, before.body)
	}

	first := e.do(t, http.MethodPost, "/api/v1/api-keys/"+created.ID+"/revoke", "", e.tokens[owner], org)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("revoke pertama = %d %s", first.StatusCode, first.body)
	}
	second := e.do(t, http.MethodPost, "/api/v1/api-keys/"+created.ID+"/revoke", "", e.tokens[owner], org)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("revoke kedua = %d, want 200 (idempoten)", second.StatusCode)
	}

	after := e.do(t, http.MethodGet, "/api/v1/api-keys", "", created.Key, "")
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("key dicabut masih dipakai = %d, want 401", after.StatusCode)
	}
}

// TestAPIKeyBearerCarriesTheOwnersRole is the point of the module: a key
// inherits the role of whoever minted it (2291), and it needs no X-Org-ID
// because the key is scoped to one workspace at creation.
func TestAPIKeyBearerCarriesTheOwnersRole(t *testing.T) {
	e := newRBACTestAPI(t)
	owner := "alice@x.test" // Owner orgA: key-nya mewarisi Owner
	org := e.orgA
	created := mintAPIKey(t, e, owner, org, "sdk")

	// Tanpa X-Org-ID: workspace datang dari key, bukan dari header.
	noOrg := e.do(t, http.MethodGet, "/api/v1/api-keys", "", created.Key, "")
	if noOrg.StatusCode != http.StatusOK {
		t.Fatalf("bearer tanpa X-Org-ID = %d %s", noOrg.StatusCode, noOrg.body)
	}

	// Peran diwarisi: owner bisa membaca audit log, yang butuh Admin.
	audit := e.do(t, http.MethodGet, "/api/v1/audit-log", "", created.Key, "")
	if audit.StatusCode != http.StatusOK {
		t.Fatalf("key owner ditolak dari audit-log = %d %s", audit.StatusCode, audit.body)
	}

	// Key yang di-mint Member mewarisi Member, jadi audit log-nya 403 — bukan
	// karena key-nya salah, tapi karena peran pemiliknya memang tidak cukup.
	// Itu yang membuktikan perannya benar-benar diwarisi dan bukan dikerasin.
	memberKey := mintAPIKey(t, e, "marta@x.test", org, "member-key")
	memberAudit := e.do(t, http.MethodGet, "/api/v1/audit-log", "", memberKey.Key, "")
	if memberAudit.StatusCode != http.StatusForbidden {
		t.Fatalf("key member di audit-log = %d, want 403", memberAudit.StatusCode)
	}
	// Member tetap boleh memakai key-nya untuk hal yang memang Member-level.
	memberList := e.do(t, http.MethodGet, "/api/v1/api-keys", "", memberKey.Key, "")
	if memberList.StatusCode != http.StatusOK {
		t.Fatalf("key member di api-keys = %d, want 200", memberList.StatusCode)
	}
}

// TestViewerCannotMintAKey pins the role gate from 6.2.3's own table: Role Min
// is Member, so a viewer is refused at the middleware. This matters because a
// key inherits its owner's role — letting a viewer mint one would be a way to
// hold a credential at a role the workspace never granted them.
func TestViewerCannotMintAKey(t *testing.T) {
	e := newRBACTestAPI(t)
	resp := e.do(t, http.MethodPost, "/api/v1/api-keys",
		`{"name":"nope"}`, e.tokens["vera@x.test"], e.orgA)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer membuat key = %d, want 403", resp.StatusCode)
	}
	// Dan dia juga tidak melihat daftar key siapa pun.
	list := e.do(t, http.MethodGet, "/api/v1/api-keys", "", e.tokens["vera@x.test"], e.orgA)
	if list.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer membaca daftar key = %d, want 403", list.StatusCode)
	}
}

// TestDeletedAPIKeyStopsAuthenticating: DELETE is physical (6.2.3), so the row
// and its hash are gone rather than merely marked.
func TestDeletedAPIKeyStopsAuthenticating(t *testing.T) {
	e := newRBACTestAPI(t)
	owner := "marta@x.test"
	org := e.orgA
	created := mintAPIKey(t, e, owner, org, "temp")

	resp := e.do(t, http.MethodDelete, "/api/v1/api-keys/"+created.ID, "", e.tokens[owner], org)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d %s", resp.StatusCode, resp.body)
	}
	gone := e.do(t, http.MethodGet, "/api/v1/api-keys/"+created.ID, "", e.tokens[owner], org)
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("detail setelah hapus = %d, want 404", gone.StatusCode)
	}
	dead := e.do(t, http.MethodGet, "/api/v1/api-keys", "", created.Key, "")
	if dead.StatusCode != http.StatusUnauthorized {
		t.Fatalf("key terhapus masih dipakai = %d, want 401", dead.StatusCode)
	}
}

// TestAPIKeyNameIsRequired: an empty or blank label is a 400, not a key named "".
func TestAPIKeyNameIsRequired(t *testing.T) {
	e := newRBACTestAPI(t)
	owner := "marta@x.test"

	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{"name":"` + strings.Repeat("x", 65) + `"}`} {
		resp := e.do(t, http.MethodPost, "/api/v1/api-keys", body, e.tokens[owner], e.orgA)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("nama %q = %d, want 400", body, resp.StatusCode)
		}
	}
}

// TestAPIKeysRequireMembership: an unauthenticated call is 401, and the routes
// are registered — a 404 here would mean the mux forgot them.
func TestAPIKeysRequireMembership(t *testing.T) {
	e := newRBACTestAPI(t)

	anon := e.do(t, http.MethodGet, "/api/v1/api-keys", "", "", e.orgA)
	if anon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tanpa sesi = %d, want 401", anon.StatusCode)
	}
}
