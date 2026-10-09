package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/cost"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

// JSONRPCRequest represents a JSON-RPC 2.0 payload
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError defines standard JSON-RPC 2.0 errors
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// AgentCard advertises capabilities and payment rates for A2A discovery
type AgentCard struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Description       string   `json:"description"`
	Capabilities      []string `json:"capabilities"`
	SupportedTokens   []string `json:"supported_tokens"`
	RateUSDPerJob     float64  `json:"rate_usd_per_job"`
	SupportedProfiles []string `json:"supported_profiles"`
	EscrowAddress     string   `json:"escrow_address"`
}

// A2ATaskStatus represents execution lifecycle of a delegated task
type A2ATaskStatus string

const (
	TaskStatusQueued    A2ATaskStatus = "queued"
	TaskStatusRunning   A2ATaskStatus = "running"
	TaskStatusCompleted A2ATaskStatus = "completed"
	TaskStatusFailed    A2ATaskStatus = "failed"
	TaskStatusRevoked   A2ATaskStatus = "revoked"
)

// MaxDelegationDepth bounds agent-created-agent chains: a root delegation is
// depth 0, its child 1, grandchild 2. Deeper chains are refused — delegation
// depth is computed server-side from the stored parent, never client-claimed.
const MaxDelegationDepth = 2

// A2ATask tracks an inter-agent computation delegation
type A2ATask struct {
	ID        string         `json:"id"`
	TaskType  string         `json:"task_type"`
	Status    A2ATaskStatus  `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	// Sub-mandate bound: the child task may never spend above MaxBudgetUSD,
	// which itself may never exceed the verified parent mandate (if any).
	MaxBudgetUSD  float64 `json:"max_budget_usd,omitempty"`
	ParentJobHash string  `json:"parent_job_hash,omitempty"`
	// Delegation lineage: Scopes may only narrow down the chain (never
	// widen), Depth is server-computed, ParentTaskID links the edge used for
	// revocation propagation.
	Scopes       []string `json:"scopes,omitempty"`
	Depth        int      `json:"depth"`
	ParentTaskID string   `json:"parent_task_id,omitempty"`
	// TenantID attributes spend and autonomy checks to the lab tenant named
	// by the parent mandate (or 'default'). Dispatch consults the tenant's
	// live autonomy level; L0 tenants cannot execute.
	TenantID string `json:"tenant_id,omitempty"`
}

// TaskExecutor performs the actual computation behind a delegated task. It is
// supplied by the orchestrator (which owns the ledger-bound job runner); the
// A2A layer never invents results. A nil executor leaves tasks queued.
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, taskType string, payload json.RawMessage, tenantID string) (map[string]any, error)
}

// TaskSubmitParams represents the input parameters for a2a.SubmitTask
type TaskSubmitParams struct {
	TaskType      string                 `json:"task_type"`
	Payload       json.RawMessage        `json:"payload"`
	MaxBudgetUSD  float64                `json:"max_budget_usd,omitempty"`
	ParentMandate *mandates.SpendMandate `json:"parent_mandate,omitempty"`
	Scopes        []string               `json:"scopes,omitempty"`
	ParentTaskID  string                 `json:"parent_task_id,omitempty"`
}

// TaskStatusParams represents the input parameters for a2a.GetTaskStatus
type TaskStatusParams struct {
	TaskID string `json:"task_id"`
}

// A2AServer provides inter-agent JSON-RPC communication and task coordination
type A2AServer struct {
	mu         sync.RWMutex
	card       AgentCard
	tasks      map[string]*A2ATask
	paymentMgr *cost.PaymentManager
	runner     *sandbox.SandboxRunner
	executor   TaskExecutor
	// autonomyChecker reports a tenant's live autonomy level (L0-L3).
	// L0 refuses execution at dispatch; unknown tenants fail closed.
	// Nil means unchecked (tests / standalone use).
	autonomyChecker func(tenantID string) (string, error)
}

// NewA2AServer creates a new A2A server instance
func NewA2AServer(escrowAddr string, pm *cost.PaymentManager, runner *sandbox.SandboxRunner) *A2AServer {
	if pm == nil {
		pm = cost.NewPaymentManager(escrowAddr)
	}

	card := AgentCard{
		Name:        "kubona-agent-collective",
		Version:     "0.2.0-ts-go",
		Description: "Autonomous Quantum-Classical Research Agent Node with Post-Quantum Cryptography",
		Capabilities: []string{
			"variational_ansatz_optimization",
			"circuit_cutting_recombination",
			"zne_error_mitigation",
			"molecular_ground_state_pyscf",
			"krylov_subspace_ground_state",
			"qir_llvm_lowering",
		},
		SupportedTokens:   []string{"USDC"},
		RateUSDPerJob:     0.05,
		SupportedProfiles: []string{"qir-llvm-v1", "openqasm-3.0", "pyscf-fci"},
		EscrowAddress:     escrowAddr,
	}

	return &A2AServer{
		card:       card,
		tasks:      make(map[string]*A2ATask),
		paymentMgr: pm,
		runner:     runner,
	}
}

// SetTaskExecutor attaches the ledger-bound computation backend. Until an
// executor is attached, submitted tasks remain queued — never completed with
// invented results.
func (s *A2AServer) SetTaskExecutor(exec TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = exec
}

// SetAutonomyChecker attaches live tenant-autonomy enforcement at dispatch.
// The checker is consulted in runTask before any execution; L0 (or unknown)
// tenants settle failed instead of running.
func (s *A2AServer) SetAutonomyChecker(checker func(tenantID string) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autonomyChecker = checker
}

// GetAgentCard returns the static agent capabilities card
func (s *A2AServer) GetAgentCard() AgentCard {
	return s.card
}

// PaymentManager returns the attached payment manager
func (s *A2AServer) PaymentManager() *cost.PaymentManager {
	return s.paymentMgr
}

// rpcError builds a JSON-RPC error response.
func rpcError(id any, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
}

// RevokeTask transitions a queued or running task to revoked. Revocation is
// terminal and propagates: children of a revoked parent cannot be created
// (SubmitTask refuses) and in-flight runs fence on the flag in runTask.
// Completed tasks are history and cannot be revoked.
func (s *A2AServer) RevokeTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}
	switch task.Status {
	case TaskStatusQueued, TaskStatusRunning:
		task.Status = TaskStatusRevoked
		task.UpdatedAt = time.Now().UTC()
		return nil
	default:
		return fmt.Errorf("task %s is %s: only queued/running tasks can be revoked", taskID, task.Status)
	}
}

// HandleRequest processes an incoming JSON-RPC request
func (s *A2AServer) HandleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	if req.JSONRPC != "2.0" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    -32600,
				Message: "Invalid Request: jsonrpc must be '2.0'",
			},
		}
	}

	switch req.Method {
	case "a2a.GetAgentCard":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  s.card,
		}

	case "a2a.SubmitTask":
		var params TaskSubmitParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid params: %v", err),
				},
			}
		}

		// Delegation lineage: resolve the parent edge server-side. The child
		// inherits at most the parent's scopes (narrowing only, never
		// widening — no privilege escalation through creation) and one
		// deeper depth. Dead or revoked parents cannot spawn.
		depth := 0
		parentScopes := append([]string(nil), s.card.Capabilities...)
		var parentTaskID string
		if params.ParentTaskID != "" {
			s.mu.RLock()
			pt, ok := s.tasks[params.ParentTaskID]
			s.mu.RUnlock()
			if !ok {
				return rpcError(req.ID, -32001, fmt.Sprintf("parent task not found: %s", params.ParentTaskID))
			}
			if pt.Status == TaskStatusFailed || pt.Status == TaskStatusRevoked {
				return rpcError(req.ID, -32602, fmt.Sprintf("parent task %s is %s: delegation refused", pt.ID, pt.Status))
			}
			depth = pt.Depth + 1
			if depth > MaxDelegationDepth {
				return rpcError(req.ID, -32602, fmt.Sprintf("delegation depth %d exceeds maximum %d", depth, MaxDelegationDepth))
			}
			parentScopes = append([]string(nil), pt.Scopes...)
			parentTaskID = pt.ID
		}
		scopes := params.Scopes
		if len(scopes) == 0 {
			scopes = append([]string(nil), parentScopes...)
		} else {
			allowed := make(map[string]bool, len(parentScopes))
			for _, sc := range parentScopes {
				allowed[sc] = true
			}
			for _, sc := range scopes {
				if !allowed[sc] {
					return rpcError(req.ID, -32602, fmt.Sprintf("scope %q exceeds parent delegation: refused", sc))
				}
			}
		}

		// Sub-mandate bound: a child task may never inherit more budget than its
		// verified parent authorizes. The parent mandate signature, expiry and
		// ceiling are verified here; the bound is recorded on the task.
		var parentJobHash string
		tenantID := "default"
		if params.ParentMandate != nil {
			if err := params.ParentMandate.Verify("", "", params.MaxBudgetUSD); err != nil {
				return &JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &JSONRPCError{
						Code:    -32602,
						Message: fmt.Sprintf("invalid parent mandate (sub-mandate refused): %v", err),
					},
				}
			}
			parentJobHash = params.ParentMandate.JobHash
			if params.ParentMandate.TenantID != "" {
				tenantID = params.ParentMandate.TenantID
			}
		}

		taskID := "task-" + uuid.New().String()[:8]
		task := &A2ATask{
			ID:            taskID,
			TaskType:      params.TaskType,
			Status:        TaskStatusQueued,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
			MaxBudgetUSD:  params.MaxBudgetUSD,
			ParentJobHash: parentJobHash,
			Scopes:        scopes,
			Depth:         depth,
			ParentTaskID:  parentTaskID,
			TenantID:      tenantID,
		}

		s.mu.Lock()
		s.tasks[taskID] = task
		exec := s.executor
		s.mu.Unlock()

		// Asynchronous ledger-bound execution. Without an executor the task
		// honestly remains queued; results are only ever written by runTask.
		if exec != nil {
			go s.runTask(taskID, params.TaskType, params.Payload, exec)
		}

		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  task,
		}

	case "a2a.GetTaskStatus":
		var params TaskStatusParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid params: %v", err),
				},
			}
		}

		s.mu.RLock()
		task, exists := s.tasks[params.TaskID]
		s.mu.RUnlock()

		if !exists {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32001,
					Message: fmt.Sprintf("Task not found: %s", params.TaskID),
				},
			}
		}

		// Return a copy: the stored task may be updated concurrently by runTask.
		taskCopy := *task
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  &taskCopy,
		}

	case "a2a.RevokeTask":
		var params TaskStatusParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcError(req.ID, -32602, fmt.Sprintf("Invalid params: %v", err))
		}
		if err := s.RevokeTask(params.TaskID); err != nil {
			return rpcError(req.ID, -32001, err.Error())
		}
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]string{"task_id": params.TaskID, "status": string(TaskStatusRevoked)},
		}

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

// runTask executes a queued task through the ledger-bound executor and writes
// the outcome back onto the stored task. Only this path may transition a task
// to completed/failed, and only with the executor's actual result.
func (s *A2AServer) runTask(taskID, taskType string, payload json.RawMessage, exec TaskExecutor) {
	s.mu.Lock()
	task, exists := s.tasks[taskID]
	if !exists {
		s.mu.Unlock()
		return
	}
	if task.Status == TaskStatusRevoked {
		// Revoked while queued: never run.
		s.mu.Unlock()
		return
	}
	checker := s.autonomyChecker
	tenantID := task.TenantID
	task.Status = TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()

	// Live autonomy gate: L0 (or unknown) tenants settle failed before any
	// execution. Revocation takes effect on next dispatch — no cached grants.
	if checker != nil {
		level, err := checker(tenantID)
		if err != nil || level == "L0" {
			reason := fmt.Sprintf("tenant %q autonomy %q: dispatch refused", tenantID, level)
			if err != nil {
				reason = fmt.Sprintf("unknown tenant %q: dispatch refused (%v)", tenantID, err)
			}
			s.mu.Lock()
			if t, ok := s.tasks[taskID]; ok && t.Status == TaskStatusRunning {
				t.Status = TaskStatusFailed
				t.Error = reason
				t.UpdatedAt = time.Now().UTC()
			}
			s.mu.Unlock()
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := exec.ExecuteTask(ctx, taskType, payload, tenantID)

	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists = s.tasks[taskID]
	if !exists {
		return
	}
	if task.Status == TaskStatusRevoked {
		// Revoked mid-flight: the abort wins; the computed result is dropped
		// (unlike jobs, A2A results are not provenance records themselves —
		// the ledger-bound job underneath retains its own receipt).
		return
	}
	task.UpdatedAt = time.Now().UTC()
	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err.Error()
		return
	}
	task.Status = TaskStatusCompleted
	task.Result = result
}

// A2AClient dispatches outbound JSON-RPC 2.0 requests to remote A2A agent nodes
type A2AClient struct {
	RemoteURL  string
	HTTPClient *http.Client
}

func NewA2AClient(remoteURL string) *A2AClient {
	return &A2AClient{
		RemoteURL:  remoteURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetAgentCard retrieves the remote agent node's capability advertisement
func (c *A2AClient) GetAgentCard(ctx context.Context) (*AgentCard, error) {
	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      uuid.New().String(),
		Method:  "a2a.GetAgentCard",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RemoteURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("A2A error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	cardBytes, _ := json.Marshal(rpcResp.Result)
	var card AgentCard
	if err := json.Unmarshal(cardBytes, &card); err != nil {
		return nil, err
	}
	return &card, nil
}

// SubmitTask delegates a research task to the remote A2A agent node with optional x402 payment proof
func (c *A2AClient) SubmitTask(ctx context.Context, taskType string, payload any, x402Token string) (*A2ATask, error) {
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	paramsRaw, _ := json.Marshal(TaskSubmitParams{
		TaskType: taskType,
		Payload:  payloadRaw,
	})

	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      uuid.New().String(),
		Method:  "a2a.SubmitTask",
		Params:  paramsRaw,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RemoteURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if x402Token != "" {
		req.Header.Set("Authorization", "x402 "+x402Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, fmt.Errorf("x402 payment required: %s", resp.Header.Get("X-Payment-Address"))
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("A2A error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	taskBytes, _ := json.Marshal(rpcResp.Result)
	var task A2ATask
	if err := json.Unmarshal(taskBytes, &task); err != nil {
		return nil, err
	}
	return &task, nil
}
