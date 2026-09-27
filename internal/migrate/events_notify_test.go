package migrate

// Trigger NOTIFY untuk SSE — ARCHITECTURE 7.2.
//
// §7.2 menetapkan hub menerima event baru lewat `LISTEN agentdeck_events`.
// Yang diuji di sini adalah pengirimnya: tanpa trigger, hub menunggu di
// channel yang tidak pernah ada yang mengisi, dan seluruh endpoint SSE jadi
// stream yang hanya mengirim heartbeat.
//
// Trigger yang tidak pernah diuji adalah trigger yang tidak pernah menyala,
// jadi tesnya benar-benar LISTEN lalu INSERT — bukan membaca definisi trigger
// dari katalog.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPgEventsTriggerNotifiesOnInsert is the load-bearing test for 7.2: an
// inserted event wakes a listener on `agentdeck_events`.
func TestPgEventsTriggerNotifiesOnInsert(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Koneksi khusus untuk LISTEN: notifikasi dikirim ke koneksi yang
	// mendengarkan, jadi tidak bisa lewat pool bersama — pool bisa memberi
	// koneksi lain untuk INSERT-nya.
	listener, err := pgxpool.New(ctx, migrateTestDSN)
	if err != nil {
		t.Fatalf("buka koneksi listener: %v", err)
	}
	defer listener.Close()

	conn, err := listener.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire listener: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN agentdeck_events"); err != nil {
		t.Fatalf("LISTEN: %v", err)
	}

	// Insert lewat koneksi lain supaya notifikasinya benar-benar dari trigger,
	// bukan dari sesi yang sama.
	var id int64
	err = pool.QueryRow(ctx,
		`INSERT INTO events (org_id, board_id, kind, payload_json)
		 VALUES ('org-notify-test', 'board-notify-test', 'task.created', '{}'::jsonb)
		 RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	notification, err := conn.Conn().WaitForNotification(waitCtx)
	if err != nil {
		t.Fatalf("trigger tidak mengirim notifikasi (hub SSE akan diam): %v", err)
	}
	if notification.Channel != "agentdeck_events" {
		t.Fatalf("channel = %q, want agentdeck_events", notification.Channel)
	}
	// Payload-nya HANYA id. Kalau isinya ikut dikirim, event besar akan gagal
	// diam-diam di batas 8000 byte milik NOTIFY.
	if got := strings.TrimSpace(notification.Payload); got == "" {
		t.Fatal("payload notifikasi kosong; penerima tidak tahu baris mana yang dibaca")
	}
	if strings.Contains(notification.Payload, "task.created") {
		t.Fatalf("payload memuat isi event, want hanya id: %q", notification.Payload)
	}
}

// TestPgEventsNotifyPayloadStaysTinyAtThePayloadCeiling pins the design decision
// behind sending only the id: N21 allows a 64 KB payload, while a NOTIFY payload
// is capped around 8000 bytes. If the trigger ever starts sending the body,
// large events stop being delivered with no error anywhere.
func TestPgEventsNotifyPayloadStaysTinyAtThePayloadCeiling(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	listener, err := pgxpool.New(ctx, migrateTestDSN)
	if err != nil {
		t.Fatalf("buka koneksi listener: %v", err)
	}
	defer listener.Close()
	conn, err := listener.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire listener: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN agentdeck_events"); err != nil {
		t.Fatalf("LISTEN: %v", err)
	}

	// Payload mendekati batas 64 KB (N21) — jauh di atas batas NOTIFY.
	big := `{"blob":"` + strings.Repeat("x", 60000) + `"}`
	var id int64
	err = pool.QueryRow(ctx,
		`INSERT INTO events (org_id, kind, payload_json)
		 VALUES ('org-notify-big', 'step.finished', $1::jsonb)
		 RETURNING id`, big).Scan(&id)
	if err != nil {
		t.Fatalf("insert event besar: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	notification, err := conn.Conn().WaitForNotification(waitCtx)
	if err != nil {
		t.Fatalf("event besar tidak menghasilkan notifikasi: %v", err)
	}
	if len(notification.Payload) > 64 {
		t.Fatalf("payload notifikasi %d byte, want hanya id (<=64): %q",
			len(notification.Payload), notification.Payload[:64])
	}
}

// TestPgEventsNotifyTriggerSurvivesReapply: migrasi repo ini dijalankan ulang di
// atas skema terisi (TestPostgresRepairMigration*), jadi trigger-nya harus tetap
// ada dan tetap satu setelah Apply dipanggil dua kali.
func TestPgEventsNotifyTriggerSurvivesReapply(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply pertama: %v", err)
	}
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply kedua: %v", err)
	}

	var n int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_trigger WHERE tgname = 'events_notify_trigger' AND NOT tgisinternal`).Scan(&n)
	if err != nil {
		t.Fatalf("hitung trigger: %v", err)
	}
	if n != 1 {
		t.Fatalf("trigger events_notify_trigger = %d, want tepat 1", n)
	}
}
