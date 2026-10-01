// Package ratelimit is the in-memory token bucket US-AD85 asks for.
//
// ARCHITECTURE §6.1 and §16 both specify it as "in-memory token bucket per IP
// dan API Key", so there is no external dependency here and nothing to configure:
// one process, one bucket table, no coordination. That also means the limit is
// per replica — a deployment that scales horizontally multiplies the budget. That
// is stated rather than hidden; the contract asks for an in-memory limiter and
// this is exactly that.
//
// The two budgets come from ARCHITECTURE §6.1 ("rate limit standar 100 req/menit,
// 60 req/menit untuk unauthenticated login/register"). The PRD's US-AD85 AC1 says
// 10 req/menit for the public endpoints instead. The PRD is the newer statement of
// intent for THIS endpoint class and the PRD wins on product behaviour, so the
// public budget is 10/min. Flagged in docs/OPEN-ISSUES.md rather than silently
// picked.
package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Budget is one bucket's size and refill rate.
type Budget struct {
	// Burst is how many requests may arrive back to back from a cold start.
	Burst int
	// PerMinute is the sustained rate.
	PerMinute int
}

// The two contract budgets, per ARCHITECTURE §6.1 and US-AD85 AC1.
//
// These are DEFAULTS, not constants: a single-address deployment behind a NAT —
// an office, a CI rig, a self-hosted install with many users — legitimately needs
// a different number, and the alternative to a knob is an operator disabling the
// limiter entirely. The knob is read in cmd/api; nothing here touches the
// environment.
var (
	// PublicBudget guards register/login: the endpoints an attacker with no
	// account can reach, and the only ones where per-IP is the right key
	// (AC3 — changing a token must not buy a fresh budget).
	PublicBudget = Budget{Burst: 10, PerMinute: 10}
	// AuthBudget guards everything authenticated: 100 req/min per identity.
	AuthBudget = Budget{Burst: 100, PerMinute: 100}
)

// Identity is what a budget is spent against.
type Identity struct {
	// Key is the bucket key: an IP for public endpoints, a hashed credential for
	// authenticated ones. Never a raw token — the map outlives the request and a
	// raw session token in a long-lived map is a credential leak waiting for a
	// heap dump.
	Key string
	// Public selects PublicBudget over AuthBudget.
	Public bool
}

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a token bucket per key. Safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
	public  Budget
	auth    Budget
	// lastSweep bounds the map's growth. Without it, one entry per distinct IP
	// is an unbounded allocation an attacker controls — the limiter itself would
	// be the DoS.
	lastSweep time.Time
}

const (
	sweepInterval = 10 * time.Minute
	// A bucket idle for this long has fully refilled, so forgetting it changes
	// no decision.
	idleTTL = 10 * time.Minute
)

// New returns a limiter. `now` is injectable so tests do not sleep; a zero
// budget falls back to the contract default, so a caller that only wants to
// override one of the two does not have to restate the other.
func New(now func() time.Time, public, auth Budget) *Limiter {
	if now == nil {
		now = time.Now
	}
	if public.PerMinute <= 0 {
		public = PublicBudget
	}
	if public.Burst <= 0 {
		public.Burst = public.PerMinute
	}
	if auth.PerMinute <= 0 {
		auth = AuthBudget
	}
	if auth.Burst <= 0 {
		auth.Burst = auth.PerMinute
	}
	return &Limiter{
		buckets:   make(map[string]*bucket),
		now:       now,
		public:    public,
		auth:      auth,
		lastSweep: now(),
	}
}

// Allow spends one token. It returns whether the request may proceed, and how
// long the caller should wait — the value for `Retry-After`, rounded UP so a
// client that obeys it exactly is not rejected again.
func (l *Limiter) Allow(id Identity) (bool, time.Duration) {
	budget := l.auth
	if id.Public {
		budget = l.public
	}
	rate := float64(budget.PerMinute) / 60.0 // tokens per second

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	b, ok := l.buckets[id.Key]
	if !ok {
		b = &bucket{tokens: float64(budget.Burst), last: now}
		l.buckets[id.Key] = b
	}

	// Refill for elapsed time, capped at the burst.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(float64(budget.Burst), b.tokens+elapsed*rate)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Time until one token is available.
	missing := 1 - b.tokens
	wait := time.Duration(math.Ceil(missing/rate*1000)) * time.Millisecond
	return false, wait
}

// sweepLocked drops buckets that have refilled to full, which is every bucket
// idle for longer than the burst divided by the rate.
func (l *Limiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for key, b := range l.buckets {
		if now.Sub(b.last) > idleTTL {
			delete(l.buckets, key)
		}
	}
}

// ClientIP is the address the server saw.
//
// RemoteAddr, not X-Forwarded-For: that header is caller-supplied, and a rate
// limit an attacker can reset by inventing an address is not a rate limit. Behind
// a trusted proxy this needs a configured allowlist to be correct — the same
// tradeoff `sessionMeta` in cmd/api already documents.
func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// CredentialKey is the bucket key for an authenticated request: a hash of the
// credential, or "" when the request carries none.
//
// Hashing rather than storing the token is deliberate. The bucket map lives for
// the process's lifetime, and a raw session token or API key sitting in it is a
// credential in memory longer than it needs to be.
func CredentialKey(r *http.Request) string {
	token := ""
	if header := r.Header.Get("Authorization"); len(header) > 7 && header[:7] == "Bearer " {
		token = header[7:]
	} else if cookie, err := r.Cookie("agentdeck_session"); err == nil {
		token = cookie.Value
	}
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

// PublicPath reports whether a path is one of the unauthenticated endpoints that
// get the tighter per-IP budget.
//
// Matched on the exact path, not a prefix: a prefix match would put any future
// `/api/v1/auth/login-as-admin` under the public budget, which is the wrong
// direction to fail.
func PublicPath(path string) bool {
	switch path {
	case "/api/v1/auth/register", "/api/v1/auth/login",
		"/api/v1/auth/password/reset", "/api/v1/auth/password/reset/confirm":
		return true
	}
	return false
}

// Middleware spends one token per request and answers 429 with `Retry-After`
// (AC2) when the budget is gone.
//
// AC4 (per-identity, not just per-IP): a request carrying a credential is keyed
// by that credential, so two operators behind one NAT do not share a budget, and
// one operator cannot escape theirs by rotating IPs.
//
// AC3 (per-IP cannot be bypassed by changing the token): the public endpoints are
// keyed by IP and ONLY by IP — presenting a different or no token changes
// nothing, because the token is not read for those paths at all.
//
// A request with neither a credential nor a public path is passed through
// untouched: it is about to be rejected by the auth middleware, and spending
// budget on it here would let an unauthenticated flood exhaust the budget of
// whatever identity it claims.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := Identity{Public: PublicPath(r.URL.Path)}
		if id.Public {
			id.Key = "ip:" + ClientIP(r)
		} else if key := CredentialKey(r); key != "" {
			id.Key = "cred:" + key
		} else {
			next.ServeHTTP(w, r)
			return
		}

		ok, retryAfter := l.Allow(id)
		if !ok {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
