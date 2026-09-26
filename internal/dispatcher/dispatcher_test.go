package dispatcher

import (
	"context"
	"errors"
	"testing"

	"agentdeck/internal/board"
)

// fakeStore records what the tick did instead of touching a database. The point of
// these tests is the tick's DECISIONS — what it claims, what it skips, what it
// releases — not the SQL underneath, which internal/board tests against Postgres.
type fakeStore struct {
	boards    []board.Board
	budget    board.BoardBudget
	budgetErr error

	blockedDeps int64
	claimed     int
	claimRunIDs [][]string
	started     []string
	ended       map[string]board.RunSummary
	released    []string
	woken       []string
	startRunErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		boards: []board.Board{{ID: "b1", OrgID: "o1"}},
		budget: board.BoardBudget{BoardID: "b1", CapMicros: 20_000_000, SpentTodayMicros: 0, Status: "ok"},
		ended:  map[string]board.RunSummary{},
	}
}

func (f *fakeStore) ClaimableBoards(context.Context) ([]board.Board, error) { return f.boards, nil }

func (f *fakeStore) BoardBudgetToday(context.Context, string, string) (board.BoardBudget, error) {
	return f.budget, f.budgetErr
}

func (f *fakeStore) BlockDependentTasks(context.Context, string, string) (int64, error) {
	return f.blockedDeps, nil
}

func (f *fakeStore) ClaimBatch(_ context.Context, _, _ string, limit int) ([]board.Task, []string, error) {
	if f.claimed >= limit {
		return nil, nil, nil
	}
	f.claimed++
	ids := []string{"run-1"}
	f.claimRunIDs = append(f.claimRunIDs, ids)
	return []board.Task{{
		ID: "t1", OrgID: "o1", BoardID: "b1", Status: board.StatusRunning,
		AssigneeAgentID: "a1",
	}}, ids, nil
}

func (f *fakeStore) StartRun(_ context.Context, _, taskID, _, runID string) (board.Run, error) {
	if f.startRunErr != nil {
		return board.Run{}, f.startRunErr
	}
	f.started = append(f.started, runID)
	return board.Run{ID: runID, TaskID: taskID, Status: board.RunRunning}, nil
}

func (f *fakeStore) StartStep(context.Context, string, string, board.Step, any) (board.Step, error) {
	return board.Step{}, nil
}
func (f *fakeStore) FinishStep(_ context.Context, _ string, _ string, _ int, s board.Step) (board.Step, error) {
	return s, nil
}
func (f *fakeStore) AppendLedger(context.Context, string, string, string, board.LedgerUsage) (board.LedgerEntry, error) {
	return board.LedgerEntry{}, nil
}
func (f *fakeStore) BumpDailyCost(context.Context, string, string, int64, int64, int64) error {
	return nil
}
func (f *fakeStore) BumpDailyRunCount(context.Context, string) error { return nil }
func (f *fakeStore) RecordRunUsage(context.Context, string, int64, int64, int64) error {
	return nil
}

func (f *fakeStore) EndRun(_ context.Context, runID, _ string, s board.RunSummary) (board.Run, error) {
	f.ended[runID] = s
	return board.Run{ID: runID, Status: board.RunEnded, Outcome: s.Outcome}, nil
}

func (f *fakeStore) HeartbeatOwned(context.Context, string) error { return nil }
func (f *fakeStore) ReclaimStale(context.Context, string, int, int) ([]board.Task, error) {
	return nil, nil
}
func (f *fakeStore) WakeDependents(_ context.Context, taskID string) error {
	f.woken = append(f.woken, taskID)
	return nil
}
func (f *fakeStore) ReleaseClaim(_ context.Context, taskID, _, _, kind, _ string) error {
	f.released = append(f.released, taskID+":"+kind)
	return nil
}

func (f *fakeStore) GetAgent(context.Context, string, string) (board.Agent, error) {
	return board.Agent{ID: "a1", OrgID: "o1", ProviderID: "p1", Model: "m"}, nil
}

func (f *fakeStore) ProviderAddress(context.Context, string, string) (string, error) {
	return "https://api.example.test", nil
}

func (f *fakeStore) AgentProviderKey(context.Context, string, string) (string, error) {
	return "", board.ErrNoProviderKey
}

// ProviderCredential is where a run's key comes from since §6A.J moved credentials
// onto the provider entity.
func (f *fakeStore) ProviderCredential(context.Context, string, string) (string, error) {
	return "sk-provider", nil
}

func (f *fakeStore) ResolveAgentPrompt(_ context.Context, a board.Agent) (board.Agent, error) {
	return a, nil
}

// fakeRunner stands in for the executor. It reports one step and returns whatever
// outcome the test wants, so the tick's own handling is what is under test.
type fakeRunner struct {
	failureKind string
	cancel      bool
	stepErr     error
	steps       int
}

func (r *fakeRunner) Run(ctx context.Context, task board.Task, _ board.ResolvedAgent, _ string, sink Sink) (string, string) {
	r.steps++
	if r.stepErr == nil {
		_ = sink.Step(1, "llm", "succeeded", board.LedgerUsage{TokensIn: 10, TokensOut: 2}, 100, 1, "catalog", "m", nil)
	}
	return "output", r.failureKind
}

func newTestDispatcher(store Store, runner Runner) *Dispatcher {
	return New(store, runner, DefaultTick, DefaultBatch, nil)
}

func TestTickSkipsClaimWhenBoardIsOverBudget(t *testing.T) {
	store := newFakeStore()
	store.budget = board.BoardBudget{BoardID: "b1", CapMicros: 1_000_000, SpentTodayMicros: 1_000_000, Status: "exceeded"}
	store.boards = nil // tickBoard is called directly, so the board comes in as an argument
	d := newTestDispatcher(store, &fakeRunner{})

	// ClaimableBoards would return the board; call tickBoard with it directly so the
	// budget gate is the only thing under test.
	d.tickBoard(context.Background(), board.Board{ID: "b1", OrgID: "o1"})

	if store.claimed != 0 {
		t.Errorf("claimed %d tasks on an over-budget board", store.claimed)
	}
}

func TestTickClaimsAndStartsWithinBudget(t *testing.T) {
	store := newFakeStore()
	d := newTestDispatcher(store, &fakeRunner{})
	d.tickBoard(context.Background(), board.Board{ID: "b1", OrgID: "o1"})
	d.workers.Wait()

	if store.claimed != 1 {
		t.Fatalf("claimed %d, want 1", store.claimed)
	}
	if len(store.started) != 1 {
		t.Fatalf("started %d runs, want 1", len(store.started))
	}
	summary, ok := store.ended["run-1"]
	if !ok {
		t.Fatal("the run was never ended")
	}
	if summary.Outcome != "succeeded" {
		t.Errorf("outcome = %q, want succeeded", summary.Outcome)
	}
	if len(store.woken) != 1 {
		t.Errorf("dependents woken %d times, want 1", len(store.woken))
	}
}

// TestTickMarksAFailedRun: the outcome and failure kind must travel to EndRun,
// because the retry decision (§10) lives behind that call.
func TestTickMarksAFailedRun(t *testing.T) {
	store := newFakeStore()
	d := newTestDispatcher(store, &fakeRunner{failureKind: "transient"})
	d.tickBoard(context.Background(), board.Board{ID: "b1", OrgID: "o1"})
	d.workers.Wait()

	summary := store.ended["run-1"]
	if summary.Outcome != "failed" || summary.FailureKind != "transient" {
		t.Errorf("summary = %+v, want failed/transient", summary)
	}
	if len(store.woken) != 0 {
		t.Error("a failed run woke its dependents")
	}
}

// TestTickReleasesAClaimItCannotStart is the regression guard for a task that was
// claimed but whose run never began: without a release it stays `running` forever,
// invisible to reclaim (no run row) and to the claim predicate (not `ready`).
func TestTickReleasesAClaimItCannotStart(t *testing.T) {
	store := newFakeStore()
	store.startRunErr = errors.New("run insert refused")
	d := newTestDispatcher(store, &fakeRunner{})

	d.tickBoard(context.Background(), board.Board{ID: "b1", OrgID: "o1"})
	d.workers.Wait()

	if len(store.released) != 1 {
		t.Fatalf("released %v, want one release", store.released)
	}
	if store.released[0] != "t1:unknown" {
		t.Errorf("released as %q", store.released[0])
	}
}

// TestStartFailureKinds pins the §10 classification of the three ways a claim
// cannot start. A retry cannot fix any of them, so each must land somewhere other
// than `ready`.
func TestStartFailureKinds(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errNoAgent, "needs_input"},
		{errArchivedAgent, "needs_input"},
		{errNoProviderAddress, "capability"},
		{errors.New("something else"), "unknown"},
	}
	for _, tc := range cases {
		if got := startFailureKind(tc.err); got != tc.want {
			t.Errorf("%v -> %q, want %q", tc.err, got, tc.want)
		}
	}
}

// TestBlockedKindsMatchTheRetryMatrix: §10.2 sends these four to `blocked`, and
// retrying any of them spends money to be refused again.
func TestBlockedKindsMatchTheRetryMatrix(t *testing.T) {
	blocked := map[string]bool{"needs_input": true, "dependency": true, "policy": true, "budget": true}
	for kind, want := range blocked {
		if _, got := board.BlockedKind(kind); got != want {
			t.Errorf("BlockedKind(%q) blocked = %v, want %v", kind, got, want)
		}
	}
	for _, kind := range []string{"transient", "capability", "unknown", ""} {
		if _, got := board.BlockedKind(kind); got {
			t.Errorf("BlockedKind(%q) blocked = true, want false", kind)
		}
	}
}

func TestNewClampsBatchnadTick(t *testing.T) {
	d := New(newFakeStore(), &fakeRunner{}, 0, 0, nil)
	if d.tick != DefaultTick {
		t.Errorf("tick = %v, want %v", d.tick, DefaultTick)
	}
	if d.batch != DefaultBatch {
		t.Errorf("batch = %d, want %d", d.batch, DefaultBatch)
	}
	// A batch above the contract ceiling (N20) is clamped rather than honoured: the
	// cap is what keeps one tick from claiming a whole board.
	d2 := New(newFakeStore(), &fakeRunner{}, 0, 1000, nil)
	if d2.batch != DefaultBatch {
		t.Errorf("batch = %d, want it clamped to %d", d2.batch, DefaultBatch)
	}
}

// TestCredentialComesFromTheProviderNotTheAgent is the regression guard for the
// §6A.J source of truth. The fake's per-agent key is deliberately unset and its
// provider key is set, so a resolver that reads the agent first gets an empty
// credential — which the provider answers with 401, reported as `capability`, and
// which looks like a bad model name instead of a wrong key lookup.
func TestCredentialComesFromTheProviderNotTheAgent(t *testing.T) {
	store := newFakeStore()
	d := newTestDispatcher(store, &fakeRunner{})

	key, err := d.credentialFor(context.Background(), board.Agent{ID: "a1", OrgID: "o1", ProviderID: "p1"})
	if err != nil {
		t.Fatalf("credentialFor: %v", err)
	}
	if key != "sk-provider" {
		t.Errorf("key = %q, want the provider's", key)
	}

	// An agent with no provider falls back to its own column, and a missing key
	// there is not an error: an unauthenticated local server is supported.
	key, err = d.credentialFor(context.Background(), board.Agent{ID: "a1", OrgID: "o1"})
	if err != nil {
		t.Fatalf("fallback credential: %v", err)
	}
	if key != "" {
		t.Errorf("key = %q, want empty for an agent with no stored credential", key)
	}
}
