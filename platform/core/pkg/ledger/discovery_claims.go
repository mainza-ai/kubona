package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/qec"
)

// Schema 17: durable, Ed25519-signed QEC discovery claim lifecycle ledger (P0-3).
const discoveryClaimsSchema = `
	CREATE TABLE IF NOT EXISTS discovery_claims (
		claim_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL DEFAULT '',
		code_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'candidate',
		evidence_tier TEXT NOT NULL DEFAULT 'T3_SAMPLED',
		credibility_tier TEXT NOT NULL DEFAULT 'L2',
		artifact_sha256 TEXT NOT NULL DEFAULT '',
		gates_digest TEXT NOT NULL DEFAULT '',
		gates_0_to_6_passed INTEGER NOT NULL DEFAULT 0,
		operator_pubkey_hex TEXT NOT NULL DEFAULT '',
		operator_signature_hex TEXT NOT NULL DEFAULT '',
		retraction_reason TEXT NOT NULL DEFAULT '',
		gate_report_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_discovery_claims_session ON discovery_claims(session_id, updated_at DESC);
`

// SaveDiscoveryClaim inserts or updates a QEC discovery claim record in SQLite.
func (l *Ledger) SaveDiscoveryClaim(rec qec.DiscoveryClaimRecord) error {
	if l == nil || l.db == nil {
		return errors.New("ledger not initialized")
	}
	if strings.TrimSpace(rec.ClaimID) == "" || strings.TrimSpace(rec.CodeName) == "" {
		return errors.New("claim_id and code_name are required")
	}
	if rec.Status == "" {
		rec.Status = qec.ClaimCandidate
	}
	if rec.GateReportJSON == "" {
		rec.GateReportJSON = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	passedInt := 0
	if rec.Gates0To6Passed {
		passedInt = 1
	}
	_, err := l.db.Exec(`
		INSERT INTO discovery_claims (
			claim_id, session_id, code_name, status, evidence_tier, credibility_tier,
			artifact_sha256, gates_digest, gates_0_to_6_passed,
			operator_pubkey_hex, operator_signature_hex, retraction_reason,
			gate_report_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(claim_id) DO UPDATE SET
			session_id=excluded.session_id,
			code_name=excluded.code_name,
			status=excluded.status,
			evidence_tier=excluded.evidence_tier,
			credibility_tier=excluded.credibility_tier,
			artifact_sha256=excluded.artifact_sha256,
			gates_digest=excluded.gates_digest,
			gates_0_to_6_passed=excluded.gates_0_to_6_passed,
			operator_pubkey_hex=excluded.operator_pubkey_hex,
			operator_signature_hex=excluded.operator_signature_hex,
			retraction_reason=excluded.retraction_reason,
			gate_report_json=excluded.gate_report_json,
			updated_at=excluded.updated_at
	`,
		rec.ClaimID, rec.SessionID, rec.CodeName, string(rec.Status), rec.EvidenceTier, rec.CredibilityTier,
		rec.ArtifactSHA256, rec.GatesDigest, passedInt,
		rec.OperatorPubkeyHex, rec.OperatorSignatureHex, rec.RetractionReason,
		rec.GateReportJSON, now, now,
	)
	return err
}

// GetDiscoveryClaim loads a persisted discovery claim by claim_id.
func (l *Ledger) GetDiscoveryClaim(claimID string) (*qec.DiscoveryClaimRecord, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("ledger not initialized")
	}
	var (
		rec       qec.DiscoveryClaimRecord
		statusStr string
		passedInt int
	)
	err := l.db.QueryRow(`
		SELECT claim_id, session_id, code_name, status, evidence_tier, credibility_tier,
		       artifact_sha256, gates_digest, gates_0_to_6_passed,
		       operator_pubkey_hex, operator_signature_hex, retraction_reason, gate_report_json
		FROM discovery_claims
		WHERE claim_id = ?
	`, claimID).Scan(
		&rec.ClaimID, &rec.SessionID, &rec.CodeName, &statusStr, &rec.EvidenceTier, &rec.CredibilityTier,
		&rec.ArtifactSHA256, &rec.GatesDigest, &passedInt,
		&rec.OperatorPubkeyHex, &rec.OperatorSignatureHex, &rec.RetractionReason, &rec.GateReportJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("discovery claim %q not found", claimID)
	}
	if err != nil {
		return nil, err
	}
	rec.Status = qec.DiscoveryClaimStatus(statusStr)
	rec.Gates0To6Passed = passedInt != 0
	return &rec, nil
}

// ListDiscoveryClaims lists persisted discovery claims for a session (or all if sessionID is empty).
func (l *Ledger) ListDiscoveryClaims(sessionID string) ([]qec.DiscoveryClaimRecord, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("ledger not initialized")
	}
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(sessionID) != "" {
		rows, err = l.db.Query(`
			SELECT claim_id, session_id, code_name, status, evidence_tier, credibility_tier,
			       artifact_sha256, gates_digest, gates_0_to_6_passed,
			       operator_pubkey_hex, operator_signature_hex, retraction_reason, gate_report_json
			FROM discovery_claims
			WHERE session_id = ?
			ORDER BY updated_at DESC
		`, sessionID)
	} else {
		rows, err = l.db.Query(`
			SELECT claim_id, session_id, code_name, status, evidence_tier, credibility_tier,
			       artifact_sha256, gates_digest, gates_0_to_6_passed,
			       operator_pubkey_hex, operator_signature_hex, retraction_reason, gate_report_json
			FROM discovery_claims
			ORDER BY updated_at DESC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]qec.DiscoveryClaimRecord, 0)
	for rows.Next() {
		var (
			rec       qec.DiscoveryClaimRecord
			statusStr string
			passedInt int
		)
		if err := rows.Scan(
			&rec.ClaimID, &rec.SessionID, &rec.CodeName, &statusStr, &rec.EvidenceTier, &rec.CredibilityTier,
			&rec.ArtifactSHA256, &rec.GatesDigest, &passedInt,
			&rec.OperatorPubkeyHex, &rec.OperatorSignatureHex, &rec.RetractionReason, &rec.GateReportJSON,
		); err != nil {
			return nil, err
		}
		rec.Status = qec.DiscoveryClaimStatus(statusStr)
		rec.Gates0To6Passed = passedInt != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}

// TransitionAndPersistDiscoveryClaim enforces the Ed25519-signed lifecycle
// transition and persists the updated state durably in SQLite.
func (l *Ledger) TransitionAndPersistDiscoveryClaim(
	claimID string,
	next qec.DiscoveryClaimStatus,
	operatorPubkeyHex string,
	operatorSignatureHex string,
	retractReason string,
) (*qec.DiscoveryClaimRecord, error) {
	rec, err := l.GetDiscoveryClaim(claimID)
	if err != nil {
		return nil, err
	}
	if err := qec.TransitionDiscoveryClaim(rec, next, operatorPubkeyHex, operatorSignatureHex, retractReason); err != nil {
		return nil, err
	}
	if err := l.SaveDiscoveryClaim(*rec); err != nil {
		return nil, err
	}
	return rec, nil
}
