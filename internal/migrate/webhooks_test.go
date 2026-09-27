package migrate

// Migrasi 0021 terhadap Postgres nyata — ARCHITECTURE 3.22, 3.23, 13, 16.
//
// Yang cuma bisa dibuktikan lawan database sungguhan:
//
//  1. `webhooks_url_chk` benar-benar menolak `http://` ke internet dan tetap
//     menerima `http` ke tiga nama loopback §16. Regex ini kode keamanan; kalau
//     salah tanda, lubangnya SSRF dan tidak ada stub yang bisa menangkapnya.
//  2. `webhook_deliveries_status_chk` dan `_attempt_chk` benar-benar menolak
//     nilai di luar enum — kolom status adalah tempat seluruh keputusan retry
//     disimpan.
//  3. `webhook_deliveries` TIDAK punya FK ke `events`: retensi 30 hari (3.24)
//     menghapus event, dan riwayat pengiriman harus tetap hidup sesudahnya.
//  4. Migrasinya idempoten (jalur repair mengulang Apply di atas skema terisi).

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/ulid"
)

// seedWebhookFixture menyiapkan org + project + board yang nyata.
//
// Ditulis dengan INSERT mentah, bukan lewat helper paket lain: FK `webhooks`
// menuntut ketiganya ada, dan `orgs` di repo ini tidak punya FK ke users —
// jadi fixture-nya cukup tiga baris dan tidak perlu user.
func seedWebhookFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (orgID, boardID string) {
	t.Helper()
	orgID, boardID = ulid.Must(), ulid.Must()
	projectID := ulid.Must()
	if _, err := pool.Exec(ctx,
		`INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)`,
		orgID, strings.ToLower("hook-"+orgID[20:]), "Hook Org"); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)`,
		projectID, orgID, strings.ToLower("proj-"+projectID[20:]), "Hook Project"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO boards (id, org_id, project_id, slug, name, columns_json)
		 VALUES ($1, $2, $3, $4, $5, '[]'::jsonb)`,
		boardID, orgID, projectID, strings.ToLower("board-"+boardID[20:]), "Hook Board"); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	return orgID, boardID
}

func TestPgWebhookURLConstraintMatchesSection16(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	orgID, boardID := seedWebhookFixture(t, ctx, pool)

	cases := []struct {
		url string
		ok  bool
		why string
	}{
		{"https://example.com/hook", true, "https bebas"},
		{"http://localhost:9000/hook", true, "loopback §16"},
		{"http://127.0.0.1:9000/hook", true, "loopback §16"},
		{"http://host.docker.internal:9000/hook", true, "loopback §16"},
		{"http://evil.example/hook", false, "http ke internet"},
		{"http://10.0.0.5/hook", false, "RFC1918"},
		{"http://169.254.169.254/latest", false, "metadata cloud"},
		{"http://localhost.evil.example/hook", false, "localhost sebagai awalan host lain"},
		{"ftp://example.com/hook", false, "skema lain"},
	}

	for i, tc := range cases {
		id := ulid.Must()
		_, err := pool.Exec(ctx, `
			INSERT INTO webhooks (id, org_id, board_id, url, secret_enc, events_json, active)
			VALUES ($1, $2, $3, $4, $5, '[]'::jsonb, true)`,
			id, orgID, boardID, tc.url, []byte("sealed"))
		if tc.ok && err != nil {
			t.Errorf("kasus %d (%s): url %q ditolak: %v", i, tc.why, tc.url, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("kasus %d (%s): url %q DITERIMA, seharusnya ditolak", i, tc.why, tc.url)
		}
	}
}

func TestPgWebhookDeliveryConstraints(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	orgID, boardID := seedWebhookFixture(t, ctx, pool)
	hookID := ulid.Must()
	if _, err := pool.Exec(ctx, `
		INSERT INTO webhooks (id, org_id, board_id, url, secret_enc, events_json, active)
		VALUES ($1, $2, $3, 'https://example.com/hook', $4, '[]'::jsonb, true)`,
		hookID, orgID, boardID, []byte("sealed")); err != nil {
		t.Fatalf("insert webhook: %v", err)
	}

	// status di luar enum ditolak.
	if _, err := pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event_id, status) VALUES ($1, 1, 'entah')`,
		hookID); err == nil {
		t.Error("status di luar enum diterima")
	}
	// attempts di luar 0..10 ditolak.
	if _, err := pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event_id, status, attempts) VALUES ($1, 1, 'pending', 11)`,
		hookID); err == nil {
		t.Error("attempts 11 diterima, padahal CHECK-nya 0..10")
	}
	// nilai yang sah diterima.
	if _, err := pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event_id, status, attempts) VALUES ($1, 1, 'pending', 0)`,
		hookID); err != nil {
		t.Errorf("pending/0 ditolak: %v", err)
	}
}

// TestPgWebhookDeliverySurvivesEventPurge menaku keputusan "tanpa FK ke events".
func TestPgWebhookDeliverySurvivesEventPurge(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Nol FK ke events berarti event_id yang tidak ada tetap bisa disimpan.
	// Itu yang membuat riwayat pengiriman bertahan setelah retensi 30 hari
	// (3.24) menghapus event-nya.
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.table_constraints
		WHERE table_name = 'webhook_deliveries' AND constraint_type = 'FOREIGN KEY'
		  AND constraint_name LIKE '%event%'`).Scan(&count); err != nil {
		t.Fatalf("query constraint: %v", err)
	}
	if count != 0 {
		t.Fatalf("ada %d FK ke events; retensi 30 hari akan menghapus riwayat pengiriman", count)
	}
}

func TestPgWebhookMigrationIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply pertama: %v", err)
	}
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply kedua: %v", err)
	}
	// Tabelnya masih ada dan index-nya terpasang sekali.
	var idx int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE tablename IN ('webhooks','webhook_deliveries')`).Scan(&idx); err != nil {
		t.Fatalf("query index: %v", err)
	}
	if idx < 5 {
		t.Fatalf("index terpasang %d, want >= 5", idx)
	}
}

// TestPgWebhookEventsJSONMustBeArray menaku webhooks_events_chk.
func TestPgWebhookEventsJSONMustBeArray(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	orgID, boardID := seedWebhookFixture(t, ctx, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO webhooks (id, org_id, board_id, url, secret_enc, events_json, active)
		VALUES ($1, $2, $3, 'https://example.com/hook', $4, '{"bukan":"array"}'::jsonb, true)`,
		ulid.Must(), orgID, boardID, []byte("sealed"))
	if err == nil {
		t.Fatal("events_json objek diterima, seharusnya ditolak")
	}
	if !strings.Contains(err.Error(), "webhooks_events_chk") {
		t.Fatalf("error = %v, want menyebut webhooks_events_chk", err)
	}
}
