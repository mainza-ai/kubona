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

func (s *Server) handleAgenticModeRoute(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	policy, source := s.loadAutonomyPolicy(sessionID)
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
	policy, _ := s.loadAutonomyPolicy(sessionID)
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
	return paused
}
