package main

// Handler approval — ARCHITECTURE 6.2.14.
//
// Yang diuji di sini adalah gerbang perannya dan bentuk responsnya, karena
// keduanya adalah bagian dari kontrak yang tidak kelihatan dari service:
//
//  - 11.3 bilang approve/reject itu Admin. PRD US-AD34 AC3 bilang member boleh,
//    dan itu ditolak dengan alasan yang tercatat di internal/board/approval.go:
//    route pembuat gate-nya Member, jadi member yang boleh memutuskan bisa
//    membuka dan menutup gate-nya sendiri.
//  - `viewer` tidak boleh membuat gate (US-AD33 AC3).
//  - `preview_json` harus dikembalikan apa adanya. Server yang mem-parse lalu
//    men-serialisasi ulang bisa menampilkan diff yang berbeda dari yang
//    diusulkan worker — jadi tes ini membandingkan byte-nya, bukan field-nya.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"agentdeck/internal/board"
)

func (s *stubRunRepo) CreateApproval(_ context.Context, a board.Approval) (board.Approval, error) {
	if s.approvals == nil {
		s.approvals = map[string]board.Approval{}
	}
	if a.ExpiresAt.IsZero() {
		a.ExpiresAt = time.Now().Add(board.ApprovalWindow)
	}
	a.CreatedAt = time.Now()
	s.approvals[a.ID] = a
	return a, nil
}

func (s *stubRunRepo) GetApproval(_ context.Context, id, orgID string) (board.Approval, error) {
	a, ok := s.approvals[id]
	// Cermin `WHERE id = $1 AND org_id = $2`: gate tenant lain adalah nol baris.
	if !ok || a.OrgID != orgID {
		return board.Approval{}, board.ErrNotFound
	}
	return a, nil
}

func (s *stubRunRepo) ListPendingApprovals(_ context.Context, orgID string) ([]board.Approval, error) {
	out := []board.Approval{}
	for _, a := range s.approvals {
		if a.OrgID == orgID && a.Decision == board.ApprovalPending && a.ExpiresAt.After(time.Now()) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *stubRunRepo) ListTaskApprovals(_ context.Context, taskID, orgID string) ([]board.Approval, error) {
	out := []board.Approval{}
	for _, a := range s.approvals {
		if a.TaskID == taskID && a.OrgID == orgID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *stubRunRepo) DecideApproval(_ context.Context, id, orgID string, decision board.ApprovalDecision, decidedBy, reason string) (bool, error) {
	a, ok := s.approvals[id]
	// Predikat `decision = 'pending'` ada di dalam UPDATE, jadi double-nya harus
	// menirunya di sini — bukan memeriksa di pemanggil.
	if !ok || a.OrgID != orgID || a.Decision != board.ApprovalPending {
		return false, nil
	}
	now := time.Now()
	a.Decision = decision
	a.DecidedBy = decidedBy
	if reason != "" {
		a.Reason = reason
	}
	a.DecidedAt = &now
	s.approvals[id] = a
	return true, nil
}

func (s *stubRunRepo) ExpireApprovals(_ context.Context, orgID string) ([]board.Approval, error) {
	out := []board.Approval{}
	for id, a := range s.approvals {
		if a.OrgID == orgID && a.Decision == board.ApprovalPending && a.ExpiresAt.Before(time.Now()) {
			a.Decision = board.ApprovalExpired
			s.approvals[id] = a
			out = append(out, a)
		}
	}
	return out, nil
}

// requestApproval drives the worker's hold request through the real handler.
func requestApproval(t *testing.T, mux *http.ServeMux, scenario rbacTestAPI, actor, preview string) string {
	t.Helper()
	body := `{"preview_json":` + preview + `,"reason":"drop a dev table"}`
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/approvals",
		actor, scenario.orgA, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("request approval = %d, want 201: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Decision string `json:"decision"`
		GateMode string `json:"gate_mode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Decision != "pending" || created.GateMode != "require" {
		t.Fatalf("gate baru = %+v, want pending/require (US-AD33 AC1)", created)
	}
	return created.ID
}

// TestRequestApprovalReturnsThePreviewUnchanged — preview_json disimpan dan
// dikembalikan apa adanya. Dikirim dengan urutan kunci yang tidak alfabetis dan
// angka pecahan yang formatnya khas, karena itulah yang membedakan "echo" dari
// "parse lalu serialisasi ulang".
func TestRequestApprovalReturnsThePreviewUnchanged(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	const preview = `{"zzz":1,"aaa":2.50,"nested":{"b":1,"a":2}}`

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/approvals",
		"marta", scenario.orgA, `{"preview_json":`+preview+`,"reason":"why"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("request = %d, want 201: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID          string          `json:"id"`
		PreviewJSON json.RawMessage `json:"preview_json"`
		TaskID      string          `json:"task_id"`
		RunID       string          `json:"run_id"`
		RequestedBy string          `json:"requested_by"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(created.PreviewJSON) != preview {
		t.Fatalf("preview berubah:\n  dikirim:  %s\n  diterima: %s", preview, created.PreviewJSON)
	}
	if created.TaskID != "task-1" || created.RunID != "run-1" {
		t.Fatalf("gate tidak menunjuk task/run yang benar: %+v", created)
	}
	// Pemanggilnya dicatat: approver berhak tahu siapa yang meminta.
	if created.RequestedBy == "" {
		t.Fatal("requested_by kosong")
	}

	// Dan membacanya kembali memberi payload yang sama.
	got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/approvals/"+created.ID,
		"vera", scenario.orgA, "")
	if got.Code != http.StatusOK {
		t.Fatalf("get approval = %d, want 200", got.Code)
	}
	var fetched struct {
		PreviewJSON json.RawMessage `json:"preview_json"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fetched.PreviewJSON) != preview {
		t.Fatalf("preview berubah saat dibaca kembali: %s", fetched.PreviewJSON)
	}
}

// TestRequestApprovalWithoutAPreviewIs400 — US-AD33 AC2. `{}` ikut ditolak:
// panjangnya lolos, isinya tidak.
func TestRequestApprovalWithoutAPreviewIs400(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	for _, body := range []string{`{"reason":"no preview"}`, `{"preview_json":{}}`, `{"preview_json":null}`} {
		w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/approvals",
			"marta", scenario.orgA, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s = %d, want 400", body, w.Code)
		}
	}
}

// TestRequestApprovalRequiresMember — US-AD33 AC3: viewer tidak boleh membuat
// gate. Marta (member) boleh; Vera (viewer) tidak.
func TestRequestApprovalRequiresMember(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/approvals",
		"vera", scenario.orgA, `{"preview_json":{"action":"drop"}}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer membuat gate = %d, want 403 (US-AD33 AC3)", w.Code)
	}
}

// TestRequestApprovalWithoutARunIs400 — `approvals.run_id` NOT NULL dan terikat
// FK. Task yang belum punya run tidak bisa punya gate, dan id run karangan bukan
// jawaban yang jujur.
func TestRequestApprovalWithoutARunIs400(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	task := repo.tasks["task-1"]
	task.CurrentRunID = ""
	repo.tasks["task-1"] = task

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/tasks/task-1/approvals",
		"marta", scenario.orgA, `{"preview_json":{"action":"drop"}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("gate tanpa run = %d, want 400: %s", w.Code, w.Body.String())
	}
}

// TestApprovalInboxIsViewerReadable — 11.3: membaca itu Viewer.
func TestApprovalInboxIsViewerReadable(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	id := requestApproval(t, mux, scenario, "marta", `{"action":"drop_table"}`)

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/approvals", "vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("inbox = %d, want 200", w.Code)
	}
	var inbox []struct {
		ID        string `json:"id"`
		TaskTitle string `json:"task_title"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(inbox) != 1 || inbox[0].ID != id {
		t.Fatalf("inbox = %+v, want satu gate %s", inbox, id)
	}
}

// TestApproveRequiresAdmin — 11.3 (owner/admin), yang mengalahkan US-AD34 AC3.
//
// Alasan yang menentukan bukan "dokumen mana yang lebih baru", tapi bahwa
// pembuat gate-nya Member: member yang boleh memutuskan bisa membuka dan
// menutup gate-nya sendiri, dan gate seperti itu bukan gate.
func TestApproveRequiresAdmin(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	id := requestApproval(t, mux, scenario, "marta", `{"action":"drop_table"}`)

	// Pembuat gate-nya sendiri tidak boleh menyetujuinya.
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"marta", scenario.orgA, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("member menyetujui gate = %d, want 403", w.Code)
	}
	// Viewer pun tidak.
	w = runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer menyetujui gate = %d, want 403", w.Code)
	}
	// Admin boleh, dan keputusannya tercatat atas namanya.
	w = runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"andre", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("admin menyetujui gate = %d, want 200: %s", w.Code, w.Body.String())
	}
	var decided struct {
		Decision  string `json:"decision"`
		DecidedBy string `json:"decided_by"`
		DecidedAt string `json:"decided_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decided.Decision != "approved" || decided.DecidedBy != "andre@x.test" || decided.DecidedAt == "" {
		t.Fatalf("keputusan = %+v", decided)
	}
}

// TestApproveTwiceIs409 — US-AD34 AC2 lewat handler.
func TestApproveTwiceIs409(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	id := requestApproval(t, mux, scenario, "marta", `{"action":"drop_table"}`)

	if w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"alice", scenario.orgA, ""); w.Code != http.StatusOK {
		t.Fatalf("approve pertama = %d, want 200", w.Code)
	}
	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"alice", scenario.orgA, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("approve kedua = %d, want 409 (US-AD34 AC2)", w.Code)
	}
}

// TestRejectRequiresReasonAndAdmin — US-AD35 AC2 dan AC4.
func TestRejectRequiresReasonAndAdmin(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	id := requestApproval(t, mux, scenario, "marta", `{"action":"drop_table"}`)

	w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/reject",
		"marta", scenario.orgA, `{"reason":"nope"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member menolak = %d, want 403 (US-AD35 AC4)", w.Code)
	}
	w = runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/reject",
		"alice", scenario.orgA, `{"reason":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("reject tanpa alasan = %d, want 400 (US-AD35 AC2)", w.Code)
	}
	w = runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/reject",
		"alice", scenario.orgA, `{"reason":"tabel dev masih dipakai"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("reject = %d, want 200: %s", w.Code, w.Body.String())
	}
	var decided struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decided.Decision != "rejected" || decided.Reason != "tabel dev masih dipakai" {
		t.Fatalf("keputusan = %+v", decided)
	}
}

// TestApprovalIsScopedToItsOrg — 11.4 di jalur HTTP. Andre adalah admin orgA;
// bella owner orgB. Gate orgA tidak boleh terlihat maupun terputuskan dari orgB.
func TestApprovalIsScopedToItsOrg(t *testing.T) {
	mux, _, scenario := newRunAPI(t)
	id := requestApproval(t, mux, scenario, "marta", `{"action":"drop_table"}`)

	if w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/approvals/"+id,
		"bella", scenario.orgB, ""); w.Code != http.StatusNotFound {
		t.Fatalf("gate lintas-tenant dibaca = %d, want 404", w.Code)
	}
	if w := runRequest(t, mux, scenario, http.MethodPost, "/api/v1/approvals/"+id+"/approve",
		"bella", scenario.orgB, ""); w.Code != http.StatusNotFound {
		t.Fatalf("gate lintas-tenant disetujui = %d, want 404", w.Code)
	}
	// Dan gate-nya tetap pending di tenant yang benar.
	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/approvals/"+id,
		"vera", scenario.orgA, "")
	var stored struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stored.Decision != "pending" {
		t.Fatalf("decision = %q; percobaan lintas-tenant mengubahnya", stored.Decision)
	}
}

// TestApprovalRoutesRequireASession.
func TestApprovalRoutesRequireASession(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	for _, path := range []string{"/api/v1/approvals", "/api/v1/approvals/01ABC"} {
		if w := runRequest(t, mux, scenario, http.MethodGet, path, "", scenario.orgA, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s tanpa sesi = %d, want 401", path, w.Code)
		}
	}
}
