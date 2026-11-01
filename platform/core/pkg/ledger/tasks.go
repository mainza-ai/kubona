package ledger

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Task is a human-owned unit of follow-up work attached to a session and an
// optional authoritative record. It is intentionally separate from layout:
// the workbench stores the reference, never task content.
type Task struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	SourceDomain string `json:"source_domain"`
	SourceID     string `json:"source_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (l *Ledger) CreateTask(sessionID, title, sourceDomain, sourceID string) (Task, error) {
	if sessionID == "" || title == "" {
		return Task{}, fmt.Errorf("task requires session_id and title")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	task := Task{
		ID: uuid.NewString(), SessionID: sessionID, Title: title, Status: "open",
		SourceDomain: sourceDomain, SourceID: sourceID, CreatedAt: now, UpdatedAt: now,
	}
	_, err := l.DB().Exec(
		`INSERT INTO tasks (id, session_id, title, status, source_domain, source_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.SessionID, task.Title, task.Status, task.SourceDomain, task.SourceID, task.CreatedAt, task.UpdatedAt,
	)
	return task, err
}

func (l *Ledger) ListTasks(sessionID string) ([]Task, error) {
	rows, err := l.DB().Query(
		`SELECT id, session_id, title, status, source_domain, source_id, created_at, updated_at
		 FROM tasks WHERE session_id = ? ORDER BY created_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Status, &t.SourceDomain, &t.SourceID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (l *Ledger) UpdateTaskStatus(id, sessionID, status string) (Task, error) {
	if status != "open" && status != "done" {
		return Task{}, fmt.Errorf("task status must be open or done")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := l.DB().Exec(
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ? AND session_id = ?`,
		status, now, id, sessionID)
	if err != nil {
		return Task{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Task{}, sql.ErrNoRows
	}
	return l.GetTask(id, sessionID)
}

func (l *Ledger) GetTask(id, sessionID string) (Task, error) {
	var t Task
	err := l.DB().QueryRow(
		`SELECT id, session_id, title, status, source_domain, source_id, created_at, updated_at
		 FROM tasks WHERE id = ? AND session_id = ?`, id, sessionID).
		Scan(&t.ID, &t.SessionID, &t.Title, &t.Status, &t.SourceDomain, &t.SourceID, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
