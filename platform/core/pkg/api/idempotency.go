package api

// S1 — content-bound idempotency rail for spend mutations.
//
// Threat (G1): a retried POST (timeout, double-click, SSE reconnect)
// double-executes spend. G7 stopped mis-attribution; this stops
// double-execution: every spend mutation accepts an optional `request_id`.
// Same (session, request_id, content-hash) replays the stored receipt
// byte-identical; same ID with different content 409s; ghosts 404 before
// any key is touched (callers must session-check first).
//
// Contract (Cloudroom pattern, independent implementation):
//   - empty request_id = no key, current behavior (console back-compat).
//   - malformed request_id = 400, nothing executed, nothing stored.
//   - new attempt = new request_id. A terminal receipt (success AND refusal)
//     replays forever; receipts are audit and are never deleted.
//   - crash between claim and complete leaves IN_PROGRESS: same-ID retry
//     waits bounded, then 409s `request_in_progress` (reconcile via list
//     endpoints, or mint a new ID for a new attempt). Stale claims
//     (crashed daemon) are reclaimable after rail-specific thresholds.
//
// All tunables live below — never a copied literal elsewhere.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Idempotency rails: fast (SubmitJob path, milliseconds) vs slow
// (tribunal / autoresearch start, minutes). Rails set the stale-claim and
// await windows; mis-railing a slow op as fast only mistimes duplicate
// waits, never correctness (the PK still serializes).
const (
	idemRailJob  = "job"
	idemRailSlow = "slow"

	idemStaleJob  = 5 * time.Minute
	idemStaleSlow = 60 * time.Minute
	idemAwaitJob  = 30 * time.Second
	idemAwaitSlow = 90 * time.Second
	idemPollEvery = 100 * time.Millisecond
)

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateRequestID enforces the request_id alphabet. Empty means "no key"
// (back-compat) and is valid; anything else must match.
func ValidateRequestID(id string) error {
	if id == "" {
		return nil
	}
	if !requestIDRe.MatchString(id) {
		return fmt.Errorf("invalid request_id %q: 1-64 chars of [A-Za-z0-9_-]", id)
	}
	return nil
}

// CanonicalContentHash (I1) hashes the spend-binding fields. encoding/json
// sorts map keys, so the digest is stable across key order and process
// restarts; RawMessage fields hash as their exact bytes (clients must reuse
// exact bytes for retries — documented on every keyed route).
func CanonicalContentHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("idempotency content not hashable: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// idemTicket is a claimed (or replayed) key. Handlers must route every exit
// through idemWriteJSON / idemWriteError so the receipt is sealed.
type idemTicket struct {
	active      bool // requestID != "" and claimed by this call
	sessionID   string
	requestID   string
	contentHash string
	rail        string
	replay      bool
	statusCode  int
	kind        string // "json" | "text"
	receipt     string // raw bytes (json) or http.Error message (text)
}

// idemRefusal is a terminal refusal before execution (400/404/409).
type idemRefusal struct {
	status int
	msg    string
}

func idemStaleAfter(rail string) time.Duration {
	if rail == idemRailSlow {
		return idemStaleSlow
	}
	return idemStaleJob
}

func idemAwaitFor(rail string) time.Duration {
	if rail == idemRailSlow {
		return idemAwaitSlow
	}
	return idemAwaitJob
}

func parseSQLiteTime(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// idemBegin validates the key and claims-or-replays it. Callers MUST have
// resolved the session first (ghosts 404 before any key row exists).
// Returns a ticket to complete, a replay ticket, or a refusal — exactly one
// non-nil.
func (s *Server) idemBegin(sessionID, requestID string, content any, rail string) (*idemTicket, *idemRefusal) {
	if requestID == "" {
		return &idemTicket{}, nil
	}
	if err := ValidateRequestID(requestID); err != nil {
		return nil, &idemRefusal{http.StatusBadRequest, err.Error()}
	}
	hash, err := CanonicalContentHash(content)
	if err != nil {
		return nil, &idemRefusal{http.StatusBadRequest, err.Error()}
	}
	if rail != idemRailSlow {
		rail = idemRailJob
	}
	for attempt := 0; attempt < 3; attempt++ {
		rec, found, err := s.ledger.IdemLookup(sessionID, requestID)
		if err != nil {
			return nil, &idemRefusal{http.StatusInternalServerError, fmt.Sprintf("idempotency lookup failed: %v", err)}
		}
		if !found {
			if cerr := s.ledger.IdemClaim(sessionID, requestID, hash, rail); cerr != nil {
				if cerr.Error() == "idempotency key already claimed" {
					continue // lost the claim race: re-read
				}
				return nil, &idemRefusal{http.StatusInternalServerError, fmt.Sprintf("idempotency claim failed: %v", cerr)}
			}
			return &idemTicket{active: true, sessionID: sessionID, requestID: requestID, contentHash: hash, rail: rail}, nil
		}
		if rec.Status == ledger.IdemStatusComplete {
			if rec.ContentHash != hash {
				return nil, &idemRefusal{http.StatusConflict, fmt.Sprintf("request_conflict: request_id %q was already used with different content (stored hash %s): retry with identical bytes or mint a new request_id", requestID, rec.ContentHash)}
			}
			if rec.ReceiptKind != "json" && rec.ReceiptKind != "text" {
				return nil, &idemRefusal{http.StatusInternalServerError, "stored idempotency receipt unreadable: reconcile via list endpoints"}
			}
			return &idemTicket{active: false, replay: true, statusCode: rec.StatusCode, kind: rec.ReceiptKind, receipt: rec.ReceiptBody}, nil
		}
		// IN_PROGRESS: different content can never share the receipt.
		if rec.ContentHash != hash {
			return nil, &idemRefusal{http.StatusConflict, fmt.Sprintf("request_conflict: request_id %q is already claimed with different content: mint a new request_id", requestID)}
		}
		// Same content, still running (or orphaned by a crash): reclaim
		// stale claims, else wait bounded for the leader, else 409.
		if created, ok := parseSQLiteTime(rec.CreatedAt); ok && time.Since(created) > idemStaleAfter(rec.Rail) {
			_ = s.ledger.IdemDelete(sessionID, requestID)
			continue // re-claim as new leader
		}
		if ticket, refusal := s.idemAwait(sessionID, requestID, hash); ticket != nil || refusal != nil {
			return ticket, refusal
		}
		return nil, &idemRefusal{http.StatusConflict, fmt.Sprintf("request_in_progress: request_id %q is still executing: retry the same ID later for its receipt, or mint a new request_id for a new attempt", requestID)}
	}
	return nil, &idemRefusal{http.StatusInternalServerError, "idempotency claim race did not settle: retry with a new request_id"}
}

// idemAwait polls a running claim for completion. Returns a replay ticket on
// success, a conflict on hash drift (shouldn't happen — defensive), or
// (nil, nil) on timeout so the caller 409s request_in_progress.
func (s *Server) idemAwait(sessionID, requestID, hash string) (*idemTicket, *idemRefusal) {
	deadline := time.Now().Add(idemAwaitFor(idemRailJob))
	// Slow-rail claims take minutes; the rail is on the stored row.
	if rec, found, err := s.ledger.IdemLookup(sessionID, requestID); err == nil && found && rec.Rail == idemRailSlow {
		deadline = time.Now().Add(idemAwaitSlow)
	}
	for time.Now().Before(deadline) {
		time.Sleep(idemPollEvery)
		rec, found, err := s.ledger.IdemLookup(sessionID, requestID)
		if err != nil || !found {
			continue
		}
		if rec.Status != ledger.IdemStatusComplete {
			continue
		}
		if rec.ContentHash != hash {
			return nil, &idemRefusal{http.StatusConflict, fmt.Sprintf("request_conflict: request_id %q completed with different content", requestID)}
		}
		return &idemTicket{active: false, replay: true, statusCode: rec.StatusCode, kind: rec.ReceiptKind, receipt: rec.ReceiptBody}, nil
	}
	return nil, nil
}

// idemReplay serves a stored receipt byte-identical to the original.
func (s *Server) idemReplay(w http.ResponseWriter, t *idemTicket) {
	w.Header().Set("X-Idempotent-Replay", "true")
	if t.kind == "text" {
		http.Error(w, t.receipt, t.statusCode)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(t.statusCode)
	_, _ = w.Write([]byte(t.receipt))
}

// idemFinish seals a terminal outcome then serves it. Marshal happens ONCE:
// the stored bytes are the served bytes.
func (s *Server) idemFinishJSON(w http.ResponseWriter, t *idemTicket, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		// Execution already happened but the receipt is unsealable: unwind
		// the claim so the client can reconcile (list endpoints) and retry
		// cleanly, instead of wedging on IN_PROGRESS.
		if t.active {
			_ = s.ledger.IdemDelete(t.sessionID, t.requestID)
		}
		http.Error(w, fmt.Sprintf("response could not be sealed; execution may have completed — reconcile via list endpoints before retrying with a new request_id: %v", err), http.StatusInternalServerError)
		return
	}
	if t.active {
		if ok, cerr := s.ledger.IdemComplete(t.sessionID, t.requestID, status, "json", string(raw)); cerr != nil || !ok {
			// Seal failure (or reclaim orphan): the spend itself is already
			// attested (job_queued / reservation rows exist); serve live and
			// record the gap so retry semantics stay honest: same-ID retry
			// 409s in_progress until reclaim instead of double-executing.
			_ = s.ledger.AppendEvent(t.sessionID, "idempotency_complete_orphaned", map[string]any{
				"request_id": t.requestID, "rail": t.rail,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

// idemFinishError seals a terminal refusal/error (http.Error format) then
// serves it. Deterministic refusals replay like successes: a new attempt
// needs a new request_id.
func (s *Server) idemFinishError(w http.ResponseWriter, t *idemTicket, status int, msg string) {
	if t.active {
		if ok, cerr := s.ledger.IdemComplete(t.sessionID, t.requestID, status, "text", msg); cerr != nil || !ok {
			_ = s.ledger.AppendEvent(t.sessionID, "idempotency_complete_orphaned", map[string]any{
				"request_id": t.requestID, "rail": t.rail,
			})
		}
	}
	http.Error(w, msg, status)
}
