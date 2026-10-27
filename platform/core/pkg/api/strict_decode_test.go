package api_test

// I6 tests: spend mutations refuse unknown fields (a typo'd mandate_ref
// 400s with a typed code instead of parsing as absent). Read/query
// routes stay lenient (not covered here by design).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postUnknownField(t *testing.T, h http.Handler, path string, body map[string]any, bearer bool) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	if bearer {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST %s with unknown field must 400, got %d %s", path, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["code"] != "bad_request" {
		t.Fatalf("POST %s refusal must carry typed bad_request: %s", path, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "bogus_field_xyz") {
		t.Fatalf("POST %s refusal must name the offending field: %s", path, w.Body.String())
	}
}

func TestSpendMutationsRefuseUnknownFields(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "strict fields")

	postUnknownField(t, h, "/api/jobs", map[string]any{
		"session_id": sid, "qasm": "OPENQASM 2.0;qreg q[1];", "backend": "local_simulator",
		"shots": 8, "bogus_field_xyz": 1,
	}, false)
	postUnknownField(t, h, "/api/chat/launch", map[string]any{
		"session_id": sid, "qasm": "OPENQASM 2.0;qreg q[1];", "bogus_field_xyz": 1,
	}, false)
	postUnknownField(t, h, "/api/chat/tribunal", map[string]any{
		"session_id": sid, "prompt": "x", "bogus_field_xyz": 1,
	}, false)
	postUnknownField(t, h, "/api/autoresearch/start", map[string]any{
		"session_id": sid, "harness_id": "vqe", "bogus_field_xyz": 1,
	}, false)
	postUnknownField(t, h, "/api/mandates/sign", map[string]any{
		"tenant_id": "t", "job_hash": "h", "max_budget_usd": 1.0, "bogus_field_xyz": 1,
	}, true)
	postUnknownField(t, h, "/api/loops", map[string]any{
		"session_id": sid, "max_steps": 3, "bogus_field_xyz": 1,
	}, true)
}
