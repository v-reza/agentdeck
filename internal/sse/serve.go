package sse

// Serve: handler HTTP untuk satu stream SSE (7.1-7.4).
//
// Tinggal di paket ini, bukan di cmd/api, karena seluruh isinya adalah detail
// protokol SSE — header no-transform, urutan replay-lalu-daftar, heartbeat, dan
// penanganan slow-consumer. Handler HTTP-nya cukup memutuskan board mana yang
// boleh dilihat pemanggil; sisanya di sini.

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Serve menulis stream SSE untuk satu board sampai klien pergi.
//
// Pemanggil sudah memverifikasi bahwa board ini milik org pemanggil — paket ini
// tidak tahu apa-apa soal tenancy, dan itu disengaja: satu tempat untuk aturan
// itu, bukan dua.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, orgID, boardID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Header wajib 7.4: `no-transform` + `X-Accel-Buffering: no` mematikan
	// buffering di Nginx/Cloudflare. Tanpa keduanya, event tertahan di proxy dan
	// UI tampak tidak pernah menerima apa pun meski servernya benar.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	// Replay sinkron LEBIH DULU, baru daftar ke broadcaster.
	//
	// Urutan sebaliknya punya jendela yang tidak bisa ditutup: event yang masuk
	// setelah pendaftaran tapi sebelum replay selesai akan terkirim dua kali
	// (sekali dari replay, sekali dari fan-out). Dengan replay dulu, event
	// sesudahnya hanya datang dari fan-out.
	//
	// Yang memicu replay adalah KEHADIRAN checkpoint, bukan nilainya. 7.2
	// berbunyi "jika ada last_event_id", dan `last_event_id=0` berarti "aku belum
	// menerima apa pun" — itu permintaan backlog penuh, bukan permintaan diam.
	// ids di `events` mulai dari 1, jadi 0 tidak pernah berarti "sudah selesai".
	if afterID, ok := LastEventID(r); ok {
		err := h.Replay(ctx, orgID, boardID, afterID, func(frame []byte) error {
			if _, err := w.Write(frame); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			// Replay gagal bukan alasan menutup stream yang sudah terbuka:
			// koneksinya sehat, dan klien tetap mendapat event baru. Yang
			// hilang hanya backlog, dan itu tercatat.
			h.log.Warn("sse: replay", "board", boardID, "org", orgID, "error", err)
		}
	}

	sub := h.Subscribe(orgID, boardID)
	defer h.Unsubscribe(sub)

	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			// Klien atau proxy menutup koneksi (7.4). `defer Unsubscribe` yang
			// membersihkan; di sini cukup berhenti.
			return
		case frame, ok := <-sub.Events():
			if !ok {
				// Slow consumer: hub menutup channel-nya (7.3). Klien akan
				// reconnect dengan Last-Event-ID dan mendapat replay.
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			// `: ping` mencegah proxy memutus koneksi idle (7.1).
			if _, err := w.Write(Heartbeat()); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// LastEventID membaca checkpoint resume klien (7.2): header `Last-Event-ID`
// lebih dulu, lalu `?last_event_id=`.
//
// Query param bukan kemewahan: `EventSource` di browser tidak bisa memasang
// header sendiri, jadi klien yang memakai fetch manual hanya punya cara itu.
// Nilai yang tidak bisa di-parse diperlakukan sebagai "tidak ada checkpoint",
// bukan sebagai error — klien yang mengirim sampah lebih baik mendapat replay
// penuh daripada stream yang tidak pernah dibuka.
// Nilai kedua melaporkan apakah checkpoint-nya ADA, bukan apakah nilainya > 0:
// `0` adalah checkpoint yang sah ("belum menerima apa pun"), dan memperlakukannya
// sebagai "tidak ada" membuat klien yang minta backlog penuh tidak mendapat apa
// pun.
func LastEventID(r *http.Request) (int64, bool) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("last_event_id")
	}
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 0 {
		// Sampah diperlakukan sebagai "tidak ada checkpoint" — replay penuh
		// lebih baik daripada stream yang tidak pernah dibuka.
		return 0, false
	}
	return id, true
}
