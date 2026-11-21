package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BootReconcile is the count of rows classified before workers start.
type BootReconcile struct {
	Epoch       int64
	Requeued    int
	Interrupted int
	Unknown     int
}

// ReconcileJobsOnBoot classifies rows left by a dead owner before workers
// start. A queued row that was never dispatched is re-enqueued. A host-only
// running row is interrupted and its reservation released. A vendor-reachable
// running row becomes OUTCOME_UNKNOWN and keeps its reservation. Nothing is
// retried, and no timer clears the unknown row.
func (r *Runner) ReconcileJobsOnBoot() (BootReconcile, error) {
	var out BootReconcile
	var maxEpoch int64
	if err := r.ledger.DB().QueryRow(`SELECT COALESCE(MAX(owner_epoch), 0) FROM jobs`).Scan(&maxEpoch); err != nil {
		return out, err
	}
	out.Epoch = maxEpoch + 1
	r.mu.Lock()
	r.ownerEpoch = out.Epoch
	r.mu.Unlock()
	if dir := strings.TrimSpace(os.Getenv("KUBONA_STATE_DIR")); dir != "" {
		if _, err := r.AdoptVendorPins(dir + "/vendor_pins.jsonl"); err != nil {
			return out, err
		}
	}

	rows, err := r.ledger.DB().Query(
		`SELECT id, job_type, params_json FROM jobs WHERE status = ?`,
		pb.JobStatus_JOB_STATUS_RUNNING.String())
	if err != nil {
		return out, err
	}
	type runningRow struct {
		id, jobType string
		params      []byte
	}
	var running []runningRow
	for rows.Next() {
		var row runningRow
		if err := rows.Scan(&row.id, &row.jobType, &row.params); err != nil {
			rows.Close()
			return out, err
		}
		running = append(running, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	var hostIDs []string
	now := time.Now().UTC()
	for _, row := range running {
		if !jobMayReachVendor(row.jobType, row.params) {
			hostIDs = append(hostIDs, row.id)
			continue
		}
		res, err := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ? WHERE id = ? AND status = ?`,
			pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN.String(), row.id, pb.JobStatus_JOB_STATUS_RUNNING.String())
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			out.Unknown++
			r.appendMoneyEvent("", "job_outcome_unknown", map[string]any{
				"job_id": row.id, "reason": "boot: vendor-reachable job owned by a dead epoch",
			})
		}
	}
	for _, id := range hostIDs {
		res, err := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ? AND status = ?`,
			pb.JobStatus_JOB_STATUS_INTERRUPTED.String(), now, id, pb.JobStatus_JOB_STATUS_RUNNING.String())
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			out.Interrupted++
			r.releaseReservation(id)
			r.appendMoneyEvent("", "job_interrupted", map[string]any{
				"job_id": id, "reason": "boot: host-only job owned by a dead epoch",
			})
		}
	}

	queued, err := r.ledger.DB().Query(`SELECT id FROM jobs WHERE status = ?`, pb.JobStatus_JOB_STATUS_QUEUED.String())
	if err != nil {
		return out, err
	}
	var queuedIDs []string
	for queued.Next() {
		var id string
		if err := queued.Scan(&id); err != nil {
			queued.Close()
			return out, err
		}
		queuedIDs = append(queuedIDs, id)
	}
	queued.Close()
	if err := queued.Err(); err != nil {
		return out, err
	}
	var overflow []*Job
	for _, id := range queuedIDs {
		job, err := r.GetJob(id)
		if err != nil {
			return out, err
		}
		// Never re-enqueue Python QuantumBridge-owned jobs onto Go's worker
		// pool: Python already owns their vendor dispatch/lifecycle.
		if isQuantumBridgeOwnedJob(job.ParamsJSON) {
			continue
		}
		if _, err := r.ledger.DB().Exec(
			`UPDATE jobs SET owner_epoch = ? WHERE id = ? AND status = ?`,
			out.Epoch, id, pb.JobStatus_JOB_STATUS_QUEUED.String()); err != nil {
			return out, err
		}
		if len(overflow) == 0 {
			select {
			case r.jobQueue <- job:
				out.Requeued++
				continue
			default:
			}
		}
		overflow = append(overflow, job)
	}
	if len(overflow) > 0 {
		var done <-chan struct{}
		if r.ctx != nil {
			done = r.ctx.Done()
		}
		r.wg.Add(1)
		go func(pending []*Job) {
			defer r.wg.Done()
			for _, job := range pending {
				select {
				case <-done:
					return
				case r.jobQueue <- job:
				}
			}
		}(overflow)
		out.Requeued += len(overflow)
	}
	return out, nil
}

// isQuantumBridgeOwnedJob identifies rows persisted by RecordQuantumBridgeJob
// whose execution lifecycle belongs to Python's QuantumBridgeService rather
// than Go's local worker pool.
func isQuantumBridgeOwnedJob(paramsJSON json.RawMessage) bool {
	if len(paramsJSON) == 0 {
		return false
	}
	var meta struct {
		Origin string `json:"origin"`
	}
	return json.Unmarshal(paramsJSON, &meta) == nil && meta.Origin == "quantum_bridge_rpc"
}

// markDispatched pins the moment a vendor-reachable call is about to leave
// the host. A later transport error is then outcome-unknown, not a refund.
// Host-only types are unchanged.
func (r *Runner) markDispatched(j *Job) {
	if j == nil || !jobMayReachVendor(j.JobType, j.ParamsJSON) {
		return
	}
	now := time.Now().UTC()
	j.DispatchedAt = &now
	_, _ = r.ledger.DB().Exec(
		`UPDATE jobs SET dispatched_at = ? WHERE id = ? AND dispatched_at IS NULL`, now, j.ID)
}

// sealOutcomeUnknown records E1: the call may have reached a vendor and the
// reservation stays. A definitive refusal, or a host-only job, returns false
// so the caller can fail and release.
func (r *Runner) sealOutcomeUnknown(j *Job, execErr error) bool {
	if j == nil || j.DispatchedAt == nil || !jobMayReachVendor(j.JobType, j.ParamsJSON) || !transportMayHaveLanded(execErr) {
		return false
	}
	j.Status = pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN
	j.CompletedAt = nil
	res, err := r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, completed_at = NULL WHERE id = ? AND status = ?`,
		j.Status.String(), j.ID, pb.JobStatus_JOB_STATUS_RUNNING.String())
	if err != nil {
		return false
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false
	}
	r.appendMoneyEvent(j.SessionID, "job_outcome_unknown", map[string]any{
		"job_id": j.ID, "reason": execErr.Error(), "evidence": "transport_after_dispatch",
	})
	return true
}

// jobMayReachVendor is the one decision for "this dispatch may already be
// on a vendor." Hardware job types always may. circuit_run may only when
// its backend is not a local simulator. Host-only types never may.
// experimentSpecForBridge copies the admitted params and adds the Go job
// id so the bridge can pin a vendor id before it acknowledges. The stored
// params are not rewritten.
func experimentSpecForBridge(j *Job) []byte {
	if j == nil {
		return nil
	}
	var spec map[string]any
	if err := json.Unmarshal(j.ParamsJSON, &spec); err != nil || spec == nil {
		return j.ParamsJSON
	}
	spec["kubona_job_id"] = j.ID
	raw, err := json.Marshal(spec)
	if err != nil {
		return j.ParamsJSON
	}
	return raw
}

// pinVendorRef copies a hardware job id out of a result before settlement.
// Empty to non-empty is the only legal write. A second id is refused by the
// immutability trigger and ignored here.
func (r *Runner) pinVendorRef(jobID string, result []byte) {
	if jobID == "" || len(result) == 0 {
		return
	}
	var probe struct {
		HardwareJobID  string   `json:"hardware_job_id"`
		HardwareJobIDs []string `json:"hardware_job_ids"`
	}
	if err := json.Unmarshal(result, &probe); err != nil {
		return
	}
	ref := strings.TrimSpace(probe.HardwareJobID)
	if ref == "" && len(probe.HardwareJobIDs) > 0 {
		ref = strings.TrimSpace(probe.HardwareJobIDs[0])
	}
	if ref == "" {
		return
	}
	_, _ = r.ledger.DB().Exec(
		`UPDATE jobs SET vendor_ref = ? WHERE id = ? AND vendor_ref = ''`, ref, jobID)
}

// AdoptVendorPins copies bridge pin lines onto jobs.vendor_ref. A line
// without a Go job id is skipped. An already-pinned row is left alone.
func (r *Runner) AdoptVendorPins(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var line struct {
			VendorJobID string `json:"vendor_job_id"`
			GoJobID     string `json:"go_job_id"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil || line.GoJobID == "" || line.VendorJobID == "" {
			continue
		}
		res, err := r.ledger.DB().Exec(
			`UPDATE jobs SET vendor_ref = ? WHERE id = ? AND vendor_ref = ''`,
			line.VendorJobID, line.GoJobID)
		if err != nil {
			return n, err
		}
		if rows, _ := res.RowsAffected(); rows == 1 {
			n++
		}
	}
	return n, sc.Err()
}

// quarantineLateResult refuses a bridge callback that names an attempt or
// owner epoch other than the one on the row. The job is not settled and
// its reservation is not touched.
func (r *Runner) quarantineLateResult(j *Job, bridgeStatus, resultJSON string) bool {
	if j == nil {
		return false
	}
	var attempt string
	var epoch int64
	if err := r.ledger.DB().QueryRow(`SELECT attempt_id, owner_epoch FROM jobs WHERE id = ?`, j.ID).Scan(&attempt, &epoch); err != nil {
		return false
	}
	var claimed struct {
		AttemptID  string `json:"attempt_id"`
		OwnerEpoch int64  `json:"owner_epoch"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &claimed); err != nil {
		return false
	}
	if (claimed.AttemptID == "" || claimed.AttemptID == attempt) && (claimed.OwnerEpoch == 0 || claimed.OwnerEpoch == epoch) {
		return false
	}
	r.appendMoneyEvent(j.SessionID, "late_result_quarantined", map[string]any{
		"job_id": j.ID, "attempt_id": claimed.AttemptID, "owner_epoch": claimed.OwnerEpoch,
		"current_attempt_id": attempt, "current_owner_epoch": epoch, "bridge_status": bridgeStatus,
	})
	return true
}

func jobMayReachVendor(jobType string, params []byte) bool {
	if !VendorReachableJobTypes[jobType] {
		return false
	}
	if jobType != "circuit_run" {
		return true
	}
	var p struct {
		RequestedBackend string `json:"requested_backend"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	return !isSimulatorBackend(p.RequestedBackend)
}

func transportMayHaveLanded(err error) bool {
	if err == nil {
		return false
	}
	if errorsIsDeadline(err) {
		return true
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"unavailable", "deadline exceeded", "context canceled", "transport", "connection reset", "connection refused"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func errorsIsDeadline(err error) bool {
	return err == context.DeadlineExceeded || err == context.Canceled ||
		strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded")
}
