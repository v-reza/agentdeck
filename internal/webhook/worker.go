package webhook

// Worker pengiriman — ARCHITECTURE 13.1, 13.3.
//
// Aliran datanya sama dengan hub SSE (7.2): INSERT `events` → trigger
// `pg_notify('agentdeck_events')` → LISTEN. Bedanya apa yang dilakukan setelah
// notifikasi datang. Hub menyiarkannya ke koneksi browser; worker mencari
// webhook yang cocok dengan board dan kind-nya, lalu mencatat satu baris
// `webhook_deliveries` per webhook.
//
// Kenapa percobaan pertama TIDAK dikirim langsung dari goroutine notifikasi:
// satu event bisa cocok dengan banyak webhook, dan mengirimnya di dalam
// callback LISTEN berarti setiap pengiriman yang lambat (batasnya 10 detik,
// 13.3) menahan notifikasi berikutnya — antrean menumpuk tepat saat sistem
// sedang sibuk, yaitu saat webhook paling dibutuhkan. Percobaan pertama dan
// retry karena itu lewat **satu jalur**: tick membaca baris yang sudah jatuh
// tempo (attempts = 0 berarti tempo-nya "sekarang"), mengirimnya, dan mencatat
// hasilnya. Satu jalur berarti satu perilaku yang bisa diuji, tanpa goroutine
// tak terbatas per event.

import (
	"context"
	"log/slog"
	"strconv"
	"time"
)

// Store adalah sisi database yang dibutuhkan worker.
//
// `OpenSecret` ada di sini, bukan di pemanggil, karena dekripsi secret adalah
// satu-satunya tempat worker menyentuh kunci master — dan tidak ada alasan
// secret terbuka lebih lama dari satu pengiriman.
type Store interface {
	Listener(ctx context.Context, channel string, fn func(payload string)) error
	EventByID(ctx context.Context, id int64) (Event, error)
	MatchingSubscriptions(ctx context.Context, boardID, kind string) ([]Subscription, error)
	CreateDelivery(ctx context.Context, webhookID string, eventID int64) (int64, error)
	Retryable(ctx context.Context, limit int) ([]Pending, error)
	MarkDelivery(ctx context.Context, id int64, status string, attempts int, code *int, lastErr string) error
	OpenSecret(ctx context.Context, sub Subscription) (string, error)
}

// Metrics menerima penghitung pengiriman. Opsional (nil = tidak dihitung),
// sama seperti di paket lain: tes membangun worker tanpa registry.
type Metrics interface {
	Add(name string, delta float64, labels ...string)
}

// Pending adalah satu pengiriman yang menunggu diproses, beserta event yang
// memicunya.
//
// Event-nya ikut dibaca bersama barisnya (satu query, LEFT JOIN di
// ListRetryableDeliveries) karena body pengiriman butuh kind, board, dan
// payload dari event. `Event` bisa nol kalau event-nya sudah dipurge retensi
// 30 hari (3.24) — lihat attempt().
type Pending struct {
	Delivery     Delivery
	Subscription Subscription
	Event        Event
}

// Worker mengirim event ke endpoint pelanggan.
type Worker struct {
	store   Store
	log     *slog.Logger
	metrics Metrics
	channel string
	client  *httpClient

	// tick adalah jeda antara dua pemindaian baris yang jatuh tempo. Default
	// satu detik supaya percobaan pertama tidak menunggu lama; jadwal retry
	// 13.3 sendiri berjarak menit, jadi tick yang lebih besar pun tetap benar.
	tick  time.Duration
	limit int

	// now bisa ditimpa tes supaya jadwal backoff tidak perlu dijalani dengan
	// tidur dua jam.
	now func() time.Time
}

// New merakit worker. `channel` adalah nama channel LISTEN yang sama dengan
// yang dipakai trigger migrasi 0020 dan hub SSE — pemanggil yang menentukan
// namanya supaya ketiganya tidak bisa menyimpang.
func New(store Store, log *slog.Logger, metrics Metrics, channel string) *Worker {
	return &Worker{
		store:   store,
		log:     log,
		metrics: metrics,
		channel: channel,
		client:  newHTTPClient(),
		tick:    time.Second,
		limit:   50,
		now:     time.Now,
	}
}

// Run menjalankan worker sampai ctx selesai.
//
// Pemindaian retry berjalan di goroutine sendiri sementara goroutine ini
// menunggu notifikasi; keduanya berhenti lewat ctx yang sama.
func (w *Worker) Run(ctx context.Context) error {
	go w.retryLoop(ctx)

	return w.store.Listener(ctx, w.channel, func(payload string) {
		id, err := strconv.ParseInt(payload, 10, 64)
		if err != nil {
			// Payload trigger adalah id event (migrasi 0020). Yang bukan angka
			// berarti ada penulis lain di channel ini; dicatat, bukan di-panic.
			w.log.Warn("webhook: payload notifikasi bukan id event", "payload", payload, "error", err)
			return
		}
		w.enqueue(ctx, id)
	})
}

// enqueue mencatat satu baris pengiriman untuk setiap webhook yang cocok.
//
// Pengirimannya sendiri tidak dilakukan di sini — lihat komentar paket. Yang
// penting: barisnya ditulis SEBELUM ada percobaan, jadi pengiriman yang gagal
// di tengah jalan tetap punya jejak dan tetap di-retry.
func (w *Worker) enqueue(ctx context.Context, eventID int64) {
	ev, err := w.store.EventByID(ctx, eventID)
	if err != nil {
		w.log.Warn("webhook: gagal membaca event", "event", eventID, "error", err)
		return
	}
	// Event tanpa board tidak punya langganan: webhook terikat ke board (3.22).
	if ev.BoardID == "" {
		return
	}

	subs, err := w.store.MatchingSubscriptions(ctx, ev.BoardID, ev.Kind)
	if err != nil {
		w.log.Warn("webhook: gagal mencari langganan", "event", eventID, "error", err)
		return
	}
	for _, sub := range subs {
		if _, err := w.store.CreateDelivery(ctx, sub.ID, eventID); err != nil {
			// Pelanggaran unique (event yang sama, webhook yang sama) bukan
			// kesalahan: itu artinya pengiriman sudah tercatat.
			w.log.Warn("webhook: gagal mencatat pengiriman", "webhook", sub.ID, "event", eventID, "error", err)
		}
	}
}

// retryLoop memindai pengiriman yang jatuh tempo sampai ctx selesai.
func (w *Worker) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

// drain memproses satu batch pengiriman yang jatuh tempo.
func (w *Worker) drain(ctx context.Context) {
	pending, err := w.store.Retryable(ctx, w.limit)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Warn("webhook: gagal membaca antrean", "error", err)
		}
		return
	}
	for _, p := range pending {
		if ctx.Err() != nil {
			return
		}
		// Belum jatuh tempo: dilewati tick ini, terambil tick berikutnya.
		if !w.due(p, w.now()) {
			continue
		}
		w.attempt(ctx, p)
	}
}

// due melaporkan apakah sebuah pengiriman sudah waktunya dicoba.
//
// 4xx TIDAK pernah diulang (13.3). Statusnya tetap `failed` seperti yang
// diminta 13.3, tapi `failed` juga status yang dibaca index retry — jadi
// kolom status saja tidak bisa membedakan "gagal, akan dicoba lagi" dari
// "gagal, sudah selesai". Yang membedakannya `response_code`: 4xx berarti
// endpointnya menolak, dan mengulanginya hanya menambah beban tanpa
// mengubah hasil. Keputusan ini diambil di sini, bukan dengan menambah
// status keempat ke skema yang sudah ditetapkan.
func (w *Worker) due(p Pending, now time.Time) bool {
	if code := p.Delivery.ResponseCode; code != nil && *code >= 400 && *code < 500 {
		return false
	}
	delay, ok := nextDelay(p.Delivery.Attempts)
	if !ok {
		return false
	}
	return !now.Before(p.Delivery.CreatedAt.Add(delay))
}

// markTerminal mencatat hasil akhir yang tidak lewat percobaan pengiriman.
//
// `attempts` sengaja TIDAK dinaikkan: purging bukan percobaan yang gagal, dan
// menaikkannya akan membuat riwayat pengiriman melaporkan percobaan yang tidak
// pernah terjadi. Barisnya tidak akan terambil lagi karena statusnya `dead`
// dan Retryable hanya memilih pending/failed.
func (w *Worker) markTerminal(ctx context.Context, p Pending, status, reason string) {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	if err := w.store.MarkDelivery(ctx, p.Delivery.ID, status, p.Delivery.Attempts, nil, reason); err != nil {
		w.log.Warn("webhook: gagal mencatat hasil akhir", "delivery", p.Delivery.ID, "error", err)
		return
	}
	if w.metrics != nil {
		w.metrics.Add("agentdeck_webhook_deliveries_total", 1, status)
	}
}

// attempt mengirim satu pengiriman dan mencatat hasilnya.
//
// `attempts` dinaikkan SETELAH percobaan selesai, bukan sebelumnya. Artinya
// pengiriman bersifat **at-least-once**: kalau prosesnya mati tepat di tengah
// pengiriman, barisnya tetap `attempts = 0` dan dikirim ulang. Itu pilihan yang
// disengaja — US-AD53 AC2 berbunyi "Event tidak hilang", dan kehilangan satu
// pengiriman lebih buruk daripada mengirimnya dua kali. Penerima membedakan
// kiriman ganda lewat `id` di body.
func (w *Worker) attempt(ctx context.Context, p Pending) {
	// Event-nya hilang karena retensi 30 hari (3.24) sementara baris
	// pengirimannya belum selesai. Tidak ada yang bisa dikirim, dan menunggu
	// tidak akan mengembalikannya — jadi ditandai `dead`, bukan dibiarkan
	// menggantung selamanya sebagai `pending`.
	if p.Event.ID == 0 {
		w.markTerminal(ctx, p, StatusDead, "event sudah dipurge retensi")
		return
	}

	secret, err := w.store.OpenSecret(ctx, p.Subscription)
	if err != nil {
		w.finish(ctx, p, 0, nil, "secret tidak bisa dibuka: "+err.Error())
		return
	}

	code, sendErr := w.send(ctx, p, secret)
	w.finish(ctx, p, code, sendErr)
}

// finish mencatat hasil satu percobaan.
func (w *Worker) finish(ctx context.Context, p Pending, code int, sendErr error, extra ...string) {
	attempts := p.Delivery.Attempts + 1

	// Pengiriman ke-`attempts` adalah percobaan terakhir yang diizinkan; kalau
	// masih gagal, statusnya `dead` (13.3).
	status, _ := classify(code, sendErr, attempts)

	lastErr := ""
	if len(extra) > 0 {
		lastErr = extra[0]
	} else if sendErr != nil {
		lastErr = sendErr.Error()
	}
	// Kolomnya TEXT tanpa batas; 500 karakter cukup untuk pesan apa pun dan
	// menjaga baris riwayat tidak tumbuh tanpa kendali.
	if len(lastErr) > 500 {
		lastErr = lastErr[:500]
	}

	var codePtr *int
	if code > 0 {
		codePtr = &code
	}
	if err := w.store.MarkDelivery(ctx, p.Delivery.ID, status, attempts, codePtr, lastErr); err != nil {
		w.log.Warn("webhook: gagal mencatat hasil pengiriman", "delivery", p.Delivery.ID, "error", err)
		return
	}

	if w.metrics != nil {
		w.metrics.Add("agentdeck_webhook_deliveries_total", 1, status)
	}
	if status == StatusDead {
		w.log.Warn("webhook: menyerah setelah percobaan terakhir",
			"webhook", p.Subscription.ID, "event", p.Delivery.EventID, "attempts", attempts)
	}
}
