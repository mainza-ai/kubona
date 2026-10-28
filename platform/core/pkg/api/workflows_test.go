package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

type fakeWorkflowBridge struct {
	pb.QuantumBridgeServiceClient
	lastPlanRequest *pb.WorkflowPlanRequest
	planResponse    *pb.WorkflowPlanResponse
	lastRunRequest  *pb.WorkflowRunRequest
	runResponse     *pb.WorkflowRunResponse
}

func (f *fakeWorkflowBridge) WorkflowPlan(_ context.Context, in *pb.WorkflowPlanRequest, _ ...grpc.CallOption) (*pb.WorkflowPlanResponse, error) {
	f.lastPlanRequest = in
	if f.planResponse != nil {
		return f.planResponse, nil
	}
	return &pb.WorkflowPlanResponse{
		Ok:                   true,
		WorkflowId:           "wf-plan-1",
		PlannedStepsJson:     `[{"id":"s1","tool":"compiler/transpile_circuit"}]`,
		UnresolvedReferences: []string{},
	}, nil
}

func (f *fakeWorkflowBridge) WorkflowRun(_ context.Context, in *pb.WorkflowRunRequest, _ ...grpc.CallOption) (*pb.WorkflowRunResponse, error) {
	f.lastRunRequest = in
	if f.runResponse != nil {
		return f.runResponse, nil
	}
	return &pb.WorkflowRunResponse{
		Ok:              true,
		WorkflowId:      "wf-run-1",
		StepsCompleted:  2,
		StepRecords:     []string{"rec_1", "rec_2"},
		LastResultsJson: `{"s2":{"ok":true}}`,
		SpentUsd:        1.25,
	}, nil
}

func setupWorkflowServer(t *testing.T, fb pb.QuantumBridgeServiceClient) (*api.Server, *session.Manager, *ledger.Ledger, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	// Workflow execution requires the bridge token in ALL envs (no
	// dev-token fallback on either side of the bridge).
	t.Setenv("KUBONA_BRIDGE_TOKEN", "test-bridge-token")
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "wf_test.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, fb, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	optOutAirGap(t, srv)
	return srv, sm, l, func() { l.Close() }
}

func postWorkflow(t *testing.T, h http.Handler, path string, bearer bool, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	if bearer {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestWorkflowPlanEndpoint(t *testing.T) {
	fb := &fakeWorkflowBridge{}
	srv, sm, _, cleanup := setupWorkflowServer(t, fb)
	defer cleanup()
	h := srv.Handler()

	sess, err := sm.CreateSession("Lab Plan Sess", 10.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 1. Unauthenticated request without bearer token is rejected with 401
	status, _ := postWorkflow(t, h, "/api/workflows/plan", false, map[string]any{
		"session_id": sess.ID,
		"workflow": map[string]any{
			"name": "test-wf",
			"steps": []any{
				map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"},
			},
		},
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", status)
	}

	// 2. Unknown session returns 404
	status, _ = postWorkflow(t, h, "/api/workflows/plan", true, map[string]any{
		"session_id": "ghost-session",
		"workflow": map[string]any{
			"name": "test-wf",
			"steps": []any{
				map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"},
			},
		},
	})
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 on unknown session, got %d", status)
	}

	// 3. Authorized request succeeds and returns plan
	status, resp := postWorkflow(t, h, "/api/workflows/plan", true, map[string]any{
		"session_id": sess.ID,
		"workflow": map[string]any{
			"name": "test-wf",
			"steps": []any{
				map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"},
			},
		},
		"inputs": map[string]any{"basis": "ibm"},
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on authorized plan, got %d (%v)", status, resp)
	}
	if resp["ok"] != true || resp["workflow_id"] != "wf-plan-1" {
		t.Fatalf("unexpected plan response: %v", resp)
	}
	if fb.lastPlanRequest == nil || fb.lastPlanRequest.SessionId != sess.ID {
		t.Fatalf("bridge plan request not forwarded correctly: %+v", fb.lastPlanRequest)
	}
}

func TestWorkflowRunEndpoint(t *testing.T) {
	fb := &fakeWorkflowBridge{}
	srv, sm, l, cleanup := setupWorkflowServer(t, fb)
	defer cleanup()
	h := srv.Handler()

	sess, err := sm.CreateSession("Lab Run Sess", 5.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	// Cap session spend limit at $5.00
	_, err = l.DB().Exec(`UPDATE sessions SET spend_limit_micro_usd = 5000000, current_spend_micro_usd = 0 WHERE id = ?`, sess.ID)
	if err != nil {
		t.Fatalf("failed to update spend limit: %v", err)
	}

	// 1. Missing session_id refuses
	status, _ := postWorkflow(t, h, "/api/workflows/run", true, map[string]any{
		"workflow": map[string]any{"name": "test-wf"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 on missing session_id, got %d", status)
	}

	// 2. Budget exceedance check: workflow ceiling ($10.00) exceeds session limit ($5.00)
	status, resp := postWorkflow(t, h, "/api/workflows/run", true, map[string]any{
		"session_id": sess.ID,
		"workflow": map[string]any{
			"name":               "expensive-wf",
			"budget_ceiling_usd": 10.0,
			"steps": []any{
				map[string]any{"id": "s1", "tool": "execution/submit_job"},
			},
		},
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 ceiling exceeded, got %d (%v)", status, resp)
	}

	// 3. Happy path: workflow ceiling $2.00 is admitted, realizes $1.25 spend, releases unused $0.75
	status, resp = postWorkflow(t, h, "/api/workflows/run", true, map[string]any{
		"session_id": sess.ID,
		"workflow": map[string]any{
			"name":               "normal-wf",
			"budget_ceiling_usd": 2.0,
			"steps": []any{
				map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"},
				map[string]any{"id": "s2", "tool": "provenance/write_record"},
			},
		},
		"force": true,
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on valid workflow run, got %d (%v)", status, resp)
	}
	if resp["ok"] != true || resp["steps_completed"] != float64(2) {
		t.Fatalf("unexpected run response: %v", resp)
	}

	// Check that SQLite books reflect realized spend ($1.25)
	sessUpdated, err := sm.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("failed to load updated session: %v", err)
	}
	if sessUpdated.CurrentSpendUSD != 1.25 {
		t.Fatalf("expected session current_spend_usd to be 1.25, got %f", sessUpdated.CurrentSpendUSD)
	}

	// Check bridge request carried session authority
	if fb.lastRunRequest == nil || !fb.lastRunRequest.SessionVerified {
		t.Fatalf("expected bridge request to carry SessionVerified=true")
	}
	if fb.lastRunRequest.TenantId != "default" {
		t.Fatalf("expected tenant_id 'default', got %q", fb.lastRunRequest.TenantId)
	}
	// No dev-token fallback: the gateway token is the bridge token.
	if fb.lastRunRequest.Token != "test-bridge-token" {
		t.Fatalf("expected bridge token forwarded, got %q", fb.lastRunRequest.Token)
	}
}

func TestWorkflowRunBridgeFailureReleasesReservation(t *testing.T) {
	fb := &fakeWorkflowBridge{
		runResponse: &pb.WorkflowRunResponse{
			Ok:           false,
			ErrorMessage: "tool step failed: timeout",
			WorkflowId:   "wf-failed-1",
			SpentUsd:     0.0,
		},
	}
	srv, sm, l, cleanup := setupWorkflowServer(t, fb)
	defer cleanup()
	h := srv.Handler()

	sess, err := sm.CreateSession("Fail Test Sess", 10.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	_, err = l.DB().Exec(`UPDATE sessions SET spend_limit_micro_usd = 10000000, current_spend_micro_usd = 0 WHERE id = ?`, sess.ID)
	if err != nil {
		t.Fatalf("failed to update spend limit: %v", err)
	}

	status, resp := postWorkflow(t, h, "/api/workflows/run", true, map[string]any{
		"session_id": sess.ID,
		"workflow": map[string]any{
			"name":               "failing-wf",
			"budget_ceiling_usd": 3.0,
			"steps": []any{
				map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"},
			},
		},
		"force": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 on failed workflow, got %d (%v)", status, resp)
	}

	// Verify that the $3.00 reservation was fully released back to 0.0
	sessAfter, _ := sm.GetSession(sess.ID)
	if sessAfter.CurrentSpendUSD != 0.0 {
		t.Fatalf("expected spend reservation to be released to 0.0, got %f", sessAfter.CurrentSpendUSD)
	}
}

func TestWorkflowCrossTenantAuthorization(t *testing.T) {
	fb := &fakeWorkflowBridge{}
	srv, sm, _, cleanup := setupWorkflowServer(t, fb)
	defer cleanup()
	h := srv.Handler()

	// Register Lab A tenant first
	if code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-a", "display_name": "Lab A"}); code != http.StatusCreated {
		t.Fatalf("create tenant A: %d", code)
	}

	// Lab A session
	sessA, err := sm.CreateSession("Lab A Sess", 20.0, "lab-a")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Bootstrap Lab B tenant and login
	if code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-b", "display_name": "Lab B"}); code != http.StatusCreated {
		t.Fatalf("create tenant B: %d", code)
	}
	bPub, bPriv, _ := ed25519.GenerateKey(nil)
	bHex := hex.EncodeToString(bPub)
	boot, _ := json.Marshal(map[string]any{"tenant_id": "lab-b", "new_admin_pubkey_hex": bHex})
	bReq := httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(boot))
	bReq.Header.Set("Authorization", "Bearer test-token")
	bRec := httptest.NewRecorder()
	h.ServeHTTP(bRec, bReq)
	if bRec.Code != http.StatusOK {
		t.Fatalf("set admin key B: %d", bRec.Code)
	}
	nonce, _ := authChallenge(t, srv)
	w, bCookie := authLogin(t, srv, "lab-b", bHex, bPriv, nonce)
	if w.Code != http.StatusOK || bCookie == "" {
		t.Fatalf("login B failed: %d", w.Code)
	}

	// 1. Lab B cookie accessing Lab A session on POST /api/workflows/plan is 403 Forbidden
	planBody, _ := json.Marshal(map[string]any{
		"session_id": sessA.ID,
		"workflow": map[string]any{
			"name":  "cross-plan",
			"steps": []any{map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"}},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/plan", bytes.NewReader(planBody))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: bCookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on cross-tenant plan, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. Lab B cookie accessing Lab A session on POST /api/workflows/run is 403 Forbidden
	runBody, _ := json.Marshal(map[string]any{
		"session_id": sessA.ID,
		"workflow": map[string]any{
			"name":  "cross-run",
			"steps": []any{map[string]any{"id": "s1", "tool": "compiler/transpile_circuit"}},
		},
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/workflows/run", bytes.NewReader(runBody))
	req2.AddCookie(&http.Cookie{Name: "kubona_session", Value: bCookie})
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on cross-tenant run, got %d (%s)", rec2.Code, rec2.Body.String())
	}
}
