package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTraceContextContinuation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health: %d", w.Code)
	}
	got := w.Header().Get("traceparent")
	if !strings.HasPrefix(got, "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Fatalf("must continue inbound trace id, got %q", got)
	}
	if strings.Contains(got, "00f067aa0ba902b7") {
		t.Fatalf("response span must be new, got %q", got)
	}
	if w.Header().Get("X-Trace-Id") != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-Id mismatch: %q", w.Header().Get("X-Trace-Id"))
	}
}

func TestHTTPTraceContextRejectsMalformed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("traceparent", "not-a-trace")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	got := w.Header().Get("traceparent")
	if len(got) != 55 || !strings.HasPrefix(got, "00-") {
		t.Fatalf("malformed input must mint a valid trace, got %q", got)
	}
	if w.Header().Get("X-Trace-Id") == "" {
		t.Fatal("trace id must be exposed for log correlation")
	}
}
