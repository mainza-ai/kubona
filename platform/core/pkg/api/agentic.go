package api

// Agentic master switch (AG-17).
//
// Tenant autonomy (L0–L3) and the AG-7 stage ladder are the outer rails. This
// file adds the one user-facing switch above them:
//
//	session caller  PUT /agentic  -> flips the master switch only
//	operator        PUT /autonomy -> full policy, may clear an operator lock
//
// Off blocks agent loops and autonomous plan execution; the manual experiment
// rail stays available. An operator halt locks the switch so a session caller
// cannot re-enable the workflow the operator stopped.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
)

func agenticModeSubpath(id string) (sessionID string, ok bool) {
	if !strings.HasSuffix(id, "/agentic") {
		return "", false
	}
	sid := strings.TrimSuffix(id, "/agentic")
	if sid == "" || strings.Contains(sid, "/") {
		return "", false
	}
	return sid, true
}

func agenticDigestSubpath(id string) (sessionID string, ok bool) {
	if !strings.HasSuffix(id, "/agentic/digest") {
		return "", false
	}
	sid := strings.TrimSuffix(id, "/agentic/digest")
	if sid == "" || strings.Contains(sid, "/") {
		return "", false
	}
	return sid, true
}

// handleAgenticDigest returns the batched receipts of notify-mode execution
// (AG-20). It is a projection of sealed plan_executed events, not a second log.
func (s *Server) handleAgenticDigest(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "agentic digest is GET-only", nil)
		return
	}
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	hours := 24
	if raw := r.URL.Query().Get("hours"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 168 {
			WriteAPIError(w, "bad_request", "hours must be an integer in [1,168]", nil)
			return
		}
		hours = n
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		WriteAPIError(w, "internal", "digest read failed", nil)
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	entries := make([]map[string]any, 0)
	counts := map[string]int{"total": 0, "accepted": 0, "notify": 0}
	for _, e := range events {
		if e.EventType != "plan_executed" {
			continue
		}
		at, ok := parseLedgerTime(e.Timestamp)
		if !ok || at.Before(cutoff) {
			continue
		}
		var body struct {
			PlanID       string `json:"plan_id"`
			JobID        string `json:"job_id"`
			Accepted     bool   `json:"accepted"`
			Status       string `json:"status"`
			AutoApproved bool   `json:"auto_approved"`
			Notify       bool   `json:"notify"`
			Risk         string `json:"risk"`
		}
		if json.Unmarshal(e.Payload, &body) != nil {
			continue
		}
		entries = append(entries, map[string]any{
			"plan_id": body.PlanID, "job_id": body.JobID, "accepted": body.Accepted,
			"status": body.Status, "auto_approved": body.AutoApproved,
			"notify": body.Notify, "risk": body.Risk, "at": e.Timestamp,
		})
		counts["total"]++
		if body.Accepted {
			counts["accepted"]++
		}
		if body.Notify {
			counts["notify"]++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID, "hours": hours, "entries": entries, "counts": counts,
	})
}

func (s *Server) handleAgenticModeRoute(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	policy, source, err := s.loadAutonomyPolicyStrict(sessionID)
	if err != nil {
		WriteAPIError(w, "autonomy_policy_unreadable", err.Error(), nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": sessionID, "agentic_enabled": policy.Agentic(),
			"agentic_locked": policy.AgenticLocked, "stages": policy.Stages,
			"max_act_budget_usd": policy.MaxActBudgetUSD, "source": source,
		})
	case http.MethodPut:
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decodeStrict(r, &body); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid agentic request: %v", err), nil)
			return
		}
		if body.Enabled == nil {
			WriteAPIError(w, "bad_request", "agentic request needs enabled (true|false)", nil)
			return
		}
		if policy.AgenticLocked {
			WriteAPIError(w, "agentic_locked",
				"an operator halted agentic work; only an operator can clear the lock", nil)
			return
		}
		policy.AgenticEnabled = body.Enabled
		policy.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		raw, _ := json.Marshal(policy)
		if err := s.ledger.SetAutonomyPolicy(sessionID, string(raw)); err != nil {
			WriteAPIError(w, "internal", "agentic mode save failed: "+err.Error(), nil)
			return
		}
		paused := 0
		if !policy.Agentic() {
			paused = s.pauseSessionLoops(sessionID)
		}
		if err := s.recordManuscriptEvent(sessionID, "agentic_mode_changed", map[string]any{
			"enabled": policy.Agentic(), "locked": policy.AgenticLocked,
			"actor": "session", "paused_loops": paused,
		}); err != nil {
			WriteAPIError(w, "internal", "agentic event failed: "+err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"policy": policy, "agentic_enabled": policy.Agentic(),
			"agentic_locked": policy.AgenticLocked, "paused_loops": paused,
		})
	default:
		WriteAPIError(w, "method_not_allowed", "agentic mode supports GET and PUT", nil)
	}
}

// requireAgentic refuses when the master switch is off or operator-locked.
func (s *Server) requireAgentic(w http.ResponseWriter, sessionID string) bool {
	policy, _, err := s.loadAutonomyPolicyStrict(sessionID)
	if err != nil {
		WriteAPIError(w, "autonomy_policy_unreadable", err.Error(), nil)
		return false
	}
	if policy.AgenticLocked {
		WriteAPIError(w, "agentic_locked",
			"an operator halted agentic work; only an operator can clear the lock", nil)
		return false
	}
	if !policy.Agentic() {
		WriteAPIError(w, "agentic_disabled",
			"agentic mode is off for this session: turn it on or run the experiment manually", nil)
		return false
	}
	return true
}

// pauseSessionLoops pauses every running loop owned by the session and returns
// the count. It never aborts: pausing preserves the state and spend books so
// the workflow can resume when the switch is turned back on.
func (s *Server) pauseSessionLoops(sessionID string) int {
	s.autoresearchMu.RLock()
	engines := make([]*autoresearch.LoopEngine, 0, len(s.activeAutoloops))
	for _, e := range s.activeAutoloops {
		if e.State().SessionID == sessionID {
			engines = append(engines, e)
		}
	}
	s.autoresearchMu.RUnlock()
	paused := 0
	for _, e := range engines {
		if e.State().Status != autoresearch.StatusRunning {
			continue
		}
		e.Pause()
		s.persistLoopRow(e)
		paused++
	}
	if s.supervisor != nil {
		paused += s.supervisor.PauseSessionLoops(sessionID)
	}
	return paused
}
