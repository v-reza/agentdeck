package board

// Approval gate terhadap Postgres nyata — ARCHITECTURE 3.13, 5.4, 8.3/8.4, 6.2.14.
//
// Empat hal di sini cuma bisa dibuktikan lawan database sungguhan:
//
//  1. `DecideApproval` mengandalkan predikat `decision = 'pending'` DI DALAM
//     UPDATE. Fake in-memory yang memeriksa lalu menulis akan selalu lolos;
//     yang perlu dibuktikan adalah bahwa baris yang sudah diputuskan benar-benar
//     nol baris, karena itulah yang membuat dua approver bersamaan menghasilkan
//     satu pemenang.
//  2. Tenggat N23 ditulis oleh SQL (`now() + interval '24 hours'`), bukan oleh
//     Go. Kalau klien yang mengirimnya, approval yang tidak pernah kedaluwarsa
//     bisa dibuat dengan mengirim tanggal jauh di depan.
//  3. Sapu expiry (8.3) adalah satu UPDATE dengan RETURNING, dan ia harus
//     benar-benar mengubah `decision` sekaligus memberi tahu task mana yang
//     perlu dipindah.
//  4. `block_kind` ditulis bersama statusnya (`BlockTask`), dan 5.4 menuntut
//     nilainya `policy` — bukan `needs_input` yang ditulis PRD.
//
// Gate: AGENTDECK_TEST_DATABASE_URL.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"agentdeck/internal/ulid"
)

// approvalFixture menjalankan run-nya dulu: `approvals.run_id` NOT NULL dan
// terikat FK ke `runs`, jadi gate cuma bisa dibuat di atas run yang benar-benar
// ada — persis seperti produksi.
func approvalFixture(t *testing.T) (runtimeFixture, Approval) {
	t.Helper()
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	approval, err := f.svc.RequestApproval(context.Background(), f.taskID, f.orgID,
		"worker@example.com", json.RawMessage(`{"action":"drop_table","table":"dev"}`), "")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	return f, approval
}

func TestPgRequestApprovalParksTheTaskAndTheRun(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	if approval.Decision != ApprovalPending {
		t.Fatalf("decision = %q, want pending", approval.Decision)
	}
	if approval.GateMode != ApprovalGateRequire {
		t.Fatalf("gate_mode = %q, want require (US-AD33 AC1)", approval.GateMode)
	}

	// N23: 24 jam, ditulis oleh SQL. Rentangnya diperiksa longgar karena
	// `expires_at` dihitung di server database, bukan di proses ini.
	window := time.Until(approval.ExpiresAt)
	if window < 23*time.Hour || window > 25*time.Hour {
		t.Fatalf("expires_at = %v dari sekarang, want ~24h (N23)", window)
	}

	// Task diparkir (5.4: running -> awaiting_approval).
	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusAwaitingApproval {
		t.Fatalf("status task = %q, want awaiting_approval", task.Status)
	}

	// Run-nya DITUTUP, tidak dibiarkan `running`. Run yang tetap hidup tanpa
	// executor akan di-reclaim 15 menit kemudian (N8) dan dijalankan ulang —
	// membelanjakan uang untuk pekerjaan yang belum disetujui manusia.
	run, err := f.svc.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status == RunRunning {
		t.Fatal("run masih running setelah gate dibuat; itu akan di-reclaim sebagai stale")
	}
	// `budget_exceeded`/`budget`, bukan `failed`: run berhenti karena ada yang
	// harus diputuskan manusia, bukan karena pekerjaannya rusak. `failed` akan
	// dibaca sebagai "kerjanya gagal" oleh siapa pun yang melihat `runs.outcome`,
	// dan mereka tidak bisa melihat gate-nya dari sana.
	if run.Outcome != "budget_exceeded" || run.FailureKind != "budget" {
		t.Fatalf("run outcome/failure_kind = %q/%q, want budget_exceeded/budget", run.Outcome, run.FailureKind)
	}

	// Event approval.requested tercatat (5.3).
	events, err := f.svc.TaskHistory(ctx, f.taskID)
	if err != nil {
		t.Fatalf("TaskEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Kind == "approval.requested" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tidak ada event approval.requested; events=%v", events)
	}
}

// TestPgRequestApprovalRequiresAPreview — US-AD33 AC2. Termasuk `{}` dan
// `null`: dua-duanya lolos pemeriksaan panjang, dan dua-duanya tidak
// memperlihatkan apa pun ke approver.
func TestPgRequestApprovalRequiresAPreview(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	ctx := context.Background()

	for _, preview := range []string{``, `{}`, `null`, `[]`, `"  "`} {
		_, err := f.svc.RequestApproval(ctx, f.taskID, f.orgID, "worker@example.com",
			json.RawMessage(preview), "")
		if !errors.Is(err, ErrApprovalPreviewRequired) {
			t.Errorf("preview %q: err = %v, want ErrApprovalPreviewRequired", preview, err)
		}
	}
	// Task-nya tidak boleh ikut terparkir oleh permintaan yang ditolak.
	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusRunning {
		t.Fatalf("status = %q setelah permintaan ditolak, want running", task.Status)
	}
}

// TestPgApproveSendsTheTaskBackToReady — 5.4 / 5.3: approve -> ready.
//
// Bukan `running` seperti yang ditulis PRD US-AD34 AC1: run-nya sudah ditutup
// saat gate dibuat, jadi tidak ada run yang bisa dilanjutkan. Task kembali
// `ready` supaya dispatcher mengklaimnya dengan run baru.
func TestPgApproveSendsTheTaskBackToReady(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	decided, err := f.svc.Approve(ctx, approval.ID, f.orgID, "owner@example.com")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if decided.Decision != ApprovalApproved {
		t.Fatalf("decision = %q, want approved", decided.Decision)
	}
	if decided.DecidedBy != "owner@example.com" {
		t.Fatalf("decided_by = %q, want owner@example.com", decided.DecidedBy)
	}
	if decided.DecidedAt == nil {
		t.Fatal("decided_at kosong pada gate yang sudah diputuskan")
	}

	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusReady {
		t.Fatalf("status = %q setelah approve, want ready", task.Status)
	}
}

// TestPgApproveTwiceIs409 — US-AD34 AC2. Yang diuji di sini adalah predikatnya
// ada di UPDATE, bukan di cek-lalu-tulis: dua pemanggilan berturut-turut harus
// menghasilkan satu 'approved' dan satu penolakan.
func TestPgApproveTwiceIs409(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	if _, err := f.svc.Approve(ctx, approval.ID, f.orgID, "owner@example.com"); err != nil {
		t.Fatalf("Approve pertama: %v", err)
	}
	if _, err := f.svc.Approve(ctx, approval.ID, f.orgID, "other@example.com"); !errors.Is(err, ErrApprovalDecided) {
		t.Fatalf("Approve kedua = %v, want ErrApprovalDecided", err)
	}
	// Keputusan pertama tidak boleh tertimpa.
	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.DecidedBy != "owner@example.com" {
		t.Fatalf("decided_by = %q; keputusan pertama tertimpa", stored.DecidedBy)
	}
}

// TestPgRejectBlocksTheTaskAsPolicy — US-AD35 AC1 + 5.4.
//
// PRD bilang `blocked(needs_input)`. Kontrak menang: approver-nya tidak meminta
// informasi tambahan, dia menolak — dan `needs_input` akan menampilkan task itu
// sebagai "menunggu jawaban", bukan "ditolak".
func TestPgRejectBlocksTheTaskAsPolicy(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	decided, err := f.svc.Reject(ctx, approval.ID, f.orgID, "owner@example.com", "jangan hapus tabel dev")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if decided.Decision != ApprovalRejected {
		t.Fatalf("decision = %q, want rejected", decided.Decision)
	}
	if decided.Reason != "jangan hapus tabel dev" {
		t.Fatalf("reason = %q", decided.Reason)
	}

	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusBlocked {
		t.Fatalf("status = %q setelah reject, want blocked", task.Status)
	}
	if task.BlockKind != "policy" {
		t.Fatalf("block_kind = %q, want policy (5.4)", task.BlockKind)
	}
}

// TestPgRejectRequiresAReason — US-AD35 AC2.
func TestPgRejectRequiresAReason(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	if _, err := f.svc.Reject(ctx, approval.ID, f.orgID, "owner@example.com", "   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Reject tanpa alasan = %v, want ErrInvalidInput", err)
	}
	// Gate-nya harus tetap pending: penolakan yang ditolak tidak boleh menutupnya.
	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.Decision != ApprovalPending {
		t.Fatalf("decision = %q setelah penolakan gagal, want pending", stored.Decision)
	}
}

// TestPgDecidingAnExpiredApprovalIs409 — US-AD35 AC3.
//
// Tiga langkah, dan yang ketiga itu intinya: gate-nya harus BENAR-BENAR
// kedaluwarsa dulu lewat sapuan yang sama dengan produksi, baru ditolak. Kalau
// langkah pertama dilewat, tes ini cuma menguji "menolak gate pending", yang
// memang harus sukses.
func TestPgDecidingAnExpiredApprovalIs409(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	// 1. Lewatkan tenggatnya, seperti 24 jam yang benar-benar berjalan.
	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE approvals SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		approval.ID); err != nil {
		t.Fatalf("majukan expires_at: %v", err)
	}
	// 2. Sapu, seperti tick dispatcher.
	expired, err := f.svc.ExpireDueApprovals(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ExpireDueApprovals: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("sweep menutup %d gate, want 1", len(expired))
	}
	// 3. Sekarang barulah penolakan terlambat diuji.
	if _, err := f.svc.Reject(ctx, approval.ID, f.orgID, "owner@example.com", "terlambat"); !errors.Is(err, ErrApprovalDecided) {
		t.Fatalf("Reject pada gate expired = %v, want ErrApprovalDecided", err)
	}
	if _, err := f.svc.Approve(ctx, approval.ID, f.orgID, "owner@example.com"); !errors.Is(err, ErrApprovalDecided) {
		t.Fatalf("Approve pada gate expired = %v, want ErrApprovalDecided", err)
	}
	// Keputusannya tetap 'expired', bukan tertimpa jadi approved/rejected.
	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.Decision != ApprovalExpired {
		t.Fatalf("decision = %q, want expired", stored.Decision)
	}
}

// TestPgExpirySweepClosesTheGateAndTheTask — 8.3 / N23 / J7.
func TestPgExpirySweepClosesTheGateAndTheTask(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	// Majukan tenggatnya ke masa lalu, seperti 24 jam yang benar-benar lewat.
	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE approvals SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		approval.ID); err != nil {
		t.Fatalf("majukan expires_at: %v", err)
	}

	// Inbox tidak boleh lagi menampilkan gate yang sudah lewat tenggat, bahkan
	// sebelum sapuannya jalan: tombolnya sudah tidak berarti.
	inbox, err := f.svc.ApprovalInbox(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ApprovalInbox: %v", err)
	}
	for _, item := range inbox {
		if item.ID == approval.ID {
			t.Fatal("gate yang sudah lewat tenggat masih muncul di inbox")
		}
	}

	expired, err := f.svc.ExpireDueApprovals(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ExpireDueApprovals: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != approval.ID {
		t.Fatalf("sweep mengembalikan %d gate, want 1 (%s)", len(expired), approval.ID)
	}

	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.Decision != ApprovalExpired {
		t.Fatalf("decision = %q setelah sweep, want expired", stored.Decision)
	}
	if stored.DecidedAt == nil {
		t.Fatal("decided_at kosong pada gate yang kedaluwarsa")
	}

	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusBlocked || task.BlockKind != "policy" {
		t.Fatalf("task setelah expiry = %q/%q, want blocked/policy (8.3)",
			task.Status, task.BlockKind)
	}

	events, err := f.svc.TaskHistory(ctx, f.taskID)
	if err != nil {
		t.Fatalf("TaskEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Kind == "approval.expired" {
			found = true
		}
	}
	if !found {
		t.Fatal("tidak ada event approval.expired")
	}

	// Sapuan kedua tidak menemukan apa-apa: 'expired' bukan 'pending'.
	again, err := f.svc.ExpireDueApprovals(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ExpireDueApprovals kedua: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("sweep kedua mengembalikan %d gate, want 0", len(again))
	}
}

// TestPgApprovalIsScopedToItsOrg — 11.4. Gate tenant lain harus tak terbedakan
// dari yang tidak ada, di baca maupun di putuskan.
func TestPgApprovalIsScopedToItsOrg(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	other := newOrg(t, ctx)

	if _, err := f.svc.GetApproval(ctx, approval.ID, other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetApproval lintas-tenant = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.Approve(ctx, approval.ID, other, "intruder@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Approve lintas-tenant = %v, want ErrNotFound", err)
	}
	inbox, err := f.svc.ApprovalInbox(ctx, other)
	if err != nil {
		t.Fatalf("ApprovalInbox org lain: %v", err)
	}
	if len(inbox) != 0 {
		t.Fatalf("org lain melihat %d gate milik tenant ini", len(inbox))
	}
	// Dan gate-nya masih pending di tenant yang benar.
	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.Decision != ApprovalPending {
		t.Fatalf("decision = %q; percobaan lintas-tenant mengubahnya", stored.Decision)
	}
}

// TestPgInboxListsPendingWithTaskTitle — inbox menampilkan apa yang diputuskan,
// bukan cuma id: judul task-nya di-join supaya layar tidak perlu satu request
// per baris.
func TestPgInboxListsPendingWithTaskTitle(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	inbox, err := f.svc.ApprovalInbox(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ApprovalInbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("inbox berisi %d gate, want 1", len(inbox))
	}
	if inbox[0].ID != approval.ID {
		t.Fatalf("inbox berisi %s, want %s", inbox[0].ID, approval.ID)
	}
	if inbox[0].TaskTitle == "" {
		t.Fatal("task_title kosong di inbox")
	}

	// Setelah diputuskan, ia keluar dari inbox tapi tetap ada di riwayat task.
	if _, err := f.svc.Approve(ctx, approval.ID, f.orgID, "owner@example.com"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	inbox, err = f.svc.ApprovalInbox(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ApprovalInbox kedua: %v", err)
	}
	if len(inbox) != 0 {
		t.Fatalf("gate yang sudah diputuskan masih di inbox: %+v", inbox)
	}
	history, err := f.svc.TaskApprovals(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("TaskApprovals: %v", err)
	}
	if len(history) != 1 || history[0].Decision != ApprovalApproved {
		t.Fatalf("riwayat task = %+v, want satu gate approved", history)
	}
}

// TestPgDecideApprovalIsAtomicUnderARace — 8.4's "Atomic State Lock".
//
// Ini satu-satunya tes yang membuktikan predikat `decision = 'pending'` DI DALAM
// UPDATE itu perlu. `decide()` membaca gate-nya dulu dan menolak yang sudah
// diputuskan, jadi jalur sekuensial tetap benar walau predikatnya dihapus —
// mutasi itu memang lolos kalau cuma tes sekuensial yang ada. Yang tidak bisa
// ditolong oleh pembacaan di Go adalah dua approver yang menekan bersamaan:
// keduanya membaca 'pending', lalu keduanya menulis. Predikatnya yang membuat
// satu dari mereka mendapat nol baris.
//
// Dua goroutine, dua koneksi (pool), satu gate. Harus tepat satu pemenang.
func TestPgDecideApprovalIsAtomicUnderARace(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	const racers = 2
	results := make(chan bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start // start keduanya sedekat mungkin
			moved, err := f.repo.DecideApproval(ctx, approval.ID, f.orgID,
				ApprovalApproved, "racer@example.com", "")
			if err != nil {
				results <- false
				return
			}
			results <- moved
		}()
	}
	close(start)

	winners := 0
	for i := 0; i < racers; i++ {
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("%d dari %d penulis berhasil; want tepat 1 (8.4 Atomic State Lock)", winners, racers)
	}

	// Dan yang tersimpan adalah hasil pemenang, bukan tulisan yang saling menimpa.
	stored, err := f.svc.GetApproval(ctx, approval.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if stored.Decision != ApprovalApproved {
		t.Fatalf("decision = %q setelah balapan, want approved", stored.Decision)
	}
	if stored.DecidedAt == nil {
		t.Fatal("decided_at kosong setelah balapan")
	}
}

// TestPgExpirySweepLeavesFutureGatesAlone — N23.
//
// Sapuan berjalan tiap tick (2 detik). Tanpa syarat tenggat, ia akan menutup
// SETIAP gate yang masih terbuka, dua detik setelah gate itu dibuat — dan fitur
// ini tidak akan pernah bisa dipakai. Jadi yang diuji bukan "gate due ditutup"
// (itu sudah di tes lain) melainkan "gate yang belum due tidak disentuh".
//
// Dua task, dua gate: satu tenggatnya dimajukan, satu tidak. Ini juga memastikan
// sapuannya menyaring per baris, bukan "kalau ada yang due, tutup semuanya".
func TestPgExpirySweepLeavesFutureGatesAlone(t *testing.T) {
	f, due := approvalFixture(t)
	ctx := context.Background()

	// Task kedua dengan gate-nya sendiri, tenggatnya masih ~24 jam.
	future := secondApproval(t, f)

	// Belum ada yang due: sapuan harus melewati keduanya.
	expired, err := f.svc.ExpireDueApprovals(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ExpireDueApprovals: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("sweep menutup %d gate yang belum lewat tenggat", len(expired))
	}
	for _, id := range []string{due.ID, future.ID} {
		stored, err := f.svc.GetApproval(ctx, id, f.orgID)
		if err != nil {
			t.Fatalf("GetApproval: %v", err)
		}
		if stored.Decision != ApprovalPending {
			t.Fatalf("gate %s = %q, want pending", id, stored.Decision)
		}
	}

	// Majukan hanya yang pertama.
	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE approvals SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		due.ID); err != nil {
		t.Fatalf("majukan expires_at: %v", err)
	}
	expired, err = f.svc.ExpireDueApprovals(ctx, f.orgID)
	if err != nil {
		t.Fatalf("ExpireDueApprovals kedua: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != due.ID {
		t.Fatalf("sweep menutup %+v, want hanya %s", expired, due.ID)
	}
	stillPending, err := f.svc.GetApproval(ctx, future.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetApproval gate kedua: %v", err)
	}
	if stillPending.Decision != ApprovalPending {
		t.Fatalf("gate kedua ikut ditutup (%q) padahal masih punya 24 jam", stillPending.Decision)
	}
	// Task yang gate-nya belum due juga tidak boleh ikut diblokir.
	futureTask, err := f.svc.GetTask(ctx, future.TaskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask task kedua: %v", err)
	}
	if futureTask.Status != StatusAwaitingApproval {
		t.Fatalf("status task kedua = %q, want awaiting_approval", futureTask.Status)
	}
}

// secondApproval membuat task kedua di board yang sama, menjalankannya, dan
// memasang gate di atasnya. Dipakai tes yang butuh lebih dari satu gate hidup —
// satu task tidak bisa punya dua (gate kedua butuh run hidup, dan run pertama
// sudah ditutup oleh gate pertama).
func secondApproval(t *testing.T, f runtimeFixture) Approval {
	t.Helper()
	ctx := context.Background()

	task, err := f.repo.CreateTask(ctx, Task{
		ID: ulid.Must(), OrgID: f.orgID, BoardID: f.boardID, Title: "Second gate",
		Status: StatusBacklog, CreatedBy: newUser(t, ctx),
		WorkspaceKind: WorkspaceScratch, GoalMode: "auto",
	})
	if err != nil {
		t.Fatalf("create task kedua: %v", err)
	}
	if _, err := f.repo.UpdateTaskStatus(ctx, task.ID, f.orgID, StatusBacklog, StatusReady); err != nil {
		t.Fatalf("ready: %v", err)
	}
	runID := ulid.Must()
	claimed, err := f.repo.ClaimReadyTasks(ctx, f.orgID, f.boardID, []string{runID}, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim task kedua: claimed=%d err=%v", len(claimed), err)
	}
	if _, err := f.svc.StartRun(ctx, f.orgID, task.ID, f.agentID, runID); err != nil {
		t.Fatalf("StartRun task kedua: %v", err)
	}
	approval, err := f.svc.RequestApproval(ctx, task.ID, f.orgID, "worker@example.com",
		json.RawMessage(`{"action":"second"}`), "")
	if err != nil {
		t.Fatalf("RequestApproval task kedua: %v", err)
	}
	return approval
}

// TestPgASecondGateNeedsALiveRun — kenapa gate kedua pada task yang sudah
// terparkir ditolak, bukan disimpan.
//
// `approvals.run_id` NOT NULL dan terikat FK ke `runs`. Begitu gate pertama
// dipasang, run-nya ditutup dan binding klaimnya dilepas — jadi gate kedua tidak
// punya run untuk ditunjuk, dan mengarang satu berarti baris approval yang
// menunjuk run yang tidak ada. Yang benar: task itu harus diklaim ulang dulu.
//
// Ini juga yang membuat "dua aksi berbahaya berturut-turut" punya jawaban yang
// jujur: yang kedua ditolak, task-nya `blocked` menunggu manusia, bukan diam-diam
// menumpuk gate yang tidak bisa diputuskan.
func TestPgASecondGateNeedsALiveRun(t *testing.T) {
	f, _ := approvalFixture(t)
	ctx := context.Background()

	if _, err := f.svc.RequestApproval(ctx, f.taskID, f.orgID, "worker@example.com",
		json.RawMessage(`{"action":"second"}`), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("gate kedua tanpa run hidup = %v, want ErrInvalidInput", err)
	}
	// Satu gate saja yang tersimpan — yang pertama.
	history, err := f.svc.TaskApprovals(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("TaskApprovals: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("riwayat berisi %d gate, want 1", len(history))
	}
}

// TestPgRequestApprovalWithoutARunIsRefused.
//
// `approvals.run_id` NOT NULL dan terikat FK ke `runs`, jadi gate tanpa run
// memang tidak bisa disimpan — tapi tanpa penjagaan eksplisit, jalurnya gagal
// lewat GetRun("") dan jawabannya ErrNotFound: 404 untuk task yang jelas-jelas
// ada di workspace pemanggil. 404 di situ membingungkan ("task-nya hilang?"),
// padahal masalahnya task itu belum punya run. Penjagaan ini yang membuat
// jawabannya ErrInvalidInput (400) dan menyebut masalah yang sebenarnya.
func TestPgRequestApprovalWithoutARunIsRefused(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	// Sengaja TANPA f.start(t): task sudah `running` dan punya current_run_id,
	// jadi binding-nya dilepas untuk meniru task yang diklaim tapi belum
	// dijalankan.
	if err := f.svc.ClearTaskCurrentRun(ctx, f.taskID, f.orgID); err != nil {
		t.Fatalf("ClearTaskCurrentRun: %v", err)
	}

	_, err := f.svc.RequestApproval(ctx, f.taskID, f.orgID, "worker@example.com",
		json.RawMessage(`{"action":"drop_table"}`), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("gate tanpa run = %v, want ErrInvalidInput (400), bukan 404", err)
	}
	// Dan tidak ada baris gate yang tertinggal.
	history, err := f.svc.TaskApprovals(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("TaskApprovals: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("permintaan yang ditolak meninggalkan %d baris gate", len(history))
	}
}

// TestPgApproveLeavesTheTaskClaimableAgain — invarian yang ditemukan probe.
//
// Gate mengakhiri run-nya, dan predikat klaim adalah `current_run_id IS NULL`
// (4b). Kalau binding-nya tidak dilepas, task-nya `ready` tapi dispatcher tidak
// akan pernah mengambilnya. Yang diuji di sini bukan statusnya, melainkan bahwa
// task itu benar-benar bisa diklaim lagi: itu satu-satunya bukti yang berarti.
func TestPgApproveLeavesTheTaskClaimableAgain(t *testing.T) {
	f, approval := approvalFixture(t)
	ctx := context.Background()

	if _, err := f.svc.Approve(ctx, approval.ID, f.orgID, "owner@example.com"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	task, err := f.svc.GetTask(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != StatusReady {
		t.Fatalf("status = %q, want ready", task.Status)
	}
	if task.CurrentRunID != "" {
		t.Fatalf("current_run_id = %q setelah approve; predikat klaim butuh NULL", task.CurrentRunID)
	}

	// Klaim ulang lewat jalur produksi. Kalau binding-nya tertinggal, ini
	// mengembalikan nol task dan tidak ada yang gagal — persis kegagalan senyap
	// yang harus ditangkap tes.
	nextRun := ulid.Must()
	claimed, err := f.repo.ClaimReadyTasks(ctx, f.orgID, f.boardID, []string{nextRun}, 1)
	if err != nil {
		t.Fatalf("ClaimReadyTasks: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != f.taskID {
		t.Fatalf("task yang disetujui tidak bisa diklaim lagi: claimed=%d", len(claimed))
	}
}
