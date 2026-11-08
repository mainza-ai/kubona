package jobs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// ReconcilePythonJobs imports terminal Python job records that have no Go
// row. Python owns the workflow-step executor; a crash after Python writes
// a terminal result and before Go adopts it leaves a ghost. This is the
// bounded recovery path: completed/failed/cancelled records only, idempotent,
// and only for sessions that exist in the Go book. Queued/running records are
// deliberately left to the live bridge.
//
// It returns counts for the boot log; individual malformed lines are skipped
// with an error count rather than making the daemon unstartable.
func (r *Runner) ReconcilePythonJobs(path string) (adopted, skipped int) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0
		}
		return 0, 1
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// Python job snapshots can exceed the 64 KiB default when a result is
	// large; use a bounded 4 MiB line scanner.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec struct {
			JobID     string          `json:"job_id"`
			SessionID string          `json:"session_id"`
			Status    string          `json:"status"`
			Result    json.RawMessage `json:"result"`
			Error     string          `json:"error"`
			CreatedAt float64         `json:"created_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil || rec.JobID == "" || rec.SessionID == "" {
			skipped++
			continue
		}
		status := strings.ToLower(strings.TrimSpace(rec.Status))
		if status != "completed" && status != "failed" && status != "cancelled" {
			continue
		}
		if _, err := r.GetJob(rec.JobID); err == nil {
			continue
		}
		if _, err := r.sessionMgr.GetSession(rec.SessionID); err != nil {
			skipped++
			continue
		}
		result := rec.Result
		if len(result) == 0 || string(result) == "null" {
			result, _ = json.Marshal(map[string]any{"error": rec.Error, "reconciled_from": "python_jobs.jsonl"})
		}
		if _, err := r.AdoptExternalJobStatus(rec.SessionID, rec.JobID, "circuit_run", status, result); err != nil {
			skipped++
			continue
		}
		adopted++
	}
	if err := scanner.Err(); err != nil {
		skipped++
	}
	return adopted, skipped
}

// AdoptExternalJobStatus is the terminal-adoption primitive used by the
// Python-job reconciliation. It is idempotent and emits the normal money
// receipt; it never creates a reservation because the Python/Go source job
// already owns its spend path.
func (r *Runner) AdoptExternalJobStatus(sessionID, jobID, jobType, status string, resultJSON []byte) (*Job, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	var pbStatus pb.JobStatus
	switch status {
	case "completed":
		pbStatus = pb.JobStatus_JOB_STATUS_COMPLETED
	case "failed":
		pbStatus = pb.JobStatus_JOB_STATUS_FAILED
	case "cancelled":
		pbStatus = pb.JobStatus_JOB_STATUS_CANCELLED
	default:
		return nil, fmt.Errorf("adopted job status %q is not terminal", status)
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("adopted job needs session and job id")
	}
	if existing, err := r.GetJob(jobID); err == nil && existing != nil {
		return existing, nil
	}
	if len(resultJSON) == 0 || !json.Valid(resultJSON) {
		return nil, fmt.Errorf("adopted job %s has no valid result", jobID)
	}
	now := time.Now().UTC()
	params, _ := json.Marshal(map[string]any{"source": "python_jobs_reconciled", "job_id": jobID})
	j := &Job{ID: jobID, SessionID: sessionID, Status: pbStatus, JobType: jobType,
		ParamsJSON: params, ResultJSON: json.RawMessage(resultJSON), CreatedAt: now, CompletedAt: &now}
	if _, err := r.ledger.DB().Exec(
		`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 0)`,
		j.ID, j.SessionID, j.Status.String(), j.JobType, string(j.ParamsJSON), string(j.ResultJSON), j.CreatedAt, now,
	); err != nil {
		return nil, fmt.Errorf("adopt external job: %w", err)
	}
	r.appendMoneyEvent(sessionID, "job_queued", map[string]any{
		"job_id": jobID, "session_id": sessionID, "job_type": jobType,
		"status": status, "adopted": true, "source": "python_reconcile",
	})
	r.appendMoneyEvent(sessionID, "job_settled", map[string]any{
		"job_id": jobID, "session_id": sessionID, "job_type": jobType,
		"status": status, "adopted": true, "source": "python_reconcile",
	})
	return j, nil
}
