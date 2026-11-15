package api

import (
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// handleResolveOutcome is the signed exit from OUTCOME_UNKNOWN. A session
// cookie is not enough: the money-releasing outcome requires the admin
// signature over these parameters. Stolen cookies cannot mint it.
func (s *Server) handleResolveOutcome(w http.ResponseWriter, r *http.Request, jobID string) {
	var req struct {
		Outcome   string              `json:"outcome"`
		Evidence  string              `json:"evidence"`
		VendorRef string              `json:"vendor_ref"`
		ActualUSD *float64            `json:"actual_usd"`
		Action    tenants.AdminAction `json:"action"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", "invalid request body: "+err.Error(), nil)
		return
	}
	if req.Action.SignatureHex == "" || req.Action.PubkeyHex == "" {
		WriteAPIError(w, "forbidden", "admin signature is required to resolve an unknown outcome", nil)
		return
	}
	job, err := s.jobRunner.GetJob(jobID)
	if err != nil || job == nil {
		WriteAPIError(w, "not_found", "job not found", nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, job.SessionID); !ok {
		return
	}
	sess, err := s.sessionMgr.GetSession(job.SessionID)
	if err != nil || sess == nil {
		WriteAPIError(w, "not_found", "job session not found", nil)
		return
	}
	params := map[string]any{
		"job_id": jobID, "outcome": req.Outcome, "evidence": req.Evidence,
		"vendor_ref": req.VendorRef, "actual_usd": req.ActualUSD,
	}
	bound, err := bindAdminAction(sess.TenantID, "resolve_job_outcome", params, req.Action)
	if err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	if _, err := s.tenantMgr.VerifyAdminAction(bound); err != nil {
		WriteAPIError(w, "forbidden", "outcome resolution signature refused: "+err.Error(), nil)
		return
	}
	if err := s.jobRunner.ResolveOutcomeUnknown(jobID, req.Outcome, req.Evidence, req.VendorRef, req.ActualUSD); err != nil {
		code := "bad_request"
		if strings.Contains(err.Error(), "not outcome-unknown") {
			code = "request_conflict"
		}
		WriteAPIError(w, code, err.Error(), nil)
		return
	}
	updated, _ := s.jobRunner.GetJob(jobID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "resolved", "job": jobViewFrom(updated)})
}
