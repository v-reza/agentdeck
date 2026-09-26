// Package executor runs one claimed task against one agent: it builds the
// prompt, makes the LLM call, and reports what each step cost.
//
// It is deliberately split from the dispatcher (ARCHITECTURE 5.3): this package
// knows how to talk to a provider and nothing about the database, so the whole
// thing is testable against an httptest server. The dispatcher owns the writes
// and implements Sink.
//
// Scope, and why it stops here: one user turn, one completion. There is no
// tool-calling loop, because `agents.tools_json` is an allowlist whose tools do
// not exist yet in this repo — inventing a tool runtime would put fabricated
// capabilities in the prompt, and an agent that is promised a tool it cannot call
// fails as `capability` (§10) on every run. `tools_json` is validated against the
// allowlist before the call so the mismatch is reported instead of silently
// ignored.
//
// ponytail: one completion per run. Upgrade path is a bounded tool loop around
// `callOnce` — `Step.Seq` and the sink already carry per-step records, so the
// loop needs no new persistence.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Failure kinds are the seven of ARCHITECTURE §10 / DECISIONS §4. The executor
// names one per failed run; the retry policy on the agent decides what happens
// next, so this package never decides to retry.
const (
	FailureTransient  = "transient"
	FailureNeedsInput = "needs_input"
	FailureCapability = "capability"
	FailureDependency = "dependency"
	FailurePolicy     = "policy"
	FailureBudget     = "budget"
	FailureUnknown    = "unknown"
)

const (
	// maxResponseBytes bounds a provider response so a hostile or broken endpoint
	// cannot stream forever. Same 2 MiB ceiling the credential probe uses.
	maxResponseBytes = 2 << 20
	// stepContentBytes bounds what is written into a step payload. The point of
	// the payload is to show a human what was sent and returned; a full
	// completion is not worth unbounded storage.
	stepContentBytes = 8 << 10
	// defaultMaxTokens is used because `agents` has no max_tokens column and the
	// contract does not define one. Without a ceiling the provider's own default
	// applies, which varies by vendor and can be very large.
	defaultMaxTokens = 4096
	// defaultTimeout applies when an agent row arrives without a runtime limit.
	defaultTimeout = 4 * time.Hour // N9
)

// Skill is one entry of `agent_skills`, resolved from `agents.skills_json`.
type Skill struct {
	Slug   string
	Name   string
	BodyMD string
}

// Agent is the execution-relevant slice of an `agents` row. The dispatcher
// resolves credentials and skills before calling: this package must never reach
// for a secret itself.
type Agent struct {
	Name              string
	Provider          string // protocol/dialect (US-AD109)
	Model             string
	BaseURL           string // validated provider base URL, no trailing slash needed
	APIKey            string // decrypted; never logged, never put in an error
	Skills            []Skill
	Tools             []string
	MaxRuntimeSeconds int
	ReasoningEffort   string
}

// Task is the execution-relevant slice of a task row.
type Task struct {
	ID    string
	Title string
	Body  string
}

// Step is one recorded unit of work. The dispatcher persists it; the fields map
// onto `steps` (ARCHITECTURE 3.6) and `ledger_entries` (3.7).
type Step struct {
	Seq          int
	Kind         string // llm | cache_read | cache_write | tool
	Status       string // running | succeeded | failed
	TokensIn     int64
	TokensOut    int64
	CacheRead    int64
	CacheWrite   int64
	Reasoning    int64
	CostMicros   int64
	PriceVersion int
	PriceSource  string
	PricingModel string
	Payload      any
}

// Result is what a finished run reports back. Error is empty on success.
type Result struct {
	Output      string
	Error       string
	FailureKind string
}

// Pricer turns observed token usage into micro-USD at a named snapshot. It is an
// interface so the executor does not import the pricing catalog, which keeps the
// two independently testable; `pricing.Resolver` satisfies it.
type Pricer interface {
	Price(model string, in, out, cached int64) (costMicros int64, version int, source, pricingModel string)
}

// Execute runs the task and reports every step through emit. The first error
// from emit aborts the run: a step that cannot be recorded must not be followed
// by more spend.
func Execute(ctx context.Context, agent Agent, task Task, pricer Pricer, emit func(Step) error) Result {
	if strings.TrimSpace(agent.BaseURL) == "" {
		// No address to call. Reported as capability rather than transient: no
		// amount of retrying invents an endpoint.
		return Result{Error: "agent has no provider address configured", FailureKind: FailureCapability}
	}

	system, dropped := buildPrompt(agent, task)
	if dropped > 0 {
		// The prompt promised tools that this runtime cannot call. Failing here is
		// the honest answer (§10 capability): sending it anyway would have the
		// model plan around tools that never appear.
		return Result{
			Error:       fmt.Sprintf("agent configures %d tool(s) this runtime cannot call", dropped),
			FailureKind: FailureCapability,
		}
	}

	timeout := time.Duration(agent.MaxRuntimeSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	completed, err := callOnce(ctx, agent, system, task)
	cost, version, source, pricingModel := pricer.Price(agent.Model,
		completed.Usage.PromptTokens, completed.Usage.CompletionTokens, completed.Usage.CachedTokens)

	step := Step{
		Seq: 1, Kind: "llm", Status: "succeeded",
		TokensIn: completed.Usage.PromptTokens, TokensOut: completed.Usage.CompletionTokens,
		CacheRead: completed.Usage.CachedTokens, Reasoning: completed.Usage.ReasoningTokens,
		CostMicros: cost, PriceVersion: version, PriceSource: source, PricingModel: pricingModel,
		Payload: map[string]any{
			"request": map[string]any{
				"model": agent.Model, "system": truncate(system, stepContentBytes),
				"messages": []map[string]string{{"role": "user", "content": truncate(userMessage(task), stepContentBytes)}},
			},
			"response": map[string]any{"content": truncate(completed.Content, stepContentBytes)},
			"usage":    completed.Usage,
		},
	}
	if err != nil {
		step.Status = "failed"
		// A failed call still spends the prompt tokens the provider counted, so
		// the step keeps whatever usage came back rather than reporting zero.
	}
	if emitErr := emit(step); emitErr != nil {
		return Result{Error: emitErr.Error(), FailureKind: FailureUnknown}
	}
	if err != nil {
		return Result{Error: err.Error(), FailureKind: classify(err)}
	}
	return Result{Output: completed.Content}
}

// userMessage is the per-task half of the prompt. The skill bodies are the
// system half; a task contributes its own title and body and nothing else.
func userMessage(task Task) string {
	body := strings.TrimSpace(task.Body)
	if body == "" {
		return task.Title
	}
	return task.Title + "\n\n" + body
}

// buildPrompt assembles the system message from the agent's skills and returns
// how many configured tools were dropped. Skills are concatenated in the order
// the agent lists them: `skills_json` is an ordered selection, and order is the
// only priority signal it carries.
func buildPrompt(agent Agent, task Task) (string, int) {
	var b strings.Builder
	b.WriteString("You are ")
	if agent.Name != "" {
		b.WriteString(agent.Name)
	} else {
		b.WriteString("an automated worker")
	}
	b.WriteString(" running one task on AgentDeck. Answer with the work itself; do not restate the task.\n")
	if agent.ReasoningEffort != "" {
		b.WriteString("\nReasoning effort: " + agent.ReasoningEffort + "\n")
	}
	for _, skill := range agent.Skills {
		b.WriteString("\n## Skill: ")
		b.WriteString(skill.Name)
		b.WriteString(" (")
		b.WriteString(skill.Slug)
		b.WriteString(")\n")
		b.WriteString(skill.BodyMD)
		b.WriteString("\n")
	}
	// Every declared tool is undeliverable today, so the count of dropped tools
	// is simply the count of declared ones.
	return b.String(), len(agent.Tools)
}

// ------------------------------------------------------------------ wire ----

type usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	// OpenAI-compatible servers nest the cache hit count under
	// prompt_tokens_details rather than reporting it flat.
	PromptDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type completion struct {
	Content string
	Usage   usage
}

type completionEnvelope struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// providerError carries the HTTP status so classify can work on it after the
// request scope has unwound.
type providerError struct {
	Status int
	Detail string
}

func (e *providerError) Error() string {
	return fmt.Sprintf("provider returned HTTP %d: %s", e.Status, e.Detail)
}

// callOnce performs the single completion. The success return value is used even
// when an error is returned: a failed call can still report token usage.
func callOnce(ctx context.Context, agent Agent, system string, task Task) (completion, error) {
	base := strings.TrimRight(agent.BaseURL, "/")
	url := base + "/chat/completions"

	body, err := json.Marshal(map[string]any{
		"model":      agent.Model,
		"max_tokens": defaultMaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userMessage(task)},
		},
	})
	if err != nil {
		return completion{}, fmt.Errorf("executor: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return completion{}, fmt.Errorf("executor: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if agent.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+agent.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		// A missing endpoint and a slow one are both worth another try; a caller
		// that ran out of time is not (the dispatcher reads that as timed_out).
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return completion{}, ctx.Err()
		}
		return completion{}, fmt.Errorf("executor: POST %s: %w", redactString(url, agent.APIKey), err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return completion{}, fmt.Errorf("executor: reading response: %w", err)
	}

	var env completionEnvelope
	// Only a 2xx body is expected to be a completion; error bodies are parsed for
	// their message but not for choices.
	_ = json.Unmarshal(raw, &env)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail := strings.TrimSpace(string(raw))
		if env.Error != nil && env.Error.Message != "" {
			detail = env.Error.Message
		}
		// The detail is the one field a provider can put the credential into: a
		// 401 body that echoes the Authorization header is the common case.
		return completion{Usage: normalizeUsage(env.Usage)},
			&providerError{Status: resp.StatusCode, Detail: redactString(truncate(detail, 500), agent.APIKey)}
	}

	if len(env.Choices) == 0 {
		return completion{Usage: normalizeUsage(env.Usage)},
			fmt.Errorf("executor: response carried no choices: %s", truncate(string(raw), 300))
	}
	return completion{
		Content: env.Choices[0].Message.Content,
		Usage:   normalizeUsage(env.Usage),
	}, nil
}

// normalizeUsage folds the nested detail objects into the flat fields the
// contract's tokeners expect. Vendors disagree on where these live: OpenAI puts
// cached tokens under `prompt_tokens_details`, some gateways report them flat,
// and Anthropic-style reasoning is under `completion_tokens_details`.
func normalizeUsage(u usage) usage {
	if u.CachedTokens == 0 {
		u.CachedTokens = u.PromptDetails.CachedTokens
	}
	if u.ReasoningTokens == 0 {
		u.ReasoningTokens = u.CompletionDetails.ReasoningTokens
	}
	return u
}

// ------------------------------------------------------------------ errors --

// classify maps a transport/HTTP failure onto the §10 taxonomy. The mapping is
// the whole reason retries are not blind: a 401 will fail again identically, and
// retrying it spends the operator's money to learn nothing.
func classify(err error) string {
	var pe *providerError
	if errors.As(err, &pe) {
		switch {
		case pe.Status == http.StatusTooManyRequests || pe.Status == http.StatusRequestTimeout,
			pe.Status >= 500:
			// 429 and 5xx are the textbook transient cases in §10.1.
			return FailureTransient
		case pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden:
			// The agent lacks access to what it needs. Not transient: the
			// credential or the permission has to change first.
			return FailureCapability
		case pe.Status == http.StatusNotFound, pe.Status == http.StatusBadRequest,
			pe.Status == http.StatusUnprocessableEntity:
			// Wrong model name, malformed request, unknown route.
			return FailureCapability
		case pe.Status == http.StatusPaymentRequired:
			return FailureBudget
		}
		return FailureUnknown
	}
	// Context errors are decided by the dispatcher (timed_out vs budget_exceeded),
	// which knows which of the two it cancelled.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return FailureUnknown
	}
	// Transport failures without a status: DNS, refused connection, TLS, resets.
	// §10.1 lists ECONNRESET as transient.
	return FailureTransient
}

// redactString removes a credential from a string on its way into an error.
//
// It is a string helper and deliberately not an error wrapper: an earlier version
// rebuilt the error with `errors.New`, which erased its type, so `errors.As` in
// classify could no longer find the `*providerError` and every status — 401, 402,
// 404 — fell through to the default `transient` bucket. That would have retried a
// bad credential forever, which is the exact waste §10 exists to prevent. Redact
// the text; leave the chain intact.
func redactString(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[REDACTED]")
}

// truncate bounds a string to n bytes, appending a marker so a reader knows the
// text was cut rather than genuinely ending there.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[...truncated]"
}
