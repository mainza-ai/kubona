package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/qec"
)

// Schema 17: durable, Ed25519-signed, session-scoped QEC discovery claim
// lifecycle ledger (P0-3 / N8).
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

// ErrDiscoveryClaimNotFound is returned when a claim does not exist in the requested session.
var ErrDiscoveryClaimNotFound = errors.New("discovery claim not found in session")

// SaveDiscoveryClaim inserts or updates a session-scoped QEC discovery claim in SQLite.
// Refuses empty session_id or cross-session claim_id collision (N8).
func (l *Ledger) SaveDiscoveryClaim(rec qec.DiscoveryClaimRecord) error {
	if l == nil || l.db == nil {
		return errors.New("ledger not initialized")
	}
	sid := strings.TrimSpace(rec.SessionID)
	cid := strings.TrimSpace(rec.ClaimID)
	if sid == "" || cid == "" || strings.TrimSpace(rec.CodeName) == "" {
		return errors.New("session_id, claim_id, and code_name are required")
	}
	// Prevent cross-session overwrite (N8 IDOR guard).
	var existingSession string
	err := l.db.QueryRow(`SELECT session_id FROM discovery_claims WHERE claim_id = ?`, cid).Scan(&existingSession)
	if err == nil && existingSession != "" && existingSession != sid {
		return fmt.Errorf("claim_id %q already belongs to another session", cid)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
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
	_, err = l.db.Exec(`
		INSERT INTO discovery_claims (
			claim_id, session_id, code_name, status, evidence_tier, credibility_tier,
			artifact_sha256, gates_digest, gates_0_to_6_passed,
			operator_pubkey_hex, operator_signature_hex, retraction_reason,
			gate_report_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(claim_id) DO UPDATE SET
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
		WHERE discovery_claims.session_id = excluded.session_id
	`,
		cid, sid, rec.CodeName, string(rec.Status), rec.EvidenceTier, rec.CredibilityTier,
		rec.ArtifactSHA256, rec.GatesDigest, passedInt,
		rec.OperatorPubkeyHex, rec.OperatorSignatureHex, rec.RetractionReason,
		rec.GateReportJSON, now, now,
	)
	return err
}

// GetDiscoveryClaim loads a persisted discovery claim strictly scoped to sessionID (N8).
func (l *Ledger) GetDiscoveryClaim(sessionID, claimID string) (*qec.DiscoveryClaimRecord, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("ledger not initialized")
	}
	sid := strings.TrimSpace(sessionID)
	cid := strings.TrimSpace(claimID)
	if sid == "" || cid == "" {
		return nil, ErrDiscoveryClaimNotFound
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
		WHERE session_id = ? AND claim_id = ?
	`, sid, cid).Scan(
		&rec.ClaimID, &rec.SessionID, &rec.CodeName, &statusStr, &rec.EvidenceTier, &rec.CredibilityTier,
		&rec.ArtifactSHA256, &rec.GatesDigest, &passedInt,
		&rec.OperatorPubkeyHex, &rec.OperatorSignatureHex, &rec.RetractionReason, &rec.GateReportJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrDiscoveryClaimNotFound, cid)
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

// TransitionAndPersistDiscoveryClaim enforces session-scoped (N8) and
// Ed25519-signed lifecycle transitions and persists the updated state in SQLite.
func (l *Ledger) TransitionAndPersistDiscoveryClaim(
	sessionID string,
	claimID string,
	next qec.DiscoveryClaimStatus,
	operatorPubkeyHex string,
	operatorSignatureHex string,
	retractReason string,
) (*qec.DiscoveryClaimRecord, error) {
	rec, err := l.GetDiscoveryClaim(sessionID, claimID)
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
