package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
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
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupTestServer(t *testing.T) (*api.Server, func()) { // Tests exercise the human-reviewer sign endpoint with an ephemeral key.
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
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

// testAdminEnvelope binds mandate-sign parameters under a tenant admin key,
// mirroring exactly what handleMandateSign hashes (effective values,
// JSON number semantics: float64 budget, int64 ttl).
func testAdminEnvelope(t *testing.T, tenantID, pubHex string, priv ed25519.PrivateKey, jobHash string, budget float64, ttl int64) *tenants.AdminAction {
	t.Helper()
	params := map[string]any{"job_hash": jobHash, "max_budget_usd": budget, "ttl_seconds": ttl}
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatal(err)
	}
	act := &tenants.AdminAction{
		TenantID: tenantID, Action: "mandate_sign", ParamsHash: hash,
		Expiry: time.Now().UTC().Add(time.Hour), PubkeyHex: pubHex,
	}
	sig := ed25519.Sign(priv, act.CanonicalPayload())
	act.SignatureHex = hex.EncodeToString(sig)
	return act
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

	// 7. Mandate Signing (tenant registered first: mandates bind lab tenants).
	// Spend authority needs the tenant admin's envelope: bootstrap the admin
	// key (first-write), then sign the exact parameters. Bearer gate first.
	tenantBody, _ := json.Marshal(map[string]any{"id": "tenant-lab-alpha", "display_name": "Lab Alpha"})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(tenantBody))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for /api/tenants, got %d", w.Code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminPubHex := hex.EncodeToString(adminPub)
	if _, err := srv.TenantManager().SetAdminKey("tenant-lab-alpha", adminPubHex, nil); err != nil {
		t.Fatalf("admin bootstrap: %v", err)
	}
	signPayload := map[string]any{
		"tenant_id":      "tenant-lab-alpha",
		"job_hash":       "job-sha256-test",
		"max_budget_usd": 25.0,
		"ttl_seconds":    int64(3600),
		"admin_action":   testAdminEnvelope(t, "tenant-lab-alpha", adminPubHex, adminPriv, "job-sha256-test", 25.0, int64(3600)),
	}
	body, _ := json.Marshal(signPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/mandates/sign, got %d: %s", w.Code, w.Body.String())
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
	// 1s timeout forces LLM fallback: template turns cannot corroborate,
	// so consensus must be false (B1-1 — fallback fiction no longer passes).
	if tribResp.Tribunal.ConsensusPassed {
		t.Fatalf("fallback tribunal must not claim consensus")
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
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&signResp)

	verifyBody, _ := json.Marshal(signResp.Mandate)
	req = httptest.NewRequest(http.MethodPost, "/api/mandates/verify", bytes.NewReader(verifyBody))
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/mandates/verify, got %d", w.Code)
	}
	var verifyResp struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&verifyResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !verifyResp.Valid {
		t.Fatalf("registered-tenant mandate must verify valid: %+v", verifyResp)
	}
}

func TestAutoresearchAndMetasearchAPIs(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	handler := srv.Handler()

	// 1. Get Harnesses
	req := httptest.NewRequest(http.MethodGet, "/api/autoresearch/harnesses", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/autoresearch/harnesses, got %d", w.Code)
	}
	var harnessResp struct {
		Harnesses []struct {
			ID string `json:"id"`
		} `json:"harnesses"`
		DockerRunner struct {
			Image string `json:"image"`
		} `json:"docker_runner"`
	}
	if err := json.NewDecoder(w.Body).Decode(&harnessResp); err != nil {
		t.Fatalf("decode harnesses error: %v", err)
	}
	if len(harnessResp.Harnesses) != 5 {
		t.Fatalf("expected 5 shipping harnesses, got %d", len(harnessResp.Harnesses))
	}
	if harnessResp.DockerRunner.Image != "kubona/quantum-runner:latest" {
		t.Fatalf("expected docker runner image kubona/quantum-runner:latest, got %s", harnessResp.DockerRunner.Image)
	}

	// 2. Start Loop
	startPayload := map[string]any{
		"harness_id":     "ansatz-search",
		"max_iterations": 10,
	}
	body, _ := json.Marshal(startPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/autoresearch/start", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("expected 201 for /api/autoresearch/start, got %d: %s", w.Code, w.Body.String())
	}
	var startResp struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		HarnessID string `json:"harness_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&startResp); err != nil {
		t.Fatalf("decode start loop error: %v", err)
	}
	if startResp.ID == "" || startResp.HarnessID != "ansatz-search" {
		t.Fatalf("invalid started loop: %+v", startResp)
	}

	// 3. List Loops
	req = httptest.NewRequest(http.MethodGet, "/api/autoresearch/loops", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/autoresearch/loops, got %d", w.Code)
	}

	// 4. Step Loop
	req = httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+startResp.ID+"/step", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for stepping loop, got %d: %s", w.Code, w.Body.String())
	}

	// 5. Pause & Resume
	req = httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+startResp.ID+"/pause", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for pause loop, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+startResp.ID+"/resume", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for resume loop, got %d", w.Code)
	}

	// 6. Stop
	req = httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+startResp.ID+"/stop", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for stop loop, got %d", w.Code)
	}

	// 7. Metasearch Proposals
	req = httptest.NewRequest(http.MethodGet, "/api/metasearch/proposals", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/metasearch/proposals, got %d", w.Code)
	}
}
