package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Human dispatch proxy (R-34): Go owns auth + autonomy + validation +
// routing + single-ledger spend reservation; execution semantics live in
// the Python rail (hermetic bridge tests).

type fakeQuantumBridge struct {
	pb.QuantumBridgeServiceClient
	lastSubmit *pb.QuantumSubmitRequest
	submitResp *pb.QuantumSubmitResponse
	statusResp *pb.QuantumJobStatusResponse
	cancelResp *pb.QuantumCancelResponse
	cancels    int
}

func (f *fakeQuantumBridge) QuantumSubmit(_ context.Context, in *pb.QuantumSubmitRequest, _ ...grpc.CallOption) (*pb.QuantumSubmitResponse, error) {
	f.lastSubmit = in
	return f.submitResp, nil
}

func (f *fakeQuantumBridge) QuantumJobStatus(_ context.Context, _ *pb.QuantumJobStatusRequest, _ ...grpc.CallOption) (*pb.QuantumJobStatusResponse, error) {
	if f.statusResp != nil {
		return f.statusResp, nil
	}
	return &pb.QuantumJobStatusResponse{Ok: true, JobId: "qjob-1", Status: "completed"}, nil
}

func (f *fakeQuantumBridge) QuantumCancel(_ context.Context, _ *pb.QuantumCancelRequest, _ ...grpc.CallOption) (*pb.QuantumCancelResponse, error) {
	f.cancels++
	if f.cancelResp != nil {
		return f.cancelResp, nil
	}
	return &pb.QuantumCancelResponse{Ok: true, JobId: "qjob-1"}, nil
}

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

func TestQuantumJobsAirGapAndSessionlessAutonomy(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "qjobs_airgap.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	h := srv.Handler()

	// Default server boots with Sovereign Air-Gap ON: external cloud QPU must 403 before touching bridge.
	code, out := postQuantumJob(t, h, true, map[string]any{
		"qasm":            "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
		"provider":        "ionq",
		"shots":           100,
		"idempotency_key": "airgap-1",
	})
	if code != http.StatusForbidden || out["code"] != "forbidden" {
		t.Fatalf("air-gapped external QPU must 403 forbidden, got %d %v", code, out)
	}

	// Opt out of air-gap, then suspend default tenant: omitting session_id ("") must NOT bypass tenant autonomy (F-NEW-05).
	optOutAirGap(t, srv)
	if _, err := l.DB().Exec(`UPDATE tenants SET status = 'suspended' WHERE id = 'default'`); err != nil {
		t.Fatalf("suspend default: %v", err)
	}
	code, out = postQuantumJob(t, h, true, map[string]any{
		"qasm":            "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
		"provider":        "local-simulator",
		"shots":           100,
		"idempotency_key": "autonomy-bypass-1",
	})
	if code != http.StatusForbidden || out["code"] != "forbidden" {
		t.Fatalf("suspended tenant with empty session_id must 403 forbidden, got %d %v", code, out)
	}
}

func TestQuantumJobsUnifiedSpendBookAndRefundIntegration(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	l, err := ledger.OpenLedger(filepath.Join(t.TempDir(), "qjobs_spend.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer l.Close()

	fb := &fakeQuantumBridge{}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, fb, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	optOutAirGap(t, srv)
	h := srv.Handler()

	// Create tenant + admin key + session with $50 ceiling.
	if code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-qpu", "display_name": "QPU Lab"}); code != http.StatusCreated {
		t.Fatalf("create tenant: %d", code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := hex.EncodeToString(adminPub)
	if _, err := srv.TenantManager().SetAdminKey("lab-qpu", adminHex, nil); err != nil {
		t.Fatalf("set admin key: %v", err)
	}
	sess, err := sm.CreateSession("qpu-exp", 50.0, "lab-qpu")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	qasm := "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
	mandate, merr := mandates.SignMandate(adminPriv, "lab-qpu",
		mandates.CanonicalJobBinding("circuit_run", sess.ID, qasm), 12.5,
		time.Now().UTC().Add(time.Hour))
	if merr != nil {
		t.Fatalf("sign mandate: %v", merr)
	}
	mandateJSON, _ := json.Marshal(mandate)

	fb.submitResp = &pb.QuantumSubmitResponse{
		Ok:               true,
		JobId:            "qjob-live-1",
		Queued:           true,
		SessionId:        sess.ID,
		EstimateJson:     `{"estimated_usd":12.5,"provider":"ionq"}`,
		FidelityJson:     `{"admitted":true}`,
		IdempotentReplay: false,
	}

	// 0. Hardware dispatch without a signed mandate refuses BEFORE the bridge
	// is called: unmandated hardware must never enqueue.
	fb.lastSubmit = nil
	if code, out := postQuantumJob(t, h, true, map[string]any{
		"session_id":      sess.ID,
		"qasm":            qasm,
		"provider":        "ionq",
		"shots":           500,
		"idempotency_key": "qpu-no-mandate",
	}); code != http.StatusForbidden || fb.lastSubmit != nil {
		t.Fatalf("unmandated hardware must 403 before dispatch, got %d %v", code, out)
	}

	// 1. First submit: forwards session ceiling + default allied jurisdictions, and reserves $12.50 in Go SQLite.
	code, out := postQuantumJob(t, h, true, map[string]any{
		"session_id":      sess.ID,
		"qasm":            qasm,
		"provider":        "ionq",
		"shots":           500,
		"idempotency_key": "qpu-idem-1",
		"mandate_ref":     string(mandateJSON),
	})
	if code != http.StatusOK || out["job_id"] != "qjob-live-1" {
		t.Fatalf("submit failed: %d %v", code, out)
	}
	if fb.lastSubmit == nil || !fb.lastSubmit.SessionVerified || fb.lastSubmit.SessionBudgetCeilingUsd != 50.0 {
		t.Fatalf("expected verified session ceiling 50.0 forwarded to bridge, got %+v", fb.lastSubmit)
	}
	if len(fb.lastSubmit.CallerJurisdictions) == 0 || fb.lastSubmit.CallerExportClass != "allied" {
		t.Fatalf("expected default allied jurisdictions forwarded to bridge, got %+v", fb.lastSubmit)
	}

	gotSess, _ := sm.GetSession(sess.ID)
	if gotSess.CurrentSpendUSD != 12.5 {
		t.Fatalf("expected session spend 12.50 in SQLite, got %v", gotSess.CurrentSpendUSD)
	}
	gotTen, _ := srv.TenantManager().GetTenant("lab-qpu")
	if gotTen.SpentUSD != 12.5 {
		t.Fatalf("expected tenant spend 12.50 in SQLite, got %v", gotTen.SpentUSD)
	}

	// 2. Idempotent replay: must NOT double-reserve spend in SQLite.
	fb.submitResp.IdempotentReplay = true
	code, _ = postQuantumJob(t, h, true, map[string]any{
		"session_id":      sess.ID,
		"qasm":            qasm,
		"provider":        "ionq",
		"shots":           500,
		"idempotency_key": "qpu-idem-1",
		"mandate_ref":     string(mandateJSON),
	})
	if code != http.StatusOK {
		t.Fatalf("replay failed: %d", code)
	}
	gotSess, _ = sm.GetSession(sess.ID)
	if gotSess.CurrentSpendUSD != 12.5 {
		t.Fatalf("idempotent replay must not double-reserve session spend, got %v", gotSess.CurrentSpendUSD)
	}

	// 3. Single-book job table integration (Phase 1 / F-NEW-05):
	//    - DeleteBlockers must see the live quantum job and its spend reservation.
	//    - GET /api/jobs?session_id= must include the job.
	blockers, berr := sm.DeleteBlockers(sess.ID)
	if berr != nil || len(blockers) == 0 {
		t.Fatalf("expected DeleteBlockers to block session deletion while quantum job is queued, got %v (err=%v)", blockers, berr)
	}
	storedJob, jerr := jr.GetJob("qjob-live-1")
	if jerr != nil || storedJob == nil || storedJob.ReservedAmount != 12.5 {
		t.Fatalf("expected qjob-live-1 recorded in SQLite jobs with ReservedAmount=12.5, got %+v (err=%v)", storedJob, jerr)
	}

	// 4. POST /api/sessions/refund against the HTTP-dispatched quantum job session succeeds and records session_spend_refunded!
	refBody := map[string]any{
		"session_id": sess.ID, "amount_usd": 5.0, "reason": "partial calibration credit",
		"action": refundSessionAction(t, "lab-qpu", sess.ID, adminHex, adminPriv, 5.0, "partial calibration credit"),
	}
	refCode, refOut := postSessionRefund(t, h, true, refBody)
	if refCode != http.StatusOK || refOut["current_spend_usd"] != 7.5 {
		t.Fatalf("session refund after quantum dispatch must succeed and leave 7.5, got %d %v", refCode, refOut)
	}

	// 5. Status transition to completed settles the job row in SQLite jobs.
	fb.statusResp = &pb.QuantumJobStatusResponse{
		Ok:         true,
		JobId:      "qjob-live-1",
		Status:     "completed",
		SessionId:  sess.ID,
		ResultJson: `{"counts":{"00":250,"11":250}}`,
	}
	stReq := httptest.NewRequest(http.MethodGet, "/api/quantum/jobs/qjob-live-1", nil)
	stReq.Header.Set("Authorization", "Bearer test-token")
	stW := httptest.NewRecorder()
	h.ServeHTTP(stW, stReq)
	if stW.Code != http.StatusOK {
		t.Fatalf("quantum job status failed: %d %s", stW.Code, stW.Body.String())
	}
	settledJob, _ := jr.GetJob("qjob-live-1")
	if settledJob == nil || settledJob.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("expected SQLite job row transitioned to JOB_STATUS_COMPLETED, got %+v", settledJob)
	}
}

// Quantum cancel must not assume the recall won the race. A provider job
// already DONE when the cancel arrives must sync as completed and settle from
// measured usage; the old handler always wrote cancelled (and refunded $0).
// Live finding 2026-10-30: IBM job dbha28pq260s73busu70 was DONE/2s while the
// local ledger recorded cancelled/$0.00.
type fakeQuantumCancelServer struct {
	pb.UnimplementedQuantumBridgeServiceServer
	status string
	result string
}

func (f *fakeQuantumCancelServer) QuantumSubmit(_ context.Context, req *pb.QuantumSubmitRequest) (*pb.QuantumSubmitResponse, error) {
	return &pb.QuantumSubmitResponse{Ok: true, JobId: "qjob-cancel", EstimateJson: "{}", FidelityJson: "{}", Queued: true, SessionId: req.SessionId}, nil
}

func (f *fakeQuantumCancelServer) QuantumCancel(_ context.Context, req *pb.QuantumCancelRequest) (*pb.QuantumCancelResponse, error) {
	return &pb.QuantumCancelResponse{Ok: true, JobId: req.JobId, ProviderRecallAttempted: true, ProviderRecallStatus: "DONE"}, nil
}

func (f *fakeQuantumCancelServer) QuantumJobStatus(_ context.Context, req *pb.QuantumJobStatusRequest) (*pb.QuantumJobStatusResponse, error) {
	return &pb.QuantumJobStatusResponse{Ok: true, JobId: req.JobId, Status: f.status, ResultJson: f.result}, nil
}

func TestQuantumCancelSyncsCompletedVendorStatus(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	l, err := ledger.OpenLedger(t.TempDir() + "/quantum-cancel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()
	fake := &fakeQuantumCancelServer{
		status: "completed",
		result: `{"hardware_job_id":"hw-cancel","result":{"actual_usd":3.2,"usage_seconds":2}}`,
	}
	gs := grpc.NewServer()
	pb.RegisterQuantumBridgeServiceServer(gs, fake)
	go func() { _ = gs.Serve(lis) }()
	defer gs.Stop()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, pb.NewQuantumBridgeServiceClient(conn), 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	h := srv.Handler()
	sid := idemSession(t, h, "quantum cancel")
	code, out := postQuantumJob(t, h, true, map[string]any{
		"qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];", "provider": "local-simulator",
		"shots": 100, "idempotency_key": "cancel-live-1", "session_id": sid,
	})
	if code != http.StatusOK {
		t.Fatalf("submit: %d %v", code, out)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/quantum/jobs/qjob-cancel/cancel", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
	var status, result string
	if err := l.DB().QueryRow(`SELECT status, result_json FROM jobs WHERE id = 'qjob-cancel'`).Scan(&status, &result); err != nil {
		t.Fatal(err)
	}
	if status != pb.JobStatus_JOB_STATUS_COMPLETED.String() {
		t.Fatalf("cancel raced with vendor completion: status=%s result=%s", status, result)
	}
	if !strings.Contains(result, "actual_usd") || !strings.Contains(result, "3.2") {
		t.Fatalf("completed settlement lost measured usage: %s", result)
	}
}
