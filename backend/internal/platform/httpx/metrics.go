package httpx

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics is a minimal Prometheus-compatible request counter and latency histogram.
// It avoids pulling the full client library for a handful of series.
type Metrics struct {
	mu       sync.Mutex
	requests map[string]uint64
	hist     map[string]*histogram
	counters map[string]uint64
}

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type histogram struct {
	counts []uint64
	sum    float64
	count  uint64
}

func NewMetrics() *Metrics {
	return &Metrics{requests: map[string]uint64{}, hist: map[string]*histogram{}, counters: map[string]uint64{}}
}

func (m *Metrics) Observe(method, route string, status int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf(`method=%q,route=%q,status="%d"`, method, route, status)
	m.requests[key]++
	hkey := fmt.Sprintf(`method=%q,route=%q`, method, route)
	h, ok := m.hist[hkey]
	if !ok {
		h = &histogram{counts: make([]uint64, len(latencyBuckets))}
		m.hist[hkey] = h
	}
	s := d.Seconds()
	for i, b := range latencyBuckets {
		if s <= b {
			h.counts[i]++
		}
	}
	h.sum += s
	h.count++
}

// Inc increments a named business counter such as payments_succeeded_total.
func (m *Metrics) Inc(name string, labels string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name+"{"+labels+"}"]++
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("# TYPE http_requests_total counter\n")
	for _, k := range sortedKeys(m.requests) {
		fmt.Fprintf(&b, "http_requests_total{%s} %d\n", k, m.requests[k])
	}
	b.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for _, k := range sortedKeys(m.hist) {
		h := m.hist[k]
		for i, le := range latencyBuckets {
			fmt.Fprintf(&b, "http_request_duration_seconds_bucket{%s,le=\"%g\"} %d\n", k, le, h.counts[i])
		}
		fmt.Fprintf(&b, "http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", k, h.count)
		fmt.Fprintf(&b, "http_request_duration_seconds_sum{%s} %g\n", k, h.sum)
		fmt.Fprintf(&b, "http_request_duration_seconds_count{%s} %d\n", k, h.count)
	}
	for _, k := range sortedKeys(m.counters) {
		fmt.Fprintf(&b, "%s %d\n", k, m.counters[k])
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(b.String()))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
