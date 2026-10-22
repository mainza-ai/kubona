package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

// fakeBridge is a deterministic test double for the quantum bridge: it echoes
// success for every RPC so the runner rail (queue → worker → ledger settle)
// is exercised end to end. Production code must never substitute canned
// values like these for measurements.
type fakeBridge struct {
	// T-01: embed the client interface so future RPC additions do not
	// break this fake (the methods below override the embedded nils
	// actually exercised; the rest panic loudly if ever called).
	pb.QuantumBridgeServiceClient
}

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
	return &pb.ClaimEvaluationResponse{ClaimId: in.ClaimId, PassesChemicalAccuracy: true, Success: true}, nil
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
		{"circuit_run", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "shots": 100}},
		{"circuit_cut", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "max_subcircuit_qubits": 2, "max_cost_usd": 1.0}},
		{"classical_anchor", map[string]any{"molecule": "H2", "bond_distance": 0.7414, "max_cost_usd": 1.0}},
		{"claim_eval", map[string]any{"claim_id": "c1", "statement": "s", "max_cost_usd": 1.0}},
		{"krylov_solve", map[string]any{"molecule": "H2", "bond_distance": 0.7414, "max_cost_usd": 1.0}},
		{"mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE", "max_cost_usd": 1.0}},
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

	// W0.2: unknown types refuse at submit with no residue (previously they
	// queued, reserved nothing, then failed in the worker).
	if _, err := r.SubmitJob("sess-1", "teleportation", map[string]any{}); err == nil {
		t.Fatalf("unknown job type must refuse at submit")
	} else if !strings.Contains(err.Error(), "unknown job type") {
		t.Fatalf("refusal must name the unknown type, got %v", err)
	}
	listed, lerr := r.ListJobs("sess-1", 50)
	if lerr != nil {
		t.Fatal(lerr)
	}
	for _, j := range listed {
		if j.JobType == "teleportation" {
			t.Fatalf("refused job must leave no row")
		}
	}
}

// W0.2: bridge-compute jobs run the spend rail — undeclared cost refuses,
// declared cost reserves against tenant + session and releases on failure.
func TestNonCircuitRunSpendRail(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	// No declared cost: refuse, no row, no spend moved.
	if _, err := r.SubmitJob("sess-1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE"}); err == nil {
		t.Fatalf("costless bridge job must refuse")
	} else if !strings.Contains(err.Error(), "max_cost_usd") {
		t.Fatalf("refusal must name max_cost_usd, got %v", err)
	}
	listed, _ := r.ListJobs("sess-1", 50)
	for _, j := range listed {
		if j.JobType == "mitigate" {
			t.Fatalf("refused job must leave no row")
		}
	}
	// Declared cost: reserves, persists on the row, settles.
	j, err := r.SubmitJob("sess-1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE", "max_cost_usd": 7.5})
	if err != nil {
		t.Fatalf("declared-cost submit: %v", err)
	}
	if j.ReservedAmount != 7.5 {
		t.Fatalf("reservation must persist on the row, got %v", j.ReservedAmount)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("declared-cost job must settle, got %+v", settled)
	}
	// circuit_run keeps params-embedded cost: a parallel declaration is
	// refused (one cost source per type, never two to confuse).
	if _, err := r.SubmitJobWithDeclaredCost("sess-1", "circuit_run", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"}, 5.0); err == nil {
		t.Fatalf("dual cost sources must refuse")
	}
}

func TestRunnerNilBridgeFailsHonestly(t *testing.T) {
	r, cleanup := setupRunner(t, nil)
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "classical_anchor", map[string]any{"molecule": "H2", "max_cost_usd": 1.0})
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

	if _, err := r.SubmitJob("sess-list", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "max_cost_usd": 1.0}); err != nil {
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

	j1, err := r.SubmitJob("sess-tenant-a1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "max_cost_usd": 1.0})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	j2, err := r.SubmitJob("sess-tenant-b1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "max_cost_usd": 1.0})
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

	if _, err := r.SubmitJob("ghost-session", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "max_cost_usd": 1.0}); err == nil {
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

	qpu := map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "ibm_brisbane", "shots": 100, "max_cost_usd": 25.0, "caller_jurisdictions": []string{"US"}}

	// No receipt: refused, nothing queued.
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("QPU job without mandate receipt must refuse")
	}

	// No declared jurisdictions: refused at routing before mandate checks.
	nojur := map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "ibm_brisbane", "shots": 100, "max_cost_usd": 25.0, "mandate_ref": "receipt-hash-1"}
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", nojur); err == nil {
		t.Fatalf("QPU job without caller_jurisdictions must refuse")
	} else if !strings.Contains(err.Error(), "caller_jurisdictions") {
		t.Fatalf("refusal must name the routing declaration, got: %v", err)
	}

	// Simulator backend on the same session: admitted.
	sim := map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "local_simulator", "shots": 100}
	j, err := r.SubmitJob("sess-qpu", "circuit_run", sim)
	if err != nil {
		t.Fatalf("simulator job must admit: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("simulator job must complete: %+v", settled)
	}

	// Seed a verified mandate receipt (tenant default, $100, unexpired).
	payload := fmt.Sprintf(`{"tenant_id":"default","job_hash":"%s","max_budget_usd":100,"expiry_timestamp":"2030-01-01T00:00:00Z"}`,
		mandates.CanonicalJobBinding("circuit_run", "sess-qpu", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"))
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
	badPayload := fmt.Sprintf(`{"tenant_id":"lab-other","job_hash":"%s","max_budget_usd":10000,"expiry_timestamp":"2030-01-01T00:00:00Z"}`,
		mandates.CanonicalJobBinding("circuit_run", "sess-qpu", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"))
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
	// W0.4: receipt bound to a DIFFERENT program refuses even when tenant
	// and budget cover — a mandate for one circuit cannot ride another.
	otherBinding := mandates.CanonicalJobBinding("circuit_run", "sess-qpu", "OPENQASM 2.0;\nqreg q[9];\ncreg c[9];\n")
	otherPayload := fmt.Sprintf(`{"tenant_id":"default","job_hash":"%s","max_budget_usd":100,"expiry_timestamp":"2030-01-01T00:00:00Z"}`, otherBinding)
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-qpu", "default", "mandate_verified_client", otherPayload,
		"0", "receipt-hash-3", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu["mandate_ref"] = "receipt-hash-3"
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("cross-program receipt must refuse")
	} else if !strings.Contains(err.Error(), "different program") {
		t.Fatalf("refusal must name the binding, got: %v", err)
	}

	// W0.7: metered hardware with zero declared cost refuses — a $0
	// reservation is unattributed spend (ReserveSpend treats 0 as no-op).
	qpu["mandate_ref"] = "receipt-hash-1"
	qpu["max_cost_usd"] = 0.0
	if _, err := r.SubmitJob("sess-qpu", "circuit_run", qpu); err == nil {
		t.Fatalf("zero-cost metered dispatch must refuse")
	} else if !strings.Contains(err.Error(), "max_cost_usd") {
		t.Fatalf("refusal must name the cost, got: %v", err)
	}
}

func TestAdmissionFidelityRefusesDeepCircuit(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	qasm := "OPENQASM 2.0;\nqreg q[2];\nh q[0];\n"
	for i := 0; i < 400; i++ {
		qasm += "cx q[0], q[1];\n"
	}
	// Shallow circuit passes fidelity but still needs spend authorization.
	_, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm":              "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0], q[1];",
		"requested_backend": "ibm_brisbane", "shots": 100, "max_cost_usd": 10.0,
		"caller_jurisdictions": []string{"US"},
	})
	if err == nil || !strings.Contains(err.Error(), "mandate_ref") {
		t.Fatalf("shallow QPU job must reach (and fail) the mandate check, got: %v", err)
	}
	_, err = r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm": qasm, "requested_backend": "ibm_brisbane", "shots": 100,
		"caller_jurisdictions": []string{"US"},
	})
	if err == nil {
		t.Fatalf("400-CX circuit must refuse at the fidelity floor")
	}
}

func TestAdmissionFidelityRefusesUnknownHardware(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	_, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm":              "OPENQASM 2.0;\nqreg q[2];\nh q[0];",
		"requested_backend": "mystery_qpu_9000", "shots": 100,
	})
	if err == nil {
		t.Fatalf("unknown hardware backend must refuse as unpredictable")
	}
}

func TestCountQasmGates(t *testing.T) {
	q, d, q2 := countQasmGates("OPENQASM 2.0;\nqreg q[3];\nh q[0];\ncx q[0], q[1];\nrz(0.5) q[2];\n")
	if q != 3 || d != 3 || q2 != 1 {
		t.Fatalf("got qubits=%d depth=%d twoQ=%d", q, d, q2)
	}
}

func TestCumulativeCeilingAcrossJobs(t *testing.T) {
	dbPath := "test_jobs_spend.db"
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
	if _, err := sm.EnsureSessionWithID("sess-spend", "spend drill", 500.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := NewRunner(l, &fakeBridge{}, 1)
	r.Start()
	defer r.Stop()

	// Deep receipt budget so only the tenant ceiling binds (default 150).
	payload := fmt.Sprintf(`{"tenant_id":"default","job_hash":"%s","max_budget_usd":10000,"expiry_timestamp":"2030-01-01T00:00:00Z"}`,
		mandates.CanonicalJobBinding("circuit_run", "sess-spend", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"))
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-spend", "default", "mandate_verified_client", payload,
		"0", "receipt-spend-1", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu := func(cost float64) map[string]any {
		return map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "ibm_brisbane",
			"shots": 100, "max_cost_usd": cost, "mandate_ref": "receipt-spend-1",
			"caller_jurisdictions": []string{"US"}}
	}

	// $100 admitted and kept on completion: spent is now 100, not 0.
	j1, err := r.SubmitJob("sess-spend", "circuit_run", qpu(100))
	if err != nil {
		t.Fatalf("first $100 job must admit: %v", err)
	}
	s1 := pollJob(t, r, j1.ID)
	if s1.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("first job must complete: %+v", s1)
	}

	// A second $100 job fits no ceiling math ($100+$100 > $150): refused,
	// even though each job alone is under the ceiling.
	if _, err := r.SubmitJob("sess-spend", "circuit_run", qpu(100)); err == nil {
		t.Fatal("second $100 job on a $150 ceiling must refuse (cumulative)")
	} else if !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("refusal must name the ceiling, got: %v", err)
	}

	// A $50 job still fits ($100+$50 <= $150): admitted.
	if _, err := r.SubmitJob("sess-spend", "circuit_run", qpu(50)); err != nil {
		t.Fatalf("$50 job within remaining budget must admit: %v", err)
	}
}

func TestFailedJobReleasesReservation(t *testing.T) {
	dbPath := "test_jobs_release.db"
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
	if _, err := sm.EnsureSessionWithID("sess-rel", "release drill", 500.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := NewRunner(l, &fakeBridge{}, 1)
	// No r.Start(): the worker must not settle the job first — this test owns
	// the terminal transition via failJob directly (deterministic, no race).
	defer r.Stop()

	payload := fmt.Sprintf(`{"tenant_id":"default","job_hash":"%s","max_budget_usd":10000,"expiry_timestamp":"2030-01-01T00:00:00Z"}`,
		mandates.CanonicalJobBinding("circuit_run", "sess-rel", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"))
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-rel", "default", "mandate_verified_client", payload,
		"0", "receipt-rel-1", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu := map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "ibm_brisbane",
		"shots": 100, "max_cost_usd": 100, "mandate_ref": "receipt-rel-1",
		"caller_jurisdictions": []string{"US"}}

	j1, err := r.SubmitJob("sess-rel", "circuit_run", qpu)
	if err != nil {
		t.Fatalf("first job must admit: %v", err)
	}
	// The worker will settle it completed; force-fail first to test release.
	// (failJob directly: terminal-failed paths must unwind spend.)
	stored, err := r.GetJob(j1.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.failJob(stored, "drill failure")
	spent := 0.0
	if err := l.DB().QueryRow(`SELECT spent_usd FROM tenants WHERE id = 'default'`).Scan(&spent); err != nil {
		t.Fatal(err)
	}
	if spent != 0 {
		t.Fatalf("failed job must release its reservation, spent=%v", spent)
	}
	// Double-release is a no-op: refusing nothing, minting nothing.
	r.failJob(stored, "drill failure again")
	if err := l.DB().QueryRow(`SELECT spent_usd FROM tenants WHERE id = 'default'`).Scan(&spent); err != nil {
		t.Fatal(err)
	}
	if spent != 0 {
		t.Fatalf("double release must stay floored at 0, spent=%v", spent)
	}
	// Budget is whole again: a fresh $100 job admits.
	if _, err := r.SubmitJob("sess-rel", "circuit_run", qpu); err != nil {
		t.Fatalf("job after release must admit: %v", err)
	}
}

func TestSessionLimitBindsEndToEnd(t *testing.T) {
	dbPath := "test_jobs_sesslimit.db"
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
	// Tighter than the $150 tenant ceiling: the session rail must bind first.
	if _, err := sm.EnsureSessionWithID("sess-cap", "capped drill", 120.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := NewRunner(l, &fakeBridge{}, 1)
	r.Start()
	defer r.Stop()

	payload := fmt.Sprintf(`{"tenant_id":"default","job_hash":"%s","max_budget_usd":10000,"expiry_timestamp":"2030-01-01T00:00:00Z"}`,
		mandates.CanonicalJobBinding("circuit_run", "sess-cap", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n"))
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-cap", "default", "mandate_verified_client", payload,
		"0", "receipt-cap-1", "pub", "sig",
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	qpu := func(cost float64) map[string]any {
		return map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "requested_backend": "ibm_brisbane",
			"shots": 100, "max_cost_usd": cost, "mandate_ref": "receipt-cap-1",
			"caller_jurisdictions": []string{"US"}}
	}
	j1, err := r.SubmitJob("sess-cap", "circuit_run", qpu(70))
	if err != nil {
		t.Fatalf("first $70 job must admit: %v", err)
	}
	if s := pollJob(t, r, j1.ID); s.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("first job must complete: %+v", s)
	}
	// $70+$70=140 fits the $150 tenant ceiling but breaks the $120 session:
	// the session rail must be the one that refuses.
	if _, err := r.SubmitJob("sess-cap", "circuit_run", qpu(70)); err == nil {
		t.Fatal("second $70 job on a $120 session must refuse")
	} else if !strings.Contains(err.Error(), "session") {
		t.Fatalf("refusal must name the session rail, got: %v", err)
	}
}

// The settled hook fires from the worker on COMPLETED with the stored
// result attached — this is the wire the planner replan hangs on.
func TestSettledHookFiresOnCompleted(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	var mu sync.Mutex
	var fired []*Job
	r.SetSettledHook(func(j *Job) {
		mu.Lock()
		defer mu.Unlock()
		fired = append(fired, j)
	})

	j, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n", "shots": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
		t.Fatalf("want COMPLETED, got %v", settled.Status)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(fired)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 1 || fired[0].ID != j.ID {
		t.Errorf("hook must fire exactly once for the settled job, got %d", len(fired))
	}
	if len(fired[0].ResultJSON) == 0 {
		t.Error("hooked job must carry its stored result")
	}
}

// B0-5: the compile gate analyzes pre-dispatch and persists provenance.
// Without a Rust core the Go fallback labels honestly; without a compiler
// the row stays empty (unanalyzed, never implied).
func TestCompileGatePersistsProvenance(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()
	r.SetCompiler(qir.NewCompiler())

	j, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm":  "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n",
		"shots": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if j.CompilerUsed == "" {
		t.Error("analyzed submit must record compiler_used")
	}
	got, err := r.GetJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompilerUsed != j.CompilerUsed || got.FallbackEstimate != j.FallbackEstimate {
		t.Errorf("provenance must round-trip: %+v", got)
	}
	if got.CompilerUsed == "rust-qir-core" && got.FallbackEstimate {
		t.Error("rust verdict and fallback flag contradict")
	}
	listed, err := r.ListJobs("sess-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, lj := range listed {
		if lj.ID == j.ID {
			found = true
			if lj.CompilerUsed != j.CompilerUsed {
				t.Error("ListJobs must carry compile provenance")
			}
		}
	}
	if !found {
		t.Error("submitted job missing from list")
	}
}

func TestCompileGateAbsentWithoutCompiler(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	j, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm":  "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
		"shots": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if j.CompilerUsed != "" || j.FallbackEstimate {
		t.Errorf("compiler-less runner must leave provenance empty, got %+v", j)
	}
}

func TestFidelityAssessmentSealedToLedger(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()
	shallow := "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0],q[1];\n"

	// ADMIT: shallow heron circuit clears the floor.
	if err := r.admitFidelity("sess-1", "ibm_brisbane", shallow); err != nil {
		t.Fatalf("shallow heron job must admit: %v", err)
	}
	// REFUSE: below-floor rigetti depth (70 two-qubit gates at 0.008 err).
	deep := "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\n" + strings.Repeat("cx q[0],q[1];\n", 70)
	if err := r.admitFidelity("sess-1", "rigetti_aspen_qpu", deep); err == nil {
		t.Fatalf("below-floor fidelity must refuse")
	}
	// REFUSE: unknown backend has no profile — denial sealed, not silent.
	if err := r.admitFidelity("sess-1", "mystery_qpu", shallow); err == nil {
		t.Fatalf("unknown-backend fidelity must refuse")
	}

	events, err := r.ledger.GetRecentEvents("sess-1", 10)
	if err != nil {
		t.Fatalf("recent events: %v", err)
	}
	seen := map[string]map[string]any{}
	for _, e := range events {
		if e.EventType != "fidelity_assessment" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("payload decode: %v", err)
		}
		seen[p["decision"].(string)+"/"+p["backend"].(string)] = p
	}
	admit, ok := seen["ADMIT/ibm_brisbane"]
	if !ok {
		t.Fatalf("admit assessment not sealed: %v", seen)
	}
	for _, k := range []string{"predicted_fidelity", "floor_threshold", "drift_tolerance", "profile_source", "depth", "reason"} {
		if _, ok := admit[k]; !ok {
			t.Errorf("admit record missing %q", k)
		}
	}
	if admit["measured"] != false {
		t.Errorf("floor-only assessment must mark measured:false")
	}
	ref, ok := seen["REFUSE/mystery_qpu"]
	if !ok {
		t.Fatalf("unknown-device refusal not sealed: %v", seen)
	}
	if ref["profile_source"] != "none" {
		t.Errorf("unassessed refusal must record profile_source:none, got %v", ref["profile_source"])
	}
	if _, ok := seen["APPLY_MITIGATION/rigetti_aspen_qpu"]; !ok {
		t.Fatalf("below-floor mitigation refusal not sealed: %v", seen)
	}
}

type bigBridge struct {
	fakeBridge
}

func (b *bigBridge) ExecuteCircuit(ctx context.Context, in *pb.CircuitExecutionRequest, opts ...grpc.CallOption) (*pb.CircuitExecutionResponse, error) {
	return &pb.CircuitExecutionResponse{Success: true, JobId: in.JobId, ErrorMessage: strings.Repeat("B", 2*1024*1024)}, nil
}

func TestOversizeResultRefused(t *testing.T) {
	r, cleanup := setupRunner(t, &bigBridge{})
	defer cleanup()
	j, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "shots": 8})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("oversize result must fail the job, got %s", settled.Status)
	}
	got, err := r.GetJob(j.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.ResultJSON) > MaxStoredResultBytes {
		t.Fatalf("stored blob must respect the cap, got %d bytes", len(got.ResultJSON))
	}
	if !strings.Contains(string(got.ResultJSON), "result_too_large") {
		t.Fatalf("failure must name result_too_large, got %s", got.ResultJSON)
	}
}

// W1.8: bridge-reported failure settles the job FAILED — never COMPLETED
// with a zero payload that downstream folds mistake for measurement.
type failBridge struct{ fakeBridge }

func (f *failBridge) MitigateCircuit(ctx context.Context, in *pb.MitigationRequest, opts ...grpc.CallOption) (*pb.MitigationResponse, error) {
	return &pb.MitigationResponse{Success: false, ErrorMessage: "mitigation refused: test"}, nil
}

func TestBridgeReportedFailureSettlesFailed(t *testing.T) {
	r, cleanup := setupRunner(t, &failBridge{})
	defer cleanup()
	j, err := r.SubmitJob("sess-1", "mitigate", map[string]any{
		"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE",
		"max_cost_usd": 1.0})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	settled := pollJob(t, r, j.ID)
	if settled.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("bridge failure must settle FAILED, got %+v", settled)
	}
	if !strings.Contains(string(settled.ResultJSON), "bridge reports failure") {
		t.Fatalf("failure reason must name the bridge report: %s", settled.ResultJSON)
	}
}
