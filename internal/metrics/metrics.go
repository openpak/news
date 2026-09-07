// Package metrics is the minimal +2 listener: request counters in
// Prometheus text format and net/http/pprof on the same port
// (ports.md: every service owns HTTP +0, gRPC +1, metrics/pprof +2).
package metrics

import (
	"net/http"
	"net/http/pprof"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

var (
	mu       sync.Mutex
	counters = map[string]*atomic.Uint64{}
)

func counter(name string) *atomic.Uint64 {
	mu.Lock()
	defer mu.Unlock()
	if c, ok := counters[name]; ok {
		return c
	}
	c := &atomic.Uint64{}
	counters[name] = c
	return c
}

// statusClass folds statuses into classes to bound cardinality.
func statusClass(code int) string {
	switch {
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// Middleware counts requests per handler name.
func Middleware(handler string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		counter("news_http_requests_total{handler=\"" + handler + "\",status=\"" + statusClass(sw.code) + "\"}").Add(1)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// ServeHTTP renders /metrics (Prometheus text format).
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	names := make([]string, 0, len(counters))
	for name := range counters {
		names = append(names, name)
	}
	snapshot := make(map[string]uint64, len(names))
	for _, name := range names {
		snapshot[name] = counters[name].Load()
	}
	mu.Unlock()

	sort.Strings(names)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, name := range names {
		w.Write([]byte(name + " " + strconv.FormatUint(snapshot[name], 10) + "\n"))
	}
}

// Handler is the metrics/pprof mux (+2).
func Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", ServeHTTP)
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}
