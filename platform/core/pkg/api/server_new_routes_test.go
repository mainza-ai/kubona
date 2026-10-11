package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/cost"
	"github.com/mainza-ai/kubona/core/pkg/grants"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
)

func setupTestRoutesServer(t *testing.T) (*api.Server, func()) {
	// Tests exercise the human-reviewer sign endpoint with an ephemeral key.
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	dbPath := "test_new_routes.db"
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

func TestPQCKeyAndVerifyEndpoints(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	// 1. GET /api/mandates/pqc-key
	req := httptest.NewRequest(http.MethodGet, "/api/mandates/pqc-key", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var keyResp struct {
		Fingerprint         string `json:"fingerprint"`
		EncapsulationKeyHex string `json:"encapsulation_key_hex"`
		Algorithm           string `json:"algorithm"`
	}
	if err := json.NewDecoder(w.Body).Decode(&keyResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if keyResp.Fingerprint == "" || keyResp.EncapsulationKeyHex == "" {
		t.Fatalf("empty key response: %+v", keyResp)
	}

	// 2. Encapsulate a test mandate
	_, priv, _ := ed25519.GenerateKey(nil)
	tenant := "tenant-pqc-1"
	jobHash := "job-hash-1"
	budget := 15.00
	expiry := time.Now().Add(1 * time.Hour).UTC()

	m, err := mandates.SignMandate(priv, tenant, jobHash, budget, expiry)
	if err != nil {
		t.Fatalf("sign mandate failed: %v", err)
	}

	hybrid, err := mandates.EncapsulateMandate(srv.PQCKeypair().EncapsulationKey, m)
	if err != nil {
		t.Fatalf("encapsulate failed: %v", err)
	}

	// 3. POST /api/mandates/pqc-verify
	body, _ := json.Marshal(hybrid)
	vReq := httptest.NewRequest(http.MethodPost, "/api/mandates/pqc-verify", bytes.NewReader(body))
	vW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(vW, vReq)

	if vW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", vW.Code, vW.Body.String())
	}
}

func TestQIRCompileEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	body := []byte(`{"qasm":"OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\nh q[0];\ncx q[0], q[1];\n"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/qir/compile", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary struct {
		NumQubits  int    `json:"num_qubits"`
		GateCount  int    `json:"gate_count"`
		QIRProfile string `json:"qir_profile"`
	}
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("failed to decode summary: %v", err)
	}
	if summary.NumQubits < 2 || !strings.HasPrefix(summary.QIRProfile, "BaseProfile") {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestKrylovAndMitigationEndpoints(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	// 1. POST /api/krylov/solve with no bridge client must fail closed (503),
	// never return a canned energy as a measurement.
	kBody := []byte(`{"molecule":"H2","bond_distance":0.7414,"subspace_dimension":4,"time_step":0.2}`)
	kReq := httptest.NewRequest(http.MethodPost, "/api/krylov/solve", bytes.NewReader(kBody))
	kW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(kW, kReq)

	if kW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for krylov without bridge, got %d", kW.Code)
	}

	var kResp struct {
		Success         bool   `json:"success"`
		ResultComputed  bool   `json:"result_computed"`
		ErrorMessage    string `json:"error_message"`
		ClassicalAnchor struct {
			FCIEnergy float64 `json:"fci_energy_hartree"`
		} `json:"classical_anchor"`
	}
	if err := json.NewDecoder(kW.Body).Decode(&kResp); err != nil {
		t.Fatalf("failed to decode krylov response: %v", err)
	}
	if kResp.Success || kResp.ResultComputed || kResp.ErrorMessage == "" {
		t.Fatalf("krylov refusal must carry success:false, result_computed:false and an error: %+v", kResp)
	}
	if kResp.ClassicalAnchor.FCIEnergy != -1.13727 {
		t.Fatalf("krylov refusal must carry the labeled cached anchor: %+v", kResp)
	}

	// 2. POST /api/mitigation/apply with no bridge client must fail closed too.
	mBody := []byte(`{"qasm":"OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0], q[1];","method":"ZNE","scale_factors":[1.0, 1.5, 2.0]}`)
	mReq := httptest.NewRequest(http.MethodPost, "/api/mitigation/apply", bytes.NewReader(mBody))
	mW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mW, mReq)

	if mW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for mitigation without bridge, got %d", mW.Code)
	}

	var mResp struct {
		Success        bool   `json:"success"`
		ResultComputed bool   `json:"result_computed"`
		MethodUsed     string `json:"method_used"`
	}
	if err := json.NewDecoder(mW.Body).Decode(&mResp); err != nil {
		t.Fatalf("failed to decode mitigation response: %v", err)
	}
	if mResp.Success || mResp.ResultComputed {
		t.Fatalf("mitigation refusal must carry success:false and result_computed:false: %+v", mResp)
	}
}

func TestMandateSignRefusesEphemeralByDefault(t *testing.T) {
	// Without KUBONA_ALLOW_EPHEMERAL_SIGNER the reviewer sign endpoint must
	// refuse (403): no silent signing with an unconfigured key.
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "")
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	signPayload := map[string]any{
		"tenant_id": "tenant QFile", "job_hash": "abc123", "max_budget_usd": 10.0, "ttl_seconds": 60,
	}
	body, _ := json.Marshal(signPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for ephemeral signer without opt-in, got %d", w.Code)
	}
}

func TestPaymentSettleEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	pm := srv.A2AServer().PaymentManager()
	ch := pm.GenerateChallenge("USDC", "0.25")

	pub, priv, _ := ed25519.GenerateKey(nil)
	msgHash := sha256.Sum256([]byte(ch.Payload))
	sig := ed25519.Sign(priv, msgHash[:])
	receipt := cost.PaymentReceipt{
		ChallengeID: ch.ChallengeID,
		Token:       ch.Token,
		Amount:      ch.Amount,
		PayerPubHex: hex.EncodeToString(pub),
		Signature:   hex.EncodeToString(sig),
		Timestamp:   time.Now().Unix(),
	}

	body, _ := json.Marshal(map[string]any{
		"receipt": receipt,
		"network": "base-mainnet",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/cost/settle", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sResp struct {
		Settled   bool   `json:"settled"`
		Simulated bool   `json:"simulated"`
		Reference string `json:"reference"`
		Network   string `json:"network"`
	}
	if err := json.NewDecoder(w.Body).Decode(&sResp); err != nil {
		t.Fatalf("failed to decode settle response: %v", err)
	}
	if !sResp.Settled || !sResp.Simulated || sResp.Reference == "" || sResp.Network != "base-mainnet" {
		t.Fatalf("settle must be explicit local-only: %+v", sResp)
	}
}

func TestHFHubEndpoints(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	// 1. GET /api/hf/search
	req1 := httptest.NewRequest(http.MethodGet, "/api/hf/search?q=qwen&library=gguf&limit=5", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	var searchResp struct {
		OK     bool `json:"ok"`
		Models []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
		} `json:"models"`
	}
	if err := json.NewDecoder(w1.Body).Decode(&searchResp); err != nil {
		t.Fatalf("failed to decode search resp: %v", err)
	}
	if !searchResp.OK || len(searchResp.Models) == 0 {
		t.Fatalf("expected models in search resp: %+v", searchResp)
	}

	// 2. GET /api/hf/models/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF
	req2 := httptest.NewRequest(http.MethodGet, "/api/hf/models/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// 3. POST /api/hf/download
	dBody := []byte(`{"repo_id":"Qwen/Qwen2.5-Coder-7B-Instruct-GGUF","filename":"model.gguf"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/api/hf/download", bytes.NewReader(dBody))
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}

	// 4. GET /api/hf/downloads
	req4 := httptest.NewRequest(http.MethodGet, "/api/hf/downloads", nil)
	w4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w4, req4)

	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w4.Code)
	}
}

func TestCustomAgentAndMeshRouteEndpoints(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	// 1. GET /api/agents/custom
	req1 := httptest.NewRequest(http.MethodGet, "/api/agents/custom", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	// 2. POST /api/agents/custom
	createBody := []byte(`{
		"id": "agent_test_analyst",
		"name": "Test Science Analyst",
		"role": "disconfirmation_officer",
		"description": "Analyzes variational convergence",
		"system_prompt": "You are a test analyst",
		"avatar_emoji": "🔬",
		"badge_color": "purple",
		"capabilities": ["test"],
		"modalities": ["text"],
		"preferred_model": "local/qwen2.5-coder:32b",
		"mcp_tools": [],
		"spend_budget_usd": 1.0,
		"memory_scopes": ["session"]
	}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/agents/custom", bytes.NewReader(createBody))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// 3. POST /api/mesh/route
	routeBody := []byte(`{
		"prompt": "Optimize VQE ansatz for FeMoco",
		"items": [{"modality": "molecule_3d", "data": "H2"}],
		"spend_budget_usd": 5.0
	}`)
	req3 := httptest.NewRequest(http.MethodPost, "/api/mesh/route", bytes.NewReader(routeBody))
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}

	var rResp struct {
		OK       bool                   `json:"ok"`
		Decision map[string]interface{} `json:"decision"`
	}
	if err := json.NewDecoder(w3.Body).Decode(&rResp); err != nil {
		t.Fatalf("failed to decode mesh route resp: %v", err)
	}
	if !rResp.OK || rResp.Decision["model"] == "" {
		t.Fatalf("unexpected route decision: %+v", rResp)
	}
}

func TestLedgerAppendIssuesServerReceipt(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]any{
		"session_id": "sess-tribunal",
		"event_type": "tribunal_commit",
		"payload":    map[string]any{"hypothesis_id": "H-1", "quorum_percent": 95.2},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ledger/append", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		OK         bool   `json:"ok"`
		RecordHash string `json:"record_hash"`
		EventID    int64  `json:"event_id"`
		PrevHash   string `json:"prev_hash"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !resp.OK || len(resp.RecordHash) != 64 || resp.EventID <= 0 {
		t.Fatalf("server-issued receipt malformed: %+v", resp)
	}

	// A second commit must chain (different hash, prev linked), proving hash linkage.
	body2, _ := json.Marshal(map[string]any{
		"session_id": "sess-tribunal",
		"event_type": "peer_review",
		"payload":    map[string]any{"score": 9},
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/ledger/append", bytes.NewReader(body2))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	var resp2 struct {
		RecordHash string `json:"record_hash"`
		PrevHash   string `json:"prev_hash"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp2.RecordHash == resp.RecordHash || resp2.PrevHash != resp.RecordHash {
		t.Fatalf("ledger chaining broken: %+v vs %+v", resp2, resp)
	}

	// Non-allowlisted event types are refused, not recorded.
	badBody, _ := json.Marshal(map[string]any{
		"session_id": "sess-tribunal",
		"event_type": "forged_advantage_claim",
		"payload":    map[string]any{},
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/ledger/append", bytes.NewReader(badBody))
	badW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badW, badReq)
	if badW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-allowlisted event, got %d", badW.Code)
	}
}

func TestSupervisorAbortSettlesJobsFailed(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	// Real session first: dispatch is attributed to registered sessions.
	sessBody, _ := json.Marshal(map[string]any{"title": "abort drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessW, sessReq)
	if sessW.Code != http.StatusCreated {
		t.Fatalf("expected 201 for session, got %d", sessW.Code)
	}
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Queue a job (no workers running in tests, so it stays QUEUED).
	jobBody, _ := json.Marshal(map[string]any{
		"session_id": sessResp.ID, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "shots": 100,
	})
	jobReq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jobBody))
	jobW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(jobW, jobReq)
	if jobW.Code != http.StatusCreated {
		t.Fatalf("expected 201 for job submit, got %d", jobW.Code)
	}

	abortBody, _ := json.Marshal(map[string]any{"session_id": sessResp.ID})
	abortReq := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abortBody))
	abortW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(abortW, abortReq)
	if abortW.Code != http.StatusOK {
		t.Fatalf("expected 200 for abort, got %d", abortW.Code)
	}
	var abortResp struct {
		OK           bool   `json:"ok"`
		AbortedCount int    `json:"aborted_count"`
		AbortReceipt string `json:"abort_receipt"`
	}
	if err := json.NewDecoder(abortW.Body).Decode(&abortResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !abortResp.OK || abortResp.AbortedCount != 1 || len(abortResp.AbortReceipt) != 64 {
		t.Fatalf("abort receipt malformed: %+v", abortResp)
	}

	// The queued job must now read back failed.
	listReq := httptest.NewRequest(http.MethodGet, "/api/jobs?session_id="+sessResp.ID, nil)
	listW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listW, listReq)
	var listResp struct {
		Jobs []struct {
			Status int32 `json:"status"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(listW.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(listResp.Jobs) != 1 || listResp.Jobs[0].Status != int32(pb.JobStatus_JOB_STATUS_FAILED) {
		t.Fatalf("aborted job must read back failed: %+v", listResp)
	}
}

func edSignForTest(t *testing.T, priv []byte, msg []byte) []byte {
	t.Helper()
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("bad privkey size %d", len(priv))
	}
	return ed25519.Sign(ed25519.PrivateKey(priv), msg)
}

func signTenantAction(t *testing.T, priv []byte, pubHex, tenantID, action string, params any) tenants.AdminAction {
	t.Helper()
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatalf("params hash: %v", err)
	}
	a := tenants.AdminAction{
		TenantID:   tenantID,
		Action:     action,
		ParamsHash: hash,
		Expiry:     time.Now().UTC().Add(5 * time.Minute),
		PubkeyHex:  pubHex,
	}
	// Sign via the mandates-equivalent canonical encoding through tenants package helper.
	sig := edSignForTest(t, priv, a.CanonicalPayload())
	a.SignatureHex = hex.EncodeToString(sig)
	return a
}

func TestTenantRoutesLifecycle(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// Register lab.
	createBody, _ := json.Marshal(map[string]any{"id": "lab-route", "display_name": "Route Lab"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created struct {
		ID            string  `json:"id"`
		AutonomyLevel string  `json:"autonomy_level"`
		SpendCeiling  float64 `json:"spend_ceiling_usd"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.AutonomyLevel != "L1" || created.SpendCeiling != 150.0 {
		t.Fatalf("new lab must start L1/150: %+v", created)
	}

	// Bootstrap admin key (first write, no envelope).
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	adminBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "new_admin_pubkey_hex": pubHex})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(adminBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin bootstrap, got %d: %s", w.Code, w.Body.String())
	}

	// Grant L2 with a bound envelope.
	grantAct := signTenantAction(t, priv, pubHex, "lab-route", "grant_autonomy", map[string]string{"level": "L2"})
	grantBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "level": "L2", "action": grantAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/autonomy", bytes.NewReader(grantBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for grant, got %d: %s", w.Code, w.Body.String())
	}

	// Mismatched params must refuse (envelope binds different level).
	badAct := signTenantAction(t, priv, pubHex, "lab-route", "grant_autonomy", map[string]string{"level": "L3"})
	badBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "level": "L2", "action": badAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/autonomy", bytes.NewReader(badBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for param-mismatched envelope, got %d", w.Code)
	}

	// Revoke drops to L0.
	revAct := signTenantAction(t, priv, pubHex, "lab-route", "revoke_autonomy", map[string]string{"reason": "drill"})
	revBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "reason": "drill", "action": revAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/revoke", bytes.NewReader(revBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for revoke, got %d: %s", w.Code, w.Body.String())
	}
	var revoked struct {
		AutonomyLevel string `json:"autonomy_level"`
	}
	if err := json.NewDecoder(w.Body).Decode(&revoked); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if revoked.AutonomyLevel != "L0" {
		t.Fatalf("revoke must drop to L0: %+v", revoked)
	}
}

func TestMandateClampedToTenantCeiling(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// Lab with a $40 ceiling.
	createBody, _ := json.Marshal(map[string]any{"id": "lab-cap", "display_name": "Cap Lab"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	adminBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-cap", "new_admin_pubkey_hex": pubHex})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(adminBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d", w.Code)
	}
	ceilAct := signTenantAction(t, priv, pubHex, "lab-cap", "set_ceiling", map[string]float64{"ceiling_usd": 40})
	ceilBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-cap", "ceiling_usd": 40.0, "action": ceilAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/ceiling", bytes.NewReader(ceilBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set ceiling: %d %s", w.Code, w.Body.String())
	}

	// Unknown tenant must refuse (no more session-id-as-tenant conflation).
	verify := func(tenant string, budget float64) bool {
		m, err := mandates.SignMandate(priv, tenant, "job-cap", budget, time.Now().UTC().Add(time.Hour))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		body, _ := json.Marshal(m)
		r := httptest.NewRequest(http.MethodPost, "/api/mandates/verify", bytes.NewReader(body))
		wr := httptest.NewRecorder()
		h.ServeHTTP(wr, r)
		var resp struct {
			Valid bool `json:"valid"`
		}
		if err := json.NewDecoder(wr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.Valid
	}
	if verify("no-such-lab", 10) {
		t.Fatalf("unknown tenant must refuse")
	}
	if !verify("lab-cap", 25) {
		t.Fatalf("$25 under $40 ceiling must verify")
	}
	if verify("lab-cap", 100) {
		t.Fatalf("$100 over $40 ceiling must refuse")
	}
}

func TestL0TenantRefusesDispatch(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// Bootstrap default-tenant admin, then revoke the lab to L0.
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	adminBody, _ := json.Marshal(map[string]any{"tenant_id": "default", "new_admin_pubkey_hex": pubHex})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(adminBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}
	revAct := signTenantAction(t, priv, pubHex, "default", "revoke_autonomy", map[string]string{"reason": "drill"})
	revBody, _ := json.Marshal(map[string]any{"tenant_id": "default", "reason": "drill", "action": revAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/revoke", bytes.NewReader(revBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}

	// Dispatch must now refuse at the edge with 403.
	jobBody, _ := json.Marshal(map[string]any{"qasm": "OPENQASM 2.0;\\nqreg q[1];\\ncreg c[1];\\n", "shots": 100})
	jobReq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jobBody))
	jobW := httptest.NewRecorder()
	h.ServeHTTP(jobW, jobReq)
	if jobW.Code != http.StatusForbidden {
		t.Fatalf("L0 tenant dispatch must 403, got %d", jobW.Code)
	}
}

func TestLedgerExportGate(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// Own session with own-tenant records exports fine.
	sessBody, _ := json.Marshal(map[string]any{"title": "export drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	expBody, _ := json.Marshal(map[string]any{"session_id": sessResp.ID, "format": "markdown"})
	expReq := httptest.NewRequest(http.MethodPost, "/api/ledger/export", bytes.NewReader(expBody))
	expW := httptest.NewRecorder()
	h.ServeHTTP(expW, expReq)
	if expW.Code != http.StatusOK {
		t.Fatalf("own-tenant export must pass, got %d: %s", expW.Code, expW.Body.String())
	}

	// Unknown session refuses (no oracle for session existence + labels).
	badBody, _ := json.Marshal(map[string]any{"session_id": "ghost", "format": "json"})
	badReq := httptest.NewRequest(http.MethodPost, "/api/ledger/export", bytes.NewReader(badBody))
	badW := httptest.NewRecorder()
	h.ServeHTTP(badW, badReq)
	if badW.Code != http.StatusForbidden {
		t.Fatalf("unknown session export must 403, got %d", badW.Code)
	}
}

func TestA2AHarnessAndQIRDelegation(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	a2a := srv.A2AServer()

	submit := func(taskType, payload string) string {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"task_type": taskType, "payload": json.RawMessage(payload)})
		resp := a2a.HandleRequest(&agents.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "a2a.SubmitTask", Params: params})
		if resp.Error != nil {
			t.Fatalf("submit %s: %+v", taskType, resp.Error)
		}
		task, _ := resp.Result.(*agents.A2ATask)
		return task.ID
	}
	poll := func(id string) *agents.A2ATask {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"task_id": id})
		for i := 0; i < 150; i++ {
			resp := a2a.HandleRequest(&agents.JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "a2a.GetTaskStatus", Params: params})
			if resp.Error != nil {
				t.Fatalf("status: %+v", resp.Error)
			}
			task, _ := resp.Result.(*agents.A2ATask)
			if task.Status == agents.TaskStatusCompleted || task.Status == agents.TaskStatusFailed {
				return task
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("task %s never settled", id)
		return nil
	}

	// QIR lowering executes locally with a ledger receipt (offline-safe).
	qirID := submit("qir_llvm_lowering", `{"qasm":"OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0], q[1];"}`)
	qirTask := poll(qirID)
	if qirTask.Status != agents.TaskStatusCompleted {
		t.Fatalf("qir delegation must complete: %+v", qirTask)
	}
	if qirTask.Result["compiler_used"] == "" || qirTask.Result["ledger_receipt"] == "" {
		t.Fatalf("qir result must carry backend label + receipt: %+v", qirTask.Result)
	}

	// Invalid harness artifact refuses before compute (no sandbox needed).
	badID := submit("variational_ansatz_optimization", `{"harness_id":"ansatz-search","artifact":"print(1)"}`)
	badTask := poll(badID)
	if badTask.Status != agents.TaskStatusFailed {
		t.Fatalf("invalid artifact must fail: %+v", badTask)
	}

	// Unknown harness refuses.
	unknownID := submit("variational_ansatz_optimization", `{"harness_id":"nope","artifact":"x"}`)
	unknownTask := poll(unknownID)
	if unknownTask.Status != agents.TaskStatusFailed {
		t.Fatalf("unknown harness must fail: %+v", unknownTask)
	}
}

func TestCrossTenantGrantFlow(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	post := func(path string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}

	// Two labs with bootstrapped admins.
	_, privA, _ := ed25519.GenerateKey(nil)
	pubAHex := hex.EncodeToString(privA.Public().(ed25519.PublicKey))
	_, privB, _ := ed25519.GenerateKey(nil)
	pubBHex := hex.EncodeToString(privB.Public().(ed25519.PublicKey))
	for _, lab := range []string{"lab-grantor", "lab-grantee"} {
		if code, _ := post("/api/tenants", map[string]any{"id": lab}); code != http.StatusCreated {
			t.Fatalf("register %s: %d", lab, code)
		}
	}
	mkAdmin := func(lab, pub string) {
		t.Helper()
		code, _ := post("/api/tenants/admin-key", map[string]any{"tenant_id": lab, "new_admin_pubkey_hex": pub})
		if code != http.StatusOK {
			t.Fatalf("bootstrap %s: %d", lab, code)
		}
	}
	mkAdmin("lab-grantor", pubAHex)
	mkAdmin("lab-grantee", pubBHex)

	didB, err := grants.DIDForAdminPubkey(pubBHex)
	if err != nil {
		t.Fatalf("did: %v", err)
	}
	privAHex := hex.EncodeToString(privA) // 64-byte ed25519 private key
	rootSealed, _, err := grants.MintGrant(
		privAHex, didB, "lab-grantor", "lab-grantee", "qpu/submit",
		[]string{grants.ScopeQPUSubmit, grants.ScopeRead}, 60.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	chain := grants.EncodeChainB64([][]byte{rootSealed})

	// Register the chain.
	code, reg := post("/api/grants/register", map[string]any{"chain": chain})
	if code != http.StatusOK || reg["ok"] != true {
		t.Fatalf("register: %d %+v", code, reg)
	}
	rootCID, _ := reg["root_cid"].(string)
	if rootCID == "" {
		t.Fatalf("no root cid: %+v", reg)
	}

	// Session in the grantee lab.
	sessBody, _ := json.Marshal(map[string]any{"title": "grant drill", "tenant_id": "lab-grantee"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatalf("session: %v", err)
	}

	// QPU job citing the grant: admitted (201, queued — no workers in tests).
	qpu := map[string]any{"session_id": sessResp.ID, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n",
		"backend": "ibm_brisbane", "shots": 100, "max_cost_usd": 25.0, "grant_chain": chain,
		"caller_jurisdictions": []string{"US"}}
	rawQ, _ := json.Marshal(qpu)
	dbgReq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(rawQ))
	dbgW := httptest.NewRecorder()
	h.ServeHTTP(dbgW, dbgReq)
	code = dbgW.Code
	if code != http.StatusCreated {
		t.Fatalf("granted QPU job must admit, got %d", code)
	}

	// Foreign-tenant input label covered by the read grant: admitted.
	qpu["input_labels"] = []string{"tenant/lab-grantor"}
	code, _ = post("/api/jobs", qpu)
	if code != http.StatusCreated {
		t.Fatalf("grant-covered foreign label must admit, got %d", code)
	}

	// Revoke as the granting admin; both paths now refuse.
	revAct := signTenantAction(t, privA, pubAHex, "lab-grantor", "revoke_grant",
		map[string]string{"root_cid": rootCID, "reason": "drill"})
	code, _ = post("/api/grants/revoke", map[string]any{"root_cid": rootCID, "reason": "drill", "action": revAct})
	if code != http.StatusOK {
		t.Fatalf("revoke: %d", code)
	}
	code, _ = post("/api/jobs", qpu)
	if code == http.StatusCreated {
		t.Fatalf("revoked grant must refuse dispatch")
	}
}

func TestTenantsListForLabsView(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	post := func(body map[string]any) int {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	if code := post(map[string]any{"id": "lab-a", "display_name": "Lab A"}); code != http.StatusCreated {
		t.Fatalf("create lab-a, got %d", code)
	}
	if code := post(map[string]any{"id": "lab-b", "display_name": "Lab B"}); code != http.StatusCreated {
		t.Fatalf("create lab-b, got %d", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tenants", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tenants, got %d", w.Code)
	}
	var resp struct {
		Tenants []struct {
			ID              string  `json:"id"`
			DisplayName     string  `json:"display_name"`
			AutonomyLevel   string  `json:"autonomy_level"`
			SpendCeilingUSD float64 `json:"spend_ceiling_usd"`
			SpentUSD        float64 `json:"spent_usd"`
			Status          string  `json:"status"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Tenants) < 3 { // default + lab-a + lab-b
		t.Fatalf("expected at least 3 labs, got %d", len(resp.Tenants))
	}
	for _, lab := range resp.Tenants {
		if lab.AutonomyLevel == "" || lab.Status == "" {
			t.Fatalf("lab row must carry autonomy + status: %+v", lab)
		}
	}
}

func TestLiteratureRouteGuards(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// Missing query: 400, never a bridge call.
	req := httptest.NewRequest(http.MethodGet, "/api/literature", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("queryless literature request must 400, got %d", w.Code)
	}

	// Wrong method: 405.
	req = httptest.NewRequest(http.MethodPost, "/api/literature?query=vqe", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST literature must 405, got %d", w.Code)
	}

	// Nil bridge test double: honest 503, never silent data.
	req = httptest.NewRequest(http.MethodGet, "/api/literature?query=vqe&limit=3", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("bridgeless literature must 503, got %d", w.Code)
	}
}

func TestCampaignVersionsEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	sessBody, _ := json.Marshal(map[string]any{"title": "versions drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessW, sessReq)
	if sessW.Code != http.StatusCreated {
		t.Fatalf("expected 201 for session, got %d", sessW.Code)
	}
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessResp.ID+"/campaign/versions?limit=10", nil)
	getW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 for versions, got %d", getW.Code)
	}
	var out struct {
		Versions []struct {
			Version int64  `json:"version"`
			Trigger string `json:"trigger"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(getW.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Versions) != 1 || out.Versions[0].Trigger != "session-create" {
		t.Errorf("expected single session-create version, got %+v", out.Versions)
	}

	// Unknown session: empty list, never an error shell.
	missReq := httptest.NewRequest(http.MethodGet, "/api/sessions/missing/campaign/versions", nil)
	missW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missW, missReq)
	if missW.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown session versions, got %d", missW.Code)
	}
}
