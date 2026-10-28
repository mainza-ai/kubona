package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	SessionID string                 `json:"session_id"`
	Workflow  json.RawMessage        `json:"workflow"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
	Force     bool                   `json:"force,omitempty"`
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

	var plannedSteps []map[string]interface{}
	if resp.PlannedStepsJson != "" {
		_ = json.Unmarshal([]byte(resp.PlannedStepsJson), &plannedSteps)
	}
	var compensationsPlanned []map[string]interface{}
	if resp.CompensationsPlannedJson != "" {
		_ = json.Unmarshal([]byte(resp.CompensationsPlannedJson), &compensationsPlanned)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                    true,
		"dry_run":               true,
		"workflow_id":           resp.WorkflowId,
		"steps":                 plannedSteps,
		"unresolved_references": resp.UnresolvedReferences,
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

	// Inspect declared ceiling for SQLite spend reservation
	var wfHeader struct {
		Name             string   `json:"name"`
		BudgetCeilingUSD *float64 `json:"budget_ceiling_usd"`
	}
	_ = json.Unmarshal(req.Workflow, &wfHeader)
	var reservedUSD float64
	if wfHeader.BudgetCeilingUSD != nil && *wfHeader.BudgetCeilingUSD > 0 {
		reservedUSD = *wfHeader.BudgetCeilingUSD
		if err := s.sessionMgr.ReserveSpend(sess.ID, reservedUSD); err != nil {
			WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("workflow budget reservation refused: %v", err), nil)
			return
		}
	}

	bridge := s.quantumBridge(w, "workflow run")
	if bridge == nil {
		if reservedUSD > 0 {
			_ = s.sessionMgr.ReleaseSpend(sess.ID, reservedUSD)
		}
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
		Token:                   "dev-token",
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

	var lastResults map[string]interface{}
	if resp.LastResultsJson != "" {
		_ = json.Unmarshal([]byte(resp.LastResultsJson), &lastResults)
	}
	var compensations []map[string]interface{}
	if resp.CompensationsJson != "" {
		_ = json.Unmarshal([]byte(resp.CompensationsJson), &compensations)
	}

	if !resp.Ok {
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

	s.ledger.AppendEvent(sess.ID, "workflow_execution_completed", map[string]interface{}{
		"workflow_id":     resp.WorkflowId,
		"session_id":      sess.ID,
		"steps_completed": resp.StepsCompleted,
		"spent_usd":       resp.SpentUsd,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"workflow_id":     resp.WorkflowId,
		"steps_completed": resp.StepsCompleted,
		"step_records":    resp.StepRecords,
		"spent_usd":       resp.SpentUsd,
		"last_results":    lastResults,
		"compensations":   compensations,
	})
}
