package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExperimentKindMappingCoversHardwareAndScienceKinds(t *testing.T) {
	cases := map[string]string{
		"vqe": "vqe_run", "geometry_optimization": "geometry_optimization",
		"vqe_repeats": "vqe_repeats", "pes": "vqe_pes", "qaoa": "qaoa_run",
		"excited_state": "excited_state", "reaction": "reaction",
		"spin_chain": "spin_chain", "thermodynamic_limit": "thermodynamic_limit", "lattice_dynamics": "lattice_dynamics",
		"conjecture": "conjecture_run",
	}
	for kind, want := range cases {
		if got, ok := experimentJobType(kind); !ok || got != want {
			t.Fatalf("experimentJobType(%q) = %q ok=%v, want %q", kind, got, ok, want)
		}
	}
	if _, ok := experimentJobType("unknown"); ok {
		t.Fatal("unknown experiment kind must refuse")
	}
}

func seedCompletedJob(t *testing.T, srv *Server, id, jobType, result string) {
	t.Helper()
	sess, err := srv.sessionMgr.CreateSession("hardware route "+id, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ledger.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, sess.ID, "JOB_STATUS_COMPLETED", jobType, "{}", result); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {})
}

func TestHardwarePESRoutesPrepareAndRefuseUnsignedLaunch(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_hw_pes_routes.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("PES routes", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"pes","result_version":"experiment-result-v1","molecule":"H2","hardware":false,"points":[{"bond_distance":0.7,"ansatz_qasm":"OPENQASM 2.0; qreg q[2];","pauli_coefficients":{"II":-1},"fci_exact_energy":-1.0,"constant_energy":0.0},{"bond_distance":0.8,"ansatz_qasm":"OPENQASM 2.0; qreg q[2];","pauli_coefficients":{"II":-1},"fci_exact_energy":-1.0,"constant_energy":0.0}]}`
	if _, err := srv.ledger.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-pes-routes", sess.ID, "JOB_STATUS_COMPLETED", "vqe_pes", "{}", result); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"session_id":"` + sess.ID + `","source_job_id":"job-pes-routes","backend":"ibm_fez","budget_usd":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/experiments/pes/hardware/prepare", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"available":true`) || !strings.Contains(w.Body.String(), `"binding"`) {
		t.Fatalf("PES prepare: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/experiments/pes/hardware", strings.NewReader(string(body)))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "mandate_ref") {
		t.Fatalf("unsigned PES launch must refuse: %d %s", w.Code, w.Body.String())
	}
}

func TestHardwareSpinRoutesPrepareAndRefuseUnsignedLaunch(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_hw_spin_routes.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("spin routes", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"spin_chain","hardware":false,"model":"tfim","n_sites":4,"coupling":1,"field":0.8,"periodic":true,"layers":2,"target_error":0.01,"state_mode":"vqe","energy_exact":-5.2,"ansatz_qasm":"OPENQASM 2.0; qreg q[4];","pauli_coefficients":{"ZZII":1.0}}`
	if _, err := srv.ledger.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-spin-routes", sess.ID, "JOB_STATUS_COMPLETED", "spin_chain", "{}", result); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"session_id":"` + sess.ID + `","source_job_id":"job-spin-routes","backend":"ibm_fez","budget_usd":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/experiments/spin-chain/hardware/prepare", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"binding"`) {
		t.Fatalf("spin prepare: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/experiments/spin-chain/hardware", strings.NewReader(string(body)))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "mandate_ref") {
		t.Fatalf("unsigned spin launch must refuse: %d %s", w.Code, w.Body.String())
	}
}

func TestHardwareQAOARoutesPrepareAndRefuseUnsignedLaunch(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_hw_qaoa_routes.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("qaoa routes", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	result := `{"success":true,"kind":"qaoa","result_version":"experiment-result-v1","circuit_qasm":"OPENQASM 2.0; qreg q[2];","optimal_cut":1,"graph":{"nodes":2,"edges":[[0,1]]},"layers":1}`
	if _, err := srv.ledger.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"job-qaoa-routes", sess.ID, "JOB_STATUS_COMPLETED", "qaoa_run", "{}", result); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"session_id":"` + sess.ID + `","source_job_id":"job-qaoa-routes","backend":"ibm_fez","budget_usd":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/experiments/qaoa/hardware/prepare", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"binding"`) {
		t.Fatalf("QAOA prepare: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/experiments/qaoa/hardware", strings.NewReader(string(body)))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "mandate_ref") {
		t.Fatalf("unsigned QAOA launch must refuse: %d %s", w.Code, w.Body.String())
	}
}

func TestExperimentRouteRefusesUnknownKind(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_experiment_kind_refusal.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("kind refusal", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"session_id": sess.ID, "spec": map[string]any{"kind": "unknown"}, "budget_usd": 1})
	req := httptest.NewRequest(http.MethodPost, "/api/experiments", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknown experiment kind") {
		t.Fatalf("unknown kind: %d %s", w.Code, w.Body.String())
	}
}
