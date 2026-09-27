package main

// Adapter LISTEN untuk hub SSE (7.2).
//
// Kenapa bukan method di `board.Service`: notifikasi Postgres hanya sampai ke
// koneksi yang menjalankan LISTEN, jadi ini butuh koneksi khusus dari pool —
// bukan `*store.Queries` yang dipegang repository. Menaruhnya di sini menjaga
// batas itu terlihat: satu koneksi diambil, dipakai untuk mendengarkan, dan
// dikembalikan saat ctx selesai.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/board"
	"agentdeck/internal/sse"
)

// sseStore menyatukan dua sumber yang dibutuhkan hub: pembacaan event lewat
// board.Service (yang menjaga scoping org), dan koneksi LISTEN lewat pool.
type sseStore struct {
	svc  *board.Service
	pool *pgxpool.Pool
	log  *slog.Logger
}

func (s sseStore) GetEvent(ctx context.Context, id int64) (sse.Event, error) {
	ev, err := s.svc.EventByID(ctx, id)
	if err != nil {
		return sse.Event{}, err
	}
	return toSSEEvent(ev), nil
}

func (s sseStore) ListBoardEventsAfter(ctx context.Context, boardID, orgID string, afterID int64, limit int) ([]sse.Event, error) {
	events, err := s.svc.BoardEventsAfter(ctx, boardID, orgID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]sse.Event, 0, len(events))
	for _, ev := range events {
		out = append(out, toSSEEvent(ev))
	}
	return out, nil
}

// Listener menjalankan `LISTEN <channel>` pada satu koneksi khusus dan
// memanggil fn untuk setiap notifikasi, sampai ctx selesai.
//
// Koneksinya **selalu** dikembalikan ke pool lewat defer: koneksi yang bocor
// akan habiskan pool setelah beberapa kali reconnect, dan gejalanya muncul jauh
// dari sini (request lain menggantung menunggu koneksi).
func (s sseStore) Listener(ctx context.Context, channel string, fn func(payload string)) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+quoteIdent(channel)); err != nil {
		return err
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			// ctx selesai adalah cara normal loop ini berhenti (shutdown).
			if ctx.Err() != nil {
				return nil
			}
			// Error lain: beri jeda singkat supaya koneksi yang bermasalah
			// tidak jadi loop panas, lalu coba lagi. LISTEN di koneksi yang
			// sama tetap berlaku selama koneksinya hidup.
			s.log.Warn("sse: menunggu notifikasi", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		fn(notification.Payload)
	}
}

// quoteIdent mengapit nama channel dengan tanda kutip ganda, supaya nama yang
// mengandung karakter khusus tidak bisa mengubah perintah LISTEN. Nilainya
// datang dari kode, bukan dari request — ini sabuk pengaman, bukan sanitasi
// input pengguna.
func quoteIdent(name string) string {
	return `"` + name + `"`
}

// toSSEEvent memetakan event domain ke bentuk yang dipakai hub. Payload-nya
// disalin apa adanya: hub menulisnya ke frame tanpa reserialisasi, jadi byte
// yang dikirim ke klien sama dengan byte yang disimpan.
func toSSEEvent(ev board.Event) sse.Event {
	return sse.Event{
		ID:        ev.ID,
		OrgID:     ev.OrgID,
		BoardID:   ev.BoardID,
		Kind:      ev.Kind,
		Payload:   ev.PayloadJSON,
		CreatedAt: ev.CreatedAt,
	}
}
