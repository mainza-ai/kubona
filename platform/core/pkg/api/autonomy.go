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
		var body autonomy.Policy
		if err := decodeStrict(r, &body); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid autonomy policy: %v", err), nil)
			return
		}
		policy, err := autonomy.Normalize(body)
		if err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		policy.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		raw, _ := json.Marshal(policy)
		if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
			WriteAPIError(w, "internal", "autonomy policy save failed: "+err.Error(), nil)
			return
		}
		if err := s.recordManuscriptEvent(sessionID, "autonomy_policy_updated", map[string]any{
			"stages": policy.Stages, "max_act_budget_usd": policy.MaxActBudgetUSD,
		}); err != nil {
			WriteAPIError(w, "internal", "autonomy event failed: "+err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policy": policy, "source": "session"})
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
	halted := autonomy.Policy{
		Stages:          map[autonomy.Stage]autonomy.Mode{},
		MaxActBudgetUSD: policy.MaxActBudgetUSD, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, stage := range autonomy.Stages {
		halted.Stages[stage] = autonomy.ModeSuggest
	}
	raw, _ := json.Marshal(halted)
	if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
		WriteAPIError(w, "internal", "autonomy halt save failed: "+err.Error(), nil)
		return
	}
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
		"cancelled_jobs": cancelled, "max_act_budget_usd": halted.MaxActBudgetUSD,
	}); err != nil {
		WriteAPIError(w, "internal", "halt event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policy": halted, "source": "session", "cancelled_jobs": cancelled,
	})
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

// loadAutonomyPolicy returns the stored policy or the default, plus its source.
func (s *Server) loadAutonomyPolicy(sessionID string) (autonomy.Policy, string) {
	raw, err := s.ledger.GetAutonomyPolicy(sessionID)
	if err != nil || strings.TrimSpace(raw) == "" {
		return autonomy.DefaultPolicy(), "default"
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
