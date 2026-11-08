package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req struct {
		SessionID           string   `json:"session_id"`
		SourceJobID         string   `json:"source_job_id"`
		Shots               int      `json:"shots"`
		Seed                int      `json:"seed"`
		BudgetUSD           float64  `json:"budget_usd"`
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
	params := map[string]any{
		"qasm": res.CircuitQASM, "requested_backend": "ibm-qiskit",
		"shots": req.Shots, "seed": req.Seed, "max_cost_usd": req.BudgetUSD,
		"mandate_ref": req.MandateRef, "grant_chain": req.GrantChain,
		"input_labels": req.InputLabels, "caller_jurisdictions": req.CallerJurisdictions,
		"caller_export_class": req.CallerExportClass, "export_reviewed": req.ExportReviewed,
		"qaoa_meta": map[string]any{
			"source_job_id": req.SourceJobID, "optimal_cut": res.OptimalCut,
			"energy_exact": res.EnergyExact, "nodes": res.Graph.Nodes, "edges": res.Graph.Edges,
		},
	}
	idem, iref := s.idemBegin(sid, req.RequestID, map[string]any{
		"session_id": sid, "source_job_id": req.SourceJobID, "backend": "ibm-qiskit",
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
	job, err := s.jobRunner.SubmitJob(sid, "circuit_run", params)
	if err != nil {
		s.idemFinishError(w, idem, http.StatusBadRequest, err.Error())
		return
	}
	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{"job": jobViewFrom(job)})
}
