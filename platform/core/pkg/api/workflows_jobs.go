package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// A workflow composed only of execution/submit_job steps can be owned by the
// Go jobs rail directly. This is the narrow first slice of the workflow
// ownership migration: no Python job exists to adopt, no ghost can be created,
// and every step is reserved/settled by the same book as a normal job.
//
// Mixed workflows still use the Python engine; their completed jobs are
// adopted by handleWorkflowRun until the full Go worker lands.
func (s *Server) runJobOnlyWorkflow(w http.ResponseWriter, r *http.Request,
	req WorkflowRunPayload, sessID, workflowID string) bool {
	var wf struct {
		Name             string   `json:"name"`
		BudgetCeilingUSD *float64 `json:"budget_ceiling_usd"`
		CreatedByAgent   bool     `json:"created_by_agent"`
		Steps            []struct {
			ID               string         `json:"id"`
			Tool             string         `json:"tool"`
			Args             map[string]any `json:"args"`
			RequiresApproval bool           `json:"requires_approval"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(req.Workflow, &wf); err != nil || len(wf.Steps) == 0 || wf.CreatedByAgent {
		return false
	}
	for _, step := range wf.Steps {
		if step.Tool != "execution/submit_job" || step.RequiresApproval {
			return false
		}
	}
	// A workflow ceiling is a pre-admission bound, not decoration. Check it
	// before validating step payload so a ceiling breach reports the same
	// refusal the Python engine reports (and no job is created).
	if wf.BudgetCeilingUSD != nil && *wf.BudgetCeilingUSD > 0 {
		if sess, err := s.sessionMgr.GetSession(sessID); err == nil &&
			sess.SpendLimitUSD > 0 && sess.CurrentSpendUSD+*wf.BudgetCeilingUSD > sess.SpendLimitUSD {
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "workflow ceiling exceeds session spend limit")
			WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("workflow budget reservation refused: session %s spend limit exceeded", sessID), nil)
			return true
		}
	}
	// Validate templates/args before moving money or creating a job.
	ctx := map[string]any{"session_id": sessID, "input": req.Inputs, "prev": map[string]any{}}
	results := map[string]any{}
	spent := 0.0
	finished := false
	defer func() {
		if !finished {
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", spent, "workflow interrupted before a terminal receipt")
		}
	}()
	for ordinal, step := range wf.Steps {
		rendered, err := renderWorkflowValue(step.Args, ctx)
		if err != nil {
			s.ledger.AppendEvent(sessID, "workflow_execution_failed", map[string]any{
				"workflow_id": workflowID, "session_id": sessID, "step": step.ID, "error": err.Error(),
			})
			WriteAPIError(w, "bad_request", err.Error(), map[string]any{"workflow_id": workflowID, "step": step.ID})
			return true
		}
		args, _ := rendered.(map[string]any)
		qasm, _ := args["circuit_qasm"].(string)
		if strings.TrimSpace(qasm) == "" {
			qasm, _ = args["qasm"].(string)
		}
		if strings.TrimSpace(qasm) == "" {
			WriteAPIError(w, "bad_request", "workflow step requires circuit_qasm", map[string]any{"workflow_id": workflowID, "step": step.ID})
			return true
		}
		backend, _ := args["provider"].(string)
		if backend == "" {
			backend, _ = args["requested_backend"].(string)
		}
		if backend == "" {
			backend = "local_simulator"
		}
		shots := intArg(args["shots"], 1024)
		seed := intArg(args["seed"], 7)
		maxCost := floatArg(args["max_cost_usd"], 0)
		if maxCost <= 0 && wf.BudgetCeilingUSD != nil && *wf.BudgetCeilingUSD > 0 {
			maxCost = *wf.BudgetCeilingUSD
		}
		if maxCost <= 0 {
			maxCost = 1.0
		}
		jobParams := map[string]any{
			"qasm": qasm, "requested_backend": backend, "shots": shots, "seed": seed,
			"max_cost_usd": maxCost, "mandate_ref": args["mandate_ref"],
			"caller_jurisdictions": args["caller_jurisdictions"],
			"caller_export_class":  args["caller_export_class"],
			"export_reviewed":      args["export_reviewed"],
		}
		if err := s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{
			RunID: workflowID, StepID: step.ID, Ordinal: ordinal + 1,
			Tool: step.Tool, Status: "running",
		}); err != nil {
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", spent, "workflow journal step failed: "+err.Error())
			finished = true
			WriteAPIError(w, "internal", "workflow journal step failed: "+err.Error(), nil)
			return true
		}
		job, err := s.jobRunner.SubmitJob(sessID, "circuit_run", jobParams)
		if err != nil {
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
				Ordinal: ordinal + 1, Tool: step.Tool, Status: "failed", Error: err.Error()})
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", spent, err.Error())
			finished = true
			s.ledger.AppendEvent(sessID, "workflow_execution_failed", map[string]any{
				"workflow_id": workflowID, "session_id": sessID, "step": step.ID, "error": err.Error(),
			})
			WriteAPIError(w, "bad_request", err.Error(), map[string]any{"workflow_id": workflowID, "step": step.ID})
			return true
		}
		terminal, ok := waitForWorkflowJob(s.jobRunner, job.ID, 120*time.Second)
		if !ok {
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
				Ordinal: ordinal + 1, Tool: step.Tool, Status: "running", JobID: job.ID})
			finished = true // leave the run journal running; boot recovery surfaces repair_required
			WriteAPIError(w, "accepted", "workflow job still running", map[string]any{
				"workflow_id": workflowID, "step": step.ID, "job_id": job.ID, "status": "running",
			})
			return true
		}
		spent += terminal.ReservedAmount
		if terminal.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
			errText := string(terminal.ResultJSON)
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
				Ordinal: ordinal + 1, Tool: step.Tool, Status: "failed", JobID: terminal.ID,
				ResultJSON: errText, Error: errText})
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", spent, errText)
			finished = true
			s.ledger.AppendEvent(sessID, "workflow_execution_failed", map[string]any{
				"workflow_id": workflowID, "session_id": sessID, "step": step.ID,
				"job_id": terminal.ID, "error": errText,
			})
			WriteAPIError(w, "bad_request", "workflow job failed: "+errText, map[string]any{
				"workflow_id": workflowID, "step": step.ID, "job_id": terminal.ID,
			})
			return true
		}
		var result map[string]any
		if err := json.Unmarshal(terminal.ResultJSON, &result); err != nil {
			result = map[string]any{"raw": string(terminal.ResultJSON)}
		}
		result["job_id"] = terminal.ID
		result["status"] = "completed"
		results[step.ID] = result
		ctx["prev"] = results
		rawResult, _ := json.Marshal(result)
		_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
			Ordinal: ordinal + 1, Tool: step.Tool, Status: "completed", JobID: terminal.ID,
			ResultJSON: string(rawResult)})
		s.ledger.AppendEvent(sessID, "workflow_step", map[string]any{
			"workflow": workflowID, "workflow_id": workflowID, "session_id": sessID,
			"step": step.ID, "tool": step.Tool, "job_id": terminal.ID,
			"result": result, "spent_usd": spent,
		})
	}
	_ = s.ledger.FinishWorkflowRun(workflowID, "completed", spent, "")
	finished = true
	s.ledger.AppendEvent(sessID, "workflow_execution_completed", map[string]any{
		"workflow_id": workflowID, "session_id": sessID, "steps_completed": len(wf.Steps), "spent_usd": spent,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "workflow_id": workflowID, "steps_completed": len(wf.Steps),
		"step_records": []string{}, "spent_usd": spent, "last_results": results,
		"compensations": []any{}, "engine": "go-jobs",
	})
	return true
}

func waitForWorkflowJob(r *jobs.Runner, id string, timeout time.Duration) (*jobs.Job, bool) {
	deadline := time.Now().Add(timeout)
	for {
		j, err := r.GetJob(id)
		if err == nil && j != nil {
			switch j.Status {
			case pb.JobStatus_JOB_STATUS_COMPLETED, pb.JobStatus_JOB_STATUS_FAILED, pb.JobStatus_JOB_STATUS_CANCELLED:
				return j, true
			}
		}
		if time.Now().After(deadline) {
			return j, false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func intArg(v any, def int) int {
	switch n := v.(type) {
	case float64:
		if n >= 1 {
			return int(n)
		}
	case int:
		if n >= 1 {
			return n
		}
	}
	return def
}

func floatArg(v any, def float64) float64 {
	if n, ok := v.(float64); ok {
		return n
	}
	return def
}

func renderWorkflowValue(v any, ctx map[string]any) (any, error) {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "{{") && strings.HasSuffix(x, "}}") {
			path := strings.Split(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(x, "{{"), "}}")), ".")
			var node any = ctx
			for _, part := range path {
				m, ok := node.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("workflow template %q does not resolve", x)
				}
				node, ok = m[part]
				if !ok {
					return nil, fmt.Errorf("workflow template %q does not resolve", x)
				}
			}
			return node, nil
		}
		return x, nil
	case map[string]any:
		out := map[string]any{}
		for k, child := range x {
			r, err := renderWorkflowValue(child, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(x))
		for _, child := range x {
			r, err := renderWorkflowValue(child, ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	default:
		return v, nil
	}
}
