package agents

import (
	"encoding/json"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/cost"
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
		Payload:  json.RawMessage(`{"circuit": "qreg q[4]; h q[0]; cx q[0], q[1];"}`),
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

	if task.ID == "" || task.Status != TaskStatusCompleted {
		t.Fatalf("unexpected task state: %+v", task)
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
