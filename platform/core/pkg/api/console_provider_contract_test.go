package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/hf"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

// providerContractBridge is a test-only bridge double. It implements the
// provider-facing RPCs so the real HTTP handlers, stores, and contract
// validator run without a network provider. Core execution is covered by the
// live stack E2E; this gate pins the handler envelope for provider routes.
type providerContractBridge struct {
	pb.QuantumBridgeServiceClient
	sessionID string
}

func (p *providerContractBridge) ComputeClassicalAnchor(context.Context, *pb.ClassicalAnchorRequest, ...grpc.CallOption) (*pb.ClassicalAnchorResponse, error) {
	return &pb.ClassicalAnchorResponse{
		Molecule: "H2", BondDistance: 0.7414, RhfEnergy: -1.11668,
		FciExactEnergy: -1.13727, Success: true,
	}, nil
}

func (p *providerContractBridge) ExecuteCircuit(context.Context, *pb.CircuitExecutionRequest, ...grpc.CallOption) (*pb.CircuitExecutionResponse, error) {
	return &pb.CircuitExecutionResponse{
		JobId: "job-contract", Success: true, ResolvedBackend: "local-simulator",
		Counts: map[string]int64{"00": 1}, StatevectorReal: []float64{1}, ExecutionTimeMs: 1,
	}, nil
}

func (p *providerContractBridge) EvaluateMolecularADMET(context.Context, *pb.ChemistryADMETRequest, ...grpc.CallOption) (*pb.ChemistryADMETResponse, error) {
	return &pb.ChemistryADMETResponse{Success: true, ResultJson: `{"molecular_weight":46.07,"clogp":-0.001,"tpsa":20.23}`}, nil
}

func (p *providerContractBridge) GroundLiterature(context.Context, *pb.LiteratureGroundingRequest, ...grpc.CallOption) (*pb.LiteratureGroundingResponse, error) {
	return &pb.LiteratureGroundingResponse{Records: []*pb.LiteratureRecord{{
		Id: "doi:10.1/contract", Title: "Contract literature", Authors: "Kubona",
		AbstractText: "fixture", Url: "https://example.org/contract", PublicationYear: 2026, CitationCount: 1,
	}}}, nil
}

func (p *providerContractBridge) RunCanary(context.Context, *pb.CanaryRunRequest, ...grpc.CallOption) (*pb.CanaryRunResponse, error) {
	return &pb.CanaryRunResponse{Ok: true, ResultsJson: `{"local-simulator":{"ok":true,"checks":{"total_shots":true}}}`}, nil
}

func (p *providerContractBridge) EvaluateCodesign(_ context.Context, req *pb.CodesignRequest, _ ...grpc.CallOption) (*pb.CodesignResponse, error) {
	var body string
	switch req.GetOperation() {
	case "budget":
		body = `{"ok":true,"rows":[],"union_bound":0}`
	case "spec":
		body = `{"ok":true,"spec_kind":"and2","num_qubits":3,"inputs":[0,1],"outputs":[2]}`
	case "evaluate":
		body = `{"ok":true,"spec_kind":"and2","result":{"valid":true,"toffoli":1,"peak_qubits":3,"tq":10,"reason":"ok"}}`
	case "batch":
		body = `{"ok":true,"spec_kind":"tq","frontier":{},"details":[]}`
	default:
		body = `{"ok":true}`
	}
	return &pb.CodesignResponse{Success: true, ResultJson: body}, nil
}

func (p *providerContractBridge) EvaluateDiscoveryPipeline(context.Context, *pb.DiscoveryPipelineRequest, ...grpc.CallOption) (*pb.DiscoveryPipelineResponse, error) {
	return &pb.DiscoveryPipelineResponse{Success: true, ResultJson: `{"ok":true,"target":"H2","candidates":[],"top_lead":null}`}, nil
}

func (p *providerContractBridge) QuantumSubmit(context.Context, *pb.QuantumSubmitRequest, ...grpc.CallOption) (*pb.QuantumSubmitResponse, error) {
	return &pb.QuantumSubmitResponse{
		Ok: true, JobId: "qjob-contract", Queued: true, SessionId: p.sessionID,
		EstimateJson: `{"estimated_usd":0}`, FidelityJson: `{}`,
	}, nil
}

func (p *providerContractBridge) QuantumJobStatus(context.Context, *pb.QuantumJobStatusRequest, ...grpc.CallOption) (*pb.QuantumJobStatusResponse, error) {
	return &pb.QuantumJobStatusResponse{
		Ok: true, JobId: "qjob-contract", Status: "completed", SessionId: p.sessionID,
		ResultJson: `{"result":{"counts":{"00":1},"shots":1}}`,
	}, nil
}

func (p *providerContractBridge) QuantumCancel(context.Context, *pb.QuantumCancelRequest, ...grpc.CallOption) (*pb.QuantumCancelResponse, error) {
	return &pb.QuantumCancelResponse{
		Ok: true, JobId: "qjob-contract", ProviderRecallAttempted: true, ProviderRecallStatus: "recalled",
	}, nil
}

func (p *providerContractBridge) WorkflowPlan(context.Context, *pb.WorkflowPlanRequest, ...grpc.CallOption) (*pb.WorkflowPlanResponse, error) {
	return &pb.WorkflowPlanResponse{Ok: true, WorkflowId: "wf-contract", PlannedStepsJson: `[{"id":"s1","tool":"execution/list_provider_registry","args":{}}]`}, nil
}

func (p *providerContractBridge) WorkflowRun(context.Context, *pb.WorkflowRunRequest, ...grpc.CallOption) (*pb.WorkflowRunResponse, error) {
	return &pb.WorkflowRunResponse{Ok: true, WorkflowId: "wf-contract", StepsCompleted: 1, StepRecords: []string{"s1"}, LastResultsJson: `{}`, SpentUsd: 0, CompensationsJson: `[]`}, nil
}
func (p *providerContractBridge) RunWorkflowStep(_ context.Context, in *pb.WorkflowStepRequest, _ ...grpc.CallOption) (*pb.WorkflowStepResponse, error) {
	return &pb.WorkflowStepResponse{Ok: true, StepId: "s1", ResultJson: `{"ok":true}`, SpentUsd: 0}, nil
}

func newProviderContractServer(t *testing.T) (*Server, *session.Manager, string, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "provider-contract-token")
	t.Setenv("KUBONA_LEASE_HMAC_KEY", strings.Repeat("x", 32))
	t.Setenv("KUBONA_BRIDGE_TOKEN", "provider-contract-token")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/provider.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	sm := session.NewManager(l)
	sess, err := sm.CreateSession("provider contract", 10, "")
	if err != nil {
		l.Close()
		t.Fatalf("session: %v", err)
	}
	bridge := &providerContractBridge{sessionID: sess.ID}
	sup := supervisor.NewSupervisor(l)
	srv := NewServer(l, sm, jobs.NewRunner(l, bridge, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	return srv, sm, sess.ID, func() { l.Close() }
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return string(raw)
}

func signAdminAction(t *testing.T, priv ed25519.PrivateKey, pubHex, tenantID, action string, params any) tenants.AdminAction {
	t.Helper()
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatalf("params hash: %v", err)
	}
	a := tenants.AdminAction{
		TenantID: tenantID, Action: action, ParamsHash: hash,
		Expiry: time.Now().UTC().Add(time.Hour), PubkeyHex: pubHex,
	}
	sig := ed25519.Sign(priv, a.CanonicalPayload())
	a.SignatureHex = hex.EncodeToString(sig)
	return a
}

func TestConsoleProviderContracts(t *testing.T) {
	srv, _, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	handler := srv.Handler()
	rewriteMsg, err := srv.sessionMgr.AddMessage(sessionID, "user", "rewrite me", nil)
	if err != nil {
		t.Fatalf("rewrite message: %v", err)
	}
	doc := loadConsoleOpenAPI(t)
	cases := []struct {
		method   string
		path     string
		template string
		body     string
	}{
		{http.MethodGet, "/api/literature?query=H2", "/api/literature", ""},
		{http.MethodGet, "/api/chemistry/admet?smiles=CCO", "/api/chemistry/admet", ""},
		{http.MethodGet, "/api/quantum/canary?providers=local-simulator", "/api/quantum/canary", ""},
		{http.MethodPost, "/api/quantum/jobs", "/api/quantum/jobs", `{"session_id":"` + sessionID + `","qasm":"qreg q[1]; h q[0];","provider":"local-simulator","shots":1,"idempotency_key":"provider-contract-1"}`},
		{http.MethodGet, "/api/quantum/jobs/qjob-contract", "/api/quantum/jobs/{id}", ""},
		{http.MethodPost, "/api/quantum/jobs/qjob-contract/cancel", "/api/quantum/jobs/{id}/cancel", `{"reason":"contract"}`},
		{http.MethodPost, "/api/codesign/evaluate", "/api/codesign/evaluate", `{}`},
		{http.MethodPost, "/api/codesign/evaluate-batch", "/api/codesign/evaluate-batch", `{}`},
		{http.MethodPost, "/api/codesign/families", "/api/codesign/families", `{}`},
		{http.MethodPost, "/api/codesign/budget", "/api/codesign/budget", `{}`},
		{http.MethodPost, "/api/discovery/pipeline", "/api/discovery/pipeline", `{"target":"H2","candidates":[{"name":"H2","smiles":"[H][H]"}]}`},
		{http.MethodPost, "/api/workflows/plan", "/api/workflows/plan", `{"workflow":{"name":"contract","steps":[{"id":"s1","tool":"execution/list_provider_registry","args":{}}]}}`},
		{http.MethodPost, "/api/workflows/run", "/api/workflows/run", `{"session_id":"` + sessionID + `","workflow":{"name":"contract","steps":[{"id":"s1","tool":"execution/list_provider_registry","args":{}}]}}`},
		{http.MethodGet, "/api/hf/search?q=Qwen&limit=2", "/api/hf/search", ""},
		{http.MethodGet, "/api/hf/models/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF", "/api/hf/models/{id}", ""},
		{http.MethodPost, "/api/llm/test", "/api/llm/test", `{"role":"copilot"}`},
		{http.MethodPost, "/api/chat/rewrite", "/api/chat/rewrite", `{"session_id":"` + sessionID + `","message_id":"` + rewriteMsg.ID + `","content":"rewritten","expected_content":"rewrite me"}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s %s status %d, want 200: %s", tc.method, tc.path, w.Code, strings.TrimSpace(w.Body.String()))
			continue
		}
		contract := responseContractFor(t, doc, tc.method, tc.template)
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s body is not JSON: %v", tc.method, tc.path, err)
			continue
		}
		if err := validateOpenAPISchema(doc, contract, body, "$"); err != nil {
			t.Errorf("%s %s violates %s: %v\nbody=%s", tc.method, tc.path, contract, err, w.Body.String())
		}
	}
}

func TestConsoleAuthAndTenantContracts(t *testing.T) {
	srv, _, _, cleanup := newProviderContractServer(t)
	defer cleanup()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	if _, err := srv.tenantMgr.CreateTenant("contract-auth", "Contract Auth Lab"); err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if _, err := srv.tenantMgr.SetAdminKey("contract-auth", pubHex, nil); err != nil {
		t.Fatalf("admin key: %v", err)
	}

	handler := srv.Handler()
	challenge := httptest.NewRequest(http.MethodPost, "/api/auth/challenge", strings.NewReader(`{}`))
	challenge.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	handler.ServeHTTP(cw, challenge)
	if cw.Code != http.StatusOK {
		t.Fatalf("challenge: %d %s", cw.Code, cw.Body.String())
	}
	var ch struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(cw.Body.Bytes(), &ch); err != nil || ch.Nonce == "" {
		t.Fatalf("challenge decode: %v %s", err, cw.Body.String())
	}
	sig := ed25519.Sign(priv, []byte("kubona-auth/v1 login "+ch.Nonce))
	loginBody, _ := json.Marshal(map[string]string{
		"tenant_id": "contract-auth", "pubkey_hex": pubHex,
		"nonce": ch.Nonce, "signature_hex": hex.EncodeToString(sig),
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	handler.ServeHTTP(lw, loginReq)
	if lw.Code != http.StatusOK {
		t.Fatalf("login: %d %s", lw.Code, lw.Body.String())
	}
	cookies := lw.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login set no session cookie")
	}
	cookie := cookies[0]

	doc := loadConsoleOpenAPI(t)
	var loginPayload any
	if err := json.Unmarshal(lw.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("login JSON: %v", err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/auth/login"), loginPayload, "$"); err != nil {
		t.Fatalf("auth.login contract: %v", err)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	sessionReq.AddCookie(cookie)
	sw := httptest.NewRecorder()
	handler.ServeHTTP(sw, sessionReq)
	if sw.Code != http.StatusOK {
		t.Fatalf("auth sessions: %d %s", sw.Code, sw.Body.String())
	}
	var sessionsDoc struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(sw.Body.Bytes(), &sessionsDoc); err != nil || len(sessionsDoc.Sessions) == 0 || sessionsDoc.Sessions[0].ID == "" {
		t.Fatalf("auth sessions decode: %v %s", err, sw.Body.String())
	}

	// Give the lab recorded spend so the refund rail has money to move.
	if _, err := srv.ledger.DB().Exec(`UPDATE tenants SET spent_micro_usd = 2000000 WHERE id = 'contract-auth'`); err != nil {
		t.Fatalf("seed spend: %v", err)
	}
	newPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("new ed25519: %v", err)
	}
	newPubHex := hex.EncodeToString(newPub)
	refundSession, err := srv.sessionMgr.CreateSession("contract refund", 10, "contract-auth")
	if err != nil {
		t.Fatalf("refund session: %v", err)
	}
	if err := srv.sessionMgr.ReserveSpend(refundSession.ID, 2); err != nil {
		t.Fatalf("seed session spend: %v", err)
	}
	refundSessionID := refundSession.ID
	obligation, err := srv.ledger.OpenObligation("job", "contract-obligation", nil, nil, nil)
	if err != nil {
		t.Fatalf("obligation: %v", err)
	}
	if err := srv.ledger.SetObligationOwner(obligation.ID, pubHex); err != nil {
		t.Fatalf("obligation owner: %v", err)
	}
	obligationID := obligation.ID

	if got, err := srv.ledger.GetObligation(obligationID); err != nil || got.Owner != pubHex {
		t.Fatalf("obligation owner = %q (err %v), want %q", got.Owner, err, pubHex)
	}

	if _, err := srv.tenantMgr.CreateTenant("contract-target", "Contract Target"); err != nil {
		t.Fatalf("target tenant: %v", err)
	}
	expiry := time.Now().UTC().Add(time.Hour)
	grantCanonical := tenants.GrantCanonicalPayload("contract-auth", "contract-target", []string{"read"}, expiry)
	grantSig := ed25519.Sign(priv, grantCanonical)
	grantBody := mustJSON(t, map[string]any{
		"source_tenant": "contract-auth", "target_tenant": "contract-target",
		"scopes": []string{"read"}, "expiry": expiry.Format(time.RFC3339),
		"grantor_pubkey_hex": pubHex, "signature_hex": hex.EncodeToString(grantSig),
	})
	gr := httptest.NewRequest(http.MethodPost, "/api/tenant-grants/issue", strings.NewReader(grantBody))
	gr.Header.Set("Authorization", "Bearer provider-contract-token")
	gr.Header.Set("Content-Type", "application/json")
	gw := httptest.NewRecorder()
	handler.ServeHTTP(gw, gr)
	if gw.Code != http.StatusOK {
		t.Fatalf("grant issue: %d %s", gw.Code, gw.Body.String())
	}
	var grantDoc struct {
		GrantID string `json:"grant_id"`
	}
	if err := json.Unmarshal(gw.Body.Bytes(), &grantDoc); err != nil || grantDoc.GrantID == "" {
		t.Fatalf("grant decode: %v %s", err, gw.Body.String())
	}
	var grantPayload any
	if err := json.Unmarshal(gw.Body.Bytes(), &grantPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/tenant-grants/issue"), grantPayload, "$"); err != nil {
		t.Fatalf("tenant.grant contract: %v", err)
	}

	revokeGrantSig := ed25519.Sign(priv, tenants.RevokeCanonicalPayload(grantDoc.GrantID, "contract revoke"))
	revokeGrantBody := mustJSON(t, map[string]any{
		"source_tenant": "contract-auth", "grant_id": grantDoc.GrantID, "reason": "contract revoke",
		"grantor_pubkey_hex": pubHex, "signature_hex": hex.EncodeToString(revokeGrantSig),
	})
	rr := httptest.NewRequest(http.MethodPost, "/api/tenant-grants/revoke", strings.NewReader(revokeGrantBody))
	rr.Header.Set("Authorization", "Bearer provider-contract-token")
	rr.Header.Set("Content-Type", "application/json")
	rw2 := httptest.NewRecorder()
	handler.ServeHTTP(rw2, rr)
	if rw2.Code != http.StatusOK {
		t.Fatalf("grant revoke: %d %s", rw2.Code, rw2.Body.String())
	}
	var revokeGrantPayload any
	if err := json.Unmarshal(rw2.Body.Bytes(), &revokeGrantPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/tenant-grants/revoke"), revokeGrantPayload, "$"); err != nil {
		t.Fatalf("tenant.grant.revoke contract: %v", err)
	}

	cases := []struct {
		path     string
		body     string
		template string
	}{
		{"/api/tenants/ceiling", `{"tenant_id":"contract-auth","ceiling_usd":50}`, "/api/tenants/ceiling"},
		{"/api/tenants/autonomy", `{"tenant_id":"contract-auth","level":"L2"}`, "/api/tenants/autonomy"},
		{"/api/tenants/refund", `{"tenant_id":"contract-auth","amount_usd":1,"reason":"contract refund"}`, "/api/tenants/refund"},
		{"/api/tenants/status", `{"tenant_id":"contract-auth","status":"suspended"}`, "/api/tenants/status"},
		{"/api/tenants/status", `{"tenant_id":"contract-auth","status":"active"}`, "/api/tenants/status"},
		{"/api/sessions/refund", `{"session_id":"` + refundSessionID + `","amount_usd":1,"reason":"contract session refund"}`, "/api/sessions/refund"},
		{"/api/obligations/" + obligationID, `{"signature_hex":"` + hex.EncodeToString(ed25519.Sign(priv, obligationCloseCanonical(obligationID))) + `"}`, "/api/obligations/{id}"},
		{"/api/mandates/sign", `{"tenant_id":"contract-auth","job_hash":"contract-job","max_budget_usd":1,"ttl_seconds":60,"admin_action":` + mustJSON(t, signAdminAction(t, priv, pubHex, "contract-auth", "mandate_sign", map[string]any{"job_hash": "contract-job", "max_budget_usd": 1.0, "ttl_seconds": int64(60)})) + `}`, "/api/mandates/sign"},
		{"/api/tenants/revoke", `{"tenant_id":"contract-auth","reason":"contract revoke"}`, "/api/tenants/revoke"},
		{"/api/tenants/admin-key", `{"tenant_id":"contract-auth","new_admin_pubkey_hex":"` + newPubHex + `","action":` + mustJSON(t, signAdminAction(t, priv, pubHex, "contract-auth", "set_admin_key", map[string]string{"new_admin_pubkey_hex": newPubHex})) + `}`, "/api/tenants/admin-key"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		if !strings.HasPrefix(tc.path, "/api/obligations/") {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s status %d, want 200: %s", tc.path, w.Code, strings.TrimSpace(w.Body.String()))
			continue
		}
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("%s body is not JSON: %v", tc.path, err)
			continue
		}
		contract := responseContractFor(t, doc, http.MethodPost, tc.template)
		if err := validateOpenAPISchema(doc, contract, body, "$"); err != nil {
			t.Errorf("%s violates %s: %v\nbody=%s", tc.path, contract, err, w.Body.String())
		}
	}

	revokeBody, _ := json.Marshal(map[string]string{"id": sessionsDoc.Sessions[0].ID})
	revokeReq := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", bytes.NewReader(revokeBody))
	revokeReq.Header.Set("Content-Type", "application/json")
	revokeReq.AddCookie(cookie)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, revokeReq)
	if rw.Code != http.StatusOK {
		t.Fatalf("auth revoke: %d %s", rw.Code, rw.Body.String())
	}
	var revokePayload any
	if err := json.Unmarshal(rw.Body.Bytes(), &revokePayload); err != nil {
		t.Fatalf("revoke JSON: %v", err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/auth/revoke"), revokePayload, "$"); err != nil {
		t.Fatalf("auth.revoke contract: %v", err)
	}
}

func TestConsoleHFDownloadContracts(t *testing.T) {
	srv, _, _, cleanup := newProviderContractServer(t)
	defer cleanup()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer slow.Close()
	oldBase := hf.HFResolveBase
	hf.HFResolveBase = slow.URL
	defer func() { hf.HFResolveBase = oldBase }()

	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	start := httptest.NewRequest(http.MethodPost, "/api/hf/download", strings.NewReader(`{"repo_id":"contract/model","filename":"weights.gguf"}`))
	start.Header.Set("Authorization", "Bearer provider-contract-token")
	start.Header.Set("Content-Type", "application/json")
	sw := httptest.NewRecorder()
	handler.ServeHTTP(sw, start)
	if sw.Code != http.StatusOK {
		t.Fatalf("hf download: %d %s", sw.Code, sw.Body.String())
	}
	var startDoc struct {
		Task struct {
			TaskID string `json:"task_id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(sw.Body.Bytes(), &startDoc); err != nil || startDoc.Task.TaskID == "" {
		t.Fatalf("hf download decode: %v %s", err, sw.Body.String())
	}
	var startBody any
	if err := json.Unmarshal(sw.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/hf/download"), startBody, "$"); err != nil {
		t.Fatalf("hf.download contract: %v", err)
	}

	cancel := httptest.NewRequest(http.MethodPost, "/api/hf/downloads/"+startDoc.Task.TaskID+"/cancel", nil)
	cancel.Header.Set("Authorization", "Bearer provider-contract-token")
	cw := httptest.NewRecorder()
	handler.ServeHTTP(cw, cancel)
	if cw.Code != http.StatusOK {
		t.Fatalf("hf cancel: %d %s", cw.Code, cw.Body.String())
	}
	var cancelBody any
	if err := json.Unmarshal(cw.Body.Bytes(), &cancelBody); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/hf/downloads/{id}/cancel"), cancelBody, "$"); err != nil {
		t.Fatalf("hf.cancel contract: %v", err)
	}
}

func TestConsoleSessionImportContract(t *testing.T) {
	srv, _, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	export := httptest.NewRequest(http.MethodPost, "/api/sessions/export", strings.NewReader(`{"session_id":"`+sessionID+`"}`))
	export.Header.Set("Authorization", "Bearer provider-contract-token")
	export.Header.Set("Content-Type", "application/json")
	ew := httptest.NewRecorder()
	handler.ServeHTTP(ew, export)
	if ew.Code != http.StatusOK {
		t.Fatalf("export: %d %s", ew.Code, ew.Body.String())
	}
	var manifest json.RawMessage
	if err := json.Unmarshal(ew.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	body, _ := json.Marshal(map[string]json.RawMessage{"manifest": manifest})
	imp := httptest.NewRequest(http.MethodPost, "/api/sessions/import", bytes.NewReader(body))
	imp.Header.Set("Authorization", "Bearer provider-contract-token")
	imp.Header.Set("Content-Type", "application/json")
	iw := httptest.NewRecorder()
	handler.ServeHTTP(iw, imp)
	if iw.Code != http.StatusOK {
		t.Fatalf("import: %d %s", iw.Code, iw.Body.String())
	}
	var payload any
	if err := json.Unmarshal(iw.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/sessions/import"), payload, "$"); err != nil {
		t.Fatalf("session.import contract: %v", err)
	}
}

func TestConsoleChatContracts(t *testing.T) {
	t.Setenv("KUBONA_LLM_CLOUD", "1")
	srv, sm, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	if _, err := sm.AddMessage(sessionID, "user", "hello", nil); err != nil {
		t.Fatalf("seed user message: %v", err)
	}

	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"contract-model"}]}`))
			return
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"CONTRACT\"},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":1}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-contract","model":"contract-model","choices":[{"index":0,"message":{"role":"assistant","content":"CONTRACT-REPLY"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer llmSrv.Close()

	snap := srv.llmSettings.GetPublicSnapshot()
	snap.ActiveProvider = llm.ProviderOpenAI
	snap.ActiveModel = "contract-model"
	snap.AirGapped = false
	snap.MaxTokens = 64
	snap.Providers[llm.ProviderOpenAI] = llm.ProviderConfig{
		Type: llm.ProviderOpenAI, Name: "Contract OpenAI-compatible",
		BaseURL: llmSrv.URL, DefaultModel: "contract-model", Enabled: true, TimeoutSec: 10,
	}
	for role := range snap.RoleAssignments {
		snap.RoleAssignments[role] = llm.RoleModelAssignment{Role: role, Provider: llm.ProviderOpenAI, Model: "contract-model"}
	}
	if err := srv.llmSettings.Update(&snap, nil); err != nil {
		t.Fatalf("llm settings: %v", err)
	}

	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	msg := call(http.MethodPost, "/api/chat/messages", `{"session_id":"`+sessionID+`","role":"user","content":"hello"}`)
	if msg.Code != http.StatusCreated {
		t.Fatalf("chat messages: %d %s", msg.Code, msg.Body.String())
	}
	var msgPayload any
	if err := json.Unmarshal(msg.Body.Bytes(), &msgPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/chat/messages"), msgPayload, "$"); err != nil {
		t.Fatalf("chat.send contract: %v", err)
	}

	regen := call(http.MethodPost, "/api/chat/regenerate", `{"session_id":"`+sessionID+`"}`)
	if regen.Code != http.StatusCreated {
		t.Fatalf("chat regenerate: %d %s", regen.Code, regen.Body.String())
	}
	var regenPayload any
	if err := json.Unmarshal(regen.Body.Bytes(), &regenPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/chat/regenerate"), regenPayload, "$"); err != nil {
		t.Fatalf("chat.regenerate contract: %v", err)
	}

	stream := call(http.MethodPost, "/api/chat/stream?session_id="+sessionID, `{"session_id":"`+sessionID+`","role":"user","content":"stream please"}`)
	if stream.Code != http.StatusOK {
		t.Fatalf("chat stream: %d %s", stream.Code, stream.Body.String())
	}
	if !strings.Contains(stream.Body.String(), "event: done") || !strings.Contains(stream.Body.String(), "CONTRACT") {
		t.Fatalf("chat stream missing done/token: %s", stream.Body.String())
	}

	synth := call(http.MethodPost, "/api/sessions/"+sessionID+"/campaign/synthesize", `{"prompt":"H2 ground state"}`)
	if synth.Code != http.StatusOK {
		t.Fatalf("campaign synth: %d %s", synth.Code, synth.Body.String())
	}
	var synthPayload any
	if err := json.Unmarshal(synth.Body.Bytes(), &synthPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/sessions/{id}/campaign/synthesize"), synthPayload, "$"); err != nil {
		t.Fatalf("campaign.synthesize contract: %v", err)
	}

	qasm := "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0],q[1];\nmeasure q -> c;\n"
	launch := call(http.MethodPost, "/api/chat/launch", `{"session_id":"`+sessionID+`","qasm":`+mustJSON(t, qasm)+`,"backend":"local-simulator","budget_usd":0}`)
	if launch.Code != http.StatusCreated {
		t.Fatalf("chat launch: %d %s", launch.Code, launch.Body.String())
	}
	var launchPayload any
	if err := json.Unmarshal(launch.Body.Bytes(), &launchPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/chat/launch"), launchPayload, "$"); err != nil {
		t.Fatalf("chat.launch contract: %v", err)
	}

	trib := call(http.MethodPost, "/api/chat/tribunal", `{"session_id":"`+sessionID+`","prompt":"H2 ground state","request_id":"chat-tribunal-contract"}`)
	if trib.Code != http.StatusOK && trib.Code != http.StatusCreated {
		t.Fatalf("chat tribunal: %d %s", trib.Code, trib.Body.String())
	}
	var tribPayload any
	if err := json.Unmarshal(trib.Body.Bytes(), &tribPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/chat/tribunal"), tribPayload, "$"); err != nil {
		t.Fatalf("tribunal.run contract: %v", err)
	}
}

func TestConsoleMetasearchContracts(t *testing.T) {
	srv, _, _, cleanup := newProviderContractServer(t)
	defer cleanup()
	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	approved := srv.metaSearchEngine.ProposeSearchRefinement("loop-a", "ansatz-search", "search.md", "old", "--- a\n+++ b\n", "contract")
	rejected := srv.metaSearchEngine.ProposeSearchRefinement("loop-b", "ansatz-search", "search.md", "old", "--- a\n+++ b\n", "contract")
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	steps := []struct {
		path string
		body string
	}{
		{"/api/metasearch/approve", `{"proposal_id":"` + approved.ID + `","signer":"contract-reviewer"}`},
		{"/api/metasearch/apply", `{"proposal_id":"` + approved.ID + `"}`},
		{"/api/metasearch/reject", `{"proposal_id":"` + rejected.ID + `","signer":"contract-reviewer"}`},
	}
	for _, step := range steps {
		w := call(step.path, step.body)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", step.path, w.Code, w.Body.String())
		}
		var payload any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, step.path), payload, "$"); err != nil {
			t.Fatalf("%s contract: %v", step.path, err)
		}
	}
}

func TestConsoleTribunalResumeContract(t *testing.T) {
	srv, _, sessID := setupResumeDrill(t)
	res, err := srv.collective.ExecuteTribunalWithSession(context.Background(), nil, nil, nil, sessID, "contract seed")
	if err != nil {
		t.Fatalf("seed tribunal: %v", err)
	}
	bundle, err := agents.MarshalTribunalBundle(res)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"session_id": sessID, "prompt": "continue", "bundle": json.RawMessage(bundle),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tribunal/resume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", w.Code, w.Body.String())
	}
	doc := loadConsoleOpenAPI(t)
	var payload any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/tribunal/resume"), payload, "$"); err != nil {
		t.Fatalf("tribunal.resume contract: %v", err)
	}
}

func TestConsoleCodesignRerunContract(t *testing.T) {
	t.Setenv("KUBONA_LLM_CLOUD", "1")
	srv, _, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-rerun","model":"contract-model","choices":[{"index":0,"message":{"role":"assistant","content":"{\"ops\":[[\"ccx\",[0,1,2],[]]]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer llmSrv.Close()
	snap := srv.llmSettings.GetPublicSnapshot()
	const pricedModel = "nvidia/nemotron-3.5-super-vl-120b-a12b"
	snap.ActiveProvider = llm.ProviderNVIDIA
	snap.ActiveModel = pricedModel
	snap.AirGapped = false
	snap.MaxTokens = 128
	snap.Providers[llm.ProviderNVIDIA] = llm.ProviderConfig{
		Type: llm.ProviderNVIDIA, Name: "Contract NVIDIA-compatible",
		BaseURL: llmSrv.URL, DefaultModel: pricedModel, Enabled: true, TimeoutSec: 10,
	}
	for role := range snap.RoleAssignments {
		snap.RoleAssignments[role] = llm.RoleModelAssignment{Role: role, Provider: llm.ProviderNVIDIA, Model: pricedModel}
	}
	snap.RoleAssignments["codesign"] = llm.RoleModelAssignment{Role: "codesign", Provider: llm.ProviderNVIDIA, Model: pricedModel}
	if err := srv.llmSettings.Update(&snap, nil); err != nil {
		t.Fatalf("llm settings: %v", err)
	}
	body := `{"session_id":"` + sessionID + `","request_id":"contract-rerun-1","spec_kind":"and2","budget_steps":1,"num_tests":8,"max_tokens_per_step":128}`
	req := httptest.NewRequest(http.MethodPost, "/api/codesign/rerun", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-contract-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("codesign rerun: %d %s", w.Code, w.Body.String())
	}
	doc := loadConsoleOpenAPI(t)
	var payload any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/codesign/rerun"), payload, "$"); err != nil {
		t.Fatalf("codesign.rerun contract: %v", err)
	}
}

type contractHarness struct{}

func (contractHarness) ID() string                     { return "contract-harness" }
func (contractHarness) FixedSetup() string             { return "contract fixed setup" }
func (contractHarness) MutableArtifact() string        { return "artifact.py" }
func (contractHarness) Metric(string) (float64, error) { return 1, nil }
func (contractHarness) ValidityCheck(string) (bool, string) {
	return true, ""
}
func (contractHarness) Evaluate(*sandbox.SandboxRunner, string) (*autoresearch.HarnessResult, error) {
	return &autoresearch.HarnessResult{
		HarnessID: "contract-harness", Metric: 1, Valid: true,
		CircuitDepth: 1, NumQubits: 2, IsolationTag: "test",
	}, nil
}

func TestConsoleAutoresearchContracts(t *testing.T) {
	t.Setenv("KUBONA_LLM_CLOUD", "1")
	srv, _, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	srv.harnessReg.Register(contractHarness{})

	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-autoresearch","model":"contract-model","choices":[{"index":0,"message":{"role":"assistant","content":"def build_circuit():\n    return None"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer llmSrv.Close()
	snap := srv.llmSettings.GetPublicSnapshot()
	snap.ActiveProvider = llm.ProviderStrata
	snap.ActiveModel = "contract-model"
	snap.AirGapped = true
	snap.MaxTokens = 64
	snap.Providers[llm.ProviderStrata] = llm.ProviderConfig{
		Type: llm.ProviderStrata, Name: "Contract local engine",
		BaseURL: llmSrv.URL, DefaultModel: "contract-model", Enabled: true, IsLocal: true, TimeoutSec: 10,
	}
	for role := range snap.RoleAssignments {
		snap.RoleAssignments[role] = llm.RoleModelAssignment{Role: role, Provider: llm.ProviderStrata, Model: "contract-model"}
	}
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{Role: "coder", Provider: llm.ProviderStrata, Model: "contract-model"}
	snap.RoleAssignments["critic"] = llm.RoleModelAssignment{Role: "critic", Provider: llm.ProviderStrata, Model: "contract-model"}
	if err := srv.llmSettings.Update(&snap, nil); err != nil {
		t.Fatalf("llm settings: %v", err)
	}

	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	validate := func(w *httptest.ResponseRecorder, template string) {
		t.Helper()
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("%s: %d %s", template, w.Code, w.Body.String())
		}
		var payload any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, template), payload, "$"); err != nil {
			t.Fatalf("%s contract: %v", template, err)
		}
	}

	start := call(http.MethodPost, "/api/autoresearch/start", `{"session_id":"`+sessionID+`","harness_id":"contract-harness","max_iterations":2,"request_id":"autoresearch-contract"}`)
	if start.Code != http.StatusCreated {
		t.Fatalf("autoresearch start: %d %s", start.Code, start.Body.String())
	}
	validate(start, "/api/autoresearch/start")
	var state struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &state); err != nil || state.ID == "" {
		t.Fatalf("autoresearch state: %v %s", err, start.Body.String())
	}

	validate(call(http.MethodPost, "/api/autoresearch/loops/"+state.ID+"/pause", `{}`), "/api/autoresearch/loops/{id}/pause")
	validate(call(http.MethodPost, "/api/autoresearch/loops/"+state.ID+"/resume", `{}`), "/api/autoresearch/loops/{id}/resume")
	validate(call(http.MethodPost, "/api/autoresearch/loops/"+state.ID+"/step", `{}`), "/api/autoresearch/loops/{id}/step")
	validate(call(http.MethodPost, "/api/autoresearch/loops/"+state.ID+"/stop", `{}`), "/api/autoresearch/loops/{id}/stop")

	managed, err := srv.supervisor.RegisterLoop(sessionID, 5)
	if err != nil {
		t.Fatalf("supervisor loop: %v", err)
	}
	validate(call(http.MethodPost, "/api/loops/"+managed.ID+"/pause", `{}`), "/api/loops/{id}/{action}")
}

func TestConsoleObservatoryContracts(t *testing.T) {
	atlas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/assets") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"asset_contract"}`))
		case strings.Contains(r.URL.Path, "/tasks:"):
			_, _ = w.Write([]byte(`{"operation_id":"op_contract","status":"SUCCEEDED"}`))
		case strings.HasSuffix(r.URL.Path, "/operations/op_contract"):
			_, _ = w.Write([]byte(`{"done":true,"response":{"promptUsed":"contract prompt","frames":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer atlas.Close()
	t.Setenv("KUBONA_ATLAS_ENABLED", "1")
	t.Setenv("WLT_API_KEY", "contract-atlas-key")
	t.Setenv("WLT_API_BASE_URL", atlas.URL)

	srv, _, sessionID, cleanup := newProviderContractServer(t)
	defer cleanup()
	snap := srv.llmSettings.GetPublicSnapshot()
	snap.AirGapped = false
	if err := srv.llmSettings.Update(&snap, nil); err != nil {
		t.Fatalf("settings: %v", err)
	}
	handler := srv.Handler()
	doc := loadConsoleOpenAPI(t)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer provider-contract-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	validate := func(w *httptest.ResponseRecorder, template string) {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", template, w.Code, w.Body.String())
		}
		var payload any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, template), payload, "$"); err != nil {
			t.Fatalf("%s contract: %v", template, err)
		}
	}

	asset := call(http.MethodPost, "/api/observatory/assets", `{"session_id":"`+sessionID+`","data_base64":"data:image/png;base64,AAAA","display_name":"contract.png"}`)
	if asset.Code != http.StatusOK {
		t.Fatalf("asset upload: %d %s", asset.Code, asset.Body.String())
	}
	var assetPayload any
	if err := json.Unmarshal(asset.Body.Bytes(), &assetPayload); err != nil {
		t.Fatal(err)
	}
	if err := validateOpenAPISchema(doc, responseContractFor(t, doc, http.MethodPost, "/api/observatory/assets"), assetPayload, "$"); err != nil {
		t.Fatalf("lens.asset contract: %v", err)
	}

	dispatch := call(http.MethodPost, "/api/observatory/lens", `{"session_id":"`+sessionID+`","task_type":"atlasTextToImage","prompt":"contract observatory"}`)
	validate(dispatch, "/api/observatory/lens")

	status := call(http.MethodGet, "/api/observatory/lens?operation_id=op_contract", "")
	if status.Code != http.StatusOK {
		t.Fatalf("lens status: %d %s", status.Code, status.Body.String())
	}
	var statusPayload any
	if err := json.Unmarshal(status.Body.Bytes(), &statusPayload); err != nil {
		t.Fatal(err)
	}
	// lens.status has no required fields; any JSON object is the contract.
	if _, ok := statusPayload.(map[string]any); !ok {
		t.Fatalf("lens.status payload is not an object: %T", statusPayload)
	}

	resolve := call(http.MethodPost, "/api/observatory/lens/resolve", `{"session_id":"`+sessionID+`","operation_id":"op_contract"}`)
	validate(resolve, "/api/observatory/lens/resolve")
}
