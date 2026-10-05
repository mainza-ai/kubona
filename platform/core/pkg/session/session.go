package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

type Session struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
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
}

type Manager struct {
	ledger *ledger.Ledger
}

func NewManager(l *ledger.Ledger) *Manager {
	return &Manager{ledger: l}
}

func (m *Manager) CreateSession(title string, spendLimitUSD float64) (*Session, error) {
	if spendLimitUSD <= 0 {
		spendLimitUSD = 100.0 // Default budget mandate
	}
	s := &Session{
		ID:              uuid.New().String(),
		Title:           title,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		Status:          "active",
		SpendLimitUSD:   spendLimitUSD,
		CurrentSpendUSD: 0.0,
	}

	query := `INSERT INTO sessions (id, title, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := m.ledger.DB().Exec(query, s.ID, s.Title, s.CreatedAt, s.UpdatedAt, s.Status, s.SpendLimitUSD, s.CurrentSpendUSD)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	m.ledger.AppendEvent(s.ID, "session_created", s)
	return s, nil
}

func (m *Manager) GetSession(id string) (*Session, error) {
	query := `SELECT id, title, created_at, updated_at, status, spend_limit_usd, current_spend_usd FROM sessions WHERE id = ?`
	row := m.ledger.DB().QueryRow(query, id)

	var s Session
	if err := row.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt, &s.Status, &s.SpendLimitUSD, &s.CurrentSpendUSD); err != nil {
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
	query := `SELECT id, title, created_at, updated_at, status, spend_limit_usd, current_spend_usd FROM sessions ORDER BY updated_at DESC LIMIT ?`
	rows, err := m.ledger.DB().Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt, &s.Status, &s.SpendLimitUSD, &s.CurrentSpendUSD); err != nil {
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
	query := `SELECT id, session_id, role, content, proposal_json, created_at FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC`
	rows, err := m.ledger.DB().Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		var propStr sql.NullString
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &propStr, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if propStr.Valid {
			raw := json.RawMessage(propStr.String)
			msg.ProposalJSON = &raw
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func (m *Manager) DeleteSession(id string) error {
	tx, err := m.ledger.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM chat_messages WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM campaign_states WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	m.ledger.AppendEvent(id, "session_deleted", map[string]string{"id": id})
	return nil
}
