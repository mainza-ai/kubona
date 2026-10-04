package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventRecord struct {
	ID        int64           `json:"id"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"session_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

type Ledger struct {
	db   *sql.DB
	lock sync.RWMutex
}

func OpenLedger(dbPath string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db dir: %w", err)
	}

	// Open with SQLite WAL mode and busy timeout for high-throughput concurrent writes
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	initSchema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT,
		created_at DATETIME,
		updated_at DATETIME,
		status TEXT,
		spend_limit_usd REAL,
		current_spend_usd REAL
	);

	CREATE TABLE IF NOT EXISTS chat_messages (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		role TEXT,
		content TEXT,
		proposal_json TEXT,
		created_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		status TEXT,
		job_type TEXT,
		params_json TEXT,
		result_json TEXT,
		created_at DATETIME,
		completed_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS loops (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		status TEXT,
		step_count INTEGER,
		max_steps INTEGER,
		diverged BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS ledger_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		session_id TEXT,
		event_type TEXT,
		payload TEXT
	);

	CREATE TABLE IF NOT EXISTS campaign_states (
		session_id TEXT PRIMARY KEY,
		state_json TEXT NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_session ON jobs(session_id);
	CREATE INDEX IF NOT EXISTS idx_ledger_session ON ledger_events(session_id);
	`

	if _, err := db.Exec(initSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize ledger schema: %w", err)
	}

	return &Ledger{db: db}, nil
}

func (l *Ledger) DB() *sql.DB {
	return l.db
}

func (l *Ledger) AppendEvent(sessionID string, eventType string, payload any) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	query := `INSERT INTO ledger_events (timestamp, session_id, event_type, payload) VALUES (?, ?, ?, ?)`
	_, err = l.db.Exec(query, time.Now().UTC().Format(time.RFC3339), sessionID, eventType, string(data))
	return err
}

func (l *Ledger) GetRecentEvents(sessionID string, limit int) ([]EventRecord, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	query := `SELECT id, timestamp, session_id, event_type, payload FROM ledger_events WHERE session_id = ? ORDER BY id DESC LIMIT ?`
	rows, err := l.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventRecord
	for rows.Next() {
		var e EventRecord
		var payloadStr string
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.SessionID, &e.EventType, &payloadStr); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payloadStr)
		events = append(events, e)
	}
	return events, nil
}

func (l *Ledger) SaveCampaignState(sessionID string, stateJSON []byte) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	query := `INSERT INTO campaign_states (session_id, state_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET state_json = excluded.state_json, updated_at = excluded.updated_at`
	_, err := l.db.Exec(query, sessionID, string(stateJSON), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (l *Ledger) GetCampaignState(sessionID string) ([]byte, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	query := `SELECT state_json FROM campaign_states WHERE session_id = ?`
	row := l.db.QueryRow(query, sessionID)

	var stateStr string
	if err := row.Scan(&stateStr); err != nil {
		return nil, err
	}
	return []byte(stateStr), nil
}

func (l *Ledger) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.db.Close()
}

