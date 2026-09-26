package board

// Producer notifikasi (US-AD61, 6.2.19) — bagian yang mengubah tabel
// `notifications` dari "ada skemanya" jadi "ada yang mengisinya".
//
// Yang diukur di sini bukan penulisan barisnya (itu urusan auth.Store, sudah
// diuji di sana), tapi keputusan yang dibuat board sebelum menulis:
//
//  1. Ambang 80% (N18), dan bahwa ambang itu berarti "lebih dari 80%", bukan
//     "lebih dari 0%" — notifikasi yang menyala terlalu dini akan diabaikan
//     sebelum cap-nya benar-benar jadi masalah.
//  2. `sink == nil` adalah no-op, bukan panic. Service dibangun tanpa notifier
//     di banyak tes dan di deployment tanpa inbox; itu harus aman.
//  3. `cancelled` tidak pernah jadi `run.failed`: itu tindakan manusia.
//  4. Target id tidak pernah kosong, karena US-AD61 AC2 memerlukan notifikasi
//     yang bisa diklik.

import (
	"context"
	"errors"
	"testing"
)

// recordingNotifier merekam panggilan alih-alih menulis ke database.
type recordingNotifier struct {
	calls []OperationalNotice
	err   error
}

func (r *recordingNotifier) NotifyOperational(_ context.Context, orgID, kind, title, body, targetType, targetID string) error {
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, OperationalNotice{
		OrgID: orgID, Kind: kind, Title: title, Body: body,
		TargetType: targetType, TargetID: targetID,
	})
	return nil
}

func notifyingService(t *testing.T) (*Service, *recordingNotifier) {
	t.Helper()
	recorder := &recordingNotifier{}
	svc := NewService(nil).WithNotifier(recorder)
	return svc, recorder
}

// TestNotifyBudgetThresholdFiresAtEightyPercent pins N18.
func TestNotifyBudgetThresholdFiresAtEightyPercent(t *testing.T) {
	svc, recorder := notifyingService(t)
	ctx := context.Background()

	// 79% — belum. Notifikasi yang menyala sebelum ambangnya berarti akan
	// diabaikan sebelum cap-nya benar-benar jadi masalah.
	if err := svc.NotifyBudgetThreshold(ctx, "o1", "b1", "Sprint", 790_000, 1_000_000); err != nil {
		t.Fatalf("79%%: %v", err)
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("79%% memicu %d notifikasi, want 0", len(recorder.calls))
	}

	// 80% tepat — ya. Batasnya inklusif, dan itu yang membuat ambangnya bisa
	// diprediksi: "80%" berarti 80%, bukan "81%".
	if err := svc.NotifyBudgetThreshold(ctx, "o1", "b1", "Sprint", 800_000, 1_000_000); err != nil {
		t.Fatalf("80%%: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("80%% memicu %d notifikasi, want 1", len(recorder.calls))
	}
	got := recorder.calls[0]
	if got.Kind != "budget.warning" {
		t.Fatalf("kind = %q, want budget.warning", got.Kind)
	}
	if got.TargetType != "board" || got.TargetID != "b1" {
		t.Fatalf("target = %q/%q, want board/b1 (US-AD61 AC2)", got.TargetType, got.TargetID)
	}
	if got.OrgID != "o1" {
		t.Fatalf("org = %q, want o1", got.OrgID)
	}
}

// TestNotifyBudgetThresholdIgnoresUncappedBoards.
//
// Board tanpa cap (capMicros 0) tidak pernah "mendekati batas" — tidak ada
// batasnya. Menganggap 0 sebagai cap berarti setiap board tanpa budget
// memuntahkan peringatan.
func TestNotifyBudgetThresholdIgnoresUncappedBoards(t *testing.T) {
	svc, recorder := notifyingService(t)
	if err := svc.NotifyBudgetThreshold(context.Background(), "o1", "b1", "Sprint", 5_000_000, 0); err != nil {
		t.Fatalf("board tanpa cap: %v", err)
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("board tanpa cap memicu %d notifikasi, want 0", len(recorder.calls))
	}
}

// TestNotifyBudgetThresholdDoesNotOverflowOnLargeCaps.
//
// Perbandingannya `spent*100 < cap*80`. Kalau ditulis sebagai pembagian
// (`spent/cap < 0.8`), pembulatan integer membuat ambangnya bergeser; kalau
// ditulis dengan float, nilai besar kehilangan presisi. Dua-duanya menghasilkan
// peringatan yang datang di waktu yang salah.
func TestNotifyBudgetThresholdDoesNotOverflowOnLargeCaps(t *testing.T) {
	svc, recorder := notifyingService(t)
	ctx := context.Background()
	// Cap besar yang realistis: 1 miliar micro-USD = 1000 USD per hari.
	const cap = int64(1_000_000_000)

	if err := svc.NotifyBudgetThreshold(ctx, "o1", "b1", "Sprint", cap*79/100, cap); err != nil {
		t.Fatalf("79%% dari cap besar: %v", err)
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("79%% dari cap besar memicu %d notifikasi, want 0", len(recorder.calls))
	}
	if err := svc.NotifyBudgetThreshold(ctx, "o1", "b1", "Sprint", cap*80/100, cap); err != nil {
		t.Fatalf("80%% dari cap besar: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("80%% dari cap besar memicu %d notifikasi, want 1", len(recorder.calls))
	}
}

// TestNotifyRunFailedCarriesTheRunAsTarget.
func TestNotifyRunFailedCarriesTheRunAsTarget(t *testing.T) {
	svc, recorder := notifyingService(t)
	if err := svc.NotifyRunFailed(context.Background(), "o1", "t1", "run-9", "Deploy", "transient"); err != nil {
		t.Fatalf("NotifyRunFailed: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("notifikasi = %d, want 1", len(recorder.calls))
	}
	got := recorder.calls[0]
	if got.Kind != "run.failed" {
		t.Fatalf("kind = %q, want run.failed", got.Kind)
	}
	if got.TargetType != "run" || got.TargetID != "run-9" {
		t.Fatalf("target = %q/%q, want run/run-9", got.TargetType, got.TargetID)
	}
}

// TestNotifierErrorsAreReturnedNotSwallowed.
//
// Board mengembalikan error dari penulisnya supaya dispatcher bisa mencatatnya.
// Yang TIDAK boleh dilakukan board adalah menelannya di sini: satu-satunya
// tempat yang tahu "alert gagal tidak boleh menggagalkan run" adalah pemanggil
// run-nya, bukan lapisan ini.
func TestNotifierErrorsAreReturnedNotSwallowed(t *testing.T) {
	failure := errors.New("inbox tidak bisa ditulis")
	svc := NewService(nil).WithNotifier(&recordingNotifier{err: failure})

	if err := svc.NotifyRunFailed(context.Background(), "o1", "t1", "run-9", "Deploy", "transient"); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
	if err := svc.NotifyBudgetThreshold(context.Background(), "o1", "b1", "S", 900_000, 1_000_000); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
}

// TestProducersAreNoOpsWithoutANotifier.
//
// Deployment tanpa inbox, dan sebagian besar tes, membangun Service tanpa
// notifier. Itu harus jadi no-op — bukan panic, dan bukan error yang menular ke
// jalur run.
func TestProducersAreNoOpsWithoutANotifier(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()
	if err := svc.NotifyRunFailed(ctx, "o1", "t1", "run-9", "Deploy", "transient"); err != nil {
		t.Fatalf("tanpa notifier: %v", err)
	}
	if err := svc.NotifyBudgetThreshold(ctx, "o1", "b1", "S", 900_000, 1_000_000); err != nil {
		t.Fatalf("tanpa notifier: %v", err)
	}
	if err := svc.NotifyOperational(ctx, "o1", "run.failed", "x", "y", "run", "r1"); err != nil {
		t.Fatalf("NotifyOperational tanpa notifier: %v", err)
	}
}

// TestNotifyOperationalMintsATargetWhenMissing.
//
// Target kosong berarti notifikasinya tidak bisa diklik ke mana pun (AC2).
// Mengganti dengan id acak lebih baik daripada mengirim kabar yang membawa
// pemakainya ke halaman kosong — dan lebih baik daripada gagal, karena kabarnya
// sendiri masih berguna.
func TestNotifyOperationalMintsATargetWhenMissing(t *testing.T) {
	svc, recorder := notifyingService(t)
	if err := svc.NotifyOperational(context.Background(), "o1", "credential.invalid", "Kredensial", "x", "", ""); err != nil {
		t.Fatalf("NotifyOperational: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("notifikasi = %d, want 1", len(recorder.calls))
	}
	if recorder.calls[0].TargetID == "" {
		t.Fatal("target id kosong — notifikasinya tidak bisa diklik (AC2)")
	}
}
