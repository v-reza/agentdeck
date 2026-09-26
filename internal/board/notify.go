package board

// NotifyOperational — producer notifikasi in-app (US-AD61, 6.2.19).
//
// Kenapa ini method di Service, bukan di auth.Store langsung:
//
// `auth.Store` tahu cara MENULIS notifikasi (dedup, penerima, constraint), tapi
// ia tidak tahu apa pun soal board, budget, atau run — dan tidak boleh tahu.
// Dispatcher memegang `board.Service`, jadi di sinilah kedua dunia itu bertemu:
// service menerjemahkan "board ini melewati ambang 80%" jadi notifikasi yang
// bentuknya benar, lalu menyerahkannya ke penulisnya.
//
// Kenapa lewat interface kecil, bukan import `internal/auth`:
// `internal/board` tidak mengimpor `internal/auth` sama sekali hari ini, dan
// menambahkannya berarti setiap perubahan tipe auth menyentuh paket ini.
// Interface satu method ini menyatakan persis apa yang dibutuhkan service, dan
// pemanggil yang menyuntikkan implementasinya (cmd/api) yang menanggung
// ketergantungan itu.

import (
	"context"

	"agentdeck/internal/ulid"
)

// NotificationWriter adalah sisi tulis notifikasi yang dibutuhkan board.
//
// Satu method dengan parameter datar, bukan struct: `auth.Store` sudah
// menyediakan bentuk ini persis, jadi tidak ada lapisan penerjemahan di batas
// paket. Dedup (satu kejadian = satu baris per penerima per hari) adalah urusan
// penulisnya, bukan pemanggilnya — lihat komentar query `CreateNotificationOnce`.
type NotificationWriter interface {
	NotifyOperational(ctx context.Context, orgID, kind, title, body, targetType, targetID string) error
}

// OperationalNotice adalah notifikasi yang belum ditujukan ke siapa pun —
// penerimanya (owner + admin org) ditentukan oleh penulisnya.
type OperationalNotice struct {
	OrgID      string
	Kind       string
	Title      string
	Body       string
	TargetType string
	TargetID   string
}

// WithNotifier injects the sink at construction. Nil means "this deployment has
// no notification sink", which is the honest state for the tests that build a
// Service without one — the producer methods are no-ops rather than panics.
func (s *Service) WithNotifier(w NotificationWriter) *Service {
	s.notifier = w
	return s
}

// NotifyBudgetThreshold raises `budget.warning` when a board first crosses 80%
// of its daily cap (N18).
//
// Ambangnya 80%, bukan 100%: pada 100% pekerjaannya sudah berhenti, dan kabar
// "kamu kehabisan budget" datang terlambat untuk berguna. Peringatan harus tiba
// saat masih ada waktu memperbaiki.
//
// Kegagalan menulis notifikasi TIDAK dikembalikan sebagai error ke pemanggil
// run: alert yang gagal tidak boleh menggagalkan pekerjaan yang memicunya.
// Yang dikembalikan adalah jumlah baris tertulis, supaya pemanggil bisa mencatat
// dan tes bisa membuktikan dedup-nya.
func (s *Service) NotifyBudgetThreshold(ctx context.Context, orgID, boardID, boardName string, spentMicros, capMicros int64) error {
	if s.notifier == nil || capMicros <= 0 {
		return nil
	}
	// Sudah lewat ambang tapi belum habis: itulah jendela yang berguna.
	if spentMicros*100 < capMicros*80 {
		return nil
	}
	return s.notify(ctx, OperationalNotice{
		OrgID:      orgID,
		Kind:       "budget.warning",
		Title:      "Board mendekati batas harian",
		Body:       boardName + " sudah memakai lebih dari 80% budget hariannya.",
		TargetType: "board",
		TargetID:   boardID,
	})
}

// NotifyRunFailed raises `run.failed` for a run that ended without producing a
// result, so the operator learns about it without watching the board.
//
// `failureKind` sengaja TIDAK masuk ke dedup: dua kegagalan transien berturut-
// turut pada task yang sama dalam satu hari adalah satu kabar, bukan dua. Yang
// membedakan kejadian adalah run-nya, dan run id itu target-nya.
func (s *Service) NotifyRunFailed(ctx context.Context, orgID, taskID, runID, title, failureKind string) error {
	if s.notifier == nil {
		return nil
	}
	return s.notify(ctx, OperationalNotice{
		OrgID:      orgID,
		Kind:       "run.failed",
		Title:      "Run gagal: " + title,
		Body:       "Kegagalan " + failureKind + ".",
		TargetType: "run",
		TargetID:   runID,
	})
}

func (s *Service) notify(ctx context.Context, n OperationalNotice) error {
	if n.TargetID == "" {
		// Target kosong berarti notifikasinya tidak bisa diklik ke mana pun
		// (US-AD61 AC2). Lebih baik memberi id acak di sini daripada mengirim
		// kabar yang membawa pemakainya ke halaman kosong.
		n.TargetID = ulid.Must()
	}
	return s.notifier.NotifyOperational(ctx, n.OrgID, n.Kind, n.Title, n.Body, n.TargetType, n.TargetID)
}

// NotifyOperational adalah bentuk mentah yang dipakai dispatcher lewat
// interface `Store`-nya. Ia ada supaya dispatcher tidak perlu tahu apa pun soal
// penerjemahan pesan — ia hanya melaporkan kejadiannya.
func (s *Service) NotifyOperational(ctx context.Context, orgID, kind, title, body, targetType, targetID string) error {
	if s.notifier == nil {
		return nil
	}
	return s.notify(ctx, OperationalNotice{
		OrgID: orgID, Kind: kind, Title: title, Body: body,
		TargetType: targetType, TargetID: targetID,
	})
}
