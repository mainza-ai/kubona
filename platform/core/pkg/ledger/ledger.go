package ledger

import (
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventRecord struct {
	ID           int64           `json:"id"`
	Timestamp    string          `json:"timestamp"`
	SessionID    string          `json:"session_id"`
	EventType    string          `json:"event_type"`
	Payload      json.RawMessage `json:"payload"`
	PrevHash     string          `json:"prev_hash,omitempty"`
	EntryHash    string          `json:"entry_hash,omitempty"`
	SignerPubkey string          `json:"signer_pubkey_hex,omitempty"`
	Signature    string          `json:"signature_hex,omitempty"`
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
		payload TEXT,
		prev_hash TEXT NOT NULL DEFAULT '',
		entry_hash TEXT NOT NULL DEFAULT '',
		signer_pubkey_hex TEXT NOT NULL DEFAULT '',
		signature_hex TEXT NOT NULL DEFAULT ''
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

	// Dynamic column migrations if table existed from an earlier schema version
	for _, col := range []string{"prev_hash", "entry_hash", "signer_pubkey_hex", "signature_hex"} {
		_, _ = db.Exec(fmt.Sprintf("ALTER TABLE ledger_events ADD COLUMN %s TEXT NOT NULL DEFAULT ''", col))
	}

	return &Ledger{db: db}, nil
}

func (l *Ledger) DB() *sql.DB {
	return l.db
}

func (l *Ledger) getLatestEntryHashLocked(sessionID string) string {
	var hash string
	var query string
	var row *sql.Row
	if sessionID != "" {
		query = `SELECT entry_hash FROM ledger_events WHERE session_id = ? AND entry_hash != '' ORDER BY id DESC LIMIT 1`
		row = l.db.QueryRow(query, sessionID)
	} else {
		query = `SELECT entry_hash FROM ledger_events WHERE entry_hash != '' ORDER BY id DESC LIMIT 1`
		row = l.db.QueryRow(query)
	}
	if err := row.Scan(&hash); err != nil || hash == "" {
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}
	return hash
}

// AppendSignedEvent writes a cryptographic, RFC 8032 Ed25519-signed event record into SQLite WAL
func (l *Ledger) AppendSignedEvent(sessionID string, eventType string, payload any, privKey ed25519.PrivateKey) (*EventRecord, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	prevHash := l.getLatestEntryHashLocked(sessionID)
	dataToSign := fmt.Sprintf("%s|%s|%s|%s", prevHash, sessionID, eventType, string(data))
	digest := sha256.Sum256([]byte(dataToSign))
	entryHashHex := hex.EncodeToString(digest[:])

	var signerPubHex, sigHex string
	if len(privKey) == ed25519.PrivateKeySize {
		sig := ed25519.Sign(privKey, digest[:])
		sigHex = hex.EncodeToString(sig)
		pub := privKey.Public().(ed25519.PublicKey)
		signerPubHex = hex.EncodeToString(pub)
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	query := `INSERT INTO ledger_events (timestamp, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := l.db.Exec(query, ts, sessionID, eventType, string(data), prevHash, entryHashHex, signerPubHex, sigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to insert signed event: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		id = 0
	}

	return &EventRecord{
		ID:           id,
		Timestamp:    ts,
		SessionID:    sessionID,
		EventType:    eventType,
		Payload:      json.RawMessage(data),
		PrevHash:     prevHash,
		EntryHash:    entryHashHex,
		SignerPubkey: signerPubHex,
		Signature:    sigHex,
	}, nil
}

// AppendEvent writes an unsigned event with cryptographic hash chaining
func (l *Ledger) AppendEvent(sessionID string, eventType string, payload any) error {
	_, err := l.AppendSignedEvent(sessionID, eventType, payload, nil)
	return err
}

// VerifyEventSignature verifies the cryptographic integrity and RFC 8032 Ed25519 signature of an EventRecord
func (l *Ledger) VerifyEventSignature(event *EventRecord) (bool, error) {
	if event == nil {
		return false, fmt.Errorf("event is nil")
	}
	if event.SignerPubkey == "" || event.Signature == "" {
		return false, fmt.Errorf("event is unsigned")
	}

	pubBytes, err := hex.DecodeString(event.SignerPubkey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	sigBytes, err := hex.DecodeString(event.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature size: %w", err)
	}

	dataToSign := fmt.Sprintf("%s|%s|%s|%s", event.PrevHash, event.SessionID, event.EventType, string(event.Payload))
	digest := sha256.Sum256([]byte(dataToSign))

	if hex.EncodeToString(digest[:]) != event.EntryHash {
		return false, fmt.Errorf("entry hash mismatch: computed %s, recorded %s", hex.EncodeToString(digest[:]), event.EntryHash)
	}

	valid := ed25519.Verify(pubBytes, digest[:], sigBytes)
	return valid, nil
}

func (l *Ledger) GetRecentEvents(sessionID string, limit int) ([]EventRecord, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	query := `SELECT id, timestamp, session_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events WHERE session_id = ? ORDER BY id DESC LIMIT ?`
	rows, err := l.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventRecord
	for rows.Next() {
		var e EventRecord
		var payloadStr string
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.SessionID, &e.EventType, &payloadStr, &e.PrevHash, &e.EntryHash, &e.SignerPubkey, &e.Signature); err != nil {
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

