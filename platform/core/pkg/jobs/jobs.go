package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	pb "github.com/mainza-ai/kubona/core/proto"
)

type Job struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	Status      pb.JobStatus    `json:"status"`
	JobType     string          `json:"job_type"` // "circuit_run", "circuit_cut", "classical_anchor", "claim_eval"
	ParamsJSON  json.RawMessage `json:"params_json"`
	ResultJSON  json.RawMessage `json:"result_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

type Runner struct {
	ledger       *ledger.Ledger
	bridgeClient pb.QuantumBridgeServiceClient
	jobQueue     chan *Job
	workers      int
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewRunner(l *ledger.Ledger, bridgeClient pb.QuantumBridgeServiceClient, workers int) *Runner {
	if workers <= 0 {
		workers = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		ledger:       l,
		bridgeClient: bridgeClient,
		jobQueue:     make(chan *Job, 1000),
		workers:      workers,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (r *Runner) Start() {
	for i := 0; i < r.workers; i++ {
		r.wg.Add(1)
		go r.workerLoop()
	}
}

func (r *Runner) Stop() {
	r.cancel()
	close(r.jobQueue)
	r.wg.Wait()
}

func (r *Runner) SubmitJob(sessionID, jobType string, params any) (*Job, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	j := &Job{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		Status:     pb.JobStatus_JOB_STATUS_QUEUED,
		JobType:    jobType,
		ParamsJSON: data,
		CreatedAt:  time.Now().UTC(),
	}

	query := `INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err = r.ledger.DB().Exec(query, j.ID, j.SessionID, j.Status.String(), j.JobType, string(j.ParamsJSON), j.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save job to db: %w", err)
	}

	r.ledger.AppendEvent(sessionID, "job_queued", j)
	r.jobQueue <- j
	return j, nil
}

func (r *Runner) workerLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case j, ok := <-r.jobQueue:
			if !ok {
				return
			}
			r.executeJob(j)
		}
	}
}

func (r *Runner) executeJob(j *Job) {
	// Update status to running
	j.Status = pb.JobStatus_JOB_STATUS_RUNNING
	r.ledger.DB().Exec(`UPDATE jobs SET status = ? WHERE id = ?`, j.Status.String(), j.ID)
	r.ledger.AppendEvent(j.SessionID, "job_started", j)

	var resultData []byte
	var execErr error

	switch j.JobType {
	case "circuit_run":
		var req pb.CircuitExecutionRequest
		if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
			execErr = err
			break
		}
		req.JobId = j.ID
		req.SessionId = j.SessionID
		resp, err := r.bridgeClient.ExecuteCircuit(r.ctx, &req)
		if err != nil {
			execErr = err
		} else {
			resultData, _ = json.Marshal(resp)
		}

	case "circuit_cut":
		var req pb.CircuitCutRequest
		if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
			execErr = err
			break
		}
		resp, err := r.bridgeClient.CutCircuit(r.ctx, &req)
		if err != nil {
			execErr = err
		} else {
			resultData, _ = json.Marshal(resp)
		}

	case "classical_anchor":
		var req pb.ClassicalAnchorRequest
		if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
			execErr = err
			break
		}
		resp, err := r.bridgeClient.ComputeClassicalAnchor(r.ctx, &req)
		if err != nil {
			execErr = err
		} else {
			resultData, _ = json.Marshal(resp)
		}

	case "claim_eval":
		var req pb.ClaimEvaluationRequest
		if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
			execErr = err
			break
		}
		resp, err := r.bridgeClient.EvaluateWavefunction(r.ctx, &req)
		if err != nil {
			execErr = err
		} else {
			resultData, _ = json.Marshal(resp)
		}

	default:
		execErr = fmt.Errorf("unknown job type: %s", j.JobType)
	}

	now := time.Now().UTC()
	j.CompletedAt = &now
	if execErr != nil {
		j.Status = pb.JobStatus_JOB_STATUS_FAILED
		errRes, _ := json.Marshal(map[string]string{"error": execErr.Error()})
		j.ResultJSON = errRes
	} else {
		j.Status = pb.JobStatus_JOB_STATUS_COMPLETED
		j.ResultJSON = resultData
	}

	r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ?`,
		j.Status.String(), string(j.ResultJSON), j.CompletedAt, j.ID,
	)
	r.ledger.AppendEvent(j.SessionID, "job_settled", j)
}

func (r *Runner) GetJob(id string) (*Job, error) {
	query := `SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at FROM jobs WHERE id = ?`
	row := r.ledger.DB().QueryRow(query, id)

	var j Job
	var statusStr, paramsStr string
	var resultStr sql.NullString
	var compAt sql.NullTime

	if err := row.Scan(&j.ID, &j.SessionID, &statusStr, &j.JobType, &paramsStr, &resultStr, &j.CreatedAt, &compAt); err != nil {
		return nil, err
	}
	j.Status = pb.JobStatus(pb.JobStatus_value[statusStr])
	j.ParamsJSON = json.RawMessage(paramsStr)
	if resultStr.Valid {
		j.ResultJSON = json.RawMessage(resultStr.String)
	}
	if compAt.Valid {
		t := compAt.Time
		j.CompletedAt = &t
	}
	return &j, nil
}

func (r *Runner) ListJobs(sessionID string, limit int) ([]*Job, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = r.ledger.DB().Query(
			`SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at FROM jobs WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`,
			sessionID, limit,
		)
	} else {
		rows, err = r.ledger.DB().Query(
			`SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at FROM jobs ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobsList []*Job
	for rows.Next() {
		var j Job
		var statusStr, paramsStr string
		var resultStr sql.NullString
		var compAt sql.NullTime
		if err := rows.Scan(&j.ID, &j.SessionID, &statusStr, &j.JobType, &paramsStr, &resultStr, &j.CreatedAt, &compAt); err != nil {
			return nil, err
		}
		j.Status = pb.JobStatus(pb.JobStatus_value[statusStr])
		j.ParamsJSON = json.RawMessage(paramsStr)
		if resultStr.Valid {
			j.ResultJSON = json.RawMessage(resultStr.String)
		}
		if compAt.Valid {
			t := compAt.Time
			j.CompletedAt = &t
		}
		jobsList = append(jobsList, &j)
	}
	return jobsList, nil
}
