package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
		os.Remove(dbPath + ".lock")
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
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	tenant := "tenant-pqc-1"
	jobHash := "job-hash-1"
	budget := 15.00
	expiry := time.Now().Add(1 * time.Hour).UTC()

	// W0.12: the mandate key must be the tenant admin key — register the
	// tenant with this key or verification refuses (self-signed spend).
	regBody, _ := json.Marshal(map[string]any{"id": tenant})
	regReq := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(regBody))
	regW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(regW, regReq)
	if regW.Code != http.StatusCreated {
		t.Fatalf("register tenant: %d", regW.Code)
	}
	if _, err := srv.TenantManager().SetAdminKey(tenant, pubHex, nil); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

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
	var vResp struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(vW.Body).Decode(&vResp); err != nil || !vResp.Valid {
		t.Fatalf("admin-key mandate must verify valid: %+v %v", vResp, err)
	}

	// W0.12: a mandate signed by a FOREIGN key for the same tenant refuses
	// (self-signed spend) even though the crypto is valid.
	_, evilPriv, _ := ed25519.GenerateKey(nil)
	evil, err := mandates.SignMandate(evilPriv, tenant, jobHash, budget, expiry)
	if err != nil {
		t.Fatalf("sign evil mandate: %v", err)
	}
	evilHybrid, err := mandates.EncapsulateMandate(srv.PQCKeypair().EncapsulationKey, evil)
	if err != nil {
		t.Fatalf("encapsulate evil: %v", err)
	}
	evilBody, _ := json.Marshal(evilHybrid)
	evilReq := httptest.NewRequest(http.MethodPost, "/api/mandates/pqc-verify", bytes.NewReader(evilBody))
	evilW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(evilW, evilReq)
	var evilResp struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(evilW.Body).Decode(&evilResp); err != nil || evilResp.Valid {
		t.Fatalf("foreign-key mandate must refuse: %+v %v", evilResp, err)
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

	// R8: empty and whitespace-only QASM refuse with 400 — compiling
	// nothing must never come back looking like a result.
	for _, empty := range []string{`{"qasm":""}`, `{"qasm":"  \n\t "}`, `{}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/qir/compile", bytes.NewReader([]byte(empty)))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("empty QASM %q must 400, got %d", empty, w.Code)
		}
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
	ch, err := pm.GenerateChallenge("USDC", "0.25")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}

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
	// W2.1: HF routes are operator-gated (external fetch + disk writes).
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	authed := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer op-token")
	}

	// 1. GET /api/hf/search
	req1 := httptest.NewRequest(http.MethodGet, "/api/hf/search?q=qwen&library=gguf&limit=5", nil)
	authed(req1)
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
	authed(req2)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// 3. POST /api/hf/download
	dBody := []byte(`{"repo_id":"Qwen/Qwen2.5-Coder-7B-Instruct-GGUF","filename":"model.gguf"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/api/hf/download", bytes.NewReader(dBody))
	authed(req3)
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}

	// 4. GET /api/hf/downloads
	req4 := httptest.NewRequest(http.MethodGet, "/api/hf/downloads", nil)
	authed(req4)
	w4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w4, req4)

	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w4.Code)
	}
}

func TestCustomAgentAndMeshRouteEndpoints(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	// K3: registry writes are operator mutations.
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	authed := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer op-token")
	}

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
	authed(req2)
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
	if !rResp.OK {
		t.Fatalf("route envelope must be ok:true, got %+v", rResp)
	}
	// No model configured anywhere: the router refuses instead of
	// inventing a default. ok:false lives on the decision, not the envelope.
	if rResp.Decision["ok"] != false {
		t.Fatalf("unconfigured routing must refuse, got %+v", rResp.Decision)
	}
	if _, ok := rResp.Decision["explanation"].(string); !ok {
		t.Fatalf("refusal must explain, got %+v", rResp.Decision)
	}

	// 3b. Explicit model routes with the model echoed, never substituted.
	routeBodyNamed := []byte(`{
		"prompt": "Optimize VQE ansatz for FeMoCo",
		"items": [{"modality": "molecule_3d", "data": "H2"}],
		"preferred_model": "local/test-model",
		"spend_budget_usd": 5.0
	}`)
	req3b := httptest.NewRequest(http.MethodPost, "/api/mesh/route", bytes.NewReader(routeBodyNamed))
	w3b := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3b, req3b)
	if w3b.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3b.Code)
	}
	var rResp2 struct {
		OK       bool                   `json:"ok"`
		Decision map[string]interface{} `json:"decision"`
	}
	if err := json.NewDecoder(w3b.Body).Decode(&rResp2); err != nil {
		t.Fatalf("failed to decode mesh route resp: %v", err)
	}
	if rResp2.Decision["ok"] != true || rResp2.Decision["model"] != "local/test-model" {
		t.Fatalf("explicit model must route verbatim, got %+v", rResp2.Decision)
	}
}

func TestLedgerAppendIssuesServerReceipt(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	// W2.1: ledger append is an operator-gated write (provenance forgery
	// rail) — bearer required; unauthenticated forging refuses.
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	authedPost := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/ledger/append", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer op-token")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	// Bearerless forgery refuses before any allowlist check.
	anonReq := httptest.NewRequest(http.MethodPost, "/api/ledger/append", bytes.NewReader([]byte(`{"session_id":"x","event_type":"tribunal_commit","payload":{}}`)))
	anonW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(anonW, anonReq)
	if anonW.Code != http.StatusUnauthorized {
		t.Fatalf("bearerless ledger append must 401, got %d", anonW.Code)
	}

	body, _ := json.Marshal(map[string]any{
		"session_id": "sess-tribunal",
		"event_type": "tribunal_commit",
		"payload":    map[string]any{"hypothesis_id": "H-1", "quorum_percent": 95.2},
	})
	w := authedPost(body)

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
	w2 := authedPost(body2)
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
	badW := authedPost(badBody)
	if badW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-allowlisted event, got %d", badW.Code)
	}
}

func TestSupervisorAbortSettlesJobsFailed(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	// W2.1: abort is an operator kill-switch — bearer required.
	t.Setenv("KUBONA_API_TOKEN", "op-token")

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
	// Bearerless kill-switch refuses before touching jobs.
	anonAbort := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abortBody))
	anonW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(anonW, anonAbort)
	if anonW.Code != http.StatusUnauthorized {
		t.Fatalf("bearerless abort must 401, got %d", anonW.Code)
	}
	abortReq := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abortBody))
	abortReq.Header.Set("Authorization", "Bearer op-token")
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
	// R4: tenant mutations sit behind the bearer rail.
	t.Setenv("KUBONA_API_TOKEN", "op-token")

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
	req.Header.Set("Authorization", "Bearer op-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin bootstrap, got %d: %s", w.Code, w.Body.String())
	}

	// Grant L2 with a bound envelope.
	grantAct := signTenantAction(t, priv, pubHex, "lab-route", "grant_autonomy", map[string]string{"level": "L2"})
	grantBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "level": "L2", "action": grantAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/autonomy", bytes.NewReader(grantBody))
	req.Header.Set("Authorization", "Bearer op-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for grant, got %d: %s", w.Code, w.Body.String())
	}

	// Mismatched params must refuse (envelope binds different level).
	badAct := signTenantAction(t, priv, pubHex, "lab-route", "grant_autonomy", map[string]string{"level": "L3"})
	badBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "level": "L2", "action": badAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/autonomy", bytes.NewReader(badBody))
	req.Header.Set("Authorization", "Bearer op-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for param-mismatched envelope, got %d", w.Code)
	}

	// Revoke drops to L0.
	revAct := signTenantAction(t, priv, pubHex, "lab-route", "revoke_autonomy", map[string]string{"reason": "drill"})
	revBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-route", "reason": "drill", "action": revAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/revoke", bytes.NewReader(revBody))
	req.Header.Set("Authorization", "Bearer op-token")
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
	// R4: tenant mutations + verify sit behind the bearer rail.
	t.Setenv("KUBONA_API_TOKEN", "op-token")

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
	req.Header.Set("Authorization", "Bearer op-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d", w.Code)
	}
	ceilAct := signTenantAction(t, priv, pubHex, "lab-cap", "set_ceiling", map[string]float64{"ceiling_usd": 40})
	ceilBody, _ := json.Marshal(map[string]any{"tenant_id": "lab-cap", "ceiling_usd": 40.0, "action": ceilAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/ceiling", bytes.NewReader(ceilBody))
	req.Header.Set("Authorization", "Bearer op-token")
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
		r.Header.Set("Authorization", "Bearer op-token")
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
	// R4: tenant mutations sit behind the bearer rail.
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	// Bootstrap default-tenant admin, then revoke the lab to L0.
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	adminBody, _ := json.Marshal(map[string]any{"tenant_id": "default", "new_admin_pubkey_hex": pubHex})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(adminBody))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}
	revAct := signTenantAction(t, priv, pubHex, "default", "revoke_autonomy", map[string]string{"reason": "drill"})
	revBody, _ := json.Marshal(map[string]any{"tenant_id": "default", "reason": "drill", "action": revAct})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/revoke", bytes.NewReader(revBody))
	req.Header.Set("Authorization", "Bearer op-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}

	// Dispatch must now refuse at the edge with 403 — against an explicit
	// default-tenant session, proving the refusal is the autonomy rule
	// (G7: missing ids 404 before autonomy is even consulted).
	sessBody, _ := json.Marshal(map[string]any{"title": "l0 drill"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var l0sess struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&l0sess); err != nil || l0sess.ID == "" {
		t.Fatalf("session setup: %v", err)
	}
	jobBody, _ := json.Marshal(map[string]any{"session_id": l0sess.ID, "qasm": "OPENQASM 2.0;\\nqreg q[1];\\ncreg c[1];\\n", "shots": 100})
	jobReq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jobBody))
	jobW := httptest.NewRecorder()
	h.ServeHTTP(jobW, jobReq)
	if jobW.Code != http.StatusForbidden {
		t.Fatalf("L0 tenant dispatch must 403, got %d", jobW.Code)
	}

	// And a missing session id 404s (no unattributed dispatch).
	jobBody, _ = json.Marshal(map[string]any{"qasm": "OPENQASM 2.0;\\nqreg q[1];\\ncreg c[1];\\n", "shots": 100})
	jobReq = httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jobBody))
	jobW = httptest.NewRecorder()
	h.ServeHTTP(jobW, jobReq)
	if jobW.Code != http.StatusNotFound {
		t.Fatalf("dispatch without session_id must 404, got %d", jobW.Code)
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
		// W0.5: delegation names its scope + budget (default-deny roots).
		params, _ := json.Marshal(map[string]any{
			"task_type": taskType, "payload": json.RawMessage(payload),
			"scopes": []string{taskType}, "max_budget_usd": 5.0,
		})
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
	// R4: grants mutations sit behind the bearer rail.
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	post := func(path string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer op-token")
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
		"backend": "ibm_brisbane", "shots": 100, "budget_usd": 25.0, "grant_chain": chain,
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

// W0.8: leaf revocation is surgical end-to-end — revoking the exact
// envelope kills chains containing it while the root-only chain still
// admits (the grantee's other delegations keep working).
func TestGrantLeafRevokeSurgical(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	post := func(path string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer op-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}
	_, privA, _ := ed25519.GenerateKey(nil)
	pubAHex := hex.EncodeToString(privA.Public().(ed25519.PublicKey))
	_, privB, _ := ed25519.GenerateKey(nil)
	pubBHex := hex.EncodeToString(privB.Public().(ed25519.PublicKey))
	for _, lab := range []string{"lab-leaf-grantor", "lab-leaf-grantee"} {
		if code, _ := post("/api/tenants", map[string]any{"id": lab}); code != http.StatusCreated {
			t.Fatalf("register %s: %d", lab, code)
		}
	}
	mkAdmin := func(lab, pub string) {
		t.Helper()
		if code, _ := post("/api/tenants/admin-key", map[string]any{"tenant_id": lab, "new_admin_pubkey_hex": pub}); code != http.StatusOK {
			t.Fatalf("bootstrap %s: %d", lab, code)
		}
	}
	mkAdmin("lab-leaf-grantor", pubAHex)
	mkAdmin("lab-leaf-grantee", pubBHex)
	didB, err := grants.DIDForAdminPubkey(pubBHex)
	if err != nil {
		t.Fatalf("did: %v", err)
	}
	_, agentPriv, _ := ed25519.GenerateKey(nil)
	didAg, err := grants.DIDForAdminPubkey(hex.EncodeToString(agentPriv.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatalf("did: %v", err)
	}
	rootSealed, _, err := grants.MintGrant(
		hex.EncodeToString(privA), didB, "lab-leaf-grantor", "lab-leaf-grantee", "qpu/submit",
		[]string{grants.ScopeQPUSubmit, grants.ScopeRead}, 60.0, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// Grantee narrows to the agent leaf (issuer == parent audience).
	leafSealed, _, err := grants.AttenuateGrant(rootSealed, hex.EncodeToString(privB), didAg,
		"qpu/submit", []string{grants.ScopeQPUSubmit}, 25.0, 10*time.Minute)
	if err != nil {
		t.Fatalf("attenuate: %v", err)
	}
	leafChain := grants.EncodeChainB64([][]byte{rootSealed, leafSealed})
	code, reg := post("/api/grants/register", map[string]any{"chain": leafChain})
	if code != http.StatusOK || reg["ok"] != true {
		t.Fatalf("register leaf chain: %d %+v", code, reg)
	}
	leafCID, _ := reg["leaf_cid"].(string)
	if leafCID == "" {
		t.Fatalf("no leaf cid: %+v", reg)
	}
	sessBody, _ := json.Marshal(map[string]any{"title": "leaf drill", "tenant_id": "lab-leaf-grantee"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatalf("session: %v", err)
	}
	leafJob := map[string]any{"session_id": sessResp.ID, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n",
		"backend": "ibm_brisbane", "shots": 100, "budget_usd": 10.0, "grant_chain": leafChain,
		"caller_jurisdictions": []string{"US"}}
	if code, _ := post("/api/jobs", leafJob); code != http.StatusCreated {
		t.Fatalf("leaf chain must admit before revocation, got %d", code)
	}
	// Revoke by leaf CID; the exact envelope dies...
	leafAct := signTenantAction(t, privA, pubAHex, "lab-leaf-grantor", "revoke_grant",
		map[string]string{"leaf_cid": leafCID, "reason": "leaf drill"})
	if code, _ := post("/api/grants/revoke", map[string]any{"leaf_cid": leafCID, "reason": "leaf drill", "action": leafAct}); code != http.StatusOK {
		t.Fatalf("leaf revoke: %d", code)
	}
	if code, _ := post("/api/jobs", leafJob); code == http.StatusCreated {
		t.Fatalf("leaf-revoked chain must refuse dispatch")
	}
	// ...while the root-only chain (no revoked envelope inside) still admits.
	rootJob := map[string]any{"session_id": sessResp.ID, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n",
		"backend": "ibm_brisbane", "shots": 100, "budget_usd": 10.0,
		"grant_chain":          grants.EncodeChainB64([][]byte{rootSealed}),
		"caller_jurisdictions": []string{"US"}}
	if code, _ := post("/api/jobs", rootJob); code != http.StatusCreated {
		t.Fatalf("root-only chain must survive leaf revocation, got %d", code)
	}
	// Both CIDs at once is not an operation.
	bothAct := signTenantAction(t, privA, pubAHex, "lab-leaf-grantor", "revoke_grant",
		map[string]string{"root_cid": "x", "leaf_cid": leafCID, "reason": "both"})
	if code, _ := post("/api/grants/revoke", map[string]any{"root_cid": "x", "leaf_cid": leafCID, "reason": "both", "action": bothAct}); code != http.StatusBadRequest {
		t.Fatalf("dual-CID revoke must 400, got %d", code)
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

// K1: exec-backed chemistry endpoints label demo defaults and name their
// interpreter; failures are 503 with cause, never silent. RDKit is absent
// in CI (chemistry extra not installed) — skip with reason there, assert
// fully where the backend exists.
func rdkitAvailable() bool {
	// Test CWD is pkg/api: ../../../.venv is platform/.venv.
	for _, py := range []string{"../../../.venv/bin/python3", "python3"} {
		cmd := exec.Command(py, "-c", "import rdkit")
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func TestChemistryADMETDefaultLabeled(t *testing.T) {
	if !rdkitAvailable() {
		t.Skip("rdkit absent: ADMET backend cannot run here")
	}
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// No SMILES: Aspirin default, labeled + interpreter named.
	req := httptest.NewRequest(http.MethodGet, "/api/chemistry/admet", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("default ADMET must answer 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["defaulted_smiles"] != true {
		t.Errorf("default SMILES must be labeled, got %v", out["defaulted_smiles"])
	}
	if _, ok := out["evaluated_by"].(string); !ok {
		t.Errorf("response must name the interpreter, got %v", out["evaluated_by"])
	}
	if out["molecular_weight"] == nil {
		t.Errorf("default evaluation must still compute, got %v", out)
	}

	// Explicit SMILES: same shape, not marked defaulted.
	req = httptest.NewRequest(http.MethodGet, "/api/chemistry/admet?smiles=CCO", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("explicit ADMET must answer 200, got %d: %s", w.Code, w.Body.String())
	}
	out = nil
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["defaulted_smiles"] != false {
		t.Errorf("explicit SMILES must not be marked defaulted, got %v", out["defaulted_smiles"])
	}
}

func TestDiscoveryPipelineDefaultsLabeled(t *testing.T) {
	if !rdkitAvailable() {
		t.Skip("rdkit absent: discovery backend cannot run here")
	}
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	post := func(body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/discovery/pipeline", bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}

	// Empty body: demo target + candidates, both labeled.
	code, out := post(map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("default pipeline must answer 200, got %d: %v", code, out)
	}
	if out["defaulted_target"] != true || out["defaulted_candidates"] != true {
		t.Errorf("demo defaults must be labeled, got target=%v candidates=%v",
			out["defaulted_target"], out["defaulted_candidates"])
	}
	if out["total_evaluated"] != float64(3) {
		t.Errorf("demo set must evaluate 3 candidates, got %v", out["total_evaluated"])
	}

	// Explicit target + candidates: computed, unmarked.
	code, out = post(map[string]any{
		"target":     "test-target",
		"candidates": []any{map[string]any{"name": "Ethanol", "smiles": "CCO"}},
	})
	if code != http.StatusOK {
		t.Fatalf("explicit pipeline must answer 200, got %d: %v", code, out)
	}
	if out["defaulted_target"] != false || out["defaulted_candidates"] != false {
		t.Errorf("explicit inputs must not be marked defaulted, got %v", out)
	}
	if out["total_evaluated"] != float64(1) {
		t.Errorf("must evaluate exactly the 1 given candidate, got %v", out)
	}
}

// K2: GET /api/loops accepts session_id scope. Unfiltered keeps the
// global view; filtered counts only that session's loops (the console
// header was counting the fleet's loops as the session's).
func TestLoopsSessionScope(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	mkLoop := func(sess string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"session_id": sess, "max_steps": 5})
		req := httptest.NewRequest(http.MethodPost, "/api/loops", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("register loop in %s: %d", sess, w.Code)
		}
	}
	mkLoop("sess-A")
	mkLoop("sess-A")
	mkLoop("sess-B")

	get := func(qs string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/loops"+qs, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out struct {
			Loops []any `json:"loops"`
		}
		_ = json.NewDecoder(w.Body).Decode(&out)
		return len(out.Loops)
	}
	if n := get(""); n != 3 {
		t.Errorf("unfiltered must list all loops, got %d", n)
	}
	if n := get("?session_id=sess-A"); n != 2 {
		t.Errorf("sess-A scope must list 2, got %d", n)
	}
	if n := get("?session_id=sess-B"); n != 1 {
		t.Errorf("sess-B scope must list 1, got %d", n)
	}
	if n := get("?session_id=ghost"); n != 0 {
		t.Errorf("unknown session must list 0, got %d", n)
	}
}

// K3: the custom-agent registry validates shape, protects seeds, persists
// across restarts, and gates writes on the operator token.
func TestCustomAgentRegistryGuards(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	h := srv.Handler()

	call := func(method, path, token string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		var req *http.Request
		if body == nil {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.RemoteAddr = "127.0.0.1:0"
		req.Header.Set("X-Forwarded-For", nextTestIP())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}
	good := map[string]any{"name": "Analyst", "role": "researcher", "spend_budget_usd": 1.0}

	// Writes need the token; listing stays public.
	if code, _ := call(http.MethodPost, "/api/agents/custom", "", good); code != http.StatusUnauthorized {
		t.Errorf("register without token must 401, got %d", code)
	}
	if code, _ := call(http.MethodDelete, "/api/agents/custom/agent_qksd_theorist", "", nil); code != http.StatusUnauthorized {
		t.Errorf("delete without token must 401, got %d", code)
	}
	if code, _ := call(http.MethodGet, "/api/agents/custom", "", nil); code != http.StatusOK {
		t.Errorf("listing must stay public, got %d", code)
	}

	// Shape validation refuses loudly.
	for name, body := range map[string]map[string]any{
		"empty name":     {"role": "x"},
		"negative spend": {"name": "n", "spend_budget_usd": -5.0},
		"system forgery": {"name": "n", "is_system_agent": true},
		"seed overwrite": {"id": "agent_qksd_theorist", "name": "Trojan"},
	} {
		if code, out := call(http.MethodPost, "/api/agents/custom", "op-token", body); code != http.StatusBadRequest {
			t.Errorf("%s must 400, got %d (%v)", name, code, out)
		}
	}

	// System seeds cannot be deleted, even by the operator.
	if code, out := call(http.MethodDelete, "/api/agents/custom/agent_qksd_theorist", "op-token", nil); code != http.StatusBadRequest {
		t.Errorf("seed delete must 400, got %d (%v)", code, out)
	}

	// Round trip: register → delete a user agent.
	code, out := call(http.MethodPost, "/api/agents/custom", "op-token", good)
	if code != http.StatusOK {
		t.Fatalf("register: %d (%v)", code, out)
	}
	id, _ := out["agent"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("registered agent must carry an id: %v", out)
	}
	delPath := "/api/agents/custom/" + id
	if code, _ := call(http.MethodDelete, delPath, "op-token", nil); code != http.StatusOK {
		t.Fatalf("delete user agent: %d", code)
	}
}

// Tribunal refuses unknown session ids instead of attaching turns,
// records and spend to a stranger session (demonstrated live: a probe
// id wrote 8 turns + claim records into an unrelated validation
// session). Empty ids still bootstrap for first-run UX.
func TestTribunalUnknownSessionRefuses(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	bad, _ := json.Marshal(map[string]any{"session_id": "ghost-session", "prompt": "H2"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tribunal", bytes.NewReader(bad))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown session tribunal must 404, got %d", w.Code)
	}
}

// Q-33: suspended is a reachable state, not a dead one. Admin-signed
// suspend → spend and dispatch refuse naming suspension; reactivate
// restores; invalid statuses and foreign keys refuse; bearer required.
func TestTenantSuspendLifecycle(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	call := func(method, path, token string, body any) (int, string) {
		t.Helper()
		var req *http.Request
		if body == nil {
			req = httptest.NewRequest(method, path, nil)
		} else {
			raw, _ := json.Marshal(body)
			req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.RemoteAddr = "127.0.0.1:0"
		req.Header.Set("X-Forwarded-For", nextTestIP())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	// Bootstrap lab + admin.
	if code, _ := call(http.MethodPost, "/api/tenants", "", map[string]any{"id": "lab-susp"}); code != http.StatusCreated {
		t.Fatalf("register: %d", code)
	}
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	setKey := func() (int, string) {
		return call(http.MethodPost, "/api/tenants/admin-key", "op-token", map[string]any{
			"tenant_id": "lab-susp", "new_admin_pubkey_hex": pubHex})
	}
	if code, body := setKey(); code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", code, body)
	}
	suspend := func(status, token string, key []byte, pub string) (int, string) {
		act := signTenantAction(t, key, pub, "lab-susp", "set_status", map[string]string{"status": status})
		return call(http.MethodPost, "/api/tenants/status", token, map[string]any{
			"tenant_id": "lab-susp", "status": status, "action": act})
	}

	// Bearer gate + value gate + key gate.
	if code, _ := suspend("suspended", "", priv, pubHex); code != http.StatusUnauthorized {
		t.Errorf("no token must 401, got %d", code)
	}
	if code, body := suspend("frozen", "op-token", priv, pubHex); code != http.StatusForbidden {
		t.Errorf("unknown status must 403, got %d %s", code, body)
	}
	_, priv2, _ := ed25519.GenerateKey(nil)
	pub2 := hex.EncodeToString(priv2.Public().(ed25519.PublicKey))
	if code, body := suspend("suspended", "op-token", priv2, pub2); code != http.StatusForbidden {
		t.Errorf("foreign admin must 403, got %d %s", code, body)
	}

	// Suspend: dispatch refuses naming suspension.
	if code, body := suspend("suspended", "op-token", priv, pubHex); code != http.StatusOK {
		t.Fatalf("suspend: %d %s", code, body)
	}
	jobBody := map[string]any{
		"session_id": "", "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
		"backend": "local_simulator", "shots": 100,
	}
	_ = jobBody
	sessBody, _ := json.Marshal(map[string]any{"title": "susp drill", "tenant_id": "lab-susp"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil || sessResp.ID == "" {
		t.Fatalf("session setup: %v", err)
	}
	jb, _ := json.Marshal(map[string]any{
		"session_id": sessResp.ID,
		"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
		"backend":    "local_simulator", "shots": 100,
	})
	jreq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jb))
	jw := httptest.NewRecorder()
	h.ServeHTTP(jw, jreq)
	if jw.Code != http.StatusForbidden || !strings.Contains(strings.ToLower(jw.Body.String()), "suspend") {
		t.Errorf("suspended dispatch must 403 naming suspension, got %d %q", jw.Code, jw.Body.String())
	}

	// Reactivate: dispatch works again.
	if code, body := suspend("active", "op-token", priv, pubHex); code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", code, body)
	}
	jreq2 := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(jb))
	jw2 := httptest.NewRecorder()
	h.ServeHTTP(jw2, jreq2)
	if jw2.Code != http.StatusCreated {
		t.Errorf("reactivated dispatch must proceed, got %d %s", jw2.Code, jw2.Body.String())
	}
}

// W0.3: tribunal + autoresearch-start enforce tenant autonomy like
// launch/jobs — L0 and suspended labs 403 with the lab named, before any
// LLM call or loop is built.
func TestTribunalAutoresearchEnforceAutonomy(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()

	if code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-w03"}); code != http.StatusCreated {
		t.Fatalf("register: %d", code)
	}
	code, body, _ := idemPost(t, h, "/api/sessions", map[string]any{"title": "w03 drill", "tenant_id": "lab-w03"})
	if code != http.StatusCreated {
		t.Fatalf("session: %d %s", code, body)
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &sess); err != nil || sess.ID == "" {
		t.Fatalf("session decode: %v %s", body, err)
	}
	tribBody := map[string]any{"session_id": sess.ID, "prompt": "H2"}
	arBody := map[string]any{"session_id": sess.ID, "harness_id": "ansatz-search", "max_iterations": 1}

	// Suspended lab: both refuse naming the lab, before any spend.
	if _, err := l.DB().Exec(`UPDATE tenants SET status = 'suspended' WHERE id = 'lab-w03'`); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if code, body, _ := idemPost(t, h, "/api/chat/tribunal", tribBody); code != http.StatusForbidden || !strings.Contains(strings.ToLower(body), "lab-w03") {
		t.Errorf("suspended tribunal must 403 naming the lab, got %d %q", code, body)
	}
	if code, body, _ := idemPost(t, h, "/api/autoresearch/start", arBody); code != http.StatusForbidden || !strings.Contains(strings.ToLower(body), "lab-w03") {
		t.Errorf("suspended autoresearch must 403 naming the lab, got %d %q", code, body)
	}

	// L0 lab: same refusal naming autonomy.
	if _, err := l.DB().Exec(`UPDATE tenants SET status = 'active', autonomy_level = 'L0' WHERE id = 'lab-w03'`); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if code, body, _ := idemPost(t, h, "/api/chat/tribunal", tribBody); code != http.StatusForbidden || !strings.Contains(strings.ToLower(body), "l0") {
		t.Errorf("L0 tribunal must 403 naming autonomy, got %d %q", code, body)
	}
	if code, body, _ := idemPost(t, h, "/api/autoresearch/start", arBody); code != http.StatusForbidden || !strings.Contains(strings.ToLower(body), "l0") {
		t.Errorf("L0 autoresearch must 403 naming autonomy, got %d %q", code, body)
	}
}

// Q-33 recovery: a suspended lab's admin keeps reactivation + key
// rotation (kill-switch without recovery is a brick). Everything else
// stays refused while suspended.
func TestTenantSuspendedRecoveryPaths(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer op-token")
		req.RemoteAddr = "127.0.0.1:0"
		req.Header.Set("X-Forwarded-For", nextTestIP())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		rawBody := w.Body.String()
		out := map[string]any{}
		_ = json.Unmarshal([]byte(rawBody), &out)
		out["_raw"] = rawBody
		return w.Code, out
	}
	if code, _ := call(http.MethodPost, "/api/tenants", map[string]any{"id": "lab-rec"}); code != http.StatusCreated {
		t.Fatalf("register: %d", code)
	}
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	signed := func(action string, params any) tenants.AdminAction {
		return signTenantAction(t, priv, pubHex, "lab-rec", action, params)
	}
	if code, _ := call(http.MethodPost, "/api/tenants/admin-key", map[string]any{
		"tenant_id": "lab-rec", "new_admin_pubkey_hex": pubHex}); code != http.StatusOK {
		t.Fatalf("bootstrap: %d", code)
	}
	suspAct := signed("set_status", map[string]string{"status": "suspended"})
	if code, _ := call(http.MethodPost, "/api/tenants/status", map[string]any{
		"tenant_id": "lab-rec", "status": "suspended", "action": suspAct}); code != http.StatusOK {
		t.Fatalf("suspend: %d", code)
	}
	// Key rotation works while suspended (post-compromise recovery).
	_, priv2, _ := ed25519.GenerateKey(nil)
	pub2 := hex.EncodeToString(priv2.Public().(ed25519.PublicKey))
	rotAct := signed("set_admin_key", map[string]string{"new_admin_pubkey_hex": pub2})
	if code, body := call(http.MethodPost, "/api/tenants/admin-key", map[string]any{
		"tenant_id": "lab-rec", "new_admin_pubkey_hex": pub2, "action": rotAct}); code != http.StatusOK {
		t.Errorf("suspended key rotation must work, got %d %v", code, body)
	}
	// But ceiling changes stay refused while suspended — signed with the
	// CURRENT (rotated) key so the refusal proves suspension, not key mismatch.
	ceilAct2 := signTenantAction(t, priv2, pub2, "lab-rec", "set_ceiling", map[string]float64{"ceiling_usd": 10})
	if code, body := call(http.MethodPost, "/api/tenants/ceiling", map[string]any{
		"tenant_id": "lab-rec", "ceiling_usd": 10.0, "action": ceilAct2}); code != http.StatusForbidden {
		t.Errorf("suspended ceiling change must 403, got %d %v", code, body)
	} else if !strings.Contains(strings.ToLower(fmt.Sprint(body["_raw"])), "suspend") {
		t.Errorf("refusal must name suspension, got %v", body)
	}
}

// G7 session-attribution strictness. Threat, not consistency: unattributed
// spend bills another lab, and adopting a stranger session writes one
// caller's content into another session's tenant recall scope. So spend
// entries 404 missing/unknown ids; content writes 404 unknown ids and mint
// fresh sessions for empty ones; reads 404 unknown ids without adopting.
func TestSessionAttributionStrictness(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	mkSession := func(title string) string {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"title": title})
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("session setup %q: %d", title, w.Code)
		}
		var s struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(w.Body).Decode(&s)
		return s.ID
	}
	post := func(path string, body any) (int, string) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer op-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	get := func(path string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer op-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	withID := func(base map[string]any, id string) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		if id != "" {
			out["session_id"] = id
		}
		return out
	}

	anchorID := mkSession("g7 anchor")

	// Spend entries: missing AND unknown session ids 404 (before autonomy,
	// before any worker sees the job).
	spendBases := map[string]map[string]any{
		"/api/jobs":               {"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "shots": 8},
		"/api/chat/launch":        {"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"},
		"/api/chat/tribunal":      {"prompt": "g7 drill"},
		"/api/autoresearch/start": {"harness_id": "ansatz-search", "max_iterations": 1},
	}
	for path, base := range spendBases {
		if code, body := post(path, withID(base, "")); code != http.StatusNotFound {
			t.Errorf("%s missing session must 404, got %d %s", path, code, body)
		}
		if code, body := post(path, withID(base, "sess-does-not-exist")); code != http.StatusNotFound {
			t.Errorf("%s unknown session must 404, got %d %s", path, code, body)
		} else if !strings.Contains(strings.ToLower(body), "session") {
			t.Errorf("%s refusal must name the session problem, got %q", path, body)
		}
	}
	// A real session still dispatches (201 queued, no workers in tests) —
	// strictness refuses ghosts, not real attribution.
	if code, body := post("/api/jobs", withID(spendBases["/api/jobs"], anchorID)); code != http.StatusCreated {
		t.Errorf("jobs with a real session must 201, got %d %s", code, body)
	}

	// Content writes: unknown explicit id 404s (never adopt a stranger).
	if code, body := post("/api/chat/messages", map[string]any{"session_id": "sess-does-not-exist", "content": "hi"}); code != http.StatusNotFound {
		t.Errorf("chat POST unknown session must 404, got %d %s", code, body)
	} else if !strings.Contains(strings.ToLower(body), "session") {
		t.Errorf("chat refusal must name the session problem, got %q", body)
	}

	// Reads: unknown ids 404 without adopting; empty chat reads [].
	if code, _ := get("/api/chat/messages?session_id=sess-does-not-exist"); code != http.StatusNotFound {
		t.Errorf("chat GET unknown session must 404, got %d", code)
	}
	if code, body := get("/api/chat/messages"); code != http.StatusOK {
		t.Errorf("chat GET empty session must 200 [], got %d %s", code, body)
	}
	if code, _ := get("/api/notebooks?session_id=sess-does-not-exist"); code != http.StatusNotFound {
		t.Errorf("notebooks unknown session must 404, got %d", code)
	}
	if code, _ := get("/api/epistemics?session_id=sess-does-not-exist"); code != http.StatusNotFound {
		t.Errorf("epistemics unknown session must 404, got %d", code)
	}
	// W2.2: unscoped epistemics never adopts sessions[0] (cross-session
	// leak) — missing ids 404 like unknown ones.
	if code, _ := get("/api/epistemics"); code != http.StatusNotFound {
		t.Errorf("epistemics empty session must 404, got %d", code)
	}
}

// W0.19: LLM settings writes get edge auth (reads stay public); provider
// URLs validate destinations (no plaintext exfiltration endpoints).
func TestLLMSettingsWriteGated(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	// Read stays public.
	req := httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("settings read must stay public, got %d", w.Code)
	}
	put := func(token string, body any) int {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPut, "/api/llm/settings", bytes.NewReader(raw))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		ww := httptest.NewRecorder()
		h.ServeHTTP(ww, r)
		return ww.Code
	}
	if code := put("", map[string]any{"temperature": 0.5}); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated settings write must 401, got %d", code)
	}
	// Authenticated benign write succeeds (positive direction).
	if code := put("op-token", map[string]any{"temperature": 0.5}); code != http.StatusOK {
		t.Fatalf("authenticated settings write must succeed, got %d", code)
	}
	// Plaintext remote endpoint refuses even when authenticated.
	if code := put("op-token", map[string]any{
		"providers": map[string]any{
			"opencode": map[string]any{"base_url": "http://evil.example.com/v1"},
		},
	}); code != http.StatusBadRequest {
		t.Fatalf("plaintext remote endpoint must 400, got %d", code)
	}
}
