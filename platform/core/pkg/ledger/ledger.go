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
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventRecord struct {
	ID           int64           `json:"id"`
	Timestamp    string          `json:"timestamp"`
	SessionID    string          `json:"session_id"`
	TenantID     string          `json:"tenant_id"`
	ConfLabel    string          `json:"conf_label"`
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

func OpenLedgerReadOnly(dbPath string) (*Ledger, error) {
	// Read-replica handle for scaled deployments: WAL allows concurrent
	// readers (even cross-process) while the single writer appends. Never
	// creates the file; writes through this handle fail at the driver.
	// Verify the file exists first so a typo fails here, not at first read.
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("read-only open: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_journal_mode=WAL&_busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db read-only: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("read-only open ping failed: %w", err)
	}
	return &Ledger{db: db}, nil
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
	CREATE TABLE IF NOT EXISTS tenants (
		id TEXT PRIMARY KEY,
		display_name TEXT,
		admin_pubkey_hex TEXT NOT NULL DEFAULT '',
		autonomy_level TEXT NOT NULL DEFAULT 'L1',
		spend_ceiling_usd REAL NOT NULL DEFAULT 150.0,
		spent_usd REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT,
		tenant_id TEXT NOT NULL DEFAULT 'default',
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
		reserved_tenant TEXT NOT NULL DEFAULT '',
		reserved_amount REAL NOT NULL DEFAULT 0,
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
		tenant_id TEXT NOT NULL DEFAULT 'default',
		conf_label TEXT NOT NULL DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS campaign_versions (
		session_id TEXT NOT NULL,
		state_json TEXT NOT NULL,
		trigger TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);
	CREATE INDEX IF NOT EXISTS idx_campaign_versions_session ON campaign_versions(session_id);

	-- Claim records: one immutable row per evaluated claim, giving every
	-- verdict a stable per-turn record ID that citations resolve. Amplitudes
	-- use -1 for "not evaluated" (tribunal consensus has no amplitudes);
	-- only claim_eval verdicts carry measured amplitudes.
	CREATE TABLE IF NOT EXISTS claim_records (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		turn_kind TEXT NOT NULL,
		statement TEXT NOT NULL,
		alpha REAL NOT NULL DEFAULT -1,
		beta REAL NOT NULL DEFAULT -1,
		gamma REAL NOT NULL DEFAULT -1,
		verdict TEXT NOT NULL,
		evidence_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);
	CREATE INDEX IF NOT EXISTS idx_claims_session ON claim_records(session_id);

	CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_session ON jobs(session_id);
	CREATE INDEX IF NOT EXISTS idx_ledger_session ON ledger_events(session_id);
	`

	if _, err := db.Exec(initSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize ledger schema: %w", err)
	}

	// Dynamic column migrations if table existed from an earlier schema version
	for _, col := range []string{"prev_hash", "entry_hash", "signer_pubkey_hex", "signature_hex", "tenant_id", "conf_label"} {
		_, _ = db.Exec(fmt.Sprintf("ALTER TABLE ledger_events ADD COLUMN %s TEXT NOT NULL DEFAULT ''", col))
	}
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = db.Exec(`ALTER TABLE tenants ADD COLUMN spent_usd REAL NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN reserved_tenant TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN reserved_amount REAL NOT NULL DEFAULT 0`)
	// Measured generation cost on chat messages (NULL = unknown, never zero-filled).
	_, _ = db.Exec(`ALTER TABLE chat_messages ADD COLUMN duration_ms INTEGER`)
	_, _ = db.Exec(`ALTER TABLE chat_messages ADD COLUMN usage_json TEXT`)
	// Normalize pre-migration rows (ALTER ... DEFAULT '' backfills '').
	_, _ = db.Exec(`UPDATE ledger_events SET tenant_id = 'default' WHERE tenant_id = ''`)
	_, _ = db.Exec(`UPDATE sessions SET tenant_id = 'default' WHERE tenant_id = ''`)
	// Confidentiality default: every pre-label row belongs to its session's
	// lab tenant (tenant/<id>), never public. Explicit 'public' is only ever
	// written by AppendLabeledEvent callers that mean it.
	_, _ = db.Exec(`UPDATE ledger_events SET conf_label = 'tenant/' || tenant_id WHERE conf_label = ''`)

	// Seed the default lab tenant (first-write admin key bootstrap happens in
	// the tenants package; the row must exist for FK-less joins to resolve).
	_, _ = db.Exec(
		`INSERT OR IGNORE INTO tenants (id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, status, created_at) VALUES ('default', 'Default lab', '', 'L1', 150.0, 'active', CURRENT_TIMESTAMP)`,
	)

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

// resolveTenantLocked maps a session to its owning lab tenant. Unknown or
// empty sessions resolve to 'default' so pre-tenancy rows stay readable.
// Callers must hold the ledger lock (appends already hold the write lock).
func (l *Ledger) resolveTenantLocked(sessionID string) string {
	if sessionID == "" {
		return "default"
	}
	var tenant string
	if err := l.db.QueryRow(`SELECT tenant_id FROM sessions WHERE id = ?`, sessionID).Scan(&tenant); err != nil || tenant == "" {
		return "default"
	}
	return tenant
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
	tenantID := l.resolveTenantLocked(sessionID)
	query := `INSERT INTO ledger_events (timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := l.db.Exec(query, ts, sessionID, tenantID, "tenant/"+tenantID, eventType, string(data), prevHash, entryHashHex, signerPubHex, sigHex)
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
		TenantID:     tenantID,
		ConfLabel:    "tenant/" + tenantID,
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

// AppendExternalEvent writes an event signed by an external client/tenant with cryptographic hash chaining
func (l *Ledger) AppendExternalEvent(sessionID string, eventType string, payload any, pubKeyHex, sigHex string) (*EventRecord, error) {
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

	ts := time.Now().UTC().Format(time.RFC3339)
	tenantID := l.resolveTenantLocked(sessionID)
	query := `INSERT INTO ledger_events (timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := l.db.Exec(query, ts, sessionID, tenantID, "tenant/"+tenantID, eventType, string(data), prevHash, entryHashHex, pubKeyHex, sigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to insert external event: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		id = 0
	}

	return &EventRecord{
		ID:           id,
		Timestamp:    ts,
		SessionID:    sessionID,
		TenantID:     tenantID,
		ConfLabel:    "tenant/" + tenantID,
		EventType:    eventType,
		Payload:      json.RawMessage(data),
		PrevHash:     prevHash,
		EntryHash:    entryHashHex,
		SignerPubkey: pubKeyHex,
		Signature:    sigHex,
	}, nil
}

// AppendLabeledEvent writes a signed event with an explicit confidentiality
// label. Only "public" or the session tenant's own "tenant/<id>" label is
// accepted: cross-tenant labels are refused (no grant model to honor them
// yet), as are empty and deny-all labels. Derived records should compute
// their label with ifc.JoinLabels over their inputs.
func (l *Ledger) AppendLabeledEvent(sessionID string, eventType string, payload any, privKey ed25519.PrivateKey, label string) (*EventRecord, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if label != "public" {
		tenantID := l.resolveTenantLocked(sessionID)
		if label != "tenant/"+tenantID {
			return nil, fmt.Errorf("label %q not writable in this session (no cross-tenant grants): refused", label)
		}
	}

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

	tenantID := l.resolveTenantLocked(sessionID)
	ts := time.Now().UTC().Format(time.RFC3339)
	query := `INSERT INTO ledger_events (timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := l.db.Exec(query, ts, sessionID, tenantID, label, eventType, string(data), prevHash, entryHashHex, signerPubHex, sigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to insert labeled event: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		id = 0
	}

	return &EventRecord{
		ID:           id,
		Timestamp:    ts,
		SessionID:    sessionID,
		TenantID:     tenantID,
		ConfLabel:    label,
		EventType:    eventType,
		Payload:      json.RawMessage(data),
		PrevHash:     prevHash,
		EntryHash:    entryHashHex,
		SignerPubkey: signerPubHex,
		Signature:    sigHex,
	}, nil
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

	query := `SELECT id, timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex FROM ledger_events WHERE session_id = ? ORDER BY id DESC LIMIT ?`
	rows, err := l.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventRecord
	for rows.Next() {
		var e EventRecord
		var payloadStr string
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.SessionID, &e.TenantID, &e.ConfLabel, &e.EventType, &payloadStr, &e.PrevHash, &e.EntryHash, &e.SignerPubkey, &e.Signature); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payloadStr)
		events = append(events, e)
	}
	return events, nil
}

func (l *Ledger) SaveCampaignState(sessionID string, stateJSON []byte) error {
	return l.SaveCampaignStateWithTrigger(sessionID, stateJSON, "")
}

// maxCampaignVersions bounds per-session history. Versions are small JSON
// snapshots; the cap prevents unbounded growth while keeping a useful
// audit trail of how the campaign evolved.
const maxCampaignVersions = 50

// SaveCampaignStateWithTrigger upserts the live row and appends an
// immutable version row recording what triggered the write (e.g. the chat
// prompt, "tribunal", "launch"). The live row stays the fast path;
// the versions table is the audit trail — state evolution, not just state.
func (l *Ledger) SaveCampaignStateWithTrigger(sessionID string, stateJSON []byte, trigger string) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	query := `INSERT INTO campaign_states (session_id, state_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET state_json = excluded.state_json, updated_at = excluded.updated_at`
	if _, err := l.db.Exec(query, sessionID, string(stateJSON), now); err != nil {
		return err
	}
	if _, err := l.db.Exec(
		`INSERT INTO campaign_versions (session_id, state_json, trigger, created_at) VALUES (?, ?, ?, ?)`,
		sessionID, string(stateJSON), trigger, now); err != nil {
		return err
	}
	// Bound history: drop oldest beyond the cap (by rowid, never by time).
	_, _ = l.db.Exec(
		`DELETE FROM campaign_versions WHERE session_id = ? AND rowid NOT IN (
			SELECT rowid FROM campaign_versions WHERE session_id = ? ORDER BY rowid DESC LIMIT ?)`,
		sessionID, sessionID, maxCampaignVersions)
	return nil
}

// CampaignVersion is one immutable snapshot of a session's campaign.
type CampaignVersion struct {
	Version   int64  `json:"version"`
	StateJSON string `json:"state_json"`
	Trigger   string `json:"trigger"`
	CreatedAt string `json:"created_at"`
}

// GetCampaignVersions returns the session's campaign history, oldest first.
func (l *Ledger) GetCampaignVersions(sessionID string, limit int) ([]CampaignVersion, error) {
	if limit <= 0 || limit > maxCampaignVersions {
		limit = maxCampaignVersions
	}
	l.lock.RLock()
	defer l.lock.RUnlock()

	rows, err := l.db.Query(
		`SELECT rowid, state_json, trigger, created_at FROM campaign_versions WHERE session_id = ? ORDER BY rowid ASC LIMIT ?`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampaignVersion
	for rows.Next() {
		var v CampaignVersion
		if err := rows.Scan(&v.Version, &v.StateJSON, &v.Trigger, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ClaimRecord is one immutable evaluated-claim row. Amplitudes of -1 mean
// "not evaluated" — only claim_eval verdicts carry measured amplitudes;
// tribunal consensus records carry the verdict alone.
type ClaimRecord struct {
	ID        string  `json:"id"`
	SessionID string  `json:"session_id"`
	TurnKind  string  `json:"turn_kind"`
	Statement string  `json:"statement"`
	Alpha     float64 `json:"alpha"`
	Beta      float64 `json:"beta"`
	Gamma     float64 `json:"gamma"`
	Verdict   string  `json:"verdict"`
	Evidence  string  `json:"evidence_json"`
	CreatedAt string  `json:"created_at"`
}

// MintClaimRecord stores one evaluated claim and returns its stable record
// ID: clm-<session8>-<seq04>, ordered per session. The sequence counts under
// the write lock so concurrent turns cannot mint the same ID twice.
func (l *Ledger) MintClaimRecord(sessionID, turnKind, statement string, alpha, beta, gamma float64, verdict, evidenceJSON string) (*ClaimRecord, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(statement) == "" || strings.TrimSpace(verdict) == "" {
		return nil, fmt.Errorf("claim record needs session, statement, and verdict")
	}
	if evidenceJSON == "" {
		evidenceJSON = "{}"
	}
	l.lock.Lock()
	defer l.lock.Unlock()

	var seq int
	if err := l.db.QueryRow(`SELECT COUNT(*) FROM claim_records WHERE session_id = ?`, sessionID).Scan(&seq); err != nil {
		return nil, err
	}
	seq++
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	id := fmt.Sprintf("clm-%s-%04d", short, seq)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := l.db.Exec(
		`INSERT INTO claim_records (id, session_id, turn_kind, statement, alpha, beta, gamma, verdict, evidence_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, turnKind, statement, alpha, beta, gamma, verdict, evidenceJSON, now); err != nil {
		return nil, err
	}
	return &ClaimRecord{ID: id, SessionID: sessionID, TurnKind: turnKind, Statement: statement,
		Alpha: alpha, Beta: beta, Gamma: gamma, Verdict: verdict, Evidence: evidenceJSON, CreatedAt: now}, nil
}

// ListClaimRecords returns a session's claim history, newest first.
func (l *Ledger) ListClaimRecords(sessionID string, limit int) ([]ClaimRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	l.lock.RLock()
	defer l.lock.RUnlock()

	rows, err := l.db.Query(
		`SELECT id, session_id, turn_kind, statement, alpha, beta, gamma, verdict, evidence_json, created_at
		 FROM claim_records WHERE session_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClaimRecord
	for rows.Next() {
		var c ClaimRecord
		if err := rows.Scan(&c.ID, &c.SessionID, &c.TurnKind, &c.Statement, &c.Alpha, &c.Beta, &c.Gamma, &c.Verdict, &c.Evidence, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
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
