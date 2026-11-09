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

// WorkflowPlanPayload is the request payload for POST /api/workflows/plan.
type WorkflowPlanPayload struct {
	SessionID string                 `json:"session_id"`
	Workflow  json.RawMessage        `json:"workflow"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
}

// WorkflowRunPayload is the request payload for POST /api/workflows/run.
type WorkflowRunPayload struct {
	SessionID     string                 `json:"session_id"`
	Workflow      json.RawMessage        `json:"workflow"`
	Inputs        map[string]interface{} `json:"inputs,omitempty"`
	Force         bool                   `json:"force,omitempty"`
	ApprovalToken string                 `json:"approval_token,omitempty"`
}

// POST /api/workflows/plan — planning/dry-run validation without execution.
func (s *Server) handleWorkflowPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to plan a workflow", nil)
		return
	}
	var req WorkflowPlanPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid JSON body: %v", err), nil)
		return
	}
	if len(req.Workflow) == 0 {
		WriteAPIError(w, "bad_request", "workflow definition is required", nil)
		return
	}

	if req.SessionID != "" {
		sess, err := s.sessionMgr.GetSession(req.SessionID)
		if err != nil {
			WriteAPIError(w, "not_found", fmt.Sprintf("session %s not found", req.SessionID), nil)
			return
		}
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
	}

	bridge := s.quantumBridge(w, "workflow plan")
	if bridge == nil {
		return
	}

	inputsJSON := ""
	if req.Inputs != nil {
		raw, _ := json.Marshal(req.Inputs)
		inputsJSON = string(raw)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := bridge.WorkflowPlan(ctx, &pb.WorkflowPlanRequest{
		WorkflowJson: string(req.Workflow),
		SessionId:    req.SessionID,
		InputsJson:   inputsJSON,
	})
	if err != nil {
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("bridge WorkflowPlan error: %v", err), nil)
		return
	}
	if !resp.Ok {
		WriteAPIError(w, "bad_request", resp.ErrorMessage, nil)
		return
	}

	plannedSteps := make([]map[string]interface{}, 0)
	if resp.PlannedStepsJson != "" {
		_ = json.Unmarshal([]byte(resp.PlannedStepsJson), &plannedSteps)
	}
	if plannedSteps == nil {
		plannedSteps = make([]map[string]interface{}, 0)
	}
	compensationsPlanned := make([]map[string]interface{}, 0)
	if resp.CompensationsPlannedJson != "" {
		_ = json.Unmarshal([]byte(resp.CompensationsPlannedJson), &compensationsPlanned)
	}
	if compensationsPlanned == nil {
		compensationsPlanned = make([]map[string]interface{}, 0)
	}

	unresolved := resp.UnresolvedReferences
	if unresolved == nil {
		unresolved = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                    true,
		"dry_run":               true,
		"workflow_id":           resp.WorkflowId,
		"steps":                 plannedSteps,
		"unresolved_references": unresolved,
		"compensations_planned": compensationsPlanned,
	})
}

// POST /api/workflows/run — executes a validated workflow against the MCP gateway with budget reservations.
func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "POST required to run a workflow", nil)
		return
	}
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req WorkflowRunPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid JSON body: %v", err), nil)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		WriteAPIError(w, "bad_request", "session_id is required to run a workflow (fail-closed spend binding)", nil)
		return
	}
	if len(req.Workflow) == 0 {
		WriteAPIError(w, "bad_request", "workflow definition is required", nil)
		return
	}

	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", fmt.Sprintf("session %s not found", req.SessionID), nil)
		return
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	if err := s.requireDispatchAutonomy(sess.ID); err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}

	var wfHeader struct {
		Name             string   `json:"name"`
		BudgetCeilingUSD *float64 `json:"budget_ceiling_usd"`
	}
	_ = json.Unmarshal(req.Workflow, &wfHeader)
	var ceilingUSD float64
	if wfHeader.BudgetCeilingUSD != nil && *wfHeader.BudgetCeilingUSD > 0 {
		ceilingUSD = *wfHeader.BudgetCeilingUSD
	}
	// Journal before any step can spend. A crash leaves a visible
	// repair_required row instead of a ghost or a silent second book.
	workflowID := fmt.Sprintf("wf-%d", time.Now().UnixNano())
	if err := s.ledger.CreateWorkflowRun(workflowID, sess.ID, string(req.Workflow), ceilingUSD); err != nil {
		WriteAPIError(w, "internal", "workflow journal create failed: "+err.Error(), nil)
		return
	}

	// Job-only workflows are owned by the Go jobs rail. This branch prevents
	// a Python job from being created and adopted as a second book.
	if s.runJobOnlyWorkflow(w, r, req, sess.ID, workflowID) {
		return
	}

	// Normal mixed workflows are Go-owned step-by-step. Approval-required or
	// agent-created workflows fall through to the Python compatibility engine.
	if s.runGoMixedWorkflow(w, r, req, sess.ID, workflowID, ceilingUSD) {
		return
	}

	// No dev fallback: the workflow gateway token is the bridge token in
	// ALL envs. The bridge builds a require_auth gateway around it; a
	// literal "dev-token" (or an empty token) would run bridged tools
	// unauthenticated, so refuse before any spend is reserved.
	bridgeToken := strings.TrimSpace(os.Getenv("KUBONA_BRIDGE_TOKEN"))
	if bridgeToken == "" {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "KUBONA_BRIDGE_TOKEN is not configured")
		WriteAPIError(w, "bad_request", "KUBONA_BRIDGE_TOKEN is not configured: refusing unauthenticated workflow execution", nil)
		return
	}

	// Inspect declared ceiling for SQLite spend reservation.
	var reservedUSD float64
	if ceilingUSD > 0 {
		reservedUSD = ceilingUSD
		if err := s.sessionMgr.ReserveSpend(sess.ID, reservedUSD); err != nil {
			_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "ceiling: "+err.Error())
			WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("workflow budget reservation refused: %v", err), nil)
			return
		}
	}

	bridge := s.quantumBridge(w, "workflow run")
	if bridge == nil {
		if reservedUSD > 0 {
			_ = s.sessionMgr.ReleaseSpend(sess.ID, reservedUSD)
		}
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "quantum bridge unavailable")
		return
	}

	inputsJSON := ""
	if req.Inputs != nil {
		raw, _ := json.Marshal(req.Inputs)
		inputsJSON = string(raw)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	resp, err := bridge.WorkflowRun(ctx, &pb.WorkflowRunRequest{
		WorkflowJson:            string(req.Workflow),
		SessionId:               sess.ID,
		Token:                   bridgeToken,
		InputsJson:              inputsJSON,
		Force:                   req.Force,
		SessionVerified:         true,
		SessionBudgetCeilingUsd: sess.SpendLimitUSD,
		SessionSpentUsd:         sess.CurrentSpendUSD,
		TenantId:                sess.TenantID,
	})
	if err != nil {
		if reservedUSD > 0 {
			_ = s.sessionMgr.ReleaseSpend(sess.ID, reservedUSD)
		}
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", 0, "bridge WorkflowRun: "+err.Error())
		WriteAPIError(w, "bridge_unavailable", fmt.Sprintf("bridge WorkflowRun error: %v", err), nil)
		return
	}

	// Reconcile spend reservation against realized spend
	if reservedUSD > 0 {
		if !resp.Ok {
			_ = s.sessionMgr.ReleaseSpend(sess.ID, reservedUSD)
		} else if resp.SpentUsd < reservedUSD {
			_ = s.sessionMgr.ReleaseSpend(sess.ID, reservedUSD-resp.SpentUsd)
		}
	} else if resp.SpentUsd > 0 {
		_ = s.sessionMgr.ReserveSpend(sess.ID, resp.SpentUsd)
	}

	_ = s.ledger.SetWorkflowExternalID(workflowID, resp.WorkflowId)
	var lastResults map[string]interface{}
	if resp.LastResultsJson != "" {
		_ = json.Unmarshal([]byte(resp.LastResultsJson), &lastResults)
	}
	var compensations []map[string]interface{}
	if resp.CompensationsJson != "" {
		_ = json.Unmarshal([]byte(resp.CompensationsJson), &compensations)
	}

	if !resp.Ok {
		_ = s.ledger.FinishWorkflowRun(workflowID, "failed", resp.SpentUsd, resp.ErrorMessage)
		s.ledger.AppendEvent(sess.ID, "workflow_execution_failed", map[string]interface{}{
			"workflow_id":   resp.WorkflowId,
			"session_id":    sess.ID,
			"error":         resp.ErrorMessage,
			"spent_usd":     resp.SpentUsd,
			"compensations": compensations,
		})
		WriteAPIError(w, "bad_request", resp.ErrorMessage, map[string]interface{}{
			"workflow_id":   resp.WorkflowId,
			"step_records":  resp.StepRecords,
			"spent_usd":     resp.SpentUsd,
			"compensations": compensations,
		})
		return
	}

	// Mirror each executed workflow step into the Go ledger. The tools still
	// run in the Python compute plane, but the session's audit trail must name
	// every step/tool/result in the same book as the workflow receipt.
	var wfDef struct {
		Steps []struct {
			ID   string `json:"id"`
			Tool string `json:"tool"`
		} `json:"steps"`
	}
	_ = json.Unmarshal(req.Workflow, &wfDef)
	toolByStep := map[string]string{}
	for _, step := range wfDef.Steps {
		toolByStep[step.ID] = step.Tool
	}
	ordinal := 0
	allStepsTerminal := true
	for _, step := range wfDef.Steps {
		ordinal++
		result, ok := lastResults[step.ID]
		if !ok {
			allStepsTerminal = false
			_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{
				RunID: workflowID, StepID: step.ID, Ordinal: ordinal,
				Tool: step.Tool, Status: "pending",
			})
			continue
		}
		raw, _ := json.Marshal(result)
		_ = s.ledger.UpsertWorkflowStep(ledger.WorkflowStep{
			RunID: workflowID, StepID: step.ID, Ordinal: ordinal,
			Tool: step.Tool, Status: "completed", ResultJSON: string(raw),
		})
	}
	for stepID, result := range lastResults {
		raw, _ := json.Marshal(result)
		// The Python workflow engine executes the step; if it returned a
		// completed job, adopt that exact job id + result into the Go jobs
		// book so /api/jobs and the session ledger do not point at a ghost.
		var adoptedID string
		if rec, ok := result.(map[string]interface{}); ok {
			jobID, _ := rec["job_id"].(string)
			if jobID != "" && strings.EqualFold(fmt.Sprint(rec["status"]), "completed") {
				if resultBody, ok := rec["result"]; ok {
					resultJSON, _ := json.Marshal(resultBody)
					tool := toolByStep[stepID]
					jobType := "circuit_run"
					if strings.Contains(tool, "qubo") {
						jobType = "circuit_run"
					}
					if _, err := s.jobRunner.AdoptExternalJob(sess.ID, jobID, jobType, resultJSON); err == nil {
						adoptedID = jobID
					}
				}
			}
		}
		payload := map[string]interface{}{
			"workflow": resp.WorkflowId, "workflow_id": resp.WorkflowId, "session_id": sess.ID,
			"step": stepID, "tool": toolByStep[stepID],
			"result": json.RawMessage(raw), "spent_usd": resp.SpentUsd,
		}
		if adoptedID != "" {
			payload["adopted_job_id"] = adoptedID
		}
		s.ledger.AppendEvent(sess.ID, "workflow_step", payload)
	}
	s.ledger.AppendEvent(sess.ID, "workflow_execution_completed", map[string]interface{}{
		"workflow_id":     resp.WorkflowId,
		"session_id":      sess.ID,
		"steps_completed": resp.StepsCompleted,
		"spent_usd":       resp.SpentUsd,
	})

	if allStepsTerminal {
		_ = s.ledger.FinishWorkflowRun(workflowID, "completed", resp.SpentUsd, "")
	} else {
		_ = s.ledger.FinishWorkflowRun(workflowID, "repair_required", resp.SpentUsd, "bridge returned no terminal result for every step")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"workflow_id":     resp.WorkflowId,
		"steps_completed": resp.StepsCompleted,
		"step_records":    resp.StepRecords,
		"spent_usd":       resp.SpentUsd,
		"last_results":    lastResults,
		"compensations":   compensations,
		"repair_required": !allStepsTerminal,
	})
}
