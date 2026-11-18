package api

// Research graph (AG-4a) with context-budgeted summary and resume (AG-4b).
//
// One read-only projection of the session's scientific state — goal, target,
// hypotheses, plans, jobs/experiments, claims, and manuscripts — assembled
// from the ledger, not a second store. The optional `budget_bytes` adds a
// deterministic summary for a resuming agent; `since_seq` adds the ledger
// events after a cursor, so a fresh process can rebuild and catch up without
// replaying the whole session.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/researchgraph"
)

func (s *Server) handleResearchGraph(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "research graph is GET-only", nil)
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
	budget := 4096
	if raw := r.URL.Query().Get("budget_bytes"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 256 || n > 262144 {
			WriteAPIError(w, "bad_request", "budget_bytes must be an integer in [256,262144]", nil)
			return
		}
		budget = n
	}
	var campaign any = map[string]any{}
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}
	hypotheses, err := s.ledger.ListHypotheses(sessionID, 100)
	if err != nil {
		WriteAPIError(w, "internal", "hypothesis read failed", nil)
		return
	}
	plans, err := s.ledger.ListPlans(sessionID, 100)
	if err != nil {
		WriteAPIError(w, "internal", "plan read failed", nil)
		return
	}
	claims, err := s.ledger.ListClaimRecords(sessionID, 100)
	if err != nil {
		WriteAPIError(w, "internal", "claim read failed", nil)
		return
	}
	manuscripts, _, err := s.ledger.ListManuscriptsPage(sessionID, 100, 0)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	jobs, err := s.jobRunner.ListJobs(sessionID, 100)
	if err != nil {
		WriteAPIError(w, "internal", "job read failed", nil)
		return
	}
	jobView := make([]map[string]any, 0, len(jobs))
	summaryRuns := make([]researchgraph.Run, 0, len(jobs))
	for _, j := range jobs {
		result := any(nil)
		if len(j.ResultJSON) > 0 {
			var v any
			if json.Unmarshal(j.ResultJSON, &v) == nil {
				result = v
			}
		}
		jobView = append(jobView, map[string]any{
			"id": j.ID, "type": j.JobType, "status": j.Status.String(),
			"created_at": j.CreatedAt, "completed_at": j.CompletedAt,
			"result": result,
		})
		summaryRuns = append(summaryRuns, researchgraph.Run{
			ID: j.ID, Type: j.JobType, Status: j.Status.String(), CreatedAt: j.CreatedAt.Format(time.RFC3339),
		})
	}
	summaryGraph := buildSummaryGraph(sessionID, campaign, hypotheses, plans, claims, summaryRuns, len(manuscripts))
	summary, report := researchgraph.Summarize(summaryGraph, budget)

	payload := map[string]any{
		"session_id":     sessionID,
		"campaign":       campaign,
		"hypotheses":     hypotheses,
		"plans":          plans,
		"jobs":           jobView,
		"claims":         claims,
		"manuscripts":    manuscripts,
		"summary":        summary,
		"summary_report": report,
		"counts": map[string]any{
			"hypotheses": len(hypotheses), "plans": len(plans),
			"jobs": len(jobView), "claims": len(claims), "manuscripts": len(manuscripts),
		},
	}
	if r.URL.Query().Has("since_seq") {
		since, err := strconv.ParseInt(r.URL.Query().Get("since_seq"), 10, 64)
		if err != nil || since < 0 {
			WriteAPIError(w, "bad_request", "since_seq must be a non-negative integer", nil)
			return
		}
		limit := 200
		if raw := r.URL.Query().Get("event_limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 500 {
				WriteAPIError(w, "bad_request", "event_limit must be an integer in [1,500]", nil)
				return
			}
			limit = n
		}
		events, err := s.ledger.GetEventsSince(sessionID, since, limit+1)
		if err != nil {
			WriteAPIError(w, "internal", "event replay failed", nil)
			return
		}
		hasMore := len(events) > limit
		if hasMore {
			events = events[:limit]
		}
		next := since
		view := make([]map[string]any, 0, len(events))
		for _, e := range events {
			next = e.ID
			view = append(view, map[string]any{
				"id": e.ID, "event_type": e.EventType, "timestamp": e.Timestamp,
				"payload": json.RawMessage(e.Payload),
			})
		}
		payload["resume"] = map[string]any{
			"since_seq": since, "next_seq": next, "has_more": hasMore, "events": view,
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func buildSummaryGraph(
	sessionID string,
	campaign any,
	hypotheses []ledger.Hypothesis,
	plans []ledger.ExperimentPlan,
	claims []ledger.ClaimRecord,
	runs []researchgraph.Run,
	manuscripts int,
) researchgraph.Graph {
	g := researchgraph.Graph{
		SessionID: sessionID, Runs: runs, ManuscriptCount: manuscripts,
	}
	if m, ok := campaign.(map[string]any); ok {
		if v, ok := m["research_goal"].(string); ok {
			g.Goal = v
		}
		if v, ok := m["target_system"].(string); ok {
			g.Target = v
		}
		if measurement, ok := m["measurement"].(map[string]any); ok {
			if v, ok := measurement["delta_mha"].(float64); ok && v != 0 {
				g.DeltaMHa = &v
			}
			if v, ok := measurement["claimable"].(bool); ok {
				g.Claimable = &v
			}
		}
		if g.DeltaMHa == nil {
			if nb, ok := m["notebook"].(map[string]any); ok {
				if v, ok := nb["delta_mha"].(float64); ok && v != 0 {
					g.DeltaMHa = &v
				}
			}
		}
	}
	for _, h := range hypotheses {
		g.Hypotheses = append(g.Hypotheses, researchgraph.Hypothesis{
			ID: h.ID, Statement: h.Statement, Rationale: h.Rationale, Falsifier: h.Falsifier,
			Status: h.Status, Source: h.Source, Score: h.Score, ClaimID: h.ClaimID,
		})
	}
	for _, p := range plans {
		plan := researchgraph.Plan{
			ID: p.ID, HypothesisID: p.HypothesisID, Statement: p.Statement, Status: p.Status,
			Observable: p.Observable, AcceptanceMetric: p.AcceptanceMetric,
		}
		if len(p.ResultJSON) > 0 {
			var scored struct {
				Accepted *bool `json:"accepted"`
			}
			if json.Unmarshal([]byte(p.ResultJSON), &scored) == nil {
				plan.Accepted = scored.Accepted
			}
		}
		g.Plans = append(g.Plans, plan)
	}
	for _, c := range claims {
		g.Claims = append(g.Claims, researchgraph.Claim{
			ID: c.ID, Statement: c.Statement, Verdict: c.Verdict,
		})
	}
	return g
}

// researchGraphSubpath matches "<session>/research-graph".
func researchGraphSubpath(id string) (string, bool) {
	if !strings.HasSuffix(id, "/research-graph") {
		return "", false
	}
	sid := strings.TrimSuffix(id, "/research-graph")
	if sid == "" || strings.Contains(sid, "/") {
		return "", false
	}
	return sid, true
}
