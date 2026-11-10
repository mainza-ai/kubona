package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	pb "github.com/mainza-ai/kubona/core/proto"
)

type discoveryStep struct {
	Kind         string         `json:"kind"`
	Spec         map[string]any `json:"spec"`
	BudgetUSD    float64        `json:"budget_usd"`
	RequestID    string         `json:"request_id"`
	AllowFailure bool           `json:"allow_failure"`
}

type discoveryRequest struct {
	SessionID    string          `json:"session_id"`
	Goal         string          `json:"goal"`
	Steps        []discoveryStep `json:"steps"`
	StepTimeout  int             `json:"step_timeout_s"`
	RequestID    string          `json:"request_id"`
	AllowFailure bool            `json:"allow_failure"`
}

type discoveryStepResult struct {
	JobID  string          `json:"job_id"`
	Kind   string          `json:"kind"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func targetMoleculeFromGoal(g string) (string, float64) {
	switch {
	case strings.Contains(g, "water") || strings.Contains(g, "h2o"):
		return "H2O", 0.96
	case strings.Contains(g, "beh2"):
		return "BeH2", 1.326
	case strings.Contains(g, "lih"):
		return "LiH", 1.595
	default:
		return "H2", 0.7414
	}
}

func planSingleDiscoveryClause(g, defaultMol string, defaultBond float64) (discoveryStep, string, error) {
	mol, bond := targetMoleculeFromGoal(g)
	if mol == "H2" && !strings.Contains(g, "h2") && defaultMol != "" {
		mol, bond = defaultMol, defaultBond
	}
	switch {
	case strings.Contains(g, "conjecture") || strings.Contains(g, "sequence"):
		return discoveryStep{}, "", fmt.Errorf("conjecture goals need explicit values; supply steps with the exact sequence terms")
	case strings.Contains(g, "thermodynamic") || strings.Contains(g, "dmrg"):
		model := "tfim"
		if strings.Contains(g, "heisenberg") {
			model = "heisenberg"
		} else if strings.Contains(g, "xxz") {
			model = "xxz"
		}
		return discoveryStep{Kind: "thermodynamic_limit", Spec: map[string]any{
			"kind": "thermodynamic_limit", "model": model, "coupling": 1.0,
			"field": 1.0, "bond_dim": 32,
		}}, fmt.Sprintf("%s thermodynamic-limit DMRG plan", model), nil
	case strings.Contains(g, "quench") || strings.Contains(g, "dynamics"):
		return discoveryStep{Kind: "lattice_dynamics", Spec: map[string]any{
			"kind": "lattice_dynamics", "coupling": 1.0,
			"field_initial": 2.0, "field_final": 0.5,
		}}, "2D TFIM lattice-dynamics quench plan", nil
	case strings.Contains(g, "geometry") || strings.Contains(g, "relax"):
		return discoveryStep{Kind: "geometry_optimization", Spec: map[string]any{
			"kind": "geometry_optimization", "molecule": mol, "bond_distance": bond,
			"layers": 2, "shots": 16384, "max_iterations": 80,
		}}, fmt.Sprintf("%s geometry-optimization plan", mol), nil
	case strings.Contains(g, "repeat") || strings.Contains(g, "reproducib"):
		return discoveryStep{Kind: "vqe_repeats", Spec: map[string]any{
			"kind": "vqe_repeats", "molecule": mol, "bond_distance": bond,
			"repeats": 3, "layers": 2, "shots": 16384, "max_iterations": 80,
		}}, fmt.Sprintf("%s multi-seed VQE repeats plan", mol), nil
	case strings.Contains(g, "reaction"):
		return discoveryStep{Kind: "reaction", Spec: map[string]any{
			"kind": "reaction", "reaction": "H2_plus_H2_to_2H2",
			"layers": 2, "shots": 8192, "max_iterations": 60,
		}}, "reaction-energy VQE plan", nil
	case strings.Contains(g, "qaoa") || strings.Contains(g, "maxcut"):
		return discoveryStep{Kind: "qaoa", Spec: map[string]any{
			"kind": "qaoa", "layers": 2, "shots": 8192,
		}}, "QAOA MaxCut plan", nil
	case strings.Contains(g, "excited"):
		return discoveryStep{Kind: "excited_state", Spec: map[string]any{
			"kind": "excited_state", "molecule": mol, "bond_distance": bond,
			"layers": 2, "shots": 16384, "max_iterations": 80, "penalty": 10.0,
		}}, fmt.Sprintf("excited-state %s plan", mol), nil
	case strings.Contains(g, "xxz"):
		return discoveryStep{Kind: "spin_chain", Spec: map[string]any{
			"kind": "spin_chain", "model": "xxz", "n_sites": 4,
			"coupling": 1.0, "field": 0.0, "anisotropy": 1.5, "periodic": true,
			"layers": 3, "shots": 16384, "max_iterations": 180,
		}}, "XXZ spin-chain plan", nil
	case strings.Contains(g, "heisenberg"):
		return discoveryStep{Kind: "spin_chain", Spec: map[string]any{
			"kind": "spin_chain", "model": "heisenberg", "n_sites": 4,
			"coupling": 1.0, "field": 0.0, "periodic": true,
			"layers": 3, "shots": 16384, "max_iterations": 180,
		}}, "Heisenberg spin-chain plan", nil
	case strings.Contains(g, "ising") || strings.Contains(g, "tfim") || strings.Contains(g, "spin"):
		return discoveryStep{Kind: "spin_chain", Spec: map[string]any{
			"kind": "spin_chain", "model": "tfim", "n_sites": 4,
			"coupling": 1.0, "field": 0.8, "periodic": true,
			"layers": 3, "shots": 16384, "max_iterations": 180,
		}}, "transverse-field Ising spin-chain plan", nil
	case strings.Contains(g, "pes") || strings.Contains(g, "dissociation") || strings.Contains(g, "potential"):
		return discoveryStep{Kind: "pes", Spec: map[string]any{
			"kind": "pes", "molecule": mol, "r_min": 0.5, "r_max": 2.0,
			"steps": 5, "layers": 2, "shots": 8192, "max_iterations": 60,
		}}, fmt.Sprintf("%s potential-energy scan plan", mol), nil
	case strings.Contains(g, "water") || strings.Contains(g, "h2o") || mol == "H2O":
		return discoveryStep{Kind: "vqe", Spec: map[string]any{
			"kind": "vqe", "molecule": "H2O", "bond_distance": 0.96,
			"layers": 2, "shots": 8192, "max_iterations": 60, "full_space": false,
		}}, "H2O active-space VQE plan", nil
	default:
		return discoveryStep{Kind: "vqe", Spec: map[string]any{
			"kind": "vqe", "molecule": mol, "bond_distance": bond,
			"layers": 2, "shots": 16384, "max_iterations": 80, "full_space": false,
		}}, fmt.Sprintf("%s ground-state VQE baseline plan", mol), nil
	}
}

// planDiscovery is a deterministic, bounded planner. It never invents a
// sequence for a conjecture or an arbitrary molecule; ambiguous goals fall
// through to the target molecule VQE baseline or refuse when data is required.
// Multi-clause goals separated by " then ", " -> ", or ";" produce up to 5 steps.
func planDiscovery(goal string) ([]discoveryStep, string, error) {
	g := strings.ToLower(strings.TrimSpace(goal))
	if g == "" {
		return nil, "", fmt.Errorf("goal is required")
	}
	defaultMol, defaultBond := targetMoleculeFromGoal(g)
	normalized := strings.ReplaceAll(g, " and then ", " then ")
	normalized = strings.ReplaceAll(normalized, " -> ", " then ")
	normalized = strings.ReplaceAll(normalized, ";", " then ")
	rawClauses := strings.Split(normalized, " then ")
	clauses := make([]string, 0, len(rawClauses))
	for _, c := range rawClauses {
		if trimmed := strings.TrimSpace(c); trimmed != "" {
			clauses = append(clauses, trimmed)
		}
	}
	if len(clauses) > 5 {
		return nil, "", fmt.Errorf("discovery protocol requires 1..5 steps")
	}
	steps := make([]discoveryStep, 0, len(clauses))
	reasons := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		step, reason, err := planSingleDiscoveryClause(clause, defaultMol, defaultBond)
		if err != nil {
			return nil, "", err
		}
		steps = append(steps, step)
		reasons = append(reasons, reason)
	}
	return steps, strings.Join(reasons, " -> "), nil
}

func terminalJobStatus(status pb.JobStatus) bool {
	switch status {
	case pb.JobStatus_JOB_STATUS_COMPLETED, pb.JobStatus_JOB_STATUS_FAILED, pb.JobStatus_JOB_STATUS_CANCELLED:
		return true
	default:
		return false
	}
}

func (s *Server) waitForJob(sessionID, jobID string, timeout time.Duration) (*jobs.Job, error) {
	deadline := time.Now().Add(timeout)
	for {
		j, err := s.jobRunner.GetJob(jobID)
		if err == nil && j != nil && j.SessionID == sessionID && terminalJobStatus(j.Status) {
			return j, nil
		}
		if time.Now().After(deadline) {
			return j, fmt.Errorf("job %s did not reach a terminal state within %s", jobID, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// POST /api/discovery/run — bounded measured-discovery protocol.
// Steps are submitted to the real job rail in order; each result is the
// stored job result (no LLM synthesis, no invented fields). When steps are
// omitted a deterministic planner selects one measured experiment from the
// goal, or refuses if the goal needs caller-supplied data.
func (s *Server) handleDiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required for discovery protocol", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req discoveryRequest
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid discovery request: %v", err), nil)
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
	steps := req.Steps
	plannedBy := "caller"
	if len(steps) == 0 {
		planned, reason, perr := planDiscovery(req.Goal)
		if perr != nil {
			WriteAPIError(w, "bad_request", perr.Error(), nil)
			return
		}
		steps = planned
		plannedBy = reason
	}
	if len(steps) == 0 || len(steps) > 5 {
		WriteAPIError(w, "bad_request", "discovery protocol requires 1..5 steps", nil)
		return
	}
	timeout := time.Duration(req.StepTimeout) * time.Second
	if req.StepTimeout <= 0 {
		timeout = 5 * time.Minute
	}
	if timeout > 15*time.Minute {
		timeout = 15 * time.Minute
	}
	var idem *idemTicket
	if req.RequestID != "" {
		var refusal *idemRefusal
		idem, refusal = s.idemBegin(sid, req.RequestID, req, idemRailSlow)
		if refusal != nil {
			refusal.write(w)
			return
		}
		if idem.replay {
			s.idemReplay(w, idem)
			return
		}
	}
	protocolID := fmt.Sprintf("disc-%d", time.Now().UTC().UnixNano())
	_ = s.ledger.AppendEvent(sid, "discovery_protocol_started", map[string]any{
		"protocol_id": protocolID, "goal": req.Goal, "planned_by": plannedBy,
		"steps": len(steps),
	})
	results := make([]discoveryStepResult, 0, len(steps))
	stopped := false
	for i, step := range steps {
		kind := strings.ToLower(strings.TrimSpace(step.Kind))
		if kind == "" {
			kind, _ = step.Spec["kind"].(string)
			kind = strings.ToLower(strings.TrimSpace(kind))
		}
		jobType, ok := experimentJobType(kind)
		if !ok {
			WriteAPIError(w, "bad_request", fmt.Sprintf("step %d: unknown experiment kind %q", i, kind), nil)
			_ = s.ledger.AppendEvent(sid, "discovery_protocol_failed", map[string]any{
				"protocol_id": protocolID, "step": i, "reason": "unknown_kind",
			})
			if idem != nil && idem.active {
				_ = s.ledger.IdemDelete(idem.sessionID, idem.requestID)
			}
			return
		}
		spec := step.Spec
		if spec == nil {
			spec = map[string]any{}
		}
		spec["kind"] = kind
		budget := step.BudgetUSD
		if budget <= 0 {
			budget = 1.0
		}
		spec["max_cost_usd"] = budget
		job, err := s.jobRunner.SubmitJob(sid, jobType, spec)
		if err != nil {
			result := discoveryStepResult{Kind: kind, Status: "refused", Error: err.Error()}
			results = append(results, result)
			_ = s.ledger.AppendEvent(sid, "discovery_step", map[string]any{
				"protocol_id": protocolID, "step": i, "kind": kind,
				"status": "refused", "error": err.Error(),
			})
			if !step.AllowFailure && !req.AllowFailure {
				stopped = true
				break
			}
			continue
		}
		final, waitErr := s.waitForJob(sid, job.ID, timeout)
		result := discoveryStepResult{JobID: job.ID, Kind: kind}
		if waitErr != nil {
			result.Status = "timeout"
			result.Error = waitErr.Error()
			_ = s.ledger.AppendEvent(sid, "discovery_step", map[string]any{
				"protocol_id": protocolID, "step": i, "kind": kind,
				"job_id": job.ID, "status": "timeout", "error": waitErr.Error(),
			})
			results = append(results, result)
			if !step.AllowFailure && !req.AllowFailure {
				stopped = true
				break
			}
			continue
		}
		result.Status = final.Status.String()
		if len(final.ResultJSON) > 0 {
			result.Result = final.ResultJSON
		}
		_ = s.ledger.AppendEvent(sid, "discovery_step", map[string]any{
			"protocol_id": protocolID, "step": i, "kind": kind,
			"job_id": job.ID, "status": result.Status,
		})
		results = append(results, result)
		if final.Status != pb.JobStatus_JOB_STATUS_COMPLETED && !step.AllowFailure && !req.AllowFailure {
			stopped = true
			break
		}
	}
	completed, failed, claimable, measured := 0, 0, 0, 0
	for _, result := range results {
		if result.Status == pb.JobStatus_JOB_STATUS_COMPLETED.String() {
			completed++
		} else {
			failed++
		}
		var probe struct {
			Claimable bool `json:"claimable"`
			Measured  bool `json:"measured"`
		}
		if len(result.Result) > 0 && json.Unmarshal(result.Result, &probe) == nil {
			if probe.Claimable {
				claimable++
			}
			if probe.Measured {
				measured++
			}
		}
	}
	stepReports := make([]map[string]any, 0, len(results))
	for _, result := range results {
		report := map[string]any{
			"kind": result.Kind, "status": result.Status,
		}
		if result.JobID != "" {
			report["job_id"] = result.JobID
		}
		if result.Error != "" {
			report["error"] = result.Error
		}
		if len(result.Result) > 0 {
			var probe struct {
				Claimable bool `json:"claimable"`
				Measured  bool `json:"measured"`
			}
			if json.Unmarshal(result.Result, &probe) == nil {
				report["measured"] = probe.Measured
				report["claimable"] = probe.Claimable
			}
		}
		stepReports = append(stepReports, report)
	}
	payload := map[string]any{
		"ok": true, "protocol_id": protocolID, "goal": req.Goal,
		"planned_by": plannedBy, "steps_requested": len(steps),
		"steps_completed": completed, "steps_failed": failed,
		"measured_results": measured, "claimable_results": claimable,
		"stopped_on_failure": stopped, "results": results,
	}
	_ = s.ledger.AppendEvent(sid, "discovery_protocol_completed", map[string]any{
		"protocol_id": protocolID, "goal": req.Goal, "planned_by": plannedBy,
		"completed": completed, "failed": failed,
		"measured": measured, "claimable": claimable, "stopped": stopped,
		"steps": stepReports,
	})
	if idem != nil && idem.active {
		s.idemFinishJSON(w, idem, http.StatusOK, payload)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// POST /api/discovery/plan — inspect the deterministic plan without running.
func (s *Server) handleDiscoveryPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required for discovery plan", nil)
		return
	}
	var req struct {
		Goal string `json:"goal"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid plan request: %v", err), nil)
		return
	}
	steps, reason, err := planDiscovery(req.Goal)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "goal": req.Goal, "planned_by": reason, "steps": steps})
}
