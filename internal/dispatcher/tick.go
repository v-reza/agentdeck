package dispatcher

import (
	"context"
	"errors"
	"time"

	"agentdeck/internal/board"
)

// Run drives the tick loop until ctx is cancelled, then waits for the workers it
// spawned (5.2). Waiting rather than returning immediately matters: a run that is
// mid-flight when the process stops would otherwise be left `running` with no
// owner, and its task would sit there until a reclaim 15 minutes later.
func (d *Dispatcher) Run(ctx context.Context) {
	// Boot recovery first, not after the first tick: a process that restarted
	// still owns the runs its previous incarnation left behind, and those runs
	// have not been heartbeating while it was down.
	d.recoverOnBoot(ctx)

	ticker := time.NewTicker(d.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.workers.Wait()
			return
		case <-ticker.C:
			d.tickOnce(ctx)
		}
	}
}

// tickOnce is 5.2's sequence: heartbeat, reclaim, budget, dependency, claim, spawn.
func (d *Dispatcher) tickOnce(ctx context.Context) {
	boards, err := d.store.ClaimableBoards(ctx)
	if err != nil {
		d.log.Error("dispatcher: listing boards", "error", err)
		return
	}
	if err := d.store.HeartbeatOwned(ctx, ""); err != nil {
		// Not fatal: the heartbeat covers every org at once, and a failure here
		// shows up as runs going stale, which is recoverable.
		d.log.Warn("dispatcher: heartbeat failed", "error", err)
	}

	// Reclaim is off-tick (every 30 s, 4b) because it is a scan over ended runs and
	// the 2 s tick is for claiming.
	if time.Since(d.lastReclaim) >= ReclaimInterval {
		d.lastReclaim = time.Now()
		if err := d.reclaim(ctx, boards); err != nil {
			d.log.Error("dispatcher: reclaim", "error", err)
		}
	}

	for _, b := range boards {
		d.tickBoard(ctx, b)
	}
}

// tickBoard claims and starts work for one board. Errors are logged per board so
// one tenant's bad state cannot stop the others.
func (d *Dispatcher) tickBoard(ctx context.Context, b board.Board) {
	// 3c: budget before claiming. Claiming first and discovering the cap after
	// would have already started runs on a board that cannot pay for them.
	budget, err := d.store.BoardBudgetToday(ctx, b.OrgID, b.ID)
	if err != nil {
		d.log.Error("dispatcher: budget", "board", b.ID, "error", err)
		return
	}
	if budget.Exceeded() {
		// Nothing new starts while the cap is reached (N16). Existing runs are
		// cancelled when their step pushes spend past the cap (N17).
		d.log.Info("dispatcher: board over budget, skipping claim",
			"board", b.ID, "spent", budget.SpentTodayMicros, "cap", budget.CapMicros)
		return
	}

	// 3d: a ready task whose parents are unfinished is not claimable, and the
	// board should say why instead of showing it as queued work.
	if moved, err := d.store.BlockDependentTasks(ctx, b.OrgID, b.ID); err != nil {
		d.log.Error("dispatcher: dependency gate", "board", b.ID, "error", err)
	} else if moved > 0 {
		d.log.Info("dispatcher: tasks waiting on dependencies", "board", b.ID, "count", moved)
	}

	tasks, runIDs, err := d.store.ClaimBatch(ctx, b.OrgID, b.ID, d.batch)
	if err != nil {
		d.log.Error("dispatcher: claim", "board", b.ID, "error", err)
		return
	}
	for i, task := range tasks {
		d.spawn(ctx, b, task, runIDs[i])
	}
}

// spawn prepares and runs one claimed task. Preparation happens before the
// goroutine so a task that cannot start (no agent, no credential) is released to
// `ready` or `blocked` from the tick itself, rather than from a goroutine that
// has to re-read state to decide.
func (d *Dispatcher) spawn(ctx context.Context, b board.Board, task board.Task, runID string) {
	agent, err := d.resolveAgent(ctx, task)
	if err != nil {
		// The claim wrote `running` and a run id but no `runs` row exists yet, so
		// this release has to apply the outcome itself rather than close a run.
		d.abortClaim(ctx, task, "", err)
		return
	}
	if _, err := d.store.StartRun(ctx, task.OrgID, task.ID, agent.ID, runID); err != nil {
		d.abortClaim(ctx, task, "", err)
		return
	}

	// A cancel that landed between the claim and this line must not spend money:
	// the provider call is the expensive part, and there is no point making it
	// for work someone has already stopped. The run is closed as `cancelled`
	// without ever calling out.
	if d.cancelRequested(ctx, runID) {
		d.closeCancelled(ctx, task, runID)
		return
	}

	d.workers.Add(1)
	go func() {
		defer d.workers.Done()
		d.runOne(ctx, b, task, agent, runID)
	}()
}

// resolveAgent turns an assignment into something runnable: the agent row, its
// skills as prompt text, and a decrypted credential.
//
// The credential is read here and never stored on the agent value that gets
// logged — `board.Agent` has no field for it in log output, and the executor
// receives it to put in one request header.
func (d *Dispatcher) resolveAgent(ctx context.Context, task board.Task) (board.ResolvedAgent, error) {
	if task.AssigneeAgentID == "" {
		return board.ResolvedAgent{}, errNoAgent
	}
	agent, err := d.store.GetAgent(ctx, task.AssigneeAgentID, task.OrgID)
	if err != nil {
		return board.ResolvedAgent{}, err
	}
	if agent.ArchivedAt != nil {
		// US-AD73: an archived agent is not a valid assignee for new work. Tasks it
		// already started finish; new ones do not begin.
		return board.ResolvedAgent{}, errArchivedAgent
	}

	// The prompt is resolved before the credential so a task that will fail on a
	// malformed skills_json does not first decrypt a secret it then discards.
	agent, err = d.store.ResolveAgentPrompt(ctx, agent)
	if err != nil {
		return board.ResolvedAgent{}, err
	}

	endpoint := d.endpointFor(ctx, agent)
	if endpoint == "" {
		// DECISIONS 6A.J keeps the env-default path for an agent with no provider of
		// its own, but this deployment has no default configured. Refusing is the
		// honest answer: guessing an address would send the operator's credential
		// somewhere nobody chose.
		return board.ResolvedAgent{}, errNoProviderAddress
	}

	key, err := d.credentialFor(ctx, agent)
	if err != nil {
		return board.ResolvedAgent{}, err
	}
	return board.ResolvedAgent{Agent: agent, BaseURL: endpoint, APIKey: key}, nil
}

// credentialFor picks the credential a run authenticates with.
//
// §6A.J is unambiguous about where this lives: secrets moved to the `providers`
// entity, once per workspace. Reading the per-agent column first would have been
// the quiet version of this bug — an agent that happens to carry a stale key would
// authenticate with it and the run would fail as `capability` (401), which looks
// like a bad model name rather than a wrong source of truth. The agent column is
// consulted only for agents that have no provider at all.
//
// An empty key is not an error: an unauthenticated local model server is a
// supported setup, which is why the columns are nullable.
func (d *Dispatcher) credentialFor(ctx context.Context, agent board.Agent) (string, error) {
	if agent.ProviderID != "" {
		key, err := d.store.ProviderCredential(ctx, agent.OrgID, agent.ProviderID)
		if err != nil {
			return "", err
		}
		return key, nil
	}
	key, err := d.store.AgentProviderKey(ctx, agent.ID, agent.OrgID)
	if err != nil && !errors.Is(err, board.ErrNoProviderKey) {
		return "", err
	}
	return key, nil
}

// endpointFor returns the base URL for an agent: its own provider's, or the
// deployment default. Empty means neither exists.
func (d *Dispatcher) endpointFor(ctx context.Context, agent board.Agent) string {
	if agent.ProviderID == "" {
		return d.defaultEndpoint
	}
	baseURL, err := d.store.ProviderAddress(ctx, agent.OrgID, agent.ProviderID)
	if err != nil {
		d.log.Error("dispatcher: reading agent provider", "agent", agent.ID, "error", err)
		return ""
	}
	return baseURL
}
