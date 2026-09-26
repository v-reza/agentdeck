package dispatcher

import (
	"context"

	"agentdeck/internal/board"
	"agentdeck/internal/executor"
	"agentdeck/internal/pricing"
)

// This package adapts the executor to the tick loop. It exists so neither side
// has to import the other: the executor takes a Pricer interface and a Sink
// interface, the dispatcher takes a Runner interface, and everything concrete is
// named here — once, in one place, where a field mismatch between executor.Step
// and board.Step is a compile error rather than a runtime surprise inside a run
// nobody is watching.

// ExecutorRunner runs a task through internal/executor.
type ExecutorRunner struct {
	// Manual is the org's own price override for this agent's model, if it has
	// one. DECISIONS 6A.C puts it at tier 1 of the four-tier resolution, above the
	// catalog, so it is passed in rather than looked up inside the executor.
	Manual *pricing.ModelPrice
}

// Run satisfies Runner. It returns the model's output and, on failure, the §10
// failure kind; an empty kind means the run succeeded.
func (r ExecutorRunner) Run(ctx context.Context, task board.Task, resolved board.ResolvedAgent, runID string, sink Sink) (string, string) {
	// The API key travels separately from the agent value: `board.Agent` is the
	// shape the UI and logs see, and it deliberately has no field for it.
	execAgent := executor.Agent{
		Name:              resolved.Name,
		Provider:          resolved.Provider,
		Model:             resolved.Model,
		BaseURL:           resolved.BaseURL,
		APIKey:            resolved.APIKey,
		Tools:             resolved.Tools,
		MaxRuntimeSeconds: resolved.MaxRuntimeSeconds,
		ReasoningEffort:   resolved.ReasoningEffort,
	}
	for _, s := range resolved.Skills {
		execAgent.Skills = append(execAgent.Skills, executor.Skill{Slug: s.Slug, Name: s.Name, BodyMD: s.BodyMD})
	}

	// The sink is the executor's interface; the dispatcher's own stepSink is
	// adapted below rather than reshaped, because the executor must not learn about
	// run ids or boards.
	result := executor.Execute(ctx, execAgent, executor.Task{ID: task.ID, Title: task.Title, Body: task.Body},
		pricer{r.Manual}, func(step executor.Step) error {
			return sink.Step(step.Seq, step.Kind, step.Status,
				board.LedgerUsage{
					Provider:         resolved.Provider,
					Model:            resolved.Model,
					TokensIn:         step.TokensIn,
					TokensOut:        step.TokensOut,
					CacheReadTokens:  step.CacheRead,
					CacheWriteTokens: step.CacheWrite,
					ReasoningTokens:  step.Reasoning,
				},
				step.CostMicros, step.PriceVersion, step.PriceSource, step.PricingModel, step.Payload)
		})

	if result.Error != "" {
		return result.Error, result.FailureKind
	}
	return result.Output, ""
}

// pricer adapts the pricing catalog to executor.Pricer.
type pricer struct{ manual *pricing.ModelPrice }

func (p pricer) Price(model string, in, out, cached int64) (int64, int, string, string) {
	res := pricing.Resolve(model, p.manual)
	cost := res.Cost(pricing.Tokens{In: in, Out: out, Cached: cached})
	return cost, res.Price.PriceVersion, string(res.Source), res.PricingModel
}
