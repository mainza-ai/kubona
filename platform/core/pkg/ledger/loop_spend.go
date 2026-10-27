package ledger

// I3 — loop spend books: atomic drawdown + guarded release. The release
// guard mirrors the jobs pattern (zero-first): the conditional zero is
// the atomic claim, so a kill racing a completion releases exactly once.

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/money"
)

// AddLoopConsumed draws measured iteration spend against the loop's
// reservation (monotonic increment; overdraw is the CALLER's signal to
// pause, never clamped here — clamping would hide the overrun).
func (l *Ledger) AddLoopConsumed(id string, amount float64) error {
	if amount < 0 {
		return fmt.Errorf("negative drawdown refused")
	}
	if amount == 0 {
		return nil
	}
	amountMicro, merr := money.FromUSD(amount)
	if merr != nil {
		return fmt.Errorf("drawdown amount: %w", merr)
	}
	if _, err := l.db.Exec(`UPDATE loops SET consumed_amount_micro_usd = consumed_amount_micro_usd + ?, consumed_amount = (consumed_amount_micro_usd + ?) / 1000000.0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		int64(amountMicro), int64(amountMicro), id); err != nil {
		return fmt.Errorf("drawdown failed: %w", err)
	}
	return nil
}

// GetLoopState loads one loop row (nil, nil when absent).
func (l *Ledger) GetLoopState(id string) (*LoopPersist, error) {
	row := l.db.QueryRow(`SELECT id, session_id, harness_id, status, step_count, max_steps, last_activity, drain, reserved_amount_micro_usd, reserved_tenant, consumed_amount_micro_usd, updated_at FROM loops WHERE id = ?`, id)
	var p LoopPersist
	var lastAct, updated sql.NullString
	var drain int
	var reservedMicro, consumedMicro int64
	if err := row.Scan(&p.ID, &p.SessionID, &p.HarnessID, &p.Status, &p.StepCount, &p.MaxSteps,
		&lastAct, &drain, &reservedMicro, &p.ReservedTenant, &consumedMicro, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.Drain = drain != 0
	p.ReservedAmount = money.MicroUSD(reservedMicro).USD()
	p.ConsumedAmount = money.MicroUSD(consumedMicro).USD()
	if lastAct.Valid {
		if t, err := time.Parse(time.RFC3339, lastAct.String); err == nil {
			p.LastActivity = t
		}
	}
	if updated.Valid {
		if t, err := time.Parse(time.RFC3339, updated.String); err == nil {
			p.UpdatedAt = t
		}
	}
	return &p, nil
}

// ClaimLoopSpend atomically claims a loop's unspent reservation for
// release: read, then conditional zero (only when still reserved). The
// zero is the claim — concurrent releasers (kill racing completion)
// release exactly once (the loser reads the win or finds n==0 and
// skips). A crash between zero and the tenant release leaves the tenant
// OVER-reserved (safe direction, visible in spent_usd, recoverable via
// the audited refund primitive) — never under, never double.
func (l *Ledger) ClaimLoopSpend(id string) (tenant string, refund float64, ok bool, err error) {
	var reservedMicro, consumedMicro int64
	var reservedTenant string
	if err := l.db.QueryRow(`SELECT reserved_amount_micro_usd, consumed_amount_micro_usd, reserved_tenant FROM loops WHERE id = ?`, id).
		Scan(&reservedMicro, &consumedMicro, &reservedTenant); err != nil {
		if err == sql.ErrNoRows {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}
	if reservedMicro <= 0 {
		return "", 0, false, nil
	}
	res, err := l.db.Exec(`UPDATE loops SET reserved_amount = 0, reserved_amount_micro_usd = 0, consumed_amount = 0, consumed_amount_micro_usd = 0, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND reserved_amount_micro_usd > 0`, id)
	if err != nil {
		return "", 0, false, fmt.Errorf("claim loop spend failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", 0, false, fmt.Errorf("claim loop spend failed: %w", err)
	}
	if n == 0 {
		return "", 0, false, nil
	}
	refundMicro := reservedMicro - consumedMicro
	if refundMicro < 0 {
		refundMicro = 0 // overrun: nothing to return (the overrun paused the loop first)
	}
	return reservedTenant, money.MicroUSD(refundMicro).USD(), true, nil
}
