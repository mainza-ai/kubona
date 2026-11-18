package api

// Hypothesis engine HTTP surface (AG-2).
//
// The generator lives in pkg/hypotheses: evidence-grounded, falsifiable probes
// derived from the measured campaign state. This file owns persistence,
// tenancy, literature attachment, and the shortlist decision. Novelty is never
// invented here: it is "unmeasured" unless a caller supplies a measured
// distance later.

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
	if strings.Contains(rest, "/") {
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
	if err := s.ledger.SetHypothesisStatus(sessionID, rest, body.Status); err != nil {
		if errors.Is(err, ledger.ErrHypothesisNotFound) {
			WriteAPIError(w, "not_found", "unknown hypothesis", nil)
			return
		}
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "hypothesis_status_changed", map[string]any{
		"hypothesis_id": rest, "status": body.Status,
	}); err != nil {
		WriteAPIError(w, "internal", "hypothesis event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": rest, "status": body.Status})
}

func (s *Server) handleHypothesisGenerate(w http.ResponseWriter, r *http.Request, sessionID, tenantID string) {
	var body struct {
		Goal  string `json:"goal"`
		Count int    `json:"count"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid hypothesis request: %v", err), nil)
		return
	}
	var campaign agents.CampaignState
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &campaign)
	}
	literature := s.hypothesisLiterature(r, campaign, body.Goal)
	candidates := hypotheses.Generate(&campaign, literature, body.Count)
	if len(candidates) == 0 {
		WriteAPIError(w, "bad_request", "no hypotheses could be derived from the current state", nil)
		return
	}
	stored := make([]ledger.Hypothesis, 0, len(candidates))
	for i, c := range candidates {
		h := &ledger.Hypothesis{
			ID:             fmt.Sprintf("hyp_%s_%d", newManuscriptID()[3:11], i),
			Statement:      c.Statement,
			Rationale:      c.Rationale,
			Falsifier:      c.Falsifier,
			ExperimentKind: c.ExperimentKind,
			ExperimentHint: c.ExperimentHint,
			Literature:     c.Literature,
			NoveltyStatus:  "unmeasured",
			Score:          c.Score,
			Status:         ledger.HypothesisProposed,
			Source:         c.Source,
		}
		if err := s.ledger.CreateHypothesis(sessionID, tenantID, h); err != nil {
			WriteAPIError(w, "internal", "hypothesis persistence failed: "+err.Error(), nil)
			return
		}
		stored = append(stored, *h)
	}
	if err := s.recordManuscriptEvent(sessionID, "hypotheses_generated", map[string]any{
		"count": len(stored), "goal": strings.TrimSpace(body.Goal),
		"target": campaign.TargetSystem,
	}); err != nil {
		WriteAPIError(w, "internal", "hypothesis event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"hypotheses": stored})
}

// hypothesisLiterature attaches real references when the bridge is available.
// A failure is not fatal: novelty stays "unmeasured" and the probe says so.
func (s *Server) hypothesisLiterature(r *http.Request, campaign agents.CampaignState, goal string) []string {
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
	refs := make([]string, 0, len(resp.Records))
	for _, rec := range resp.Records {
		if rec.Id != "" {
			refs = append(refs, rec.Id)
		}
	}
	return refs
}
