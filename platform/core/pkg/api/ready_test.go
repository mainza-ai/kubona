package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyFailsWhenDatabaseIsClosed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	srv.Ledger().Close()
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed DB must make readiness 503, got %d: %s", w.Code, w.Body.String())
	}
}
