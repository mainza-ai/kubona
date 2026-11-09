package ledger

import (
	"fmt"
	"time"
)

// WorkflowApproval is a single-use, content-bound human approval for one
// workflow definition. The token itself is signed by the server's reviewer
// key; only its hash is stored, so a database read cannot replay it.
type WorkflowApproval struct {
	TokenHash      string `json:"token_hash"`
	SessionID      string `json:"session_id"`
	WorkflowSHA256 string `json:"workflow_sha256"`
	ApprovedBy     string `json:"approved_by"`
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
	UsedAt         string `json:"used_at,omitempty"`
}

func workflowApprovalNow() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateWorkflowApproval stores an unused approval. The token hash is the
// primary key, so an identical mint is idempotent only when it is byte-equal;
// a different token for the same workflow is a distinct approval.
func (l *Ledger) CreateWorkflowApproval(a WorkflowApproval) error {
	if a.TokenHash == "" || a.SessionID == "" || a.WorkflowSHA256 == "" || a.ExpiresAt == "" {
		return fmt.Errorf("workflow approval needs token_hash, session, workflow hash, and expiry")
	}
	now := workflowApprovalNow()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	if _, err := l.db.Exec(`DELETE FROM workflow_approvals WHERE expires_at < ?`, now); err != nil {
		return err
	}
	_, err := l.db.Exec(
		`INSERT INTO workflow_approvals
		 (token_hash, session_id, workflow_sha256, approved_by, expires_at, created_at, used_at)
		 VALUES (?, ?, ?, ?, ?, ?, '')`,
		a.TokenHash, a.SessionID, a.WorkflowSHA256, a.ApprovedBy, a.ExpiresAt, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("store workflow approval: %w", err)
	}
	return nil
}

// ConsumeWorkflowApproval atomically marks a matching, unexpired, unused
// approval as used. It returns false for every mismatch: wrong session,
// wrong workflow bytes, expired, already used, or unknown.
func (l *Ledger) ConsumeWorkflowApproval(tokenHash, sessionID, workflowSHA256 string, now time.Time) (bool, error) {
	if tokenHash == "" || sessionID == "" || workflowSHA256 == "" {
		return false, nil
	}
	res, err := l.db.Exec(
		`UPDATE workflow_approvals SET used_at = ?
		  WHERE token_hash = ? AND session_id = ? AND workflow_sha256 = ?
		    AND used_at = '' AND expires_at > ?`,
		now.UTC().Format(time.RFC3339), tokenHash, sessionID, workflowSHA256,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// WorkflowApprovalByHash is used only by diagnostics/audit callers; the run
// path never trusts this read instead of the atomic consume.
func (l *Ledger) WorkflowApprovalByHash(tokenHash string) (*WorkflowApproval, error) {
	var a WorkflowApproval
	err := l.db.QueryRow(
		`SELECT token_hash, session_id, workflow_sha256, approved_by, expires_at, created_at, used_at
		   FROM workflow_approvals WHERE token_hash = ?`, tokenHash).
		Scan(&a.TokenHash, &a.SessionID, &a.WorkflowSHA256, &a.ApprovedBy, &a.ExpiresAt, &a.CreatedAt, &a.UsedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
