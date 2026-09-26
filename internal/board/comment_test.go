package board

// Komentar terhadap Postgres nyata — ARCHITECTURE 3.16, 6.2.17, US-AD42.
//
// Empat hal di sini cuma bisa dibuktikan lawan database sungguhan:
//
//  1. `comments_author_chk` benar-benar menolak baris dengan dua penulis atau
//     nol penulis. Stub tidak punya constraint, jadi tanpa tes ini kolomnya
//     bisa saja salah diisi dan tidak ada yang tahu.
//  2. `body` tersimpan apa adanya — spasi di ujung dipangkas service, isi di
//     tengah tidak boleh berubah.
//  3. Predikat penulis di `UpdateCommentBody`/`DeleteComment` benar-benar ada di
//     SQL, bukan cuma di service. Ini pertahanan lini kedua: kalau service lupa
//     memeriksa, SQL tetap harus menolak.
//  4. `comment.created` masuk ke `events` — kind itu ada di CHECK tabel events,
//     dan timeline yang tidak mencatat komentar adalah timeline yang bohong.
//
// Fixture-nya memakai runtimeFixture (task yang benar-benar ada, org yang benar-
// benar ada) karena `comments_task_fk` menuntut task yang nyata.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agentdeck/internal/ulid"
)

// commentFixture mengembalikan fixture task yang siap dikomentari.
func commentFixture(t *testing.T) runtimeFixture {
	t.Helper()
	f := newRuntimeFixture(t, "transient_only", 3)
	f.start(t)
	return f
}

func TestPgCreateCommentStoresBothAuthorShapes(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()

	// Jalur user.
	fromUser, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, "halo dari user",
		CommentAuthor{UserID: "user-1"})
	if err != nil {
		t.Fatalf("CreateComment (user): %v", err)
	}
	if fromUser.AuthorUserID != "user-1" || fromUser.AuthorAgentID != "" {
		t.Fatalf("author = %q/%q, want user-1/''", fromUser.AuthorUserID, fromUser.AuthorAgentID)
	}

	// Jalur agent — kolomnya nullable, dan DECISIONS §6 mengizinkannya. Kalau ini
	// gagal, worker tidak akan pernah bisa meninggalkan catatan di task.
	fromAgent, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, "halo dari agent",
		CommentAuthor{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("CreateComment (agent): %v", err)
	}
	if fromAgent.AuthorAgentID != "agent-1" || fromAgent.AuthorUserID != "" {
		t.Fatalf("author = %q/%q, want ''/agent-1", fromAgent.AuthorUserID, fromAgent.AuthorAgentID)
	}

	// Keduanya terbaca, berurutan.
	list, err := f.svc.ListComments(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("komentar %d, want 2", len(list))
	}
	if list[0].ID >= list[1].ID {
		t.Fatalf("urutan id %d,%d — want menaik", list[0].ID, list[1].ID)
	}
}

// TestPgCommentAuthorConstraintIsReal — comments_author_chk.
//
// Service menolak dua-penulis/nol-penulis dengan ErrInvalidInput, jadi jalur itu
// tidak pernah sampai ke SQL. Tes ini memanggil repository langsung supaya yang
// diukur memang constraint-nya: kalau constraint-nya hilang dari migrasi, tes ini
// yang memberitahu, bukan produksi.
func TestPgCommentAuthorConstraintIsReal(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()
	repo := f.svc.repo

	_, err := repo.CreateComment(ctx, Comment{
		OrgID: f.orgID, TaskID: f.taskID,
		AuthorUserID: "user-1", AuthorAgentID: "agent-1", Body: "dua penulis",
	})
	if err == nil {
		t.Fatal("dua penulis diterima SQL — comments_author_chk hilang")
	}

	_, err = repo.CreateComment(ctx, Comment{
		OrgID: f.orgID, TaskID: f.taskID, Body: "nol penulis",
	})
	if err == nil {
		t.Fatal("nol penulis diterima SQL — comments_author_chk hilang")
	}
}

func TestPgCommentBodyRoundTripsUnchanged(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()

	// Baris baru, tanda kutip, unicode. `body` itu TEXT, bukan JSONB — jadi
	// tidak ada normalisasi yang boleh terjadi.
	body := "baris satu\nbaris \"dua\" — 日本語 ok"
	created, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, body, CommentAuthor{UserID: "user-1"})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if created.Body != body {
		t.Fatalf("body tersimpan %q, want %q", created.Body, body)
	}

	list, err := f.svc.ListComments(ctx, f.taskID, f.orgID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(list) != 1 || list[0].Body != body {
		t.Fatalf("body terbaca %q, want %q", list[0].Body, body)
	}
}

// TestPgCommentWriteIsAuthorScopedInSQL.
//
// Service sudah memeriksa kepemilikan sebelum menulis, jadi tes ini sengaja
// memanggil repository langsung: yang diukur adalah predikat penulis di SQL.
// Tanpa predikat itu, satu bug di service cukup untuk membiarkan anggota mana pun
// mengubah komentar orang lain, dan tidak ada lapisan kedua yang menahan.
func TestPgCommentWriteIsAuthorScopedInSQL(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()
	repo := f.svc.repo

	comment, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, "asli", CommentAuthor{UserID: "user-1"})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	// Penulis lain: nol baris, dan pesannya ErrNotFound.
	if _, err := repo.UpdateCommentBody(ctx, comment.ID, f.orgID, "user-2", "bajakan"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateCommentBody oleh bukan penulis = %v, want ErrNotFound", err)
	}
	if deleted, err := repo.DeleteComment(ctx, comment.ID, f.orgID, "user-2"); err != nil || deleted {
		t.Fatalf("DeleteComment oleh bukan penulis = %v/%v, want false/nil", deleted, err)
	}
	// Dan isinya benar-benar tidak berubah.
	stored, err := repo.GetComment(ctx, comment.ID, f.orgID)
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if stored.Body != "asli" {
		t.Fatalf("body = %q setelah percobaan edit orang lain", stored.Body)
	}

	// Org lain: juga tidak boleh. Ini isolasi tenant di lapisan SQL.
	if _, err := repo.UpdateCommentBody(ctx, comment.ID, "org-lain", "user-1", "lintas tenant"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateCommentBody lintas org = %v, want ErrNotFound", err)
	}

	// Penulisnya sendiri: boleh.
	updated, err := f.svc.EditComment(ctx, comment.ID, f.orgID, "user-1", "diubah")
	if err != nil {
		t.Fatalf("EditComment oleh penulis: %v", err)
	}
	if updated.Body != "diubah" {
		t.Fatalf("body = %q, want diubah", updated.Body)
	}
	if err := f.svc.DeleteComment(ctx, comment.ID, f.orgID, "user-1"); err != nil {
		t.Fatalf("DeleteComment oleh penulis: %v", err)
	}
	// Sudah terhapus: 404 lagi.
	if err := f.svc.DeleteComment(ctx, comment.ID, f.orgID, "user-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hapus dua kali = %v, want ErrNotFound", err)
	}
}

// TestPgCommentRecordsTheTimelineEvent — ARCHITECTURE 3.15.
//
// `comment.created` ada di CHECK tabel events. Timeline yang tidak mencatat
// komentar berarti percakapan hilang dari riwayat task, dan itu justru satu-
// satunya alasan orang membuka timeline.
func TestPgCommentRecordsTheTimelineEvent(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()

	if _, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, "catat ini", CommentAuthor{UserID: "user-1"}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	events, err := f.svc.TaskHistory(ctx, f.taskID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Kind == "comment.created" {
			found = true
			if !strings.Contains(string(e.PayloadJSON), "catat ini") {
				t.Errorf("payload event = %s, want kutipan komentar", e.PayloadJSON)
			}
		}
	}
	if !found {
		t.Fatalf("nol event comment.created di timeline (%d event)", len(events))
	}
}

// TestPgCommentRefusesAnUnknownTask.
//
// Task yang tidak ada harus 404 lewat pemetaan ErrNoRows, bukan diterima dengan
// komentar menggantung. `comments_task_fk` akan menolaknya juga, tapi lewat
// error constraint — yang sampai ke pemanggil sebagai 500, bukan 404.
func TestPgCommentRefusesAnUnknownTask(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()

	_, err := f.svc.CreateComment(ctx, "01"+ulid.Must()[2:], f.orgID, "ke mana saja", CommentAuthor{UserID: "user-1"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("komentar pada task tak dikenal = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.ListComments(ctx, "01"+ulid.Must()[2:], f.orgID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list komentar task tak dikenal = %v, want ErrNotFound", err)
	}
}

// TestPgCommentListIsScopedToTheOrg.
func TestPgCommentListIsScopedToTheOrg(t *testing.T) {
	f := commentFixture(t)
	ctx := context.Background()

	if _, err := f.svc.CreateComment(ctx, f.taskID, f.orgID, "milik org A", CommentAuthor{UserID: "user-1"}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	// Task yang sama, org yang berbeda: 404 dari GetTask, bukan daftar kosong.
	// Daftar kosong akan mengonfirmasi bahwa task-nya ada di suatu tempat.
	if _, err := f.svc.ListComments(ctx, f.taskID, "org-lain"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list komentar dari org lain = %v, want ErrNotFound", err)
	}
}
