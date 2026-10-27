package api

import (
	"net/http"
)

// handleLedgerVerify exposes the chain audit as a production read surface.
// An optional session_id scopes it; otherwise every session is verified.
// The response is a report, not a mutation: ok is false when any fault is
// found, so an operator or CI can gate on it.
func (s *Server) handleLedgerVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	faults, err := s.ledger.VerifyChain(sessionID)
	if err != nil {
		WriteAPIError(w, "ledger_verify_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          len(faults) == 0,
		"session_id":  sessionID,
		"fault_count": len(faults),
		"faults":      faults,
	})
}
