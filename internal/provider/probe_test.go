package provider

// ProbeInference is the first inference call in this codebase, so it gets a test
// that talks to a real HTTP server rather than a fake: the failure it exists to
// prevent (a gateway that answers /models without checking auth but rejects
// inference) is only observable over the wire.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// upstream is a stand-in for the operator's own LLM endpoint. It records what it
// received, which is what turns "the probe authenticated" into an assertion.
type upstream struct {
	srv    *httptest.Server
	status int
	body   string

	mu      sync.Mutex
	method  string
	path    string
	auth    string
	payload map[string]any
	rawBody string
	hits    int
}

func newUpstream(t *testing.T, status int, body string) *upstream {
	t.Helper()
	u := &upstream{status: status, body: body}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)

		u.mu.Lock()
		u.hits++
		u.method = r.Method
		u.path = r.URL.Path
		u.auth = r.Header.Get("Authorization")
		u.payload = decoded
		u.rawBody = string(raw)
		u.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(u.status)
		_, _ = io.WriteString(w, u.body)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) seen() (method, path, auth string, payload map[string]any, hits int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.method, u.path, u.auth, u.payload, u.hits
}

// TestProbeInferencePostsOneTokenToChatCompletions is AC3's mechanism. It pins
// all three things that make this a credential check rather than a liveness
// check: the path is /chat/completions (not /models), max_tokens is 1 (the
// cheapest call that still authenticates), and the credential travels in the
// Authorization header.
func TestProbeInferencePostsOneTokenToChatCompletions(t *testing.T) {
	up := newUpstream(t, http.StatusOK, `{"choices":[{"message":{"content":"pong"}}]}`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ProbeInference(ctx, nil, up.srv.URL, "sk-tes...0000", "gpt-4o-mini"); err != nil {
		t.Fatalf("ProbeInference: %v", err)
	}

	method, path, auth, payload, hits := up.seen()
	if hits != 1 {
		t.Fatalf("upstream saw %d requests, want exactly 1", hits)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %s, want POST", method)
	}
	if !strings.HasSuffix(path, "/chat/completions") {
		t.Fatalf("path = %s, want it to end in /chat/completions", path)
	}
	if auth != "Bearer sk-tes...0000" {
		t.Fatalf("Authorization = %q, want the credential as a bearer token", auth)
	}
	if got := payload["max_tokens"]; got != float64(1) {
		t.Fatalf("max_tokens = %v, want 1 (AC3's minimal inference)", got)
	}
	if got := payload["model"]; got != "gpt-4o-mini" {
		t.Fatalf("model = %v, want the caller-supplied name", got)
	}
}

// TestProbeInferenceRejects401 is the case the whole design exists for. A
// gateway that serves /models to anyone still refuses inference without a good
// key, so a non-2xx must be an error and must not be mistaken for success.
func TestProbeInferenceRejects401(t *testing.T) {
	up := newUpstream(t, http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`)

	err := ProbeInference(context.Background(), nil, up.srv.URL, "sk-wrong", "gpt-4o-mini")
	if err == nil {
		t.Fatal("ProbeInference reported success for a 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error %q does not name the status that caused it", err)
	}
}

// TestProbeInferenceDoesNotLeakTheCredential guards the error path. The upstream
// body is never read into the error, because a provider is free to echo the
// request back and that echo would carry the key into a log line.
func TestProbeInferenceDoesNotLeakTheCredential(t *testing.T) {
	const apiKey = "sk-secr...wxyz"
	// An upstream that echoes the credential in its error body — the exact shape
	// that would leak if the body were copied into the message.
	up := newUpstream(t, http.StatusBadRequest, `{"error":"bad request with key `+apiKey+`"}`)

	err := ProbeInference(context.Background(), nil, up.srv.URL, apiKey, "gpt-4o-mini")
	if err == nil {
		t.Fatal("ProbeInference reported success for a 400")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("error message leaked the credential: %q", err)
	}
}

// TestProbeInferenceExplainsALoopbackDialFailure pins the hint that turns the
// most confusing failure an operator can hit into an actionable one.
//
// The scenario is measured, not hypothetical: from inside the api container,
// `localhost:20128` is the container itself and refuses the connection, while
// the same 9Router answers 200 on `host.docker.internal:20128`. Without the hint
// the operator reads "connection refused" about a service they can reach from
// their own shell, and concludes the product is broken.
func TestProbeInferenceExplainsALoopbackDialFailure(t *testing.T) {
	// A closed port on loopback: the address passes the guard (127.0.0.1 is
	// allowlisted) and the dial fails.
	up := newUpstream(t, http.StatusOK, `{}`)
	deadURL := up.srv.URL
	up.srv.Close()

	err := ProbeInference(context.Background(), nil, deadURL, "sk-tes...0000", "m")
	if err == nil {
		t.Fatal("ProbeInference reported success against a closed server")
	}
	if !strings.Contains(err.Error(), "loopback address") {
		t.Fatalf("loopback dial failure did not explain the container case: %v", err)
	}
	// The hint must not name a host the guard would refuse. `http` outside the
	// two loopback spellings is rejected (DECISIONS 6A.F), so suggesting
	// host.docker.internal would send the operator to a dead end.
	if strings.Contains(err.Error(), "host.docker.internal") {
		t.Fatalf("the hint names a host the SSRF guard rejects: %v", err)
	}
}

// TestProbeInferenceDoesNotHintOnAPublicHost is the other half, and it is what
// keeps the hint honest. A public host that refuses the connection is a plain
// outage; mentioning containers there would send the operator looking in the
// wrong place.
func TestProbeInferenceDoesNotHintOnAPublicHost(t *testing.T) {
	// example.com resolves publicly and the port is closed for our purposes;
	// what matters is that the host is not loopback.
	err := ProbeInference(context.Background(), nil, "https://this-host-does-not-exist.invalid/v1", "sk-tes...0000", "m")
	if err == nil {
		t.Fatal("ProbeInference reported success against an unresolvable host")
	}
	if strings.Contains(err.Error(), "host.docker.internal") {
		t.Fatalf("a non-loopback failure carried the container hint: %v", err)
	}
}

// TestProbeInferenceRejectsUnreachableHost keeps the SSRF guard in front of the
// probe. A private address must be refused before anything is dialed, and the
// upstream must see zero hits — the assertion that separates "refused" from
// "dialed and failed".
func TestProbeInferenceRejectsUnreachableHost(t *testing.T) {
	up := newUpstream(t, http.StatusOK, `{}`)

	// 169.254.169.254 is the cloud metadata endpoint; the guard rejects it.
	err := ProbeInference(context.Background(), nil, "http://169.254.169.254/v1", "sk-tes...0000", "m")
	if err == nil {
		t.Fatal("ProbeInference accepted a link-local address")
	}
	if _, _, _, _, hits := up.seen(); hits != 0 {
		t.Fatal("the guard ran after the request, not before it")
	}
}

// TestProbeInferenceClassifiesUpstreamFailure pins the distinction the caller
// cannot make from a status code alone: "the credential was refused" is a
// different fact from "the upstream could not answer".
//
// It matters because a gateway that fronts many upstreams answers 401 for a
// model whose upstream is down while the credential it was handed is perfectly
// good. Reading 401 as "bad key" would report a healthy credential as rejected,
// and reading 502 as "bad key" is the bug this whole change exists to fix.
func TestProbeInferenceClassifiesUpstreamFailure(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrCredentialRejected},
		{http.StatusForbidden, ErrCredentialRejected},
		{http.StatusBadRequest, ErrUpstreamError},
		{http.StatusPaymentRequired, ErrUpstreamError},
		{http.StatusTooManyRequests, ErrUpstreamError},
		{http.StatusInternalServerError, ErrUpstreamError},
		{http.StatusBadGateway, ErrUpstreamError},
		{http.StatusServiceUnavailable, ErrUpstreamError},
	}

	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			up := newUpstream(t, tc.status, `{"error":"nope"}`)

			err := ProbeInference(context.Background(), nil, up.srv.URL, "sk-tes...0000", "m")
			if err == nil {
				t.Fatalf("ProbeInference reported success for HTTP %d", tc.status)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("HTTP %d classified as %v, want %v", tc.status, err, tc.want)
			}
			// The status has to survive into the message: it is what an
			// operator reads to tell "my key is wrong" from "my gateway is
			// broken", and it is the only part of the upstream answer that is
			// safe to echo.
			if !strings.Contains(err.Error(), strconv.Itoa(tc.status)) {
				t.Fatalf("error %q does not name HTTP %d", err, tc.status)
			}
		})
	}
}

// TestProbeInferenceUnreachableIsNotACredentialFailure is the other half: a
// host that answers nothing at all must not be reported as a rejected
// credential. Otherwise a provider whose endpoint is merely down gets its key
// blamed, and the operator rotates a key that was never the problem.
func TestProbeInferenceUnreachableIsNotACredentialFailure(t *testing.T) {
	// A server that was up and is now closed: the address is valid and the
	// dial fails, which is the shape of an upstream that is down.
	up := newUpstream(t, http.StatusOK, `{}`)
	deadURL := up.srv.URL
	up.srv.Close()

	err := ProbeInference(context.Background(), nil, deadURL, "sk-tes...0000", "m")
	if err == nil {
		t.Fatal("ProbeInference reported success against a closed server")
	}
	if errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("an unreachable host was reported as a rejected credential: %v", err)
	}
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("error = %v, want it to match ErrUnreachable", err)
	}
}
