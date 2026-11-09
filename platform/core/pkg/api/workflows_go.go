package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// runGoMixedWorkflow owns the normal mixed workflow sequence in Go. Job steps
// use the jobs rail; non-job steps execute exactly one tool through the
// RunWorkflowStep RPC. The Python whole-workflow engine is retained only as a
// compatibility fallback for approval-required or agent-created workflows.
func (s *Server) runGoMixedWorkflow(w http.ResponseWriter, r *http.Request,
	req WorkflowRunPayload, sessID, workflowID string, ceilingUSD float64) bool {
	var wf struct {
		Name           string `json:"name"`
		CreatedByAgent bool   `json:"created_by_agent"`
		Steps          []struct {
			ID               string         `json:"id"`
			Tool             string         `json:"tool"`
			Args             map[string]any `json:"args"`
			RequiresApproval bool           `json:"requires_approval"`
		} `json:"steps"`
		OnFailure []struct {
			ID   string         `json:"id"`
			Tool string         `json:"tool"`
			Args map[string]any `json:"args"`
		} `json:"on_failure"`
	}
	if err := json.Unmarshal(req.Workflow, &wf); err != nil || len(wf.Steps) == 0 {
		return false
	}
	needsApproval := wf.CreatedByAgent
	for _, step := range wf.Steps {
		if step.ID == "" || step.Tool == "" {
			return false
		}
		needsApproval = needsApproval || step.RequiresApproval
	}
	for _, step := range wf.OnFailure {
		if step.ID == "" || step.Tool == "" {
			return false
		}
	}
	bridge := s.quantumBridge(w, "workflow step")
	if bridge == nil {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "quantum bridge unavailable")
		return true
	}
	bridgeToken := strings.TrimSpace(os.Getenv("KUBONA_BRIDGE_TOKEN"))
	if bridgeToken == "" {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "KUBONA_BRIDGE_TOKEN is not configured")
		WriteAPIError(w, "bad_request", "KUBONA_BRIDGE_TOKEN is not configured: refusing unauthenticated workflow execution", nil)
		return true
	}
	sess, err := s.sessionMgr.GetSession(sessID)
	if err != nil {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "session disappeared")
		WriteAPIError(w, "not_found", "session disappeared before workflow execution", nil)
		return true
	}
	workflowHash := workflowSHA256(req.Workflow)
	if needsApproval {
		payload, aerr := s.verifyWorkflowApproval(req.ApprovalToken, sessID, workflowHash)
		if aerr != nil {
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, aerr.Error())
			WriteAPIError(w, "approval_required", aerr.Error(), map[string]any{
				"workflow_sha256": workflowHash,
				"remediation":     "POST /api/workflows/approve with the exact workflow, then pass approval_token to /api/workflows/run",
			})
			return true
		}
		used, uerr := s.ledger.ConsumeWorkflowApproval(
			workflowSHA256([]byte(req.ApprovalToken)), sessID, workflowHash, time.Now().UTC())
		if uerr != nil || !used {
			msg := "workflow approval could not be consumed"
			if uerr != nil {
				msg += ": " + uerr.Error()
			} else {
				msg += ": token is unknown, expired, already used, or bound to another workflow"
			}
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, msg)
			WriteAPIError(w, "approval_required", msg, nil)
			return true
		}
		s.ledger.AppendEvent(sessID, "workflow_approval_consumed", map[string]any{
			"workflow_id": workflowID, "workflow_sha256": workflowHash,
			"approved_by": payload.ApprovedBy, "approval_hash": workflowSHA256([]byte(req.ApprovalToken)),
		})
	}
	ctxValues := map[string]any{"session_id": sessID, "input": req.Inputs, "prev": map[string]any{}}
	results := map[string]any{}
	spent := 0.0
	finishFailed := func(stepID, message string, comps []map[string]any) {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", spent, message)
		s.ledger.AppendEvent(sessID, "workflow_execution_failed", map[string]any{
			"workflow_id": workflowID, "session_id": sessID, "step": stepID,
			"error": message, "spent_usd": spent, "compensations": comps,
		})
		WriteAPIError(w, "bad_request", message, map[string]any{
			"workflow_id": workflowID, "step": stepID,
			"spent_usd": spent, "compensations": comps, "engine": "go-mixed",
		})
	}
	runCompensations := func(stepID string) []map[string]any {
		comps := []map[string]any{}
		for _, comp := range wf.OnFailure {
			rendered, rerr := renderWorkflowValue(comp.Args, ctxValues)
			if rerr != nil {
				comps = append(comps, map[string]any{"id": comp.ID, "status": "failed", "error": rerr.Error()})
				continue
			}
			args, _ := rendered.(map[string]any)
			comps = append(comps, s.runGoWorkflowCompensation(bridge, bridgeToken, workflowID, sessID, sess.TenantID, comp.ID, comp.Tool, args))
		}
		_ = stepID
		return comps
	}

	for ordinal, step := range wf.Steps {
		rendered, rerr := renderWorkflowValue(step.Args, ctxValues)
		if rerr != nil {
			finishFailed(step.ID, rerr.Error(), runCompensations(step.ID))
			return true
		}
		args, _ := rendered.(map[string]any)
		if step.Tool == "execution/submit_job" {
			jobParams, qerr := workflowJobParams(args, ceilingUSD, spent)
			if qerr != nil {
				finishFailed(step.ID, qerr.Error(), runCompensations(step.ID))
				return true
			}
			if err := s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{
				RunID: workflowID, StepID: step.ID, Ordinal: ordinal + 1,
				Tool: step.Tool, Status: "running",
			}); err != nil {
				finishFailed(step.ID, "workflow journal step failed: "+err.Error(), nil)
				return true
			}
			job, jerr := s.jobRunner.SubmitJob(sessID, "circuit_run", jobParams)
			if jerr != nil {
				_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
					Ordinal: ordinal + 1, Tool: step.Tool, Status: "failed", Error: jerr.Error()})
				finishFailed(step.ID, jerr.Error(), runCompensations(step.ID))
				return true
			}
			terminal, ok := waitForWorkflowJob(s.jobRunner, job.ID, 120*time.Second)
			if !ok {
				_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
					Ordinal: ordinal + 1, Tool: step.Tool, Status: "running", JobID: job.ID})
				WriteAPIError(w, "accepted", "workflow job still running", map[string]any{
					"workflow_id": workflowID, "step": step.ID, "job_id": job.ID, "status": "running", "engine": "go-mixed",
				})
				return true
			}
			if terminal.Status != pb.JobStatus_JOB_STATUS_COMPLETED {
				errText := string(terminal.ResultJSON)
				_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
					Ordinal: ordinal + 1, Tool: step.Tool, Status: "failed", JobID: terminal.ID,
					ResultJSON: errText, Error: errText})
				finishFailed(step.ID, "workflow job failed: "+errText, runCompensations(step.ID))
				return true
			}
			spent += terminal.ReservedAmount
			var result map[string]any
			if err := json.Unmarshal(terminal.ResultJSON, &result); err != nil {
				result = map[string]any{"raw": string(terminal.ResultJSON)}
			}
			result["job_id"] = terminal.ID
			result["status"] = "completed"
			results[step.ID] = result
			ctxValues["prev"] = results
			rawResult, _ := json.Marshal(result)
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
				Ordinal: ordinal + 1, Tool: step.Tool, Status: "completed", JobID: terminal.ID,
				ResultJSON: string(rawResult)})
			s.ledger.AppendEvent(sessID, "workflow_step", map[string]any{
				"workflow": workflowID, "workflow_id": workflowID, "session_id": sessID,
				"step": step.ID, "tool": step.Tool, "job_id": terminal.ID,
				"result": result, "spent_usd": spent, "engine": "go-mixed",
			})
			continue
		}

		stepJSON, _ := json.Marshal(map[string]any{"id": step.ID, "tool": step.Tool, "args": args})
		resp, err := bridge.RunWorkflowStep(r.Context(), &pb.WorkflowStepRequest{
			StepJson: string(stepJSON), WorkflowId: workflowID, SessionId: sessID,
			Token: bridgeToken, InputsJson: workflowJSON(req.Inputs), SessionVerified: true,
			SessionBudgetCeilingUsd: sess.SpendLimitUSD, SessionSpentUsd: sess.CurrentSpendUSD,
			TenantId: sess.TenantID,
		})
		if err != nil || !resp.GetOk() {
			msg := "workflow step RPC failed"
			if err != nil {
				msg += ": " + err.Error()
			} else {
				msg = resp.GetErrorMessage()
			}
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
				Ordinal: ordinal + 1, Tool: step.Tool, Status: "failed", Error: msg})
			finishFailed(step.ID, msg, runCompensations(step.ID))
			return true
		}
		spent += resp.GetSpentUsd()
		if resp.GetSpentUsd() > 0 {
			if rerr := s.sessionMgr.ReserveSpend(sessID, resp.GetSpentUsd()); rerr != nil {
				finishFailed(step.ID, "step spend booking failed: "+rerr.Error(), runCompensations(step.ID))
				return true
			}
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(resp.GetResultJson()), &result); err != nil {
			result = map[string]any{"raw": resp.GetResultJson()}
		}
		result["status"] = "completed"
		results[step.ID] = result
		ctxValues["prev"] = results
		rawResult, _ := json.Marshal(result)
		_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{RunID: workflowID, StepID: step.ID,
			Ordinal: ordinal + 1, Tool: step.Tool, Status: "completed", ResultJSON: string(rawResult)})
		s.ledger.AppendEvent(sessID, "workflow_step", map[string]any{
			"workflow": workflowID, "workflow_id": workflowID, "session_id": sessID,
			"step": step.ID, "tool": step.Tool, "result": result,
			"spent_usd": spent, "engine": "go-mixed",
		})
	}
	_ = s.ledger.FinishWorkflowRun(workflowID, "completed", spent, "")
	s.ledger.AppendEvent(sessID, "workflow_execution_completed", map[string]any{
		"workflow_id": workflowID, "session_id": sessID,
		"steps_completed": len(wf.Steps), "spent_usd": spent, "engine": "go-mixed",
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "workflow_id": workflowID, "steps_completed": len(wf.Steps),
		"step_records": []string{}, "spent_usd": spent, "last_results": results,
		"compensations": []any{}, "engine": "go-mixed",
	})
	return true
}

func (s *Server) runGoWorkflowCompensation(bridge pb.QuantumBridgeServiceClient, token string,
	workflowID, sessionID, tenantID, id, tool string, args map[string]any) map[string]any {
	// Compensation is best-effort and must never masquerade as the main step.
	out := map[string]any{"id": id, "tool": tool, "status": "failed"}
	// Only non-job tools can be safely compensated through the step RPC; job
	// compensations need an explicit inverse and are recorded as unsupported.
	if tool == "execution/submit_job" {
		out["error"] = "job compensation requires an explicit inverse; not dispatched"
		return out
	}
	raw, _ := json.Marshal(map[string]any{"id": id, "tool": tool, "args": args})
	resp, err := bridge.RunWorkflowStep(context.Background(), &pb.WorkflowStepRequest{
		StepJson: string(raw), WorkflowId: workflowID + ":comp:" + id,
		SessionId: sessionID, TenantId: tenantID, Token: token,
	})
	if err != nil || !resp.GetOk() {
		if err != nil {
			out["error"] = err.Error()
		} else {
			out["error"] = resp.GetErrorMessage()
		}
		return out
	}
	out["status"] = "completed"
	out["result"] = json.RawMessage(resp.GetResultJson())
	return out
}

func workflowJobParams(args map[string]any, ceilingUSD, spent float64) (map[string]any, error) {
	qasm, _ := args["circuit_qasm"].(string)
	if strings.TrimSpace(qasm) == "" {
		qasm, _ = args["qasm"].(string)
	}
	if strings.TrimSpace(qasm) == "" {
		return nil, fmt.Errorf("workflow step requires circuit_qasm")
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
	if maxCost <= 0 && ceilingUSD > 0 {
		maxCost = ceilingUSD
	}
	if maxCost <= 0 {
		maxCost = 1.0
	}
	if ceilingUSD > 0 && spent+maxCost > ceilingUSD {
		return nil, fmt.Errorf("workflow ceiling %.4f exceeded at step cost %.4f", ceilingUSD, maxCost)
	}
	return map[string]any{
		"qasm": qasm, "requested_backend": backend, "shots": shots, "seed": seed,
		"max_cost_usd": maxCost, "mandate_ref": args["mandate_ref"],
		"caller_jurisdictions": args["caller_jurisdictions"],
		"caller_export_class":  args["caller_export_class"],
		"export_reviewed":      args["export_reviewed"],
	}, nil
}

func workflowJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
