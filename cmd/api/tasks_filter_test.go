package main

// §6.2.16 — filter `GET /boards/{board_id}/tasks` dan `assignee_agent_id` di
// `POST /boards/{board_id}/tasks`.
//
// Kenapa file ini ada: baris kontrak untuk endpoint list menulis "filter
// `status, assignee, search`" dengan status ✅, padahal handler-nya membaca nol
// query param. Filter itu hanya hidup di `TableView.tsx`, client-side. Akibatnya
// `curl` dan aplikasi tidak sepakat soal board yang sama, dan status ✅ di
// dokumen tidak dipegang kode mana pun — pola §18 lagi.
//
// Jalur create juga kehilangan satu field: US-AD11 AC1 menyebut
// `assignee_agent_id` opsional, `CreateTaskParams` sudah punya slotnya sejak
// awal, tapi `Service.CreateTask` tidak pernah menerimanya, jadi selalu kosong.
//
// Mux-nya dari `registerBoardRoutes`, jadi gate peran dan wiring yang diuji di
// sini adalah yang dipakai produksi.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// seedTaskFull is `seedTask` with a chosen title, status and assignee, so a
// filter test can build rows that differ on exactly one axis.
func (f columnFixture) seedTaskFull(t *testing.T, ctx context.Context, orgID, id, title string, status board.TaskStatus, assignee string) {
	t.Helper()
	boardID := f.boardA
	if orgID != f.scenario.orgA {
		boardID = f.boardB
	}
	if _, err := f.repo.CreateTask(ctx, board.Task{
		ID: id, OrgID: orgID, BoardID: boardID,
		Title: title, Status: status, AssigneeAgentID: assignee,
	}); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

// listTasks drives GET /boards/{id}/tasks through the real route with a raw
// query string, so the test asserts what the URL does rather than what a helper
// chose to send.
func (f columnFixture) listTasks(t *testing.T, actor, orgID, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/boards/" + f.boardA + "/tasks"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	return w
}

// taskTitles decodes a list response into the titles it returned.
func taskTitles(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var rows []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode list: %v (body %q)", err, w.Body.String())
	}
	titles := make([]string, 0, len(rows))
	for _, r := range rows {
		titles = append(titles, r.Title)
	}
	return titles
}

// TestListTasksHonoursTheAdvertisedFilters is the contract's own promise.
func TestListTasksHonoursTheAdvertisedFilters(t *testing.T) {
	f := columnEditAPI(t)
	ctx := context.Background()

	// Three tasks that differ on every axis the filter can use, plus an agent to
	// point one of them at.
	agent := board.Agent{ID: "agent-1", OrgID: f.scenario.orgA, Name: "agent-backend"}
	f.repo.agents[agent.ID] = agent

	// Seeded with the exact title and assignee each case needs. `seedTask` names
	// rows "Task <id>" and the fake deliberately panics on the repository methods
	// it does not implement (`AssignTask`, `UpdateTaskFields`), so going through
	// them would test the double rather than the query.
	f.seedTaskFull(t, ctx, f.scenario.orgA, "task-alpha", "Migrate sessions to Redis-less", board.StatusBacklog, "")
	f.seedTaskFull(t, ctx, f.scenario.orgA, "task-beta", "Fix payment webhook retry", board.StatusReady, "")
	f.seedTaskFull(t, ctx, f.scenario.orgA, "task-gamma", "Fix payment webhook timeout", board.StatusReady, agent.ID)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"no filter returns the whole board", "", []string{"Migrate sessions to Redis-less", "Fix payment webhook retry", "Fix payment webhook timeout"}},
		{"status narrows to one column", "status=backlog", []string{"Migrate sessions to Redis-less"}},
		{"assignee narrows to one agent", "assignee=agent-1", []string{"Fix payment webhook timeout"}},
		{"search matches a substring, not the whole title", "search=webhook", []string{"Fix payment webhook retry", "Fix payment webhook timeout"}},
		{"search is case insensitive", "search=REDIS", []string{"Migrate sessions to Redis-less"}},
		{"filters compose", "status=ready&search=retry", []string{"Fix payment webhook retry"}},
		{"an unmatched search returns nothing, not everything", "search=nothing-matches-this", nil},
		{"an assignee nobody holds returns nothing", "assignee=agent-nobody", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := f.listTasks(t, "vera", f.scenario.orgA, tc.query)
			if w.Code != http.StatusOK {
				t.Fatalf("list %q: status = %d, want 200 (body %q)", tc.query, w.Code, w.Body.String())
			}
			got := taskTitles(t, w)
			if len(got) != len(tc.want) {
				t.Fatalf("list %q returned %v, want %v", tc.query, got, tc.want)
			}
			// Order comes from the statement (priority DESC, created_at DESC), and
			// these rows share both, so compare as a set rather than pinning an
			// order the query does not promise.
			seen := map[string]bool{}
			for _, title := range got {
				seen[title] = true
			}
			for _, title := range tc.want {
				if !seen[title] {
					t.Errorf("list %q returned %v, missing %q", tc.query, got, title)
				}
			}
		})
	}
}

// TestListTasksRejectsAnUnknownStatus keeps the filter from silently returning
// an empty board: `status=donee` is a caller mistake, not a column with no work.
func TestListTasksRejectsAnUnknownStatus(t *testing.T) {
	f := columnEditAPI(t)
	w := f.listTasks(t, "vera", f.scenario.orgA, "status=donee")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
	}
}

// TestListTasksFiltersStayInsideTheTenant is the isolation half: a filter is
// still a read, so it must not become a way to see another workspace's board.
func TestListTasksFiltersStayInsideTheTenant(t *testing.T) {
	f := columnEditAPI(t)
	ctx := context.Background()
	f.seedTaskFull(t, ctx, f.scenario.orgA, "task-orgA", "Task task-orgA", board.StatusBacklog, "")
	f.seedTaskFull(t, ctx, f.scenario.orgB, "task-orgB", "Task task-orgB", board.StatusBacklog, "")

	// bella owns orgB; reading orgA's board with a filter must stay empty rather
	// than leak orgA's row through a filter that "matched".
	w := f.listTasks(t, "bella", f.scenario.orgB, "search=Task")
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	for _, title := range taskTitles(t, w) {
		if title == "Task task-orgA" {
			t.Fatalf("orgB's filtered read leaked orgA's task: %v", taskTitles(t, w))
		}
	}
}

// TestCreateTaskAcceptsAnAssignee is US-AD11 AC1 / AC5.
//
// AC5 is the one worth pinning: a task with no agent must still be valid and land
// in backlog, so the assignee is optional rather than required.
func TestCreateTaskAcceptsAnAssignee(t *testing.T) {
	f := columnEditAPI(t)
	agent := board.Agent{ID: "agent-1", OrgID: f.scenario.orgA, Name: "agent-backend"}
	f.repo.agents[agent.ID] = agent

	post := func(t *testing.T, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/boards/"+f.boardA+"/tasks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.scenario.tokens["marta@x.test"])
		req.Header.Set("X-Org-ID", f.scenario.orgA)
		w := httptest.NewRecorder()
		f.mux.ServeHTTP(w, req)
		return w
	}

	t.Run("an assigned task carries the agent id", func(t *testing.T) {
		w := post(t, `{"title":"Assigned work","body":"do the thing","assignee_agent_id":"agent-1"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %q)", w.Code, w.Body.String())
		}
		var row struct {
			Status          string `json:"status"`
			AssigneeAgentID string `json:"assignee_agent_id"`
			Body            string `json:"body"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if row.AssigneeAgentID != "agent-1" {
			t.Errorf("assignee_agent_id = %q, want %q", row.AssigneeAgentID, "agent-1")
		}
		if row.Status != string(board.StatusBacklog) {
			t.Errorf("status = %q, want %q (AC1)", row.Status, board.StatusBacklog)
		}
		if row.Body != "do the thing" {
			t.Errorf("body = %q, want it stored (AC1)", row.Body)
		}
	})

	t.Run("no agent is valid and still lands in backlog", func(t *testing.T) {
		w := post(t, `{"title":"Unowned work"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (AC5: an unassigned task is valid) (body %q)", w.Code, w.Body.String())
		}
		var row struct {
			Status          string `json:"status"`
			AssigneeAgentID string `json:"assignee_agent_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if row.Status != string(board.StatusBacklog) {
			t.Errorf("status = %q, want %q", row.Status, board.StatusBacklog)
		}
		if row.AssigneeAgentID != "" {
			t.Errorf("assignee_agent_id = %q, want empty", row.AssigneeAgentID)
		}
	})

	// The foreign key only proves the id exists somewhere, so an id from another
	// workspace must be refused by the service rather than written.
	t.Run("an agent from another workspace is refused", func(t *testing.T) {
		foreign := board.Agent{ID: "agent-orgB", OrgID: f.scenario.orgB, Name: "foreign"}
		f.repo.agents[foreign.ID] = foreign
		w := post(t, `{"title":"Cross-tenant","assignee_agent_id":"agent-orgB"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (an id from another workspace must not be assigned) (body %q)", w.Code, w.Body.String())
		}
	})
}
