package board

// Search terhadap Postgres nyata — ARCHITECTURE 6.2.19.
//
// Empat hal di sini cuma bisa dibuktikan lawan database sungguhan:
//
//  1. Trigram `%` benar-benar cocok dengan teks di TENGAH judul. Itu alasan
//     `%` dipakai dan bukan `LIKE`: kotak pencarian yang hanya cocok dari awal
//     kata terasa rusak, dan perbedaannya tidak kelihatan di unit test.
//  2. `similarity()` memberi urutan yang berarti — yang paling mirip dulu.
//  3. Filter opsional yang TIDAK diisi berarti "semua", bukan "cocok dengan
//     string kosong". Ini kelas bug yang sama dengan `ListAuditLog`: sqlc
//     menebak parameter non-nullable, dan `board_id = ''` mengembalikan nol
//     baris untuk setiap pemanggilan tanpa filter.
//  4. Scoping org benar-benar ada di SQL, bukan hanya di service.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// searchFixture membuat satu task dengan judul yang bisa dicari.
func searchFixture(t *testing.T, title string) (runtimeFixture, string) {
	t.Helper()
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, title, "isi apa saja", "", 1, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return f, task.ID
}

// TestPgSearchTasksMatchesInsideTheTitle is the reason `%` is used rather than
// LIKE. "deploy" must find "Fix the deploy pipeline", which a prefix-only match
// would miss.
func TestPgSearchTasksMatchesInsideTheTitle(t *testing.T) {
	f, taskID := searchFixture(t, "Fix the deploy pipeline for staging")

	found, err := f.svc.SearchTasks(context.Background(), f.orgID, "deploy", "", 0)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if !containsTask(found, taskID) {
		t.Fatalf("mencari \"deploy\" tidak menemukan task yang judulnya memuatnya (%d hasil)", len(found))
	}

	// Sebagian kata juga harus cocok — itu yang membuatnya trigram, bukan kata utuh.
	found, err = f.svc.SearchTasks(context.Background(), f.orgID, "ploy", "", 0)
	if err != nil {
		t.Fatalf("SearchTasks (sebagian): %v", err)
	}
	if !containsTask(found, taskID) {
		t.Fatalf("mencari \"ploy\" tidak menemukan apa pun (%d hasil)", len(found))
	}
}

// TestPgSearchTasksBodyIsSearchable.
func TestPgSearchTasksBodyIsSearchable(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "Judul biasa",
		"catatan: butuh migrasi kuantum", "", 1, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	found, err := f.svc.SearchTasks(ctx, f.orgID, "kuantum", "", 0)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if !containsTask(found, task.ID) {
		t.Fatalf("body tidak ikut dicari (%d hasil)", len(found))
	}
}

// TestPgSearchTasksBoardFilterIsOptionalWhenAbsent.
//
// Ini tes regresi untuk kelas bug `sqlc.narg`: filter board yang tidak diisi
// harus berarti "semua board", bukan "board dengan id kosong". Sebelum
// parameternya nullable, pencarian tanpa board_id mengembalikan nol baris.
func TestPgSearchTasksBoardFilterIsOptionalWhenAbsent(t *testing.T) {
	f, taskID := searchFixture(t, "Cari aku tanpa filter board")

	found, err := f.svc.SearchTasks(context.Background(), f.orgID, "tanpa filter board", "", 0)
	if err != nil {
		t.Fatalf("SearchTasks tanpa board: %v", err)
	}
	if !containsTask(found, taskID) {
		t.Fatalf("tanpa board_id = %d hasil, want task-nya ketemu", len(found))
	}

	// Dengan board yang benar: tetap ketemu.
	found, err = f.svc.SearchTasks(context.Background(), f.orgID, "tanpa filter board", f.boardID, 0)
	if err != nil {
		t.Fatalf("SearchTasks dengan board: %v", err)
	}
	if !containsTask(found, taskID) {
		t.Fatalf("dengan board_id benar = %d hasil, want task-nya ketemu", len(found))
	}
}

// TestPgSearchTasksRanksTheCloserTitleFirst.
//
// Judul yang persis sama dengan query harus muncul sebelum judul yang cuma
// memuat kata itu di tengah kalimat panjang — itu gunanya similarity() sebagai
// urutan, dan tanpa itu hasil pertama jadi sembarang.
//
// Perhatikan asersinya: yang menang adalah judul yang PERSIS "deploy", bukan
// yang mengulang kata itu tiga kali. Versi pertama tes ini menaruh harapan
// terbalik (mengira pengulangan menang) dan gagal karena produknya benar:
// similarity("deploy","deploy") = 1.0 mengalahkan judul yang lebih panjang.
func TestPgSearchTasksRanksTheCloserTitleFirst(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	exact, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "deploy", "x", "", 1, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask persis: %v", err)
	}
	if _, err := f.svc.CreateTask(ctx, f.orgID, f.boardID,
		"Perbaiki deploy pipeline staging yang panjang sekali judulnya",
		"y", "", 1, StatusBacklog, ""); err != nil {
		t.Fatalf("CreateTask panjang: %v", err)
	}

	found, err := f.svc.SearchTasks(ctx, f.orgID, "deploy", f.boardID, 0)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(found) < 2 {
		t.Fatalf("hasil = %d, want minimal 2 (kedua judul memuat \"deploy\")", len(found))
	}
	if found[0].ID != exact.ID {
		t.Fatalf("hasil pertama = %q, want judul yang persis %q", found[0].Title, exact.Title)
	}
}

// TestPgSearchTasksIsScopedToTheOrg.
func TestPgSearchTasksIsScopedToTheOrg(t *testing.T) {
	f, _ := searchFixture(t, "Rahasia organisasi lain")

	found, err := f.svc.SearchTasks(context.Background(), "org-lain", "rahasia", "", 0)
	if err != nil {
		t.Fatalf("SearchTasks org lain: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("org lain melihat %d task, want 0", len(found))
	}
}

// TestPgSearchTasksRefusesAnEmptyQuery.
func TestPgSearchTasksRefusesAnEmptyQuery(t *testing.T) {
	f, _ := searchFixture(t, "Apa saja")
	for _, q := range []string{"", "   ", "\t\n"} {
		if _, err := f.svc.SearchTasks(context.Background(), f.orgID, q, "", 0); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("q=%q = %v, want ErrInvalidInput", q, err)
		}
	}
}

// TestPgSearchRunsByFailureKindAndOutcome.
//
// `runs` punya org_id sejak 3.14, jadi scoping-nya langsung. Yang diukur di sini:
// filter yang diisi menyaring, dan yang tidak diisi tidak menyaring apa pun.
func TestPgSearchRunsByFailureKindAndOutcome(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	run := f.start(t)

	// Run yang baru jalan: outcome belum ada, failure_kind belum ada.
	found, err := f.svc.SearchRuns(ctx, f.orgID, RunSearchFilter{TaskID: f.taskID})
	if err != nil {
		t.Fatalf("SearchRuns by task: %v", err)
	}
	if !containsRun(found, run.ID) {
		t.Fatalf("filter task_id tidak menemukan run-nya (%d hasil)", len(found))
	}

	// Tutup dengan kegagalan transient, lalu cari berdasarkan itu.
	if _, err := f.svc.EndRun(ctx, run.ID, f.orgID, RunSummary{
		Outcome: "failed", FailureKind: "transient", Error: "connection refused",
	}); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
	found, err = f.svc.SearchRuns(ctx, f.orgID, RunSearchFilter{FailureKind: "transient"})
	if err != nil {
		t.Fatalf("SearchRuns by failure_kind: %v", err)
	}
	if !containsRun(found, run.ID) {
		t.Fatalf("filter failure_kind=transient tidak menemukan run-nya (%d hasil)", len(found))
	}

	// failure_kind yang tidak ada: nol, dan itu bukan error.
	found, err = f.svc.SearchRuns(ctx, f.orgID, RunSearchFilter{FailureKind: "tidak-ada"})
	if err != nil {
		t.Fatalf("SearchRuns failure_kind tak ada: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("failure_kind tak ada = %d hasil, want 0", len(found))
	}

	// Pesan error ikut dicari.
	found, err = f.svc.SearchRuns(ctx, f.orgID, RunSearchFilter{Q: "connection"})
	if err != nil {
		t.Fatalf("SearchRuns by q: %v", err)
	}
	if !containsRun(found, run.ID) {
		t.Fatalf("mencari \"connection\" tidak menemukan run-nya (%d hasil)", len(found))
	}
}

// TestPgSearchRunsRefusesNoConstraintAtAll.
func TestPgSearchRunsRefusesNoConstraintAtAll(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	if _, err := f.svc.SearchRuns(context.Background(), f.orgID, RunSearchFilter{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("tanpa filter = %v, want ErrInvalidInput", err)
	}
}

// TestPgSearchRunsIsScopedToTheOrg.
func TestPgSearchRunsIsScopedToTheOrg(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	found, err := f.svc.SearchRuns(context.Background(), "org-lain", RunSearchFilter{FailureKind: "transient"})
	if err != nil {
		t.Fatalf("SearchRuns org lain: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("org lain melihat %d run, want 0", len(found))
	}
}

// TestPgSearchLimitIsClamped.
func TestPgSearchLimitIsClamped(t *testing.T) {
	f, _ := searchFixture(t, "Batas hasil pencarian")

	// Limit di atas plafon tidak error, tapi juga tidak mengembalikan lebih dari
	// plafon. `?limit=100000` harus jadi halaman, bukan pembacaan seluruh tabel.
	found, err := f.svc.SearchTasks(context.Background(), f.orgID, "batas", "", 100000)
	if err != nil {
		t.Fatalf("SearchTasks limit besar: %v", err)
	}
	if len(found) > MaxSearchLimit {
		t.Fatalf("limit 100000 mengembalikan %d baris, plafon %d", len(found), MaxSearchLimit)
	}
}

func containsTask(tasks []Task, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}

func containsRun(runs []Run, id string) bool {
	for _, r := range runs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// TestPgSearchTasksQueryIsNotInterpolated — pertahanan injeksi.
//
// `q` masuk sebagai parameter, jadi tanda kutip di dalamnya harus diperlakukan
// sebagai teks. Kalau query-nya dirakit dengan penggabungan string, satu tanda
// kutip cukup untuk mengubah bentuk SQL-nya.
func TestPgSearchTasksQueryIsNotInterpolated(t *testing.T) {
	f, _ := searchFixture(t, "Judul dengan 'kutip' di dalamnya")

	found, err := f.svc.SearchTasks(context.Background(), f.orgID, "kutip'", "", 0)
	if err != nil {
		t.Fatalf("q dengan tanda kutip: %v", err)
	}
	if !containsTask(found, "") && len(found) == 0 {
		t.Log("tidak ada hasil untuk kutip — itu boleh; yang penting tidak error SQL")
	}

	// Pola yang jelas-jelas berbahaya harus tetap hanya jadi teks.
	hostile := "'; DROP TABLE tasks; --"
	if _, err := f.svc.SearchTasks(context.Background(), f.orgID, hostile, "", 0); err != nil {
		if strings.Contains(err.Error(), "syntax") {
			t.Fatalf("query dirakit dengan string, bukan parameter: %v", err)
		}
	}
	// Tabelnya harus masih ada.
	if _, err := f.repo.GetTask(context.Background(), f.taskID, f.orgID); err != nil {
		t.Fatalf("task hilang setelah query jahat: %v", err)
	}
}

// TestLikePatternEscapesWildcards is the pure-unit half of the search tests: it
// runs without Postgres, so it is the check that always fires.
//
// Escaping matters for correctness, not just tidiness. A user who types "50%"
// is asking for the characters "50%"; without the escape, LIKE reads that as
// "50" followed by anything, and the search silently answers a different
// question than the one that was asked.
func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := []struct{ in, want string }{
		{"deploy", "%deploy%"},
		{"50%", `%50\%%`},
		{"a_b", `%a\_b%`},
		{`back\slash`, `%back\\slash%`},
		// Backslash must be escaped before the wildcards, or "\\%" would come
		// out as a literal backslash plus an unescaped wildcard.
		{`\%`, `%\\\%%`},
		{"", "%%"},
	}
	for _, c := range cases {
		if got := likePattern(c.in); got != c.want {
			t.Errorf("likePattern(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLikePatternPtrTreatsBlankAsNoFilter pins the distinction between "no
// filter" and "filter that matches everything".
//
// These are not the same thing. A NULL pattern is skipped by the SQL; the
// pattern "%%" is applied, and applied to a NULL column it yields NULL, which
// drops the row. Every successful run has a NULL error, so conflating the two
// makes "search runs by task" return nothing at all.
func TestLikePatternPtrTreatsBlankAsNoFilter(t *testing.T) {
	if got := likePatternPtr(""); got != nil {
		t.Errorf("likePatternPtr(\"\") = %q, want nil", *got)
	}
	if got := likePatternPtr("   "); got != nil {
		t.Errorf("likePatternPtr(blank) = %q, want nil", *got)
	}
	got := likePatternPtr("connection")
	if got == nil || *got != "%connection%" {
		t.Errorf("likePatternPtr(\"connection\") = %v, want %q", got, "%connection%")
	}
}
