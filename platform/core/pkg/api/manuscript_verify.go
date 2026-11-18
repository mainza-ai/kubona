package api

// Manuscript verification (AG-5).
//
// Verification checks two things against recorded state, never against the
// manuscript itself: every measurement-like number must appear in a job result,
// claim evidence, plan result, or campaign snapshot, and every citation must
// resolve externally (DOI match, or a close title match when no DOI is given).
//
// The report is persisted bound to the content hash and recomputed at signoff.
// A fabricated number or dead citation refuses signoff; an unavailable
// resolver refuses unless the operator explicitly acknowledges it. The
// acknowledgment is sealed in the signed event, so "unverified" is never
// silently promoted to "verified".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/verify"
	pb "github.com/mainza-ai/kubona/core/proto"
)

func (s *Server) handleManuscriptVerification(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if strings.TrimSpace(ms.VerificationJSON) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"report": nil, "status": "not_verified"})
			return
		}
		var report verify.Report
		if err := json.Unmarshal([]byte(ms.VerificationJSON), &report); err != nil {
			WriteAPIError(w, "internal", "stored verification report is unreadable", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"report": report, "status": report.Summary.Status})
	case http.MethodPost:
		report, err := s.verifyManuscript(r.Context(), sessionID, ms)
		if err != nil {
			WriteAPIError(w, "internal", "verification failed: "+err.Error(), nil)
			return
		}
		if err := s.ledger.SetManuscriptVerification(sessionID, id, report.JSON()); err != nil {
			WriteAPIError(w, "internal", "verification persistence failed: "+err.Error(), nil)
			return
		}
		if err := s.recordManuscriptEvent(sessionID, "manuscript_verified", map[string]any{
			"manuscript_id": id, "content_hash": ms.ContentHash,
			"status": report.Summary.Status, "numbers_checked": report.Summary.NumbersChecked,
			"citations_checked": report.Summary.CitationsChecked,
			"blocking":          report.Summary.Blocking,
		}); err != nil {
			WriteAPIError(w, "internal", "verification event failed: "+err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"report": report, "status": report.Summary.Status})
	default:
		WriteAPIError(w, "method_not_allowed", "verification supports GET and POST", nil)
	}
}

// verifyManuscript computes the report for the manuscript's current content.
func (s *Server) verifyManuscript(ctx context.Context, sessionID string, ms *ledger.Manuscript) (verify.Report, error) {
	evidence := s.manuscriptVerificationEvidence(sessionID, ms)
	numbers := verify.VerifyNumbers(manuscriptText(ms), evidence)
	citations := make([]verify.Citation, 0, len(ms.Citations))
	for _, c := range ms.Citations {
		citations = append(citations, verify.Citation{Key: c.Key, Title: c.Title, DOI: c.DOI})
	}
	findings := verify.VerifyCitations(ctx, citations, bridgeCitationResolver{client: s.bridgeClient()})
	return verify.BuildReport(ms.ContentHash, numbers, findings, time.Now()), nil
}

// manuscriptVerificationEvidence collects the recorded numbers a manuscript is
// checked against. The manuscript's own prose and review scores are excluded:
// a document cannot verify itself.
func (s *Server) manuscriptVerificationEvidence(sessionID string, ms *ledger.Manuscript) verify.Evidence {
	docs := []verify.Document{}
	seen := map[string]bool{}
	add := func(id, raw string) {
		if strings.TrimSpace(raw) == "" || seen[id] {
			return
		}
		seen[id] = true
		docs = append(docs, verify.Document{ID: id, JSON: raw})
	}
	add("manuscript.evidence", ms.EvidenceJSON)
	add("manuscript.campaign", ms.CampaignJSON)
	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil {
		add("campaign.live", string(raw))
	}
	if jobs, err := s.jobRunner.ListJobs(sessionID, 200); err == nil {
		for _, j := range jobs {
			add("job:"+j.ID, string(j.ResultJSON))
		}
	}
	if claims, err := s.ledger.ListClaimRecords(sessionID, 200); err == nil {
		for _, c := range claims {
			add("claim:"+c.ID, c.Evidence)
		}
	}
	if plans, err := s.ledger.ListPlans(sessionID, 200); err == nil {
		for _, p := range plans {
			add("plan:"+p.ID, p.ResultJSON)
		}
	}
	return verify.Evidence{Documents: docs}
}

func manuscriptText(ms *ledger.Manuscript) string {
	parts := []string{ms.Title, ms.Abstract}
	for _, sec := range ms.Sections {
		parts = append(parts, sec.Title, sec.LaTeX)
	}
	return strings.Join(parts, "\n")
}

type bridgeCitationResolver struct {
	client pb.QuantumBridgeServiceClient
}

func (r bridgeCitationResolver) Resolve(ctx context.Context, query string) ([]verify.ResolvedWork, error) {
	if r.client == nil {
		return nil, fmt.Errorf("literature bridge is not configured")
	}
	lookup, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := r.client.GroundLiterature(lookup, &pb.LiteratureGroundingRequest{
		Query: query, Limit: 5,
	})
	if err != nil {
		return nil, err
	}
	works := make([]verify.ResolvedWork, 0, len(resp.Records))
	for _, rec := range resp.Records {
		if rec.Id == "" {
			continue
		}
		works = append(works, verify.ResolvedWork{ID: rec.Id, Title: rec.Title})
	}
	return works, nil
}
