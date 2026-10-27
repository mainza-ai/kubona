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

// ChainDigest is the exported, single-source v1 digest used by import
// validation. It must stay byte-identical to the v1 writer and VerifyChain.
func ChainDigest(prevHash, sessionID, eventType, payload string) string {
	d := chainDigestBytes(prevHash, sessionID, eventType, payload)
	return hex.EncodeToString(d[:])
}

// canonicalPayload re-encodes a JSON payload deterministically (sorted keys,
// no insignificant whitespace). v2 hashes this, so an export/import JSON
// round-trip cannot invalidate the chain by normalising payload bytes.
func canonicalPayload(payload []byte) []byte {
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return payload
	}
	out, err := json.Marshal(v)
	if err != nil {
		return payload
	}
	return out
}

// chainDigestBytesV2 commits the metadata that v1 left outside the digest:
// tenant, confidentiality label, timestamp, and event type, plus a
// canonical-payload hash. A DB writer can no longer relabel or move an event
// without breaking the chain.
func chainDigestBytesV2(prevHash, sessionID, tenantID, confLabel, timestamp, eventType string, payload []byte) [32]byte {
	inner := sha256.Sum256(canonicalPayload(payload))
	return sha256.Sum256([]byte(fmt.Sprintf("kubona-ledger-v2|%s|%s|%s|%s|%s|%s|%x",
		prevHash, sessionID, tenantID, confLabel, timestamp, eventType, inner[:])))
}

// mandateSignatureMessage reconstructs the exact message an external
// mandate holder signed (mandates.SpendMandate.CanonicalPayload). External
// mandate events store enough fields to verify without trusting the chain.
func mandateSignatureMessage(payload []byte) ([]byte, error) {
	var p struct {
		TenantID        string  `json:"tenant_id"`
		JobHash         string  `json:"job_hash"`
		MaxBudgetUSD    float64 `json:"max_budget_usd"`
		ExpiryTimestamp string  `json:"expiry_timestamp"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	if p.TenantID == "" || p.JobHash == "" || p.ExpiryTimestamp == "" {
		return nil, fmt.Errorf("mandate payload missing tenant/job/expiry")
	}
	return []byte(fmt.Sprintf("%s:%s:%.4f:%s", p.TenantID, p.JobHash, p.MaxBudgetUSD, p.ExpiryTimestamp)), nil
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

	query := `SELECT id, timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex, hash_version, signature_domain FROM ledger_events WHERE id <= ? ORDER BY id`
	args := []any{maxID}
	if sessionID != "" {
		query = `SELECT id, timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex, hash_version, signature_domain FROM ledger_events WHERE id <= ? AND session_id = ? ORDER BY id`
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
		if err := rows.Scan(&id, &ev.Timestamp, &ev.SessionID, &ev.TenantID, &ev.ConfLabel,
			&ev.EventType, &payloadStr, &ev.PrevHash, &ev.EntryHash, &ev.SignerPubkey,
			&ev.Signature, &ev.HashVersion, &ev.SignatureDomain); err != nil {
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
		var digest [32]byte
		if ev.HashVersion >= 2 {
			digest = chainDigestBytesV2(ev.PrevHash, ev.SessionID, ev.TenantID, ev.ConfLabel,
				ev.Timestamp, ev.EventType, []byte(ev.Payload))
		} else {
			digest = chainDigestBytes(ev.PrevHash, ev.SessionID, ev.EventType, string(ev.Payload))
		}
		recomputed := hex.EncodeToString(digest[:])
		if recomputed != ev.EntryHash {
			faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
				Kind:   "hash",
				Detail: "entry_hash does not recompute from (prev|session|type|payload)"})
		}
		if ev.SignatureDomain == "external-opaque" {
			// Recorded proof whose signed message is not reconstructable.
			// Explicitly not claimed as verified; no chain signature fault.
		} else if ev.SignerPubkey != "" || ev.Signature != "" {
			pubBytes, err1 := hex.DecodeString(ev.SignerPubkey)
			sigBytes, err2 := hex.DecodeString(ev.Signature)
			switch {
			case err1 != nil || len(pubBytes) != ed25519.PublicKeySize:
				faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
					Kind: "signature", Detail: "unparseable signer public key"})
			case err2 != nil || len(sigBytes) != ed25519.SignatureSize:
				faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
					Kind: "signature", Detail: "unparseable signature"})
			case ev.SignatureDomain == "mandate-v1":
				msg, merr := mandateSignatureMessage([]byte(ev.Payload))
				if merr != nil || !ed25519.Verify(pubBytes, msg, sigBytes) {
					faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
						Kind: "signature", Detail: "external mandate signature does not verify"})
				}
			default:
				if !ed25519.Verify(pubBytes, digest[:], sigBytes) {
					faults = append(faults, ChainFault{EventID: id, Session: ev.SessionID,
						Kind: "signature", Detail: "signature does not verify"})
				}
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
