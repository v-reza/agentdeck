package main

// Budget board dan laporan biaya org — sisa §6.2.15.
//
// Yang diuji di sini adalah gerbang perannya dan bentuk responsnya. Bentuknya
// penting karena `frontend/src/lib/domain.ts` sudah mendeklarasikan tipe
// `BoardBudget` dan `CostSummary` sejak lama: layar finops-nya sudah ada dan
// selama ini menampilkan empty state karena endpoint-nya 404. Nama field yang
// meleset sedikit pun akan membuat halaman itu kosong lagi, tanpa error.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"agentdeck/internal/board"
)

// BoardBudgetToday meniru query agregat: cap dari baris board, terpakai dari
// daily_board_costs. Board yang belum punya baris hari ini tetap menjawab cap-nya
// dengan terpakai nol — guardrail tidak boleh bergantung pada ada-tidaknya baris.
func (s *stubRunRepo) BoardBudgetToday(_ context.Context, orgID, boardID string) (board.BoardBudget, error) {
	// Mirror the statement's `WHERE b.id = $1 AND b.org_id = $2`: a board that is
	// not there is zero rows, which is ErrNotFound. A double that answers for any
	// id would hide the difference between "no budget" and "no board".
	if orgID == "" || !s.boards[boardID] {
		return board.BoardBudget{}, board.ErrNotFound
	}
	if b, ok := s.budgets[boardID]; ok {
		return b, nil
	}
	capMicros := int64(board.DefaultBudgetDailyMicros)
	if c, ok := s.caps[boardID]; ok {
		capMicros = c
	}
	return board.BoardBudget{
		BoardID: boardID, CapMicros: capMicros, Day: time.Now().UTC().Format("2006-01-02"),
		Status: "ok",
	}, nil
}

// UpdateBoardBudget menulis cap-nya, seperti statement aslinya.
func (s *stubRunRepo) UpdateBoardBudget(_ context.Context, id, orgID string, budgetMicros int64) error {
	if orgID == "" || !s.boards[id] {
		return board.ErrNotFound
	}
	if s.caps == nil {
		s.caps = map[string]int64{}
	}
	s.caps[id] = budgetMicros
	if b, ok := s.budgets[id]; ok {
		b.CapMicros = budgetMicros
		// Status diturunkan ulang dari cap baru: menaikkan cap di atas pemakaian
		// harus mengembalikan statusnya ke ok, dan double yang membekukan status
		// lama akan menyembunyikan bug itu.
		b.Status = statusFor(b.SpentTodayMicros, budgetMicros)
		s.budgets[id] = b
	}
	return nil
}

func statusFor(spent, cap int64) string {
	switch {
	case spent >= cap:
		return "exceeded"
	case cap > 0 && spent >= cap*8/10:
		return "warning"
	default:
		return "ok"
	}
}

func (s *stubRunRepo) OrgCostSummary(_ context.Context, orgID string) (board.CostSummary, error) {
	if orgID == "" {
		return board.CostSummary{}, board.ErrNotFound
	}
	return s.summary, nil
}

// TestBoardBudgetReportsCapAndSpend — §6.2.15, N16/N18.
//
// The field names are the contract the finops screen already codes against.
func TestBoardBudgetReportsCapAndSpend(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.budgets = map[string]board.BoardBudget{
		"board-a": {
			BoardID: "board-a", CapMicros: 20_000_000, SpentTodayMicros: 16_500_000,
			RunCount: 2, TokensIn: 100, TokensOut: 20, Status: "warning",
			Day: "2026-09-27",
		},
	}

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/boards/board-a/budget",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("budget = %d, want 200: %s", w.Code, w.Body.String())
	}

	var body struct {
		BoardID           string `json:"board_id"`
		Day               string `json:"day"`
		BudgetDailyMicros int64  `json:"budget_daily_micros"`
		SpentMicros       int64  `json:"spent_micros"`
		RunCount          int    `json:"run_count"`
		TokensIn          int64  `json:"tokens_in"`
		TokensOut         int64  `json:"tokens_out"`
		ThresholdCrossed  bool   `json:"threshold_crossed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BoardID != "board-a" || body.Day != "2026-09-27" {
		t.Fatalf("identity fields wrong: %+v", body)
	}
	if body.BudgetDailyMicros != 20_000_000 || body.SpentMicros != 16_500_000 {
		t.Fatalf("cap/spend wrong: %+v", body)
	}
	if body.RunCount != 2 || body.TokensIn != 100 || body.TokensOut != 20 {
		t.Fatalf("aggregate fields wrong: %+v", body)
	}
	// N18: 16.5M of 20M is past 80%, so the alert flag is set even though the cap
	// itself has not been reached.
	if !body.ThresholdCrossed {
		t.Fatal("N18: 82.5% of the cap did not raise threshold_crossed")
	}
}

// TestBoardBudgetOnAFreshBoardIsZeroNotAnError — N16's guardrail must not depend
// on a daily_board_costs row existing.
func TestBoardBudgetOnAFreshBoardIsZeroNotAnError(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/boards/board-a/budget",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("budget on a fresh board = %d, want 200", w.Code)
	}
	var body struct {
		BudgetDailyMicros int64 `json:"budget_daily_micros"`
		SpentMicros       int64 `json:"spent_micros"`
		ThresholdCrossed  bool  `json:"threshold_crossed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SpentMicros != 0 || body.BudgetDailyMicros == 0 {
		t.Fatalf("fresh board: cap=%d spent=%d", body.BudgetDailyMicros, body.SpentMicros)
	}
	if body.ThresholdCrossed {
		t.Fatal("a board with no spend reported the 80% alert")
	}
}

// TestBoardBudgetUnknownBoardIs404.
func TestBoardBudgetUnknownBoardIs404(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/boards/nope/budget",
		"vera", scenario.orgA, "").Code; got != http.StatusNotFound {
		t.Fatalf("unknown board budget = %d, want 404", got)
	}
}

// TestUpdateBoardBudgetRaisesTheCap — §6.2.15, Admin.
func TestUpdateBoardBudgetRaisesTheCap(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.budgets = map[string]board.BoardBudget{
		"board-a": {BoardID: "board-a", CapMicros: 20_000_000, SpentTodayMicros: 19_000_000,
			Status: "warning", Day: "2026-09-27"},
	}

	w := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/boards/board-a/budget",
		"alice", scenario.orgA, `{"budget_daily_micros":50000000}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch budget = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := repo.caps["board-a"]; got != 50_000_000 {
		t.Fatalf("cap stored = %d, want 50_000_000", got)
	}
	// The response is the budget after the write, so the caller does not have to
	// re-query to see what the new cap applies to — and raising the cap above the
	// spend clears the alert.
	var body struct {
		BudgetDailyMicros int64 `json:"budget_daily_micros"`
		SpentMicros       int64 `json:"spent_micros"`
		ThresholdCrossed  bool  `json:"threshold_crossed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BudgetDailyMicros != 50_000_000 || body.SpentMicros != 19_000_000 {
		t.Fatalf("response does not reflect the new cap: %+v", body)
	}
	if body.ThresholdCrossed {
		t.Fatal("raising the cap above the spend left the N18 alert set")
	}
}

// TestBoardBudgetAtTheCapStillRaisesTheAlert — N18.
//
// The flag is "at or past 80%", and `exceeded` is past it. A flag that only
// checked `warning` would go quiet at exactly the moment the board stops
// spending, which is the worst possible time for the alert to disappear.
func TestBoardBudgetAtTheCapStillRaisesTheAlert(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.budgets = map[string]board.BoardBudget{
		"board-a": {
			BoardID: "board-a", CapMicros: 20_000_000, SpentTodayMicros: 20_000_000,
			Status: "exceeded", Day: "2026-09-27",
		},
	}

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/boards/board-a/budget",
		"vera", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("budget = %d, want 200", w.Code)
	}
	var body struct {
		SpentMicros      int64 `json:"spent_micros"`
		ThresholdCrossed bool  `json:"threshold_crossed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SpentMicros != 20_000_000 {
		t.Fatalf("spend = %d", body.SpentMicros)
	}
	if !body.ThresholdCrossed {
		t.Fatal("at the cap the N18 alert is off; it must stay on past the threshold")
	}
}

// TestUpdateBoardBudgetRequiresAdmin — §6.2.15's Role Min.
func TestUpdateBoardBudgetRequiresAdmin(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/boards/board-a/budget",
		"marta", scenario.orgA, `{"budget_daily_micros":1}`).Code; got != http.StatusForbidden {
		t.Fatalf("member patching the budget = %d, want 403", got)
	}
	if _, wrote := repo.caps["board-a"]; wrote {
		t.Fatal("a forbidden patch still wrote the cap")
	}
}

// TestUpdateBoardBudgetRejectsAMissingField — a zero cap is legitimate, so an
// absent field must not be read as one.
func TestUpdateBoardBudgetRejectsAMissingField(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/boards/board-a/budget",
		"alice", scenario.orgA, `{}`).Code; got != http.StatusBadRequest {
		t.Fatalf("patch without the field = %d, want 400", got)
	}
	// Zero is a real value, and it must be accepted.
	if got := runRequest(t, mux, scenario, http.MethodPatch, "/api/v1/boards/board-a/budget",
		"alice", scenario.orgA, `{"budget_daily_micros":0}`).Code; got != http.StatusOK {
		t.Fatalf("patch with an explicit zero = %d, want 200", got)
	}
}

// TestCostSummaryShape — US-AD32 reporting, against the shape the FE declares.
func TestCostSummaryShape(t *testing.T) {
	mux, repo, scenario := newRunAPI(t)
	repo.summary = board.CostSummary{
		TotalMicros: 12_345_678,
		ByModel: []board.CostByModel{
			{Model: "gpt-4o", Provider: "openai_compatible", CostMicros: 12_000_000, TokensIn: 900, TokensOut: 100, Runs: 3},
		},
		ByBoard: []board.CostByBoard{
			{BoardID: "board-a", BoardName: "Sprint", CostMicros: 12_345_678, TokensIn: 900, TokensOut: 100, Runs: 3},
		},
	}

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/orgs/"+scenario.orgA+"/cost-summary",
		"alice", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("cost summary = %d, want 200: %s", w.Code, w.Body.String())
	}

	var body struct {
		TotalMicros int64 `json:"total_micros"`
		WindowDays  int   `json:"window_days"`
		ByModel     []struct {
			Model      string `json:"model"`
			Provider   string `json:"provider"`
			CostMicros int64  `json:"cost_micros"`
			Runs       int    `json:"runs"`
		} `json:"by_model"`
		ByBoard []struct {
			BoardID string `json:"board_id"`
			Name    string `json:"name"`
			Runs    int    `json:"runs"`
		} `json:"by_board"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TotalMicros != 12_345_678 || body.WindowDays != 30 {
		t.Fatalf("totals wrong: %+v", body)
	}
	if len(body.ByModel) != 1 || body.ByModel[0].Model != "gpt-4o" || body.ByModel[0].Provider != "openai_compatible" {
		t.Fatalf("by_model wrong: %+v", body.ByModel)
	}
	if len(body.ByBoard) != 1 || body.ByBoard[0].Name != "Sprint" {
		t.Fatalf("by_board wrong: %+v", body.ByBoard)
	}
}

// TestCostSummaryWithNoSpendIsAnEmptyReport — US-AD32 AC3: zero, not an error.
func TestCostSummaryWithNoSpendIsAnEmptyReport(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	w := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/orgs/"+scenario.orgA+"/cost-summary",
		"alice", scenario.orgA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("empty cost summary = %d, want 200", w.Code)
	}
	// Arrays, not null: the screen iterates them.
	var body struct {
		TotalMicros int64           `json:"total_micros"`
		ByModel     json.RawMessage `json:"by_model"`
		ByBoard     json.RawMessage `json:"by_board"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TotalMicros != 0 {
		t.Fatalf("total = %d, want 0", body.TotalMicros)
	}
	if string(body.ByModel) != "[]" || string(body.ByBoard) != "[]" {
		t.Fatalf("empty report must use empty arrays, got by_model=%s by_board=%s", body.ByModel, body.ByBoard)
	}
}

// TestCostSummaryRequiresAdmin — §6.2.15's Role Min for the org-wide report.
func TestCostSummaryRequiresAdmin(t *testing.T) {
	mux, _, scenario := newRunAPI(t)

	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/orgs/"+scenario.orgA+"/cost-summary",
		"vera", scenario.orgA, "").Code; got != http.StatusForbidden {
		t.Fatalf("viewer reading the org cost summary = %d, want 403", got)
	}
	// andre is an Admin of orgA, so the same route answers him.
	if got := runRequest(t, mux, scenario, http.MethodGet, "/api/v1/orgs/"+scenario.orgA+"/cost-summary",
		"andre", scenario.orgA, "").Code; got != http.StatusOK {
		t.Fatalf("admin reading the org cost summary = %d, want 200", got)
	}
}
