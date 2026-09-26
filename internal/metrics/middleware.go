package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Middleware records every request that reaches the router, except /healthz.
//
// Why /healthz is excluded: it is the container liveness probe, hit every few
// seconds forever, and counting it buries the signal §14.5 alerts on
// (`rate(agentdeck_http_requests_total{status=~"5.."})`). §14.2 says "selain
// /healthz" for the same reason.
//
// Why the path label comes from r.Pattern and not r.URL.Path: r.Pattern is the
// route the mux matched, so every `/api/v1/tasks/{id}` collapses to one label
// value. Labelling by the raw URL would mint a new series per task id — the
// cardinality explosion that takes a Prometheus instance down. r.Pattern is
// empty when no route matched (a 404), so those are bucketed explicitly rather
// than being recorded as one series per bogus URL a scanner tries.
func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthz" {
			next.ServeHTTP(w, req)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)

		path := routePath(req.Pattern)
		elapsed := time.Since(start).Seconds()
		r.Add("agentdeck_http_requests_total", 1, req.Method, path, strconv.Itoa(rec.status))
		r.Observe("agentdeck_http_request_duration_seconds", elapsed, req.Method, path)
	})
}

// statusRecorder captures the status code on the way out.
//
// The 200 default matters: a handler that writes a body without calling
// WriteHeader has answered 200, and net/http never tells the caller that
// explicitly. Guessing 0 would report a successful request as a server error.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	return s.ResponseWriter.Write(b)
}

// routePath turns the mux's matched pattern into a label value.
//
// Go reports r.Pattern as the full registered pattern INCLUDING the method —
// "GET /api/v1/tasks/{id}" — while §14.2's `path` label means the path. Leaving
// the method in would produce a label no dashboard query matches, and it would
// also duplicate the `method` label. The method is dropped, and an empty
// pattern (no route matched, i.e. a 404) becomes "unmatched" rather than one
// series per URL a scanner probes.
func routePath(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}
