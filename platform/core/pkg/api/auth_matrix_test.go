package api_test

// Regression pin for the 2026-10-26 audit: these state-changing routes
// were reachable without any credential. Each must refuse an anonymous
// caller before parsing/acting. The list is deliberately explicit so a
// future route re-registration cannot silently drop the wrapper.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStateChangingRoutesRefuseAnonymous(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/sessions/import"},
		{http.MethodPost, "/api/sessions/refund"},
		{http.MethodPost, "/api/a2a/v1"},
		{http.MethodPost, "/api/a2a/v1/challenge"},
		{http.MethodPost, "/api/llm/test"},
		{http.MethodPost, "/api/loops"},
		{http.MethodPost, "/api/loops/no-such-loop/kill"},
		{http.MethodGet, "/api/memory/recall?session_id=x"},
		{http.MethodPost, "/api/memory/retain"},
		{http.MethodPost, "/api/mandates/pqc-verify"},
		// Phase-0 gates: spend/compute writers that were reachable
		// without any credential (mandate verification mints ledger
		// receipts + obligations; settle mints settlement receipts;
		// krylov/mitigation proxy metered compute).
		{http.MethodPost, "/api/mandates/verify"},
		{http.MethodPost, "/api/cost/settle"},
		{http.MethodPost, "/api/krylov/solve"},
		{http.MethodPost, "/api/mitigation/apply"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader("{}"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s must 401 unauthenticated, got %d (%s)",
				c.method, c.path, w.Code, w.Body.String())
		}
	}
}
