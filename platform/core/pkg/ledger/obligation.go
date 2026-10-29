package ledger

// R2 — obligation store: machine-enforced closure for claims and mandates.
// Vocabulary without enforcement closes on prose; this table is what R2
// queries instead ("no claim reaches closed without verifier authorities
// on record — asserted by query, not by reading code").
//
// Rules live in pkg/agents/obligation.go (AttemptClose); this file is dumb
// storage: obligations + cases + unknowns. Owner is set-once (a second,
// different owner refuses — duplicate owners keep it open by refusal).
// Cases are keyed (obligation, kind, authority): re-recording the same
// key replaces the verdict (latest evidence wins); a new key adds.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const obligationSchema = `
CREATE TABLE IF NOT EXISTS obligations (
	id TEXT PRIMARY KEY,
	version INTEGER NOT NULL DEFAULT 1,
	subject_kind TEXT NOT NULL,
	subject_id TEXT NOT NULL,
	owner TEXT NOT NULL DEFAULT '',
	required_cases_json TEXT NOT NULL DEFAULT '[]',
	required_authorities_json TEXT NOT NULL DEFAULT '[]',
	dependencies_json TEXT NOT NULL DEFAULT '[]',
	unknowns_json TEXT NOT NULL DEFAULT '[]',
	disposition TEXT NOT NULL DEFAULT 'open',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(subject_kind, subject_id)
);
CREATE TABLE IF NOT EXISTS obligation_cases (
	obligation_id TEXT NOT NULL,
	case_kind TEXT NOT NULL,
	verdict TEXT NOT NULL,
	authority TEXT NOT NULL,
	evidence_id TEXT NOT NULL DEFAULT '',
	recorded_at TEXT NOT NULL,
	PRIMARY KEY (obligation_id, case_kind, authority)
);`

// Obligation is one closable unit of proof.
type Obligation struct {
	ID                  string
	Version             int
	SubjectKind         string
	SubjectID           string
	Owner               string
	RequiredCases       []string
	RequiredAuthorities []string
	Dependencies        []string
	Unknowns            []Unknown
	Disposition         string // "open" | "closed"
}

// Unknown is a residual unknown (P1-9): an identified gap that blocks
// closure until resolved. Unknowns have IDs; closing with any unresolved
// unknown refuses.
type Unknown struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// ObligationCase is one recorded verdict on an obligation.
type ObligationCase struct {
	CaseKind  string
	Verdict   string // "pass" | "fail" | "contested"
	Authority string
	Evidence  string
}

func newObligationID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "ob_" + hex.EncodeToString(b[:]), nil
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// errorsIsNoRows reports sql.ErrNoRows without string-matching.
func errorsIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// OpenObligation creates (or returns the existing) obligation for a
// subject. Idempotent on (subject_kind, subject_id): retries and
// double-opens return the same row instead of forking obligations.
func (l *Ledger) OpenObligation(subjectKind, subjectID string, requiredCases, requiredAuthorities, dependencies []string) (*Obligation, error) {
	if subjectKind == "" || subjectID == "" {
		return nil, fmt.Errorf("obligation needs a subject kind and id (empty input never closes)")
	}
	if existing, err := l.GetObligationBySubject(subjectKind, subjectID); err == nil && existing != nil {
		return existing, nil
	}
	id, err := newObligationID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = l.DB().Exec(`INSERT INTO obligations
		(id, version, subject_kind, subject_id, required_cases_json, required_authorities_json, dependencies_json, unknowns_json, disposition, created_at, updated_at)
		VALUES (?, 1, ?, ?, ?, ?, ?, '[]', 'open', ?, ?)`,
		id, subjectKind, subjectID, mustJSON(requiredCases), mustJSON(requiredAuthorities), mustJSON(dependencies), now, now)
	if err != nil {
		// Lost the open race: return the winner (idempotent under storm).
		if existing, gerr := l.GetObligationBySubject(subjectKind, subjectID); gerr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("open obligation: %w", err)
	}
	return l.GetObligation(id)
}

func scanObligation(row interface {
	Scan(...any) error
}) (*Obligation, error) {
	var ob Obligation
	var reqCases, reqAuth, deps, unknowns string
	var created, updated string
	if err := row.Scan(&ob.ID, &ob.Version, &ob.SubjectKind, &ob.SubjectID, &ob.Owner,
		&reqCases, &reqAuth, &deps, &unknowns, &ob.Disposition, &created, &updated); err != nil {
		return nil, err
	}
	for _, tc := range []struct {
		raw string
		dst any
	}{
		{reqCases, &ob.RequiredCases}, {reqAuth, &ob.RequiredAuthorities},
		{deps, &ob.Dependencies}, {unknowns, &ob.Unknowns},
	} {
		if tc.raw == "" {
			tc.raw = "[]"
		}
		if err := json.Unmarshal([]byte(tc.raw), tc.dst); err != nil {
			return nil, fmt.Errorf("obligation %s corrupt JSON: %w", ob.ID, err)
		}
	}
	return &ob, nil
}

// GetObligation loads by id (nil, nil when absent).
func (l *Ledger) GetObligation(id string) (*Obligation, error) {
	row := l.DB().QueryRow(`SELECT id, version, subject_kind, subject_id, owner,
		required_cases_json, required_authorities_json, dependencies_json, unknowns_json,
		disposition, created_at, updated_at FROM obligations WHERE id = ?`, id)
	ob, err := scanObligation(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return ob, nil
}

// GetObligationBySubject loads by subject (nil, nil when absent).
func (l *Ledger) GetObligationBySubject(kind, subject string) (*Obligation, error) {
	row := l.DB().QueryRow(`SELECT id, version, subject_kind, subject_id, owner,
		required_cases_json, required_authorities_json, dependencies_json, unknowns_json,
		disposition, created_at, updated_at FROM obligations WHERE subject_kind = ? AND subject_id = ?`, kind, subject)
	ob, err := scanObligation(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return ob, nil
}

// ListOpenObligations returns non-closed obligations, oldest first
// (steady-state review order), with optional owner and subject-kind
// filters (empty = unfiltered). Bounded by limit (<=0 means 100, capped
// at 1000). Read-only: the review queue reads through this, never around
// it — no unbounded full-table exposure.
func (l *Ledger) ListOpenObligations(limit int, owner, subjectKind string) ([]*Obligation, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := `SELECT id, version, subject_kind, subject_id, owner,
		required_cases_json, required_authorities_json, dependencies_json, unknowns_json,
		disposition, created_at, updated_at FROM obligations WHERE disposition != 'closed'`
	args := []any{}
	if owner != "" {
		query += ` AND owner = ?`
		args = append(args, owner)
	}
	if subjectKind != "" {
		query += ` AND subject_kind = ?`
		args = append(args, subjectKind)
	}
	query += ` ORDER BY updated_at ASC LIMIT ?`
	args = append(args, limit)
	rows, err := l.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Obligation
	for rows.Next() {
		ob, err := scanObligation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ob)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SetObligationOwner sets the single accountable owner. A second,
// different owner refuses (duplicate owners keep it open by refusal);
// re-setting the same owner is a no-op receipt.
func (l *Ledger) SetObligationOwner(id, owner string) error {
	if owner == "" {
		return fmt.Errorf("owner must be non-empty")
	}
	ob, err := l.GetObligation(id)
	if err != nil {
		return err
	}
	if ob == nil {
		return fmt.Errorf("unknown obligation %q", id)
	}
	if ob.Owner != "" {
		if ob.Owner == owner {
			return nil
		}
		return fmt.Errorf("obligation %s already owned by %q: duplicate owners refused", id, ob.Owner)
	}
	_, err = l.DB().Exec(`UPDATE obligations SET owner = ?, updated_at = ? WHERE id = ?`,
		owner, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// RecordObligationCase stores one verdict (latest wins per key). Cases on
// a CLOSED obligation refuse: closed history is immutable — new evidence
// opens a new obligation (new version subject), never rewrites closure.
func (l *Ledger) RecordObligationCase(id string, c ObligationCase) error {
	if c.CaseKind == "" || c.Authority == "" {
		return fmt.Errorf("case needs a kind and an authority")
	}
	switch c.Verdict {
	case "pass", "fail", "contested":
	default:
		return fmt.Errorf("case verdict must be pass|fail|contested, got %q", c.Verdict)
	}
	ob, err := l.GetObligation(id)
	if err != nil {
		return err
	}
	if ob == nil {
		return fmt.Errorf("unknown obligation %q", id)
	}
	if ob.Disposition == "closed" {
		return fmt.Errorf("obligation %s is closed: new evidence opens a new obligation, never rewrites closure", id)
	}
	_, err = l.DB().Exec(`INSERT INTO obligation_cases
		(obligation_id, case_kind, verdict, authority, evidence_id, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(obligation_id, case_kind, authority)
		DO UPDATE SET verdict = excluded.verdict, evidence_id = excluded.evidence_id, recorded_at = excluded.recorded_at`,
		id, c.CaseKind, c.Verdict, c.Authority, c.Evidence, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListObligationCases returns all recorded cases.
func (l *Ledger) ListObligationCases(id string) ([]ObligationCase, error) {
	rows, err := l.DB().Query(`SELECT case_kind, verdict, authority, evidence_id FROM obligation_cases WHERE obligation_id = ? ORDER BY case_kind, authority`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObligationCase
	for rows.Next() {
		var c ObligationCase
		if err := rows.Scan(&c.CaseKind, &c.Verdict, &c.Authority, &c.Evidence); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RecordObligationUnknown adds a residual unknown (blocks closure until
// resolved). Same ID twice is a no-op receipt.
func (l *Ledger) RecordObligationUnknown(id string, u Unknown) error {
	if u.ID == "" {
		return fmt.Errorf("unknown needs an id")
	}
	ob, err := l.GetObligation(id)
	if err != nil {
		return err
	}
	if ob == nil {
		return fmt.Errorf("unknown obligation %q", id)
	}
	for _, e := range ob.Unknowns {
		if e.ID == u.ID {
			return nil
		}
	}
	ob.Unknowns = append(ob.Unknowns, u)
	_, err = l.DB().Exec(`UPDATE obligations SET unknowns_json = ?, updated_at = ? WHERE id = ?`,
		mustJSON(ob.Unknowns), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ResolveObligationUnknown removes a residual unknown. Unknown IDs are
// never reused within an obligation (audit keeps the resolution event in
// cases — callers record a pass case alongside).
func (l *Ledger) ResolveObligationUnknown(id, unknownID string) error {
	ob, err := l.GetObligation(id)
	if err != nil {
		return err
	}
	if ob == nil {
		return fmt.Errorf("unknown obligation %q", id)
	}
	kept := ob.Unknowns[:0]
	found := false
	for _, e := range ob.Unknowns {
		if e.ID == unknownID {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return fmt.Errorf("obligation %s has no unknown %q", id, unknownID)
	}
	if kept == nil {
		kept = []Unknown{}
	}
	_, err = l.DB().Exec(`UPDATE obligations SET unknowns_json = ?, updated_at = ? WHERE id = ?`,
		mustJSON(kept), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// SetObligationDisposition flips open → closed (only the lifecycle may
// call this, after AttemptClose passes).
func (l *Ledger) SetObligationDisposition(id, disposition string) error {
	if disposition != "open" && disposition != "closed" {
		return fmt.Errorf("disposition must be open|closed, got %q", disposition)
	}
	_, err := l.DB().Exec(`UPDATE obligations SET disposition = ?, updated_at = ? WHERE id = ?`,
		disposition, time.Now().UTC().Format(time.RFC3339), id)
	return err
}
