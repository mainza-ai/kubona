package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func discoveryClaimsSubpath(id string) (sessionID, rest string, ok bool) {
	const marker = "/discovery-claims"
	idx := strings.Index(id, marker)
	if idx <= 0 {
		return "", "", false
	}
	after := id[idx+len(marker):]
	if after != "" && !strings.HasPrefix(after, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(after, "/"), true
}

// handleDiscoveryClaimsRoute exposes durable Schema-17 QEC discovery_claims
// lifecycle operations:
//
//	GET  /api/sessions/{id}/discovery-claims
//	POST /api/sessions/{id}/discovery-claims               (create/upsert candidate)
//	POST /api/sessions/{id}/discovery-claims/{claimId}/transition  (verified / Ed25519 claimed / retracted)
func (s *Server) handleDiscoveryClaimsRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
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
			claims, err := s.ledger.ListDiscoveryClaims(sessionID)
			if err != nil {
				WriteAPIError(w, "internal", "failed to list discovery claims", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"session_id": sessionID,
				"claims":     claims,
			})
			return

		case http.MethodPost:
			var rec qec.DiscoveryClaimRecord
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&rec); err != nil {
				WriteAPIError(w, "bad_request", "invalid discovery claim JSON", nil)
				return
			}
			rec.SessionID = sessionID
			if rec.Status == "" {
				rec.Status = qec.ClaimCandidate
			}
			// Never allow bypassing lifecycle on creation: new claims start as candidate.
			if rec.Status != qec.ClaimCandidate {
				WriteAPIError(w, "bad_request", "new discovery claims must enter in candidate status", nil)
				return
			}
			if err := s.ledger.SaveDiscoveryClaim(rec); err != nil {
				WriteAPIError(w, "bad_request", err.Error(), nil)
				return
			}
			saved, err := s.ledger.GetDiscoveryClaim(rec.ClaimID)
			if err != nil {
				WriteAPIError(w, "internal", "failed to reload saved discovery claim", nil)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"claim": saved})
			return
		default:
			WriteAPIError(w, "method_not_allowed", "GET or POST required", nil)
			return
		}
	}

	// POST /api/sessions/{id}/discovery-claims/{claimId}/transition
	if strings.HasSuffix(rest, "/transition") {
		if r.Method != http.MethodPost {
			WriteAPIError(w, "method_not_allowed", "POST required", nil)
			return
		}
		claimID := strings.TrimSuffix(rest, "/transition")
		var req struct {
			Status               string `json:"status"`
			OperatorPubkeyHex    string `json:"operator_pubkey_hex"`
			OperatorSignatureHex string `json:"operator_signature_hex"`
			RetractionReason     string `json:"retraction_reason"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			WriteAPIError(w, "bad_request", "invalid transition payload", nil)
			return
		}
		updated, err := s.ledger.TransitionAndPersistDiscoveryClaim(
			claimID,
			qec.DiscoveryClaimStatus(strings.TrimSpace(req.Status)),
			req.OperatorPubkeyHex,
			req.OperatorSignatureHex,
			req.RetractionReason,
		)
		if err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"claim": updated})
		return
	}

	WriteAPIError(w, "not_found", "unknown discovery-claims subpath", nil)
}
