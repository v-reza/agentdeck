package main

// Handler komentar — ARCHITECTURE 6.2.17, US-AD42.
//
// Dua hal yang cuma bisa dibuktikan di lapisan ini:
//
//  1. Gerbang peran: viewer membaca, tidak menulis (AC2). Itu hidup di tabel
//     route, bukan di service, jadi unit test service tidak akan menangkapnya.
//  2. Bentuk respons 400 vs 404. Body kosong dan body kepanjangan dua-duanya 400
//     (AC3), sedangkan komentar milik orang lain dan komentar yang tidak ada
//     dua-duanya 404 — pemanggil tidak boleh bisa membedakan keduanya.
//
// Kepemilikan komentar diuji lawan Postgres di internal/board/comment_test.go;
// di sini repo-nya stub, jadi yang diukur adalah permukaan HTTP-nya.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"agentdeck/internal/board"
)

// stubCommentRepo melengkapi stubRunRepo dengan tabel komentar in-memory.
//
// Sengaja permisif di satu tempat saja: `ListTaskComments` tidak memfilter org,
// sama seperti SQL aslinya yang memfilter lewat `task_id AND org_id` di klausa
// WHERE — di sini task-nya sudah dicek lebih dulu oleh `GetTask`, jadi efeknya
// sama. Kalau service lupa memanggil GetTask, isolasi tenant-nya hilang, dan itu
// memang yang ingin ditangkap.
func (s *stubRunRepo) CreateComment(_ context.Context, c board.Comment) (board.Comment, error) {
	s.commentSeq++
	c.ID = s.commentSeq
	s.comments = append(s.comments, c)
	return c, nil
}

func (s *stubRunRepo) GetComment(_ context.Context, id int64, orgID string) (board.Comment, error) {
	for _, c := range s.comments {
		if c.ID == id && c.OrgID == orgID {
			return c, nil
		}
	}
	return board.Comment{}, board.ErrNotFound
}

func (s *stubRunRepo) ListTaskComments(_ context.Context, taskID, orgID string) ([]board.Comment, error) {
	out := []board.Comment{}
	for _, c := range s.comments {
		if c.TaskID == taskID && c.OrgID == orgID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *stubRunRepo) UpdateCommentBody(_ context.Context, id int64, orgID, authorUserID, body string) (board.Comment, error) {
	for i, c := range s.comments {
		if c.ID == id && c.OrgID == orgID && c.AuthorUserID == authorUserID {
			s.comments[i].Body = body
			return s.comments[i], nil
		}
	}
	return board.Comment{}, board.ErrNotFound
}

func (s *stubRunRepo) DeleteComment(_ context.Context, id int64, orgID, authorUserID string) (bool, error) {
	for i, c := range s.comments {
		if c.ID == id && c.OrgID == orgID && c.AuthorUserID == authorUserID {
			s.comments = append(s.comments[:i], s.comments[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func TestCreateCommentAcceptsAMemberAndRefusesAViewer(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	// AC2: viewer membaca, tidak menulis.
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/comments", "vera", scenario.orgA, `{"body":"halo"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer menulis komentar = %d, want 403", w.Code)
	}
	if len(repo.comments) != 0 {
		t.Fatalf("komentar viewer tersimpan: %+v", repo.comments)
	}

	w = runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/comments", "marta", scenario.orgA, `{"body":"halo"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("member menulis komentar = %d (%s), want 201", w.Code, w.Body.String())
	}
	if len(repo.comments) != 1 {
		t.Fatalf("komentar tersimpan %d, want 1", len(repo.comments))
	}
	// AC1: author_user_id terisi, author_agent_id tidak.
	if repo.comments[0].AuthorUserID == "" || repo.comments[0].AuthorAgentID != "" {
		t.Fatalf("author = %q/%q, want user terisi dan agent kosong",
			repo.comments[0].AuthorUserID, repo.comments[0].AuthorAgentID)
	}
}

// TestCommentBodyLimitsAre400 — US-AD42 AC3.
//
// Ketiga bentuk ditolak dengan status yang sama, dan yang penting: kosong dan
// kepanjangan dua-duanya 400, bukan 500. Spasi-saja masuk akal untuk ditolak
// sebagai kosong — CHECK di tabel pun `btrim(body) <> ”`.
func TestCommentBodyLimitsAre400(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	cases := []struct {
		name string
		body string
	}{
		{"kosong", `{"body":""}`},
		{"spasi saja", `{"body":"   "}`},
		{"kepanjangan", `{"body":"` + strings.Repeat("a", board.MaxCommentBody+1) + `"}`},
	}
	for _, tc := range cases {
		w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/comments", "marta", scenario.orgA, tc.body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d (%s), want 400", tc.name, w.Code, w.Body.String())
		}
	}
	if len(repo.comments) != 0 {
		t.Fatalf("komentar ditolak tapi tersimpan %d", len(repo.comments))
	}

	// Batasnya inklusif: tepat MaxCommentBody diterima. Kalau tes ini hilang,
	// "off by one" pada batas tidak akan ketahuan sama sekali.
	ok := `{"body":"` + strings.Repeat("a", board.MaxCommentBody) + `"}`
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/comments", "marta", scenario.orgA, ok)
	if w.Code != http.StatusCreated {
		t.Fatalf("body tepat di batas = %d (%s), want 201", w.Code, w.Body.String())
	}
}

// TestCommentBodyCountsRunesNotBytes.
//
// MaxCommentBody dihitung dengan rune. Kalau dihitung byte, komentar berbahasa
// Indonesia yang penuh karakter non-ASCII akan ditolak jauh sebelum batas yang
// tertulis, dan pesan errornya tidak akan menjelaskan kenapa.
func TestCommentBodyCountsRunesNotBytes(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	// "é" 2 byte; MaxCommentBody rune = 2x byte-nya.
	body := `{"body":"` + strings.Repeat("é", board.MaxCommentBody) + `"}`
	if len(body) <= board.MaxCommentBody {
		t.Fatal("fixture tidak lebih panjang dalam byte daripada batas rune — tes tidak mengukur apa pun")
	}
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/comments", "marta", scenario.orgA, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("body %d rune = %d (%s), want 201", board.MaxCommentBody, w.Code, w.Body.String())
	}
}

func TestCommentListIsReadableByAViewer(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.comments = append(repo.comments, board.Comment{
		ID: 1, OrgID: scenario.orgA, TaskID: "task-1", AuthorUserID: "marta@x.test", Body: "catatan",
	})
	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/tasks/task-1/comments", "vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("viewer membaca komentar = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "catatan") {
		t.Fatalf("body = %s, want komentar di dalamnya", w.Body.String())
	}
}

// TestCommentEditAndDeleteAreAuthorOnly.
//
// Bentuk 404-nya yang penting: komentar milik orang lain harus menjawab hal yang
// sama persis dengan komentar yang tidak ada. Kalau yang pertama 403, endpoint
// ini jadi alat untuk menebak komentar siapa yang ada di workspace.
func TestCommentEditAndDeleteAreAuthorOnly(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.comments = append(repo.comments, board.Comment{
		ID: 7, OrgID: scenario.orgA, TaskID: "task-1", AuthorUserID: scenario.userIDs["marta@x.test"], Body: "asli",
	})

	// bella menulisnya, jadi bella boleh mengedit.
	w := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/comments/7", "marta", scenario.orgA, `{"body":"diubah"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("penulis mengedit = %d (%s), want 200", w.Code, w.Body.String())
	}
	if repo.comments[0].Body != "diubah" {
		t.Fatalf("body = %q, want diubah", repo.comments[0].Body)
	}

	// andre bukan penulisnya — dan admin, jadi dia lolos gerbang peran Member
	// dan yang menolaknya memang cek kepemilikan, bukan gerbang peran.
	w = runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/comments/7", "andre", scenario.orgA, `{"body":"bajakan"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("bukan penulis mengedit = %d (%s), want 404", w.Code, w.Body.String())
	}
	if repo.comments[0].Body != "diubah" {
		t.Fatalf("body berubah jadi %q oleh bukan penulis", repo.comments[0].Body)
	}

	w = runRequest(t, mux, scenario, http.MethodDelete, "/api/v1/comments/7", "andre", scenario.orgA, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("bukan penulis menghapus = %d, want 404", w.Code)
	}
	if len(repo.comments) != 1 {
		t.Fatalf("komentar terhapus oleh bukan penulis")
	}

	w = runRequest(t, mux, scenario, http.MethodDelete, "/api/v1/comments/7", "marta", scenario.orgA, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("penulis menghapus = %d, want 204", w.Code)
	}
	if len(repo.comments) != 0 {
		t.Fatalf("komentar masih ada setelah dihapus penulis")
	}
}

// TestCommentRoutesRejectAMissingComment: id yang tidak ada = 404, dan id yang
// bukan angka = 400 (bukan 500 — route-nya cocok, jadi ini kesalahan pemanggil).
func TestCommentRoutesRejectAMissingComment(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	w := runRequest(t, mux, scenario, http.MethodDelete, "/api/v1/comments/999", "marta", scenario.orgA, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("hapus komentar tidak ada = %d, want 404", w.Code)
	}
	w = runRequest(t, mux, scenario, http.MethodDelete, "/api/v1/comments/abc", "marta", scenario.orgA, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("hapus id non-numerik = %d, want 400", w.Code)
	}
}
