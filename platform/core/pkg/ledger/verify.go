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

// VerifyChain audits the ledger hash chain(s): per-session prev_hash linkage,
// entry-hash recomputation, and Ed25519 signatures where present. Unsigned
// events check linkage + hash only (they never claimed a signer).
//
// sessionID scopes the audit to one session; empty audits every session in a
// single ordered pass. A clean ledger returns no faults. This is the audit
// backbone for scaled deployments (read replicas, per-tenant shards): any
// copy of the ledger must verify before it is trusted.
func (l *Ledger) VerifyChain(sessionID string) ([]ChainFault, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	query := `SELECT id, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events ORDER BY id`
	args := []any{}
	if sessionID != "" {
		query = `SELECT id, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events WHERE session_id = ? ORDER BY id`
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
		dataToSign := fmt.Sprintf("%s|%s|%s|%s", ev.PrevHash, ev.SessionID, ev.EventType, string(ev.Payload))
		digest := sha256.Sum256([]byte(dataToSign))
		if hex.EncodeToString(digest[:]) != ev.EntryHash {
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
		}
		expectedPrev[ev.SessionID] = ev.EntryHash
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chain audit iteration failed: %w", err)
	}
	return faults, nil
}
