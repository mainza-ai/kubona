package api

// Hypothesis engine HTTP surface (AG-2).
//
// The deterministic generator lives in pkg/hypotheses: evidence-grounded,
// falsifiable probes derived from the measured campaign state. An optional LLM
// proposer adds model-authored candidates under a strict JSON contract. This
// file owns persistence, tenancy, literature attachment, measured novelty, the
// tournament, and the collapse decision.
//
// Novelty is never invented here: it is "unmeasured" unless a prior-art corpus
// exists and the lexical measure produced a labelled distance. Collapse is
// never a status change masquerading as evidence: it needs a completed,
// accepted plan or an existing verified claim.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/hypotheses"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	pb "github.com/mainza-ai/kubona/core/proto"
)

func hypothesisSubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/hypotheses")
	if idx <= 0 {
		return "", "", false
	}
	rest = strings.TrimPrefix(id[idx:], "/hypotheses")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(rest, "/"), true
}

func (s *Server) handleHypothesisRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			limit := 50
			if raw := r.URL.Query().Get("limit"); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			items, err := s.ledger.ListHypotheses(sessionID, limit)
			if err != nil {
				WriteAPIError(w, "internal", "hypothesis list failed", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"hypotheses": items})
		case http.MethodPost:
			s.handleHypothesisGenerate(w, r, sessionID, sess.TenantID)
		default:
			WriteAPIError(w, "method_not_allowed", "hypotheses support GET and POST", nil)
		}
		return
	}
	if rest == "tournament" {
		s.handleHypothesisTournament(w, r, sessionID)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[1] == "collapse" && parts[0] != "" {
		s.handleHypothesisCollapse(w, r, sessionID, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		WriteAPIError(w, "not_found", "unknown hypothesis route", nil)
		return
	}
	if r.Method != http.MethodPatch {
		WriteAPIError(w, "method_not_allowed", "hypothesis status is PATCH-only", nil)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid status: %v", err), nil)
		return
	}
	if strings.EqualFold(strings.TrimSpace(body.Status), ledger.HypothesisCollapsed) {
		WriteAPIError(w, "bad_request",
			"collapse needs evidence: POST /hypotheses/{id}/collapse with plan_id or claim_id", nil)
		return
	}
	if err := s.ledger.SetHypothesisStatus(sessionID, parts[0], body.Status); err != nil {
		if errors.Is(err, ledger.ErrHypothesisNotFound) {
			WriteAPIError(w, "not_found", "unknown hypothesis", nil)
			return
		}
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "hypothesis_status_changed", map[string]any{
		"hypothesis_id": parts[0], "status": body.Status,
	}); err != nil {
		WriteAPIError(w, "internal", "hypothesis event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": parts[0], "status": body.Status})
}

func (s *Server) handleHypothesisGenerate(w http.ResponseWriter, r *http.Request, sessionID, tenantID string) {
	var body struct {
		Goal   string `json:"goal"`
		Count  int    `json:"count"`
		UseLLM bool   `json:"use_llm"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid hypothesis request: %v", err), nil)
		return
	}
	var campaign agents.CampaignState
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}
	priorArt := s.hypothesisLiterature(r, campaign, body.Goal)
	refs := make([]string, 0, len(priorArt))
	for _, p := range priorArt {
		if p.ID != "" {
			refs = append(refs, p.ID)
		}
	}
	candidates := hypotheses.Generate(&campaign, refs, body.Count)
	llmInfo := map[string]any{"requested": body.UseLLM, "used": false}
	if body.UseLLM {
		if s.hypothesisCompleter == nil {
			llmInfo["error"] = "no LLM provider is configured"
		} else {
			// The handler is synchronous, so an unbounded provider hang would
			// hold the request open indefinitely.
			ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
			proposed, err := hypotheses.Propose(ctx, s.hypothesisCompleter, hypotheses.ProposeOptions{
				Goal: body.Goal, Target: campaign.TargetSystem,
				Measured:   campaign.Measurement != nil && campaign.Measurement.Measured,
				DeltaMHa:   campaignDeltaMHa(&campaign),
				Hardware:   campaign.Measurement != nil && campaign.Measurement.Hardware,
				Claimable:  campaign.Measurement != nil && campaign.Measurement.Claimable,
				Literature: priorArt, Count: body.Count,
			})
			cancel()
			if err != nil {
				llmInfo["error"] = err.Error()
			} else {
				llmInfo["used"] = true
				llmInfo["count"] = len(proposed)
				candidates = hypotheses.Merge(candidates, proposed, body.Count)
			}
		}
	}
	if len(candidates) == 0 {
		WriteAPIError(w, "bad_request", "no hypotheses could be derived from the current state", nil)
		return
	}
	for i := range candidates {
		c := &candidates[i]
		if distance, method, ok := hypotheses.MeasureNovelty(c.Statement, priorArt); ok {
			c.NoveltyStatus, c.NoveltyMethod = "measured", method
			c.NoveltyDistance = &distance
		} else {
			c.NoveltyStatus, c.NoveltyMethod, c.NoveltyDistance = "unmeasured", "", nil
		}
	}
	stored := make([]ledger.Hypothesis, 0, len(candidates))
	for i, c := range candidates {
		h := &ledger.Hypothesis{
			ID:              fmt.Sprintf("hyp_%s_%d", newManuscriptID()[3:11], i),
			Statement:       c.Statement,
			Rationale:       c.Rationale,
			Falsifier:       c.Falsifier,
			ExperimentKind:  c.ExperimentKind,
			ExperimentHint:  c.ExperimentHint,
			Literature:      c.Literature,
			NoveltyStatus:   c.NoveltyStatus,
			NoveltyMethod:   c.NoveltyMethod,
			NoveltyDistance: c.NoveltyDistance,
			Score:           c.Score,
			Status:          ledger.HypothesisProposed,
			Source:          c.Source,
		}
		if err := s.ledger.CreateHypothesis(sessionID, tenantID, h); err != nil {
			WriteAPIError(w, "internal", "hypothesis persistence failed: "+err.Error(), nil)
			return
		}
		stored = append(stored, *h)
	}
	if err := s.recordManuscriptEvent(sessionID, "hypotheses_generated", map[string]any{
		"count": len(stored), "goal": strings.TrimSpace(body.Goal),
		"target": campaign.TargetSystem, "llm_used": llmInfo["used"],
	}); err != nil {
		WriteAPIError(w, "internal", "hypothesis event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"hypotheses": stored, "llm": llmInfo})
}

func campaignDeltaMHa(c *agents.CampaignState) float64 {
	if c == nil {
		return 0
	}
	if c.Measurement != nil && c.Measurement.Measured {
		if d := c.Measurement.DeltaMHa; d != 0 {
			return absFloat(d)
		}
	}
	if c.Notebook != nil && c.Notebook.DeltaMHa != 0 {
		return absFloat(c.Notebook.DeltaMHa)
	}
	return 0
}

func (s *Server) handleHypothesisTournament(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "hypothesis tournament is POST-only", nil)
		return
	}
	var body struct {
		Rounds int `json:"rounds"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid tournament request: %v", err), nil)
		return
	}
	items, err := s.ledger.ListHypotheses(sessionID, 200)
	if err != nil {
		WriteAPIError(w, "internal", "hypothesis read failed", nil)
		return
	}
	competitors := make([]hypotheses.Competitor, 0, len(items))
	for _, h := range items {
		if h.Status == ledger.HypothesisRejected || h.Status == ledger.HypothesisCollapsed {
			continue
		}
		competitors = append(competitors, hypotheses.Competitor{
			ID: h.ID,
			Candidate: hypotheses.Candidate{
				Statement: h.Statement, Rationale: h.Rationale, Falsifier: h.Falsifier,
				ExperimentKind: h.ExperimentKind, ExperimentHint: h.ExperimentHint,
				Literature: h.Literature, Score: h.Score, Source: h.Source,
				NoveltyStatus: h.NoveltyStatus,
			},
		})
	}
	if len(competitors) < 2 {
		WriteAPIError(w, "bad_request", "a tournament needs at least two open hypotheses", nil)
		return
	}
	result := hypotheses.RunTournament(competitors, body.Rounds, nil)
	byID := make(map[string]ledger.Hypothesis, len(items))
	for _, h := range items {
		byID[h.ID] = h
	}
	ranking := make([]map[string]any, 0, len(result.Ranking))
	for _, c := range result.Ranking {
		if err := s.ledger.SetHypothesisScore(sessionID, c.ID, c.Candidate.Score); err != nil {
			WriteAPIError(w, "internal", "score persistence failed: "+err.Error(), nil)
			return
		}
		h := byID[c.ID]
		h.Score = c.Candidate.Score
		ranking = append(ranking, map[string]any{
			"id": h.ID, "statement": h.Statement, "score": h.Score,
			"status": h.Status, "source": h.Source,
		})
	}
	if err := s.recordManuscriptEvent(sessionID, "hypotheses_tournament", map[string]any{
		"judge": result.Judge, "rounds": result.Rounds, "matches": len(result.Matches),
		"ranking": ranking, "meta_review": result.MetaReview,
	}); err != nil {
		WriteAPIError(w, "internal", "tournament event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"judge": result.Judge, "rounds": result.Rounds,
		"matches": len(result.Matches), "ranking": ranking,
		"meta_review": result.MetaReview,
	})
}

func (s *Server) handleHypothesisCollapse(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "collapse is POST-only", nil)
		return
	}
	var body struct {
		PlanID  string `json:"plan_id"`
		ClaimID string `json:"claim_id"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid collapse request: %v", err), nil)
		return
	}
	h, err := s.ledger.GetHypothesis(sessionID, id)
	if errors.Is(err, ledger.ErrHypothesisNotFound) {
		WriteAPIError(w, "not_found", "unknown hypothesis", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "hypothesis read failed", nil)
		return
	}
	if h.Status == ledger.HypothesisRejected {
		WriteAPIError(w, "bad_request", "a rejected hypothesis cannot be collapsed", nil)
		return
	}
	claimID := strings.TrimSpace(body.ClaimID)
	planID := strings.TrimSpace(body.PlanID)
	var evidence map[string]any
	switch {
	case claimID != "":
		rec, err := s.ledger.GetClaimRecord(sessionID, claimID)
		if err != nil {
			WriteAPIError(w, "internal", "claim read failed", nil)
			return
		}
		if rec == nil {
			WriteAPIError(w, "not_found", "unknown claim", nil)
			return
		}
		if rec.Verdict != "verified" {
			WriteAPIError(w, "hypothesis_not_evidenced",
				fmt.Sprintf("claim %s has verdict %q; only a verified claim can collapse a hypothesis", claimID, rec.Verdict), nil)
			return
		}
		evidence = map[string]any{"claim_id": claimID, "verdict": rec.Verdict}
	case planID != "":
		plan, err := s.ledger.GetPlan(sessionID, planID)
		if errors.Is(err, ledger.ErrPlanNotFound) {
			WriteAPIError(w, "not_found", "unknown plan", nil)
			return
		}
		if err != nil {
			WriteAPIError(w, "internal", "plan read failed", nil)
			return
		}
		if plan.Status != ledger.PlanCompleted {
			WriteAPIError(w, "hypothesis_not_evidenced",
				fmt.Sprintf("plan %s is %s; only a completed plan can collapse a hypothesis", planID, plan.Status), nil)
			return
		}
		var scored struct {
			Accepted  bool    `json:"accepted"`
			JobID     string  `json:"job_id"`
			Measured  float64 `json:"measured"`
			Target    float64 `json:"target"`
			Tolerance float64 `json:"tolerance"`
		}
		if err := json.Unmarshal([]byte(plan.ResultJSON), &scored); err != nil || !scored.Accepted {
			WriteAPIError(w, "hypothesis_not_evidenced",
				"the plan did not accept its registered acceptance metric", nil)
			return
		}
		evidence = map[string]any{
			"plan_id": plan.ID, "job_id": scored.JobID, "observable": plan.Observable,
			"measured": scored.Measured, "target": scored.Target,
			"tolerance": scored.Tolerance, "acceptance_metric": plan.AcceptanceMetric,
		}
		raw, _ := json.Marshal(evidence)
		// 0.9/0.05/0.05 is the platform's verified-claim prior; the measured
		// plan result is the evidence, never a re-derived probability.
		rec, err := s.ledger.MintClaimRecord(sessionID, "hypothesis_collapse",
			h.Statement, 0.9, 0.05, 0.05, "verified", string(raw))
		if err != nil {
			WriteAPIError(w, "internal", "claim minting failed: "+err.Error(), nil)
			return
		}
		claimID = rec.ID
		if job, jerr := s.jobRunner.GetJob(scored.JobID); jerr == nil && job != nil && job.SessionID == sessionID {
			s.linkJobEvidence(sessionID, claimID, job)
		}
	default:
		WriteAPIError(w, "hypothesis_not_evidenced",
			"collapse needs a completed accepted plan_id or an existing verified claim_id", nil)
		return
	}
	if err := s.ledger.CollapseHypothesis(sessionID, id, claimID); err != nil {
		WriteAPIError(w, "internal", "collapse failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "hypothesis_status_changed", map[string]any{
		"hypothesis_id": id, "status": ledger.HypothesisCollapsed,
		"claim_id": claimID, "evidence": evidence,
	}); err != nil {
		WriteAPIError(w, "internal", "collapse event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "status": ledger.HypothesisCollapsed,
		"claim_id": claimID, "evidence": evidence,
	})
}

// hypothesisLiterature attaches real references when the bridge is available.
// A failure is not fatal: novelty stays "unmeasured" and the probe says so.
func (s *Server) hypothesisLiterature(r *http.Request, campaign agents.CampaignState, goal string) []hypotheses.PriorArt {
	client := s.bridgeClient()
	if client == nil {
		return nil
	}
	query := strings.TrimSpace(goal)
	if query == "" {
		query = strings.TrimSpace(campaign.ResearchGoal)
	}
	if query == "" {
		query = campaign.TargetSystem
	}
	if query == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	resp, err := client.GroundLiterature(ctx, &pb.LiteratureGroundingRequest{Query: query, Limit: 5})
	if err != nil {
		return nil
	}
	art := make([]hypotheses.PriorArt, 0, len(resp.Records))
	for _, rec := range resp.Records {
		if rec.Id == "" {
			continue
		}
		art = append(art, hypotheses.PriorArt{
			ID: rec.Id, Title: rec.Title, Abstract: rec.AbstractText,
		})
	}
	return art
}
