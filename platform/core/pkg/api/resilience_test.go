package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecoveryReturnsTyped500(t *testing.T) {
	h := requestIDAndRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("drill")
	}))
	req := httptest.NewRequest(http.MethodGet, "/drill", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic must return 500, got %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("panic response must carry a request id")
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["code"] != "internal" {
		t.Fatalf("panic must return a typed internal error: %s", w.Body.String())
	}
}

func TestPanicIsCountedAs500(t *testing.T) {
	s := &Server{httpMetrics: newHTTPMetrics()}
	h := s.instrument(requestIDAndRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("drill")
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drill", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic must be 500, got %d", w.Code)
	}
	var buf bytes.Buffer
	s.httpMetrics.render(&buf)
	if !strings.Contains(buf.String(), `status="500"`) {
		t.Fatalf("panic must be counted as 500:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), `method="GET",status="200"`) {
		t.Fatalf("panic must not be counted as 200:\n%s", buf.String())
	}
}
