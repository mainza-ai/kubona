package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// ErrInvalidAmount refuses negative/NaN/Inf spend (W0.7, mirrored from the
// tenant manager: one rule, two books).
var ErrInvalidAmount = fmt.Errorf("spend amount must be a finite number >= 0: negative/NaN/Inf refuse")

// validAmount rejects negative/NaN/Inf spend: NaN comparisons are all
// false, so an unvalidated NaN sails through every limit check.
func validAmount(amountUSD float64) error {
	if math.IsNaN(amountUSD) || math.IsInf(amountUSD, 0) || amountUSD < 0 {
		return ErrInvalidAmount
	}
	return nil
}

// ErrContentMismatch is returned when expected_content CAS fails (re-read
// and retry). Handlers map it to 409.
var ErrContentMismatch = fmt.Errorf("message changed since read: expected_content mismatch (concurrent edit — re-read and retry)")

type Session struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	TenantID        string    `json:"tenant_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Status          string    `json:"status"` // "active", "idle", "archived"
	SpendLimitUSD   float64   `json:"spend_limit_usd"`
	CurrentSpendUSD float64   `json:"current_spend_usd"`
}

type ChatMessage struct {
	ID           string           `json:"id"`
	SessionID    string           `json:"session_id"`
	Role         string           `json:"role"` // "user", "assistant", "system"
	Content      string           `json:"content"`
	ProposalJSON *json.RawMessage `json:"proposal_json,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	// Measured generation cost, persisted when the serving path reports it
	// (streaming usage chunks / unary timing). Nil when unknown — never estimated.
	DurationMs *int64           `json:"duration_ms,omitempty"`
	UsageJSON  *json.RawMessage `json:"usage_json,omitempty"`
}

type Manager struct {
	ledger *ledger.Ledger
}

func NewManager(l *ledger.Ledger) *Manager {
	return &Manager{ledger: l}
}

func (m *Manager) CreateSession(title string, spendLimitUSD float64, tenantID string) (*Session, error) {
	if spendLimitUSD <= 0 {
		spendLimitUSD = 100.0 // Default budget mandate
	}
	if tenantID == "" {
		tenantID = "default"
	}
	var exists string
	if err := m.ledger.DB().QueryRow(`SELECT id FROM tenants WHERE id = ?`, tenantID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("unknown tenant %q: sessions must belong to a registered lab tenant", tenantID)
	}
	s := &Session{
		ID:              uuid.New().String(),
		Title:           title,
		TenantID:        tenantID,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		Status:          "active",
		SpendLimitUSD:   spendLimitUSD,
		CurrentSpendUSD: 0.0,
	}

	query := `INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := m.ledger.DB().Exec(query, s.ID, s.Title, s.TenantID, s.CreatedAt, s.UpdatedAt, s.Status, s.SpendLimitUSD, s.CurrentSpendUSD)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	m.ledger.AppendEvent(s.ID, "session_created", s)
	return s, nil
}

// EnsureSessionWithID returns the session with the exact id, creating it
// under the given tenant when absent. Used for deterministic system sessions
// (e.g. per-tenant A2A delegation) whose ids are derived, not minted.
func (m *Manager) EnsureSessionWithID(id, title string, spendLimitUSD float64, tenantID string) (*Session, error) {
	if s, err := m.GetSession(id); err == nil {
		return s, nil
	}
	if spendLimitUSD <= 0 {
		spendLimitUSD = 100.0
	}
	if tenantID == "" {
		tenantID = "default"
	}
	var exists string
	if err := m.ledger.DB().QueryRow(`SELECT id FROM tenants WHERE id = ?`, tenantID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("unknown tenant %q: sessions must belong to a registered lab tenant", tenantID)
	}
	now := time.Now().UTC()
	s := &Session{
		ID:              id,
		Title:           title,
		TenantID:        tenantID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Status:          "active",
		SpendLimitUSD:   spendLimitUSD,
		CurrentSpendUSD: 0.0,
	}
	query := `INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := m.ledger.DB().Exec(query, s.ID, s.Title, s.TenantID, s.CreatedAt, s.UpdatedAt, s.Status, s.SpendLimitUSD, s.CurrentSpendUSD); err != nil {
		// Lost a creation race: re-read whatever is there now.
		if existing, rerr := m.GetSession(id); rerr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	m.ledger.AppendEvent(s.ID, "session_created", s)
	return s, nil
}

func (m *Manager) GetSession(id string) (*Session, error) {
	query := `SELECT id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd FROM sessions WHERE id = ?`
	row := m.ledger.DB().QueryRow(query, id)

	var s Session
	if err := row.Scan(&s.ID, &s.Title, &s.TenantID, &s.CreatedAt, &s.UpdatedAt, &s.Status, &s.SpendLimitUSD, &s.CurrentSpendUSD); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, err
	}
	return &s, nil
}

func (m *Manager) ListSessions(limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd FROM sessions ORDER BY updated_at DESC LIMIT ?`
	rows, err := m.ledger.DB().Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.TenantID, &s.CreatedAt, &s.UpdatedAt, &s.Status, &s.SpendLimitUSD, &s.CurrentSpendUSD); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (m *Manager) AddMessage(sessionID, role, content string, proposal *json.RawMessage) (*ChatMessage, error) {
	msg := &ChatMessage{
		ID:           uuid.New().String(),
		SessionID:    sessionID,
		Role:         role,
		Content:      content,
		ProposalJSON: proposal,
		CreatedAt:    time.Now().UTC(),
	}

	var propStr sql.NullString
	if proposal != nil {
		propStr = sql.NullString{String: string(*proposal), Valid: true}
	}

	query := `INSERT INTO chat_messages (id, session_id, role, content, proposal_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := m.ledger.DB().Exec(query, msg.ID, msg.SessionID, msg.Role, msg.Content, propStr, msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert chat message: %w", err)
	}

	m.ledger.DB().Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, msg.CreatedAt, sessionID)
	m.ledger.AppendEvent(sessionID, "chat_message_appended", msg)
	return msg, nil
}

func (m *Manager) GetMessages(sessionID string) ([]ChatMessage, error) {
	query := `SELECT id, session_id, role, content, proposal_json, created_at, duration_ms, usage_json FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC`
	rows, err := m.ledger.DB().Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		var propStr sql.NullString
		var durMs sql.NullInt64
		var usageStr sql.NullString
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &propStr, &msg.CreatedAt, &durMs, &usageStr); err != nil {
			return nil, err
		}
		if propStr.Valid {
			raw := json.RawMessage(propStr.String)
			msg.ProposalJSON = &raw
		}
		if durMs.Valid {
			v := durMs.Int64
			msg.DurationMs = &v
		}
		if usageStr.Valid {
			raw := json.RawMessage(usageStr.String)
			msg.UsageJSON = &raw
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// ClearMessages deletes every chat message in a session (operator "clear
// chat"). The session, campaign state, and ledger history survive; the clear
// itself is sealed as a ledger event so the erasure is auditable.
func (m *Manager) ClearMessages(sessionID string) (int64, error) {
	res, err := m.ledger.DB().Exec(`DELETE FROM chat_messages WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to clear chat messages: %w", err)
	}
	n, _ := res.RowsAffected()
	m.ledger.AppendEvent(sessionID, "chat_cleared", map[string]any{"deleted": n})
	return n, nil
}

// DropTrailingAssistant deletes assistant messages after the last user
// message and returns that user message's content. Powers regenerate
// without duplicating the user turn. Returns an error when no user message
// exists to regenerate from.
func (m *Manager) DropTrailingAssistant(sessionID string) (string, error) {
	msgs, err := m.GetMessages(sessionID)
	if err != nil {
		return "", err
	}
	lastUser := -1
	for i, msg := range msgs {
		if msg.Role == "user" {
			lastUser = i
		}
	}
	if lastUser == -1 {
		return "", fmt.Errorf("no user message to regenerate from")
	}
	if _, err := m.ledger.DB().Exec(
		`DELETE FROM chat_messages WHERE session_id = ? AND role = 'assistant' AND created_at > (SELECT created_at FROM chat_messages WHERE session_id = ? AND role = 'user' ORDER BY created_at DESC LIMIT 1)`,
		sessionID, sessionID); err != nil {
		return "", fmt.Errorf("failed to drop trailing assistant messages: %w", err)
	}
	return msgs[lastUser].Content, nil
}

// RewriteUserMessage replaces a user message's content and deletes every
// message after it (branching: the edited turn restarts the thread from
// that point). Only role='user' rows are rewritable; anything else refuses.
// expectedContent, when non-empty, is a compare-and-swap guard: mismatch
// 409s at the handler (concurrent edits last-writer-win silently otherwise).
// Returns the updated message.
func (m *Manager) RewriteUserMessage(sessionID, messageID, content, expectedContent string) (*ChatMessage, error) {
	if content == "" {
		return nil, fmt.Errorf("rewritten content must not be empty")
	}
	var role, created string
	var current string
	err := m.ledger.DB().QueryRow(
		`SELECT role, created_at, content FROM chat_messages WHERE id = ? AND session_id = ?`,
		messageID, sessionID).Scan(&role, &created, &current)
	if err != nil {
		return nil, fmt.Errorf("message not found in session")
	}
	if role != "user" {
		return nil, fmt.Errorf("only user messages can be rewritten")
	}
	if expectedContent != "" && current != expectedContent {
		return nil, ErrContentMismatch
	}
	tx, err := m.ledger.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE chat_messages SET content = ? WHERE id = ?`, content, messageID); err != nil {
		return nil, fmt.Errorf("failed to rewrite message: %w", err)
	}
	// Positional truncation (not timestamp comparison: same-second inserts
	// share created_at granularity, so time comparison under-deletes).
	rows, err := tx.Query(`SELECT id FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC, rowid ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var mid string
		if err := rows.Scan(&mid); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, mid)
	}
	rows.Close()
	if rerr := rows.Err(); rerr != nil {
		return nil, rerr
	}
	cut := false
	for _, mid := range ids {
		if mid == messageID {
			cut = true
			continue
		}
		if cut {
			if _, err := tx.Exec(`DELETE FROM chat_messages WHERE id = ?`, mid); err != nil {
				return nil, fmt.Errorf("failed to truncate thread: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	m.ledger.AppendEvent(sessionID, "chat_rewritten", map[string]any{"message_id": messageID})
	var msg ChatMessage
	var propStr sql.NullString
	var durMs sql.NullInt64
	var usageStr sql.NullString
	err = m.ledger.DB().QueryRow(
		`SELECT id, session_id, role, content, proposal_json, created_at, duration_ms, usage_json FROM chat_messages WHERE id = ?`,
		messageID).Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &propStr, &msg.CreatedAt, &durMs, &usageStr)
	if err != nil {
		return nil, err
	}
	if propStr.Valid {
		raw := json.RawMessage(propStr.String)
		msg.ProposalJSON = &raw
	}
	return &msg, nil
}

// SetMessageUsage persists measured generation cost on a stored message.
// Duration of 0 / nil usage means "unknown" and stores NULL — never zeros
// presented as measurements.
func (m *Manager) SetMessageUsage(messageID string, durationMs int64, usage *json.RawMessage) error {
	var dur any
	if durationMs > 0 {
		dur = durationMs
	}
	var usageStr any
	if usage != nil {
		usageStr = string(*usage)
	}
	_, err := m.ledger.DB().Exec(
		`UPDATE chat_messages SET duration_ms = ?, usage_json = ? WHERE id = ?`,
		dur, usageStr, messageID)
	return err
}

func (m *Manager) DeleteSession(id string) error {
	// I4 fail-closed cascade (S5): refuse while live work or money trails
	// exist; else delete ALL session rows (previously only chat +
	// campaign_states + sessions, orphaning jobs/loops/events/claims/
	// versions/layouts/keys).
	if blockers, err := m.DeleteBlockers(id); err != nil {
		return err
	} else if len(blockers) > 0 {
		return fmt.Errorf("session %s not deleted: %s (settle or abort first)", id, strings.Join(blockers, "; "))
	}
	tx, err := m.ledger.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"chat_messages", "campaign_states", "campaign_versions", "claim_records", "canvas_layouts", "jobs", "loops", "ledger_events", "idempotency_keys"} {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE session_id = ?`, table), id); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	// Deleting nothing is not a deletion: callers (and the ledger receipt
	// below) must not report success for an id that never existed.
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	m.ledger.AppendEvent(id, "session_deleted", map[string]string{"id": id})
	return nil
}

// ReserveSpend atomically reserves amountUSD against the session's spend
// limit (same CAS discipline as the tenant ceiling). A limit <= 0 means
// uncapped — sessions created without a budget keep working exactly as
// before enforcement existed. Sealed as a ledger event on success.
func (m *Manager) ReserveSpend(sessionID string, amountUSD float64) error {
	if err := validAmount(amountUSD); err != nil {
		return err
	}
	if amountUSD == 0 {
		return nil
	}
	res, err := m.ledger.DB().Exec(
		`UPDATE sessions SET current_spend_usd = current_spend_usd + ? WHERE id = ? AND (spend_limit_usd <= 0 OR current_spend_usd + ? <= spend_limit_usd)`,
		amountUSD, sessionID, amountUSD)
	if err != nil {
		return fmt.Errorf("session spend reservation failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("session spend reservation failed: %w", err)
	}
	if n == 0 {
		var exists string
		if err := m.ledger.DB().QueryRow(`SELECT id FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
			return fmt.Errorf("unknown session %q: dispatch refused", sessionID)
		}
		return fmt.Errorf("session %q spend limit exceeded: dispatch refused", sessionID)
	}
	m.ledger.AppendEvent(sessionID, "session_spend_reserved", map[string]any{
		"session_id": sessionID, "amount_usd": amountUSD})
	return nil
}

// ReleaseSpend returns a session reservation (terminal-failed jobs only).
// Same W0.7 discipline as the tenant book: atomic guarded subtract,
// over-release refuses without mutating (float-dust epsilon + MAX(0,...)
// absorb representation error only), refusal seals a
// session_spend_release_refused audit event. Unknown sessions refuse.
func (m *Manager) ReleaseSpend(sessionID string, amountUSD float64) error {
	if err := validAmount(amountUSD); err != nil {
		return err
	}
	if amountUSD == 0 {
		return nil
	}
	res, err := m.ledger.DB().Exec(
		`UPDATE sessions SET current_spend_usd = MAX(0, current_spend_usd - ?) WHERE id = ? AND current_spend_usd + 1e-9 >= ?`,
		amountUSD, sessionID, amountUSD)
	if err != nil {
		return fmt.Errorf("session spend release failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("session spend release failed: %w", err)
	}
	if n == 1 {
		m.ledger.AppendEvent(sessionID, "session_spend_released", map[string]any{
			"session_id": sessionID, "amount_usd": amountUSD})
		return nil
	}
	var exists string
	if err := m.ledger.DB().QueryRow(`SELECT id FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		return fmt.Errorf("unknown session %q: dispatch refused", sessionID)
	}
	m.ledger.AppendEvent(sessionID, "session_spend_release_refused", map[string]any{
		"session_id": sessionID, "amount_usd": amountUSD,
		"reason": "release exceeds recorded spend: caller bug, books untouched",
	})
	return fmt.Errorf("session spend release of %.4f exceeds recorded spend: refused, books untouched", amountUSD)
}
