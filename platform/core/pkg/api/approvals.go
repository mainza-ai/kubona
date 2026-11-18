package api

// Durable approval inbox (AG-19).
//
// The inbox is a projection of proposed experiment plans, not a second store:
// pending plans wait for a decision, deferred plans are parked until a future
// time, and every decision is sealed as an event. A plan can be decided once;
// execution still requires the approved status.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		DeferredUntil string `json:"deferred_until"`
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
	switch decision {
	case "approve", "reject":
		status := ledger.PlanApproved
		if decision == "reject" {
			status = ledger.PlanRejected
		}
		if err := s.ledger.SetPlanStatus(sessionID, planID, status); err != nil {
			WriteAPIError(w, "internal", "plan decision failed: "+err.Error(), nil)
			return
		}
		if err := s.recordManuscriptEvent(sessionID, "plan_approval_decided", map[string]any{
			"plan_id": planID, "decision": decision, "status": status, "reason": reason,
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
