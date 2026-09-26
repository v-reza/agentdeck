// Package metrics is the Prometheus surface of ARCHITECTURE §14.2.
//
// Why hand-rolled instead of prometheus/client_golang: P1 fixes the stack at
// "nol framework web, nol dependency yang bisa rusak saat major version naik",
// and N14 caps the binary at 30 MB. The client library pulls six modules for
// what the exposition format — a stable, line-oriented spec — does not need.
// What is traded away: no histogram quantiles, no exemplars, no process
// collectors. §14.2 asks for counters, gauges and one histogram, all of which
// are arithmetic this package does itself.
//
// Concurrency: every metric is guarded by one mutex. Scrapes are rare and the
// write path is a map increment, so a sharded registry would buy nothing but
// bugs. The HTTP path is recorded after the response, so it is off the critical
// path of the request it measures.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// histogramBuckets are the §14.2 latency buckets, in seconds. They are the
// spec's, not a guess: N1 asks for p95 ≤ 150 ms on reads, which is only
// observable if the 0.1 and 0.25 boundaries exist.
var histogramBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5}

// kind is the Prometheus type of a series. Only the three §14.2 needs.
type kind string

const (
	counter   kind = "counter"
	gauge     kind = "gauge"
	histogram kind = "histogram"
)

// series is one declared metric: its type, help text, and whether anything in
// the running binary actually writes to it.
//
// `produced` is the honest part. §14.2 defines thirteen metrics; some belong to
// modules that do not exist yet (SSE) or to a call path not instrumented in
// this phase (per-query SQL timing). They are still declared — a dashboard that
// references an absent metric is a broken dashboard — but they are declared as
// unwritten, so "0" is not mistaken for "healthy and quiet".
type series struct {
	name     string
	kind     kind
	help     string
	labels   []string
	produced bool

	mu       sync.Mutex
	counters map[string]float64
	gauges   map[string]float64
	buckets  map[string][]uint64
	sums     map[string]float64
	observes map[string]uint64
}

// Registry holds every declared series and renders them for a scrape.
type Registry struct {
	mu     sync.RWMutex
	series map[string]*series
	order  []string
}

func newSeries(name string, k kind, help string, labels []string, produced bool) *series {
	return &series{
		name: name, kind: k, help: help, labels: labels, produced: produced,
		counters: map[string]float64{}, gauges: map[string]float64{},
		buckets: map[string][]uint64{}, sums: map[string]float64{}, observes: map[string]uint64{},
	}
}

// NewRegistry declares the §14.2 metric set. A metric that nothing produces is
// still registered, so a scrape never silently loses a series an alert depends
// on.
func NewRegistry() *Registry {
	r := &Registry{series: map[string]*series{}}
	add := func(s *series) {
		r.series[s.name] = s
		r.order = append(r.order, s.name)
	}
	// HTTP (produced by Middleware below).
	add(newSeries("agentdeck_http_requests_total", counter,
		"Total HTTP requests handled, excluding /healthz.", []string{"method", "path", "status"}, true))
	add(newSeries("agentdeck_http_request_duration_seconds", histogram,
		"HTTP request latency in seconds.", []string{"method", "path"}, true))
	// Dispatcher / runtime.
	add(newSeries("agentdeck_tasks_claimed_total", counter,
		"Tasks successfully claimed by the dispatcher.", []string{"board_id", "agent_id"}, false))
	add(newSeries("agentdeck_tasks_status_total", gauge,
		"Tasks per status on a board.", []string{"board_id", "status"}, false))
	add(newSeries("agentdeck_runs_total", counter,
		"Runs reaching a terminal state.", []string{"outcome"}, false))
	add(newSeries("agentdeck_runs_active", gauge,
		"Runs currently running.", []string{"org_id"}, false))
	add(newSeries("agentdeck_dispatcher_loop_duration_ms", gauge,
		"Duration of one dispatcher tick in milliseconds (N19: 2s interval).", []string{"board_id"}, false))
	// Realtime — the module does not exist yet, kept declared on purpose.
	add(newSeries("agentdeck_sse_connections_active", gauge,
		"Active SSE connections (alarm above 1000 per instance).", nil, false))
	add(newSeries("agentdeck_sse_events_sent_total", counter,
		"SSE frames sent to clients.", []string{"event_kind"}, false))
	// Budget.
	add(newSeries("agentdeck_budget_exceeded_total", counter,
		"Times a board's daily budget was exceeded.", []string{"board_id"}, false))
	// Database.
	add(newSeries("agentdeck_db_pool_connections", gauge,
		"pgx pool connections by state.", []string{"state"}, false))
	add(newSeries("agentdeck_db_query_duration_seconds", histogram,
		"Duration of critical SQL statements.", []string{"query_name"}, false))
	add(newSeries("agentdeck_heartbeat_lag_seconds", gauge,
		"Seconds since the last heartbeat of a run (alarm above 300).", []string{"run_id"}, false))
	return r
}

// labelKey encodes label values into a map key. Values are joined with a byte
// that cannot appear in a Prometheus label value, so two different label sets
// cannot collide into one series.
func labelKey(values []string) string {
	return strings.Join(values, "\x1f")
}

func (s *series) check(labels []string) {
	if len(labels) != len(s.labels) {
		panic(fmt.Sprintf("metrics: %s expects %d labels, got %d", s.name, len(s.labels), len(labels)))
	}
}

// Add increments a counter.
func (r *Registry) Add(name string, delta float64, labels ...string) {
	s := r.series[name]
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check(labels)
	s.counters[labelKey(labels)] += delta
}

// Set assigns a gauge.
func (r *Registry) Set(name string, value float64, labels ...string) {
	s := r.series[name]
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check(labels)
	s.gauges[labelKey(labels)] = value
}

// Observe records one histogram sample.
func (r *Registry) Observe(name string, value float64, labels ...string) {
	s := r.series[name]
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check(labels)
	key := labelKey(labels)
	if s.buckets[key] == nil {
		s.buckets[key] = make([]uint64, len(histogramBuckets)+1)
	}
	// Store NON-cumulative: exactly one bucket takes the sample, and WriteTo
	// accumulates on render. Incrementing every bucket the value fits into and
	// then accumulating again would square the counts (observed: a 2-sample
	// histogram rendering as 1,2,3,5,7,9,11,13).
	placed := false
	for i, b := range histogramBuckets {
		if value <= b {
			s.buckets[key][i]++
			placed = true
			break
		}
	}
	if !placed {
		s.buckets[key][len(histogramBuckets)]++ // +Inf
	}
	s.sums[key] += value
	s.observes[key]++
}

// WriteTo renders the registry in the Prometheus text exposition format.
//
// Series with no samples yet are rendered with their HELP/TYPE and no value
// line, which is what an unregistered metric looks like to a scrape — the
// alternative (printing 0) would make "not instrumented" indistinguishable from
// "instrumented and idle".
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	r.mu.RLock()
	names := append([]string(nil), r.order...)
	r.mu.RUnlock()

	var sb strings.Builder
	for _, name := range names {
		s := r.series[name]
		s.mu.Lock()
		sb.WriteString("# HELP " + s.name + " " + escapeHelp(s.help) + "\n")
		sb.WriteString("# TYPE " + s.name + " " + string(s.kind) + "\n")
		switch s.kind {
		case counter:
			for _, k := range sortedKeys(s.counters) {
				sb.WriteString(s.render(k, strconv.FormatFloat(s.counters[k], 'f', -1, 64)))
			}
		case gauge:
			for _, k := range sortedKeys(s.gauges) {
				sb.WriteString(s.render(k, strconv.FormatFloat(s.gauges[k], 'f', -1, 64)))
			}
		case histogram:
			for _, k := range sortedKeys(s.buckets) {
				cumulative := uint64(0)
				for i, b := range histogramBuckets {
					cumulative += s.buckets[k][i]
					sb.WriteString(s.renderSuffix(k, "_bucket", strconv.FormatUint(cumulative, 10),
						"le", strconv.FormatFloat(b, 'f', -1, 64)))
				}
				cumulative += s.buckets[k][len(histogramBuckets)]
				sb.WriteString(s.renderSuffix(k, "_bucket", strconv.FormatUint(cumulative, 10), "le", "+Inf"))
				sb.WriteString(s.renderSuffix(k, "_sum", strconv.FormatFloat(s.sums[k], 'f', -1, 64)))
				sb.WriteString(s.renderSuffix(k, "_count", strconv.FormatUint(s.observes[k], 10)))
			}
		}
		s.mu.Unlock()
	}
	n, err := io.WriteString(w, sb.String())
	return int64(n), err
}

// render writes one sample line for a series with the given extra label pair
// appended (used by histogram buckets).
func (s *series) render(key, value string, extra ...string) string {
	return s.renderSuffix(key, "", value, extra...)
}

func (s *series) renderSuffix(key, suffix, value string, extra ...string) string {
	var sb strings.Builder
	sb.WriteString(s.name)
	sb.WriteString(suffix)
	values := strings.Split(key, "\x1f")
	if len(s.labels) == 0 && key == "" {
		values = nil
	}
	if len(s.labels) > 0 || len(extra) > 0 {
		sb.WriteString("{")
		first := true
		for i, l := range s.labels {
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(l + `="` + escapeLabel(values[i]) + `"`)
		}
		for i := 0; i+1 < len(extra); i += 2 {
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(extra[i] + `="` + escapeLabel(extra[i+1]) + `"`)
		}
		sb.WriteString("}")
	}
	sb.WriteString(" " + value + "\n")
	return sb.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// escapeLabel escapes a label value per the exposition spec: backslash, double
// quote and newline. Without it a path containing a quote would produce a scrape
// the Prometheus parser rejects.
func escapeLabel(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}

// escapeHelp escapes help text: backslash and newline only, since the help line
// is not quoted.
func escapeHelp(v string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	return r.Replace(v)
}
