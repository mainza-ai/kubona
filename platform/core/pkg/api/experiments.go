package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
)

// experimentJobType is the single kind -> job-type mapping shared by the
// direct experiment route and the discovery protocol.
func experimentJobType(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "vqe":
		return "vqe_run", true
	case "pes":
		return "vqe_pes", true
	case "qaoa":
		return "qaoa_run", true
	case "excited_state":
		return "excited_state", true
	case "reaction":
		return "reaction", true
	case "spin_chain":
		return "spin_chain", true
	case "conjecture":
		return "conjecture_run", true
	default:
		return "", false
	}
}

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
	jobType, ok := experimentJobType(kind)
	if !ok {
		WriteAPIError(w, "bad_request", "unknown experiment kind: use vqe, pes, qaoa, excited_state, reaction, spin_chain, or conjecture", nil)
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
		Loop                bool     `json:"loop"`
		MaxIterations       int      `json:"max_iterations"`
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
		Layers      int     `json:"layers"`
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
	layerCount := res.Layers
	if layerCount < 1 || layerCount > 8 {
		layerCount = 1
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
		Layers: layerCount,
		Nodes:  res.Graph.Nodes, Edges: res.Graph.Edges,
		Loop: req.Loop, MaxIterations: req.MaxIterations,
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
	Loop                bool     `json:"loop"`
	MaxIterations       int      `json:"max_iterations"`
	Mitigation          string   `json:"mitigation"`
	MitigationScales    []int    `json:"mitigation_scales"`
	CallerJurisdictions []string `json:"caller_jurisdictions"`
	CallerExportClass   string   `json:"caller_export_class"`
	ExportReviewed      bool     `json:"export_reviewed"`
	RequestID           string   `json:"request_id"`
}

// hardwareVQEParamsFromSource builds the one canonical parent spec from a
// completed local VQE job (H2, active-space LiH/BeH2). It never executes anything.
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
		AnsatzQPY         string             `json:"ansatz_qpy"`
		ActiveSpace       struct {
			FullSpace bool `json:"full_space"`
		} `json:"active_space"`
	}
	if err := json.Unmarshal(source.ResultJSON, &res); err != nil ||
		!res.Success || res.Kind != "vqe" || res.Hardware ||
		(res.ActiveSpace.FullSpace) ||
		(strings.ToUpper(res.Molecule) != "H2" && strings.ToUpper(res.Molecule) != "LIH" && strings.ToUpper(res.Molecule) != "BEH2" && strings.ToUpper(res.Molecule) != "H2O") ||
		len(res.Theta) == 0 || len(res.PauliCoefficients) == 0 ||
		strings.TrimSpace(res.AnsatzQASM) == "" {
		return p, fmt.Errorf("source job carries no local VQE optimizer state and ansatz_qasm")
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
	layerCount := res.Layers
	if layerCount < 1 || layerCount > 8 {
		layerCount = 1
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
		Layers: layerCount, Shots: req.Shots, Seed: req.Seed,
		Theta: res.Theta, PauliCoefficients: res.PauliCoefficients,
		FCIExactEnergy: res.FCIExactEnergy, ConstantEnergy: res.ConstantEnergy,
		Provider: "ibm-qiskit", QASM: res.AnsatzQASM,
		Mitigation: req.Mitigation, MitigationScales: req.MitigationScales,
		Loop: req.Loop, MaxIterations: req.MaxIterations, AnsatzQPY: res.AnsatzQPY,
		RequestedBackend: backend, FidelityQASM: res.AnsatzQASM,
		MaxCostUSD: req.BudgetUSD, MandateRef: req.MandateRef,
		CallerJurisdictions: req.CallerJurisdictions,
		CallerExportClass:   req.CallerExportClass, ExportReviewed: req.ExportReviewed,
	}
	return p, nil
}

// POST /api/experiments/vqe/hardware/prepare — return the canonical parent
// mandate binding for a local active-space/H2 VQE source. No spend, no job, no dispatch.
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

// POST /api/experiments/vqe/hardware — execute a locally optimized active-space/H2 VQE
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

// spinChainHardwareIntent is shared by prepare and launch.
type spinChainHardwareIntent struct {
	SessionID           string   `json:"session_id"`
	SourceJobID         string   `json:"source_job_id"`
	Shots               int      `json:"shots"`
	Seed                int      `json:"seed"`
	BudgetUSD           float64  `json:"budget_usd"`
	Backend             string   `json:"backend"`
	MandateRef          string   `json:"mandate_ref"`
	TargetError         float64  `json:"target_error"`
	CallerJurisdictions []string `json:"caller_jurisdictions"`
	CallerExportClass   string   `json:"caller_export_class"`
	ExportReviewed      bool     `json:"export_reviewed"`
	RequestID           string   `json:"request_id"`
}

func (s *Server) spinChainHardwareParamsFromSource(req spinChainHardwareIntent) (jobs.HardwareExperimentParams, error) {
	var p jobs.HardwareExperimentParams
	source, err := s.jobRunner.GetJob(req.SourceJobID)
	if err != nil || source == nil || source.SessionID != req.SessionID {
		return p, fmt.Errorf("source spin-chain job not found in this session")
	}
	var res struct {
		Kind              string             `json:"kind"`
		Success           bool               `json:"success"`
		Hardware          bool               `json:"hardware"`
		Model             string             `json:"model"`
		NSites            int                `json:"n_sites"`
		Coupling          float64            `json:"coupling"`
		Field             float64            `json:"field"`
		Anisotropy        float64            `json:"anisotropy"`
		Periodic          bool               `json:"periodic"`
		Layers            int                `json:"layers"`
		TargetError       float64            `json:"target_error"`
		StateMode         string             `json:"state_mode"`
		EnergyExact       float64            `json:"energy_exact"`
		AnsatzQASM        string             `json:"ansatz_qasm"`
		PauliCoefficients map[string]float64 `json:"pauli_coefficients"`
	}
	if err := json.Unmarshal(source.ResultJSON, &res); err != nil ||
		source.JobType != "spin_chain" || !res.Success || res.Kind != "spin_chain" || res.Hardware ||
		res.NSites < 2 || res.NSites > 14 ||
		len(res.PauliCoefficients) == 0 || strings.TrimSpace(res.AnsatzQASM) == "" {
		return p, fmt.Errorf("source job carries no local spin-chain ansatz and Pauli coefficients")
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
	targetError := req.TargetError
	if targetError == 0 {
		targetError = res.TargetError
	}
	if targetError <= 0 || targetError > 1 {
		targetError = 1e-3
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = strings.TrimSpace(os.Getenv("KUBONA_IBM_QISKIT_BACKEND"))
	}
	if backend == "" {
		return p, fmt.Errorf("backend is required (or set KUBONA_IBM_QISKIT_BACKEND)")
	}
	p = jobs.HardwareExperimentParams{
		Kind: "spin_chain_hardware", Model: res.Model, NSites: res.NSites,
		TargetError: targetError, StateMode: res.StateMode, SourceJobID: req.SourceJobID,
		Layers: res.Layers, Shots: req.Shots, Seed: req.Seed,
		QASM: res.AnsatzQASM, FidelityQASM: res.AnsatzQASM,
		PauliCoefficients: res.PauliCoefficients, EnergyExact: res.EnergyExact,
		Provider: "ibm-qiskit", RequestedBackend: backend,
		MaxCostUSD: req.BudgetUSD, MandateRef: req.MandateRef,
		CallerJurisdictions: req.CallerJurisdictions,
		CallerExportClass:   req.CallerExportClass, ExportReviewed: req.ExportReviewed,
	}
	return p, nil
}

// POST /api/experiments/spin-chain/hardware/prepare — return the canonical
// parent mandate binding for a local spin-chain source.
func (s *Server) handlePrepareSpinChainHardware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to prepare spin-chain hardware", nil)
		return
	}
	var req spinChainHardwareIntent
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid spin-chain hardware request: %v", err), nil)
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
	p, err := s.spinChainHardwareParamsFromSource(req)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"binding": jobs.CanonicalHardwareJobBinding("spin_chain_hardware", sid, p),
		"spec": map[string]any{
			"model": p.Model, "n_sites": p.NSites, "provider": p.Provider,
			"requested_backend": p.RequestedBackend, "shots": p.Shots, "seed": p.Seed,
			"max_cost_usd": p.MaxCostUSD, "mandate_required": true,
		},
	})
}

// POST /api/experiments/spin-chain/hardware — run the locally optimized
// spin-chain ansatz on a governed QPU batch. The source job supplies the
// exact anchor, ansatz, and Pauli coefficients; the parent owns mandate,
// fidelity, reservation, batch, usage, and settlement.
func (s *Server) handleSpinChainHardware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to launch spin-chain hardware", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req spinChainHardwareIntent
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid spin-chain hardware request: %v", err), nil)
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
	p, err := s.spinChainHardwareParamsFromSource(req)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if p.MandateRef == "" {
		WriteAPIError(w, "bad_request", "mandate_ref is required: prepare the parent binding, sign it, then launch", map[string]any{
			"binding": jobs.CanonicalHardwareJobBinding("spin_chain_hardware", sid, p),
		})
		return
	}
	idem, iref := s.idemBegin(sid, req.RequestID, map[string]any{
		"session_id": sid, "source_job_id": req.SourceJobID, "backend": p.RequestedBackend,
		"shots": p.Shots, "seed": p.Seed, "budget_usd": p.MaxCostUSD,
		"mandate_ref": p.MandateRef, "target_error": p.TargetError,
		"jurisdictions": req.CallerJurisdictions,
	}, idemRailJob)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}
	job, err := s.jobRunner.SubmitJob(sid, "spin_chain_hardware", p)
	if err != nil {
		s.idemFinishError(w, idem, http.StatusBadRequest, err.Error())
		return
	}
	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{"job": jobViewFrom(job)})
}
