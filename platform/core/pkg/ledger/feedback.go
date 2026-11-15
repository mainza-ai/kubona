package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ChatFeedback is the durable, editable vote on one assistant message.
// The ledger event remains the audit trail; this row is the current state so
// a reload can hydrate the vote and a second click can clear it.
type ChatFeedback struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	Vote      string `json:"vote"` // up | down
	Reason    string `json:"reason,omitempty"`
	Actor     string `json:"actor"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// UpsertChatFeedback records the current vote, returning the previous vote
// ("" when this is the first vote). An empty vote deletes the row.
func (l *Ledger) UpsertChatFeedback(sessionID, messageID, vote, reason, actor string) (previous string, err error) {
	if sessionID == "" || messageID == "" {
		return "", fmt.Errorf("feedback needs session and message ids")
	}
	if vote != "up" && vote != "down" && vote != "" {
		return "", fmt.Errorf("vote must be up, down, or empty")
	}
	if len([]rune(reason)) > 500 {
		return "", fmt.Errorf("feedback reason exceeds 500 characters")
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	if err := l.db.QueryRow(`SELECT vote FROM chat_feedback WHERE message_id = ? AND session_id = ?`, messageID, sessionID).Scan(&previous); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if vote == "" {
		_, err := l.db.Exec(`DELETE FROM chat_feedback WHERE message_id = ? AND session_id = ?`, messageID, sessionID)
		return previous, err
	}
	_, err = l.db.Exec(`INSERT INTO chat_feedback
		(message_id, session_id, vote, reason, actor, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET vote = excluded.vote, reason = excluded.reason,
			actor = excluded.actor, updated_at = excluded.updated_at`,
		messageID, sessionID, vote, strings.TrimSpace(reason), actor, now, now)
	return previous, err
}

// ListChatFeedback returns current votes for a session, newest first.
func (l *Ledger) ListChatFeedback(sessionID string) ([]ChatFeedback, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	rows, err := l.db.Query(`SELECT message_id, session_id, vote, reason, actor, created_at, updated_at
		FROM chat_feedback WHERE session_id = ? ORDER BY updated_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatFeedback
	for rows.Next() {
		var f ChatFeedback
		if err := rows.Scan(&f.MessageID, &f.SessionID, &f.Vote, &f.Reason, &f.Actor, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// NotificationState is the per-session read/resolve disposition for one
// derived notification key.
type NotificationState struct {
	ReadAt     string `json:"read_at,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// NotificationStates returns the stored dispositions for a session.
func (l *Ledger) NotificationStates(sessionID string) (map[string]NotificationState, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	rows, err := l.db.Query(`SELECT notification_key, read_at, resolved_at FROM notification_states WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]NotificationState{}
	for rows.Next() {
		var key string
		var st NotificationState
		if err := rows.Scan(&key, &st.ReadAt, &st.ResolvedAt); err != nil {
			return nil, err
		}
		// SQLite's DATETIME affinity normalizes an empty text value to the
		// zero time when scanned; treat that as "unset".
		if strings.HasPrefix(st.ReadAt, "0001-01-01") {
			st.ReadAt = ""
		}
		if strings.HasPrefix(st.ResolvedAt, "0001-01-01") {
			st.ResolvedAt = ""
		}
		out[key] = st
	}
	return out, nil
}

// MarkNotifications sets read and/or resolve state. Resolve also marks read;
// state is server-owned so it follows the user across devices.
func (l *Ledger) MarkNotifications(sessionID string, keys []string, resolve bool) error {
	if sessionID == "" || len(keys) == 0 {
		return nil
	}
	if len(keys) > 500 {
		return fmt.Errorf("too many notification keys")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	l.lock.Lock()
	defer l.lock.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 256 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO notification_states
			(session_id, notification_key, read_at, resolved_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(session_id, notification_key) DO UPDATE SET
				read_at = CASE WHEN excluded.read_at != '' THEN excluded.read_at ELSE notification_states.read_at END,
				resolved_at = CASE WHEN excluded.resolved_at != '' THEN excluded.resolved_at ELSE notification_states.resolved_at END`,
			sessionID, key, now, func() string {
				if resolve {
					return now
				}
				return ""
			}()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
