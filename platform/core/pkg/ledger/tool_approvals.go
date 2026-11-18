package ledger

// One-time tool approvals (AG-21b).
//
// A session policy can mark a copilot tool `ask`. That must mean "hold this
// exact call for a human", not "silently allow" or "refuse forever". A request
// binds the tool name and a canonical digest of its arguments; an operator or
// the session owner approves or denies it; an approval is consumed exactly once
// by a dispatch whose arguments hash to the same digest. Denying never touches
// a different call shape, and an expired grant cannot be spent.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const toolApprovalsSchema = `
CREATE TABLE IF NOT EXISTS tool_approvals (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tool TEXT NOT NULL,
	args_hash TEXT NOT NULL,
	args_json TEXT NOT NULL DEFAULT '{}',
	args_truncated INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'pending',
	reason TEXT NOT NULL DEFAULT '',
	decided_by TEXT NOT NULL DEFAULT '',
	requested_at TEXT NOT NULL,
	decided_at TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	used_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_session
	ON tool_approvals(session_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_grant
	ON tool_approvals(session_id, tool, args_hash, status);`

const (
	ToolApprovalPending  = "pending"
	ToolApprovalApproved = "approved"
	ToolApprovalDenied   = "denied"
	ToolApprovalUsed     = "used"
)

// ToolApprovalGrantTTL bounds how long an approved call may wait before the
// approval goes stale. A human approved one specific call, not a standing
// license for the tool.
const ToolApprovalGrantTTL = 15 * time.Minute

type ToolApproval struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Tool      string `json:"tool"`
	ArgsHash  string `json:"args_hash"`
	ArgsJSON  string `json:"args"`
	// ArgsTruncated marks a preview that was cut for display; the digest is
	// always over the full canonical arguments.
	ArgsTruncated bool   `json:"args_truncated,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	DecidedBy     string `json:"decided_by,omitempty"`
	RequestedAt   string `json:"requested_at"`
	DecidedAt     string `json:"decided_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	UsedAt        string `json:"used_at,omitempty"`
	// Expired is derived on read: an approved grant past its TTL that was never
	// spent. It is reported, not rewritten, so the ledger keeps the facts.
	Expired bool `json:"expired,omitempty"`
}

var ErrToolApprovalNotFound = errors.New("tool approval not found")
var ErrToolApprovalNotPending = errors.New("tool approval is no longer pending")

// RequestToolApproval records a pending request for one exact call. An
// identical pending request is reused, so a retrying model cannot flood the
// inbox with duplicates.
func (l *Ledger) RequestToolApproval(sessionID, tool, argsHash, argsJSON string, argsTruncated bool) (*ToolApproval, bool, error) {
	if sessionID == "" || tool == "" || argsHash == "" {
		return nil, false, fmt.Errorf("tool approval needs session, tool, and args hash")
	}
	row := l.DB().QueryRow(`SELECT id, session_id, tool, args_hash, args_json,
		args_truncated, status, reason, decided_by, requested_at, decided_at,
		expires_at, used_at
		FROM tool_approvals
		WHERE session_id = ? AND tool = ? AND args_hash = ? AND status = 'pending'
		ORDER BY requested_at DESC LIMIT 1`, sessionID, tool, argsHash)
	existing, err := scanToolApproval(row)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	now := nowUTC()
	id := fmt.Sprintf("tap_%d", time.Now().UTC().UnixNano())
	if _, err := l.DB().Exec(`INSERT INTO tool_approvals
		(id, session_id, tool, args_hash, args_json, args_truncated, status, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		id, sessionID, tool, argsHash, argsJSON, argsTruncated, now); err != nil {
		return nil, false, fmt.Errorf("insert tool approval: %w", err)
	}
	return &ToolApproval{
		ID: id, SessionID: sessionID, Tool: tool, ArgsHash: argsHash,
		ArgsJSON: argsJSON, ArgsTruncated: argsTruncated,
		Status: ToolApprovalPending, RequestedAt: now,
	}, true, nil
}

func scanToolApproval(row *sql.Row) (*ToolApproval, error) {
	var a ToolApproval
	if err := row.Scan(&a.ID, &a.SessionID, &a.Tool, &a.ArgsHash, &a.ArgsJSON,
		&a.ArgsTruncated, &a.Status, &a.Reason, &a.DecidedBy, &a.RequestedAt,
		&a.DecidedAt, &a.ExpiresAt, &a.UsedAt); err != nil {
		return nil, err
	}
	a.Expired = a.Status == ToolApprovalApproved && a.UsedAt == "" &&
		grantExpired(a.ExpiresAt)
	return &a, nil
}

func (l *Ledger) GetToolApproval(sessionID, id string) (*ToolApproval, error) {
	row := l.DB().QueryRow(`SELECT id, session_id, tool, args_hash, args_json,
		args_truncated, status, reason, decided_by, requested_at, decided_at,
		expires_at, used_at
		FROM tool_approvals WHERE session_id = ? AND id = ?`, sessionID, id)
	a, err := scanToolApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrToolApprovalNotFound
	}
	return a, err
}

// ListToolApprovals returns pending requests first, then recent decisions.
func (l *Ledger) ListToolApprovals(sessionID string, limit int) ([]ToolApproval, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := l.DB().Query(`SELECT id, session_id, tool, args_hash, args_json,
		args_truncated, status, reason, decided_by, requested_at, decided_at,
		expires_at, used_at
		FROM tool_approvals WHERE session_id = ?
		ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, requested_at DESC
		LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolApproval{}
	for rows.Next() {
		var a ToolApproval
		if err := rows.Scan(&a.ID, &a.SessionID, &a.Tool, &a.ArgsHash, &a.ArgsJSON,
			&a.ArgsTruncated, &a.Status, &a.Reason, &a.DecidedBy, &a.RequestedAt,
			&a.DecidedAt, &a.ExpiresAt, &a.UsedAt); err != nil {
			return nil, err
		}
		a.Expired = a.Status == ToolApprovalApproved && a.UsedAt == "" &&
			grantExpired(a.ExpiresAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecideToolApproval is a compare-and-set: only a pending request moves. An
// approval gets a TTL; a denial is final.
func (l *Ledger) DecideToolApproval(sessionID, id, decision, reason, actor string) (*ToolApproval, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	var expiresAt, status string
	switch decision {
	case "approve", ToolApprovalApproved:
		status = ToolApprovalApproved
		expiresAt = time.Now().UTC().Add(ToolApprovalGrantTTL).Format(time.RFC3339)
	case "deny", ToolApprovalDenied:
		status = ToolApprovalDenied
	default:
		return nil, fmt.Errorf("tool approval decision %q is not approve|deny", decision)
	}
	now := nowUTC()
	res, err := l.DB().Exec(`UPDATE tool_approvals SET status = ?, reason = ?,
		decided_by = ?, decided_at = ?, expires_at = ?
		WHERE session_id = ? AND id = ? AND status = 'pending'`,
		status, reason, actor, now, expiresAt, sessionID, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := l.GetToolApproval(sessionID, id); errors.Is(err, ErrToolApprovalNotFound) {
			return nil, ErrToolApprovalNotFound
		}
		return nil, ErrToolApprovalNotPending
	}
	return l.GetToolApproval(sessionID, id)
}

// ConsumeToolApproval spends one unexpired approval for the exact call. The
// conditional UPDATE makes the spend atomic; a concurrent duplicate dispatch
// cannot reuse the same grant.
func (l *Ledger) ConsumeToolApproval(sessionID, tool, argsHash string) (string, bool, error) {
	now := nowUTC()
	var id string
	err := l.DB().QueryRow(`SELECT id FROM tool_approvals
		WHERE session_id = ? AND tool = ? AND args_hash = ? AND status = 'approved'
			AND used_at = '' AND expires_at > ?
		ORDER BY decided_at ASC LIMIT 1`, sessionID, tool, argsHash, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	res, err := l.DB().Exec(`UPDATE tool_approvals SET status = 'used', used_at = ?
		WHERE id = ? AND status = 'approved' AND used_at = ''`, now, id)
	if err != nil {
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", false, nil
	}
	return id, true, nil
}

// grantExpired reports an approved grant past its TTL. An empty timestamp is
// not expired (legacy rows), but it also cannot be spent by Consume.
func grantExpired(expiresAt string) bool {
	if expiresAt == "" {
		return false
	}
	at, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return !at.After(time.Now().UTC())
}
