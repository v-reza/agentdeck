package board

// Budget board dan laporan biaya org terhadap Postgres nyata — sisa §6.2.15.
//
// `OrgCostSummary` adalah SQL baru (dua GROUPING SETS), jadi ini satu-satunya
// tempat ia benar-benar dijalankan. Dan `BoardBudgetToday` sekarang juga
// mengembalikan `day`/`tokens_in`/`tokens_out`: kolom yang ada di query tapi
// tidak pernah sampai ke pemanggil adalah pola yang sudah tiga kali muncul di
// sesi ini (sessions.user_agent, runs.cancel_requested_at), jadi yang baru
// ditambahkan ikut diuji sampai ke nilainya.
//
// Gate: AGENTDECK_TEST_DATABASE_URL, sama seperti paket ini.

import (
	"context"
	"testing"
)

// TestPgBoardBudgetReportsTheAggregate — N16/N18.
func TestPgBoardBudgetReportsTheAggregate(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// 16M of the fixture's 20M cap is exactly the N18 threshold (80%).
	if err := f.svc.BumpDailyCost(ctx, f.orgID, f.boardID, 16_000_000, 1000, 200); err != nil {
		t.Fatalf("bump cost: %v", err)
	}
	if err := f.svc.BumpDailyRunCount(ctx, f.boardID); err != nil {
		t.Fatalf("bump run count: %v", err)
	}

	b, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if b.SpentTodayMicros != 16_000_000 || b.CapMicros != DefaultBudgetDailyMicros {
		t.Fatalf("spend/cap = %d/%d", b.SpentTodayMicros, b.CapMicros)
	}
	if b.RunCount != 1 {
		t.Fatalf("run_count = %d, want 1", b.RunCount)
	}
	// The three fields added for GET /boards/{id}/budget. They are in the
	// aggregate row the dispatcher already writes, so reporting them must not
	// require a second read — but it does require them to survive the adapter.
	if b.TokensIn != 1000 || b.TokensOut != 200 {
		t.Fatalf("tokens = %d/%d, want 1000/200", b.TokensIn, b.TokensOut)
	}
	if b.Day == "" {
		t.Fatal("day is empty; the response cannot say which day it measured")
	}
	if b.Status != "warning" {
		t.Fatalf("status at 80%% = %q, want warning", b.Status)
	}
}

// TestPgBoardBudgetOnAFreshBoardKeepsItsCap — the guardrail must not depend on a
// daily_board_costs row existing for today.
func TestPgBoardBudgetOnAFreshBoardKeepsItsCap(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	b, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if b.CapMicros != DefaultBudgetDailyMicros {
		t.Fatalf("cap = %d, want %d", b.CapMicros, DefaultBudgetDailyMicros)
	}
	if b.SpentTodayMicros != 0 || b.RunCount != 0 || b.TokensIn != 0 {
		t.Fatalf("fresh board reports spend: %+v", b)
	}
	if b.Day == "" {
		t.Fatal("a board with no rows for today still needs a day in the response")
	}
}

// TestPgUpdateBoardBudgetChangesWhatTheGateReads — §6.2.15, and the point of the
// endpoint: the number the dispatcher's cost gate reads is the number this
// writes. A patch that only touched `boards.budget_daily_micros` without the
// aggregate seeing it would be a cap that does not stop anything.
func TestPgUpdateBoardBudgetChangesWhatTheGateReads(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	// Spend 19M against the 20M default: over the 80% alert, under the cap.
	if err := f.svc.BumpDailyCost(ctx, f.orgID, f.boardID, 19_000_000, 0, 0); err != nil {
		t.Fatalf("bump: %v", err)
	}
	before, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if before.Status != "warning" || before.Exceeded() {
		t.Fatalf("setup: status=%q exceeded=%v", before.Status, before.Exceeded())
	}

	// Raise the cap above the spend: the alert must clear.
	if err := f.svc.UpdateBoardBudget(ctx, f.boardID, f.orgID, 50_000_000); err != nil {
		t.Fatalf("update budget: %v", err)
	}
	raised, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget after raise: %v", err)
	}
	if raised.CapMicros != 50_000_000 {
		t.Fatalf("cap = %d, want 50_000_000", raised.CapMicros)
	}
	if raised.Status != "ok" {
		t.Fatalf("after raising the cap: status = %q, want ok", raised.Status)
	}
	if raised.SpentTodayMicros != 19_000_000 {
		t.Fatalf("raising the cap changed the spend: %d", raised.SpentTodayMicros)
	}

	// Lower it below the spend: the gate must now refuse.
	if err := f.svc.UpdateBoardBudget(ctx, f.boardID, f.orgID, 1_000_000); err != nil {
		t.Fatalf("lower budget: %v", err)
	}
	lowered, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget after lowering: %v", err)
	}
	if !lowered.Exceeded() {
		t.Fatalf("spend 19M against a 1M cap reports %q, not exceeded", lowered.Status)
	}
}

// TestPgUpdateBoardBudgetRejectsNegative — the service floor.
func TestPgUpdateBoardBudgetRejectsNegative(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()

	if err := f.svc.UpdateBoardBudget(ctx, f.boardID, f.orgID, -1); err == nil {
		t.Fatal("a negative cap was accepted")
	}
	// Zero is legitimate: it means "stop spending".
	if err := f.svc.UpdateBoardBudget(ctx, f.boardID, f.orgID, 0); err != nil {
		t.Fatalf("a zero cap was refused: %v", err)
	}
	b, err := f.svc.BoardBudgetToday(ctx, f.orgID, f.boardID)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if !b.Exceeded() {
		t.Fatal("a zero cap does not read as exceeded, so it would not stop a claim")
	}
}

// TestPgCostSummaryTotalsMatchTheirParts — the invariant the single statement
// exists to guarantee: the headline is the sum of the breakdowns.
func TestPgCostSummaryTotalsMatchTheirParts(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)
	ctx := context.Background()

	for _, e := range []LedgerUsage{
		{Provider: "openai", Model: "gpt-4o", Kind: "llm", TokensIn: 100, TokensOut: 50,
			CostMicros: 1500, PriceVersion: 1, PriceSource: "catalog", PricingModel: "gpt-4o"},
		{Provider: "anthropic", Model: "claude-3", Kind: "llm", TokensIn: 200, TokensOut: 20,
			CostMicros: 2500, PriceVersion: 1, PriceSource: "catalog", PricingModel: "claude-3"},
	} {
		if _, err := f.svc.AppendLedger(ctx, f.orgID, f.runID, f.taskID, e); err != nil {
			t.Fatalf("AppendLedger: %v", err)
		}
	}

	summary, err := f.svc.OrgCostSummary(ctx, f.orgID)
	if err != nil {
		t.Fatalf("OrgCostSummary: %v", err)
	}
	if summary.TotalMicros != 4000 {
		t.Fatalf("total = %d, want 4000", summary.TotalMicros)
	}
	if len(summary.ByModel) != 2 {
		t.Fatalf("by_model rows = %d, want 2: %+v", len(summary.ByModel), summary.ByModel)
	}
	var modelSum int64
	for _, m := range summary.ByModel {
		modelSum += m.CostMicros
		if m.Model == "" || m.Provider == "" {
			t.Fatalf("a model row lost its identity: %+v", m)
		}
	}
	if modelSum != summary.TotalMicros {
		t.Fatalf("by_model sums to %d but the total says %d", modelSum, summary.TotalMicros)
	}
	if len(summary.ByBoard) != 1 {
		t.Fatalf("by_board rows = %d, want 1: %+v", len(summary.ByBoard), summary.ByBoard)
	}
	if summary.ByBoard[0].BoardID != f.boardID || summary.ByBoard[0].CostMicros != 4000 {
		t.Fatalf("by_board row wrong: %+v", summary.ByBoard[0])
	}
	if summary.ByBoard[0].BoardName == "" {
		t.Fatal("by_board lost the board's name")
	}
}

// TestPgCostSummaryIsScopedToItsOrg — tenant boundary on the report.
func TestPgCostSummaryIsScopedToItsOrg(t *testing.T) {
	f := newRuntimeFixture(t, "always", 3)
	f.start(t)
	ctx := context.Background()

	if _, err := f.svc.AppendLedger(ctx, f.orgID, f.runID, f.taskID, LedgerUsage{
		Provider: "openai", Model: "gpt-4o", Kind: "llm", CostMicros: 5000,
		PriceVersion: 1, PriceSource: "catalog", PricingModel: "gpt-4o",
	}); err != nil {
		t.Fatalf("AppendLedger: %v", err)
	}

	// Another org sees none of it — and sees a well-formed empty report, not an
	// error (US-AD32 AC3).
	other := newOrg(t, ctx)
	summary, err := f.svc.OrgCostSummary(ctx, other)
	if err != nil {
		t.Fatalf("OrgCostSummary for another org: %v", err)
	}
	if summary.TotalMicros != 0 || len(summary.ByModel) != 0 || len(summary.ByBoard) != 0 {
		t.Fatalf("another org sees this org's spend: %+v", summary)
	}
	// The arrays are non-nil so the handler serialises [] rather than null.
	if summary.ByModel == nil || summary.ByBoard == nil {
		t.Fatal("empty report has nil slices; the JSON would be null instead of []")
	}
}
