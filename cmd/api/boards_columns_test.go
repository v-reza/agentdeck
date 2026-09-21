package main

// US-AD10 — board column editing. All five acceptance criteria, each with a test
// that fails for the *right* reason:
//
//	AC1 : a new column appears at the end of the layout (append, not insert).
//	AC2 : removing a column that still holds tasks is a 409.
//	AC3 : two columns with the same name is a 409, not a 400.
//	AC4 : editing the layout is owner/admin; member/viewer get 403.
//	AC5 : a solo (B2C) user can edit columns without an admin role.
//
// The mux comes from registerBoardRoutes — the same function main.go calls — so
// these assert the production role gate and the production handler wiring rather
// than a copy of either. A copy would keep passing after main.go was loosened,
// which is the exact failure mode being guarded.
//
// The gate tests never reach the handler: the role gate runs first, so the
// service is built with no repository. If a denied request did reach the
// handler it would panic instead of answering 403 — which is the bug.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
)

// columnFixture is the shared setup for the US-AD10 tests: one mux, one
// repository, one RBAC scenario, and a board in each tenant.
//
// The scenario is created once and shared by every subtest: newRBACTestAPI
// registers real users and mints real sessions, so calling it twice would build
// two unrelated tenants and the board id would not belong to the tokens under
// test.
type columnFixture struct {
	mux      *http.ServeMux
	repo     *fakeBoardRepo
	scenario rbacTestAPI
	boardA   string // orgA — a team workspace (owner/admin/member/viewer)
	boardB   string // orgB — bella's solo workspace, for AC5
}

func columnEditAPI(t *testing.T) columnFixture {
	t.Helper()
	scenario := newRBACTestAPI(t)
	repo := newFakeBoardRepo()

	seed := func(orgID, projectID, boardID string) string {
		t.Helper()
		if _, err := repo.CreateProject(context.Background(), board.Project{
			ID: projectID, OrgID: orgID, Slug: projectID, Name: "Demo",
		}); err != nil {
			t.Fatalf("seed project %s: %v", projectID, err)
		}
		b, err := repo.CreateBoard(context.Background(), board.Board{
			ID: boardID, OrgID: orgID, ProjectID: projectID,
			Slug: boardID, Name: "Sprint",
			Columns:           board.DefaultColumns,
			BudgetDailyMicros: board.DefaultBudgetDailyMicros,
		})
		if err != nil {
			t.Fatalf("seed board %s: %v", boardID, err)
		}
		return b.ID
	}

	f := columnFixture{
		repo:     repo,
		scenario: scenario,
		boardA:   seed(scenario.orgA, "proj-a", "board-a"),
		boardB:   seed(scenario.orgB, "proj-b", "board-b"),
	}
	f.mux = http.NewServeMux()
	registerBoardRoutes(f.mux, scenario.api, board.NewService(repo), nil)
	return f
}

func patchColumns(t *testing.T, mux *http.ServeMux, scenario rbacTestAPI, actor, orgID, boardID string, cols []board.Column) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(columnsRequest{Columns: cols})
	if err != nil {
		t.Fatalf("marshal columns: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/boards/"+boardID+"/columns", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

// decodeColumns reads the layout out of a layout response.
func decodeColumns(t *testing.T, recorder *httptest.ResponseRecorder) []board.Column {
	t.Helper()
	var got []board.Column
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return got
}

// TestUpdateBoardColumnsRequiresAdmin is US-AD10 AC4.
//
// This is the criterion that was wrong before the test existed: the layout used
// to be writable through PATCH /boards/{id}, which is Member-gated, so a member
// could edit the board structure despite the AC saying owner/admin only.
func TestUpdateBoardColumnsRequiresAdmin(t *testing.T) {
	f := columnEditAPI(t)

	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false}, // owner
		{actor: "andre", wantDenied: false}, // admin
		{actor: "marta", wantDenied: true},  // member
		{actor: "vera", wantDenied: true},   // viewer
	}

	for _, tc := range cases {
		t.Run(tc.actor, func(t *testing.T) {
			recorder := patchColumns(t, f.mux, f.scenario, tc.actor, f.scenario.orgA, f.boardA, board.DefaultColumns)
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s editing columns: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s editing columns: status = 403, want the gate to allow admin+", tc.actor)
			}
		})
	}
}

// TestSoloWorkspaceCanEditColumns is US-AD10 AC5 (B2C).
//
// The AC exists because the product's primary user is a solo builder who is
// owner, admin and member at once — the admin gate must not become a wall that
// requires inviting a second person. Bella owns orgB and is its only member, so
// this fails if the gate ever required a role the solo user cannot hold.
func TestSoloWorkspaceCanEditColumns(t *testing.T) {
	f := columnEditAPI(t)

	recorder := patchColumns(t, f.mux, f.scenario, "bella", f.scenario.orgB, f.boardB, board.DefaultColumns)
	if recorder.Code != http.StatusOK {
		t.Fatalf("solo owner editing columns: status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// TestBoardColumnsAreNotWritableThroughTheMemberRoute pins the escalation the
// AC4 test only implies: PATCH /boards/{id} is Member-gated, so if it also
// accepted `columns` the Admin gate on the columns route would be decorative. A
// member sending a layout there must not change the layout.
func TestBoardColumnsAreNotWritableThroughTheMemberRoute(t *testing.T) {
	f := columnEditAPI(t)

	// marta is a member: allowed through the PATCH /boards/{id} gate.
	body, err := json.Marshal(map[string]any{
		"columns": []board.Column{{Key: "backlog", Name: "Renamed by a member"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/boards/"+f.boardA, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.scenario.tokens["marta@x.test"])
	req.Header.Set("X-Org-ID", f.scenario.orgA)
	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("member on PATCH /boards/{id}: status = 403, want the route to stay Member-gated")
	}

	stored, err := f.repo.GetBoard(context.Background(), f.boardA, f.scenario.orgA)
	if err != nil {
		t.Fatalf("read back board: %v", err)
	}
	for _, c := range stored.Columns {
		if c.Name == "Renamed by a member" {
			t.Fatal("a member changed the board layout through the Member-gated route: the Admin gate on /columns is bypassable")
		}
	}
}

// TestAddingAColumnAppendsItToTheLayout is US-AD10 AC1.
//
// The AC pins the position, not just the status code: "kolom baru muncul di
// urutan terakhir". A layout that accepted the column but inserted it anywhere
// else — or that let the caller's order be ignored — would satisfy a status-code
// check while breaking what the AC actually promises.
func TestAddingAColumnAppendsItToTheLayout(t *testing.T) {
	f := columnEditAPI(t)

	appended := append(append([]board.Column{}, board.DefaultColumns...), board.Column{Key: "blocked", Name: "Blocked"})
	recorder := patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, appended)
	if recorder.Code != http.StatusOK {
		t.Fatalf("adding a column: status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}

	got := decodeColumns(t, recorder)
	if len(got) != len(appended) {
		t.Fatalf("layout has %d columns, want %d", len(got), len(appended))
	}
	last := got[len(got)-1]
	if last.Key != "blocked" || last.Name != "Blocked" {
		t.Fatalf("new column is at position %d (%+v), want it last", len(got)-1, last)
	}
	// The pre-existing order must survive: an append that reshuffles the first
	// five columns is still a broken layout.
	for i, want := range board.DefaultColumns {
		if got[i].Key != want.Key {
			t.Fatalf("column %d = %q, want %q (existing order changed)", i, got[i].Key, want.Key)
		}
	}
}

// TestColumnNameMustBeUniqueOnTheBoard is US-AD10 AC3.
//
// The AC asks for a 409 specifically, because the operator's fix is "choose a
// different name" rather than "fix your payload". Before this test the check
// only compared column *keys* — internal, never edited by hand — so renaming
// two columns to "Review" was accepted and the board rendered two columns the
// operator could not tell apart.
func TestColumnNameMustBeUniqueOnTheBoard(t *testing.T) {
	f := columnEditAPI(t)

	cases := []struct {
		name  string
		cols  []board.Column
		want  int
		label string
	}{
		{
			name: "exact duplicate",
			cols: []board.Column{
				{Key: "backlog", Name: "Review"},
				{Key: "ready", Name: "Review"},
			},
			want:  http.StatusConflict,
			label: "two columns with the same name must be a 409",
		},
		{
			name: "duplicate differing only in case and padding",
			cols: []board.Column{
				{Key: "backlog", Name: "Review"},
				{Key: "ready", Name: "  review  "},
			},
			want:  http.StatusConflict,
			label: "names a human reads as identical must collide",
		},
		{
			name: "distinct names",
			cols: []board.Column{
				{Key: "backlog", Name: "Backlog"},
				{Key: "ready", Name: "Ready"},
			},
			want:  http.StatusOK,
			label: "distinct names must be accepted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, tc.cols)
			if recorder.Code != tc.want {
				t.Fatalf("%s: status = %d, want %d", tc.label, recorder.Code, tc.want)
			}
		})
	}
}

// TestRemovingAColumnWithTasksIsAConflict is US-AD10 AC2.
//
// The status -> column mapping is the subtle part, and the reason this test uses
// `awaiting_approval` rather than a plain `running` task: awaiting_approval is a
// distinct status that *renders in* the running column, so a check comparing
// status strings to column keys would see the running column as empty and delete
// a column still holding work. The same trap exists for failed/cancelled in
// `done`.
func TestRemovingAColumnWithTasksIsAConflict(t *testing.T) {
	f := columnEditAPI(t)

	task, err := f.repo.CreateTask(context.Background(), board.Task{
		ID: "task-waiting", OrgID: f.scenario.orgA, BoardID: f.boardA,
		Title: "waiting on approval", Status: board.StatusAwaitingApproval,
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Drop the `running` column, which is where an awaiting_approval task renders.
	withoutRunning := []board.Column{
		{Key: "backlog", Name: "Backlog"},
		{Key: "ready", Name: "Ready"},
		{Key: "review", Name: "Review"},
		{Key: "done", Name: "Done"},
	}
	recorder := patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, withoutRunning)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("removing a column holding an awaiting_approval task: status = %d, want 409", recorder.Code)
	}

	// The layout must be unchanged after the rejected write, not half-applied.
	stored, err := f.repo.GetBoard(context.Background(), f.boardA, f.scenario.orgA)
	if err != nil {
		t.Fatalf("read back board: %v", err)
	}
	if len(stored.Columns) != len(board.DefaultColumns) {
		t.Fatalf("rejected write changed the layout: %d columns, want %d", len(stored.Columns), len(board.DefaultColumns))
	}

	// Moving the task out of the column clears the guard: the 409 is about the
	// tasks, not about the column being special.
	if _, err := f.repo.UpdateTaskStatus(context.Background(), task.ID, f.scenario.orgA, board.StatusAwaitingApproval, board.StatusDone); err != nil {
		t.Fatalf("move task: %v", err)
	}
	recorder = patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, withoutRunning)
	if recorder.Code != http.StatusOK {
		t.Fatalf("removing the now-empty column: status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// TestRemovingAnEmptyColumnSucceeds covers the other half of AC2: the guard must
// not block the normal case, or the feature is unusable and the 409 above would
// be "passing" for the wrong reason. It also pins column order, because the
// order is the operator's intent and a reorder that silently drops a column
// would otherwise pass.
func TestRemovingAnEmptyColumnSucceeds(t *testing.T) {
	f := columnEditAPI(t)

	cols := []board.Column{
		{Key: "backlog", Name: "Backlog"},
		{Key: "ready", Name: "Ready"},
	}
	recorder := patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, cols)
	if recorder.Code != http.StatusOK {
		t.Fatalf("removing empty columns: status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}

	got := decodeColumns(t, recorder)
	if len(got) != len(cols) {
		t.Fatalf("response has %d columns, want %d", len(got), len(cols))
	}
	for i := range cols {
		if got[i].Key != cols[i].Key || got[i].Name != cols[i].Name {
			t.Fatalf("column %d = %+v, want %+v", i, got[i], cols[i])
		}
	}
}

// TestReorderingColumnsPersists covers the "mengurutkan ulang" half of the
// story title: the layout is an ordered list, so a reorder must survive the
// round trip rather than being normalised back to the default order.
func TestReorderingColumnsPersists(t *testing.T) {
	f := columnEditAPI(t)

	reversed := make([]board.Column, 0, len(board.DefaultColumns))
	for i := len(board.DefaultColumns) - 1; i >= 0; i-- {
		reversed = append(reversed, board.DefaultColumns[i])
	}

	recorder := patchColumns(t, f.mux, f.scenario, "andre", f.scenario.orgA, f.boardA, reversed)
	if recorder.Code != http.StatusOK {
		t.Fatalf("reordering: status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}

	got := decodeColumns(t, recorder)
	for i := range reversed {
		if got[i].Key != reversed[i].Key {
			t.Fatalf("column %d = %q, want %q (order did not persist)", i, got[i].Key, reversed[i].Key)
		}
	}
}

// TestColumnEditsAreTenantScoped proves the layout routes resolve the board
// through the caller's own org, so a board id from another tenant is a 404
// rather than a cross-tenant write.
//
// The actor has to be a real member of orgB — bella, its owner — or the RBAC
// gate answers 403 first and the request never reaches the lookup this test is
// about. Asserting 404 through an outsider would pass for the wrong reason and
// would keep passing even if the handler stopped scoping by org entirely.
func TestColumnEditsAreTenantScoped(t *testing.T) {
	f := columnEditAPI(t)

	recorder := patchColumns(t, f.mux, f.scenario, "bella", f.scenario.orgB, f.boardA, board.DefaultColumns)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("editing orgA's board with orgB selected: status = %d, want 404", recorder.Code)
	}

	// An outsider is rejected by the gate, not by the lookup: 403, not 404. The
	// two are different claims and the gate must not be the thing that happens
	// to make the 404 above look right.
	outsider := patchColumns(t, f.mux, f.scenario, "alice", f.scenario.orgB, f.boardA, board.DefaultColumns)
	if outsider.Code != http.StatusForbidden {
		t.Fatalf("non-member editing through orgB: status = %d, want 403", outsider.Code)
	}
}

// fakeBoardRepo is the in-memory Repository the column tests drive.
//
// It embeds the Repository interface rather than implementing all 30 methods:
// only the layout path is under test, so an unimplemented method panics with a
// nil-pointer dereference if a test ever reaches it. That is deliberate — a
// silent zero value would let a test "pass" against a repository that never
// persisted anything. Every method that *is* implemented scopes by orgID and
// returns ErrNotFound on a mismatch, which is what the pgx repository does with
// `WHERE org_id = $n`.
type fakeBoardRepo struct {
	board.Repository

	projects map[string]board.Project
	boards   map[string]board.Board
	tasks    map[string]board.Task
	agents   map[string]board.Agent
	// providerKeys stands in for agents.provider_api_key_enc. It holds the
	// sealed bytes exactly as the handler handed them over, so a test can assert
	// what was persisted without a database. Postgres itself owns the generated
	// has_provider_key column; the fake mirrors it in Set/Clear below.
	providerKeys map[string][]byte
}

func newFakeBoardRepo() *fakeBoardRepo {
	return &fakeBoardRepo{
		projects:     map[string]board.Project{},
		boards:       map[string]board.Board{},
		tasks:        map[string]board.Task{},
		agents:       map[string]board.Agent{},
		providerKeys: map[string][]byte{},
	}
}

// ---- agents (US-AD20) -------------------------------------------------------
//
// The fake mirrors the two behaviours Postgres owns for this table: the name is
// unique per project (agents_project_name_key) and every read is org-scoped.
// Reproducing them here is what lets the service's error mapping be the thing
// under test rather than the database's.

func (r *fakeBoardRepo) CreateAgent(_ context.Context, a board.Agent) (board.Agent, error) {
	for _, existing := range r.agents {
		if existing.ProjectID == a.ProjectID && existing.Name == a.Name {
			return board.Agent{}, board.ErrAgentNameTaken
		}
	}
	r.agents[a.ID] = a
	return a, nil
}

func (r *fakeBoardRepo) GetAgent(_ context.Context, id, orgID string) (board.Agent, error) {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return board.Agent{}, board.ErrNotFound
	}
	return a, nil
}

// ListAgents mirrors the real statement's column set, which is the point: the
// sqlc ListAgents now selects base_url and archived_at alongside the pre-0008
// columns, so the fake passes them through. It used to blank both, because the
// statement did not select them and a fake returning the full stored struct
// would have let a test believe the list reports archived_at when production's
// row type could not carry it. That gap is closed in queries.sql.
//
// Archived rows are still returned: archiving keeps the row, and this screen is
// where a user finds it again to unarchive.
func (r *fakeBoardRepo) ListAgents(_ context.Context, orgID, projectID string) ([]board.Agent, error) {
	out := []board.Agent{}
	for _, a := range r.agents {
		if a.OrgID == orgID && a.ProjectID == projectID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeBoardRepo) DeleteAgent(_ context.Context, id, orgID string) error {
	a, ok := r.agents[id]
	if !ok || a.OrgID != orgID {
		return board.ErrNotFound
	}
	delete(r.agents, id)
	return nil
}

func (r *fakeBoardRepo) CountAgentRunningTasks(_ context.Context, id, orgID string) (int, error) {
	n := 0
	for _, task := range r.tasks {
		if task.OrgID == orgID && task.AssigneeAgentID == id && task.Status == board.StatusRunning {
			n++
		}
	}
	return n, nil
}

func (r *fakeBoardRepo) CreateProject(_ context.Context, p board.Project) (board.Project, error) {
	r.projects[p.ID] = p
	return p, nil
}

func (r *fakeBoardRepo) GetProject(_ context.Context, id, orgID string) (board.Project, error) {
	p, ok := r.projects[id]
	if !ok || p.OrgID != orgID {
		return board.Project{}, board.ErrNotFound
	}
	return p, nil
}

func (r *fakeBoardRepo) CreateBoard(_ context.Context, b board.Board) (board.Board, error) {
	r.boards[b.ID] = b
	return b, nil
}

func (r *fakeBoardRepo) GetBoard(_ context.Context, id, orgID string) (board.Board, error) {
	b, ok := r.boards[id]
	if !ok || b.OrgID != orgID {
		return board.Board{}, board.ErrNotFound
	}
	return b, nil
}

func (r *fakeBoardRepo) UpdateBoardColumns(_ context.Context, id, orgID string, cols []board.Column) error {
	b, err := r.GetBoard(context.Background(), id, orgID)
	if err != nil {
		return err
	}
	b.Columns = cols
	r.boards[id] = b
	return nil
}

func (r *fakeBoardRepo) CreateTask(_ context.Context, task board.Task) (board.Task, error) {
	if _, err := r.GetBoard(context.Background(), task.BoardID, task.OrgID); err != nil {
		return board.Task{}, err
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeBoardRepo) GetTask(_ context.Context, id, orgID string) (board.Task, error) {
	task, ok := r.tasks[id]
	if !ok || task.OrgID != orgID {
		return board.Task{}, board.ErrNotFound
	}
	return task, nil
}

func (r *fakeBoardRepo) ListBoardTasks(_ context.Context, orgID, boardID string) ([]board.Task, error) {
	out := []board.Task{}
	for _, task := range r.tasks {
		if task.OrgID == orgID && task.BoardID == boardID {
			out = append(out, task)
		}
	}
	return out, nil
}

func (r *fakeBoardRepo) UpdateTaskStatus(_ context.Context, id, orgID string, from, to board.TaskStatus) (board.Task, error) {
	task, err := r.GetTask(context.Background(), id, orgID)
	if err != nil {
		return board.Task{}, err
	}
	// Mirror the real repository's optimistic guard: a transition whose expected
	// `from` does not match the stored status is a conflict, not a blind write.
	if task.Status != from {
		return board.Task{}, board.ErrConflict
	}
	task.Status = to
	r.tasks[id] = task
	return task, nil
}
