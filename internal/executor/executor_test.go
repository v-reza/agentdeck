package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePricer returns fixed numbers so the test asserts plumbing, not pricing.
type fakePricer struct{}

func (fakePricer) Price(string, int64, int64, int64) (int64, int, string, string) {
	return 4242, 7, "catalog", "test-model"
}

func collect(t *testing.T) (*[]Step, func(Step) error) {
	t.Helper()
	steps := []Step{}
	return &steps, func(s Step) error {
		steps = append(steps, s)
		return nil
	}
}

func agentFor(url string) Agent {
	return Agent{
		Name: "Worker", Provider: "openai_compatible", Model: "test-model",
		BaseURL: url, APIKey: "sk-secret-value-123",
		Skills: []Skill{{Slug: "triage", Name: "Triage", BodyMD: "Sort the work by urgency."}},
	}
}

// TestExecuteSendsSkillBodyAndReadsUsage is the load-bearing test: the prompt the
// model receives must contain the skill body, and the usage the provider reports
// must reach the step with a price attached.
func TestExecuteSendsSkillBodyAndReadsUsage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-secret-value-123" {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}],
			"usage":{"prompt_tokens":100,"completion_tokens":20,
			"prompt_tokens_details":{"cached_tokens":30},
			"completion_tokens_details":{"reasoning_tokens":5}}}`))
	}))
	defer srv.Close()

	steps, emit := collect(t)
	res := Execute(context.Background(), agentFor(srv.URL), Task{ID: "t1", Title: "Fix it", Body: "Urgent."},
		fakePricer{}, emit)

	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.Output != "done" {
		t.Errorf("output = %q", res.Output)
	}

	msgs := gotBody["messages"].([]any)
	system := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(system, "Sort the work by urgency.") {
		t.Errorf("system prompt missing the skill body: %q", system)
	}
	if !strings.Contains(system, "Worker") {
		t.Errorf("system prompt missing the agent name: %q", system)
	}
	user := msgs[1].(map[string]any)["content"].(string)
	if !strings.Contains(user, "Fix it") || !strings.Contains(user, "Urgent.") {
		t.Errorf("user message missing the task: %q", user)
	}

	if len(*steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(*steps))
	}
	s := (*steps)[0]
	if s.TokensIn != 100 || s.TokensOut != 20 {
		t.Errorf("tokens = %d/%d, want 100/20", s.TokensIn, s.TokensOut)
	}
	// Cached and reasoning tokens are nested by OpenAI-compatible servers; reading
	// only the flat fields silently prices cache reads as full-price input.
	if s.CacheRead != 30 || s.Reasoning != 5 {
		t.Errorf("cache/reasoning = %d/%d, want 30/5", s.CacheRead, s.Reasoning)
	}
	if s.CostMicros != 4242 || s.PriceVersion != 7 || s.PriceSource != "catalog" {
		t.Errorf("price plumbing = %d/%d/%q", s.CostMicros, s.PriceVersion, s.PriceSource)
	}
	if s.Kind != "llm" || s.Status != "succeeded" || s.Seq != 1 {
		t.Errorf("step = %+v", s)
	}
}

// TestExecuteClassifiesFailure pins the retry taxonomy decisions. A 401 retried
// blindly spends money to learn the same thing again; a 429 is exactly the case
// retries exist for.
func TestExecuteClassifiesFailure(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusTooManyRequests, FailureTransient},
		{http.StatusInternalServerError, FailureTransient},
		{http.StatusServiceUnavailable, FailureTransient},
		{http.StatusUnauthorized, FailureCapability},
		{http.StatusForbidden, FailureCapability},
		{http.StatusNotFound, FailureCapability},
		{http.StatusPaymentRequired, FailureBudget},
		{http.StatusTeapot, FailureUnknown},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
		}))
		steps, emit := collect(t)
		res := Execute(context.Background(), agentFor(srv.URL), Task{Title: "t"}, fakePricer{}, emit)
		srv.Close()

		if res.FailureKind != tc.want {
			t.Errorf("HTTP %d -> %q, want %q", tc.status, res.FailureKind, tc.want)
		}
		if res.Error == "" {
			t.Errorf("HTTP %d reported no error", tc.status)
		}
		// A failed call still needs its step recorded: the tokens were spent.
		if len(*steps) != 1 || (*steps)[0].Status != "failed" {
			t.Errorf("HTTP %d: failed call produced no failed step: %+v", tc.status, *steps)
		}
	}
}

// TestExecuteNeverLeaksTheKey: a provider that echoes the credential into its
// error body must not get it written into runs.error, which the UI renders.
func TestExecuteNeverLeaksTheKey(t *testing.T) {
	const key = "sk-secret-value-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// The echo is the attack: the credential comes back inside the payload.
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key ` + key + ` supplied"}}`))
	}))
	defer srv.Close()

	steps, emit := collect(t)
	res := Execute(context.Background(), agentFor(srv.URL), Task{Title: "t"}, fakePricer{}, emit)
	if res.Error == "" {
		t.Fatal("expected an error")
	}
	if strings.Contains(res.Error, key) {
		t.Errorf("API key leaked into the error: %q", res.Error)
	}
	if !strings.Contains(res.Error, "[REDACTED]") {
		t.Errorf("expected a redaction marker, got %q", res.Error)
	}
	// The same rule applies to the stored step payload.
	blob, _ := json.Marshal((*steps)[0].Payload)
	if strings.Contains(string(blob), key) {
		t.Errorf("API key leaked into the step payload")
	}
}

// TestExecuteRefusesToFollowRedirects: the credential rides in a header, and a
// redirect would replay it against a host this run never validated.
func TestExecuteRefusesToFollowRedirects(t *testing.T) {
	var stolen string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stolen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"gotcha"}}]}`))
	}))
	defer elsewhere.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/chat/completions", http.StatusFound)
	}))
	defer srv.Close()

	steps, emit := collect(t)
	_ = steps
	res := Execute(context.Background(), agentFor(srv.URL), Task{Title: "t"}, fakePricer{}, emit)
	if stolen != "" {
		t.Errorf("redirect replayed the credential to another host: %q", stolen)
	}
	if res.Error == "" {
		t.Error("a 302 was treated as success")
	}
	if strings.Contains(res.Output, "gotcha") {
		t.Error("redirect body was used as the completion")
	}
}

// TestExecuteFailsOnUndeliverableTools: promising a tool the runtime cannot call
// makes every run fail as capability, so the mismatch is reported up front
// instead of being sent to the model as a lie.
func TestExecuteFailsOnUndeliverableTools(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	agent := agentFor(srv.URL)
	agent.Tools = []string{"web_search", "run_shell"}
	steps, emit := collect(t)
	res := Execute(context.Background(), agent, Task{Title: "t"}, fakePricer{}, emit)

	if res.FailureKind != FailureCapability {
		t.Errorf("kind = %q, want %q", res.FailureKind, FailureCapability)
	}
	if called {
		t.Error("the provider was called despite an undeliverable tool list")
	}
	if len(*steps) != 0 {
		t.Errorf("no spend should have been recorded, got %+v", *steps)
	}
}

// TestExecuteMissingAddressIsCapability: no endpoint is not a transient fault.
func TestExecuteMissingAddressIsCapability(t *testing.T) {
	agent := agentFor("")
	steps, emit := collect(t)
	res := Execute(context.Background(), agent, Task{Title: "t"}, fakePricer{}, emit)
	if res.FailureKind != FailureCapability {
		t.Errorf("kind = %q, want %q", res.FailureKind, FailureCapability)
	}
	if len(*steps) != 0 {
		t.Errorf("steps = %d, want 0", len(*steps))
	}
}

// TestExecuteNoChoicesIsAnError: a 200 with an empty choices array is not a
// success, and must not be recorded as one.
func TestExecuteNoChoicesIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":0}}`))
	}))
	defer srv.Close()

	steps, emit := collect(t)
	res := Execute(context.Background(), agentFor(srv.URL), Task{Title: "t"}, fakePricer{}, emit)
	if res.Error == "" {
		t.Fatal("empty choices was accepted as success")
	}
	if len(*steps) != 1 || (*steps)[0].Status != "failed" {
		t.Errorf("step not recorded as failed: %+v", *steps)
	}
	// The prompt tokens were still billed by the provider, so they are kept.
	if (*steps)[0].TokensIn != 5 {
		t.Errorf("tokens_in = %d, want 5", (*steps)[0].TokensIn)
	}
}

// TestExecuteStopsWhenAStepCannotBeRecorded: emitting is how spend becomes
// auditable, so a sink failure must abort rather than continue spending.
func TestExecuteStopsWhenAStepCannotBeRecorded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()

	res := Execute(context.Background(), agentFor(srv.URL), Task{Title: "t"}, fakePricer{},
		func(Step) error { return context.DeadlineExceeded })
	if res.Error == "" {
		t.Error("a failed emit was reported as success")
	}
}

// TestBuildPromptNamesEverySkillInOrder: `skills_json` is an ordered selection and
// order is its only priority signal.
func TestBuildPromptNamesEverySkillInOrder(t *testing.T) {
	agent := agentFor("https://example.test")
	agent.Skills = []Skill{
		{Slug: "a", Name: "Alpha", BodyMD: "first"},
		{Slug: "b", Name: "Beta", BodyMD: "second"},
	}
	prompt, dropped := buildPrompt(agent, Task{})
	if dropped != 0 {
		t.Fatalf("dropped = %d", dropped)
	}
	if strings.Index(prompt, "first") > strings.Index(prompt, "second") {
		t.Errorf("skills rendered out of order:\n%s", prompt)
	}
}
