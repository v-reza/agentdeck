package main

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"agentdeck/internal/metrics"
)

// Health and metrics surface of ARCHITECTURE §6.2.1.
//
// /livez is deliberately dumber than /readyz: liveness answers "is this process
// alive", so it must NOT check the database. A liveness probe that fails when
// Postgres is down gets the container restarted — which does not fix Postgres,
// and turns a dependency outage into a restart loop. /readyz is where the DB
// check belongs, and it already has one.
//
// /metrics is Admin-only per §6.2.1, but it is scraped by Prometheus, which
// cannot carry a session cookie. The contract's answer is "Basic Auth / Int":
// credentials from the environment, compared in constant time. When no
// credentials are configured the endpoint answers 404 rather than 200 — an
// unauthenticated metrics endpoint exposes every board id and org id in the
// label sets, which is tenant topology, so "closed by default" is the only
// safe default for a self-hosted install nobody hardened.
func registerMetricsRoutes(mux *http.ServeMux, reg *metrics.Registry, auth string) {
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})

	user, pass, configured := splitBasicAuth(auth)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			http.NotFound(w, r)
			return
		}
		gotUser, gotPass, ok := r.BasicAuth()
		if !ok || !constantTimeEqual(gotUser, user) || !constantTimeEqual(gotPass, pass) {
			// The realm is what makes a browser prompt; a scraper just reads 401.
			w.Header().Set("WWW-Authenticate", `Basic realm="agentdeck metrics"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if _, err := reg.WriteTo(w); err != nil {
			// The status is already committed by the first byte of the body;
			// there is nothing to change, and logging here would recurse into
			// the log pipeline. A partial scrape is self-evident to Prometheus.
			return
		}
	})
}

// splitBasicAuth parses "user:password". Anything that would leave the endpoint
// reachable with a credential an attacker can guess is treated as unconfigured:
// no colon at all, an empty string, or an empty user with an empty password
// (":" — the value that looks set but authenticates as "" / ""). The failure
// mode is always a closed endpoint, never an endpoint that accepts nothing.
func splitBasicAuth(v string) (user, pass string, ok bool) {
	i := strings.IndexByte(v, ':')
	if i < 0 {
		return "", "", false
	}
	user, pass = v[:i], v[i+1:]
	if user == "" && pass == "" {
		return "", "", false
	}
	return user, pass, true
}

// constantTimeEqual compares without leaking length or content through timing.
// `subtle` returns 0 for different lengths, so the length check is folded in
// rather than short-circuiting before the comparison.
//
// Known gap: no test enforces the constant-time property. Swapping this for `==`
// keeps every test green — a timing difference is not observable from a unit
// test, and mutation testing confirms it. The protection is real but unguarded;
// if this line is ever "simplified", nothing will fail.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
