package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// B0-3: launch refuses vacuous dispatches with the reason stated — no
// silent Bell circuit, no silent local_simulator. Refusals leave no
// residue (SubmitJob never runs).
func TestChatLaunchRefusesEmpty(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	sessBody, _ := json.Marshal(map[string]any{"title": "launch drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessW, sessReq)
	if sessW.Code != http.StatusCreated {
		t.Fatalf("session: %d", sessW.Code)
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sess); err != nil {
		t.Fatal(err)
	}

	launch := func(payload map[string]any) (int, string) {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/chat/launch", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	if code, body := launch(map[string]any{"session_id": sess.ID}); code != http.StatusBadRequest || !strings.Contains(body, "requires qasm") {
		t.Errorf("empty launch must 400 naming qasm, got %d %q", code, body)
	}
	if code, body := launch(map[string]any{
		"session_id": sess.ID,
		"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
	}); code != http.StatusBadRequest || !strings.Contains(body, "requires backend") {
		t.Errorf("backend-less launch must 400 naming backend, got %d %q", code, body)
	}
	// Proposal without either still refuses (proposal is not a circuit).
	if code, _ := launch(map[string]any{
		"session_id": sess.ID,
		"proposal":   map[string]any{"target": "vibes", "recommended_tier": ""},
	}); code != http.StatusBadRequest {
		t.Errorf("circuit-less proposal must 400, got %d", code)
	}

	// Explicit circuit + backend dispatches (QUEUED: no workers in tests).
	code, _ := launch(map[string]any{
		"session_id": sess.ID,
		"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
		"backend":    "local_simulator",
		"budget_usd": 1.0,
	})
	if code != http.StatusCreated {
		t.Errorf("explicit launch must dispatch, got %d", code)
	}
}

// R17: proposal attribution survives dispatch — designed_by rides the job
// params (row + job_queued event carry params_json verbatim). Present is
// transcribed; absent stays absent (never inferred).
func TestChatLaunchCarriesDesignedBy(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	sessBody, _ := json.Marshal(map[string]any{"title": "attribution drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessW, sessReq)
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sess); err != nil {
		t.Fatal(err)
	}
	qasm := "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n"
	launch := func(proposal map[string]any) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"session_id": sess.ID, "backend": "local_simulator",
			"budget_usd": 1.0, "proposal": proposal,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/chat/launch", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("launch must dispatch, got %d %s", w.Code, w.Body.String())
		}
		var out struct {
			Job struct {
				Params map[string]any `json:"params_json"`
			} `json:"job"`
		}
		if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out.Job.Params
	}
	// Fallback-stamped proposal keeps its stamp.
	params := launch(map[string]any{"qasm": qasm, "designed_by": "server-fallback"})
	if params["designed_by"] != "server-fallback" {
		t.Errorf("designed_by must survive dispatch, got %v", params["designed_by"])
	}
	// Unstamped proposal stays unstamped — authorship is never inferred.
	params = launch(map[string]any{"qasm": qasm})
	if _, ok := params["designed_by"]; ok {
		t.Errorf("absent attribution must stay absent, got %v", params["designed_by"])
	}
}
