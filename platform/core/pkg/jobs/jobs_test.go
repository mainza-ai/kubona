package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

// fakeBridge is a deterministic test double for the quantum bridge: it echoes
// success for every RPC so the runner rail (queue → worker → ledger settle)
// is exercised end to end. Production code must never substitute canned
// values like these for measurements.
type fakeBridge struct{}

func (f *fakeBridge) ExecuteCircuit(ctx context.Context, in *pb.CircuitExecutionRequest, opts ...grpc.CallOption) (*pb.CircuitExecutionResponse, error) {
	return &pb.CircuitExecutionResponse{Success: true, JobId: in.JobId}, nil
}
func (f *fakeBridge) CutCircuit(ctx context.Context, in *pb.CircuitCutRequest, opts ...grpc.CallOption) (*pb.CircuitCutResponse, error) {
	return &pb.CircuitCutResponse{Success: true}, nil
}
func (f *fakeBridge) ComputeClassicalAnchor(ctx context.Context, in *pb.ClassicalAnchorRequest, opts ...grpc.CallOption) (*pb.ClassicalAnchorResponse, error) {
	return &pb.ClassicalAnchorResponse{Success: true, Molecule: in.Molecule}, nil
}
func (f *fakeBridge) EvaluateWavefunction(ctx context.Context, in *pb.ClaimEvaluationRequest, opts ...grpc.CallOption) (*pb.ClaimEvaluationResponse, error) {
	return &pb.ClaimEvaluationResponse{ClaimId: in.ClaimId, PassesChemicalAccuracy: true}, nil
}
func (f *fakeBridge) GroundLiterature(ctx context.Context, in *pb.LiteratureGroundingRequest, opts ...grpc.CallOption) (*pb.LiteratureGroundingResponse, error) {
	return &pb.LiteratureGroundingResponse{Cached: false}, nil
}
func (f *fakeBridge) ExecuteQuantumKrylov(ctx context.Context, in *pb.QuantumKrylovRequest, opts ...grpc.CallOption) (*pb.QuantumKrylovResponse, error) {
	return &pb.QuantumKrylovResponse{Success: true, GroundStateEnergy: -1.13727}, nil
}
func (f *fakeBridge) MitigateCircuit(ctx context.Context, in *pb.MitigationRequest, opts ...grpc.CallOption) (*pb.MitigationResponse, error) {
	return &pb.MitigationResponse{Success: true, MethodUsed: in.Method}, nil
}

func setupRunner(t *testing.T, bridge pb.QuantumBridgeServiceClient) (*Runner, func()) {
	t.Helper()
	dbPath := "test_jobs.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	// Admission requires registered sessions: pre-register the ids tests use.
	sm := session.NewManager(l)
	for _, sid := range []string{"sess-1", "sess-list"} {
		if _, err := sm.EnsureSessionWithID(sid, "jobs test session", 100.0, "default"); err != nil {
			t.Fatalf("seed session %s: %v", sid, err)
		}
	}
	r := NewRunner(l, bridge, 2)
	r.Start()
	cleanup := func() {
		r.Stop()
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return r, cleanup
}

func pollJob(t *testing.T, r *Runner, id string) *Job {
	t.Helper()
	for i := 0; i < 100; i++ {
		j, err := r.GetJob(id)
		if err != nil {
			t.Fatalf("GetJob failed: %v", err)
		}
		if j.Status == pb.JobStatus_JOB_STATUS_COMPLETED || j.Status == pb.JobStatus_JOB_STATUS_FAILED {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s never settled", id)
	return nil
}

func TestRunnerAllJobTypesSettle(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	cases := []struct {
		jobType string
		params  any
	}{
		{"circuit_run", map[string]any{"qasm": "OPENQASM 2.0;", "shots": 100}},
		{"circuit_cut", map[string]any{"qasm": "OPENQASM 2.0;", "max_subcircuit_qubits": 2}},
		{"classical_anchor", map[string]any{"molecule": "H2", "bond_distance": 0.7414}},
		{"claim_eval", map[string]any{"claim_id": "c1", "statement": "s"}},
		{"krylov_solve", map[string]any{"molecule": "H2", "bond_distance": 0.7414}},
		{"mitigate", map[string]any{"qasm": "OPENQASM 2.0;", "method": "ZNE"}},
	}
	for _, tc := range cases {
		j, err := r.SubmitJob("sess-1", tc.jobType, tc.params)
		if err != nil {
			t.Fatalf("SubmitJob %s failed: %v", tc.jobType, err)
		}
		settled := pollJob(t, r, j.ID)
		if settled.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
			t.Fatalf("job %s not completed: %+v", tc.jobType, settled)
		}
		if len(settled.ResultJSON) == 0 {
			t.Fatalf("job %s has empty result", tc.jobType)
		}
	}
}

func TestRunnerUnknownJobTypeFails(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "teleportation", map[string]any{})
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("unknown job type must fail, got %+v", settled)
	}
}

func TestRunnerNilBridgeFailsHonestly(t *testing.T) {
	r, cleanup := setupRunner(t, nil)
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "classical_anchor", map[string]any{"molecule": "H2"})
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("nil bridge must fail the job, got %+v", settled)
	}
}

func TestRunnerListJobs(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	if _, err := r.SubmitJob("sess-list", "mitigate", map[string]any{"qasm": "q;"}); err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}
	listed, err := r.ListJobs("sess-list", 10)
	if err != nil || len(listed) == 0 {
		t.Fatalf("ListJobs failed: %v (n=%d)", err, len(listed))
	}
}

func TestRunnerCancelTenantJobs(t *testing.T) {
	dbPath := "test_jobs_tenant_abort.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	// No Start(): without workers every job stays QUEUED and countable.
	r := NewRunner(l, &fakeBridge{}, 2)
	defer r.Stop()

	// Admission requires registered lab tenants.
	tm := tenants.NewManager(l)
	for _, lab := range []string{"lab-a", "lab-b"} {
		if _, err := tm.CreateTenant(lab, lab); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
	}

	seedSession := func(id, tenant string) {
		t.Helper()
		if _, err := r.ledger.DB().Exec(
			`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "t", tenant, "2026-10-09T00:00:00Z", "2026-10-09T00:00:00Z", "active", 100.0, 0.0,
		); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedSession("sess-tenant-a1", "lab-a")
	seedSession("sess-tenant-b1", "lab-b")

	j1, err := r.SubmitJob("sess-tenant-a1", "mitigate", map[string]any{"qasm": "q;"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	j2, err := r.SubmitJob("sess-tenant-b1", "mitigate", map[string]any{"qasm": "q;"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	n, err := r.CancelTenantJobs("lab-a", "tenant revoked")
	if err != nil || n != 1 {
		t.Fatalf("expected 1 aborted for lab-a, got %d err=%v", n, err)
	}
	a1, err := r.GetJob(j1.ID)
	if err != nil || a1.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("lab-a job must be failed: %+v err=%v", a1, err)
	}
	b1, err := r.GetJob(j2.ID)
	if err != nil || b1.Status != pb.JobStatus_JOB_STATUS_QUEUED {
		t.Fatalf("lab-b job must be untouched: %+v err=%v", b1, err)
	}
}

func TestAdmissionRefusesUnknownSession(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	if _, err := r.SubmitJob("ghost-session", "mitigate", map[string]any{"qasm": "q;"}); err == nil {
		t.Fatalf("unknown session must refuse at admission")
	}
}

func TestAdmissionQPUNeedsMandateReceipt(t *testing.T) {
	dbPath := "test_jobs_admit.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-qpu", "qpu drill", 500.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := NewRunner(l, &fakeBridge{}, 1)
	r.Start()
	defer r.Stop()

	qpu := map[string]any{"qasm": "q;", "requested_backend": "ibm_brisbane", "shots": 100, "max_cost_usd": 25.0}

	// No receipt: refused, nothing queued.
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("QPU job without mandate receipt must refuse")
	}

	// Simulator backend on the same session: admitted.
	sim := map[string]any{"qasm": "q;", "requested_backend": "local_simulator", "shots": 100}
	j, err := r.SubmitJob("sess-qpu", "circuit_run", sim)
	if err != nil {
		t.Fatalf("simulator job must admit: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("simulator job must complete: %+v", settled)
	}

	// Seed a verified mandate receipt (tenant default, $100, unexpired).
	payload := `{"tenant_id":"default","job_hash":"job-qpu","max_budget_usd":100,"expiry_timestamp":"2030-01-01T00:00:00Z"}`
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-qpu", "default", "mandate_verified_client", payload,
		"0", "receipt-hash-1", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu["mandate_ref"] = "receipt-hash-1"
	j2, err := r.SubmitJob("sess-qpu", "circuit_run", qpu)
	if err != nil {
		t.Fatalf("QPU job with covering receipt must admit: %v", err)
	}
	settled2 := pollJob(t, r, j2.ID)
	if settled2.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("admitted QPU job must complete: %+v", settled2)
	}

	// Over-budget receipt: refused.
	qpu["max_cost_usd"] = 500.0
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("job above mandate budget must refuse")
	}

	// Wrong-tenant receipt: refused.
	badPayload := `{"tenant_id":"lab-other","job_hash":"job-qpu","max_budget_usd":10000,"expiry_timestamp":"2030-01-01T00:00:00Z"}`
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-qpu", "default", "mandate_verified_client", badPayload,
		"0", "receipt-hash-2", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu["mandate_ref"] = "receipt-hash-2"
	qpu["max_cost_usd"] = 25.0
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("foreign-tenant receipt must refuse")
	}
}
