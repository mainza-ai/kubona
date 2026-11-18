package ledger

// Read-only job records for provenance export (AG-6b). The Runner owns job
// execution; the ledger owns the durable row, so the exporter can read it
// without starting a runner or touching the API.

import "database/sql"

type JobRecord struct {
	ID          string `json:"id"`
	JobType     string `json:"job_type"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

func (l *Ledger) ListJobRecords(sessionID string, limit int) ([]JobRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := l.DB().Query(`SELECT id, COALESCE(job_type, ''), COALESCE(status, ''),
		COALESCE(created_at, ''), completed_at
		FROM jobs WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobRecord{}
	for rows.Next() {
		var rec JobRecord
		var completed sql.NullString
		if err := rows.Scan(&rec.ID, &rec.JobType, &rec.Status, &rec.CreatedAt, &completed); err != nil {
			return nil, err
		}
		if completed.Valid {
			rec.CompletedAt = completed.String
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
