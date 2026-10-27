package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/tracing"
)

type httpMetricKey struct {
	Method string
	Status int
}

// httpMetrics is a small, dependency-free Prometheus text exposition. Labels
// stay low-cardinality (method + status), never paths or tenants.
type httpMetrics struct {
	mu       sync.Mutex
	counts   map[httpMetricKey]int64
	durSum   map[httpMetricKey]float64
	durCnt   map[httpMetricKey]int64
	inflight int64
}

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{
		counts: map[httpMetricKey]int64{},
		durSum: map[httpMetricKey]float64{},
		durCnt: map[httpMetricKey]int64{},
	}
}

func (m *httpMetrics) begin() { m.mu.Lock(); m.inflight++; m.mu.Unlock() }
func (m *httpMetrics) end()   { m.mu.Lock(); m.inflight--; m.mu.Unlock() }

func (m *httpMetrics) observe(method string, status int, d time.Duration) {
	key := httpMetricKey{Method: method, Status: status}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key]++
	m.durSum[key] += d.Seconds()
	m.durCnt[key]++
}

func (m *httpMetrics) render(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]httpMetricKey, 0, len(m.counts))
	for k := range m.counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})
	fmt.Fprintln(w, "# HELP kubona_http_requests_total HTTP requests by method and status.")
	fmt.Fprintln(w, "# TYPE kubona_http_requests_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "kubona_http_requests_total{method=%q,status=%q} %d\n",
			k.Method, fmt.Sprint(k.Status), m.counts[k])
	}
	fmt.Fprintln(w, "# HELP kubona_http_request_duration_seconds HTTP request duration.")
	fmt.Fprintln(w, "# TYPE kubona_http_request_duration_seconds summary")
	for _, k := range keys {
		fmt.Fprintf(w, "kubona_http_request_duration_seconds_sum{method=%q,status=%q} %f\n",
			k.Method, fmt.Sprint(k.Status), m.durSum[k])
		fmt.Fprintf(w, "kubona_http_request_duration_seconds_count{method=%q,status=%q} %d\n",
			k.Method, fmt.Sprint(k.Status), m.durCnt[k])
	}
	fmt.Fprintln(w, "# HELP kubona_http_inflight_requests Requests currently executing.")
	fmt.Fprintln(w, "# TYPE kubona_http_inflight_requests gauge")
	fmt.Fprintf(w, "kubona_http_inflight_requests %d\n", m.inflight)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Flush preserves SSE streaming through the instrumentation wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.httpMetrics.render(w)
}

func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		s.httpMetrics.begin()
		start := time.Now()
		defer func() {
			s.httpMetrics.end()
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			dur := time.Since(start)
			s.httpMetrics.observe(r.Method, status, dur)
			if span, ok := tracing.FromContext(r.Context()); ok {
				level := slog.LevelDebug
				if status >= 500 {
					level = slog.LevelError
				} else if status >= 400 {
					level = slog.LevelWarn
				}
				slog.Log(r.Context(), level, "http_request",
					"method", r.Method, "path", r.URL.Path, "status", status,
					"duration_ms", dur.Milliseconds(), "trace_id", span.TraceID,
					"span_id", span.SpanID, "parent_span_id", span.ParentSpanID)
			}
		}()
		next.ServeHTTP(sw, r)
	})
}
