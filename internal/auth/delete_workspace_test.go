package auth

// DELETE /orgs/{id} terhadap Postgres nyata.
//
// Yang tidak bisa dibuktikan fake in-memory: bahwa penutupannya benar-benar
// SOFT di database. `SoftDeleteOrg` adalah satu statement UPDATE, dan tes
// in-memory hanya melihat map-nya — ia tidak akan tahu kalau statement-nya
// keliru menulis DELETE, atau kalau `deleted_at` ditulis tapi penyaring
// `deleted_at IS NULL` di jalur baca lain lupa dipasang.
//
// Gated pada AGENTDECK_TEST_DATABASE_URL seperti suite ini.

import (
	"context"
	"errors"
	"testing"
)

// TestPgDeleteWorkspaceIsSoftAndIdempotent.
func TestPgDeleteWorkspaceIsSoftAndIdempotent(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, ws, token, err := store.Register(ctx, uniqueEmail(t, "close@example.com"),
		"password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Soft: barisnya masih ada, dengan tanda tutupnya. Hard delete membuat
	// jendela pemulihan 30 hari US-AD98 AC5 mustahil.
	closed, err := store.OrgIncludingDeleted(ctx, ws.ID)
	if err != nil {
		t.Fatalf("baris org hilang setelah DELETE — itu hard delete: %v", err)
	}
	if closed.DeletedAt == nil {
		t.Fatal("orgs.deleted_at kosong setelah DELETE")
	}

	// Workspace yang ditutup berhenti resolve lewat jalur hidup...
	if _, _, err := store.GetOrg(ctx, ws.ID, alice.Email); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("GetOrg pada org tertutup = %v, want ErrWorkspaceNotFound", err)
	}
	// ...dan tidak muncul lagi di daftar milik anggotanya.
	orgs, err := store.Workspaces(ctx, alice.Email)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	for _, org := range orgs {
		if org.WorkspaceID == ws.ID {
			t.Fatal("workspace tertutup masih muncul di daftar org milik anggotanya")
		}
	}

	// Idempoten: permintaan kedua sukses, bukan ErrWorkspaceNotFound.
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace kedua = %v, want nil (idempoten)", err)
	}

	// Sesi akun tetap hidup: yang ditutup workspace-nya, bukan akunnya
	// (beda dari US-AD98 CloseAccount, yang mencabut seluruh sesi).
	if _, ok := store.Authenticate(ctx, token); !ok {
		t.Fatal("menutup satu workspace mencabut sesi akunnya")
	}
}

// TestPgDeleteWorkspaceKeepsTheData — yang membuat soft delete berarti: baris
// di bawahnya tidak ikut hilang. Kalau cascade-nya jalan, "pulihkan dalam 30
// hari" cuma bisa mengembalikan cangkang kosong.
func TestPgDeleteWorkspaceKeepsTheData(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, ws, _, err := store.Register(ctx, uniqueEmail(t, "keep@example.com"),
		"password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Keanggotaan owner-nya masih ada di DB. Dibaca langsung karena Store tidak
	// mengekspos anggota org yang sudah ditutup — dan memang tidak seharusnya.
	var memberships int
	if err := pgPool(t).QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE org_id = $1`, ws.ID).Scan(&memberships); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships == 0 {
		t.Fatal("keanggotaan ikut terhapus: penutupan ini hard, bukan soft")
	}
}

// TestPgDeleteWorkspaceIsOwnerOnly — matriks RBAC 11.3 di jalur produksi.
func TestPgDeleteWorkspaceIsOwnerOnly(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, ws, _, err := store.Register(ctx, uniqueEmail(t, "own@example.com"),
		"password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	adminEmail := uniqueEmail(t, "adm@example.com")
	if err := store.AddMember(ctx, ws.ID, alice.Email, adminEmail, Admin); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	memberEmail := uniqueEmail(t, "mem@example.com")
	if err := store.AddMember(ctx, ws.ID, alice.Email, memberEmail, Member); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	for _, actor := range []string{adminEmail, memberEmail} {
		if err := store.DeleteWorkspace(ctx, ws.ID, actor); !errors.Is(err, ErrForbidden) {
			t.Fatalf("DeleteWorkspace oleh %s = %v, want ErrForbidden", actor, err)
		}
	}
	// Dan workspace-nya masih hidup setelah dua penolakan.
	if _, _, err := store.GetOrg(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("workspace tidak selamat dari penolakan: %v", err)
	}
}

// TestPgDeleteWorkspaceIsScopedToItsTenant — owner org lain tidak bisa
// menutup workspace yang bukan miliknya, dan jawabannya ErrForbidden (403),
// bukan ErrWorkspaceNotFound: org-nya ada, pemanggilnya yang tidak berhak.
func TestPgDeleteWorkspaceIsScopedToItsTenant(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	_, wsA, _, err := store.Register(ctx, uniqueEmail(t, "a@example.com"),
		"password123", "A", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	bob, _, _, err := store.Register(ctx, uniqueEmail(t, "b@example.com"),
		"password123", "B", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register B: %v", err)
	}

	if err := store.DeleteWorkspace(ctx, wsA.ID, bob.Email); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteWorkspace lintas-tenant = %v, want ErrForbidden", err)
	}
	// Unknown id tetap 404: itu beda dari "ada tapi bukan milikmu".
	if err := store.DeleteWorkspace(ctx, "01ZZZZZZZZZZZZZZZZZZZZZZZZZZ", bob.Email); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("DeleteWorkspace id tak dikenal = %v, want ErrWorkspaceNotFound", err)
	}
}

// TestPgDeleteWorkspaceBumpsDeletedAtOnlyOnce — idempoten juga berarti tidak
// memperbarui cap waktunya: kalau `deleted_at` ditulis ulang di request kedua,
// jendela pemulihan 30 hari ikut bergeser tiap kali ada yang menekan tombolnya
// lagi, dan workspace itu tidak pernah benar-benar dihapus permanen.
func TestPgDeleteWorkspaceBumpsDeletedAtOnlyOnce(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, ws, _, err := store.Register(ctx, uniqueEmail(t, "once@example.com"),
		"password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	first, err := store.OrgIncludingDeleted(ctx, ws.ID)
	if err != nil {
		t.Fatalf("OrgIncludingDeleted: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace kedua: %v", err)
	}
	second, err := store.OrgIncludingDeleted(ctx, ws.ID)
	if err != nil {
		t.Fatalf("OrgIncludingDeleted kedua: %v", err)
	}
	if first.DeletedAt == nil || second.DeletedAt == nil {
		t.Fatal("deleted_at kosong")
	}
	if !first.DeletedAt.Equal(*second.DeletedAt) {
		t.Fatalf("deleted_at bergeser di request kedua: %v -> %v", first.DeletedAt, second.DeletedAt)
	}
}

// TestPgDeleteWorkspaceStaysIdempotentWithoutTheMembershipRow.
//
// Guard "sudah tertutup -> sukses" di service terlihat redundan: statement-nya
// sudah punya `AND deleted_at IS NULL`, jadi penulisan kedua memang tidak
// mengubah apa pun. Yang membedakannya adalah keanggotaan. Sebuah org yang
// ditutup lalu dibersihkan operator (atau akun pemiliknya sendiri ditutup,
// US-AD98 AC2) bisa kehilangan baris membership-nya; tanpa guard itu, DELETE
// berikutnya jatuh ke pemeriksaan peran dan menjawab 403 — untuk workspace yang
// sudah tertutup, di route yang kontraknya bilang idempoten.
func TestPgDeleteWorkspaceStaysIdempotentWithoutTheMembershipRow(t *testing.T) {
	store := pgTestStore(t)
	ctx := context.Background()

	alice, ws, _, err := store.Register(ctx, uniqueEmail(t, "orphan@example.com"),
		"password123", "Alice", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Operator (atau penutupan akun pemiliknya) membersihkan keanggotaannya.
	if _, err := pgPool(t).Exec(ctx,
		`DELETE FROM memberships WHERE org_id = $1`, ws.ID); err != nil {
		t.Fatalf("bersihkan memberships: %v", err)
	}

	// Request kedua harus tetap sukses. Jawaban yang benar bukan 403: org-nya
	// sudah tertutup, tidak ada izin yang tersisa untuk diperiksa.
	if err := store.DeleteWorkspace(ctx, ws.ID, alice.Email); err != nil {
		t.Fatalf("DeleteWorkspace tanpa baris membership = %v, want nil (idempoten)", err)
	}
}
