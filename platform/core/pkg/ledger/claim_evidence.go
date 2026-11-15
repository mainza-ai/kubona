package ledger

// B4 — claim provenance as rows. "Verified" is only as strong as the measured
// evidence it points to; the summary here is what the API and the review gate
// read, so a verdict can never carry more authority than its links.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Closed evidence kinds. Only job_result and classical_anchor are measured
// records; general_knowledge is an explicit, named assertion — not a bypass.
var claimEvidenceKinds = map[string]bool{
	"job_result":        true,
	"classical_anchor":  true,
	"literature":        true,
	"tribunal_turn":     true,
	"general_knowledge": true,
}

type ClaimEvidence struct {
	ClaimID          string `json:"claim_id"`
	Kind             string `json:"kind"`
	JobID            string `json:"job_id,omitempty"`
	ResultSHA256     string `json:"result_sha256,omitempty"`
	ReceiptEntryHash string `json:"receipt_entry_hash,omitempty"`
	SourceRef        string `json:"source_ref,omitempty"`
	CreatedAt        string `json:"created_at"`
}

// ResultDigest is the canonical digest for a measured result blob. One helper
// so every producer hashes the same bytes the same way.
func ResultDigest(result []byte) string {
	sum := sha256.Sum256(result)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AddClaimEvidence links one evidence row to a claim. The claim must exist in
// the session; kind and the fields the kind requires are enforced. Repeat
// inserts are idempotent (primary key).
func (l *Ledger) AddClaimEvidence(sessionID, claimID string, ev ClaimEvidence) error {
	ev.ClaimID = strings.TrimSpace(claimID)
	ev.Kind = strings.TrimSpace(ev.Kind)
	if ev.ClaimID == "" {
		return fmt.Errorf("claim evidence needs a claim id")
	}
	if !claimEvidenceKinds[ev.Kind] {
		return fmt.Errorf("unknown claim evidence kind %q", ev.Kind)
	}
	switch ev.Kind {
	case "job_result", "classical_anchor":
		if ev.JobID == "" || ev.ResultSHA256 == "" {
			return fmt.Errorf("%s evidence needs job_id and result_sha256", ev.Kind)
		}
	case "literature", "tribunal_turn", "general_knowledge":
		if ev.SourceRef == "" {
			return fmt.Errorf("%s evidence needs source_ref", ev.Kind)
		}
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	if sessionID != "" {
		var exists int
		if err := l.db.QueryRow(`SELECT COUNT(*) FROM claim_records WHERE id = ? AND session_id = ?`, ev.ClaimID, sessionID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("claim %s not found in session %s", ev.ClaimID, shortID(sessionID))
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := l.db.Exec(`INSERT OR IGNORE INTO claim_evidence
		(claim_id, kind, job_id, result_sha256, receipt_entry_hash, source_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ClaimID, ev.Kind, ev.JobID, ev.ResultSHA256, ev.ReceiptEntryHash, ev.SourceRef, now)
	if err != nil {
		return err
	}
	ev.CreatedAt = now
	return nil
}

// GetClaimRecord returns one claim scoped to its session (nil, nil when absent).
func (l *Ledger) GetClaimRecord(sessionID, claimID string) (*ClaimRecord, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	var rec ClaimRecord
	err := l.db.QueryRow(`SELECT id, session_id, turn_kind, statement, alpha, beta, gamma, verdict, evidence_json, created_at
		FROM claim_records WHERE id = ? AND session_id = ?`, claimID, sessionID).
		Scan(&rec.ID, &rec.SessionID, &rec.TurnKind, &rec.Statement, &rec.Alpha, &rec.Beta, &rec.Gamma, &rec.Verdict, &rec.Evidence, &rec.CreatedAt)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// ListClaimEvidence returns one claim's evidence rows, stable by kind then id.
func (l *Ledger) ListClaimEvidence(claimID string) ([]ClaimEvidence, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	rows, err := l.db.Query(`SELECT claim_id, kind, job_id, result_sha256, receipt_entry_hash, source_ref, created_at
		FROM claim_evidence WHERE claim_id = ? ORDER BY kind, job_id, source_ref`, claimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ClaimEvidence, 0, 4)
	for rows.Next() {
		var ev ClaimEvidence
		if err := rows.Scan(&ev.ClaimID, &ev.Kind, &ev.JobID, &ev.ResultSHA256, &ev.ReceiptEntryHash, &ev.SourceRef, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ClaimProvenanceSummary is the rule read, not a score.
type ClaimProvenanceSummary struct {
	Measured         int  `json:"measured"`
	GeneralKnowledge bool `json:"general_knowledge"`
	// Complete: a verified claim has at least one measured record or an
	// explicit general-knowledge assertion. Non-verified verdicts are not
	// graded by this rule (contested is allowed to be incomplete).
	Complete bool `json:"complete"`
}

// ClaimProvenance returns the claim, its evidence rows, and the summary.
func (l *Ledger) ClaimProvenance(sessionID, claimID string) (*ClaimRecord, []ClaimEvidence, ClaimProvenanceSummary, error) {
	claim, err := l.GetClaimRecord(sessionID, claimID)
	if err != nil || claim == nil {
		return claim, nil, ClaimProvenanceSummary{}, err
	}
	evidence, err := l.ListClaimEvidence(claimID)
	if err != nil {
		return nil, nil, ClaimProvenanceSummary{}, err
	}
	var summary ClaimProvenanceSummary
	for _, ev := range evidence {
		switch ev.Kind {
		case "job_result", "classical_anchor":
			summary.Measured++
		case "general_knowledge":
			summary.GeneralKnowledge = true
		}
	}
	summary.Complete = claim.Verdict != "verified" || summary.Measured > 0 || summary.GeneralKnowledge
	return claim, evidence, summary, nil
}
