package api

// Receipts and undo for reversible agentic actions (AG-23).
//
// Jobs and claims are immutable evidence and are never undone. The actions that
// can be reversed safely are the oversight decisions themselves: the agentic
// mode switch, a plan approval/rejection that has not run, and a deferral. An
// undo appends a new event; history is never rewritten.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func receiptSubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/receipts")
	if idx <= 0 {
		return "", "", false
	}
	rest = strings.TrimPrefix(id[idx:], "/receipts")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

type undoReceipt struct {
	ID        int64           `json:"id"`
	EventType string          `json:"event_type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	Undoable  bool            `json:"undoable"`
	Reason    string          `json:"reason,omitempty"`
}

func (s *Server) handleReceiptsRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if rest == "" {
		if r.Method != http.MethodGet {
			WriteAPIError(w, "method_not_allowed", "receipts are GET-only", nil)
			return
		}
		s.listReceipts(w, sessionID)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "undo" {
		WriteAPIError(w, "not_found", "unknown receipt route", nil)
		return
	}
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "undo is POST-only", nil)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		WriteAPIError(w, "bad_request", "receipt id must be a positive integer", nil)
		return
	}
	s.undoReceipt(w, sessionID, id)
}

func (s *Server) listReceipts(w http.ResponseWriter, sessionID string) {
	events, err := s.ledger.GetRecentEvents(sessionID, 200)
	if err != nil {
		WriteAPIError(w, "internal", "receipt read failed", nil)
		return
	}
	undone := map[int64]bool{}
	for _, e := range events {
		if e.EventType == "action_undone" {
			var p struct {
				TargetID int64 `json:"target_id"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				undone[p.TargetID] = true
			}
		}
	}
	policy, _, _ := s.loadAutonomyPolicyStrict(sessionID)
	out := make([]undoReceipt, 0, 20)
	for _, e := range events {
		switch e.EventType {
		case "agentic_mode_changed", "plan_approval_decided", "plan_deferred":
		default:
			continue
		}
		receipt := undoReceipt{ID: e.ID, EventType: e.EventType, Timestamp: e.Timestamp, Payload: e.Payload}
		if undone[e.ID] {
			receipt.Reason = "already undone"
		} else {
			receipt.Undoable, receipt.Reason = s.receiptUndoable(sessionID, e, policy)
		}
		out = append(out, receipt)
		if len(out) >= 50 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "receipts": out})
}

func (s *Server) receiptUndoable(sessionID string, e ledger.EventRecord, policy autonomy.Policy) (bool, string) {
	switch e.EventType {
	case "agentic_mode_changed":
		if policy.AgenticLocked {
			return false, "locked by an operator halt"
		}
		return true, ""
	case "plan_approval_decided", "plan_deferred":
		var p struct {
			PlanID string `json:"plan_id"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || p.PlanID == "" {
			return false, "receipt has no plan id"
		}
		plan, err := s.ledger.GetPlan(sessionID, p.PlanID)
		if err != nil {
			return false, "plan not found"
		}
		if e.EventType == "plan_approval_decided" && (plan.Status == "running" || plan.Status == "completed" || plan.Status == "failed") {
			return false, "plan has already run"
		}
		if e.EventType == "plan_deferred" && plan.Status != "proposed" {
			return false, "plan is no longer proposed"
		}
		return true, ""
	}
	return false, "not a reversible action"
}

func (s *Server) undoReceipt(w http.ResponseWriter, sessionID string, id int64) {
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", "receipt read failed", nil)
		return
	}
	var target *ledger.EventRecord
	for i := range events {
		if events[i].ID == id {
			target = &events[i]
			break
		}
	}
	if target == nil {
		WriteAPIError(w, "not_found", "unknown receipt", nil)
		return
	}
	for _, e := range events {
		if e.EventType != "action_undone" {
			continue
		}
		var p struct {
			TargetID int64 `json:"target_id"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.TargetID == id {
			WriteAPIError(w, "undo_not_reversible", "this receipt has already been undone", nil)
			return
		}
	}
	policy, _, policyErr := s.loadAutonomyPolicyStrict(sessionID)
	if policyErr != nil {
		WriteAPIError(w, "autonomy_policy_unreadable", policyErr.Error(), nil)
		return
	}
	action := ""
	switch target.EventType {
	case "agentic_mode_changed":
		if policy.AgenticLocked {
			WriteAPIError(w, "undo_not_reversible", "agentic mode is locked by an operator halt", nil)
			return
		}
		policy.AgenticEnabled = boolPtr(!policy.Agentic())
		policy.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		raw, _ := json.Marshal(policy)
		if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
			WriteAPIError(w, "internal", "undo save failed: "+err.Error(), nil)
			return
		}
		paused := 0
		if !policy.Agentic() {
			paused = s.pauseSessionLoops(sessionID)
		}
		_ = s.recordManuscriptEvent(sessionID, "agentic_mode_changed", map[string]any{
			"enabled": policy.Agentic(), "locked": policy.AgenticLocked,
			"actor": "undo", "paused_loops": paused,
		})
		action = fmt.Sprintf("agentic mode %t", policy.Agentic())
	case "plan_approval_decided":
		var p struct {
			PlanID string `json:"plan_id"`
		}
		if json.Unmarshal(target.Payload, &p) != nil || p.PlanID == "" {
			WriteAPIError(w, "undo_not_reversible", "receipt has no plan id", nil)
			return
		}
		plan, err := s.ledger.GetPlan(sessionID, p.PlanID)
		if err != nil {
			WriteAPIError(w, "not_found", "plan not found", nil)
			return
		}
		if plan.Status == "running" || plan.Status == "completed" || plan.Status == "failed" {
			WriteAPIError(w, "undo_not_reversible", "plan has already run", nil)
			return
		}
		if err := s.ledger.SetPlanStatus(sessionID, p.PlanID, ledger.PlanProposed); err != nil {
			WriteAPIError(w, "internal", "undo plan decision failed: "+err.Error(), nil)
			return
		}
		action = "plan returned to proposed"
	case "plan_deferred":
		var p struct {
			PlanID string `json:"plan_id"`
		}
		if json.Unmarshal(target.Payload, &p) != nil || p.PlanID == "" {
			WriteAPIError(w, "undo_not_reversible", "receipt has no plan id", nil)
			return
		}
		plan, err := s.ledger.GetPlan(sessionID, p.PlanID)
		if err != nil || plan.Status != "proposed" {
			WriteAPIError(w, "undo_not_reversible", "plan is no longer proposed", nil)
			return
		}
		if err := s.ledger.SetPlanDeferral(sessionID, p.PlanID, "", ""); err != nil {
			WriteAPIError(w, "internal", "undo deferral failed: "+err.Error(), nil)
			return
		}
		action = "deferral removed"
	default:
		WriteAPIError(w, "undo_not_reversible", "receipt is not a reversible action", nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "action_undone", map[string]any{
		"target_id": id, "event_type": target.EventType, "action": action,
	}); err != nil {
		WriteAPIError(w, "internal", "undo event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "target_id": id, "action": action})
}

func boolPtr(v bool) *bool { b := v; return &b }
