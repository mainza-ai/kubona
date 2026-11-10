package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
