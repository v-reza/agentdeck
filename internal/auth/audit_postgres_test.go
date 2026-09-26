package auth

// Audit log + notifikasi terhadap Postgres nyata — ARCHITECTURE 3.17/3.21,
// 6.2.19, US-AD95, US-AD61.
//
// Empat hal di sini cuma bisa dibuktikan lawan database sungguhan:
//
//  1. `ListAuditLog` benar-benar menyaring lewat SQL. Terutama yang paling mudah
//     salah: filter yang TIDAK diisi harus berarti "semua", bukan "cocok dengan
//     string kosong". Sebelum memakai `sqlc.narg`, sqlc menebak parameternya
//     non-nullable, dan `actor = ''` akan mengembalikan nol baris untuk setiap
//     pemanggilan tanpa filter — jawaban yang salah tapi kelihatan benar.
//  2. Cursor paginasi benar-benar eksklusif (`id < cursor`), bukan inklusif.
//     Kalau inklusif, halaman kedua mengulang baris terakhir halaman pertama.
//  3. `notifications_kind_chk` dan `notifications_id_ulid_chk` benar-benar ada di
//     skema yang jalan, bukan cuma di file migrasi.
//  4. `MarkNotificationsRead` menyaring user dan org di SQL, jadi id milik
//     pengguna lain tidak pernah tersentuh walau id-nya benar.
//
// Fixture-nya mendaftarkan pengguna sungguhan karena `notifications_user_fk`
// menuntut user yang nyata dan `notifications_org_fk` menuntut org yang nyata.

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentdeck/internal/ulid"
)

// auditFixture mendaftarkan satu pengguna dan mengembalikan store, user, dan org.
func auditFixture(t *testing.T) (*Store, User, Workspace) {
	t.Helper()
	store := pgTestStore(t)
	email := uniqueEmail(t, "audit@example.com")
	user, workspace, _, err := store.Register(context.Background(), email, "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return store, user, workspace
}

// writeAuditRows menulis baris audit langsung, karena hanya satu aksi di seluruh
// sistem yang punya penulis audit (`org.rename`), dan tes filter butuh beberapa
// aksi dengan aktor berbeda.
func writeAuditRows(t *testing.T, ctx context.Context, orgID, actorID string, actions ...string) {
	t.Helper()
	for _, action := range actions {
		if _, err := pgPool(t).Exec(ctx,
			`INSERT INTO audit_log (org_id, actor_user_id, action, target_type, target_id, ip)
			 VALUES ($1, $2, $3, 'task', $4, '127.0.0.1')`,
			orgID, actorID, action, ulid.Must()); err != nil {
			t.Fatalf("insert audit row: %v", err)
		}
	}
}

// TestPgAuditLogUnfilteredReturnsEverything is the regression test for the
// sqlc.narg bug class: a zero AuditFilter must mean "no filter", not "match the
// empty string".
func TestPgAuditLogUnfilteredReturnsEverything(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	writeAuditRows(t, ctx, workspace.ID, user.ID, "board.create", "task.move", "board.delete")

	entries, err := store.AuditLog(ctx, workspace.ID, AuditFilter{})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("tanpa filter = %d baris, want 3", len(entries))
	}
	// Newest first: `ORDER BY id DESC`.
	if entries[0].Action != "board.delete" {
		t.Fatalf("baris pertama = %q, want board.delete (terbaru dulu)", entries[0].Action)
	}
	if entries[0].ActorUserID != user.ID {
		t.Fatalf("actor = %q, want %q", entries[0].ActorUserID, user.ID)
	}
}

// TestPgAuditLogFiltersAreApplied.
func TestPgAuditLogFiltersAreApplied(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	// Aktor kedua harus pengguna yang benar-benar ada: `audit_log` punya
	// `audit_log_actor_user_fk`, jadi ULID karangan ditolak database. Itu
	// constraint yang benar — catatan audit harus menunjuk aktor nyata.
	other, _, _, err := store.Register(ctx, uniqueEmail(t, "actor2@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register aktor kedua: %v", err)
	}
	writeAuditRows(t, ctx, workspace.ID, user.ID, "board.create", "task.move")
	writeAuditRows(t, ctx, workspace.ID, other.ID, "board.create")

	action := "board.create"
	entries, err := store.AuditLog(ctx, workspace.ID, AuditFilter{Action: &action})
	if err != nil {
		t.Fatalf("AuditLog(action): %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("filter action = %d baris, want 2", len(entries))
	}

	actor := other.ID
	entries, err = store.AuditLog(ctx, workspace.ID, AuditFilter{ActorUserID: &actor})
	if err != nil {
		t.Fatalf("AuditLog(actor): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("filter actor = %d baris, want 1", len(entries))
	}
	if entries[0].ActorUserID != other.ID {
		t.Fatalf("actor = %q, want %q", entries[0].ActorUserID, other.ID)
	}

	// Kombinasi: actor yang sama tapi aksi berbeda = nol baris.
	entries, err = store.AuditLog(ctx, workspace.ID, AuditFilter{ActorUserID: &actor, Action: &action})
	if err != nil {
		t.Fatalf("AuditLog(actor+action): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("filter gabungan = %d baris, want 1", len(entries))
	}
}

// TestPgAuditLogCursorIsExclusive — US-AD95's pagination.
//
// Menelusuri SEMUA halaman, bukan cuma dua, karena fixture mendaftarkan
// pengguna dan itu ikut menulis baris audit. Kalau tesnya mengasumsikan empat
// baris mengisi tepat dua halaman, jumlah baris lain di tabel membuatnya gagal
// karena alasan yang salah — dan itu memang yang terjadi pertama kali.
//
// Yang diukur: tidak ada baris yang muncul di dua halaman, dan keempat aksi yang
// ditulis benar-benar terjangkau. Kalau cursornya inklusif, halaman berikutnya
// mengulang baris terakhir halaman sebelumnya dan pemeriksaan pertama menangkapnya.
func TestPgAuditLogCursorIsExclusive(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	written := []string{"a.one", "a.two", "a.three", "a.four"}
	writeAuditRows(t, ctx, workspace.ID, user.ID, written...)

	seen := map[string]int{}
	var cursor int64
	for page := 0; page < 20; page++ {
		entries, err := store.AuditLog(ctx, workspace.ID, AuditFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("halaman %d: %v", page, err)
		}
		if len(entries) == 0 {
			break
		}
		for _, e := range entries {
			seen[e.Action]++
		}
		// Cursor untuk halaman berikutnya: id baris terakhir halaman ini. Karena
		// urutannya DESC, itu id terkecil di halaman — dibaca lewat query, sama
		// seperti handler membacanya dari baris terakhir respons.
		last := entries[len(entries)-1].Action
		if err := pgPool(t).QueryRow(ctx,
			`SELECT id FROM audit_log WHERE org_id = $1 AND action = $2 ORDER BY id DESC LIMIT 1`,
			workspace.ID, last).Scan(&cursor); err != nil {
			t.Fatalf("ambil cursor untuk %q: %v", last, err)
		}
	}

	for action, n := range seen {
		if n != 1 {
			t.Errorf("%q muncul %d kali lintas halaman — cursor harusnya eksklusif", action, n)
		}
	}
	for _, action := range written {
		if seen[action] != 1 {
			t.Errorf("%q muncul %d kali, want tepat 1", action, seen[action])
		}
	}
}

// TestPgAuditLogIsScopedToTheOrg — US-AD95 AC4 + isolasi tenant.
func TestPgAuditLogIsScopedToTheOrg(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	writeAuditRows(t, ctx, workspace.ID, user.ID, "mine.only")

	entries, err := store.AuditLog(ctx, ulid.Must(), AuditFilter{})
	if err != nil {
		t.Fatalf("AuditLog org lain: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("org lain melihat %d baris, want 0", len(entries))
	}
}

// TestPgNotificationsRoundTripAndBadge.
func TestPgNotificationsRoundTripAndBadge(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()

	created, err := store.Notify(ctx, Notification{
		UserID: user.ID, OrgID: workspace.ID, Kind: "run.failed",
		Title: "Run gagal", Body: "transient", TargetType: "run", TargetID: ulid.Must(),
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(created.ID) != 26 {
		t.Fatalf("id = %q, want ULID 26 karakter (notifications_id_ulid_chk)", created.ID)
	}
	if created.ReadAt != nil {
		t.Fatal("notifikasi baru langsung terbaca")
	}

	items, unread, err := store.Notifications(ctx, user.ID, workspace.ID, 0)
	if err != nil {
		t.Fatalf("Notifications: %v", err)
	}
	if len(items) != 1 || unread != 1 {
		t.Fatalf("items/unread = %d/%d, want 1/1", len(items), unread)
	}
	if items[0].TargetType != "run" || items[0].TargetID == "" {
		t.Fatalf("target = %q/%q, want run/<id> (US-AD61 AC2 deep-link)",
			items[0].TargetType, items[0].TargetID)
	}
}

// TestPgNotificationKindConstraintIsReal.
//
// Service menolak kind yang tidak dikenal, jadi jalur itu tidak pernah sampai ke
// SQL. Tes ini memanggil repository langsung: kalau `notifications_kind_chk`
// hilang dari migrasi, tes inilah yang memberitahu.
func TestPgNotificationKindConstraintIsReal(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	repo := store.repo

	_, err := repo.CreateNotification(ctx, Notification{
		ID: ulid.Must(), UserID: user.ID, OrgID: workspace.ID,
		Kind: "bukan.kind", Title: "x",
	})
	if err == nil {
		t.Fatal("kind tidak dikenal diterima SQL — notifications_kind_chk hilang")
	}

	// Dan id yang bukan ULID 26 karakter juga ditolak.
	_, err = repo.CreateNotification(ctx, Notification{
		ID: "pendek", UserID: user.ID, OrgID: workspace.ID,
		Kind: "run.failed", Title: "x",
	})
	if err == nil {
		t.Fatal("id pendek diterima SQL — notifications_id_ulid_chk hilang")
	}
}

// TestPgMarkNotificationsReadIsScopedToTheUser.
//
// Id yang benar tapi milik orang lain harus tidak tersentuh. Kalau predikat
// user_id hilang dari SQL, satu bug di service cukup untuk membiarkan siapa pun
// menghapus badge orang lain.
func TestPgMarkNotificationsReadIsScopedToTheUser(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()

	mine, err := store.Notify(ctx, Notification{
		UserID: user.ID, OrgID: workspace.ID, Kind: "budget.warning", Title: "milikku",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// Notifikasi kedua, milik pengguna lain di org yang sama.
	other, _, _, err := store.Register(ctx, uniqueEmail(t, "other@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	theirs, err := store.Notify(ctx, Notification{
		UserID: other.ID, OrgID: workspace.ID, Kind: "budget.warning", Title: "punya orang",
	})
	if err != nil {
		t.Fatalf("Notify other: %v", err)
	}

	// user menandai id MILIK ORANG LAIN. Nol baris harus berubah.
	changed, err := store.MarkNotificationsRead(ctx, user.ID, workspace.ID, []string{theirs.ID}, false)
	if err != nil {
		t.Fatalf("MarkNotificationsRead: %v", err)
	}
	if changed != 0 {
		t.Fatalf("menandai milik orang lain mengubah %d baris, want 0", changed)
	}
	_, unreadOther, err := store.Notifications(ctx, other.ID, workspace.ID, 0)
	if err != nil {
		t.Fatalf("Notifications other: %v", err)
	}
	if unreadOther != 1 {
		t.Fatalf("badge orang lain berubah jadi %d, want tetap 1", unreadOther)
	}

	// Yang miliknya sendiri: satu baris berubah.
	changed, err = store.MarkNotificationsRead(ctx, user.ID, workspace.ID, []string{mine.ID}, false)
	if err != nil {
		t.Fatalf("MarkNotificationsRead own: %v", err)
	}
	if changed != 1 {
		t.Fatalf("menandai milik sendiri mengubah %d baris, want 1", changed)
	}
	_, unreadMine, err := store.Notifications(ctx, user.ID, workspace.ID, 0)
	if err != nil {
		t.Fatalf("Notifications mine: %v", err)
	}
	if unreadMine != 0 {
		t.Fatalf("badge sendiri = %d setelah ditandai, want 0", unreadMine)
	}
}

// TestPgMarkNotificationsReadAllCoversOnlyUnread.
func TestPgMarkNotificationsReadAllCoversOnlyUnread(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.Notify(ctx, Notification{
			UserID: user.ID, OrgID: workspace.ID, Kind: "credential.invalid", Title: "kredensial",
		}); err != nil {
			t.Fatalf("Notify %d: %v", i, err)
		}
	}
	changed, err := store.MarkNotificationsRead(ctx, user.ID, workspace.ID, nil, true)
	if err != nil {
		t.Fatalf("MarkNotificationsRead(all): %v", err)
	}
	if changed != 3 {
		t.Fatalf("all menandai %d baris, want 3", changed)
	}
	// Panggilan kedua: nol, karena semuanya sudah terbaca.
	changed, err = store.MarkNotificationsRead(ctx, user.ID, workspace.ID, nil, true)
	if err != nil {
		t.Fatalf("MarkNotificationsRead(all) kedua: %v", err)
	}
	if changed != 0 {
		t.Fatalf("all kedua menandai %d baris, want 0", changed)
	}
}

// TestPgNotifyRejectsUnknownKindAndTarget.
func TestPgNotifyRejectsUnknownKindAndTarget(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()

	if _, err := store.Notify(ctx, Notification{
		UserID: user.ID, OrgID: workspace.ID, Kind: "bukan.kind", Title: "x",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("kind tidak dikenal = %v, want ErrInvalidInput", err)
	}
	if _, err := store.Notify(ctx, Notification{
		UserID: user.ID, OrgID: workspace.ID, Kind: "run.failed", Title: "x",
		TargetType: "bukan-target",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("target tidak dikenal = %v, want ErrInvalidInput", err)
	}
	if _, err := store.MarkNotificationsRead(ctx, user.ID, workspace.ID, nil, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("read tanpa ids/all = %v, want ErrInvalidInput", err)
	}
}

// TestPgNotificationsAreIsolatedPerOrg — US-AD61 AC4.
func TestPgNotificationsAreIsolatedPerOrg(t *testing.T) {
	store, user, workspace := auditFixture(t)
	ctx := context.Background()
	if _, err := store.Notify(ctx, Notification{
		UserID: user.ID, OrgID: workspace.ID, Kind: "run.failed", Title: "di org ini",
	}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// Org yang sama, pengguna yang sama, tapi org yang diminta berbeda: nol.
	items, unread, err := store.Notifications(ctx, user.ID, ulid.Must(), 0)
	if err != nil {
		t.Fatalf("Notifications org lain: %v", err)
	}
	if len(items) != 0 || unread != 0 {
		t.Fatalf("org lain melihat %d/%d, want 0/0", len(items), unread)
	}
	_ = time.Now
}

// TestPgNotifyOnceWritesToAdminsAndOwnersExactlyOnce is the producer's contract
// (US-AD61 AC1) and the reason the dedup lives in SQL.
//
// The dispatcher raises `budget.warning` from a check that runs after every
// step. Without the guard, one long run writes the same notice dozens of times
// and the badge stops meaning anything. This test writes the same notice twice
// and asserts the second is a no-op, then checks that a *different* target is
// still delivered — the dedup must be per-event, not a global mute.
func TestPgNotifyOnceWritesToAdminsAndOwnersExactlyOnce(t *testing.T) {
	store, owner, workspace := auditFixture(t)
	ctx := context.Background()

	// A member must not receive operational notices: they cannot act on them.
	member, _, _, err := store.Register(ctx, uniqueEmail(t, "member@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register member: %v", err)
	}
	if err := store.AddMember(ctx, workspace.ID, owner.Email, member.Email, Member); err != nil {
		t.Fatalf("add member: %v", err)
	}

	notice := Notification{
		OrgID: workspace.ID, Kind: "budget.warning",
		Title: "Board mendekati batas harian", Body: "80% dari cap",
		TargetType: "board", TargetID: workspace.ID,
	}

	written, err := store.NotifyOnce(ctx, notice)
	if err != nil {
		t.Fatalf("NotifyOnce: %v", err)
	}
	if written != 1 {
		t.Fatalf("penerima pertama = %d baris, want 1 (hanya owner)", written)
	}

	// Same event again: no new row.
	written, err = store.NotifyOnce(ctx, notice)
	if err != nil {
		t.Fatalf("NotifyOnce kedua: %v", err)
	}
	if written != 0 {
		t.Fatalf("kejadian yang sama ditulis %d baris lagi, want 0", written)
	}

	// A different target is a different event: still delivered.
	other := notice
	other.TargetID = ulid.Must()
	written, err = store.NotifyOnce(ctx, other)
	if err != nil {
		t.Fatalf("NotifyOnce target lain: %v", err)
	}
	if written != 1 {
		t.Fatalf("target berbeda = %d baris, want 1", written)
	}

	// The member got nothing; the owner got two (one per event).
	_, ownerUnread, err := store.Notifications(ctx, owner.ID, workspace.ID, 0)
	if err != nil {
		t.Fatalf("Notifications owner: %v", err)
	}
	if ownerUnread != 2 {
		t.Fatalf("owner punya %d belum dibaca, want 2", ownerUnread)
	}
	_, memberUnread, err := store.Notifications(ctx, member.ID, workspace.ID, 0)
	if err != nil {
		t.Fatalf("Notifications member: %v", err)
	}
	if memberUnread != 0 {
		t.Fatalf("member menerima %d notifikasi operasional, want 0", memberUnread)
	}
}

// TestPgOrgAdminsAndOwnersListsBothRoles.
func TestPgOrgAdminsAndOwnersListsBothRoles(t *testing.T) {
	store, owner, workspace := auditFixture(t)
	ctx := context.Background()

	admin, _, _, err := store.Register(ctx, uniqueEmail(t, "admin@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if err := store.AddMember(ctx, workspace.ID, owner.Email, admin.Email, Admin); err != nil {
		t.Fatalf("add admin: %v", err)
	}
	viewer, _, _, err := store.Register(ctx, uniqueEmail(t, "viewer@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	if err := store.AddMember(ctx, workspace.ID, owner.Email, viewer.Email, Viewer); err != nil {
		t.Fatalf("add viewer: %v", err)
	}

	got, err := store.repo.OrgAdminsAndOwners(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("OrgAdminsAndOwners: %v", err)
	}
	found := map[string]bool{}
	for _, id := range got {
		found[id] = true
	}
	if !found[owner.ID] || !found[admin.ID] {
		t.Fatalf("penerima = %v, want owner dan admin", got)
	}
	if found[viewer.ID] {
		t.Fatalf("viewer ikut jadi penerima: %v", got)
	}
	// Org lain: kosong.
	other, err := store.repo.OrgAdminsAndOwners(ctx, ulid.Must())
	if err != nil {
		t.Fatalf("OrgAdminsAndOwners org lain: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("org lain punya %d penerima, want 0", len(other))
	}
}
