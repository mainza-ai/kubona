package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIV1AliasMatchesLegacy(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	for _, path := range []string{"/api/health", "/api/v1/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if got := w.Header().Get("Kubona-API-Version"); got != "1" {
		t.Fatalf("version header: %q", got)
	}
}
