package session

import "fmt"

// DeleteBlockedError is a fail-closed refusal with named, actionable blockers.
// The API maps it to 409 delete_blocked; callers must not report success.
type DeleteBlockedError struct {
	SessionID string
	Blockers  []string
}

func (e *DeleteBlockedError) Error() string {
	return fmt.Sprintf("session %s not deleted: %v (settle or abort first)", e.SessionID, e.Blockers)
}

// DeleteReceiptError means the session rows were deleted and committed, but the
// audit receipt could not be appended. The deletion is real; report it as a
// warning, never as "nothing was deleted".
type DeleteReceiptError struct {
	SessionID string
	Err       error
}

func (e *DeleteReceiptError) Error() string {
	return fmt.Sprintf("session %s deleted, but the audit receipt failed: %v", e.SessionID, e.Err)
}

func (e *DeleteReceiptError) Unwrap() error { return e.Err }
