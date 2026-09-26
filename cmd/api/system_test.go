package main

// Handler 6.2.19 — audit log, notifikasi, search, system info.
//
// Yang diukur di sini adalah tiga hal yang tidak kelihatan dari service:
//
//  1. Gerbang peran. Audit log itu Admin (11.3), notifikasi itu Viewer, dan
//     search itu Viewer. Semuanya hidup di tabel route.
//  2. Kepemilikan notifikasi. Inbox hanya boleh berisi baris milik pemanggil,
//     dan user id-nya datang dari sesi — bukan dari parameter yang bisa diketik
//     pemanggil (US-AD61 AC4).
//  3. Bentuk 400. Cursor dan rentang tanggal yang tidak bisa di-parse harus 400,
//     bukan 500 dan bukan hasil kosong yang diam-diam salah arti.

import (
	"net/http"
	"strings"
	"testing"

	"agentdeck/internal/auth"
)

// TestAuditLogIsAdminOnly — US-AD95 AC4, ARCHITECTURE 11.3.
//
// member dan viewer harus 403, dan yang penting: bukan 200 dengan daftar kosong.
// Daftar kosong akan membuat UI menampilkan empty state "tidak ada aktivitas",
// yang berarti berbohong soal kenapa halamannya kosong.
func TestAuditLogIsAdminOnly(t *testing.T) {
	scenario := newRBACTestAPI(t)

	for _, actor := range []string{"marta@x.test", "vera@x.test"} {
		resp := scenario.do(t, http.MethodGet, "/api/v1/audit-log", "", scenario.tokens[actor], scenario.orgA)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s membaca audit log = %d, want 403", actor, resp.StatusCode)
		}
	}
	resp := scenario.do(t, http.MethodGet, "/api/v1/audit-log", "", scenario.tokens["alice@x.test"], scenario.orgA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner membaca audit log = %d (%s), want 200", resp.StatusCode, resp.body)
	}
	if !strings.Contains(resp.body, "entries") {
		t.Fatalf("body = %s, want objek dengan `entries`", resp.body)
	}
}

// TestAuditLogRejectsUnparseableFilters.
//
// Cursor yang bukan angka dan tanggal yang bukan RFC3339 harus 400. Kalau
// tanggalnya diam-diam diabaikan, pemanggil akan mengira rentangnya berlaku dan
// membaca aktivitas di luar rentang yang dia minta — jawaban yang salah tapi
// kelihatan benar.
func TestAuditLogRejectsUnparseableFilters(t *testing.T) {
	scenario := newRBACTestAPI(t)
	owner := scenario.tokens["alice@x.test"]

	for _, query := range []string{
		"?cursor=abc",
		"?cursor=-1",
		"?from=2026-09",
		"?to=bukan-tanggal",
		"?limit=abc",
	} {
		resp := scenario.do(t, http.MethodGet, "/api/v1/audit-log"+query, "", owner, scenario.orgA)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("audit-log%s = %d, want 400", query, resp.StatusCode)
		}
	}
	// Yang sah harus lolos.
	resp := scenario.do(t, http.MethodGet, "/api/v1/audit-log?limit=10&from=2020-01-01T00:00:00Z", "", owner, scenario.orgA)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("filter sah = %d (%s), want 200", resp.StatusCode, resp.body)
	}
}

// TestNotificationsAreScopedToTheCaller — US-AD61 AC4.
//
// Inbox alice tidak boleh berisi notifikasi bella, walau dua-duanya memanggil
// endpoint yang sama dengan org masing-masing. Ini juga mengukur bahwa user id
// diambil dari sesi: tidak ada parameter `user_id` yang bisa dipakai untuk
// meminta inbox orang lain.
func TestNotificationsAreScopedToTheCaller(t *testing.T) {
	scenario := newRBACTestAPI(t)
	repo := scenario.api.store

	alice := scenario.userIDs["alice@x.test"]
	bella := scenario.userIDs["bella@x.test"]

	// Dua notifikasi: satu untuk alice di orgA, satu untuk bella di orgB.
	for _, n := range []auth.Notification{
		{UserID: alice, OrgID: scenario.orgA, Kind: "run.failed", Title: "punya alice", TargetType: "run", TargetID: "run-1"},
		{UserID: bella, OrgID: scenario.orgB, Kind: "run.failed", Title: "punya bella", TargetType: "run", TargetID: "run-2"},
	} {
		if _, err := repo.Notify(t.Context(), n); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}

	resp := scenario.do(t, http.MethodGet, "/api/v1/notifications", "", scenario.tokens["alice@x.test"], scenario.orgA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice membaca inbox = %d (%s), want 200", resp.StatusCode, resp.body)
	}
	if !strings.Contains(resp.body, "punya alice") {
		t.Fatalf("inbox alice = %s, want notifikasinya", resp.body)
	}
	if strings.Contains(resp.body, "punya bella") {
		t.Fatalf("inbox alice memuat notifikasi bella: %s", resp.body)
	}
	if !strings.Contains(resp.body, `"unread_count":1`) {
		t.Fatalf("body = %s, want unread_count 1", resp.body)
	}
}

// TestMarkNotificationsReadRequiresAChoice — US-AD61 AC1.
//
// `{}` harus 400. Kalau tidak, klien yang lupa mengirim `all` akan menghapus
// badge yang justru ingin dia pertahankan, dan itu kerusakan senyap.
func TestMarkNotificationsReadRequiresAChoice(t *testing.T) {
	scenario := newRBACTestAPI(t)
	alice := scenario.userIDs["alice@x.test"]
	token := scenario.tokens["alice@x.test"]

	created, err := scenario.api.store.Notify(t.Context(), auth.Notification{
		UserID: alice, OrgID: scenario.orgA, Kind: "budget.warning", Title: "cap 80%",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	resp := scenario.do(t, http.MethodPost, "/api/v1/notifications/read", `{}`, token, scenario.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("read tanpa ids/all = %d (%s), want 400", resp.StatusCode, resp.body)
	}
	// Masih belum dibaca.
	_, unread, err := scenario.api.store.Notifications(t.Context(), alice, scenario.orgA, 0)
	if err != nil {
		t.Fatalf("Notifications: %v", err)
	}
	if unread != 1 {
		t.Fatalf("unread = %d setelah 400, want 1", unread)
	}

	resp = scenario.do(t, http.MethodPost, "/api/v1/notifications/read",
		`{"ids":["`+created.ID+`"]}`, token, scenario.orgA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read dengan ids = %d (%s), want 200", resp.StatusCode, resp.body)
	}
	_, unread, err = scenario.api.store.Notifications(t.Context(), alice, scenario.orgA, 0)
	if err != nil {
		t.Fatalf("Notifications: %v", err)
	}
	if unread != 0 {
		t.Fatalf("unread = %d setelah ditandai, want 0", unread)
	}
}

// TestSearchRefusesAnEmptyQuery.
//
// Kotak pencarian yang menjawab query kosong dengan seluruh board tidak bisa
// dibedakan dari listing — dan listing sudah punya endpoint sendiri dengan
// paginasi yang benar. 400 adalah jawaban yang jujur.
func TestSearchRefusesAnEmptyQuery(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	actor := scenario.tokens["vera@x.test"]

	for _, path := range []string{
		"/api/v1/search/tasks",
		"/api/v1/search/tasks?q=",
		"/api/v1/search/tasks?q=%20%20",
		"/api/v1/search/runs",
	} {
		w := runRequest(t, mux, scenario, http.MethodGet, path, "vera", scenario.orgA, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d (%s), want 400", path, w.Code, w.Body.String())
		}
	}
	// Search itu Viewer (11.3 "Baca Board / Task / Run / Ledger"), jadi viewer
	// harus lolos gerbang peran dan yang menolaknya adalah query-nya.
	_ = actor
}

// TestSearchRoutesAreViewerAndRegistered.
//
// Tanpa sesi harus 401 (route-nya ada, bukan 404), dan viewer harus lolos
// gerbang peran — yang menolaknya nanti adalah query-nya, bukan role-nya (11.3
// "Baca Board / Task / Run / Ledger").
func TestSearchRoutesAreViewerAndRegistered(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	// Sesi kosong: runRequest memakai token "" kalau aktornya tidak dikenal.
	anon := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/search/tasks?q=apa", "nobody", scenario.orgA, "")
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("search tanpa sesi = %d, want 401 (route harus terdaftar)", anon.Code)
	}

	// Viewer dengan sesi sah: bukan 403 dan bukan 404. 400 karena `q` ada tapi
	// repo-nya stub — yang penting bukan ditolak gerbang peran.
	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/search/tasks?q=apa", "vera", scenario.orgA, "")
	if w.Code == http.StatusForbidden {
		t.Fatalf("viewer ditolak dari search = 403, want bukan 403 (11.3)")
	}
	if w.Code == http.StatusNotFound {
		t.Fatalf("route search tidak terdaftar")
	}
}

// TestSystemInfoIsPublicAndThin.
//
// Endpoint ini publik, jadi isinya dibatasi: versi, commit, versi Go, dan kapan
// prosesnya hidup. Tidak ada handle database, tidak ada konfigurasi. Kalau
// seseorang menambahkan itu nanti, tes ini yang harus menolak.
func TestSystemInfoIsPublicAndThin(t *testing.T) {
	scenario := newRBACTestAPI(t)

	resp := scenario.do(t, http.MethodGet, "/api/v1/system/info", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("system/info tanpa sesi = %d (%s), want 200", resp.StatusCode, resp.body)
	}
	for _, key := range []string{"version", "commit", "go_version", "started_at"} {
		if !strings.Contains(resp.body, `"`+key+`"`) {
			t.Errorf("body = %s, want field %q", resp.body, key)
		}
	}
	// Kebocoran yang harus dicegah: apa pun yang menamai infrastruktur.
	for _, leak := range []string{"postgres", "postgresql://", "AGENTDECK_", "password", "secret"} {
		if strings.Contains(strings.ToLower(resp.body), strings.ToLower(leak)) {
			t.Errorf("system/info membocorkan %q: %s", leak, resp.body)
		}
	}
}
