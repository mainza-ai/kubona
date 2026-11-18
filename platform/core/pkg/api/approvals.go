package api

// Durable approval inbox (AG-19).
//
// The inbox is a projection of proposed experiment plans, not a second store:
// pending plans wait for a decision, deferred plans are parked until a future
// time, and every decision is sealed as an event. A plan can be decided once;
// execution still requires the approved status.
//
// A reviewer may edit the pre-registered protocol (AG-19b) before approving.
// The edit is validated, the risk classifier is re-run on the edited plan, and
// both the previous values and the edited field names are sealed so the change
// is auditable and reversible. Rejections carry feedback that stays attached to
// the plan instead of disappearing into chat.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func approvalSubpath(id string) (sessionID, planID string, ok bool) {
	idx := strings.Index(id, "/approvals")
	if idx <= 0 {
		return "", "", false
	}
	rest := strings.TrimPrefix(id[idx:], "/approvals")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

// planEditFields is a sparse reviewer edit: nil means "unchanged", so clearing
// a numeric field is not the same as omitting it.
type planEditFields struct {
	Statement        *string         `json:"statement"`
	Method           *string         `json:"method"`
	Observable       *string         `json:"observable"`
	AcceptanceMetric *string         `json:"acceptance_metric"`
	Tolerance        *float64        `json:"tolerance"`
	StopRule         *string         `json:"stop_rule"`
	BudgetUSD        *float64        `json:"budget_usd"`
	Spec             json.RawMessage `json:"spec"`
}

func (s *Server) handleApprovalsRoute(w http.ResponseWriter, r *http.Request, sessionID, planID string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if planID == "" {
		if r.Method != http.MethodGet {
			WriteAPIError(w, "method_not_allowed", "approval inbox is GET-only; decide on /approvals/{plan_id}", nil)
			return
		}
		plans, err := s.ledger.ListPlans(sessionID, 200)
		if err != nil {
			WriteAPIError(w, "internal", "approval read failed", nil)
			return
		}
		pending := make([]ledger.ExperimentPlan, 0, len(plans))
		deferred := make([]ledger.ExperimentPlan, 0, len(plans))
		now := time.Now().UTC()
		for _, p := range plans {
			if p.Status != ledger.PlanProposed {
				continue
			}
			if until, err := time.Parse(time.RFC3339, p.DeferredUntil); err == nil && until.After(now) {
				deferred = append(deferred, p)
				continue
			}
			pending = append(pending, p)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID, "pending": pending, "deferred": deferred,
			"counts": map[string]any{"pending": len(pending), "deferred": len(deferred)},
		})
		return
	}
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "approval decisions are POST-only", nil)
		return
	}
	var body struct {
		Decision      string          `json:"decision"`
		Reason        string          `json:"reason"`
		DeferredUntil string          `json:"deferred_until"`
		Feedback      string          `json:"feedback"`
		Edits         *planEditFields `json:"edits"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid approval request: %v", err), nil)
		return
	}
	plan, err := s.ledger.GetPlan(sessionID, planID)
	if errors.Is(err, ledger.ErrPlanNotFound) {
		WriteAPIError(w, "not_found", "unknown plan", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "plan read failed", nil)
		return
	}
	if plan.Status != ledger.PlanProposed {
		WriteAPIError(w, "plan_not_pending",
			fmt.Sprintf("plan is %s; only a proposed plan can be decided", plan.Status), nil)
		return
	}
	decision := strings.ToLower(strings.TrimSpace(body.Decision))
	reason := strings.TrimSpace(body.Reason)
	feedback := strings.TrimSpace(body.Feedback)
	if len(feedback) > 2000 {
		WriteAPIError(w, "bad_request", "feedback is limited to 2000 characters", nil)
		return
	}
	switch decision {
	case "approve", "reject":
		status := ledger.PlanApproved
		if decision == "reject" {
			status = ledger.PlanRejected
		}
		previous := map[string]any{}
		edited := []string{}
		if body.Edits != nil {
			if decision == "reject" {
				WriteAPIError(w, "bad_request", "edits only apply to an approval; attach feedback to a rejection", nil)
				return
			}
			edited, previous, err = s.applyPlanEdits(sessionID, plan, *body.Edits)
			if err != nil {
				WriteAPIError(w, "bad_request", err.Error(), nil)
				return
			}
		}
		// Undo returns to the pre-decision review state even when the reviewer
		// changed nothing but the decision itself.
		previous["review_feedback"] = plan.ReviewFeedback
		previous["reviewed_at"] = plan.ReviewedAt
		plan.ReviewFeedback = feedback
		plan.ReviewedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.ledger.UpdatePlanReview(sessionID, plan); err != nil {
			if errors.Is(err, ledger.ErrPlanNotPending) {
				WriteAPIError(w, "plan_not_pending", "plan changed while you were reviewing it; refresh the inbox", nil)
				return
			}
			WriteAPIError(w, "internal", "plan review failed: "+err.Error(), nil)
			return
		}
		if err := s.ledger.SetPlanStatus(sessionID, planID, status); err != nil {
			WriteAPIError(w, "internal", "plan decision failed: "+err.Error(), nil)
			return
		}
		if err := s.recordManuscriptEvent(sessionID, "plan_approval_decided", map[string]any{
			"plan_id": planID, "decision": decision, "status": status, "reason": reason,
			"feedback": feedback, "edited_fields": edited, "previous": previous,
		}); err != nil {
			WriteAPIError(w, "internal", "approval event failed: "+err.Error(), nil)
			return
		}
	case "defer":
		until, err := time.Parse(time.RFC3339, strings.TrimSpace(body.DeferredUntil))
		if err != nil || !until.After(time.Now().UTC()) {
			WriteAPIError(w, "bad_request", "defer needs a future deferred_until in RFC3339", nil)
			return
		}
		if err := s.ledger.SetPlanDeferral(sessionID, planID, until.UTC().Format(time.RFC3339), reason); err != nil {
			WriteAPIError(w, "internal", "plan deferral failed: "+err.Error(), nil)
			return
		}
		if err := s.recordManuscriptEvent(sessionID, "plan_deferred", map[string]any{
			"plan_id": planID, "deferred_until": until.UTC().Format(time.RFC3339), "reason": reason,
		}); err != nil {
			WriteAPIError(w, "internal", "deferral event failed: "+err.Error(), nil)
			return
		}
	default:
		WriteAPIError(w, "bad_request", "decision must be approve, reject, or defer", nil)
		return
	}
	updated, _ := s.ledger.GetPlan(sessionID, planID)
	writeJSON(w, http.StatusOK, map[string]any{"plan": updated, "decision": decision})
}

// applyPlanEdits validates and applies a reviewer edit to the plan struct in
// place, re-runs the risk classifier on the edited protocol, and returns the
// previous values of every touched field (the undo snapshot) plus the field
// names. It never writes; the caller persists once validation passes.
func (s *Server) applyPlanEdits(sessionID string, plan *ledger.ExperimentPlan, edits planEditFields) ([]string, map[string]any, error) {
	previous := map[string]any{}
	edited := []string{}
	touch := func(name string, old any) {
		previous[name] = old
		edited = append(edited, name)
	}
	if edits.Statement != nil {
		v := strings.TrimSpace(*edits.Statement)
		if v == "" || len(v) > 2000 {
			return nil, nil, fmt.Errorf("edited statement must be 1..2000 characters")
		}
		touch("statement", plan.Statement)
		plan.Statement = v
	}
	if edits.Method != nil {
		v := strings.TrimSpace(*edits.Method)
		if v == "" || len(v) > 64 {
			return nil, nil, fmt.Errorf("edited method must be 1..64 characters")
		}
		touch("method", plan.Method)
		plan.Method = v
	}
	if edits.Observable != nil {
		v := strings.TrimSpace(*edits.Observable)
		if v == "" || len(v) > 128 {
			return nil, nil, fmt.Errorf("edited observable must be 1..128 characters")
		}
		touch("observable", plan.Observable)
		plan.Observable = v
	}
	if edits.AcceptanceMetric != nil {
		v := strings.TrimSpace(*edits.AcceptanceMetric)
		if v == "" || len(v) > 128 {
			return nil, nil, fmt.Errorf("edited acceptance_metric must be 1..128 characters")
		}
		touch("acceptance_metric", plan.AcceptanceMetric)
		plan.AcceptanceMetric = v
	}
	if edits.Tolerance != nil {
		v := *edits.Tolerance
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e9 {
			return nil, nil, fmt.Errorf("edited tolerance must be finite in [0, 1e9]")
		}
		touch("tolerance", plan.Tolerance)
		plan.Tolerance = v
	}
	if edits.StopRule != nil {
		v := strings.TrimSpace(*edits.StopRule)
		if v == "" || len(v) > 500 {
			return nil, nil, fmt.Errorf("edited stop_rule must be 1..500 characters")
		}
		touch("stop_rule", plan.StopRule)
		plan.StopRule = v
	}
	if edits.BudgetUSD != nil {
		v := *edits.BudgetUSD
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1e6 {
			return nil, nil, fmt.Errorf("edited budget_usd must be finite in (0, 1e6]")
		}
		touch("budget_usd", plan.BudgetUSD)
		plan.BudgetUSD = v
	}
	if len(edits.Spec) > 0 {
		var spec map[string]any
		if err := json.Unmarshal(edits.Spec, &spec); err != nil || spec == nil {
			return nil, nil, fmt.Errorf("edited spec must be a JSON object")
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, nil, fmt.Errorf("edited spec cannot be encoded: %v", err)
		}
		if len(raw) > 64*1024 {
			return nil, nil, fmt.Errorf("edited spec exceeds 64 KiB")
		}
		touch("spec", plan.SpecJSON)
		plan.SpecJSON = string(raw)
	}
	if len(edited) == 0 {
		return edited, previous, nil
	}
	var campaign agents.CampaignState
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}
	var spec map[string]any
	_ = json.Unmarshal([]byte(plan.SpecJSON), &spec)
	risk := autonomy.Classify(autonomy.RiskInput{
		Method: plan.Method, Spec: spec, BudgetUSD: plan.BudgetUSD,
		Statement: plan.Statement, Target: campaign.TargetSystem,
	})
	touch("risk", plan.Risk)
	touch("risk_reasons", plan.RiskReasons)
	plan.Risk = risk.Level
	plan.RiskReasons = risk.Reasons
	return edited, previous, nil
}

// restorePlanReview reverts the field snapshot sealed with an approval event.
// It is used by receipt undo so "return to proposed" also returns the
// protocol the reviewer actually had, not the edited one.
func (s *Server) restorePlanReview(sessionID, planID string, previous json.RawMessage) (bool, error) {
	if len(previous) == 0 || string(previous) == "null" || string(previous) == "{}" {
		return false, nil
	}
	var prev map[string]any
	if err := json.Unmarshal(previous, &prev); err != nil {
		return false, fmt.Errorf("sealed review snapshot is unreadable: %w", err)
	}
	plan, err := s.ledger.GetPlan(sessionID, planID)
	if err != nil {
		return false, err
	}
	if v, ok := prev["statement"].(string); ok {
		plan.Statement = v
	}
	if v, ok := prev["method"].(string); ok {
		plan.Method = v
	}
	if v, ok := prev["observable"].(string); ok {
		plan.Observable = v
	}
	if v, ok := prev["acceptance_metric"].(string); ok {
		plan.AcceptanceMetric = v
	}
	if v, ok := prev["tolerance"].(float64); ok {
		plan.Tolerance = v
	}
	if v, ok := prev["stop_rule"].(string); ok {
		plan.StopRule = v
	}
	if v, ok := prev["budget_usd"].(float64); ok {
		plan.BudgetUSD = v
	}
	if v, ok := prev["spec"].(string); ok {
		plan.SpecJSON = v
	}
	if v, ok := prev["risk"].(string); ok {
		plan.Risk = v
	}
	if raw, ok := prev["risk_reasons"].([]any); ok {
		reasons := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok {
				reasons = append(reasons, text)
			}
		}
		plan.RiskReasons = reasons
	}
	if v, ok := prev["review_feedback"].(string); ok {
		plan.ReviewFeedback = v
	}
	if v, ok := prev["reviewed_at"].(string); ok {
		plan.ReviewedAt = v
	}
	if err := s.ledger.SetPlanStatus(sessionID, planID, ledger.PlanProposed); err != nil {
		return false, err
	}
	if err := s.ledger.UpdatePlanReview(sessionID, plan); err != nil {
		return false, err
	}
	return true, nil
}
