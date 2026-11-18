package ledger

// Session autonomy policy persistence (AG-7). The policy is a small JSON
// document; the ledger stores it verbatim so the API can validate and normalize
// it, and so a restart resumes the same safety posture.

const autonomySchema = `
CREATE TABLE IF NOT EXISTS session_autonomy (
	session_id TEXT PRIMARY KEY,
	policy_json TEXT NOT NULL DEFAULT '{}',
	updated_at TEXT NOT NULL
);`

func (l *Ledger) GetAutonomyPolicy(sessionID string) (string, error) {
	var raw string
	err := l.DB().QueryRow(`SELECT policy_json FROM session_autonomy WHERE session_id = ?`,
		sessionID).Scan(&raw)
	if err != nil {
		if errorsIsNoRows(err) {
			return "", nil
		}
		return "", err
	}
	return raw, nil
}

func (l *Ledger) SetAutonomyPolicy(sessionID, policyJSON string) error {
	_, err := l.DB().Exec(`INSERT INTO session_autonomy (session_id, policy_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET policy_json = excluded.policy_json,
		updated_at = excluded.updated_at`, sessionID, policyJSON, nowUTC())
	return err
}
