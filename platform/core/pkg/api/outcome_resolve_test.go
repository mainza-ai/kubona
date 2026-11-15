package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveOutcomeRequiresSignature(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/job-1/resolve", strings.NewReader(`{"outcome":"vendor_never_received","evidence":"no vendor job","action":{}}`))
	req.Header.Set("Authorization", "Bearer op-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "admin signature is required") {
		t.Fatalf("unsigned resolution must refuse: %d %s", w.Code, w.Body.String())
	}
}
