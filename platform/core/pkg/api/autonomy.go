package api

// Stage-level autonomy and risk screening (AG-7).
//
// Tenant autonomy (L0–L3) remains the outer bound. This file adds the session
// rail: per-stage modes, a risk classification for each plan, and a halt
// switch that drops every stage to suggest and cancels live jobs. The
// execution path is the one place the server enforces the rail; proposal
// stages stay open so an agent can always ask, never spend.

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

func autonomySubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/autonomy")
	if idx <= 0 {
		return "", "", false
	}
	rest = strings.TrimPrefix(id[idx:], "/autonomy")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

func (s *Server) handleAutonomyRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if rest == "halt" {
		s.handleAutonomyHalt(w, r, sessionID)
		return
	}
	if rest == "history" {
		s.handleAutonomyHistory(w, r, sessionID)
		return
	}
	if rest == "rollback" {
		s.handleAutonomyRollback(w, r, sessionID)
		return
	}
	if rest != "" {
		WriteAPIError(w, "not_found", "unknown autonomy route", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, source := s.loadAutonomyPolicy(sessionID)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID, "policy": policy, "source": source,
		})
	case http.MethodPut:
		if !s.requireOperator(w, r) {
			return
		}
		// The body's switch fields are optional: an omitted field preserves
		// the stored value, so a stage-only edit cannot silently re-enable a
		// session or clear an operator halt lock.
		var body struct {
			Stages          map[autonomy.Stage]autonomy.Mode `json:"stages"`
			MaxActBudgetUSD float64                          `json:"max_act_budget_usd"`
			AgenticEnabled  *bool                            `json:"agentic_enabled"`
			AgenticLocked   *bool                            `json:"agentic_locked"`
			ToolPermissions map[string]string                `json:"tool_permissions"`
		}
		if err := decodeStrict(r, &body); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid autonomy policy: %v", err), nil)
			return
		}
		previous, _ := s.loadAutonomyPolicy(sessionID)
		enabled, locked := previous.AgenticEnabled, previous.AgenticLocked
		if body.AgenticEnabled != nil {
			enabled = body.AgenticEnabled
		}
		if body.AgenticLocked != nil {
			locked = *body.AgenticLocked
		}
		tools := previous.ToolPermissions
		if body.ToolPermissions != nil {
			tools = body.ToolPermissions
		}
		next, err := autonomy.Normalize(autonomy.Policy{
			Stages: body.Stages, MaxActBudgetUSD: body.MaxActBudgetUSD,
			AgenticEnabled: enabled, AgenticLocked: locked,
			ToolPermissions: tools,
		})
		if err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		raw, _ := json.Marshal(next)
		if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
			WriteAPIError(w, "internal", "autonomy policy save failed: "+err.Error(), nil)
			return
		}
		paused := 0
		if !next.Agentic() {
			paused = s.pauseSessionLoops(sessionID)
		}
		if err := s.recordManuscriptEvent(sessionID, "autonomy_policy_updated", map[string]any{
			"stages": next.Stages, "max_act_budget_usd": next.MaxActBudgetUSD,
			"agentic_enabled": next.Agentic(), "agentic_locked": next.AgenticLocked,
			"tool_permissions": next.ToolPermissions,
		}); err != nil {
			WriteAPIError(w, "internal", "autonomy event failed: "+err.Error(), nil)
			return
		}
		if next.Agentic() != previous.Agentic() || next.AgenticLocked != previous.AgenticLocked {
			if err := s.recordManuscriptEvent(sessionID, "agentic_mode_changed", map[string]any{
				"enabled": next.Agentic(), "locked": next.AgenticLocked,
				"actor": "operator", "paused_loops": paused,
			}); err != nil {
				WriteAPIError(w, "internal", "agentic event failed: "+err.Error(), nil)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"policy": next, "source": "session", "paused_loops": paused,
		})
	default:
		WriteAPIError(w, "method_not_allowed", "autonomy supports GET and PUT; use /autonomy/halt to stop", nil)
	}
}

func (s *Server) handleAutonomyHalt(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "autonomy halt is POST-only", nil)
		return
	}
	if !s.requireOperator(w, r) {
		return
	}
	policy, _ := s.loadAutonomyPolicy(sessionID)
	disabled := false
	halted := autonomy.Policy{
		Stages:          map[autonomy.Stage]autonomy.Mode{},
		MaxActBudgetUSD: policy.MaxActBudgetUSD,
		AgenticEnabled:  &disabled,
		AgenticLocked:   true,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	for _, stage := range autonomy.Stages {
		halted.Stages[stage] = autonomy.ModeSuggest
	}
	raw, _ := json.Marshal(halted)
	if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
		WriteAPIError(w, "internal", "autonomy halt save failed: "+err.Error(), nil)
		return
	}
	paused := s.pauseSessionLoops(sessionID)
	cancelled := 0
	if s.jobRunner != nil {
		n, err := s.jobRunner.CancelSessionJobs(sessionID, "autonomy halt")
		if err != nil {
			WriteAPIError(w, "internal", "job cancellation failed: "+err.Error(), nil)
			return
		}
		cancelled = n
	}
	if err := s.recordManuscriptEvent(sessionID, "autonomy_halted", map[string]any{
		"cancelled_jobs": cancelled, "paused_loops": paused,
		"max_act_budget_usd": halted.MaxActBudgetUSD,
		"agentic_enabled":    false, "agentic_locked": true,
	}); err != nil {
		WriteAPIError(w, "internal", "halt event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policy": halted, "source": "session", "cancelled_jobs": cancelled,
		"paused_loops": paused,
	})
}

// handleAutonomyHistory lists the sealed policy/switch events (AG-24).
func (s *Server) handleAutonomyHistory(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "autonomy history is GET-only", nil)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", "autonomy history read failed", nil)
		return
	}
	allowed := map[string]bool{
		"autonomy_policy_updated":     true,
		"autonomy_halted":             true,
		"autonomy_policy_rolled_back": true,
		"agentic_mode_changed":        true,
	}
	entries := make([]map[string]any, 0, limit)
	for _, e := range events {
		if !allowed[e.EventType] {
			continue
		}
		entries = append(entries, map[string]any{
			"id": e.ID, "event_type": e.EventType, "timestamp": e.Timestamp,
			"payload": json.RawMessage(e.Payload),
		})
		if len(entries) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "entries": entries})
}

// handleAutonomyRollback restores the policy captured by a sealed event.
// Only operator policy/halt events are rollback targets; the rollback itself is
// a new sealed event (history is never rewritten).
func (s *Server) handleAutonomyRollback(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "autonomy rollback is POST-only", nil)
		return
	}
	if !s.requireOperator(w, r) {
		return
	}
	var body struct {
		EventID int64 `json:"event_id"`
	}
	if err := decodeStrict(r, &body); err != nil || body.EventID <= 0 {
		WriteAPIError(w, "bad_request", "rollback needs a positive event_id", nil)
		return
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", "autonomy history read failed", nil)
		return
	}
	var target *ledger.EventRecord
	for i := range events {
		if events[i].ID == body.EventID {
			target = &events[i]
			break
		}
	}
	if target == nil {
		WriteAPIError(w, "not_found", "unknown autonomy event", nil)
		return
	}
	if target.EventType != "autonomy_policy_updated" && target.EventType != "autonomy_halted" {
		WriteAPIError(w, "bad_request", "only a policy update or halt event is a rollback target", nil)
		return
	}
	var captured struct {
		Stages          map[string]string `json:"stages"`
		MaxActBudgetUSD float64           `json:"max_act_budget_usd"`
		AgenticEnabled  *bool             `json:"agentic_enabled"`
		AgenticLocked   bool              `json:"agentic_locked"`
	}
	if err := json.Unmarshal(target.Payload, &captured); err != nil {
		WriteAPIError(w, "bad_request", "captured policy is unreadable", nil)
		return
	}
	stages := map[autonomy.Stage]autonomy.Mode{}
	for stage, mode := range captured.Stages {
		stages[autonomy.Stage(stage)] = autonomy.Mode(mode)
	}
	restored, err := autonomy.Normalize(autonomy.Policy{
		Stages: stages, MaxActBudgetUSD: captured.MaxActBudgetUSD,
		AgenticEnabled: captured.AgenticEnabled, AgenticLocked: captured.AgenticLocked,
	})
	if err != nil {
		WriteAPIError(w, "bad_request", "captured policy is invalid: "+err.Error(), nil)
		return
	}
	previous, _, err := s.loadAutonomyPolicyStrict(sessionID)
	if err != nil {
		WriteAPIError(w, "autonomy_policy_unreadable", err.Error(), nil)
		return
	}
	restored.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(restored)
	if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
		WriteAPIError(w, "internal", "rollback save failed: "+err.Error(), nil)
		return
	}
	paused := 0
	if !restored.Agentic() {
		paused = s.pauseSessionLoops(sessionID)
	}
	if err := s.recordManuscriptEvent(sessionID, "autonomy_policy_rolled_back", map[string]any{
		"source_event_id": body.EventID, "agentic_enabled": restored.Agentic(),
		"agentic_locked": restored.AgenticLocked, "paused_loops": paused,
	}); err != nil {
		WriteAPIError(w, "internal", "rollback event failed: "+err.Error(), nil)
		return
	}
	if restored.Agentic() != previous.Agentic() || restored.AgenticLocked != previous.AgenticLocked {
		_ = s.recordManuscriptEvent(sessionID, "agentic_mode_changed", map[string]any{
			"enabled": restored.Agentic(), "locked": restored.AgenticLocked,
			"actor": "operator", "paused_loops": paused,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": restored, "source_event_id": body.EventID})
}

// requireOperator enforces the operator bearer on policy mutations.
func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request) bool {
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK || tenant != "" {
		WriteAPIError(w, "forbidden", "autonomy mutations require the operator bearer", nil)
		return false
	}
	return true
}

// defaultPolicyForSession starts from the tenant's operator-set default when a
// session has no policy of its own.
func (s *Server) defaultPolicyForSession(sessionID string) (autonomy.Policy, string) {
	policy := autonomy.DefaultPolicy()
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil || sess == nil {
		return policy, "default"
	}
	if enabled, err := s.ledger.GetTenantAgenticDefault(sess.TenantID); err == nil && enabled != nil {
		policy.AgenticEnabled = enabled
		return policy, "tenant-default"
	}
	return policy, "default"
}

// loadAutonomyPolicyStrict refuses an unreadable policy instead of silently
// enabling the default. Enforcement paths use this.
func (s *Server) loadAutonomyPolicyStrict(sessionID string) (autonomy.Policy, string, error) {
	raw, err := s.ledger.GetAutonomyPolicy(sessionID)
	if err != nil {
		return autonomy.Policy{}, "", err
	}
	if strings.TrimSpace(raw) == "" {
		policy, source := s.defaultPolicyForSession(sessionID)
		return policy, source, nil
	}
	var policy autonomy.Policy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return autonomy.Policy{}, "", fmt.Errorf("stored autonomy policy is unreadable: %w", err)
	}
	normalized, err := autonomy.Normalize(policy)
	if err != nil {
		return autonomy.Policy{}, "", fmt.Errorf("stored autonomy policy is invalid: %w", err)
	}
	return normalized, "session", nil
}

// loadAutonomyPolicy returns the stored policy or the default, plus its source.
func (s *Server) loadAutonomyPolicy(sessionID string) (autonomy.Policy, string) {
	raw, err := s.ledger.GetAutonomyPolicy(sessionID)
	if err != nil || strings.TrimSpace(raw) == "" {
		return s.defaultPolicyForSession(sessionID)
	}
	var policy autonomy.Policy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return autonomy.DefaultPolicy(), "default"
	}
	normalized, err := autonomy.Normalize(policy)
	if err != nil {
		return autonomy.DefaultPolicy(), "default"
	}
	return normalized, "session"
}

// planRiskInput builds the classifier input from a stored plan and its spec.
func planRiskInput(plan *ledger.ExperimentPlan, policy autonomy.Policy, target string) autonomy.RiskInput {
	var spec map[string]any
	_ = json.Unmarshal([]byte(plan.SpecJSON), &spec)
	return autonomy.RiskInput{
		Method: plan.Method, Spec: spec, BudgetUSD: plan.BudgetUSD,
		Statement: plan.Statement, Target: target, MaxActBudgetUSD: policy.MaxActBudgetUSD,
	}
}
