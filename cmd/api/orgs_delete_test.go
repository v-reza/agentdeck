package main

// DELETE /orgs/{id} — ARCHITECTURE 6.2.4, RBAC matrix 11.3.
//
// Tiga hal yang bikin endpoint ini gampang salah, dan tiga-tiganya diuji di sini:
//
//  1. Gerbang perannya Owner. Matriks 11.3 bilang admin pun tidak boleh —
//     menghapus workspace itu satu-satunya aksi yang tidak dilimpahkan.
//  2. Idempoten (kolom "Idempotent: Ya" di 6.2.4). Request kedua harus sukses,
//     bukan 404. Ini yang bikin route-nya tidak boleh lewat orgContextMiddleware:
//     middleware itu me-resolve tenant lewat GetOrgByID, yang menyaring
//     `deleted_at IS NULL`, jadi workspace yang baru saja ditutup pemanggilnya
//     sendiri akan gagal resolve di request kedua.
//  3. Soft, bukan hard. US-AD98 AC5 memberi jendela pemulihan 30 hari, dan
//     DELETE keras membuat jendela itu mustahil. Jadi yang diperiksa adalah
//     `orgs.deleted_at` terisi — bukan barisnya hilang.

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestDeleteOrgIsOwnerOnly — matriks 11.3: owner ya, admin tidak.
func TestDeleteOrgIsOwnerOnly(t *testing.T) {
	for _, actor := range []string{"andre@x.test", "marta@x.test", "vera@x.test"} {
		test := newRBACTestAPI(t)

		resp := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", test.tokens[actor], test.orgA)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s menghapus org = %d, want 403 (body: %s)", actor, resp.StatusCode, resp.body)
		}
		// Dan workspace-nya masih hidup: 403 yang tetap menghapus lebih buruk
		// daripada 500.
		after := test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA, "", test.tokens[actor], test.orgA)
		if after.StatusCode != http.StatusOK {
			t.Fatalf("%s: org hilang setelah 403 (GET = %d)", actor, after.StatusCode)
		}
	}
}

// TestDeleteOrgClosesItForEveryone — owner boleh, dan efeknya terlihat oleh
// anggota lain: workspace berhenti resolve.
func TestDeleteOrgClosesItForEveryone(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", test.tokens["alice@x.test"], test.orgA)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("owner menghapus org = %d, want 204 (body: %s)", resp.StatusCode, resp.body)
	}

	// Anggota lain kehilangan aksesnya tanpa 403 khusus — workspace-nya sudah
	// tidak ada bagi mereka.
	gone := test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA, "", test.tokens["vera@x.test"], test.orgA)
	if gone.StatusCode != http.StatusNotFound && gone.StatusCode != http.StatusForbidden {
		t.Fatalf("setelah ditutup, GET org = %d, want 404/403", gone.StatusCode)
	}

	// Workspace tertutup tidak boleh muncul lagi di switcher (US-AD98 AC2:
	// "berhenti resolve").
	list := test.do(t, http.MethodGet, "/api/v1/orgs", "", test.tokens["alice@x.test"], "")
	var orgs []map[string]string
	if err := json.Unmarshal([]byte(list.body), &orgs); err != nil {
		t.Fatalf("decode orgs: %v", err)
	}
	for _, org := range orgs {
		if org["id"] == test.orgA {
			t.Fatal("workspace yang ditutup masih muncul di GET /orgs")
		}
	}
}

// TestDeleteOrgIsIdempotent — kolom "Idempotent: Ya" di 6.2.4.
//
// Request kedua harus sukses. Inilah yang tidak bisa dicapai lewat
// orgContextMiddleware: middleware itu membaca org hidup, jadi request kedua
// akan 404 — untuk workspace yang baru saja ditutup pemanggilnya sendiri.
func TestDeleteOrgIsIdempotent(t *testing.T) {
	test := newRBACTestAPI(t)

	first := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", test.tokens["alice@x.test"], test.orgA)
	if first.StatusCode != http.StatusNoContent {
		t.Fatalf("hapus pertama = %d, want 204 (body: %s)", first.StatusCode, first.body)
	}
	second := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", test.tokens["alice@x.test"], test.orgA)
	if second.StatusCode != http.StatusNoContent {
		t.Fatalf("hapus kedua = %d, want 204 (idempoten, bukan 404) — body: %s",
			second.StatusCode, second.body)
	}
}

// TestDeleteOrgIsSoft — US-AD98 AC5. Barisnya harus masih ada di database,
// dengan deleted_at terisi; DELETE keras membuat jendela pemulihan 30 hari
// mustahil dan karena itu melanggar AC-nya.
func TestDeleteOrgIsSoft(t *testing.T) {
	test := newRBACTestAPI(t)

	if resp := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "",
		test.tokens["alice@x.test"], test.orgA); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("hapus = %d, want 204", resp.StatusCode)
	}

	// Org-nya sudah tidak resolve lewat jalur hidup...
	if _, _, err := test.api.store.GetOrg(t.Context(), test.orgA, "alice@x.test"); err == nil {
		t.Fatal("workspace yang ditutup masih resolve lewat GetOrg")
	}
	// ...tapi barisnya masih ada dan membawa tanda tutupnya.
	closed, err := test.api.store.OrgIncludingDeleted(t.Context(), test.orgA)
	if err != nil {
		t.Fatalf("baris org hilang setelah DELETE — itu hard delete, bukan soft: %v", err)
	}
	if closed.DeletedAt == nil {
		t.Fatal("orgs.deleted_at kosong: DELETE tidak menandai apa pun")
	}
}

// TestDeleteOrgUnknownIs404 — id yang tidak ada bukan "sudah ditutup".
func TestDeleteOrgUnknownIs404(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodDelete, "/api/v1/orgs/01ZZZZZZZZZZZZZZZZZZZZZZZZZZ", "",
		test.tokens["alice@x.test"], "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("hapus org tak dikenal = %d, want 404 (body: %s)", resp.StatusCode, resp.body)
	}
}

// TestDeleteOrgRequiresASession.
func TestDeleteOrgRequiresASession(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", "", test.orgA)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("hapus tanpa sesi = %d, want 401", resp.StatusCode)
	}
}

// TestDeleteOrgForeignTenantIsForbidden — owner orgB tidak boleh menghapus orgA,
// dan jawabannya 403 (bukan 404): org-nya ada, pemanggilnya yang tidak berhak.
func TestDeleteOrgForeignTenantIsForbidden(t *testing.T) {
	test := newRBACTestAPI(t)

	resp := test.do(t, http.MethodDelete, "/api/v1/orgs/"+test.orgA, "", test.tokens["bella@x.test"], test.orgB)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("owner org lain menghapus orgA = %d, want 403 (body: %s)", resp.StatusCode, resp.body)
	}
	// orgA harus tetap hidup.
	alive := test.do(t, http.MethodGet, "/api/v1/orgs/"+test.orgA, "", test.tokens["alice@x.test"], test.orgA)
	if alive.StatusCode != http.StatusOK {
		t.Fatalf("orgA tidak selamat dari hapus lintas-tenant: GET = %d", alive.StatusCode)
	}
}
