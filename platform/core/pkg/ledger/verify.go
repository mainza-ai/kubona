package ledger

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ChainFault is one broken link or signature in the hash chain.
type ChainFault struct {
	EventID int64  `json:"event_id"`
	Session string `json:"session_id"`
	Kind    string `json:"kind"` // "link" | "hash" | "signature"
	Detail  string `json:"detail"`
}

const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// CommitEventTypes must carry valid Ed25519 signatures (W1.12): these
// events authorize spend, membership, revocation, deletion, and verdicts.
// An unsigned commit-class event verifies linkage+hash only — meaning a
// history rewrite with recomputed hashes verifies clean. The audit faults
// them so unsigned authority can never pass as verified history.
var CommitEventTypes = map[string]bool{
	"tribunal_commit": true, "peer_review": true,
	"mandate_verified_client": true, "mandate_verified_pqc_mlkem768": true,
	"grant_issued": true, "grant_revoked": true,
	"session_deleted": true,
}

// chainDigestBytes is the single byte-encoding both writers
// (AppendSignedEvent/AppendExternalEvent) and this audit use (W1.12): the
// signer and verifier cannot rot apart. Format v1 ("|" joined) is kept for
// chain continuity — every existing entry_hash was computed this way, so
// migrating formats would fault the whole history. Known limitation: "|"
// inside a payload is ambiguous in theory; exploitation still requires a
// sha256 second preimage (infeasible), and session/event-type fields come
// from constrained vocabularies in practice.
func chainDigestBytes(prevHash, sessionID, eventType, payload string) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", prevHash, sessionID, eventType, payload)))
}

// ChainDigest is the exported, single-source digest used by import
// validation. It must stay byte-identical to the writer and VerifyChain.
func ChainDigest(prevHash, sessionID, eventType, payload string) string {
	d := chainDigestBytes(prevHash, sessionID, eventType, payload)
	return hex.EncodeToString(d[:])
}

// VerifyChain audits the ledger hash chain(s): per-session prev_hash linkage,
// entry-hash recomputation, Ed25519 signatures where present, and signature
// presence on commit-class events. Unsigned non-commit events check linkage +
// hash only (they never claimed a signer).
//
// sessionID scopes the audit to one session; empty audits every session in a
// single ordered pass. A clean ledger returns no faults. This is the audit
// backbone for scaled deployments (read replicas, per-tenant shards): any
// copy of the ledger must verify before it is trusted.
//
// W1.12 snapshot discipline: the audit snapshots the max row id first and
// never holds the ledger lock during the scan (the old code held RLock for
// the full table, blocking writers for the whole audit).
func (l *Ledger) VerifyChain(sessionID string) ([]ChainFault, error) {
	var maxID int64
	row := l.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM ledger_events`)
	if err := row.Scan(&maxID); err != nil {
		return nil, fmt.Errorf("chain audit snapshot failed: %w", err)
	}

	query := `SELECT id, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events WHERE id <= ? ORDER BY id`
	args := []any{maxID}
	if sessionID != "" {
		query = `SELECT id, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events WHERE id <= ? AND session_id = ? ORDER BY id`
		args = append(args, sessionID)
	}
	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("chain audit query failed: %w", err)
	}
	defer rows.Close()

	expectedPrev := map[string]string{}
	var faults []ChainFault
	for rows.Next() {
		var id int64
		var ev EventRecord
		var payloadStr string
		if err := rows.Scan(&id, &ev.SessionID, &ev.EventType, &payloadStr,
			&ev.PrevHash, &ev.EntryHash, &ev.SignerPubkey, &ev.Signature); err != nil {
			return nil, fmt.Errorf("chain audit scan failed: %w", err)
		}
		ev.ID = id
		ev.Payload = json.RawMessage(payloadStr)
		want, seen := expectedPrev[ev.SessionID]
		if !seen {
			want = genesisHash
		}
		if ev.PrevHash != want {
			faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
				Kind:   "link",
				Detail: fmt.Sprintf("prev_hash %q breaks chain (expected %q)", ev.PrevHash, want)})
		}
		digest := chainDigestBytes(ev.PrevHash, ev.SessionID, ev.EventType, string(ev.Payload))
		recomputed := ChainDigest(ev.PrevHash, ev.SessionID, ev.EventType, string(ev.Payload))
		if recomputed != ev.EntryHash {
			faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
				Kind:   "hash",
				Detail: "entry_hash does not recompute from (prev|session|type|payload)"})
		}
		if ev.SignerPubkey != "" || ev.Signature != "" {
			pubBytes, err1 := hex.DecodeString(ev.SignerPubkey)
			sigBytes, err2 := hex.DecodeString(ev.Signature)
			switch {
			case err1 != nil || len(pubBytes) != ed25519.PublicKeySize:
				faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
					Kind: "signature", Detail: "unparseable signer public key"})
			case err2 != nil || len(sigBytes) != ed25519.SignatureSize:
				faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
					Kind: "signature", Detail: "unparseable signature"})
			case !ed25519.Verify(pubBytes, digest[:], sigBytes):
				faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
					Kind: "signature", Detail: "signature does not verify"})
			}
		} else if CommitEventTypes[ev.EventType] {
			faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
				Kind:   "signature",
				Detail: fmt.Sprintf("commit-class event %q is unsigned: rewrite-forgeable, never verified history", ev.EventType)})
		}
		// W1.12: re-anchor on the RECOMPUTED digest, never the reported
		// hash. Chaining from the reported hash let a forged link become
		// the new anchor, masking the break's extent downstream.
		expectedPrev[ev.SessionID] = recomputed
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chain audit iteration failed: %w", err)
	}
	return faults, nil
}
