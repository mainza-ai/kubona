package agents

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/cost"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func TestA2AServer_GetAgentCard(t *testing.T) {
	pm := cost.NewPaymentManager("0x123")
	runner := sandbox.NewSandboxRunnerWithConfig(sandbox.DefaultConfig())
	server := NewA2AServer("0x123", pm, runner)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "a2a.GetAgentCard",
	}

	resp := server.HandleRequest(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	card, ok := resp.Result.(AgentCard)
	if !ok {
		t.Fatalf("result is not AgentCard: %T", resp.Result)
	}

	if card.Name != "kubona-agent-collective" {
		t.Fatalf("unexpected agent name: %s", card.Name)
	}
	if len(card.Capabilities) == 0 {
		t.Fatalf("expected non-empty capabilities")
	}
}

func TestA2AServer_TaskSubmissionAndStatus(t *testing.T) {
	pm := cost.NewPaymentManager("0x123")
	runner := sandbox.NewSandboxRunnerWithConfig(sandbox.DefaultConfig())
	server := NewA2AServer("0x123", pm, runner)

	// 1. Submit task
	params, _ := json.Marshal(TaskSubmitParams{
		TaskType: "circuit_cutting_recombination",
		Scopes:   []string{"circuit_cutting_recombination"}, MaxBudgetUSD: 5.0,
		Payload: json.RawMessage(`{"circuit": "qreg q[4]; h q[0]; cx q[0], q[1];"}`),
	})

	subReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "a2a.SubmitTask",
		Params:  params,
	}

	subResp := server.HandleRequest(subReq)
	if subResp.Error != nil {
		t.Fatalf("SubmitTask error: %+v", subResp.Error)
	}

	task, ok := subResp.Result.(*A2ATask)
	if !ok {
		t.Fatalf("result is not *A2ATask: %T", subResp.Result)
	}

	// Without an executor attached, the task must honestly remain queued —
	// never completed with an invented result.
	if task.ID == "" || task.Status != TaskStatusQueued {
		t.Fatalf("expected queued task without executor, got: %+v", task)
	}

	// 2. Query task status
	statusParams, _ := json.Marshal(TaskStatusParams{TaskID: task.ID})
	statusReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "a2a.GetTaskStatus",
		Params:  statusParams,
	}

	statusResp := server.HandleRequest(statusReq)
	if statusResp.Error != nil {
		t.Fatalf("GetTaskStatus error: %+v", statusResp.Error)
	}

	queriedTask, ok := statusResp.Result.(*A2ATask)
	if !ok || queriedTask.ID != task.ID {
		t.Fatalf("queried task mismatch: %+v", statusResp.Result)
	}
}

func TestA2AServer_MethodNotFound(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      99,
		Method:  "unknown.method",
	}

	resp := server.HandleRequest(req)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected MethodNotFound error (-32601), got %+v", resp.Error)
	}
}

// fakeExecutor is a deterministic stand-in for the ledger-bound backend.
type fakeExecutor struct {
	result map[string]any
	err    error
}

func (f *fakeExecutor) ExecuteTask(ctx context.Context, taskType string, payload json.RawMessage, tenantID string) (map[string]any, error) {
	return f.result, f.err
}

// blockingExecutor holds task execution until released, making
// in-flight states deterministic under test (no worker race).
type blockingExecutor struct {
	release chan struct{}
	result  map[string]any
}

func (f *blockingExecutor) ExecuteTask(ctx context.Context, taskType string, payload json.RawMessage, tenantID string) (map[string]any, error) {
	select {
	case <-f.release:
		return f.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func submitTask(t *testing.T, server *A2AServer, params TaskSubmitParams, id any) *A2ATask {
	t.Helper()
	raw, _ := json.Marshal(params)
	resp := server.HandleRequest(&JSONRPCRequest{JSONRPC: "2.0", ID: id, Method: "a2a.SubmitTask", Params: raw})
	if resp.Error != nil {
		t.Fatalf("SubmitTask error: %+v", resp.Error)
	}
	task, ok := resp.Result.(*A2ATask)
	if !ok {
		t.Fatalf("result is not *A2ATask: %T", resp.Result)
	}
	return task
}

func pollTaskStatus(t *testing.T, server *A2AServer, taskID string) *A2ATask {
	t.Helper()
	raw, _ := json.Marshal(TaskStatusParams{TaskID: taskID})
	for i := 0; i < 100; i++ {
		resp := server.HandleRequest(&JSONRPCRequest{JSONRPC: "2.0", ID: 1000 + i, Method: "a2a.GetTaskStatus", Params: raw})
		if resp.Error != nil {
			t.Fatalf("GetTaskStatus error: %+v", resp.Error)
		}
		task := resp.Result.(*A2ATask)
		if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s never settled", taskID)
	return nil
}

func TestA2AServer_AsyncExecutionThroughExecutor(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"energy_hartree": -1.13727, "computed": true}})

	task := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf",
		Scopes:   []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
		Payload: json.RawMessage(`{"molecule":"H2"}`),
	}, 10)
	if task.Status != TaskStatusQueued {
		t.Fatalf("expected queued on submit, got %s", task.Status)
	}

	settled := pollTaskStatus(t, server, task.ID)
	if settled.Status != TaskStatusCompleted {
		t.Fatalf("expected completed, got %+v", settled)
	}
	if settled.Result["energy_hartree"] != -1.13727 {
		t.Fatalf("executor result not propagated: %+v", settled.Result)
	}
}

func TestA2AServer_ExecutorFailureMarksFailed(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{err: errors.New("bridge unavailable")})

	task := submitTask(t, server, TaskSubmitParams{
		TaskType: "zne_error_mitigation",
		Scopes:   []string{"zne_error_mitigation"}, MaxBudgetUSD: 5.0,
		Payload: json.RawMessage(`{"qasm":"OPENQASM 2.0;"}`),
	}, 11)
	settled := pollTaskStatus(t, server, task.ID)
	if settled.Status != TaskStatusFailed || settled.Error == "" {
		t.Fatalf("expected failed with error, got %+v", settled)
	}
}

func TestA2AServer_SubMandateCapEnforced(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"ok": true}})

	pub, priv, _ := ed25519.GenerateKey(nil)
	_ = pub
	parent, err := mandates.SignMandate(priv, "tenant-a", "jobhash-parent", 50.0, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("sign parent: %v", err)
	}

	// Within budget: accepted and bound recorded.
	task := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes:       []string{"molecular_ground_state_pyscf"},
		MaxBudgetUSD: 25.0, ParentMandate: parent,
	}, 12)
	if task.MaxBudgetUSD != 25.0 || task.ParentJobHash != "jobhash-parent" {
		t.Fatalf("sub-mandate bound not recorded: %+v", task)
	}

	// Over budget: refused at submit time.
	overRaw, _ := json.Marshal(TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes:       []string{"molecular_ground_state_pyscf"},
		MaxBudgetUSD: 75.0, ParentMandate: parent,
	})
	resp := server.HandleRequest(&JSONRPCRequest{JSONRPC: "2.0", ID: 13, Method: "a2a.SubmitTask", Params: overRaw})
	if resp.Error == nil {
		t.Fatalf("expected sub-mandate refusal over parent ceiling, got %+v", resp.Result)
	}

	// Tampered parent signature: refused.
	tampered := *parent
	tampered.SignatureHex = parent.SignatureHex[:len(parent.SignatureHex)-2] + "ff"
	tampRaw, _ := json.Marshal(TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes:       []string{"molecular_ground_state_pyscf"},
		MaxBudgetUSD: 10.0, ParentMandate: &tampered,
	})
	resp = server.HandleRequest(&JSONRPCRequest{JSONRPC: "2.0", ID: 14, Method: "a2a.SubmitTask", Params: tampRaw})
	if resp.Error == nil {
		t.Fatalf("expected refusal for tampered parent mandate, got %+v", resp.Result)
	}
}

func submitRaw(t *testing.T, server *A2AServer, method string, params any, id any) *JSONRPCResponse {
	t.Helper()
	raw, _ := json.Marshal(params)
	return server.HandleRequest(&JSONRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: raw})
}

func TestA2ADelegationDepthAndScopes(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"ok": true}})

	root := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf",
		Scopes:   []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
		Payload: json.RawMessage(`{"molecule":"H2"}`),
	}, 20)
	if root.Depth != 0 || len(root.Scopes) != 1 {
		t.Fatalf("root must be depth 0 with exactly its declared scope: %+v", root)
	}

	// W0.5: root without explicit scopes refuses (default-deny) even though
	// the capability is advertised — inheritance is for children only.
	resp := submitRaw(t, server, "a2a.SubmitTask", TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		MaxBudgetUSD: 5.0,
	}, 200)
	if resp.Error == nil {
		t.Fatalf("scope-free root must refuse")
	}

	// Narrowing scopes down the chain is allowed.
	child := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		ParentTaskID: root.ID, Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 4.0,
	}, 21)
	if child.Depth != 1 {
		t.Fatalf("child must be depth 1: %+v", child)
	}
	grandchild := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		ParentTaskID: child.ID, MaxBudgetUSD: 3.0,
	}, 22)
	if grandchild.Depth != 2 || len(grandchild.Scopes) != 1 {
		t.Fatalf("grandchild must inherit narrowed scopes at depth 2: %+v", grandchild)
	}

	// Depth 3 refused.
	resp = submitRaw(t, server, "a2a.SubmitTask", TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		ParentTaskID: grandchild.ID, MaxBudgetUSD: 2.0,
	}, 23)
	if resp.Error == nil {
		t.Fatalf("depth-3 delegation must refuse")
	}

	// Scope widening refused (grandchild only holds one scope).
	resp = submitRaw(t, server, "a2a.SubmitTask", TaskSubmitParams{
		TaskType: "zne_error_mitigation", Payload: json.RawMessage(`{"qasm":"q;"}`),
		ParentTaskID: grandchild.ID, Scopes: []string{"zne_error_mitigation", "qir_llvm_lowering"}, MaxBudgetUSD: 2.0,
	}, 24)
	if resp.Error == nil {
		t.Fatalf("scope escalation must refuse")
	}

	// Unknown parent refused.
	resp = submitRaw(t, server, "a2a.SubmitTask", TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		ParentTaskID: "task-nonexistent", MaxBudgetUSD: 2.0,
	}, 25)
	if resp.Error == nil {
		t.Fatalf("unknown parent must refuse")
	}
}

func TestA2ARevokePropagates(t *testing.T) {
	// Deterministic revoke: the executor blocks until released, so the
	// task cannot settle before the revoke lands (the old instant-result
	// fake raced the worker and flaked under -race).
	release := make(chan struct{})
	blocking := &blockingExecutor{release: release, result: map[string]any{"ok": true}}
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(blocking)

	root := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
	}, 30)
	if err := server.RevokeTask(root.ID); err != nil {
		t.Fatalf("revoke queued task: %v", err)
	}

	// Children of a revoked parent are refused.
	resp := submitRaw(t, server, "a2a.SubmitTask", TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		ParentTaskID: root.ID,
	}, 31)
	if resp.Error == nil {
		t.Fatalf("child of revoked parent must refuse")
	}
	close(release)

	// Revoking a settled task refuses (history is immutable).
	done := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
	}, 32)
	settled := pollTaskStatus(t, server, done.ID)
	if settled.Status != TaskStatusCompleted {
		t.Fatalf("expected completed: %+v", settled)
	}
	if err := server.RevokeTask(done.ID); err == nil {
		t.Fatalf("revoking completed history must refuse")
	}

	// RPC surface mirrors the method.
	rpcResp := submitRaw(t, server, "a2a.RevokeTask", TaskStatusParams{TaskID: "task-nope"}, 33)
	if rpcResp.Error == nil {
		t.Fatalf("revoking unknown task must error")
	}
}

func TestA2AL0TenantRefusesDispatch(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"ok": true}})
	server.SetAutonomyChecker(func(tenantID string) (string, error) {
		return "L0", nil
	})

	task := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
	}, 40)
	settled := pollTaskStatus(t, server, task.ID)
	if settled.Status != TaskStatusFailed {
		t.Fatalf("L0 tenant must settle failed, got %+v", settled)
	}
	if settled.Error == "" {
		t.Fatalf("L0 refusal must carry a reason")
	}
}

func TestA2AUnknownTenantFailsClosed(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"ok": true}})
	server.SetAutonomyChecker(func(tenantID string) (string, error) {
		return "", errors.New("no such tenant")
	})

	task := submitTask(t, server, TaskSubmitParams{
		TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
		Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 5.0,
	}, 41)
	settled := pollTaskStatus(t, server, task.ID)
	if settled.Status != TaskStatusFailed {
		t.Fatalf("unknown tenant must settle failed, got %+v", settled)
	}
}

func TestA2AW05DelegationRail(t *testing.T) {
	server := NewA2AServer("0x123", nil, nil)
	server.SetTaskExecutor(&fakeExecutor{result: map[string]any{"ok": true}})

	pub, priv, _ := ed25519.GenerateKey(nil)
	_ = pub
	signParent := func(tenant, jobHash string, budget float64) *mandates.SpendMandate {
		m, err := mandates.SignMandate(priv, tenant, jobHash, budget, time.Now().UTC().Add(time.Hour))
		if err != nil {
			t.Fatalf("sign parent: %v", err)
		}
		return m
	}
	base := func() TaskSubmitParams {
		return TaskSubmitParams{
			TaskType: "molecular_ground_state_pyscf", Payload: json.RawMessage(`{"molecule":"H2"}`),
			Scopes: []string{"molecular_ground_state_pyscf"}, MaxBudgetUSD: 10.0,
		}
	}

	// Zero budget: refused (delegation without a budget cannot spend).
	bad := base()
	bad.MaxBudgetUSD = 0
	if resp := submitRaw(t, server, "a2a.SubmitTask", bad, 50); resp.Error == nil {
		t.Fatalf("zero-budget delegation must refuse")
	}

	// Empty-tenant parent mandate: refused (authorizes nothing-specific).
	bad = base()
	bad.ParentMandate = signParent("", "jobhash-x", 50.0)
	if resp := submitRaw(t, server, "a2a.SubmitTask", bad, 51); resp.Error == nil {
		t.Fatalf("tenant-less parent mandate must refuse")
	}

	// Empty-jobhash parent mandate: refused.
	bad = base()
	bad.ParentMandate = signParent("tenant-a", "", 50.0)
	if resp := submitRaw(t, server, "a2a.SubmitTask", bad, 52); resp.Error == nil {
		t.Fatalf("job-less parent mandate must refuse")
	}

	// Payload declares more than the task budget: refused (over-delegation).
	bad = base()
	bad.Payload = json.RawMessage(`{"molecule":"H2","max_cost_usd":25.0}`)
	if resp := submitRaw(t, server, "a2a.SubmitTask", bad, 53); resp.Error == nil {
		t.Fatalf("payload cost above task budget must refuse")
	}

	// Scope enforcement at run: granted scope differs from task type —
	// submit narrows honestly, but a mismatched stored scope settles failed.
	// (Submit validates subset; run validates membership.)
	mismatch := base()
	mismatch.Scopes = []string{"zne_error_mitigation"}
	if resp := submitRaw(t, server, "a2a.SubmitTask", mismatch, 54); resp.Error != nil {
		t.Fatalf("in-capability scope must submit: %v", resp.Error)
	} else {
		var task A2ATask
		raw, _ := json.Marshal(resp.Result)
		if err := json.Unmarshal(raw, &task); err != nil {
			t.Fatalf("decode: %v", err)
		}
		settled := pollTaskStatus(t, server, task.ID)
		if settled.Status != TaskStatusFailed {
			t.Fatalf("out-of-scope execution must settle failed, got %+v", settled)
		}
	}
}
