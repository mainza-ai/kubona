package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Human dispatch proxy (R-34): Go owns auth + autonomy + validation +
// routing; execution semantics live in the Python rail (hermetic bridge
// tests). These pins stop at the bridge boundary — full-path verification
// needs live keys (see the R-34 runbook).

func postQuantumJob(t *testing.T, h http.Handler, bearer bool, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/quantum/jobs", bytes.NewReader(raw))
	if bearer {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestQuantumJobsAuthAndValidation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()
	good := map[string]any{
		"qasm":            "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
		"provider":        "local-simulator",
		"shots":           100,
		"idempotency_key": "human-1",
	}

	// No bearer → 401. No bridge contact for any refusal below: the
	// test daemon has no bridge, so any 503-shaped answer would be a
	// validation gap (validation must precede the proxy call).
	if code, out := postQuantumJob(t, h, false, good); code != http.StatusUnauthorized || out["code"] != "unauthorized" {
		t.Fatalf("bearerless dispatch must 401 typed, got %d %v", code, out)
	}
	// Missing idempotency key → 400, never dispatched.
	noKey := map[string]any{"qasm": good["qasm"], "provider": "local-simulator", "shots": 100}
	if code, _ := postQuantumJob(t, h, true, noKey); code != http.StatusBadRequest {
		t.Fatalf("keyless dispatch must 400, got %d", code)
	}
	// Unknown provider → 400 with the backends hint.
	badProv := map[string]any{"qasm": good["qasm"], "provider": "nope-9000",
		"shots": 100, "idempotency_key": "human-2"}
	if code, out := postQuantumJob(t, h, true, badProv); code != http.StatusBadRequest {
		t.Fatalf("unknown provider must 400, got %d %v", code, out)
	}
	// Ghost session → 404 ghost_session, not a silent implicit session.
	ghost := map[string]any{"qasm": good["qasm"], "provider": "local-simulator",
		"shots": 100, "idempotency_key": "human-3", "session_id": "ghost"}
	if code, out := postQuantumJob(t, h, true, ghost); code != http.StatusNotFound || out["code"] != "ghost_session" {
		t.Fatalf("ghost session must 404 typed, got %d %v", code, out)
	}
}

func TestQuantumJobActionsNeedID(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/quantum/jobs/", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("job actions without an id must 400, got %d", w.Code)
	}
}
