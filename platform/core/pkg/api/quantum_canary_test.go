package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuantumCanaryValidation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Unknown provider → 400, never proxied.
	req := httptest.NewRequest(http.MethodGet, "/api/quantum/canary?providers=nope-9", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown canary provider must 400, got %d", w.Code)
	}
	// Wrong method → 405.
	req = httptest.NewRequest(http.MethodPost, "/api/quantum/canary?providers=local-simulator", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("canary POST must 405, got %d", w.Code)
	}
	// No bridge in tests: the live path fails soft to 503 (declared
	// catalog stays the UI fallback) instead of hanging or faking data.
	req = httptest.NewRequest(http.MethodGet, "/api/quantum/canary?providers=local-simulator", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("bridgeless canary must 503, got %d", w.Code)
	}
	// HW-03: explicit external hardware canary without allow_hardware=true
	// and auth must refuse (403) — never dispatch live QPU on an open GET.
	req = httptest.NewRequest(http.MethodGet, "/api/quantum/canary?providers=ibm-qiskit", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated ibm-qiskit canary without allow_hardware must 403, got %d", w.Code)
	}
	// Even with allow_hardware=true, anonymous callers are refused (401 or 403 if air-gapped).
	req = httptest.NewRequest(http.MethodGet, "/api/quantum/canary?providers=ibm-qiskit&allow_hardware=true", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("anonymous ibm-qiskit canary with allow_hardware must 401/403, got %d", w.Code)
	}
}
