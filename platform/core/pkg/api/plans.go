package api

// Experiment plans (AG-3): a pre-registered protocol binding a hypothesis to
// an observable, an acceptance metric, a tolerance, and a stop rule. A plan
// must be approved before execution; execution submits the stored spec through
// the job rail and scores the result against the registered tolerance.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/plans"
)

func planSubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/plans")
	if idx <= 0 {
		return "", "", false
	}
	rest = strings.TrimPrefix(id[idx:], "/plans")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

func (s *Server) handlePlanRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.ledger.ListPlans(sessionID, 50)
			if err != nil {
				WriteAPIError(w, "internal", "plan list failed", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"plans": items})
		case http.MethodPost:
			s.handlePlanCreate(w, r, sessionID, sess.TenantID)
		default:
			WriteAPIError(w, "method_not_allowed", "plans support GET and POST", nil)
		}
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[1] == "execute" {
		s.handlePlanExecute(w, r, sessionID, parts[0])
		return
	}
	if len(parts) != 1 {
		WriteAPIError(w, "not_found", "unknown plan route", nil)
		return
	}
	if r.Method != http.MethodPatch {
		WriteAPIError(w, "method_not_allowed", "plan status is PATCH-only", nil)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid status: %v", err), nil)
		return
	}
	switch strings.ToLower(strings.TrimSpace(body.Status)) {
	case ledger.PlanApproved, ledger.PlanRejected, ledger.PlanProposed:
	default:
		WriteAPIError(w, "bad_request", "plan status must be approved, rejected, or proposed", nil)
		return
	}
	if err := s.ledger.SetPlanStatus(sessionID, parts[0], body.Status); err != nil {
		if errors.Is(err, ledger.ErrPlanNotFound) {
			WriteAPIError(w, "not_found", "unknown plan", nil)
			return
		}
		WriteAPIError(w, "internal", "plan status failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "plan_status_changed", map[string]any{
		"plan_id": parts[0], "status": body.Status,
	}); err != nil {
		WriteAPIError(w, "internal", "plan event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": parts[0], "status": body.Status})
}

func (s *Server) handlePlanCreate(w http.ResponseWriter, r *http.Request, sessionID, tenantID string) {
	var body struct {
		HypothesisID     string          `json:"hypothesis_id"`
		Statement        string          `json:"statement"`
		Method           string          `json:"method"`
		Observable       string          `json:"observable"`
		AcceptanceMetric string          `json:"acceptance_metric"`
		Tolerance        float64         `json:"tolerance"`
		StopRule         string          `json:"stop_rule"`
		BudgetUSD        float64         `json:"budget_usd"`
		Executor         string          `json:"executor"`
		Spec             json.RawMessage `json:"spec"`
		// AcknowledgePriorNegative is the explicit override when the registry
		// holds a strongly overlapping negative result.
		AcknowledgePriorNegative bool `json:"acknowledge_prior_negative"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid plan: %v", err), nil)
		return
	}
	if body.Observable == "" || body.AcceptanceMetric == "" || body.StopRule == "" {
		WriteAPIError(w, "bad_request", "plan needs observable, acceptance_metric, and stop_rule", nil)
		return
	}
	if body.Tolerance < 0 {
		WriteAPIError(w, "bad_request", "tolerance must be >= 0", nil)
		return
	}
	var hypothesis *ledger.Hypothesis
	if body.HypothesisID != "" {
		h, err := s.ledger.GetHypothesis(sessionID, body.HypothesisID)
		if err != nil {
			WriteAPIError(w, "not_found", "unknown hypothesis", nil)
			return
		}
		hypothesis = h
		switch h.Status {
		case ledger.HypothesisRejected:
			WriteAPIError(w, "bad_request", "a rejected hypothesis cannot be planned", nil)
			return
		case ledger.HypothesisShortlist, ledger.HypothesisCollapsed:
			// A human (or the tournament plus shortlist) has admitted the
			// probe; only admitted probes may spend experiment budget.
		default:
			WriteAPIError(w, "hypothesis_not_shortlisted",
				"hypothesis must be shortlisted (or collapsed) before it can be planned", nil)
			return
		}
		if strings.TrimSpace(body.Statement) == "" {
			body.Statement = h.Statement
		}
		if strings.TrimSpace(body.Method) == "" {
			body.Method = h.ExperimentKind
		}
	}
	if strings.TrimSpace(body.Statement) == "" || strings.TrimSpace(body.Method) == "" {
		WriteAPIError(w, "bad_request", "plan needs a statement and method", nil)
		return
	}
	method := strings.ToLower(strings.TrimSpace(body.Method))
	if _, ok := experimentJobType(method); !ok {
		WriteAPIError(w, "bad_request", "unknown plan method: "+body.Method, nil)
		return
	}

	// AG-9: a strongly overlapping prior negative result refuses the plan
	// unless the caller acknowledges it. This is how a second campaign avoids
	// repeating a known failure without being silently blocked.
	prior, err := s.ledger.LookupKnowledge(tenantID, body.Statement, 5)
	if err != nil {
		WriteAPIError(w, "internal", "knowledge lookup failed", nil)
		return
	}
	priorNegative := make([]ledger.KnowledgeMatch, 0, len(prior))
	for _, m := range prior {
		if m.Similarity >= priorNegativeThreshold {
			priorNegative = append(priorNegative, m)
		}
	}
	if len(priorNegative) > 0 && !body.AcknowledgePriorNegative {
		WriteAPIError(w, "prior_negative_exists",
			fmt.Sprintf("%d prior negative result(s) overlap this plan; acknowledge them or change the plan", len(priorNegative)),
			map[string]any{"matches": priorNegative, "threshold": priorNegativeThreshold})
		return
	}

	var campaign agents.CampaignState
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}

	// AG-3b: a hypothesis can be planned without a caller-supplied spec. The
	// synthesizer grounds every value in the campaign and the hypothesis's
	// declared patch; a kind it cannot ground refuses instead of guessing.
	var spec map[string]any
	var specBasis []string
	explicit := len(body.Spec) > 0 && string(body.Spec) != "null"
	if explicit {
		if err := json.Unmarshal(body.Spec, &spec); err != nil || spec == nil {
			WriteAPIError(w, "bad_request", "plan spec is not a JSON object", nil)
			return
		}
		specBasis = []string{"spec supplied by the caller"}
	} else {
		if hypothesis == nil {
			WriteAPIError(w, "bad_request",
				"plan needs a concrete experiment spec, or a hypothesis_id to synthesize from", nil)
			return
		}
		facts := planFactsFromCampaign(&campaign)
		synthesized, basis, err := plans.Synthesize(plans.Request{
			Method: method, Hint: hypothesis.ExperimentHint, Patch: hypothesis.SpecPatch,
		}, facts)
		if err != nil {
			WriteAPIError(w, "spec_unavailable", err.Error(), nil)
			return
		}
		spec, specBasis = synthesized, basis
	}
	spec["kind"] = method
	rawSpec, _ := json.Marshal(spec)
	// AG-7: classify at registration so the risk is visible before anyone
	// approves or executes. The execution path reclassifies with the policy in
	// force at that moment.
	policy, _ := s.loadAutonomyPolicy(sessionID)
	risk := autonomy.Classify(autonomy.RiskInput{
		Method: method, Spec: spec, BudgetUSD: body.BudgetUSD,
		Statement: body.Statement, Target: campaign.TargetSystem,
		MaxActBudgetUSD: policy.MaxActBudgetUSD,
	})
	plan := &ledger.ExperimentPlan{
		ID:               fmt.Sprintf("plan_%s", newManuscriptID()[3:11]),
		HypothesisID:     body.HypothesisID,
		Statement:        strings.TrimSpace(body.Statement),
		Method:           method,
		Observable:       strings.TrimSpace(body.Observable),
		AcceptanceMetric: strings.TrimSpace(body.AcceptanceMetric),
		Tolerance:        body.Tolerance,
		StopRule:         strings.TrimSpace(body.StopRule),
		BudgetUSD:        body.BudgetUSD,
		Executor:         strings.TrimSpace(body.Executor),
		SpecJSON:         string(rawSpec),
		SpecBasis:        specBasis,
		Risk:             risk.Level,
		RiskReasons:      risk.Reasons,
		Status:           ledger.PlanProposed,
	}
	if err := s.ledger.CreatePlan(sessionID, tenantID, plan); err != nil {
		WriteAPIError(w, "internal", "plan persistence failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "plan_created", map[string]any{
		"plan_id": plan.ID, "hypothesis_id": plan.HypothesisID, "method": plan.Method,
		"prior_negative_ack": body.AcknowledgePriorNegative, "prior_negative_matches": len(priorNegative),
	}); err != nil {
		WriteAPIError(w, "internal", "plan event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"plan": plan, "prior_negative": priorNegative,
		"prior_negative_ack": body.AcknowledgePriorNegative,
	})
}

func (s *Server) handlePlanExecute(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "plan execution is POST-only", nil)
		return
	}
	plan, err := s.ledger.GetPlan(sessionID, id)
	if errors.Is(err, ledger.ErrPlanNotFound) {
		WriteAPIError(w, "not_found", "unknown plan", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "plan read failed", nil)
		return
	}
	var spec map[string]any
	if err := json.Unmarshal([]byte(plan.SpecJSON), &spec); err != nil || spec == nil {
		WriteAPIError(w, "bad_request", "stored plan spec is unreadable", nil)
		return
	}
	if err := s.requireDispatchAutonomy(sessionID); err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	// AG-7: the session policy and the plan's risk decide whether the agent may
	// execute. "suggest" always refuses; high risk always needs approval.
	var campaign agents.CampaignState
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}
	if !s.requireAgentic(w, sessionID) {
		return
	}
	policy, _ := s.loadAutonomyPolicy(sessionID)
	decision := autonomy.DecideExecute(policy, plan.Status, autonomy.RiskInput{
		Method: plan.Method, Spec: spec, BudgetUSD: plan.BudgetUSD,
		Statement: plan.Statement, Target: campaign.TargetSystem,
		MaxActBudgetUSD: policy.MaxActBudgetUSD,
	})
	if !decision.Allowed {
		code := decision.Code
		if code == "" {
			code = "risk_approval_required"
		}
		WriteAPIError(w, code, decision.Reason, map[string]any{
			"risk": decision.Risk, "execute_mode": policy.ModeFor(autonomy.StageExecute),
			"max_act_budget_usd": policy.MaxActBudgetUSD,
		})
		return
	}
	spec["kind"] = plan.Method
	spec["max_cost_usd"] = plan.BudgetUSD
	jobType, ok := experimentJobType(plan.Method)
	if !ok {
		WriteAPIError(w, "bad_request", "unknown plan method: "+plan.Method, nil)
		return
	}
	job, err := s.jobRunner.SubmitJob(sessionID, jobType, spec)
	if err != nil {
		_ = s.ledger.SetPlanResult(sessionID, id, ledger.PlanFailed, "", `{"error":"submission refused"}`)
		WriteAPIError(w, "bad_request", "plan submission refused: "+err.Error(), nil)
		return
	}
	_ = s.ledger.SetPlanStatus(sessionID, id, ledger.PlanRunning)
	_ = s.recordManuscriptEvent(sessionID, "plan_started", map[string]any{
		"plan_id": id, "job_id": job.ID, "method": plan.Method,
		"auto_approved": decision.AutoApprove, "notify": decision.Notify,
		"risk": decision.Risk.Level,
	})
	final, err := s.waitForJob(sessionID, job.ID, 10*time.Minute)
	if err != nil {
		_ = s.ledger.SetPlanResult(sessionID, id, ledger.PlanFailed, job.ID, `{"error":"timeout"}`)
		WriteAPIError(w, "timeout", "plan job did not finish: "+err.Error(), nil)
		return
	}
	result := map[string]any{}
	if len(final.ResultJSON) > 0 {
		_ = json.Unmarshal(final.ResultJSON, &result)
	}
	measured, measuredOK := numberAt(result, plan.Observable)
	target, targetOK := numberAt(result, plan.AcceptanceMetric)
	accepted := measuredOK && targetOK && absFloat(measured-target) <= plan.Tolerance
	status := ledger.PlanRejected
	if accepted {
		status = ledger.PlanCompleted
	}
	payload, _ := json.Marshal(map[string]any{
		"job_id": job.ID, "job_status": final.Status.String(),
		"observable": plan.Observable, "measured": measured,
		"acceptance_metric": plan.AcceptanceMetric, "target": target,
		"tolerance": plan.Tolerance, "accepted": accepted, "result": result,
		"auto_approved": decision.AutoApprove, "notify": decision.Notify,
		"risk": decision.Risk.Level,
	})
	if err := s.ledger.SetPlanResult(sessionID, id, status, job.ID, string(payload)); err != nil {
		WriteAPIError(w, "internal", "plan result failed: "+err.Error(), nil)
		return
	}
	_ = s.recordManuscriptEvent(sessionID, "plan_executed", map[string]any{
		"plan_id": id, "job_id": job.ID, "accepted": accepted, "status": status,
		"auto_approved": decision.AutoApprove, "notify": decision.Notify,
		"risk": decision.Risk.Level,
	})
	if status == ledger.PlanRejected || status == ledger.PlanFailed {
		s.publishPlanNegative(sessionID, plan, status, string(payload))
	}
	updated, _ := s.ledger.GetPlan(sessionID, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"plan": updated, "accepted": accepted, "result": result,
		"auto_approved": decision.AutoApprove, "notify": decision.Notify,
		"risk": decision.Risk,
	})
}

// numberAt reads a dotted numeric path from a job result.
func numberAt(m map[string]any, path string) (float64, bool) {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur, ok = obj[part]
		if !ok {
			return 0, false
		}
	}
	switch v := cur.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
