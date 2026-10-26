package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleCampaignRoute maps /api/campaigns/{id}/flight-recorder to the flight recorder handler
func (s *Server) handleCampaignRoute(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/campaigns/")
	if strings.HasSuffix(id, "/flight-recorder") {
		sessionID := strings.TrimSuffix(id, "/flight-recorder")
		s.handleCampaignFlightRecorder(w, r, sessionID)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

// handleCampaignFlightRecorder provides a unified 60-second cold-auditor flight recorder endpoint (B.1)
func (s *Server) handleCampaignFlightRecorder(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// 1. Terminal verdict and stop reason from claims or recent events
	terminalVerdict := "IN_PROGRESS"
	stopReason := "Active"
	claims, _ := s.ledger.ListClaimRecords(sessionID, 5)
	if len(claims) > 0 {
		terminalVerdict = claims[0].Verdict
		stopReason = claims[0].Statement
	}

	// 2. Budget and spend metrics
	spentUSD := sess.CurrentSpendUSD
	spendLimitUSD := sess.SpendLimitUSD

	// Inspect recent events to determine exact halt reason and step count
	events, _ := s.ledger.GetRecentEvents(sessionID, 50)
	stepCount := 0
	for _, e := range events {
		if e.EventType == "tool_loop_capped" || e.EventType == "tool_loop_breaker" {
			terminalVerdict = "BUDGET_EXHAUSTED"
			stopReason = "Session spend ceiling or tool loop step bound reached"
		} else if e.EventType == "loop_diverged" {
			terminalVerdict = "DIVERGED"
			stopReason = "Loop diverged beyond tolerance"
		} else if e.EventType == "loop_killed" {
			terminalVerdict = "ABORTED"
			stopReason = "Loop explicitly aborted by operator or supervisor"
		}
		if e.EventType == "tool_called" || e.EventType == "tribunal_turn" {
			stepCount++
		}
	}

	rawState, _ := s.ledger.GetCampaignState(sessionID)
	duration := time.Since(sess.CreatedAt).Seconds()

	resp := map[string]any{
		"ok":               true,
		"session_id":       sessionID,
		"tenant_id":        sess.TenantID,
		"terminal_verdict": terminalVerdict,
		"spent_usd":        spentUSD,
		"spend_limit_usd":  spendLimitUSD,
		"duration_seconds": duration,
		"step_count":       stepCount,
		"stop_reason":      stopReason,
		"last_claims":      claims,
		"recent_events":    events,
		"campaign_state":   json.RawMessage(rawState),
	}
	writeJSON(w, http.StatusOK, resp)
}
