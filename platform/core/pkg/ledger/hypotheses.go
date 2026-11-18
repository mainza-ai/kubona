package ledger

// Scientific hypotheses (AG-2): falsifiable candidates derived from the
// measured campaign state and literature. A hypothesis is not a claim: it is
// a probe with a stated falsifier, a proposed experiment family, and an
// explicit novelty status. Only later evidence can collapse it into a claim.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const hypothesesSchema = `
CREATE TABLE IF NOT EXISTS hypotheses (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	statement TEXT NOT NULL,
	rationale TEXT NOT NULL DEFAULT '',
	falsifier TEXT NOT NULL,
	experiment_kind TEXT NOT NULL DEFAULT '',
	experiment_hint TEXT NOT NULL DEFAULT '',
	literature_json TEXT NOT NULL DEFAULT '[]',
	novelty_status TEXT NOT NULL DEFAULT 'unmeasured',
	novelty_method TEXT NOT NULL DEFAULT '',
	novelty_distance REAL,
	score REAL NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'proposed',
	source TEXT NOT NULL DEFAULT 'diagnostic',
	claim_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hypotheses_session
	ON hypotheses(session_id, created_at DESC);`

const (
	HypothesisProposed  = "proposed"
	HypothesisShortlist = "shortlisted"
	HypothesisRejected  = "rejected"
	HypothesisCollapsed = "collapsed"
)

type Hypothesis struct {
	ID              string   `json:"id"`
	SessionID       string   `json:"session_id"`
	Statement       string   `json:"statement"`
	Rationale       string   `json:"rationale"`
	Falsifier       string   `json:"falsifier"`
	ExperimentKind  string   `json:"experiment_kind"`
	ExperimentHint  string   `json:"experiment_hint"`
	Literature      []string `json:"literature_refs"`
	NoveltyStatus   string   `json:"novelty_status"`
	NoveltyMethod   string   `json:"novelty_method,omitempty"`
	NoveltyDistance *float64 `json:"novelty_distance,omitempty"`
	Score           float64  `json:"score"`
	Status          string   `json:"status"`
	Source          string   `json:"source"`
	ClaimID         string   `json:"claim_id,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

var ErrHypothesisNotFound = errors.New("hypothesis not found")

func validHypothesisStatus(v string) bool {
	switch v {
	case HypothesisProposed, HypothesisShortlist, HypothesisRejected, HypothesisCollapsed:
		return true
	}
	return false
}

func (l *Ledger) CreateHypothesis(sessionID, tenantID string, h *Hypothesis) error {
	if h == nil || h.ID == "" || h.Statement == "" || h.Falsifier == "" {
		return fmt.Errorf("hypothesis needs id, statement, and falsifier")
	}
	if h.Status == "" {
		h.Status = HypothesisProposed
	}
	if !validHypothesisStatus(h.Status) {
		return fmt.Errorf("hypothesis status %q is not proposed|shortlisted|rejected|collapsed", h.Status)
	}
	if h.NoveltyStatus == "" {
		h.NoveltyStatus = "unmeasured"
	}
	if h.Literature == nil {
		h.Literature = []string{}
	}
	lit, err := jsonStrings(h.Literature)
	if err != nil {
		return err
	}
	now := nowUTC()
	_, err = l.DB().Exec(`INSERT INTO hypotheses
		(id, session_id, tenant_id, statement, rationale, falsifier, experiment_kind,
		 experiment_hint, literature_json, novelty_status, novelty_method, novelty_distance,
		 score, status, source, claim_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, sessionID, tenantID, h.Statement, h.Rationale, h.Falsifier, h.ExperimentKind,
		h.ExperimentHint, lit, h.NoveltyStatus, h.NoveltyMethod, h.NoveltyDistance, h.Score,
		h.Status, h.Source, h.ClaimID, now, now)
	if err != nil {
		return fmt.Errorf("insert hypothesis: %w", err)
	}
	h.SessionID, h.CreatedAt, h.UpdatedAt = sessionID, now, now
	return nil
}

func (l *Ledger) ListHypotheses(sessionID string, limit int) ([]Hypothesis, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := l.DB().Query(`SELECT id, session_id, statement, rationale, falsifier,
		experiment_kind, experiment_hint, literature_json, novelty_status, novelty_method,
		novelty_distance, score, status, source, claim_id, created_at, updated_at
		FROM hypotheses WHERE session_id = ? ORDER BY score DESC, created_at ASC LIMIT ?`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hypothesis{}
	for rows.Next() {
		var h Hypothesis
		var lit string
		var distance sql.NullFloat64
		if err := rows.Scan(&h.ID, &h.SessionID, &h.Statement, &h.Rationale, &h.Falsifier,
			&h.ExperimentKind, &h.ExperimentHint, &lit, &h.NoveltyStatus, &h.NoveltyMethod,
			&distance, &h.Score, &h.Status, &h.Source, &h.ClaimID, &h.CreatedAt,
			&h.UpdatedAt); err != nil {
			return nil, err
		}
		_ = jsonUnmarshalStrings(lit, &h.Literature)
		if distance.Valid {
			v := distance.Float64
			h.NoveltyDistance = &v
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SetHypothesisScore persists a tournament-derived score. Scores are a ranking
// signal, not a truth value; the judge that produced them is recorded on the
// tournament event.
func (l *Ledger) SetHypothesisScore(sessionID, id string, score float64) error {
	if score < 0 || score > 1 {
		return fmt.Errorf("hypothesis score %v is outside [0,1]", score)
	}
	res, err := l.DB().Exec(`UPDATE hypotheses SET score = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, score, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrHypothesisNotFound
	}
	return nil
}

// SetHypothesisNovelty records a measured novelty distance and the method that
// produced it. A distance without a method is refused: an unlabelled number
// cannot be trusted or reproduced.
func (l *Ledger) SetHypothesisNovelty(sessionID, id, status, method string, distance *float64) error {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "measured", "unmeasured":
	default:
		return fmt.Errorf("novelty status %q is not measured|unmeasured", status)
	}
	if status == "measured" && (distance == nil || strings.TrimSpace(method) == "") {
		return fmt.Errorf("measured novelty needs a distance and a method")
	}
	if status == "unmeasured" {
		distance = nil
		method = ""
	}
	res, err := l.DB().Exec(`UPDATE hypotheses SET novelty_status = ?, novelty_method = ?,
		novelty_distance = ?, updated_at = ? WHERE session_id = ? AND id = ?`,
		status, method, distance, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrHypothesisNotFound
	}
	return nil
}

// CollapseHypothesis turns a hypothesis into a cited claim. The claim id is
// evidence: collapse is refused without one, so a hypothesis can never become
// a claim by status change alone.
func (l *Ledger) CollapseHypothesis(sessionID, id, claimID string) error {
	if strings.TrimSpace(claimID) == "" {
		return fmt.Errorf("collapsing a hypothesis needs a claim id")
	}
	res, err := l.DB().Exec(`UPDATE hypotheses SET status = ?, claim_id = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, HypothesisCollapsed, claimID, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrHypothesisNotFound
	}
	return nil
}

func (l *Ledger) SetHypothesisStatus(sessionID, id, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if !validHypothesisStatus(status) {
		return fmt.Errorf("hypothesis status %q is not proposed|shortlisted|rejected|collapsed", status)
	}
	res, err := l.DB().Exec(`UPDATE hypotheses SET status = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, status, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrHypothesisNotFound
	}
	return nil
}

// Small local JSON helpers keep this file independent of the API layer.
func jsonStrings(v []string) (string, error) {
	if len(v) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func jsonUnmarshalStrings(raw string, out *[]string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}
