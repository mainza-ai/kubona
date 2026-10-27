package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsExposeHTTPCounters(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()

	health := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	healthW := httptest.NewRecorder()
	h.ServeHTTP(healthW, health)
	if healthW.Code != http.StatusOK {
		t.Fatalf("health: %d", healthW.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"kubona_http_requests_total",
		`method="GET"`,
		`status="200"`,
		"kubona_http_inflight_requests",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("metrics content-type: %q", ct)
	}
}
