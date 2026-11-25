package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Schema 18: durable, session-scoped codesign_runs receipts (Phase 0 / G1).
const codesignRunsSchema = `
	CREATE TABLE IF NOT EXISTS codesign_runs (
		run_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		operation TEXT NOT NULL DEFAULT 'evaluate',
		spec_kind TEXT NOT NULL,
		spec_table_sha256 TEXT NOT NULL DEFAULT '',
		artifact_sha256 TEXT NOT NULL DEFAULT '',
		artifact_json TEXT NOT NULL DEFAULT '[]',
		kappa INTEGER NOT NULL DEFAULT 1,
		seed INTEGER NOT NULL DEFAULT 7,
		num_tests INTEGER NOT NULL DEFAULT 64,
		max_failures INTEGER NOT NULL DEFAULT 0,
		valid INTEGER NOT NULL DEFAULT 0,
		toffoli INTEGER NOT NULL DEFAULT 0,
		peak_qubits INTEGER NOT NULL DEFAULT 0,
		tq INTEGER NOT NULL DEFAULT 0,
		eps_bound REAL NOT NULL DEFAULT 1.0,
		verification_method TEXT NOT NULL DEFAULT '',
		depth INTEGER NOT NULL DEFAULT 0,
		toffoli_depth INTEGER NOT NULL DEFAULT 0,
		result_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_codesign_runs_session ON codesign_runs(session_id, created_at DESC);
`

// CodesignRunRecord is one durable, receipted circuit-codesign evaluation.
type CodesignRunRecord struct {
	RunID              string  `json:"run_id"`
	SessionID          string  `json:"session_id"`
	Operation          string  `json:"operation"`
	SpecKind           string  `json:"spec_kind"`
	SpecTableSHA256    string  `json:"spec_table_sha256"`
	ArtifactSHA256     string  `json:"artifact_sha256"`
	ArtifactJSON       string  `json:"artifact_json"`
	Kappa              int     `json:"kappa"`
	Seed               int     `json:"seed"`
	NumTests           int     `json:"num_tests"`
	MaxFailures        int     `json:"max_failures"`
	Valid              bool    `json:"valid"`
	Toffoli            int     `json:"toffoli"`
	PeakQubits         int     `json:"peak_qubits"`
	TQ                 int     `json:"tq"`
	EpsBound           float64 `json:"eps_bound"`
	VerificationMethod string  `json:"verification_method"`
	Depth              int     `json:"depth"`
	ToffoliDepth       int     `json:"toffoli_depth"`
	ResultJSON         string  `json:"result_json"`
	CreatedAt          string  `json:"created_at"`
}

// ErrCodesignRunNotFound is returned when a run_id does not exist in the requested session.
var ErrCodesignRunNotFound = errors.New("codesign run not found in session")

// SaveCodesignRun persists a durable codesign evaluation receipt scoped to SessionID.
func (l *Ledger) SaveCodesignRun(rec CodesignRunRecord) error {
	if l == nil || l.db == nil {
		return errors.New("ledger not initialized")
	}
	rid := strings.TrimSpace(rec.RunID)
	sid := strings.TrimSpace(rec.SessionID)
	if rid == "" || sid == "" || strings.TrimSpace(rec.SpecKind) == "" {
		return errors.New("run_id, session_id, and spec_kind are required")
	}
	if rec.Operation == "" {
		rec.Operation = "evaluate"
	}
	if rec.ArtifactJSON == "" {
		rec.ArtifactJSON = "[]"
	}
	if rec.ResultJSON == "" {
		rec.ResultJSON = "{}"
	}
	validInt := 0
	if rec.Valid {
		validInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := l.db.Exec(`
		INSERT INTO codesign_runs (
			run_id, session_id, operation, spec_kind, spec_table_sha256,
			artifact_sha256, artifact_json, kappa, seed, num_tests, max_failures,
			valid, toffoli, peak_qubits, tq, eps_bound, verification_method,
			depth, toffoli_depth, result_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rid, sid, rec.Operation, rec.SpecKind, rec.SpecTableSHA256,
		rec.ArtifactSHA256, rec.ArtifactJSON, rec.Kappa, rec.Seed, rec.NumTests, rec.MaxFailures,
		validInt, rec.Toffoli, rec.PeakQubits, rec.TQ, rec.EpsBound, rec.VerificationMethod,
		rec.Depth, rec.ToffoliDepth, rec.ResultJSON, now,
	)
	return err
}

// GetCodesignRun fetches a single codesign run strictly scoped to sessionID.
func (l *Ledger) GetCodesignRun(sessionID, runID string) (*CodesignRunRecord, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("ledger not initialized")
	}
	sid := strings.TrimSpace(sessionID)
	rid := strings.TrimSpace(runID)
	if sid == "" || rid == "" {
		return nil, ErrCodesignRunNotFound
	}
	var (
		rec      CodesignRunRecord
		validInt int
	)
	err := l.db.QueryRow(`
		SELECT run_id, session_id, operation, spec_kind, spec_table_sha256,
		       artifact_sha256, artifact_json, kappa, seed, num_tests, max_failures,
		       valid, toffoli, peak_qubits, tq, eps_bound, verification_method,
		       depth, toffoli_depth, result_json, created_at
		FROM codesign_runs
		WHERE session_id = ? AND run_id = ?
	`, sid, rid).Scan(
		&rec.RunID, &rec.SessionID, &rec.Operation, &rec.SpecKind, &rec.SpecTableSHA256,
		&rec.ArtifactSHA256, &rec.ArtifactJSON, &rec.Kappa, &rec.Seed, &rec.NumTests, &rec.MaxFailures,
		&validInt, &rec.Toffoli, &rec.PeakQubits, &rec.TQ, &rec.EpsBound, &rec.VerificationMethod,
		&rec.Depth, &rec.ToffoliDepth, &rec.ResultJSON, &rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrCodesignRunNotFound, rid)
	}
	if err != nil {
		return nil, err
	}
	rec.Valid = validInt != 0
	return &rec, nil
}

// ListCodesignRuns lists durable codesign runs for a session, newest first.
func (l *Ledger) ListCodesignRuns(sessionID string, limit int) ([]CodesignRunRecord, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("ledger not initialized")
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil, errors.New("session_id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := l.db.Query(`
		SELECT run_id, session_id, operation, spec_kind, spec_table_sha256,
		       artifact_sha256, artifact_json, kappa, seed, num_tests, max_failures,
		       valid, toffoli, peak_qubits, tq, eps_bound, verification_method,
		       depth, toffoli_depth, result_json, created_at
		FROM codesign_runs
		WHERE session_id = ?
		ORDER BY created_at DESC, rowid DESC
		LIMIT ?
	`, sid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CodesignRunRecord, 0)
	for rows.Next() {
		var (
			rec      CodesignRunRecord
			validInt int
		)
		if err := rows.Scan(
			&rec.RunID, &rec.SessionID, &rec.Operation, &rec.SpecKind, &rec.SpecTableSHA256,
			&rec.ArtifactSHA256, &rec.ArtifactJSON, &rec.Kappa, &rec.Seed, &rec.NumTests, &rec.MaxFailures,
			&validInt, &rec.Toffoli, &rec.PeakQubits, &rec.TQ, &rec.EpsBound, &rec.VerificationMethod,
			&rec.Depth, &rec.ToffoliDepth, &rec.ResultJSON, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		rec.Valid = validInt != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ResolveLedgerRef checks whether ref resolves to a real persisted record in
// SQLite (codesign_runs.run_id, discovery_claims.claim_id, jobs.id, or
// events.entry_hash / events.id). Returns (tableKind, true) on match.
func (l *Ledger) ResolveLedgerRef(ref string) (string, bool) {
	if l == nil || l.db == nil {
		return "", false
	}
	r := strings.TrimSpace(ref)
	if r == "" {
		return "", false
	}
	// Strip optional "ledger:" prefix (e.g. "ledger:run:cs_..." or bare "cs_...")
	if idx := strings.LastIndex(r, ":"); idx >= 0 && idx+1 < len(r) {
		cand := strings.TrimSpace(r[idx+1:])
		if cand != "" {
			if kind, ok := l.resolveRawRef(cand); ok {
				return kind, true
			}
		}
	}
	return l.resolveRawRef(r)
}

func (l *Ledger) resolveRawRef(r string) (string, bool) {
	var exists int
	if err := l.db.QueryRow(`SELECT 1 FROM codesign_runs WHERE run_id = ? LIMIT 1`, r).Scan(&exists); err == nil {
		return "codesign_run", true
	}
	if err := l.db.QueryRow(`SELECT 1 FROM discovery_claims WHERE claim_id = ? LIMIT 1`, r).Scan(&exists); err == nil {
		return "discovery_claim", true
	}
	if err := l.db.QueryRow(`SELECT 1 FROM jobs WHERE id = ? LIMIT 1`, r).Scan(&exists); err == nil {
		return "job", true
	}
	if err := l.db.QueryRow(`SELECT 1 FROM events WHERE entry_hash = ? OR CAST(id AS TEXT) = ? LIMIT 1`, r, r).Scan(&exists); err == nil {
		return "event", true
	}
	return "", false
}
