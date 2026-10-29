package api

// Mandate-review queue (steady-state ops surface). Open obligations without
// settlement ARE the queue: this surface lists them with close-readiness
// (AttemptClose reasons, read-only) and triggers closure evaluation.
// Closure itself is rule-bound (AttemptClose); this endpoint only decides
// WHO may trigger it, never WHETHER it closes.

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// obligationCloseCanonical is the exact bytes an owner key signs to
// authorize a close trigger: "obligation-close\n" + obligation id.
// Bearer transport + owner-key signature (no session needed); the
// signature is the authority, the bearer is the transport gate.
func obligationCloseCanonical(id string) []byte {
	return []byte("obligation-close\n" + id)
}

// obligationRow renders one obligation with its close-readiness for the
// review queue. Cases carry kinds/verdicts/authorities (evidence IDs, not
// payloads — refs, never flooded).
func obligationRow(s *Server, ob *ledger.Obligation) (map[string]any, error) {
	cases, err := s.ledger.ListObligationCases(ob.ID)
	if err != nil {
		return nil, err
	}
	rendered := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		rendered = append(rendered, map[string]any{
			"kind": c.CaseKind, "verdict": c.Verdict,
			"authority": c.Authority, "evidence": c.Evidence,
		})
	}
	ready, reasons, err := agents.PreviewClose(s.ledger, ob.ID)
	if err != nil {
		return nil, err
	}
	unknownIDs := make([]string, 0, len(ob.Unknowns))
	for _, u := range ob.Unknowns {
		unknownIDs = append(unknownIDs, u.ID)
	}
	return map[string]any{
		"id": ob.ID, "subject_kind": ob.SubjectKind, "subject_id": ob.SubjectID,
		"owner": ob.Owner, "disposition": ob.Disposition,
		"required_cases": ob.RequiredCases, "required_authorities": ob.RequiredAuthorities,
		"cases": rendered, "unknowns": unknownIDs,
		"close_ready": ready, "close_reasons": reasons,
	}, nil
}

// handleObligationsOpen lists open obligations for review, oldest first.
// Auth (D-09 shaped): session-cookie callers see only rows owned by their
// lab's sessions (owner "session:"+id whose session tenant matches); bearer
// sees all with optional ?owner= ?subject_kind= filters. Anonymous: single-lab
// store only, else 401. Ghosts never 404 here — the list only returns allowed
// rows, so there is no oracle to protect.
func (s *Server) handleObligationsOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "queue is GET-only", nil)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK {
		WriteAPIError(w, "unauthorized", "log in or present the operator token to read the queue", nil)
		return
	}
	q := r.URL.Query()
	ownerFilter, kindFilter := q.Get("owner"), q.Get("subject_kind")
	limit := 100
	obs, err := s.ledger.ListOpenObligations(limit, "", "")
	if err != nil {
		WriteAPIError(w, "internal", "queue read failed", nil)
		return
	}
	rows := make([]map[string]any, 0, len(obs))
	for _, ob := range obs {
		if tenant != "" {
			// Lab-scoped: only rows owned by this lab's sessions.
			// Key-owned obligations (admin/reviewer pubkeys) are invisible
			// to lab sessions — they surface to bearer operators.
			sid, ok := strings.CutPrefix(ob.Owner, "session:")
			if !ok || sid == "" {
				continue
			}
			sess, err := s.sessionMgr.GetSession(sid)
			if err != nil || sess == nil || sess.TenantID != tenant {
				continue
			}
		}
		if ownerFilter != "" && ob.Owner != ownerFilter {
			continue
		}
		if kindFilter != "" && ob.SubjectKind != kindFilter {
			continue
		}
		row, err := obligationRow(s, ob)
		if err != nil {
			WriteAPIError(w, "internal", "queue read failed", nil)
			return
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "obligations": rows, "count": len(rows)})
}

// handleObligationClose triggers AttemptClose on one obligation. It decides
// WHO may trigger, never WHETHER it closes (AttemptClose decides that).
// Authority (either):
//  1. session path: cookie caller whose lab owns the owner session
//     (owner "session:"+id with matching tenant — lab trust model);
//  2. signature path: Ed25519 over obligationCloseCanonical(id) by the
//     owner KEY itself (admin/reviewer pubkey owners) — bearer-gated
//     transport, signature authority.
//
// Bearer alone is NEVER sufficient (no god-mode mutations). Unknown or
// unowned ids 403 without distinguishing (no existence oracle).
func (s *Server) handleObligationClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "close is POST-only", nil)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK {
		WriteAPIError(w, "unauthorized", "log in or present the operator token", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/obligations/")
	if id == "" || strings.Contains(id, "/") {
		WriteAPIError(w, "bad_request", "close path is /api/obligations/{id}", nil)
		return
	}
	ob, err := s.ledger.GetObligation(id)
	if err != nil || ob == nil {
		// No oracle: unknown ids refuse exactly like unowned ones.
		WriteAPIError(w, "forbidden", "not your obligation to close: refused", nil)
		return
	}
	authorized := false
	if tenant != "" {
		// Session path: caller lab must own the owner session.
		if sid, ok := strings.CutPrefix(ob.Owner, "session:"); ok && sid != "" {
			if sess, err := s.sessionMgr.GetSession(sid); err == nil && sess != nil && sess.TenantID == tenant {
				authorized = true
			}
		}
	}
	if !authorized && tenant == "" {
		// Signature path: bearer transport + owner-key signature. Body
		// carries the hex signature; the owner key IS the authority.
		var req struct {
			SignatureHex string `json:"signature_hex"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
			return
		}
		if req.SignatureHex != "" {
			if pub, err := hex.DecodeString(strings.TrimSpace(ob.Owner)); err == nil && len(pub) == ed25519.PublicKeySize {
				if sig, err := hex.DecodeString(strings.TrimSpace(req.SignatureHex)); err == nil && len(sig) == ed25519.SignatureSize {
					authorized = ed25519.Verify(pub, obligationCloseCanonical(id), sig)
				}
			}
		}
	}
	if !authorized {
		WriteAPIError(w, "forbidden", "not your obligation to close: refused", nil)
		return
	}
	if ob.Disposition == "closed" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "closed": true, "already_closed": true})
		return
	}
	closed, reasons, err := agents.AttemptClose(s.ledger, id)
	if err != nil {
		WriteAPIError(w, "internal", "close evaluation failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "closed": closed, "reasons": reasons})
}
