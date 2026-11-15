package jobs

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/money"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// ResolveOutcomeUnknown is the signed operator exit from OUTCOME_UNKNOWN.
// It does not accept a timer. vendor_never_received is the only exit that
// releases the reservation, and only when no vendor reference is pinned.
// written_off keeps the estimate. vendor_confirmed settles measured usage
// when the operator supplies it, and otherwise keeps the estimate.
func (r *Runner) ResolveOutcomeUnknown(jobID, outcome, evidence, vendorRef string, actualUSD *float64) error {
	outcome = strings.TrimSpace(outcome)
	evidence = strings.TrimSpace(evidence)
	vendorRef = strings.TrimSpace(vendorRef)
	switch outcome {
	case "vendor_confirmed", "vendor_never_received", "written_off":
	default:
		return fmt.Errorf("unknown outcome resolution %q", outcome)
	}
	if outcome != "vendor_confirmed" && evidence == "" {
		return fmt.Errorf("evidence note is required")
	}
	if outcome == "vendor_confirmed" && vendorRef == "" && actualUSD == nil {
		return fmt.Errorf("vendor_confirmed requires a vendor id or measured usage")
	}
	if actualUSD != nil && (math.IsNaN(*actualUSD) || math.IsInf(*actualUSD, 0) || *actualUSD < 0) {
		return fmt.Errorf("actual_usd must be finite and non-negative")
	}
	var status, currentRef, sessionID, tenant string
	var reserved int64
	err := r.ledger.DB().QueryRow(
		`SELECT status, vendor_ref, session_id, reserved_tenant, reserved_amount_micro_usd FROM jobs WHERE id = ?`, jobID).
		Scan(&status, &currentRef, &sessionID, &tenant, &reserved)
	if err != nil {
		return fmt.Errorf("job not found")
	}
	if status != pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN.String() {
		return fmt.Errorf("job is not outcome-unknown")
	}
	if outcome == "vendor_never_received" && (currentRef != "" || vendorRef != "") {
		return fmt.Errorf("vendor reference is pinned; never-received is refused")
	}
	if vendorRef != "" && currentRef != "" && vendorRef != currentRef {
		return fmt.Errorf("vendor reference does not match the pinned id")
	}
	terminal := pb.JobStatus_JOB_STATUS_FAILED
	if outcome == "vendor_confirmed" {
		terminal = pb.JobStatus_JOB_STATUS_COMPLETED
	}
	body := map[string]any{"outcome": outcome, "evidence": evidence}
	if outcome == "written_off" {
		body["error"] = "outcome_unknown_written_off"
	}
	if vendorRef != "" {
		body["hardware_job_id"] = vendorRef
	} else if currentRef != "" {
		body["hardware_job_id"] = currentRef
	}
	if actualUSD != nil {
		body["actual_usd"] = *actualUSD
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status = ?`,
		terminal.String(), string(raw), now, jobID, pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN.String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("job is not outcome-unknown")
	}
	if vendorRef != "" && currentRef == "" {
		_, _ = r.ledger.DB().Exec(
			`UPDATE jobs SET vendor_ref = ? WHERE id = ? AND vendor_ref = ''`, vendorRef, jobID)
	}
	j := &Job{ID: jobID, SessionID: sessionID, ReservedTenant: tenant, ReservedAmount: money.MicroUSD(reserved).USD()}
	if outcome != "written_off" {
		r.settleTerminalFromResult(j, string(raw))
	}
	r.appendMoneyEvent(sessionID, "job_outcome_resolved", map[string]any{
		"job_id": jobID, "outcome": outcome, "evidence": evidence, "vendor_ref": vendorRef,
	})
	return nil
}
