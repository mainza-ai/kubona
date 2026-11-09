package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
)

// POST /api/experiments — submit a real variational experiment (VQE or PES).
//
// The body carries a versioned spec; the Go worker submits a ledger-bound job
// and the bridge owns the PySCF/Qiskit execution. This is deliberately the
// same admission shape as /api/jobs: known session, dispatch autonomy,
// caller/session tenant binding, and content-bound idempotency. The job result
// folds into the campaign through the settled hook.
func (s *Server) handleExperiments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to submit an experiment", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req struct {
		SessionID string          `json:"session_id"`
		Kind      string          `json:"kind"`
		Spec      json.RawMessage `json:"spec"`
		BudgetUSD float64         `json:"budget_usd"`
		RequestID string          `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid experiment request: %v", err), nil)
		return
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required: experiments are spend-attributed", nil)
		return
	}
	if len(req.Spec) == 0 || string(req.Spec) == "null" {
		WriteAPIError(w, "bad_request", "spec is required (versioned experiment object)", nil)
		return
	}
	var spec map[string]any
	if err := json.Unmarshal(req.Spec, &spec); err != nil || spec == nil {
		WriteAPIError(w, "bad_request", "spec must be a JSON object", nil)
		return
	}
	kind, _ := spec["kind"].(string)
	if kind == "" {
		kind = req.Kind
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	var jobType string
	switch kind {
	case "vqe":
		jobType = "vqe_run"
	case "pes":
		jobType = "vqe_pes"
	case "qaoa":
		jobType = "qaoa_run"
	default:
		WriteAPIError(w, "bad_request", "unknown experiment kind: use vqe, pes, or qaoa", nil)
		return
	}
	spec["kind"] = kind
	if req.BudgetUSD <= 0 {
		req.BudgetUSD = 1.0
	}
	// The generic non-circuit admission planner requires a declared spend
	// envelope even for zero-cost simulator experiments; the envelope is
	// released on completion because the result reports no vendor usage.
	spec["max_cost_usd"] = req.BudgetUSD
	sid, serr := s.requireSession(req.SessionID)
	if serr != nil {
		http.Error(w, serr.Error(), http.StatusNotFound)
		return
	}
	req.SessionID = sid
	if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	params, _ := json.Marshal(spec)
	idem, iref := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"session_id": req.SessionID, "kind": kind, "spec": spec, "budget_usd": req.BudgetUSD,
	}, idemRailJob)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}
	job, err := s.jobRunner.SubmitJob(req.SessionID, jobType, json.RawMessage(params))
	if err != nil {
		s.idemFinishError(w, idem, http.StatusBadRequest, err.Error())
		return
	}
	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{"job": jobViewFrom(job)})
}

// POST /api/experiments/qaoa/hardware — launch the optimized simulator QAOA
// circuit on the governed circuit-run rail. The local QAOA job supplies the
// exact QASM and graph; this route does not run a second optimizer and does
// not bypass QPU admission: the circuit-run path still requires a verified
// spend mandate, declared jurisdiction, fidelity admission, and settlement.
func (s *Server) handleQAOAHardware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to launch hardware QAOA", nil)
		return
	}
	prepareOnly := strings.HasSuffix(r.URL.Path, "/prepare")
	if !prepareOnly && s.refuseIfDiskBlocked(w) {
		return
	}
	var req struct {
		SessionID           string   `json:"session_id"`
		SourceJobID         string   `json:"source_job_id"`
		Shots               int      `json:"shots"`
		Seed                int      `json:"seed"`
		BudgetUSD           float64  `json:"budget_usd"`
		Backend             string   `json:"backend"`
		MandateRef          string   `json:"mandate_ref"`
		GrantChain          []string `json:"grant_chain"`
		InputLabels         []string `json:"input_labels"`
		CallerJurisdictions []string `json:"caller_jurisdictions"`
		CallerExportClass   string   `json:"caller_export_class"`
		ExportReviewed      bool     `json:"export_reviewed"`
		RequestID           string   `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid hardware QAOA request: %v", err), nil)
		return
	}
	sid, err := s.requireSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", err.Error(), nil)
		return
	}
	if err := s.requireDispatchAutonomy(sid); err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
		return
	}
	source, err := s.jobRunner.GetJob(req.SourceJobID)
	if err != nil || source == nil || source.SessionID != sid {
		WriteAPIError(w, "not_found", "source QAOA job not found in this session", nil)
		return
	}
	var res struct {
		Kind        string  `json:"kind"`
		CircuitQASM string  `json:"circuit_qasm"`
		OptimalCut  int     `json:"optimal_cut"`
		EnergyExact float64 `json:"energy_exact"`
		Graph       struct {
			Nodes int      `json:"nodes"`
			Edges [][2]int `json:"edges"`
		} `json:"graph"`
	}
	if err := json.Unmarshal(source.ResultJSON, &res); err != nil || res.Kind != "qaoa" || strings.TrimSpace(res.CircuitQASM) == "" {
		WriteAPIError(w, "bad_request", "source job carries no optimized QAOA circuit_qasm", nil)
		return
	}
	if req.Shots <= 0 {
		req.Shots = 1024
	}
	if req.Seed <= 0 {
		req.Seed = 7
	}
	if req.BudgetUSD <= 0 {
		req.BudgetUSD = 1.0
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = strings.TrimSpace(os.Getenv("KUBONA_IBM_QISKIT_BACKEND"))
	}
	if backend == "" {
		WriteAPIError(w, "bad_request", "backend is required (or set KUBONA_IBM_QISKIT_BACKEND)", nil)
		return
	}
	p := jobs.HardwareExperimentParams{
		Kind: "qaoa_hardware", Provider: "ibm-qiskit", RequestedBackend: backend,
		QASM: res.CircuitQASM, FidelityQASM: res.CircuitQASM,
		OptimalCut: res.OptimalCut, EnergyExact: res.EnergyExact,
		Nodes: res.Graph.Nodes, Edges: res.Graph.Edges,
		Shots: req.Shots, Seed: req.Seed, MaxCostUSD: req.BudgetUSD,
		MandateRef: req.MandateRef, CallerJurisdictions: req.CallerJurisdictions,
		CallerExportClass: req.CallerExportClass, ExportReviewed: req.ExportReviewed,
	}
	binding := jobs.CanonicalHardwareJobBinding("qaoa_hardware", sid, p)
	if prepareOnly {
		writeJSON(w, http.StatusOK, map[string]any{"binding": binding, "spec": map[string]any{
			"kind": "qaoa_hardware", "provider": p.Provider, "requested_backend": p.RequestedBackend,
			"shots": p.Shots, "seed": p.Seed, "max_cost_usd": p.MaxCostUSD, "mandate_required": true,
		}})
		return
	}
	if p.MandateRef == "" {
		WriteAPIError(w, "bad_request", "mandate_ref is required: use the prepare flow or sign the parent binding first", map[string]any{
			"binding": binding,
		})
		return
	}
	idem, iref := s.idemBegin(sid, req.RequestID, map[string]any{
		"session_id": sid, "source_job_id": req.SourceJobID, "backend": backend,
		"shots": req.Shots, "seed": req.Seed, "budget_usd": req.BudgetUSD,
		"mandate_ref": req.MandateRef, "jurisdictions": req.CallerJurisdictions,
	}, idemRailJob)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}
	job, err := s.jobRunner.SubmitJob(sid, "qaoa_hardware", p)
	if err != nil {
		s.idemFinishError(w, idem, http.StatusBadRequest, err.Error())
		return
	}
	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{"job": jobViewFrom(job)})
}

// hardwareVQEIntent is the shared request shape for prepare and launch.
// Prepare returns the parent mandate binding; launch requires the resulting
// mandate_ref. Both load only a local source job in the same session.
type hardwareVQEIntent struct {
	SessionID           string   `json:"session_id"`
	SourceJobID         string   `json:"source_job_id"`
	Shots               int      `json:"shots"`
	Seed                int      `json:"seed"`
	BudgetUSD           float64  `json:"budget_usd"`
	Backend             string   `json:"backend"`
	MandateRef          string   `json:"mandate_ref"`
	CallerJurisdictions []string `json:"caller_jurisdictions"`
	CallerExportClass   string   `json:"caller_export_class"`
	ExportReviewed      bool     `json:"export_reviewed"`
	RequestID           string   `json:"request_id"`
}

// hardwareVQEParamsFromSource builds the one canonical parent spec from a
// completed local H2 VQE job. It never executes anything.
func (s *Server) hardwareVQEParamsFromSource(req hardwareVQEIntent) (jobs.HardwareExperimentParams, error) {
	var p jobs.HardwareExperimentParams
	source, err := s.jobRunner.GetJob(req.SourceJobID)
	if err != nil || source == nil || source.SessionID != req.SessionID {
		return p, fmt.Errorf("source VQE job not found in this session")
	}
	var res struct {
		Kind              string             `json:"kind"`
		Success           bool               `json:"success"`
		Hardware          bool               `json:"hardware"`
		Molecule          string             `json:"molecule"`
		BondDistance      float64            `json:"bond_distance"`
		Layers            int                `json:"layers"`
		FCIExactEnergy    float64            `json:"fci_exact_energy"`
		ConstantEnergy    float64            `json:"constant_energy"`
		Theta             []float64          `json:"theta"`
		PauliCoefficients map[string]float64 `json:"pauli_coefficients"`
		AnsatzQASM        string             `json:"ansatz_qasm"`
		ActiveSpace       struct {
			FullSpace bool `json:"full_space"`
		} `json:"active_space"`
	}
	if err := json.Unmarshal(source.ResultJSON, &res); err != nil ||
		!res.Success || res.Kind != "vqe" || res.Hardware ||
		(res.ActiveSpace.FullSpace) ||
		(strings.ToUpper(res.Molecule) != "H2" && strings.ToUpper(res.Molecule) != "LIH" && strings.ToUpper(res.Molecule) != "BEH2") ||
		len(res.Theta) == 0 || len(res.PauliCoefficients) == 0 ||
		strings.TrimSpace(res.AnsatzQASM) == "" {
		return p, fmt.Errorf("source job carries no local H2 VQE optimizer state and ansatz_qasm")
	}
	if req.Shots <= 0 {
		req.Shots = 1024
	}
	if req.Seed <= 0 {
		req.Seed = 7
	}
	if req.BudgetUSD <= 0 {
		req.BudgetUSD = 1.0
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = strings.TrimSpace(os.Getenv("KUBONA_IBM_QISKIT_BACKEND"))
	}
	if backend == "" {
		return p, fmt.Errorf("backend is required (or set KUBONA_IBM_QISKIT_BACKEND)")
	}
	p = jobs.HardwareExperimentParams{
		Kind: "vqe_hardware", Molecule: res.Molecule, BondDistance: res.BondDistance,
		Layers: res.Layers, Shots: req.Shots, Seed: req.Seed,
		Theta: res.Theta, PauliCoefficients: res.PauliCoefficients,
		FCIExactEnergy: res.FCIExactEnergy, ConstantEnergy: res.ConstantEnergy,
		Provider: "ibm-qiskit", QASM: res.AnsatzQASM,
		RequestedBackend: backend, FidelityQASM: res.AnsatzQASM,
		MaxCostUSD: req.BudgetUSD, MandateRef: req.MandateRef,
		CallerJurisdictions: req.CallerJurisdictions,
		CallerExportClass:   req.CallerExportClass, ExportReviewed: req.ExportReviewed,
	}
	return p, nil
}

// POST /api/experiments/vqe/hardware/prepare — return the canonical parent
// mandate binding for a local H2 VQE source. No spend, no job, no dispatch.
func (s *Server) handlePrepareHardwareVQE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to prepare hardware VQE", nil)
		return
	}
	var req hardwareVQEIntent
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid hardware VQE request: %v", err), nil)
		return
	}
	sid, err := s.requireSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", err.Error(), nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
		return
	}
	req.SessionID = sid
	p, err := s.hardwareVQEParamsFromSource(req)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"binding": jobs.CanonicalHardwareExperimentBinding(sid, p),
		"spec": map[string]any{
			"molecule": p.Molecule, "provider": p.Provider,
			"requested_backend": p.RequestedBackend, "shots": p.Shots, "seed": p.Seed,
			"max_cost_usd": p.MaxCostUSD, "mandate_required": true,
		},
	})
}

// POST /api/experiments/vqe/hardware — execute a locally optimized H2 VQE
// ansatz on a real QPU as one parent batch. The source local job supplies the
// optimizer state and ansatz QASM; the Go parent owns the experiment mandate,
// reservation, vendor batch, settlement, and claim evidence.
func (s *Server) handleHardwareVQE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to launch hardware VQE", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req hardwareVQEIntent
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid hardware VQE request: %v", err), nil)
		return
	}
	sid, err := s.requireSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", err.Error(), nil)
		return
	}
	if err := s.requireDispatchAutonomy(sid); err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
		return
	}
	req.SessionID = sid
	p, err := s.hardwareVQEParamsFromSource(req)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if p.MandateRef == "" {
		WriteAPIError(w, "bad_request", "mandate_ref is required: prepare the parent binding, sign it, then launch", map[string]any{
			"binding": jobs.CanonicalHardwareExperimentBinding(sid, p),
		})
		return
	}
	idem, iref := s.idemBegin(sid, req.RequestID, map[string]any{
		"session_id": sid, "source_job_id": req.SourceJobID, "backend": p.RequestedBackend,
		"shots": p.Shots, "seed": p.Seed, "budget_usd": p.MaxCostUSD,
		"mandate_ref": p.MandateRef, "jurisdictions": p.CallerJurisdictions,
	}, idemRailJob)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}
	job, err := s.jobRunner.SubmitJob(sid, "hardware_experiment", p)
	if err != nil {
		s.idemFinishError(w, idem, http.StatusBadRequest, err.Error())
		return
	}
	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{"job": jobViewFrom(job)})
}
