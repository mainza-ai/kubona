package ledger

// C11 — run-scoped allowances: one signed envelope, many bounded draws.
// Admission validates scope; the draw itself is an atomic CAS inside the
// same transaction as the spend reservation and job insert.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	RunAllowanceActive    = "active"
	RunAllowanceSuspended = "suspended"
	RunAllowanceExhausted = "exhausted"
	RunAllowanceExpired   = "expired"
	RunAllowanceRevoked   = "revoked"
)

type RunAllowance struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	SessionID              string    `json:"session_id"`
	CampaignID             string    `json:"campaign_id,omitempty"`
	ScopeDigest            string    `json:"scope_digest"`
	Backends               []string  `json:"backends"`
	JobTypes               []string  `json:"job_types"`
	MaxPerJobMicroUSD      int64     `json:"max_per_job_micro_usd"`
	MaxTotalMicroUSD       int64     `json:"max_total_micro_usd"`
	RemainingMicroUSD      int64     `json:"remaining_micro_usd"`
	MaxJobs                int       `json:"max_jobs"`
	JobsUsed               int       `json:"jobs_used"`
	ShotsCap               int       `json:"shots_cap"`
	Status                 string    `json:"status"`
	SuspendedReason        string    `json:"suspended_reason,omitempty"`
	IssuedAt               time.Time `json:"issued_at"`
	ExpiresAt              time.Time `json:"expires_at"`
	BasisRateCard          string    `json:"basis_rate_card"`
	BasisCalibrationDigest string    `json:"basis_calibration_digest,omitempty"`
	Nonce                  string    `json:"nonce"`
	SignerPubkeyHex        string    `json:"signer_pubkey_hex"`
	SignatureHex           string    `json:"signature_hex"`
}

// MintRunAllowance stores a verified allowance. Validation of the signature
// and scope happens in the API layer (it owns the operator keys); this method
// enforces the store-level invariants.
func (l *Ledger) MintRunAllowance(a RunAllowance) (*RunAllowance, error) {
	if a.ID == "" || a.TenantID == "" || a.SessionID == "" || a.Nonce == "" ||
		a.SignerPubkeyHex == "" || a.SignatureHex == "" || a.ScopeDigest == "" {
		return nil, fmt.Errorf("run allowance needs id, tenant, session, nonce, signer, signature, and scope digest")
	}
	if a.MaxTotalMicroUSD <= 0 || a.MaxPerJobMicroUSD <= 0 || a.MaxPerJobMicroUSD > a.MaxTotalMicroUSD ||
		a.MaxJobs < 1 || a.ShotsCap < 1 {
		return nil, fmt.Errorf("run allowance caps are invalid")
	}
	if a.ExpiresAt.Before(time.Now().UTC()) {
		return nil, fmt.Errorf("run allowance expiry is already in the past")
	}
	a.RemainingMicroUSD = a.MaxTotalMicroUSD
	a.JobsUsed = 0
	a.Status = RunAllowanceActive
	a.IssuedAt = time.Now().UTC()
	l.lock.Lock()
	defer l.lock.Unlock()
	_, err := l.db.Exec(`INSERT INTO run_allowances
		(id, tenant_id, session_id, campaign_id, scope_digest, backends_json, job_types_json,
		 max_per_job_micro_usd, max_total_micro_usd, remaining_micro_usd, max_jobs, jobs_used, shots_cap,
		 status, suspended_reason, issued_at, expires_at, basis_rate_card, basis_calibration_digest,
		 nonce, signer_pubkey_hex, signature_hex)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.TenantID, a.SessionID, a.CampaignID, a.ScopeDigest, mustJSON(a.Backends), mustJSON(a.JobTypes),
		a.MaxPerJobMicroUSD, a.MaxTotalMicroUSD, a.RemainingMicroUSD, a.MaxJobs, a.ShotsCap,
		a.Status, a.IssuedAt, a.ExpiresAt, a.BasisRateCard, a.BasisCalibrationDigest,
		a.Nonce, a.SignerPubkeyHex, a.SignatureHex)
	if err != nil {
		return nil, fmt.Errorf("mint run allowance: %w", err)
	}
	return &a, nil
}

func (l *Ledger) GetRunAllowance(id string) (*RunAllowance, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return l.getRunAllowanceLocked(id)
}

func (l *Ledger) getRunAllowanceLocked(id string) (*RunAllowance, error) {
	var a RunAllowance
	var backends, jobTypes, issued, expires string
	err := l.db.QueryRow(`SELECT id, tenant_id, session_id, campaign_id, scope_digest, backends_json, job_types_json,
		max_per_job_micro_usd, max_total_micro_usd, remaining_micro_usd, max_jobs, jobs_used, shots_cap,
		status, suspended_reason, issued_at, expires_at, basis_rate_card, basis_calibration_digest,
		nonce, signer_pubkey_hex, signature_hex
		FROM run_allowances WHERE id = ?`, id).
		Scan(&a.ID, &a.TenantID, &a.SessionID, &a.CampaignID, &a.ScopeDigest, &backends, &jobTypes,
			&a.MaxPerJobMicroUSD, &a.MaxTotalMicroUSD, &a.RemainingMicroUSD, &a.MaxJobs, &a.JobsUsed, &a.ShotsCap,
			&a.Status, &a.SuspendedReason, &issued, &expires, &a.BasisRateCard, &a.BasisCalibrationDigest,
			&a.Nonce, &a.SignerPubkeyHex, &a.SignatureHex)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	_ = unmarshalJSONString(backends, &a.Backends)
	_ = unmarshalJSONString(jobTypes, &a.JobTypes)
	a.IssuedAt = parseLedgerTime(issued)
	a.ExpiresAt = parseLedgerTime(expires)
	return &a, nil
}

// DrawRunAllowanceTx atomically consumes one draw inside the job admission
// transaction. The CAS predicate is the authority: status, expiry, per-draw
// and remaining caps all fail closed. Returns the remaining amount after the
// draw. Duplicate (allowance, job) rows abort the whole transaction.
func (l *Ledger) DrawRunAllowanceTx(tx *sql.Tx, id, jobID, requestID string, amountMicro int64) (int64, error) {
	if amountMicro <= 0 {
		return 0, fmt.Errorf("run allowance draw needs a positive amount")
	}
	now := time.Now().UTC()
	res, err := tx.Exec(`UPDATE run_allowances
		SET remaining_micro_usd = remaining_micro_usd - ?, jobs_used = jobs_used + 1
		WHERE id = ? AND status = ? AND datetime(expires_at) > datetime(?) AND remaining_micro_usd >= ? AND jobs_used < max_jobs`,
		amountMicro, id, RunAllowanceActive, now.Format(time.RFC3339), amountMicro)
	if err != nil {
		return 0, fmt.Errorf("run allowance draw failed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		// Name the reason for the refusal without a second writer.
		var status string
		var remaining int64
		var jobsUsed, maxJobs int
		var expires string
		if qerr := tx.QueryRow(`SELECT status, remaining_micro_usd, jobs_used, max_jobs, expires_at FROM run_allowances WHERE id = ?`, id).
			Scan(&status, &remaining, &jobsUsed, &maxJobs, &expires); qerr != nil {
			return 0, fmt.Errorf("run allowance %s not found", id)
		}
		switch {
		case status != RunAllowanceActive:
			return 0, fmt.Errorf("run allowance %s is %s: refused", id, status)
		case !parseLedgerTime(expires).IsZero() && !now.Before(parseLedgerTime(expires)):
			return 0, fmt.Errorf("run allowance %s expired at %s: refused", id, expires)
		case remaining < amountMicro:
			return 0, fmt.Errorf("run allowance %s has $%.4f remaining; draw needs $%.4f", id, float64(remaining)/1e6, float64(amountMicro)/1e6)
		default:
			return 0, fmt.Errorf("run allowance %s exhausted (%d/%d jobs): refused", id, jobsUsed, maxJobs)
		}
	}
	if _, err := tx.Exec(`INSERT INTO run_allowance_draws (allowance_id, job_id, request_id, amount_micro_usd, created_at)
		VALUES (?, ?, ?, ?, ?)`, id, jobID, requestID, amountMicro, now); err != nil {
		return 0, fmt.Errorf("run allowance draw record failed: %w", err)
	}
	if _, err := tx.Exec(`UPDATE run_allowances SET status = ?
		WHERE id = ? AND status = ? AND (remaining_micro_usd <= 0 OR jobs_used >= max_jobs)`,
		RunAllowanceExhausted, id, RunAllowanceActive); err != nil {
		return 0, fmt.Errorf("run allowance exhaustion update failed: %w", err)
	}
	var remaining int64
	if err := tx.QueryRow(`SELECT remaining_micro_usd FROM run_allowances WHERE id = ?`, id).Scan(&remaining); err != nil {
		return 0, err
	}
	return remaining, nil
}

// SuspendRunAllowance stops new draws but keeps the audit trail. Revocation is
// the same terminal state with a reason naming the operator action.
func (l *Ledger) SuspendRunAllowance(id, reason, status string) error {
	if status != RunAllowanceSuspended && status != RunAllowanceRevoked {
		return fmt.Errorf("unknown allowance stop status %q", status)
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	res, err := l.db.Exec(`UPDATE run_allowances SET status = ?, suspended_reason = ?
		WHERE id = ? AND status = ?`, status, reason, id, RunAllowanceActive)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("run allowance %s is not active", id)
	}
	return nil
}

// ListRunAllowances returns a session's allowances, newest first.
func (l *Ledger) ListRunAllowances(sessionID string) ([]RunAllowance, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	rows, err := l.db.Query(`SELECT id FROM run_allowances WHERE session_id = ? ORDER BY issued_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]RunAllowance, 0, len(ids))
	for _, id := range ids {
		a, err := l.getRunAllowanceLocked(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			out = append(out, *a)
		}
	}
	return out, nil
}

// SuspendActiveRunAllowances stops every active allowance with one reason.
func (l *Ledger) SuspendActiveRunAllowances(reason string) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	res, err := l.db.Exec(`UPDATE run_allowances SET status = ?, suspended_reason = ? WHERE status = ?`,
		RunAllowanceSuspended, reason, RunAllowanceActive)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func unmarshalJSONString(raw string, dst any) error {
	if raw == "" {
		raw = "[]"
	}
	return json.Unmarshal([]byte(raw), dst)
}

// ListActiveRunAllowances returns every active allowance (air-gap suspension).
func (l *Ledger) ListActiveRunAllowances() ([]RunAllowance, error) {
	l.lock.RLock()
	var ids []string
	rows, err := l.db.Query(`SELECT id FROM run_allowances WHERE status = ?`, RunAllowanceActive)
	if err != nil {
		l.lock.RUnlock()
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	l.lock.RUnlock()
	out := make([]RunAllowance, 0, len(ids))
	for _, id := range ids {
		a, err := l.GetRunAllowance(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			out = append(out, *a)
		}
	}
	return out, nil
}
