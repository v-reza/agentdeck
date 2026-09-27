package sse

// Hub SSE — ARCHITECTURE 7.1-7.4.
//
// Aliran datanya: INSERT ke `events` → trigger `pg_notify('agentdeck_events', id)`
// (migrasi 0020) → Hub membaca barisnya lewat `GetEvent` → fan-out ke channel
// tiap klien → frame SSE ditulis ke response.
//
// Kenapa LISTEN dan bukan polling: §7.4 menetapkan `http.Request.Context().Done()`
// sebagai pemicu pembersihan, dan §7.2 menyebut `LISTEN agentdeck_events`
// sebagai jalur event baru. Polling akan membuat latensi p95 bergantung pada
// interval poll, sementara N3 menuntut ≤1.5s.
//
// Kenapa NOTIFY hanya membawa `id`: payload event boleh sampai 64 KB (N21),
// sementara payload NOTIFY dibatasi ~8000 byte. Mengirim isinya lewat NOTIFY
// berarti event besar hilang tanpa error di mana pun.
//
// Paket ini sengaja tidak tahu HTTP maupun Postgres: `Store` adalah irisan
// kecil yang dibutuhkannya, jadi hub bisa diuji tanpa database.

import (
	"context"
	"encoding/json"
	"time"
)

// clientBuffer adalah ukuran channel per klien (7.3: 128 event).
const clientBuffer = 128

// keepAliveInterval adalah jeda `: ping` (7.1: 15 detik).
const keepAliveInterval = 15 * time.Second

// replayLimit adalah plafon replay `Last-Event-ID` (7.2: LIMIT 500).
const replayLimit = 500

// dropLogEvery membatasi frekuensi log slow-consumer supaya satu klien yang
// macet tidak membanjiri log.
const dropLogEvery = time.Minute

// Event adalah satu baris `events` dalam bentuk yang dibutuhkan hub. Payload
// tetap `json.RawMessage` supaya hub tidak perlu tahu bentuk isinya — dan
// supaya tidak ada reserialisasi yang mengubah byte yang dikirim ke klien.
type Event struct {
	ID        int64
	OrgID     string
	BoardID   string
	Kind      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// Store adalah irisan kecil yang dibutuhkan hub dari penyimpanan.
type Store interface {
	// GetEvent membaca satu event berdasarkan id, hasil NOTIFY.
	GetEvent(ctx context.Context, id int64) (Event, error)
	// ListBoardEventsAfter mengembalikan event board setelah satu id (replay 7.2).
	ListBoardEventsAfter(ctx context.Context, boardID, orgID string, afterID int64, limit int) ([]Event, error)
	// Listener menjalankan fn untuk setiap NOTIFY pada channel ini, dan
	// mengembalikan hanya setelah ctx selesai. Implementasinya memegang satu
	// koneksi khusus: notifikasi hanya sampai ke koneksi yang LISTEN.
	Listener(ctx context.Context, channel string, fn func(payload string)) error
}

// Metrics adalah irisan registry metrik yang dipakai hub (N14). Ini interface,
// bukan tipe konkret, supaya hub bisa diuji tanpa registry.
type Metrics interface {
	Set(name string, value float64, labels ...string)
	Add(name string, delta float64, labels ...string)
}
