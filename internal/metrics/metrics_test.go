package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The exposition format is a wire contract with Prometheus: if a line is
// malformed the whole scrape is rejected, so these tests assert the rendered
// text, not just that a map was written.

func scrape(t *testing.T, r *Registry) string {
	t.Helper()
	var sb strings.Builder
	if _, err := r.WriteTo(&sb); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return sb.String()
}

func TestCounterRendersWithLabels(t *testing.T) {
	r := NewRegistry()
	r.Add("agentdeck_http_requests_total", 3, "GET", "/api/v1/tasks/{id}", "200")
	out := scrape(t, r)
	want := `agentdeck_http_requests_total{method="GET",path="/api/v1/tasks/{id}",status="200"} 3`
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestDeclaredButUnproducedSeriesStillRendersHelpAndType(t *testing.T) {
	// §14.2 lists metrics whose modules do not exist yet (SSE). They must still
	// appear, or a dashboard referencing them breaks; but they must have no
	// value line, so "not instrumented" is distinguishable from "zero".
	out := scrape(t, NewRegistry())
	if !strings.Contains(out, "# TYPE agentdeck_sse_connections_active gauge") {
		t.Error("SSE gauge not declared")
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "agentdeck_sse_connections_active") {
			t.Errorf("an unproduced gauge emitted a sample line: %q", line)
		}
	}
}

func TestLabelValuesAreEscaped(t *testing.T) {
	// A path with a quote or a newline would otherwise terminate the label and
	// produce a scrape Prometheus rejects.
	r := NewRegistry()
	r.Add("agentdeck_http_requests_total", 1, "GET", "/weird\"path\nnext", "200")
	out := scrape(t, r)
	if !strings.Contains(out, `path="/weird\"path\nnext"`) {
		t.Errorf("label not escaped:\n%s", out)
	}
	if strings.Count(out, "agentdeck_http_requests_total{") != 1 {
		t.Errorf("escaping produced an extra series:\n%s", out)
	}
}

func TestHistogramIsCumulativeWithSumAndCount(t *testing.T) {
	r := NewRegistry()
	r.Observe("agentdeck_http_request_duration_seconds", 0.02, "GET", "/a")
	r.Observe("agentdeck_http_request_duration_seconds", 0.3, "GET", "/a")
	out := scrape(t, r)
	// 0.02 falls in the 0.05 bucket; 0.3 falls in 0.5. Cumulative means the
	// 0.5 bucket counts both.
	for _, want := range []string{
		`agentdeck_http_request_duration_seconds_bucket{method="GET",path="/a",le="0.05"} 1`,
		`agentdeck_http_request_duration_seconds_bucket{method="GET",path="/a",le="0.5"} 2`,
		`agentdeck_http_request_duration_seconds_bucket{method="GET",path="/a",le="+Inf"} 2`,
		`agentdeck_http_request_duration_seconds_sum{method="GET",path="/a"} 0.32`,
		`agentdeck_http_request_duration_seconds_count{method="GET",path="/a"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUnknownMetricIsIgnoredNotPanicked(t *testing.T) {
	// A typo'd metric name in a future call site must not take the process down.
	r := NewRegistry()
	r.Add("agentdeck_typo_total", 1, "x")
	if strings.Contains(scrape(t, r), "agentdeck_typo_total") {
		t.Error("undeclared metric leaked into the scrape")
	}
}

// newMux builds the shape main.go uses: a mux with wildcard routes, wrapped.
func newMux(r *Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/boards/{id}/tasks", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	return r.Middleware(mux)
}

// TestPathLabelUsesTheRoutePatternNotTheURL is the cardinality test. Labelling
// by the raw URL mints one series per task id, which is how a metrics backend
// falls over. r.Pattern is the route the mux matched, so both ids collapse.
func TestPathLabelUsesTheRoutePatternNotTheURL(t *testing.T) {
	r := NewRegistry()
	h := newMux(r)
	for _, id := range []string{"01AAA", "01BBB", "01CCC"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+id, nil))
	}
	out := scrape(t, r)
	// Only the counter carries one line per route; the histogram repeats the
	// same label set once per bucket, so counting raw occurrences would be
	// measuring the histogram's bucket count, not cardinality.
	want := `agentdeck_http_requests_total{method="GET",path="/api/v1/tasks/{id}",status="200"} 3`
	if !strings.Contains(out, want) {
		t.Errorf("want a single collapsed series %q, got:\n%s", want, out)
	}
	if got := strings.Count(out, "agentdeck_http_requests_total{"); got != 1 {
		t.Errorf("want exactly one counter series for the route, got %d", got)
	}
	if strings.Contains(out, "01AAA") || strings.Contains(out, "01BBB") {
		t.Errorf("a raw id leaked into a label:\n%s", out)
	}
}

func TestUnmatchedPathIsBucketedNotEchoed(t *testing.T) {
	// A scanner hitting /wp-login.php must not create a series per URL.
	r := NewRegistry()
	h := newMux(r)
	for _, p := range []string{"/wp-login.php", "/.env", "/admin"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
	}
	out := scrape(t, r)
	if !strings.Contains(out, `path="unmatched"`) {
		t.Errorf("unmatched requests were not bucketed:\n%s", out)
	}
	if strings.Contains(out, "wp-login") || strings.Contains(out, ".env") {
		t.Errorf("a probe URL leaked into a label:\n%s", out)
	}
}

func TestHealthzIsExcludedFromCounters(t *testing.T) {
	// §14.2 says "selain /healthz": the liveness probe runs every few seconds
	// forever and would bury the signal the §14.5 alerts depend on.
	r := NewRegistry()
	h := newMux(r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if out := scrape(t, r); strings.Contains(out, `path="/healthz"`) {
		t.Errorf("/healthz was counted:\n%s", out)
	}
}

func TestStatusIsRecordedPerResponse(t *testing.T) {
	r := NewRegistry()
	h := newMux(r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/boards/01X/tasks", nil))
	out := scrape(t, r)
	if !strings.Contains(out, `status="404"`) {
		t.Errorf("404 not recorded:\n%s", out)
	}
}

func TestHandlerThatNeverSetsStatusCountsAs200(t *testing.T) {
	// net/http never tells the caller "200" explicitly; guessing 0 would report
	// every successful request as a server error.
	r := NewRegistry()
	h := newMux(r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/01Z", nil))
	if !strings.Contains(scrape(t, r), `status="200"`) {
		t.Errorf("implicit 200 not recorded:\n%s", scrape(t, r))
	}
}

func TestRoutePathDropsTheMethodPrefix(t *testing.T) {
	// Go reports r.Pattern as "GET /api/v1/tasks/{id}". The §14.2 `path` label
	// means the path; keeping the method would duplicate the `method` label and
	// produce a value no dashboard query matches.
	cases := map[string]string{
		"GET /api/v1/tasks/{id}":   "/api/v1/tasks/{id}",
		"POST /api/v1/boards/{id}": "/api/v1/boards/{id}",
		"":                         "unmatched",
		"/already/a/path":          "/already/a/path",
	}
	for in, want := range cases {
		if got := routePath(in); got != want {
			t.Errorf("routePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRecordedPathNeverCarriesTheMethod(t *testing.T) {
	// Guards the rendering end, so the bug cannot return via a different call
	// site: whatever the mux reports, no label value may start with a method.
	r := NewRegistry()
	h := newMux(r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/01Z", nil))
	out := scrape(t, r)
	if strings.Contains(out, `path="GET `) {
		t.Errorf("method leaked into the path label:\n%s", out)
	}
	if !strings.Contains(out, `path="/api/v1/tasks/{id}"`) {
		t.Errorf("path label not recorded:\n%s", out)
	}
}
