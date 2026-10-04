package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupTestServer(t *testing.T) (*api.Server, func()) {
	dbPath := "test_api.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}

	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)

	srv := api.NewServer(l, sm, jr, sup, wd)

	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}

	return srv, cleanup
}

func TestAPIRoutes(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	handler := srv.Handler()

	// 1. Health
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/health, got %d", w.Code)
	}

	// 2. Agents
	req = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/agents, got %d", w.Code)
	}

	// 3. Fidelity
	req = httptest.NewRequest(http.MethodGet, "/api/fidelity?device=ibm_heron&depth=6&two_qubit_gates=2", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/fidelity, got %d", w.Code)
	}

	// 4. Epistemics
	req = httptest.NewRequest(http.MethodGet, "/api/epistemics", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/epistemics, got %d", w.Code)
	}

	// 5. Fleet Radar
	req = httptest.NewRequest(http.MethodGet, "/api/fleet", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/fleet, got %d", w.Code)
	}

	// 6. Host Port Radar
	req = httptest.NewRequest(http.MethodGet, "/api/radar", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/radar, got %d", w.Code)
	}

	// 7. Mandate Signing
	signPayload := map[string]any{
		"tenant_id":      "tenant-lab-alpha",
		"job_hash":       "job-sha256-test",
		"max_budget_usd": 25.0,
		"ttl_seconds":    3600,
	}
	body, _ := json.Marshal(signPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/mandates/sign, got %d", w.Code)
	}

	// 8. Notebooks
	req = httptest.NewRequest(http.MethodGet, "/api/results/notebooks", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/results/notebooks, got %d", w.Code)
	}

	// 9. Multi-Agent Tribunal Debate
	tribunalPayload := map[string]any{
		"session_id": "test-session-tribunal",
		"prompt":     "Investigate LiH ground state dissociation VQE",
	}
	body, _ = json.Marshal(tribunalPayload)
	testCtx, testCancel := context.WithTimeout(t.Context(), 1*time.Second)
	defer testCancel()
	req = httptest.NewRequest(http.MethodPost, "/api/chat/tribunal", bytes.NewReader(body)).WithContext(testCtx)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	t.Logf("Tribunal response code: %d, body: %q", w.Code, w.Body.String())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/chat/tribunal, got %d: %s", w.Code, w.Body.String())
	}
	var tribResp struct {
		Tribunal struct {
			Target          string `json:"target"`
			ConsensusPassed bool   `json:"consensus_passed"`
			Turns           []any  `json:"turns"`
		} `json:"tribunal"`
	}
	if err := json.NewDecoder(w.Body).Decode(&tribResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(tribResp.Tribunal.Turns) != 4 {
		t.Fatalf("expected 4 tribunal turns, got %d", len(tribResp.Tribunal.Turns))
	}
	if !tribResp.Tribunal.ConsensusPassed {
		t.Fatalf("expected tribunal consensus to pass")
	}

	// 10. Client Mandate Verification
	var signResp struct {
		Mandate struct {
			TenantID        string  `json:"tenant_id"`
			JobHash         string  `json:"job_hash"`
			MaxBudgetUSD    float64 `json:"max_budget_usd"`
			ExpiryTimestamp string  `json:"expiry_timestamp"`
			PublicKeyHex    string  `json:"public_key_hex"`
			SignatureHex    string  `json:"signature_hex"`
		} `json:"mandate"`
	}
	signBytes, _ := json.Marshal(signPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(signBytes))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&signResp)

	verifyBody, _ := json.Marshal(signResp.Mandate)
	req = httptest.NewRequest(http.MethodPost, "/api/mandates/verify", bytes.NewReader(verifyBody))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/mandates/verify, got %d", w.Code)
	}
}
