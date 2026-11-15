package jobs

// C11 — run-scoped allowance admission. The API mints a signed envelope; this
// rail validates it against the exact job, the live rate card, the live
// calibration, and the structured taint flag before the atomic draw.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/money"
	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// allowanceRefOf reads the allowance id from admitted params.
func allowanceRefOf(params []byte) string {
	var probe struct {
		RunAllowanceID string `json:"run_allowance_id"`
	}
	if err := json.Unmarshal(params, &probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.RunAllowanceID)
}

// calibrationBasisDigest is the same calibration identity the proposal rail
// binds: device profile name|source|version|calibrated_at.
func (r *Runner) calibrationBasisDigest(backend string) (string, error) {
	device, err := fidelity.ResolveDeviceForBackend(backend)
	if err != nil {
		return "", err
	}
	profile, ok := r.fidelityGate.Profile(device)
	if !ok {
		return "", fmt.Errorf("no calibration profile for %q", backend)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		profile.Name, profile.Source, profile.Version, profile.CalibratedAt,
	}, "|")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// planRunAllowance validates a metered circuit_run against its signed
// allowance. The draw itself is the atomic authority; this check exists so a
// refusal happens before any reservation work.
func (r *Runner) planRunAllowance(tenantID, sessionID, jobType string, p *circuitRunParams) error {
	if p.RunAllowanceID == "" {
		return nil
	}
	if strings.TrimSpace(p.MandateRef) != "" {
		return fmt.Errorf("run_allowance_id and mandate_ref are mutually exclusive: name one authority")
	}
	if isSimulatorBackend(p.RequestedBackend) {
		return fmt.Errorf("run allowances cover metered hardware only; simulator runs need no allowance")
	}
	backend, err := NormalizeBackend(p.RequestedBackend)
	if err != nil {
		return err
	}
	if p.BasisTainted {
		return fmt.Errorf("this launch consumed untrusted content: a run allowance cannot cover it; per-job approval is required")
	}
	a, err := r.ledger.GetRunAllowance(p.RunAllowanceID)
	if err != nil {
		return fmt.Errorf("run allowance cannot be read: %w", err)
	}
	if a == nil {
		return fmt.Errorf("run allowance %s not found: refused", p.RunAllowanceID)
	}
	if a.TenantID != tenantID || a.SessionID != sessionID {
		return fmt.Errorf("run allowance %s is scoped to another tenant/session: refused", p.RunAllowanceID)
	}
	if a.Status != "active" {
		return fmt.Errorf("run allowance %s is %s: refused", p.RunAllowanceID, a.Status)
	}
	if !a.ExpiresAt.IsZero() && !time.Now().UTC().Before(a.ExpiresAt) {
		return fmt.Errorf("run allowance %s expired at %s: refused", p.RunAllowanceID, a.ExpiresAt.Format(time.RFC3339))
	}
	if !containsString(a.Backends, backend) {
		return fmt.Errorf("backend %q is outside the allowance's backend list: refused", backend)
	}
	if !containsString(a.JobTypes, jobType) {
		return fmt.Errorf("job type %q is outside the allowance's type list: refused", jobType)
	}
	if p.Shots > 0 && a.ShotsCap > 0 && p.Shots > a.ShotsCap {
		return fmt.Errorf("shots %d exceed the allowance cap %d: refused", p.Shots, a.ShotsCap)
	}
	if p.MaxCostUSD <= 0 {
		return fmt.Errorf("run allowance draws need a positive max_cost_usd")
	}
	amountMicro, merr := money.FromUSD(p.MaxCostUSD)
	if merr != nil {
		return fmt.Errorf("run allowance amount: %w", merr)
	}
	if int64(amountMicro) > a.MaxPerJobMicroUSD {
		return fmt.Errorf("job cost $%.4f exceeds the allowance per-job cap $%.4f: refused", p.MaxCostUSD, float64(a.MaxPerJobMicroUSD)/1e6)
	}
	if int64(amountMicro) > a.RemainingMicroUSD {
		return fmt.Errorf("job cost $%.4f exceeds the allowance remaining $%.4f: refused", p.MaxCostUSD, float64(a.RemainingMicroUSD)/1e6)
	}
	if a.BasisRateCard != "" {
		current := rates.CardSnapshot().EffectiveDate
		if current != a.BasisRateCard {
			_ = r.ledger.SuspendRunAllowance(a.ID, "rate_card_moved", "suspended")
			return fmt.Errorf("rate card moved since the allowance was signed (%s → %s): re-approve", a.BasisRateCard, current)
		}
	}
	if a.BasisCalibrationDigest != "" {
		current, cerr := r.calibrationBasisDigest(backend)
		if cerr != nil {
			_ = r.ledger.SuspendRunAllowance(a.ID, "calibration_basis_unavailable", "suspended")
			return fmt.Errorf("calibration basis unavailable: %w", cerr)
		}
		if current != a.BasisCalibrationDigest {
			_ = r.ledger.SuspendRunAllowance(a.ID, "calibration_moved", "suspended")
			return fmt.Errorf("calibration changed since the allowance was signed: re-approve")
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
