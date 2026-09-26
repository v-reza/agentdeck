package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/metrics"
)

// /metrics is the one endpoint whose failure mode is silent: an open one serves
// every board id and org id in its label sets to anyone who can reach the port,
// which is tenant topology. These tests pin the closed-by-default behaviour.

func metricsServer(auth string) http.Handler {
	mux := http.NewServeMux()
	registerMetricsRoutes(mux, metrics.NewRegistry(), auth)
	return mux
}

func get(t *testing.T, h http.Handler, path string, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLivezAnswersWithoutTouchingTheDatabase(t *testing.T) {
	// Liveness must not depend on Postgres: a probe that fails when the DB is
	// down gets the container restarted, which does not fix Postgres and turns
	// a dependency outage into a restart loop. The handler takes no pool at all,
	// so this test is really asserting the signature.
	rec := get(t, metricsServer(""), "/livez", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("body = %q, want %q", body, "ok\n")
	}
}

func TestMetricsIsClosedWhenUnconfigured(t *testing.T) {
	// 404, not 401: an install nobody hardened should not advertise that a
	// metrics endpoint exists at all.
	for _, auth := range []string{"", "no-colon-here", ":"} {
		rec := get(t, metricsServer(auth), "/metrics", "", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("auth=%q: status = %d, want 404", auth, rec.Code)
		}
	}
}

func TestMetricsRejectsMissingAndWrongCredentials(t *testing.T) {
	h := metricsServer("scraper:s3cret")
	rec := get(t, h, "/metrics", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no credentials: status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic ") {
		t.Errorf("WWW-Authenticate = %q, want a Basic challenge", got)
	}
	for _, c := range [][2]string{
		{"scraper", "wrong"},
		{"wrong", "s3cret"},
		{"scraper", "s3cret "}, // a trailing space is a different password
		{"SCRAPER", "s3cret"},  // and the user is case-sensitive
	} {
		if rec := get(t, h, "/metrics", c[0], c[1]); rec.Code != http.StatusUnauthorized {
			t.Errorf("credentials %v: status = %d, want 401", c, rec.Code)
		}
	}
}

func TestMetricsServesTheExpositionFormatWhenAuthenticated(t *testing.T) {
	rec := get(t, metricsServer("scraper:s3cret"), "/metrics", "scraper", "s3cret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("Content-Type = %q, want the Prometheus text format", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "# TYPE agentdeck_http_requests_total counter") {
		t.Errorf("scrape missing the HTTP counter:\n%s", body[:min(len(body), 200)])
	}
}

func TestMetricsPasswordMayContainColons(t *testing.T) {
	// splitBasicAuth cuts on the FIRST colon, so a password with colons works.
	rec := get(t, metricsServer("user:a:b:c"), "/metrics", "user", "a:b:c")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a colon-bearing password", rec.Code)
	}
}

func TestSplitBasicAuth(t *testing.T) {
	cases := []struct {
		in       string
		user, pw string
		ok       bool
	}{
		{"u:p", "u", "p", true},
		{"u:", "u", "", true}, // a named user with an empty password is configured
		{":p", "", "p", true}, // so is an empty user with a real password
		{":", "", "", false},  // but "" / "" is a credential anyone can guess
		{"", "", "", false},
		{"nocolon", "", "", false},
	}
	for _, c := range cases {
		u, p, ok := splitBasicAuth(c.in)
		if u != c.user || p != c.pw || ok != c.ok {
			t.Errorf("splitBasicAuth(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, u, p, ok, c.user, c.pw, c.ok)
		}
	}
}
