package api

// Research graph (AG-4a): one read-only projection of the session's scientific
// state — goal, target, hypotheses, plans, jobs/experiments, claims, and
// manuscripts. It is assembled from the ledger, not a second store, so it
// cannot drift from the evidence it summarizes.

import (
	"encoding/json"
	"net/http"
	"strings"
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
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":  sessionID,
		"campaign":    campaign,
		"hypotheses":  hypotheses,
		"plans":       plans,
		"jobs":        jobView,
		"claims":      claims,
		"manuscripts": manuscripts,
		"counts": map[string]any{
			"hypotheses": len(hypotheses), "plans": len(plans),
			"jobs": len(jobView), "claims": len(claims), "manuscripts": len(manuscripts),
		},
	})
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
