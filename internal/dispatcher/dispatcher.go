// Package dispatcher is the tick loop of ARCHITECTURE 5: it is the only writer of
// task/run state, and it runs once per board on a 2 s tick (N19).
//
// What is here: the boot recovery, the heartbeat, the stale-run reclaim, the
// dependency gate, the budget gate, the batch claim with one run per task, and a
// per-task goroutine that executes the run and records what it cost.
//
// What is deliberately absent, and why — these are the parts that would be
// fabrications rather than implementations:
//
//   - **Agent selection.** 5.2 phase 3d says "pick the best agent or fail", but
//     no contract clause defines what "best" means. This resolves the task's own
//     `assignee_agent_id` and nothing else, which is what US-AD11 AC5 describes.
//   - **Tool execution.** See internal/executor: no tool runtime exists, so an
//     agent that declares tools fails as `capability` instead of being sent a
//     prompt promising tools it cannot call.
//   - **Approvals.** The `awaiting_approval` / `gate_mode` path needs the
//     approval machinery wired to a run, which is a separate slice.
//   - **SSE.** The screens already poll; a hub would add a second delivery path
//     for the same data before the first one is exercised.
package dispatcher

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"agentdeck/internal/board"
)

// Defaults come from the contract: N19 tick 2 s, N20 batch <= 20, N7 heartbeat
// 60 s, N8 stale reclaim 15 min.
const (
	DefaultTick      = 2 * time.Second
	DefaultBatch     = board.RunBucketSize
	DefaultHeartbeat = 60 * time.Second
	ReclaimInterval  = 30 * time.Second
	ReclaimBatch     = 50
)

// Store is the slice of the board service this package needs. It is an interface
// so the tick can be tested against a fake, and so the dispatcher cannot reach
// into board internals it has no business touching.
type Store interface {
	ClaimableBoards(ctx context.Context) ([]board.Board, error)
	BoardBudgetToday(ctx context.Context, orgID, boardID string) (board.BoardBudget, error)
	BlockDependentTasks(ctx context.Context, orgID, boardID string) (int64, error)
	ClaimBatch(ctx context.Context, orgID, boardID string, limit int) ([]board.Task, []string, error)
	StartRun(ctx context.Context, orgID, taskID, agentID, runID string) (board.Run, error)
	StartStep(ctx context.Context, orgID, runID string, step board.Step, payload any) (board.Step, error)
	FinishStep(ctx context.Context, orgID, runID string, seq int, step board.Step) (board.Step, error)
	AppendLedger(ctx context.Context, orgID, runID, taskID string, usage board.LedgerUsage) (board.LedgerEntry, error)
	BumpDailyCost(ctx context.Context, orgID, boardID string, micros, tokensIn, tokensOut int64) error
	BumpDailyRunCount(ctx context.Context, boardID string) error
	RecordRunUsage(ctx context.Context, runID string, micros, tokensIn, tokensOut int64) error
	EndRun(ctx context.Context, runID, orgID string, summary board.RunSummary) (board.Run, error)
	HeartbeatOwned(ctx context.Context, orgID string) error
	ReclaimStale(ctx context.Context, orgID string, limit, maxAttempts int) ([]board.Task, error)
	ReleaseClaim(ctx context.Context, taskID, orgID, runID, failureKind, detail string) error
	WakeDependents(ctx context.Context, taskID string) error
	GetAgent(ctx context.Context, id, orgID string) (board.Agent, error)
	ProviderAddress(ctx context.Context, orgID, id string) (string, error)
	ProviderCredential(ctx context.Context, orgID, id string) (string, error)
	AgentProviderKey(ctx context.Context, id, orgID string) (string, error)
	ResolveAgentPrompt(ctx context.Context, agent board.Agent) (board.Agent, error)
}

// Runner executes a claimed task. It is an interface so the dispatcher's own
// testing does not need a provider, and so the executor stays the only package
// that knows how to make an LLM call.
type Runner interface {
	Run(ctx context.Context, task board.Task, agent board.ResolvedAgent, runID string, sink Sink) (string, string)
}

// Sink is what the executor reports through. The dispatcher implements it, which
// is how the storage decisions stay in this package (5.3).
type Sink interface {
	Step(seq int, kind, status string, usage board.LedgerUsage, costMicros int64, priceVersion int, priceSource, pricingModel string, payload any) error
}

// Dispatcher is one instance's loop. Several may run at once: the claim SQL is
// SKIP LOCKED, and claim_lock keeps each instance's heartbeat to its own runs.
type Dispatcher struct {
	store   Store
	runner  Runner
	tick    time.Duration
	batch   int
	log     *slog.Logger
	workers sync.WaitGroup

	// defaultEndpoint is the deployment's fallback base URL for agents that carry
	// no provider of their own (DECISIONS 6A.J). Empty means there is no fallback,
	// in which case such an agent cannot run.
	defaultEndpoint string

	// cancelled tracks runs this instance ended because the board hit its cap, so
	// the goroutine reports `budget_exceeded` rather than a generic failure.
	mu        sync.Mutex
	cancelled map[string]bool

	lastReclaim time.Time
}

// New builds a dispatcher. A nil logger falls back to the default so a caller can
// never accidentally lose the tick's diagnostics.
func New(store Store, runner Runner, tick time.Duration, batch int, log *slog.Logger) *Dispatcher {
	if tick <= 0 {
		tick = DefaultTick
	}
	if batch <= 0 || batch > DefaultBatch {
		batch = DefaultBatch
	}
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{
		store: store, runner: runner, tick: tick, batch: batch, log: log,
		cancelled: map[string]bool{},
	}
}
