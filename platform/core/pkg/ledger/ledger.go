package ledger

import (
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	// lockFile holds an exclusive flock on <dbPath>.lock for the ledger's
	// lifetime: exactly one read-write daemon per state directory. A second
	// OpenLedger on the same path refuses instead of split-braining the
	// in-memory planes (job queue, sessions, hindsight) that SQLite alone
	// cannot reconcile. The lock releases on process exit — no stale
	// cleanup, no pidfiles. Read-only handles never take it.
	lockFile *os.File
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
	// Single-writer guard: refuse a second read-write daemon on this state
	// directory instead of running split-brain. SQLite serializes writes,
	// but the job queue, session cache, and memory store live in-process —
	// two daemons would dispatch twice and remember separately.
	lockFile, err := os.OpenFile(dbPath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open state lock: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("state directory locked by another daemon (%s): stop it first — refusing second writer", dbPath)
	}
	// Open with SQLite WAL mode and busy timeout for high-throughput concurrent writes
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		releaseLock(lockFile)
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
		compiler_used TEXT NOT NULL DEFAULT '',
		fallback_estimate INTEGER NOT NULL DEFAULT 0,
		mandate_ref TEXT NOT NULL DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS canvas_layouts (
		session_id TEXT PRIMARY KEY,
		layout_json TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS custom_agents (
		id TEXT PRIMARY KEY,
		manifest_json TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS metasearch_proposals (
		id TEXT PRIMARY KEY,
		proposal_json TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_session ON jobs(session_id);
	CREATE INDEX IF NOT EXISTS idx_ledger_session ON ledger_events(session_id);

	-- Idempotency receipts (S1): content-bound deduplication keys for spend
	-- mutations. One row per (session, request_id): the first execution
	-- claims IN_PROGRESS, then records its terminal outcome. Retries replay
	-- the stored receipt byte-identical; same-ID-different-content 409s.
	-- Spend receipts are audit: no TTL, never deleted.
	CREATE TABLE IF NOT EXISTS idempotency_keys (
		session_id TEXT NOT NULL,
		request_id TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		rail TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'IN_PROGRESS',
		status_code INTEGER NOT NULL DEFAULT 0,
		receipt_kind TEXT NOT NULL DEFAULT '',
		receipt_body TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, request_id)
	);
	` + obligationSchema + scoresSchema

	if _, err := db.Exec(initSchema); err != nil {
		db.Close()
		releaseLock(lockFile)
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
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN compiler_used TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN fallback_estimate INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN mandate_ref TEXT NOT NULL DEFAULT ''`)
	// Measured generation cost on chat messages (NULL = unknown, never zero-filled).
	_, _ = db.Exec(`ALTER TABLE chat_messages ADD COLUMN duration_ms INTEGER`)
	_, _ = db.Exec(`ALTER TABLE chat_messages ADD COLUMN usage_json TEXT`)
	// S3: loop lifecycle persistence. The loops table previously had no
	// writer (engines lived in memory only); sleep state, drain flags, and
	// the I3-reserved spend column persist here.
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN harness_id TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN last_activity DATETIME`)
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN drain INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN reserved_amount REAL NOT NULL DEFAULT 0`)
	// I3: per-iteration loop accounting — who funded the reservation and
	// how much of it the iterations have consumed (drawdown). Release at
	// loop end returns reserved - consumed to reserved_tenant.
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN reserved_tenant TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE loops ADD COLUMN consumed_amount REAL NOT NULL DEFAULT 0`)
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

	return &Ledger{db: db, lockFile: lockFile}, nil
}

// releaseLock drops a state-dir flock best-effort (open-failure paths).
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}

func (l *Ledger) DB() *sql.DB {
	return l.db
}

func (l *Ledger) getLatestEntryHashLocked(sessionID string) string {
	// W2.13 design (not a bug): chains are PER-SESSION — prev_hash links
	// the latest event of the same session_id, not the global tip. A
	// global chain would serialize every tenant's writes through one tip
	// and let one session's fork invalidate another's; per-session chains
	// keep each lab's audit trail independent (VerifyChain audits one
	// session or all sessions chain-by-chain). The empty-session ("")
	// branch covers pre-tenancy/global-plane rows only.
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
	// S6 closed taxonomy: unregistered types refuse here, so a typo'd
	// type can never silently fork the audit trail. Register new types
	// in events.go (KnownEventTypes) with their payload contract.
	if !IsKnownEventType(eventType) {
		return nil, fmt.Errorf("unregistered event_type %q: add it to KnownEventTypes (pkg/ledger/events.go) with its payload contract", eventType)
	}
	l.lock.Lock()
	defer l.lock.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	prevHash := l.getLatestEntryHashLocked(sessionID)
	digest := chainDigestBytes(prevHash, sessionID, eventType, string(data))
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

// CanonicalHash (I1, single source) hashes a value over sorted-key JSON,
// so digests are stable across key order and process restarts. Used by the
// idempotency guard and the session-migration manifest — never a second
// copy elsewhere.
func CanonicalHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("content not hashable: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Idempotency rail statuses (S1). Keys move IN_PROGRESS -> COMPLETE exactly
// once; a COMPLETE row is terminal and replays forever.
const (
	IdemStatusInProgress = "IN_PROGRESS"
	IdemStatusComplete   = "COMPLETE"
)

// IdempotencyRecord is one claimed request key with its terminal receipt.
type IdempotencyRecord struct {
	SessionID   string
	RequestID   string
	ContentHash string
	Rail        string
	Status      string
	StatusCode  int
	ReceiptKind string // "json" (raw body bytes) or "text" (http.Error message)
	ReceiptBody string
	CreatedAt   string
	UpdatedAt   string
}

// IdemLookup returns the key row, or found=false when no execution has
// claimed this (session, request) pair yet.
func (l *Ledger) IdemLookup(sessionID, requestID string) (IdempotencyRecord, bool, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	var rec IdempotencyRecord
	err := l.db.QueryRow(
		`SELECT session_id, request_id, content_hash, rail, status, status_code, receipt_kind, receipt_body, created_at, updated_at FROM idempotency_keys WHERE session_id = ? AND request_id = ?`,
		sessionID, requestID).Scan(
		&rec.SessionID, &rec.RequestID, &rec.ContentHash, &rec.Rail, &rec.Status,
		&rec.StatusCode, &rec.ReceiptKind, &rec.ReceiptBody, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("idempotency lookup failed: %w", err)
	}
	return rec, true, nil
}

// IdemClaim inserts an IN_PROGRESS row. A primary-key conflict means a
// duplicate is already claimed (or completed): the caller must re-read via
// IdemLookup instead of executing.
//
// W2.11: only a PRIMARY-KEY conflict means "already claimed". Any other
// insert error (disk, schema, NOT NULL) used to fall into the re-read and
// come back as "already claimed" whenever a row happened to exist —
// masking the real failure. Non-PK errors now surface wrapped.
func (l *Ledger) IdemClaim(sessionID, requestID, contentHash, rail string) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	_, err := l.db.Exec(
		`INSERT INTO idempotency_keys (session_id, request_id, content_hash, rail, status, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sessionID, requestID, contentHash, rail, IdemStatusInProgress)
	if err == nil {
		return nil
	}
	if !isPrimaryKeyConflict(err) {
		return fmt.Errorf("idempotency claim failed: %w", err)
	}
	var existing IdempotencyRecord
	qerr := l.db.QueryRow(
		`SELECT session_id, request_id, content_hash, rail, status, status_code, receipt_kind, receipt_body, created_at, updated_at FROM idempotency_keys WHERE session_id = ? AND request_id = ?`,
		sessionID, requestID).Scan(
		&existing.SessionID, &existing.RequestID, &existing.ContentHash, &existing.Rail, &existing.Status,
		&existing.StatusCode, &existing.ReceiptKind, &existing.ReceiptBody, &existing.CreatedAt, &existing.UpdatedAt)
	if qerr == sql.ErrNoRows {
		return fmt.Errorf("idempotency claim failed: %w", err)
	}
	if qerr != nil {
		return fmt.Errorf("idempotency claim failed: %w", err)
	}
	return fmt.Errorf("idempotency key already claimed")
}

// isPrimaryKeyConflict reports a SQLite PRIMARY KEY violation (W2.11).
// mattn exposes Code()/ExtendedCode() (19 = CONSTRAINT, 1555 = PRIMARYKEY);
// anything else — disk, schema, NOT NULL — is a real failure, never a race.
func isPrimaryKeyConflict(err error) bool {
	var ec interface{ ExtendedCode() int }
	if errors.As(err, &ec) {
		if code := ec.ExtendedCode(); code != 1555 && code != 19 {
			return false
		}
		return strings.Contains(err.Error(), "UNIQUE constraint failed")
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// IdemComplete records the terminal outcome. It only transitions
// IN_PROGRESS rows: completing an already-completed (or reclaimed) key is a
// no-op returning found=false, so a leader orphaned by a reclaim never
// overwrites the reclaimer's receipt.
func (l *Ledger) IdemComplete(sessionID, requestID string, statusCode int, receiptKind, receiptBody string) (bool, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	res, err := l.db.Exec(
		`UPDATE idempotency_keys SET status = ?, status_code = ?, receipt_kind = ?, receipt_body = ?, updated_at = CURRENT_TIMESTAMP WHERE session_id = ? AND request_id = ? AND status = ?`,
		IdemStatusComplete, statusCode, receiptKind, receiptBody, sessionID, requestID, IdemStatusInProgress)
	if err != nil {
		return false, fmt.Errorf("idempotency complete failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("idempotency complete failed: %w", err)
	}
	return n == 1, nil
}

// IdemDelete removes a key row (reclaim of a stale IN_PROGRESS claim, or
// unwinding a claim whose execution result could not be sealed).
func (l *Ledger) IdemDelete(sessionID, requestID string) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	_, err := l.db.Exec(`DELETE FROM idempotency_keys WHERE session_id = ? AND request_id = ?`, sessionID, requestID)
	if err != nil {
		return fmt.Errorf("idempotency delete failed: %w", err)
	}
	return nil
}

// AppendExternalEvent writes an event signed by an external client/tenant with cryptographic hash chaining
func (l *Ledger) AppendExternalEvent(sessionID string, eventType string, payload any, pubKeyHex, sigHex string) (*EventRecord, error) {
	if !IsKnownEventType(eventType) {
		return nil, fmt.Errorf("unregistered event_type %q: add it to KnownEventTypes (pkg/ledger/events.go) with its payload contract", eventType)
	}
	l.lock.Lock()
	defer l.lock.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	prevHash := l.getLatestEntryHashLocked(sessionID)
	digest := chainDigestBytes(prevHash, sessionID, eventType, string(data))
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
	if !IsKnownEventType(eventType) {
		return nil, fmt.Errorf("unregistered event_type %q: add it to KnownEventTypes (pkg/ledger/events.go) with its payload contract", eventType)
	}
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
	digest := chainDigestBytes(prevHash, sessionID, eventType, string(data))
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
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key: %d bytes (want %d)", len(pubBytes), ed25519.PublicKeySize)
	}

	sigBytes, err := hex.DecodeString(event.Signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature size: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature size: %d bytes (want %d)", len(sigBytes), ed25519.SignatureSize)
	}

	digest := chainDigestBytes(event.PrevHash, event.SessionID, event.EventType, string(event.Payload))

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

// LoopPersist is the persisted projection of an autoresearch loop (S3).
// Engines live in memory; this row is the restart-surviving truth.
// ReservedAmount/ReservedTenant/ConsumedAmount are the I3 spend books:
// reserved_total upfront, consumed drawn per iteration, released back to
// reserved_tenant (reserved - consumed) at loop end.
type LoopPersist struct {
	ID             string
	SessionID      string
	HarnessID      string
	Status         string
	StepCount      int
	MaxSteps       int
	LastActivity   time.Time
	Drain          bool
	ReservedAmount float64
	ReservedTenant string
	ConsumedAmount float64
	UpdatedAt      time.Time
}

// SaveLoopState upserts a loop row (I3 spend books included: reserved
// total, funding tenant, consumed drawdown).
func (l *Ledger) SaveLoopState(p LoopPersist) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	drain := 0
	if p.Drain {
		drain = 1
	}
	_, err := l.db.Exec(`INSERT INTO loops (id, session_id, harness_id, status, step_count, max_steps, diverged, created_at, updated_at, last_activity, drain, reserved_amount, reserved_tenant, consumed_amount)
		VALUES (?, ?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, step_count=excluded.step_count, max_steps=excluded.max_steps,
		updated_at=CURRENT_TIMESTAMP, last_activity=excluded.last_activity, drain=excluded.drain, reserved_amount=excluded.reserved_amount,
		reserved_tenant=excluded.reserved_tenant, consumed_amount=excluded.consumed_amount`,
		p.ID, p.SessionID, p.HarnessID, p.Status, p.StepCount, p.MaxSteps, p.LastActivity.UTC().Format(time.RFC3339), drain, p.ReservedAmount, p.ReservedTenant, p.ConsumedAmount)
	if err != nil {
		return fmt.Errorf("save loop state failed: %w", err)
	}
	return nil
}

// ListLoopStates returns all persisted loop rows (empty table = no error).
func (l *Ledger) ListLoopStates() ([]LoopPersist, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	rows, err := l.db.Query(`SELECT id, session_id, harness_id, status, step_count, max_steps, last_activity, drain, reserved_amount, reserved_tenant, consumed_amount, updated_at FROM loops ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoopPersist
	for rows.Next() {
		var p LoopPersist
		var lastAct, updated sql.NullString
		var drain int
		if err := rows.Scan(&p.ID, &p.SessionID, &p.HarnessID, &p.Status, &p.StepCount, &p.MaxSteps, &lastAct, &drain, &p.ReservedAmount, &p.ReservedTenant, &p.ConsumedAmount, &updated); err != nil {
			return nil, err
		}
		p.Drain = drain != 0
		if lastAct.Valid {
			if t, err := time.Parse(time.RFC3339, lastAct.String); err == nil {
				p.LastActivity = t
			}
		}
		if updated.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updated.String); err == nil {
				p.UpdatedAt = t
			} else if t, err := time.Parse(time.RFC3339, updated.String); err == nil {
				p.UpdatedAt = t
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// ReconcileLoopsOnBoot marks every non-terminal persisted loop sleeping:
// engines are in-memory, so nothing can be running after a restart.
// Terminal states are untouched — the loops table has two writers with
// two vocabularies (autoresearch engine + supervisor), so the skip set
// must cover both: completed/aborted (both writers) + killed
// (supervisor-only). Sleeping rows are already settled, never re-swept.
// Canonical single source: session.TerminalLoopStatuses
// (pkg/session/migrate.go) — parity enforced by
// TestReconcileSkipsAllTerminalLoopStatuses in the session package
// (ledger cannot import session/supervisor: both import ledger).
func (l *Ledger) ReconcileLoopsOnBoot() (int, error) {
	rows, err := l.ListLoopStates()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range rows {
		if p.Status == "completed" || p.Status == "aborted" || p.Status == "killed" || p.Status == "sleeping" {
			continue
		}
		p.Status = "sleeping"
		if err := l.SaveLoopState(p); err != nil {
			return n, err
		}
		_ = l.AppendEvent(p.SessionID, "autoresearch_loop_reconciled", map[string]any{
			"loop_id": p.ID, "status": "sleeping", "reason": "daemon restart: in-memory engine gone",
		})
		n++
	}
	return n, nil
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

// VerifyCampaignHistory asserts the live row equals the newest version.
// Every writer funnels through SaveCampaignStateWithTrigger, so divergence
// means a bypass (or a partial failure between the two writes) — fail
// loudly here instead of drifting silently. Sessions with no campaign at
// all verify cleanly (nothing to diverge).
func (l *Ledger) VerifyCampaignHistory(sessionID string) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	var live []byte
	err := l.db.QueryRow(`SELECT state_json FROM campaign_states WHERE session_id = ?`, sessionID).Scan(&live)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var newest []byte
	var trigger string
	err = l.db.QueryRow(
		`SELECT state_json, trigger FROM campaign_versions WHERE session_id = ? ORDER BY rowid DESC LIMIT 1`,
		sessionID).Scan(&newest, &trigger)
	if err == sql.ErrNoRows {
		return fmt.Errorf("campaign drift: live row exists with no versions for session %s", shortID(sessionID))
	}
	if err != nil {
		return err
	}
	if string(live) != string(newest) {
		return fmt.Errorf("campaign drift: live row != newest version (trigger %q) for session %s", trigger, shortID(sessionID))
	}
	return nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
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
	err := l.db.Close()
	releaseLock(l.lockFile)
	l.lockFile = nil
	return err
}
