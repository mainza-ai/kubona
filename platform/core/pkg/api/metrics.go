package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
	gzipOK bool
}

func (w *statusWriter) gzipAccepted() bool { return w.gzipOK }

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
	s.renderProcessMetrics(w)
	s.renderMemoryStoreMetrics(w)
	s.renderOpsMetrics(w)
}

// renderProcessMetrics exposes the runtime and cgroup numbers an operator
// needs to separate a Go heap problem from a native/container problem. The
// Go soft limit (GOMEMLIMIT) is a runtime target, not RSS; both are emitted.
func (s *Server) renderProcessMetrics(w io.Writer) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintln(w, "# HELP kubona_process_heap_bytes Bytes of allocated Go heap objects.")
	fmt.Fprintln(w, "# TYPE kubona_process_heap_bytes gauge")
	fmt.Fprintf(w, "kubona_process_heap_bytes %d\n", m.HeapAlloc)
	fmt.Fprintln(w, "# HELP kubona_process_heap_sys_bytes Bytes of Go heap obtained from the OS.")
	fmt.Fprintln(w, "# TYPE kubona_process_heap_sys_bytes gauge")
	fmt.Fprintf(w, "kubona_process_heap_sys_bytes %d\n", m.HeapSys)
	fmt.Fprintln(w, "# HELP kubona_process_stack_bytes Bytes of goroutine stacks.")
	fmt.Fprintln(w, "# TYPE kubona_process_stack_bytes gauge")
	fmt.Fprintf(w, "kubona_process_stack_bytes %d\n", m.StackInuse)
	fmt.Fprintln(w, "# HELP kubona_process_goroutines Current goroutine count.")
	fmt.Fprintln(w, "# TYPE kubona_process_goroutines gauge")
	fmt.Fprintf(w, "kubona_process_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintln(w, "# HELP kubona_process_gc_cycles_total Completed GC cycles.")
	fmt.Fprintln(w, "# TYPE kubona_process_gc_cycles_total counter")
	fmt.Fprintf(w, "kubona_process_gc_cycles_total %d\n", m.NumGC)
	fmt.Fprintln(w, "# HELP kubona_process_gc_pause_seconds_total Cumulative GC pause time.")
	fmt.Fprintln(w, "# TYPE kubona_process_gc_pause_seconds_total counter")
	fmt.Fprintf(w, "kubona_process_gc_pause_seconds_total %f\n", float64(m.PauseTotalNs)/1e9)
	fmt.Fprintln(w, "# HELP kubona_process_gc_goal_bytes Heap goal for the next GC cycle.")
	fmt.Fprintln(w, "# TYPE kubona_process_gc_goal_bytes gauge")
	fmt.Fprintf(w, "kubona_process_gc_goal_bytes %d\n", m.NextGC)
	if s.ledger != nil {
		if rss := readBridgeRSS(s.ledger.Path()); rss > 0 {
			fmt.Fprintln(w, "# HELP kubona_bridge_rss_bytes Bridge process RSS (reported file, fresh within 60s).")
			fmt.Fprintln(w, "# TYPE kubona_bridge_rss_bytes gauge")
			fmt.Fprintf(w, "kubona_bridge_rss_bytes %d\n", rss)
		}
	}
	if rss, limit := processMemory(); rss > 0 {
		fmt.Fprintln(w, "# HELP kubona_process_rss_bytes Resident set size (Linux /proc).")
		fmt.Fprintln(w, "# TYPE kubona_process_rss_bytes gauge")
		fmt.Fprintf(w, "kubona_process_rss_bytes %d\n", rss)
		if limit > 0 {
			fmt.Fprintln(w, "# HELP kubona_cgroup_memory_limit_bytes Container memory limit.")
			fmt.Fprintln(w, "# TYPE kubona_cgroup_memory_limit_bytes gauge")
			fmt.Fprintf(w, "kubona_cgroup_memory_limit_bytes %d\n", limit)
			fmt.Fprintln(w, "# HELP kubona_cgroup_memory_bytes Current container memory use.")
			fmt.Fprintln(w, "# TYPE kubona_cgroup_memory_bytes gauge")
			fmt.Fprintf(w, "kubona_cgroup_memory_bytes %d\n", rss)
		}
	}
}

// processMemory reads the kernel's RSS and cgroup limit when available. On
// non-Linux hosts it returns (0, 0) and the gauges are omitted rather than
// fabricated from Go's heap accounting.
func processMemory() (rss, limit int64) {
	if raw, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 2 {
			if pages, perr := strconv.ParseInt(fields[1], 10, 64); perr == nil {
				rss = pages * int64(os.Getpagesize())
			}
		}
	}
	for _, p := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		if b, err := os.ReadFile(p); err == nil {
			raw := strings.TrimSpace(string(b))
			if raw == "" || raw == "max" {
				continue
			}
			if v, perr := strconv.ParseInt(raw, 10, 64); perr == nil && v > 0 {
				limit = v
				break
			}
		}
	}
	return rss, limit
}

// readBridgeRSS reads the bridge's atomic RSS report from the shared state
// directory. A value older than 60s is not emitted: a stale gauge is worse
// than no gauge.
func readBridgeRSS(dbPath string) int64 {
	if dbPath == "" {
		return 0
	}
	path := filepath.Join(filepath.Dir(dbPath), "bridge-rss.json")
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > 60*time.Second {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var payload struct {
		RSSBytes int64 `json:"rss_bytes"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	if payload.RSSBytes < 0 {
		return 0
	}
	return payload.RSSBytes
}

// renderMemoryStoreMetrics exposes the agentic store's cardinality and vector
// footprint so tenant growth is visible before it becomes an OOM.
func (s *Server) renderMemoryStoreMetrics(w io.Writer) {
	if s.memoryStore == nil {
		return
	}
	st := s.memoryStore.Stats()
	fmt.Fprintln(w, "# HELP kubona_memory_items Total retained memory items.")
	fmt.Fprintln(w, "# TYPE kubona_memory_items gauge")
	fmt.Fprintf(w, "kubona_memory_items %d\n", st.Items)
	fmt.Fprintln(w, "# HELP kubona_memory_vectors Stored embeddings.")
	fmt.Fprintln(w, "# TYPE kubona_memory_vectors gauge")
	fmt.Fprintf(w, "kubona_memory_vectors %d\n", st.Vectors)
	fmt.Fprintln(w, "# HELP kubona_memory_retracted Retraction tombstones still resident.")
	fmt.Fprintln(w, "# TYPE kubona_memory_retracted gauge")
	fmt.Fprintf(w, "kubona_memory_retracted %d\n", st.Retracted)
	fmt.Fprintln(w, "# HELP kubona_memory_vector_bytes Bytes held by in-memory embeddings.")
	fmt.Fprintln(w, "# TYPE kubona_memory_vector_bytes gauge")
	fmt.Fprintf(w, "kubona_memory_vector_bytes %d\n", st.VectorBytes)
}

func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, gzipOK: acceptsGzip(r)}
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
