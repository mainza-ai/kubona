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
// W2.1 threat model: writes get edge auth — unauthenticated forging of
// tribunal_commit/peer_review refuses (401/503) before any allowlist check.
// Reads (export below) stay public behind limiters; this append never serves
// anonymous callers. Bounded (64KB) and rate-limited (human-paced bucket).
func (s *Server) handleLedgerAppend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.ledgerLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	// 64KB payload cap: one client must not stuff an unbounded blob into
	// the hash-chained ledger.
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

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
// W2.1 threat model: the abort rail is an availability kill-switch (a write
// that destroys queued work) — operator bearer auth + its own limiter.
// Unauthenticated abort refuses before any session is touched.
func (s *Server) handleSupervisorAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.abortLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	// 64KB payload cap: abort reasons are short strings, not blobs.
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

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
	// T-09b: known session — bind caller to session tenant, in addition
	// to the record-label check below (which binds records to the
	// session, not the caller to the session).
	if !s.authorizeSessionRead(w, r, sess) {
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
