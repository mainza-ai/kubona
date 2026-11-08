package ledger

import (
	"fmt"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/money"
)

// WorkflowRun is the durable header for one workflow execution. The row is
// created before dispatch and updated only after a terminal step result is
// persisted; status=running on boot therefore means interrupted.
type WorkflowRun struct {
	ID           string  `json:"id"`
	SessionID    string  `json:"session_id"`
	ExternalID   string  `json:"external_id,omitempty"`
	Status       string  `json:"status"`
	WorkflowJSON string  `json:"-"`
	CeilingUSD   float64 `json:"budget_ceiling_usd"`
	SpentUSD     float64 `json:"spent_usd"`
	Error        string  `json:"error,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// WorkflowStep is one durable step row. A completed step always has the job
// or result that settled it; a running/pending row is the repair surface.
type WorkflowStep struct {
	RunID      string `json:"run_id"`
	StepID     string `json:"step_id"`
	Ordinal    int    `json:"ordinal"`
	Tool       string `json:"tool"`
	Status     string `json:"status"`
	JobID      string `json:"job_id,omitempty"`
	ResultJSON string `json:"result_json,omitempty"`
	Error      string `json:"error,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

func workflowNow() string { return time.Now().UTC().Format(time.RFC3339) }

func microUSD(usd float64) (int64, error) {
	m, err := money.FromUSD(usd)
	if err != nil {
		return 0, err
	}
	return int64(m), nil
}

// CreateWorkflowRun persists the run header before any step can spend.
func (l *Ledger) CreateWorkflowRun(id, sessionID, workflowJSON string, ceilingUSD float64) error {
	if id == "" || sessionID == "" || workflowJSON == "" {
		return fmt.Errorf("workflow run needs id, session, and definition")
	}
	micro, err := microUSD(ceilingUSD)
	if err != nil {
		return err
	}
	now := workflowNow()
	_, err = l.db.Exec(
		`INSERT INTO workflow_runs (id, session_id, status, workflow_json, budget_ceiling_micro_usd, created_at, updated_at)
		 VALUES (?, ?, 'running', ?, ?, ?, ?)`,
		id, sessionID, workflowJSON, micro, now, now)
	return err
}

// SetWorkflowExternalID records the Python workflow id after the bridge call.
func (l *Ledger) SetWorkflowExternalID(id, externalID string) error {
	_, err := l.db.Exec(`UPDATE workflow_runs SET external_id = ?, updated_at = ? WHERE id = ?`,
		externalID, workflowNow(), id)
	return err
}

// UpsertWorkflowStep writes the latest durable state for one step.
func (l *Ledger) UpsertWorkflowStep(step WorkflowStep) error {
	if step.RunID == "" || step.StepID == "" {
		return fmt.Errorf("workflow step needs run_id and step_id")
	}
	if step.Status == "" {
		step.Status = "pending"
	}
	now := workflowNow()
	_, err := l.db.Exec(
		`INSERT INTO workflow_steps (run_id, step_id, ordinal, tool, status, job_id, result_json, error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, step_id) DO UPDATE SET
		   ordinal=excluded.ordinal, tool=excluded.tool, status=excluded.status,
		   job_id=excluded.job_id, result_json=excluded.result_json,
		   error=excluded.error, updated_at=excluded.updated_at`,
		step.RunID, step.StepID, step.Ordinal, step.Tool, step.Status,
		step.JobID, step.ResultJSON, step.Error, now)
	return err
}

// FinishWorkflowRun stores a terminal run status and realized spend.
func (l *Ledger) FinishWorkflowRun(id, status string, spentUSD float64, errText string) error {
	switch status {
	case "completed", "failed", "repair_required":
	default:
		return fmt.Errorf("workflow run status %q is not completed|failed|repair_required", status)
	}
	micro, err := microUSD(spentUSD)
	if err != nil {
		return err
	}
	_, err = l.db.Exec(
		`UPDATE workflow_runs SET status = ?, spent_micro_usd = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, micro, errText, workflowNow(), id)
	return err
}

// RecoverInterruptedWorkflowRuns marks running rows left by a crashed process.
// It never assumes success: the operator sees repair_required, and the job
// rows (if any) remain the source of step truth.
func (l *Ledger) RecoverInterruptedWorkflowRuns() (int64, error) {
	res, err := l.db.Exec(
		`UPDATE workflow_runs SET status = 'repair_required', error = 'interrupted: no terminal workflow receipt on boot', updated_at = ? WHERE status = 'running'`,
		workflowNow())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListWorkflowRuns returns a session's newest runs, with steps loaded by the
// caller via ListWorkflowSteps when needed.
func (l *Ledger) ListWorkflowRuns(sessionID string, limit int) ([]WorkflowRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := l.db.Query(
		`SELECT id, session_id, external_id, status, workflow_json,
		        budget_ceiling_micro_usd, spent_micro_usd, error, created_at, updated_at
		   FROM workflow_runs WHERE session_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRun{}
	for rows.Next() {
		var r WorkflowRun
		var ceiling, spent int64
		if err := rows.Scan(&r.ID, &r.SessionID, &r.ExternalID, &r.Status, &r.WorkflowJSON,
			&ceiling, &spent, &r.Error, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.CeilingUSD = money.MicroUSD(ceiling).USD()
		r.SpentUSD = money.MicroUSD(spent).USD()
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListWorkflowSteps returns all step rows for one run in definition order.
func (l *Ledger) ListWorkflowSteps(runID string) ([]WorkflowStep, error) {
	rows, err := l.db.Query(
		`SELECT run_id, step_id, ordinal, tool, status, job_id, result_json, error, updated_at
		   FROM workflow_steps WHERE run_id = ? ORDER BY ordinal, step_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowStep{}
	for rows.Next() {
		var s WorkflowStep
		if err := rows.Scan(&s.RunID, &s.StepID, &s.Ordinal, &s.Tool, &s.Status,
			&s.JobID, &s.ResultJSON, &s.Error, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
