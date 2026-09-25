package main

// US-AD59 — arsip task.
//
//	AC1 : arsip -> status `archived`, dan `ListBoardTasks` berhenti
//	      mengembalikannya (query-nya sendiri sudah `status != 'archived'`).
//	AC2 : arsip hanya sah dari status terminal (done/failed/cancelled);
//	      task aktif -> 409.
//	AC3 : mengarsipkan task yang sudah `archived` idempoten -> 200.
//	AC4 : arsip butuh minimal admin; member/viewer -> 403.
//
// Kenapa file ini ada: nol test menyentuh arsip task sebelum ini, dan tiga dari
// empat AC-nya memang tidak dipegang kode. Terukur sebelum perbaikan:
//
//	member -> 200  (AC4 bocor: route-nya Member, jadi siapa pun di atas viewer)
//	backlog -> 200 (AC2 bocor: `MoveTask` tidak melihat status tujuan sama sekali)
//
// Dua-duanya lewat `POST /tasks/{id}/move`, satu-satunya jalur arsip yang ada.
// `POST /tasks/{id}/archive` masih ⬜ di kontrak, jadi tidak diuji di sini.
//
// Mux-nya dari `registerBoardRoutes`, fungsi yang sama yang dipanggil main.go,
// jadi gerbang peran yang diuji di sini adalah gerbang produksi.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// seedTask menaruh satu task di board-a lewat repo, bukan lewat HTTP: status
// awalnya harus ditentukan test, dan `POST /boards/{id}/tasks` selalu membuat
// task `backlog`.
func (f columnFixture) seedTask(t *testing.T, id, orgID string, status board.TaskStatus) board.Task {
	t.Helper()
	task, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: id, OrgID: orgID, BoardID: f.boardA, Title: "Task " + id, Status: status,
	})
	if err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
	return task
}

// moveTask drives POST /tasks/{id}/move through the real route.
func (f columnFixture) moveTask(t *testing.T, actor, orgID, taskID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/move", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// TestArchiveTaskRequiresAdmin is AC4.
//
// `member` is the interesting actor: it is above the route's floor, so a
// Member-gated route answers 200 and the criterion silently fails. Only
// `owner`/`admin` may retire a task.
func TestArchiveTaskRequiresAdmin(t *testing.T) {
	// Every actor gets its own task. Sharing one would make the assertions
	// order-dependent: the first 200 retires the row, and every later actor
	// would then be reading the idempotent path instead of the gate.
	cases := []struct {
		actor string
		role  string
		want  int
	}{
		{"vera", "viewer", http.StatusForbidden},
		{"marta", "member", http.StatusForbidden},
		{"andre", "admin", http.StatusOK},
		{"alice", "owner", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			f := columnEditAPI(t)
			id := "task-" + tc.role
			f.seedTask(t, id, f.scenario.orgA, board.StatusDone)
			recorder := f.moveTask(t, tc.actor, f.scenario.orgA, id, `{"from":"done","to":"archived"}`)
			if recorder.Code != tc.want {
				t.Errorf("%s (%s) arsip task: status = %d, want %d (body %q)",
					tc.actor, tc.role, recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}
}

// TestArchiveTaskRefusesANonTerminalTask is AC2.
//
// A task that is still moving — backlog, ready, running — must not be retired:
// the row would leave the board while a run still holds it. 409, because the
// caller's payload is well-formed and the state is what refuses it.
func TestArchiveTaskRefusesANonTerminalTask(t *testing.T) {
	f := columnEditAPI(t)
	for _, status := range []board.TaskStatus{
		board.StatusBacklog, board.StatusReady, board.StatusRunning, board.StatusReview,
	} {
		id := "task-" + string(status)
		f.seedTask(t, id, f.scenario.orgA, status)
		recorder := f.moveTask(t, "andre", f.scenario.orgA, id, `{"from":"`+string(status)+`","to":"archived"}`)
		if recorder.Code != http.StatusConflict {
			t.Errorf("arsip task %s: status = %d, want 409 (body %q)", status, recorder.Code, recorder.Body.String())
		}
	}
}

// TestArchiveTaskIsIdempotent is AC3.
//
// Archiving a task that is already archived answers 200 with the current row.
// The `from` the caller states is the pre-archive status, so a retry cannot be
// expressed as `from: archived` — the server treats a target that equals the
// stored status as a no-op rather than a 409 on its own earlier write.
func TestArchiveTaskIsIdempotent(t *testing.T) {
	f := columnEditAPI(t)
	f.seedTask(t, "task-done", f.scenario.orgA, board.StatusDone)

	first := f.moveTask(t, "andre", f.scenario.orgA, "task-done", `{"from":"done","to":"archived"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("arsip pertama: status = %d, want 200 (body %q)", first.Code, first.Body.String())
	}
	second := f.moveTask(t, "andre", f.scenario.orgA, "task-done", `{"from":"done","to":"archived"}`)
	if second.Code != http.StatusOK {
		t.Errorf("arsip kedua: status = %d, want 200 (body %q)", second.Code, second.Body.String())
	}
}

// TestArchivedTaskLeavesTheBoardListing is AC1's observable half.
//
// The query already filters `status != 'archived'`, so this pins the behaviour
// end to end rather than the SQL: after the archive lands, the board's own list
// endpoint stops returning the row.
func TestArchivedTaskLeavesTheBoardListing(t *testing.T) {
	f := columnEditAPI(t)
	f.seedTask(t, "task-done", f.scenario.orgA, board.StatusDone)
	f.seedTask(t, "task-backlog", f.scenario.orgA, board.StatusBacklog)

	if recorder := f.moveTask(t, "andre", f.scenario.orgA, "task-done", `{"from":"done","to":"archived"}`); recorder.Code != http.StatusOK {
		t.Fatalf("arsip: status = %d (body %q)", recorder.Code, recorder.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/boards/"+f.boardA+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens["vera@x.test"])
	req.Header.Set("X-Org-ID", f.scenario.orgA)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list tasks: status = %d (body %q)", w.Code, w.Body.String())
	}

	got := decodeTaskTitles(t, w.Body.Bytes())
	for _, title := range got {
		if title == "Task task-done" {
			t.Errorf("task yang sudah diarsip masih ada di daftar board: %v", got)
		}
	}
	if len(got) != 1 {
		t.Errorf("jumlah task di board = %d (%v), want 1", len(got), got)
	}
}

// TestArchiveIsNotAMemberShortcutForOtherMoves is AC4's other edge: raising the
// floor for the archive target must not raise it for ordinary column moves,
// which the board's drag-and-drop depends on at Member level.
func TestArchiveIsNotAMemberShortcutForOtherMoves(t *testing.T) {
	f := columnEditAPI(t)
	f.seedTask(t, "task-backlog", f.scenario.orgA, board.StatusBacklog)

	recorder := f.moveTask(t, "marta", f.scenario.orgA, "task-backlog", `{"from":"backlog","to":"ready"}`)
	if recorder.Code != http.StatusOK {
		t.Errorf("member pindah kolom biasa: status = %d, want 200 (body %q)", recorder.Code, recorder.Body.String())
	}
}

// TestArchiveGateIsCheckedAfterAuthentication: an unauthenticated caller gets
// 401, not 403. The order matters — a role refusal on an anonymous request
// would tell a stranger that the task exists.
func TestArchiveGateIsCheckedAfterAuthentication(t *testing.T) {
	f := columnEditAPI(t)
	f.seedTask(t, "task-done", f.scenario.orgA, board.StatusDone)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-done/move", bytes.NewBufferString(`{"from":"done","to":"archived"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", f.scenario.orgA)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tanpa sesi: status = %d, want 401", w.Code)
	}
}

// decodeTaskTitles pulls just the titles out of a board task list response.
func decodeTaskTitles(t *testing.T, body []byte) []string {
	t.Helper()
	var rows []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode task list %q: %v", body, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}
