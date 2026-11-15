package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// handleOperationStatus exposes the idempotency receipt for one mutation.
// It is a read-only reconciliation path: the client can prove applied/refused
// after a timeout instead of guessing or replaying blindly.
func (s *Server) handleOperationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "operation status is GET-only", nil)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	requestID := strings.TrimPrefix(r.URL.Path, "/api/operations/")
	if err := ValidateRequestID(requestID); err != nil || requestID == "" {
		WriteAPIError(w, "bad_request", "operation id must be 1-64 chars of [A-Za-z0-9_-]", nil)
		return
	}
	rec, found, err := s.ledger.IdemLookup(sessionID, requestID)
	if err != nil {
		WriteAPIError(w, "internal", "operation lookup failed", nil)
		return
	}
	if !found {
		WriteAPIError(w, "not_found", "no operation receipt for this request id", nil)
		return
	}
	status := "in_progress"
	if rec.Status == ledger.IdemStatusComplete {
		status = "complete"
	}
	disposition, reason := operationDisposition(rec)
	out := map[string]any{
		"request_id":  rec.RequestID,
		"status":      status,
		"status_code": rec.StatusCode,
		"rail":        rec.Rail,
		"updated_at":  rec.UpdatedAt,
		"disposition": disposition,
		"reason":      reason,
	}
	if rec.Status == ledger.IdemStatusComplete && len(rec.ReceiptBody) <= 64*1024 {
		if rec.ReceiptKind == "json" {
			out["receipt"] = json.RawMessage(rec.ReceiptBody)
		} else {
			out["receipt"] = rec.ReceiptBody
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// operationDisposition is the closed receipt vocabulary. admitted is not
// applied. outcome_unknown is not a refusal and not a success.
func operationDisposition(rec ledger.IdempotencyRecord) (string, string) {
	if rec.Status != ledger.IdemStatusComplete {
		return "admitted", "queued"
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(rec.ReceiptBody), &body)
	if unchanged, _ := body["unchanged"].(bool); unchanged {
		return "no_op", "unchanged"
	}
	if status, _ := body["status"].(string); status == "JOB_STATUS_OUTCOME_UNKNOWN" {
		return "outcome_unknown", "vendor_unconfirmed"
	}
	if job, ok := body["job"].(map[string]any); ok {
		if st, _ := job["status"].(string); st == "outcome_unknown" || st == "JOB_STATUS_OUTCOME_UNKNOWN" {
			return "outcome_unknown", "vendor_unconfirmed"
		}
	}
	if rec.StatusCode >= 400 {
		return "refused", "handler_refused"
	}
	return "applied", "completed"
}
