package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ifc"
)

// tribunalLedgerEventTypes is the allowlist of console-committable ledger
// events. The ledger is append-only and hash-chained: arbitrary client-chosen
// event types would let any UI forge provenance vocabulary, so only tribunal
// and review outcomes may be committed through this route.
var tribunalLedgerEventTypes = map[string]bool{
	"tribunal_commit": true,
	"peer_review":     true,
	"annotation":      true,
}

// LedgerAppendAllowlist exposes the route allowlist for the taxonomy
// conformance test (S6): the allowlist must stay a subset of the ledger
// registry, or the two lists rot apart silently.
func LedgerAppendAllowlist() []string {
	out := make([]string, 0, len(tribunalLedgerEventTypes))
	for et := range tribunalLedgerEventTypes {
		out = append(out, et)
	}
	return out
}

// handleLedgerAppend seals a console-side outcome (tribunal verdict, review,
// annotation) into the hash-chained ledger and returns the server-issued
// record hash. The receipt is computed by the ledger — never by the client.
func (s *Server) handleLedgerAppend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string         `json:"session_id"`
		EventType string         `json:"event_type"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if !tribunalLedgerEventTypes[req.EventType] {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"ok":    false,
			"error": "event_type not committable through this route",
		})
		return
	}

	rec, err := s.ledger.AppendSignedEvent(req.SessionID, req.EventType, req.Payload, s.signerPrivKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                      true,
		"record_hash":             rec.EntryHash,
		"event_id":                rec.ID,
		"prev_hash":               rec.PrevHash,
		"reviewer_key_configured": s.reviewerKeyConfigured,
	})
}

// handleSupervisorAbort executes a Ring-0 reflex intercept for a session: all
// unsettled jobs are settled failed with the abort reason, in-flight workers
// are fenced via the runner's abort registry, and the abort itself is sealed
// in the ledger. The response carries the aborted count and the abort receipt.
func (s *Server) handleSupervisorAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		req.Reason = "emergency abort from console"
	}

	count, err := s.jobRunner.CancelSessionJobs(req.SessionID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events, err := s.ledger.GetRecentEvents(req.SessionID, 1)
	if err != nil || len(events) == 0 {
		http.Error(w, "abort executed but receipt unreadable", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"aborted_count": count,
		"abort_receipt": events[0].EntryHash,
		"session_id":    req.SessionID,
	})
}

// handleLedgerExport renders a session's ledger selection for download
// behind the confidentiality gate. Export refuses the whole selection when
// any record's label is unreadable by the session's lab tenant — partial
// exports would leak record existence (a covert channel), and per-record
// redaction would still confirm what was withheld. Foreign-tenant records
// have no grant model to honor yet, so they refuse outright.
func (s *Server) handleLedgerExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Format    string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil {
		http.Error(w, "unknown session: export refused", http.StatusForbidden)
		return
	}
	if req.Format != "json" && req.Format != "markdown" {
		http.Error(w, "format must be json or markdown", http.StatusBadRequest)
		return
	}

	events, err := s.ledger.GetRecentEvents(req.SessionID, 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, e := range events {
		if !ifc.LabelReadableBy(e.ConfLabel, sess.TenantID) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"ok":    false,
				"error": fmt.Sprintf("selection contains records labeled %q: export refused (no cross-tenant grants)", e.ConfLabel),
			})
			return
		}
	}

	if req.Format == "json" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session_id": req.SessionID, "events": events})
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Ledger export — session %s (tenant %s)\n\n", sess.ID, sess.TenantID))
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("## %s — %s\n- hash: `%s`\n- label: `%s`\n\n",
			e.EventType, e.Timestamp, e.EntryHash, e.ConfLabel))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session_id": req.SessionID, "markdown": sb.String()})
}
