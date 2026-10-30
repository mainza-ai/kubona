package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type rerunBridge struct {
	pb.UnimplementedQuantumBridgeServiceServer
	specJSON string
	evalJSON string
	ops      []string
}

func (b *rerunBridge) EvaluateCodesign(_ context.Context, req *pb.CodesignRequest) (*pb.CodesignResponse, error) {
	b.ops = append(b.ops, req.Operation)
	switch req.Operation {
	case "spec":
		return &pb.CodesignResponse{Success: true, ResultJson: b.specJSON}, nil
	case "evaluate":
		return &pb.CodesignResponse{Success: true, ResultJson: b.evalJSON}, nil
	default:
		return &pb.CodesignResponse{Success: false, ErrorMessage: "unknown operation"}, nil
	}
}

func setupRerunServer(t *testing.T, llmCalls *int64) (*api.Server, *ledger.Ledger, *rerunBridge, func()) {
	t.Helper()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(llmCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test", "model": "test-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": `{"ops":[["ccx",[0,1,2],[]]]}`},
			}},
			"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20},
		})
	}))
	t.Cleanup(llm.Close)
	t.Setenv("KUBONA_LLM_PROVIDER", "strata")
	t.Setenv("STRATA_MODEL", "test-model")
	t.Setenv("STRATA_BASE_URL", llm.URL+"/v1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_RATE_CODESIGN", "100,100")

	l, err := ledger.OpenLedger(t.TempDir() + "/rerun.db")
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	fake := &rerunBridge{
		specJSON: `{"ok":true,"spec_kind":"and2","num_qubits":3,"inputs":[0,1],"outputs":[2]}`,
		evalJSON: `{"ok":true,"spec_kind":"and2","result":{"valid":true,"toffoli":1,"peak_qubits":3,"tq":10,"reason":"ok"}}`,
	}
	gs := grpc.NewServer()
	pb.RegisterQuantumBridgeServiceServer(gs, fake)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, pb.NewQuantumBridgeServiceClient(conn), 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	return srv, l, fake, func() { l.Close() }
}

func rerunPost(t *testing.T, h http.Handler, path string, body any) (int, string, http.Header) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String(), w.Header()
}

func TestCodesignRerunRefusesAnonymous(t *testing.T) {
	var calls int64
	srv, _, _, cleanup := setupRerunServer(t, &calls)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "rerun-anon")
	code, body, _ := idemPost(t, h, "/api/codesign/rerun", map[string]any{
		"session_id": sid, "request_id": "anon-1", "spec_kind": "and2",
		"budget_steps": 1, "num_tests": 8, "max_tokens_per_step": 64,
	})
	if code != http.StatusUnauthorized || !strings.Contains(body, "anonymous spend is refused") {
		t.Fatalf("anonymous rerun must 401, got %d: %s", code, body)
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Fatal("anonymous refusal must happen before any LLM egress")
	}
}

func TestCodesignRerunSealsReceiptAndReplays(t *testing.T) {
	var calls int64
	srv, l, fake, cleanup := setupRerunServer(t, &calls)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "rerun")
	body := map[string]any{
		"session_id": sid, "request_id": "rerun-1", "spec_kind": "and2",
		"budget_steps": 2, "num_tests": 8, "seed": 7, "max_tokens_per_step": 128,
	}
	code, raw1, _ := rerunPost(t, h, "/api/codesign/rerun", body)
	if code != http.StatusOK {
		t.Fatalf("rerun must 200, got %d: %s", code, raw1)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw1), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true || out["best"] == nil {
		t.Fatalf("rerun must return best: %s", raw1)
	}
	if out["billing"] != "none" || out["payer"] != "local" {
		t.Fatalf("local provider must not bill: %s", raw1)
	}
	if out["provenance_status"] != "sealed" {
		t.Fatalf("receipt must be sealed: %s", raw1)
	}
	if len(fake.ops) < 3 {
		t.Fatalf("expected spec + two evaluates, got %v", fake.ops)
	}
	var n int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE session_id = ? AND event_type = 'trial'`, sid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one trial event, got %d", n)
	}
	before := atomic.LoadInt64(&calls)
	code, raw2, hdr := rerunPost(t, h, "/api/codesign/rerun", body)
	if code != http.StatusOK || raw1 != raw2 {
		t.Fatalf("replay must be byte-identical: %d\n%s\n%s", code, raw1, raw2)
	}
	if hdr.Get("X-Idempotent-Replay") != "true" {
		t.Fatal("replay must carry X-Idempotent-Replay:true")
	}
	if after := atomic.LoadInt64(&calls); after != before {
		t.Fatalf("replay must not call the LLM: before=%d after=%d", before, after)
	}
}

func TestCodesignRerunRequiresRequestID(t *testing.T) {
	var calls int64
	srv, _, _, cleanup := setupRerunServer(t, &calls)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "rerun-no-key")
	code, raw, _ := rerunPost(t, h, "/api/codesign/rerun", map[string]any{
		"session_id": sid, "spec_kind": "and2",
	})
	if code != http.StatusBadRequest || !strings.Contains(raw, "request_id is required") {
		t.Fatalf("missing request_id must 400, got %d: %s", code, raw)
	}
}

func TestCodesignRerunCloudNeedsOptIn(t *testing.T) {
	var calls int64
	srv, _, _, cleanup := setupRerunServer(t, &calls)
	defer cleanup()
	// Reassign the codesign role to a cloud provider; air-gap off so the
	// refusal tested here is the explicit KUBONA_LLM_CLOUD gate, not the
	// Sovereign Air-Gap.
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "test-cloud")
	h := srv.Handler()
	getW := httptest.NewRecorder()
	h.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil))
	var snap map[string]any
	if err := json.NewDecoder(getW.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	snap["air_gapped"] = false
	snap["role_assignments"].(map[string]any)["codesign"] = map[string]any{
		"role": "codesign", "provider": "nvidia", "model": "test-cloud",
	}
	// The provider was created disabled at boot (no key was set yet). This
	// test is about the KUBONA_LLM_CLOUD gate, so enable it explicitly.
	provs := snap["providers"].(map[string]any)
	nv := provs["nvidia"].(map[string]any)
	nv["enabled"] = true
	raw, _ := json.Marshal(snap)
	put := httptest.NewRequest(http.MethodPut, "/api/llm/settings", bytes.NewReader(raw))
	put.Header.Set("Authorization", "Bearer test-token")
	putW := httptest.NewRecorder()
	h.ServeHTTP(putW, put)
	if putW.Code != http.StatusOK {
		t.Fatalf("settings update: %d %s", putW.Code, putW.Body.String())
	}
	sid := idemSession(t, h, "rerun-cloud")
	code, body, _ := rerunPost(t, h, "/api/codesign/rerun", map[string]any{
		"session_id": sid, "request_id": "cloud-1", "spec_kind": "and2",
		"budget_steps": 1, "num_tests": 8, "max_tokens_per_step": 64,
	})
	if code != http.StatusForbidden || !strings.Contains(body, "KUBONA_LLM_CLOUD=1") {
		t.Fatalf("cloud without opt-in must 403, got %d: %s", code, body)
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Fatal("refusal must happen before any LLM egress")
	}
}

func TestCodesignRerunBYOKEstimatesButDoesNotDebitSession(t *testing.T) {
	var calls int64
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/nemotron-3.5-super-vl-120b-a12b")
	t.Setenv("KUBONA_LLM_CLOUD", "1")
	srv, _, _, cleanup := setupRerunServer(t, &calls)
	defer cleanup()
	h := srv.Handler()

	getW := httptest.NewRecorder()
	h.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/api/llm/settings", nil))
	var snap map[string]any
	if err := json.NewDecoder(getW.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	snap["air_gapped"] = false
	snap["role_assignments"].(map[string]any)["codesign"] = map[string]any{
		"role": "codesign", "provider": "nvidia",
		"model": "nvidia/nemotron-3.5-super-vl-120b-a12b",
	}
	provs := snap["providers"].(map[string]any)
	nv := provs["nvidia"].(map[string]any)
	nv["enabled"] = true
	// The fake LLM server is the provider endpoint for this test.
	nv["base_url"] = fakeLLMBaseURL(t)
	raw, _ := json.Marshal(snap)
	put := httptest.NewRequest(http.MethodPut, "/api/llm/settings", bytes.NewReader(raw))
	put.Header.Set("Authorization", "Bearer test-token")
	putW := httptest.NewRecorder()
	h.ServeHTTP(putW, put)
	if putW.Code != http.StatusOK {
		t.Fatalf("settings update: %d %s", putW.Code, putW.Body.String())
	}

	sid := idemSession(t, h, "rerun-byok")
	code, body, _ := rerunPost(t, h, "/api/codesign/rerun", map[string]any{
		"session_id": sid, "request_id": "byok-1", "spec_kind": "and2",
		"budget_steps": 2, "num_tests": 8, "max_tokens_per_step": 128,
		"max_usd": 0.10,
	})
	if code != http.StatusOK {
		t.Fatalf("BYOK rerun must 200, got %d: %s", code, body)
	}
	var out struct {
		Billing      string  `json:"billing"`
		Payer        string  `json:"payer"`
		EstimatedUSD float64 `json:"estimated_usd"`
		SessionID    string  `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Billing != "provider" || out.Payer != "user-key" || out.EstimatedUSD <= 0 {
		t.Fatalf("BYOK must record a provider estimate: %s", body)
	}
	getSess := httptest.NewRecorder()
	h.ServeHTTP(getSess, httptest.NewRequest(http.MethodGet, "/api/sessions/"+sid, nil))
	if getSess.Code != http.StatusOK {
		t.Fatalf("session read: %d %s", getSess.Code, getSess.Body.String())
	}
	var sess struct {
		CurrentSpendUSD float64 `json:"current_spend_usd"`
	}
	if err := json.Unmarshal(getSess.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if sess.CurrentSpendUSD != 0 {
		t.Fatalf("BYOK must not debit the QPU session cap: %.6f", sess.CurrentSpendUSD)
	}
}

func fakeLLMBaseURL(t *testing.T) string {
	t.Helper()
	// setupRerunServer sets STRATA_BASE_URL to the fake server; reuse it for
	// the NVIDIA provider in the BYOK test.
	return strings.TrimSuffix(os.Getenv("STRATA_BASE_URL"), "/v1") + "/v1"
}

func TestParseCodesignProposalStrict(t *testing.T) {
	ops, err := parseProposalForTest(`{"ops":[["ccx",[0,1,2],[]]]}`, 3, 64)
	if err != nil || len(ops) != 1 || ops[0].Name != "ccx" {
		t.Fatalf("valid proposal refused: %v %+v", err, ops)
	}
	if _, err := parseProposalForTest(`{"ops":[["h",[0],[]]]}`, 3, 64); err == nil {
		t.Fatal("unknown gate must refuse")
	}
	if _, err := parseProposalForTest(`{"ops":[["x",[0],[]]]} trailing`, 3, 64); err == nil {
		t.Fatal("trailing data must refuse")
	}
	if _, err := parseProposalForTest(`{"ops":[["ccx",[0,1,3],[]]]}`, 3, 64); err == nil {
		t.Fatal("out-of-range qubit must refuse")
	}
}

func parseProposalForTest(text string, n, max int) ([]api.CodesignOpForTest, error) {
	ops, err := api.ParseCodesignProposalForTest(text, n, max)
	if err != nil {
		return nil, err
	}
	out := make([]api.CodesignOpForTest, 0, len(ops))
	for _, o := range ops {
		out = append(out, o)
	}
	return out, nil
}
