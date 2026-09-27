package sse

// Implementasi Hub: registri klien, fan-out, dan loop LISTEN.

import (
	"log/slog"
	"sync"
	"time"
)

// Subscriber adalah satu koneksi SSE yang hidup.
type Subscriber struct {
	orgID   string
	boardID string
	// ch menerima frame yang sudah jadi. Buffer 128 (7.3).
	ch chan []byte
	// dropped menandai klien yang pernah kena slow-consumer. Dipakai supaya
	// log-nya tidak diulang tiap event.
	dropped bool
}

// Hub memegang semua koneksi SSE yang hidup dan menyalurkan event ke mereka.
type Hub struct {
	store   Store
	log     *slog.Logger
	metrics Metrics
	channel string

	mu   sync.RWMutex
	subs map[*Subscriber]struct{}

	// pingInterval dan replayLimit bisa ditimpa di tes supaya tidak perlu
	// menunggu 15 detik.
	pingInterval time.Duration
	replayLimit  int
}

// NewHub merakit hub. `channel` adalah nama channel LISTEN, dan pemanggil yang
// menentukannya supaya migrasi trigger dan kode hub tidak bisa menyimpang.
func NewHub(store Store, log *slog.Logger, metrics Metrics, channel string) *Hub {
	return &Hub{
		store:        store,
		log:          log,
		metrics:      metrics,
		channel:      channel,
		subs:         map[*Subscriber]struct{}{},
		pingInterval: keepAliveInterval,
		replayLimit:  replayLimit,
	}
}

// Subscribe mendaftarkan satu klien. Pemanggil **wajib** memanggil Unsubscribe
// (biasanya lewat defer) atau hub akan menganggap koneksinya hidup selamanya
// dan terus mengirim ke channel yang tidak ada yang membaca.
func (h *Hub) Subscribe(orgID, boardID string) *Subscriber {
	sub := &Subscriber{orgID: orgID, boardID: boardID, ch: make(chan []byte, clientBuffer)}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	n := len(h.subs)
	h.mu.Unlock()
	h.setGauge(n)
	return sub
}

// Unsubscribe melepas klien. Aman dipanggil dua kali.
func (h *Hub) Unsubscribe(sub *Subscriber) {
	h.mu.Lock()
	if _, ok := h.subs[sub]; ok {
		delete(h.subs, sub)
		close(sub.ch)
	}
	n := len(h.subs)
	h.mu.Unlock()
	h.setGauge(n)
}

// PingInterval adalah jeda heartbeat yang dipakai handler. Dibuka sebagai
// method supaya handler tidak menyalin konstanta 15 detik dan menyimpang dari
// hub saat tes menimpanya.
func (h *Hub) PingInterval() time.Duration { return h.pingInterval }

// Log mengembalikan logger hub supaya handler bisa mencatat dengan tujuan yang
// sama, tanpa membawa logger sendiri.
func (h *Hub) Log() *slog.Logger { return h.log }

// Events mengembalikan channel frame milik klien. Read-only bagi pemanggil.
func (s *Subscriber) Events() <-chan []byte { return s.ch }

// Clients melaporkan jumlah koneksi hidup. Dipakai handler untuk log dan tes.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// broadcast mengirim frame ke setiap klien yang cocok.
//
// Klien yang channelnya penuh **ditutup**, bukan diblokir: menunggu satu klien
// yang macet akan menghentikan fan-out untuk semua klien lain (7.3). Klien yang
// ditutup akan reconnect dengan Last-Event-ID dan melewatkan replay dari
// database, jadi tidak ada event yang hilang permanen.
func (h *Hub) broadcast(ev Event) {
	frame := FormatFrame(ev)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs {
		if sub.boardID != ev.BoardID || sub.orgID != ev.OrgID {
			continue
		}
		select {
		case sub.ch <- frame:
		default:
			if !sub.dropped {
				sub.dropped = true
				h.log.Warn("sse: slow consumer dropped, closing connection",
					"board", ev.BoardID, "org", ev.OrgID, "buffer", clientBuffer)
			}
			h.metrics.Add("agentdeck_sse_dropped_total", 1)
			// Tutup channel: writer di handler melihatnya dan mengakhiri
			// response, lalu klien reconnect. Menutup di sini aman karena
			// Unsubscribe memeriksa keanggotaan sebelum menutup lagi.
			delete(h.subs, sub)
			close(sub.ch)
			h.setGauge(len(h.subs))
		}
	}
}
