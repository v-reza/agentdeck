package main

// GET /boards/{id}/assignable-agents — picker modal "Buat Task Baru" (US-AD11
// AC1, US-AD14 AC1).
//
// Sebelum ini picker board-scoped hanya bisa dibaca lewat
// `GET /agents/{id}/tasks`, dan itu pun cuma kalau agent-nya sudah memegang
// task: `AgentAssignment` menurunkan board dari task pertama agent itu, jadi
// agent tanpa task mengembalikan picker kosong. Modal create-task tidak punya
// agent untuk memulai — pertanyaannya justru "siapa yang bisa saya assign di
// board ini" — sehingga jalur lama tidak bisa menjawabnya sama sekali.
//
// Satu bentuk dengan `picker` di `GET /agents/{id}/tasks`, satu query yang sama
// (`ListAssignableAgentsForBoard`), jadi dua layar tidak bisa drift.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentdeck/internal/board"
)

func (f columnFixture) assignableAgents(t *testing.T, actor, orgID, boardID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/boards/"+boardID+"/assignable-agents", nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

func decodeAssignable(t *testing.T, w *httptest.ResponseRecorder) []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
} {
	t.Helper()
	var rows []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode picker: %v (body %q)", err, w.Body.String())
	}
	return rows
}

// TestAssignableAgentsListsTheBoardsAgents is the happy path: the modal needs a
// picker before any task exists, which is the case the old endpoint could not
// serve.
func TestAssignableAgentsListsTheBoardsAgents(t *testing.T) {
	f := columnEditAPI(t)
	projectID := f.repo.boards[f.boardA].ProjectID
	f.repo.agents["agent-backend"] = board.Agent{ID: "agent-backend", OrgID: f.scenario.orgA, ProjectID: projectID, Name: "agent-backend"}
	f.repo.agents["agent-docs"] = board.Agent{ID: "agent-docs", OrgID: f.scenario.orgA, ProjectID: projectID, Name: "agent-docs"}

	w := f.assignableAgents(t, "vera", f.scenario.orgA, f.boardA)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	got := decodeAssignable(t, w)
	if len(got) != 2 {
		t.Fatalf("picker returned %d agents, want 2: %v", len(got), got)
	}
	// Sorted by name in the statement; assert the set, not the order, so a
	// deliberate ORDER BY change does not fail a test about membership.
	seen := map[string]bool{}
	for _, row := range got {
		seen[row.Name] = true
	}
	for _, name := range []string{"agent-backend", "agent-docs"} {
		if !seen[name] {
			t.Errorf("picker missing %q: %v", name, got)
		}
	}
}

// TestAssignableAgentsHidesArchivedAgents is US-AD73 AC2 on this route: an agent
// that was retired must not be assignable, and the picker must not offer it.
func TestAssignableAgentsHidesArchivedAgents(t *testing.T) {
	f := columnEditAPI(t)
	projectID := f.repo.boards[f.boardA].ProjectID
	f.repo.agents["agent-live"] = board.Agent{ID: "agent-live", OrgID: f.scenario.orgA, ProjectID: projectID, Name: "agent-live"}
	retired := time.Now()
	f.repo.agents["agent-dead"] = board.Agent{ID: "agent-dead", OrgID: f.scenario.orgA, ProjectID: projectID, Name: "agent-dead", ArchivedAt: &retired}

	w := f.assignableAgents(t, "vera", f.scenario.orgA, f.boardA)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	for _, row := range decodeAssignable(t, w) {
		if row.Name == "agent-dead" {
			t.Errorf("picker offered an archived agent: %v", decodeAssignable(t, w))
		}
	}
}

// TestAssignableAgentsStaysInsideTheTenant: the picker is a read of one board's
// project, so a foreign board id must not resolve.
func TestAssignableAgentsStaysInsideTheTenant(t *testing.T) {
	f := columnEditAPI(t)
	// orgB's board, read with orgA's session: the board lookup is org-scoped.
	w := f.assignableAgents(t, "alice", f.scenario.orgA, f.boardB)
	if w.Code == http.StatusOK {
		t.Fatalf("status = 200, want a refusal (orgA must not read orgB's board picker) (body %q)", w.Body.String())
	}
}
