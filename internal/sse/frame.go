package sse

// Frame W3C (7.1), loop LISTEN, dan replay (7.2).

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// nowUnix adalah satu-satunya pemakaian jam di paket ini, dipisah supaya
// heartbeat bisa diuji tanpa bergantung waktu nyata.
func nowUnix() int64 { return time.Now().Unix() }

// FormatFrame menyusun satu frame SSE dari sebuah event.
//
// Urutan field mengikuti contoh di 7.1 (`id`, `event`, `data`) dan frame
// diakhiri baris kosong — tanpa itu, klien tidak pernah menganggap frame selesai
// dan event-nya tidak muncul sampai frame berikutnya datang.
//
// `data` ditulis satu baris. Payload dari Postgres sudah JSON padat, jadi tidak
// ada newline di dalamnya; kalau ada, frame-nya harus dipecah per baris `data:`
// sesuai spesifikasi W3C. Kita menormalkannya di sini supaya klien tidak pernah
// menerima frame yang rusak.
func FormatFrame(ev Event) []byte {
	payload := ev.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	data := strings.ReplaceAll(string(payload), "\n", "")
	var b strings.Builder
	b.WriteString("id: ")
	b.WriteString(strconv.FormatInt(ev.ID, 10))
	b.WriteString("\nevent: ")
	b.WriteString(ev.Kind)
	b.WriteString("\ndata: ")
	b.WriteString(data)
	b.WriteString("\n\n")
	return []byte(b.String())
}

// Heartbeat mengembalikan frame komentar `: ping` (7.1). Komentar SSE diabaikan
// klien, jadi ini murni menjaga koneksi tetap terbuka di depan reverse proxy.
func Heartbeat() []byte {
	return []byte(": keep-alive ping " + strconv.FormatInt(nowUnix(), 10) + "\n\n")
}

// ReplayLimit adalah plafon replay (7.2: LIMIT 500). Dibuka supaya handler
// yang melakukan replay satu-run memakai angka yang sama, bukan menyalinnya.
func ReplayLimit() int { return replayLimit }

// setGauge melaporkan jumlah koneksi hidup (N14).
func (h *Hub) setGauge(n int) {
	if h.metrics != nil {
		h.metrics.Set("agentdeck_sse_clients", float64(n))
	}
}

// Run memulai loop LISTEN dan berhenti saat ctx selesai.
//
// Dipanggil sekali per proses. Kalau Store.Listener mengembalikan error, Run
// mencatatnya dan berhenti — hub tanpa LISTEN hanya akan mengirim heartbeat,
// dan itu lebih baik terlihat di log daripada diam-diam jadi stream kosong.
func (h *Hub) Run(ctx context.Context) error {
	h.log.Info("sse: listening", "channel", h.channel)
	return h.store.Listener(ctx, h.channel, func(payload string) {
		id, err := strconv.ParseInt(strings.TrimSpace(payload), 10, 64)
		if err != nil {
			// Bukan id: trigger kita selalu mengirim id, jadi ini berarti ada
			// penulis lain di channel yang sama. Diabaikan, bukan diproses.
			h.log.Warn("sse: notifikasi bukan id", "payload", payload)
			return
		}
		ev, err := h.store.GetEvent(ctx, id)
		if err != nil {
			// Baris hilang atau DB sedang tidak bisa dihubungi. Event-nya tidak
			// dikirim sekarang, tapi klien yang reconnect akan mendapatkannya
			// lewat replay — jadi ini warning, bukan penutupan hub.
			h.log.Warn("sse: baca event", "id", id, "error", err)
			return
		}
		h.broadcast(ev)
	})
}

// Replay mengirim event board setelah `afterID` ke klien, sesuai 7.2.
//
// Dikirim sinkron sebelum koneksi didaftarkan ke broadcaster: kalau klien
// didaftarkan dulu, event yang masuk di antara replay dan pendaftaran bisa
// terkirim dua kali atau terlewat.
func (h *Hub) Replay(ctx context.Context, orgID, boardID string, afterID int64, send func([]byte) error) error {
	events, err := h.store.ListBoardEventsAfter(ctx, boardID, orgID, afterID, h.replayLimit)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := send(FormatFrame(ev)); err != nil {
			return err
		}
	}
	return nil
}
