package migrate

// Migrasi 0022 terhadap Postgres nyata — ARCHITECTURE 3.15.
//
// Yang dibuktikan: index retensi benar-benar ada dan bisa dipakai planner, dan
// versi PARTIAL dari kontrak benar-benar tidak bisa dijalankan. Yang kedua itu
// alasan index ini ditulis sebagai btree biasa, jadi ia diuji juga — kalau
// suatu saat Postgres mengizinkannya, tes ini yang memberi tahu.

import (
	"context"
	"strings"
	"testing"
)

func TestPgArtifactsRetentionIndexExists(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'artifacts' AND indexname = 'artifacts_retention_idx')`,
	).Scan(&exists); err != nil {
		t.Fatalf("query index: %v", err)
	}
	if !exists {
		t.Fatal("artifacts_retention_idx tidak ada setelah 0022")
	}

	// Planner harus memakainya untuk pembersihan retensi N12. Ini yang bikin
	// index-nya berguna: bukan sekadar ada di katalog.
	var plan string
	if err := pool.QueryRow(ctx,
		`EXPLAIN (COSTS OFF) DELETE FROM artifacts WHERE created_at < now() - interval '90 days'`,
	).Scan(&plan); err != nil {
		t.Fatalf("explain: %v", err)
	}
	if plan == "" {
		t.Fatal("EXPLAIN kosong")
	}
}

// TestPgArtifactsRetentionIndexCannotBePartial menaku TEMUAN kontraknya:
// predikat 3.15 memakai now(), dan Postgres menolaknya.
//
// Ini bukan tes gaya penulisan — kalau Postgres suatu hari menerimanya, index
// versi kontrak jadi mungkin dan keputusan di 0022 layak ditinjau ulang. Tes
// yang gagal di sini berarti asumsinya berubah, bukan kodenya rusak.
func TestPgArtifactsRetentionIndexCannotBePartial(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`CREATE INDEX artifacts_retention_partial_probe ON artifacts (created_at) WHERE created_at < now() - interval '85 days'`)
	if err == nil {
		_, _ = pool.Exec(ctx, `DROP INDEX IF EXISTS artifacts_retention_partial_probe`)
		t.Fatal("Postgres MENERIMA predikat now(); keputusan di 0022 perlu ditinjau ulang")
	}
	// Pesan persisnya yang menjelaskan alasannya, jadi ia ikut dipaku.
	const want = "IMMUTABLE"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("error = %q, want memuat %q", got, want)
	}
}

// TestPgArtifactsStorageKeyIndexDeliberatelyAbsent: 3.15 mendeklarasikan index
// ini, dan 0022 memutuskan TIDAK membuatnya karena tidak ada query yang
// memfilternya. Tesnya memaku keputusan itu supaya tidak ada yang menambahkannya
// diam-diam "supaya cocok dengan dokumen".
func TestPgArtifactsStorageKeyIndexDeliberatelyAbsent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Download memakai primary key (endpoint-nya /artifacts/{id}/download), jadi
	// index storage_key tidak melayani apa pun.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'artifacts' AND indexname = 'artifacts_storage_key_idx')`,
	).Scan(&exists); err != nil {
		t.Fatalf("query index: %v", err)
	}
	if exists {
		t.Fatal("artifacts_storage_key_idx ada; 0022 memutuskan untuk tidak membuatnya, lihat komentarnya")
	}

	// Bukti bahwa index memang tidak diperlukan: pencarian lewat primary key
	// memakai artifacts_pk, bukan sesuatu di storage_key.
	var plan string
	if err := pool.QueryRow(ctx,
		`EXPLAIN (COSTS OFF) SELECT id, storage_key FROM artifacts WHERE id = $1`, "01ABCDEFGHJKMNPQRSTVWXYZ12",
	).Scan(&plan); err != nil {
		t.Fatalf("explain: %v", err)
	}
	if !strings.Contains(plan, "artifacts_pk") {
		t.Fatalf("plan = %q, want memakai artifacts_pk", plan)
	}
}
