package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// clock is a manual clock so no test sleeps.
type clock struct{ t time.Time }

func newClock() *clock                   { return &clock{t: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)} }
func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func request(path, token string, ip string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, nil)
	r.RemoteAddr = ip + ":54321"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// US-AD85 AC1: the public endpoints are 10 req/min per IP.
func TestPublicBudgetIsTenPerMinutePerIP(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.7"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.7"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request: got %d, want 429", rec.Code)
	}

	// A different IP is unaffected: the budget is per address.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.8"))
	if rec.Code != http.StatusOK {
		t.Fatalf("other IP: got %d, want 200", rec.Code)
	}
}

// US-AD85 AC2: the 429 carries Retry-After.
func TestTooManyRequestsCarriesRetryAfter(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 10; i++ {
		h.ServeHTTP(httptest.NewRecorder(), request("/api/v1/auth/register", "", "198.51.100.4"))
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/register", "", "198.51.100.4"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	header := rec.Header().Get("Retry-After")
	if header == "" {
		t.Fatal("Retry-After is missing on a 429")
	}
	seconds, err := time.ParseDuration(header + "s")
	if err != nil || seconds <= 0 {
		t.Fatalf("Retry-After %q is not a positive number of seconds", header)
	}
	// 10/min is one token every 6 seconds, so the answer is 6s. The assertion is
	// EXACT on purpose: a duration rounded DOWN to whole seconds yields 5, and a
	// client that waits 5s is refused again — the header would be wrong in the
	// one direction that matters. A range assertion (`<= 7s`) passes for both 5
	// and 6 and catches neither.
	if seconds != 6*time.Second {
		t.Fatalf("Retry-After = %v, want exactly 6s (60s / 10 per min)", seconds)
	}
}

// The budget refills: after the wait the request goes through again.
func TestBudgetRefillsOverTime(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 10; i++ {
		h.ServeHTTP(httptest.NewRecorder(), request("/api/v1/auth/login", "", "192.0.2.1"))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "192.0.2.1"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected to be limited, got %d", rec.Code)
	}

	c.advance(6 * time.Second)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "192.0.2.1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("after one refill interval: got %d, want 200", rec.Code)
	}
}

// US-AD85 AC3: on a public endpoint, changing the token buys nothing — the
// budget is spent against the IP and the token is not read at all.
func TestPublicBudgetCannotBeBypassedWithAnotherToken(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 10; i++ {
		h.ServeHTTP(httptest.NewRecorder(), request("/api/v1/auth/login", "token-a", "203.0.113.99"))
	}

	// Same IP, a brand new token, and no token at all: all three are still 429.
	for _, token := range []string{"token-b", "token-c", ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, request("/api/v1/auth/login", token, "203.0.113.99"))
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("token %q: got %d, want 429 — the public budget must be per-IP only", token, rec.Code)
		}
	}
}

// US-AD85 AC4: authenticated endpoints are limited per identity, not per IP, so
// two operators behind one address do not share a budget.
func TestAuthenticatedBudgetIsPerIdentityNotPerIP(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	// Spend the whole budget for token-a from one address.
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, request("/api/v1/boards", "token-a", "198.51.100.10"))
		if rec.Code != http.StatusOK {
			t.Fatalf("token-a request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/boards", "token-a", "198.51.100.10"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("token-a 101st: got %d, want 429", rec.Code)
	}

	// token-b, SAME address: untouched.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/boards", "token-b", "198.51.100.10"))
	if rec.Code != http.StatusOK {
		t.Fatalf("token-b from the same IP: got %d, want 200 — the budget is per identity", rec.Code)
	}

	// And token-a from a DIFFERENT address is still out: rotating IPs must not
	// buy a fresh budget for the same credential.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/boards", "token-a", "198.51.100.11"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("token-a from a new IP: got %d, want 429", rec.Code)
	}
}

// An authenticated request with no credential is passed through: the auth
// middleware rejects it, and the limiter must not spend anything on it.
func TestRequestWithoutCredentialIsPassedThrough(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 200; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, request("/api/v1/boards", "", "203.0.113.200"))
		if rec.Code != http.StatusOK {
			t.Fatalf("anonymous request %d: got %d — the limiter must not touch it", i+1, rec.Code)
		}
	}
}

// The bucket map must not grow without bound: one entry per distinct IP is an
// allocation the attacker controls.
func TestIdleBucketsAreSwept(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 50; i++ {
		h.ServeHTTP(httptest.NewRecorder(), request("/api/v1/auth/login", "", "203.0.113."+itoa(i)))
	}
	l.mu.Lock()
	before := len(l.buckets)
	l.mu.Unlock()
	if before != 50 {
		t.Fatalf("expected 50 buckets, got %d", before)
	}

	// Past the idle TTL and past the sweep interval, one more request sweeps.
	c.advance(11 * time.Minute)
	h.ServeHTTP(httptest.NewRecorder(), request("/api/v1/auth/login", "", "198.51.100.250"))

	l.mu.Lock()
	after := len(l.buckets)
	l.mu.Unlock()
	if after != 1 {
		t.Fatalf("expected the idle buckets to be swept, %d remain", after)
	}
}

// A token bucket must not let a burst through twice: the 11th back-to-back
// request on a 10-burst budget is refused even with no time passing.
func TestBurstIsNotExceededWithinOneInstant(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})
	allowed := 0
	for i := 0; i < 25; i++ {
		ok, _ := l.Allow(Identity{Key: "ip:test", Public: true})
		if ok {
			allowed++
		}
	}
	if allowed != PublicBudget.Burst {
		t.Fatalf("allowed %d, want exactly the burst %d", allowed, PublicBudget.Burst)
	}
}

// ClientIP must not trust X-Forwarded-For: an attacker who can reset their
// budget by inventing an address does not have a rate limit.
func TestClientIPIgnoresForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.5:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := ClientIP(r); got != "203.0.113.5" {
		t.Fatalf("ClientIP = %q, want the socket address", got)
	}
}

// CredentialKey must never return the token itself.
func TestCredentialKeyIsHashed(t *testing.T) {
	secret := "adk_live_supersecretvalue"
	r := request("/api/v1/boards", secret, "203.0.113.5")
	key := CredentialKey(r)
	if key == "" {
		t.Fatal("CredentialKey returned empty for a Bearer token")
	}
	if key == secret || len(key) != 32 {
		t.Fatalf("CredentialKey = %q; want a hash, never the token", key)
	}
	// Stable for the same credential, different for another.
	if CredentialKey(request("/api/v1/boards", secret, "10.0.0.1")) != key {
		t.Fatal("the same credential produced two different keys")
	}
	if CredentialKey(request("/api/v1/boards", secret+"x", "203.0.113.5")) == key {
		t.Fatal("a different credential produced the same key")
	}
}

// The public budget is matched on the exact path: a future endpoint that merely
// shares a prefix must not inherit it.
func TestPublicPathIsExact(t *testing.T) {
	if !PublicPath("/api/v1/auth/login") {
		t.Fatal("login must be public-budgeted")
	}
	if PublicPath("/api/v1/auth/login-as-admin") {
		t.Fatal("a prefix match would put an unknown endpoint on the public budget")
	}
	if PublicPath("/api/v1/boards") {
		t.Fatal("an authenticated path must not be public-budgeted")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// A zero budget falls back to the contract default, so a caller that overrides
// only one of the two does not have to restate the other.
func TestZeroBudgetFallsBackToTheContractDefault(t *testing.T) {
	l := New(nil, Budget{PerMinute: 5}, Budget{})
	if l.public.PerMinute != 5 {
		t.Fatalf("public = %d/min, want the override 5", l.public.PerMinute)
	}
	if l.public.Burst != 5 {
		t.Fatalf("public burst = %d, want it to follow PerMinute when unset", l.public.Burst)
	}
	if l.auth.PerMinute != AuthBudget.PerMinute || l.auth.Burst != AuthBudget.Burst {
		t.Fatalf("auth = %+v, want the contract default %+v", l.auth, AuthBudget)
	}
}

// A configured budget is the one that applies, at the boundary.
func TestConfiguredBudgetIsEnforced(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{Burst: 3, PerMinute: 3}, Budget{})
	h := l.Middleware(okHandler())

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.31"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.31"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request on a 3/min budget: got %d, want 429", rec.Code)
	}
}

// The refill rate is the contract's rate, not an approximation. With a 10/min
// budget exactly one token accrues per 6s: 5.9s must still be refused and 6s
// must pass. A limiter that refills faster silently hands out more than the
// documented budget; one that refills slower throttles a compliant client.
func TestRefillRateIsExact(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{}, Budget{})

	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow(Identity{Key: "ip:exact", Public: true}); !ok {
			t.Fatalf("request %d inside the burst was refused", i+1)
		}
	}
	if ok, _ := l.Allow(Identity{Key: "ip:exact", Public: true}); ok {
		t.Fatal("the 11th back-to-back request was allowed")
	}

	c.advance(5900 * time.Millisecond)
	if ok, _ := l.Allow(Identity{Key: "ip:exact", Public: true}); ok {
		t.Fatal("a token accrued before the 6s interval elapsed")
	}

	c.advance(100 * time.Millisecond)
	if ok, _ := l.Allow(Identity{Key: "ip:exact", Public: true}); !ok {
		t.Fatal("no token after exactly 6s")
	}
	if ok, _ := l.Allow(Identity{Key: "ip:exact", Public: true}); ok {
		t.Fatal("a second token accrued from the same 6s interval")
	}
}

// Retry-After must round UP, and the contract budget cannot prove that on its
// own: 10/min is one token every 6.000s exactly, so truncating and ceiling agree.
// A budget whose wait has a fractional part separates them — 7/min is one token
// every 8.571s, and a client told "8" comes back 571ms early and is refused
// again. This is the assertion that pins the rounding direction.
func TestRetryAfterRoundsUpWhenTheWaitIsFractional(t *testing.T) {
	c := newClock()
	l := New(c.now, Budget{Burst: 1, PerMinute: 7}, Budget{})
	h := l.Middleware(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.77"))
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("/api/v1/auth/login", "", "203.0.113.77"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", rec.Code)
	}
	// 60/7 = 8.571428... -> 9, not 8.
	if got := rec.Header().Get("Retry-After"); got != "9" {
		t.Fatalf("Retry-After = %q, want \"9\" (ceil of 8.571s)", got)
	}
}
