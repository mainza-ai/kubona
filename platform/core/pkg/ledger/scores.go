package ledger

// R6 — transcript scores: per-role rubric measurements over recorded
// tribunal transcripts. Scores are RECORDED, never enforced (measurement
// before gating): nothing reads them to allow/refuse today. The rubric
// itself is rule-based text signals (Q-26: no LLM judging) computed in
// pkg/agents/scoring.go; this file stores + lists them.

import (
	"encoding/json"
	"fmt"
	"time"
)

const scoresSchema = `
CREATE TABLE IF NOT EXISTS transcript_scores (
	session_id TEXT NOT NULL,
	obligation_id TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	score REAL NOT NULL,
	signals_json TEXT NOT NULL DEFAULT '{}',
	recorded_at TEXT NOT NULL,
	PRIMARY KEY (obligation_id, role)
);`

// TranscriptScore is one role's measured score for a run.
type TranscriptScore struct {
	SessionID    string
	ObligationID string
	Role         string
	Score        float64
	Signals      map[string]bool
}

// RecordTranscriptScores stores one run's per-role scores (upsert per
// role: rescoring replaces, never duplicates).
func (l *Ledger) RecordTranscriptScores(scores []TranscriptScore) error {
	for _, s := range scores {
		if s.SessionID == "" || s.Role == "" {
			return fmt.Errorf("score needs a session and a role")
		}
		if s.Score < 0 || s.Score > 1 {
			return fmt.Errorf("score %f out of [0,1]", s.Score)
		}
		raw, err := json.Marshal(s.Signals)
		if err != nil {
			return err
		}
		if _, err := l.DB().Exec(`INSERT INTO transcript_scores
			(session_id, obligation_id, role, score, signals_json, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(obligation_id, role)
			DO UPDATE SET score = excluded.score, signals_json = excluded.signals_json, recorded_at = excluded.recorded_at`,
			s.SessionID, s.ObligationID, s.Role, s.Score, string(raw),
			time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("record score: %w", err)
		}
	}
	return nil
}

// ListTranscriptScores returns a run's scores by obligation (empty when
// unscored — scoring is best-effort, absence is not failure).
func (l *Ledger) ListTranscriptScores(obligationID string) ([]TranscriptScore, error) {
	rows, err := l.DB().Query(`SELECT session_id, obligation_id, role, score, signals_json
		FROM transcript_scores WHERE obligation_id = ? ORDER BY role`, obligationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TranscriptScore
	for rows.Next() {
		var s TranscriptScore
		var sig string
		if err := rows.Scan(&s.SessionID, &s.ObligationID, &s.Role, &s.Score, &sig); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(sig), &s.Signals); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
