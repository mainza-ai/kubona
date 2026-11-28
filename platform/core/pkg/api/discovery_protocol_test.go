package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	pb "github.com/mainza-ai/kubona/core/proto"
)

func newResponseRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func httptestRequest(method, target string, body []byte) *http.Request {
	return httptest.NewRequest(method, target, bytes.NewReader(body))
}

func TestPlanDiscoverySelectsMeasuredKinds(t *testing.T) {
	cases := []struct {
		goal string
		kind string
	}{
		{"measure the H2 excited state gap", "excited_state"},
		{"transverse-field Ising spin chain", "spin_chain"},
		{"Heisenberg chain ground energy", "spin_chain"},
		{"H2 potential energy surface", "pes"},
		{"water active-space VQE", "vqe"},
		{"ground state baseline", "vqe"},
		{"TFIM DMRG thermodynamic limit", "thermodynamic_limit"},
		{"2D Ising quench dynamics", "lattice_dynamics"},
		{"LiH geometry relaxation", "geometry_optimization"},
		{"H2 multi-seed reproducibility repeats", "vqe_repeats"},
		{"H2 reaction energy", "reaction"},
		{"MaxCut QAOA graph", "qaoa"},
		{"XXZ spin chain", "spin_chain"},
	}
	for _, tc := range cases {
		steps, reason, err := planDiscovery(tc.goal)
		if err != nil {
			t.Fatalf("%q: %v", tc.goal, err)
		}
		if len(steps) != 1 || steps[0].Kind != tc.kind {
			t.Fatalf("%q: got %+v (%s)", tc.goal, steps, reason)
		}
	}

	multiSteps, reason, err := planDiscovery("LiH geometry optimization then PES dissociation scan then excited state gap")
	if err != nil {
		t.Fatalf("multi-step plan failed: %v", err)
	}
	if len(multiSteps) != 3 || multiSteps[0].Kind != "geometry_optimization" ||
		multiSteps[1].Kind != "pes" || multiSteps[2].Kind != "excited_state" ||
		multiSteps[1].Spec["molecule"] != "LiH" {
		t.Fatalf("multi-step plan wrong: %+v (%s)", multiSteps, reason)
	}
}

func TestPlanDiscoveryRefusesUnboundedConjecture(t *testing.T) {
	_, _, err := planDiscovery("find a new conjecture")
	if err == nil || !strings.Contains(err.Error(), "explicit values") {
		t.Fatalf("conjecture plan must refuse without data, got %v", err)
	}
	if _, _, err := planDiscovery("  "); err == nil {
		t.Fatal("empty goal must refuse")
	}
}

func TestTerminalJobStatus(t *testing.T) {
	for _, status := range []pb.JobStatus{
		pb.JobStatus_JOB_STATUS_COMPLETED,
		pb.JobStatus_JOB_STATUS_FAILED,
		pb.JobStatus_JOB_STATUS_CANCELLED,
	} {
		if !terminalJobStatus(status) {
			t.Fatalf("%s must be terminal", status)
		}
	}
	for _, status := range []pb.JobStatus{
		pb.JobStatus_JOB_STATUS_QUEUED,
		pb.JobStatus_JOB_STATUS_RUNNING,
	} {
		if terminalJobStatus(status) {
			t.Fatalf("%s must not be terminal", status)
		}
	}
}

func TestDiscoveryHandlersPlanAndFailurePath(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_discovery_protocol.db")
	defer cleanup()

	planBody := []byte(`{"goal":"transverse-field Ising spin chain"}`)
	planRec := newResponseRecorder()
	srv.handleDiscoveryPlan(planRec, httptestRequest(http.MethodPost, "/api/discovery/plan", planBody))
	if planRec.Code != http.StatusOK {
		t.Fatalf("plan status %d: %s", planRec.Code, planRec.Body.String())
	}
	var plan map[string]any
	if err := json.Unmarshal(planRec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan["ok"] != true || plan["steps"] == nil {
		t.Fatalf("plan payload wrong: %v", plan)
	}

	sess, err := srv.sessionMgr.CreateSession("discovery drill", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"session_id": sess.ID,
		"goal":       "run a measured experiment",
		"steps": []map[string]any{{
			"kind": "conjecture",
			"spec": map[string]any{"values": []int{1, 1, 2, 3, 5}, "holdout": 1},
		}},
		"step_timeout_s": 5,
	})
	// M0 (Doc 2 #4 / B11): DefaultPolicy has StageExecute=approve, so an unapproved
	// /api/discovery/run MUST be refused with 409 plan_not_approved.
	refusedRec := newResponseRecorder()
	srv.handleDiscoveryRun(refusedRec, httptestRequest(http.MethodPost, "/api/discovery/run", body))
	if refusedRec.Code != http.StatusConflict || !strings.Contains(refusedRec.Body.String(), "plan_not_approved") {
		t.Fatalf("expected 409 plan_not_approved in default approve mode, got %d: %s", refusedRec.Code, refusedRec.Body.String())
	}

	// Switch session to ModeAct to exercise execution + registered ledger event trail.
	actPol := autonomy.DefaultPolicy()
	actPol.Stages[autonomy.StageExecute] = autonomy.ModeAct
	polRaw, _ := json.Marshal(actPol)
	if err := srv.ledger.SetAutonomyPolicy(sess.ID, string(polRaw)); err != nil {
		t.Fatal(err)
	}
	// High-risk / over-budget step in ModeAct MUST still require explicit approval.
	overBudgetBody, _ := json.Marshal(map[string]any{
		"session_id": sess.ID,
		"goal":       "run expensive hardware experiment",
		"steps": []map[string]any{{
			"kind":       "conjecture",
			"budget_usd": 50.0,
			"spec":       map[string]any{"values": []int{1, 1, 2, 3, 5}, "holdout": 1},
		}},
		"step_timeout_s": 5,
	})
	highRiskRec := newResponseRecorder()
	srv.handleDiscoveryRun(highRiskRec, httptestRequest(http.MethodPost, "/api/discovery/run", overBudgetBody))
	if highRiskRec.Code != http.StatusConflict || !strings.Contains(highRiskRec.Body.String(), "risk_approval_required") {
		t.Fatalf("expected 409 risk_approval_required for over-budget step in act mode, got %d: %s", highRiskRec.Code, highRiskRec.Body.String())
	}

	rec := newResponseRecorder()
	srv.handleDiscoveryRun(rec, httptestRequest(http.MethodPost, "/api/discovery/run", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("run status %d: %s", rec.Code, rec.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run["ok"] != true || run["steps_failed"] != float64(1) {
		t.Fatalf("bridge-less step must be recorded as failed, got %v", run)
	}
	// M0 (Doc 2 #8): verify discovery_protocol_* and discovery_step events persisted.
	events, err := srv.ledger.GetRecentEvents(sess.ID, 50)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	seenTypes := map[string]bool{}
	for _, ev := range events {
		seenTypes[ev.EventType] = true
	}
	for _, want := range []string{"discovery_protocol_started", "discovery_step", "discovery_protocol_completed"} {
		if !seenTypes[want] {
			t.Fatalf("M0 regression: expected ledger event %q to be recorded, saw %v", want, seenTypes)
		}
	}
}

func TestSpinChainHardwareParamsFromSource(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_spin_hardware_source.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("spin hardware source", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"spin_chain","hardware":false,"model":"tfim","n_sites":4,"coupling":1,"field":0.8,"periodic":true,"layers":2,"target_error":0.01,"state_mode":"vqe","energy_exact":-5.2,"ansatz_qasm":"OPENQASM 2.0; qreg q[4];","pauli_coefficients":{"ZZII":1.0}}`
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-spin-source", sess.ID, "JOB_STATUS_COMPLETED", "spin_chain", "{}", result); err != nil {
		t.Fatal(err)
	}
	p, err := srv.spinChainHardwareParamsFromSource(spinChainHardwareIntent{
		SessionID: sess.ID, SourceJobID: "job-spin-source", Backend: "ibm_fez", BudgetUSD: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "spin_chain_hardware" || p.Model != "tfim" || p.NSites != 4 || p.QASM == "" || len(p.PauliCoefficients) != 1 {
		t.Fatalf("hardware params wrong: %+v", p)
	}
	if _, err := srv.spinChainHardwareParamsFromSource(spinChainHardwareIntent{
		SessionID: sess.ID, SourceJobID: "missing", Backend: "ibm_fez",
	}); err == nil {
		t.Fatal("missing source must refuse")
	}
	exactResult := strings.Replace(result, `"state_mode":"vqe"`, `"state_mode":"exact"`, 1)
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-spin-exact", sess.ID, "JOB_STATUS_COMPLETED", "spin_chain", "{}", exactResult); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.spinChainHardwareParamsFromSource(spinChainHardwareIntent{
		SessionID: sess.ID, SourceJobID: "job-spin-exact", Backend: "ibm_fez",
	}); err == nil || !strings.Contains(err.Error(), "state_mode=vqe") {
		t.Fatalf("exact-state source must refuse before dispatch, got: %v", err)
	}
}

func TestHardwarePESParamsRequirePerPointCircuits(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_hardware_pes_source.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("hardware PES source", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"pes","result_version":"experiment-result-v1","molecule":"H2","hardware":false,"points":[{"bond_distance":0.7,"ansatz_qasm":"OPENQASM 2.0; qreg q[2];","pauli_coefficients":{"II":-1},"fci_exact_energy":-1.0,"constant_energy":0.0},{"bond_distance":0.8,"ansatz_qasm":"OPENQASM 2.0; qreg q[2];","pauli_coefficients":{"II":-1},"fci_exact_energy":-1.0,"constant_energy":0.0}]}`
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-pes-source", sess.ID, "JOB_STATUS_COMPLETED", "vqe_pes", "{}", result); err != nil {
		t.Fatal(err)
	}
	p, err := srv.hardwarePESParamsFromSource(hardwarePESIntent{
		SessionID: sess.ID, SourceJobID: "job-pes-source", Backend: "ibm_fez", BudgetUSD: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "hardware_pes" || len(p.Points) != 2 || p.Points[0].QASM == "" {
		t.Fatalf("hardware PES params wrong: %+v", p)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-pes-no-qasm", sess.ID, "JOB_STATUS_COMPLETED", "vqe_pes", "{}", strings.Replace(result, `"ansatz_qasm":"OPENQASM 2.0; qreg q[2];"`, `"ansatz_qasm":""`, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.hardwarePESParamsFromSource(hardwarePESIntent{
		SessionID: sess.ID, SourceJobID: "job-pes-no-qasm", Backend: "ibm_fez",
	}); err == nil || !strings.Contains(err.Error(), "ansatz_qasm") {
		t.Fatalf("PES without per-point qasm must refuse, got: %v", err)
	}
}
