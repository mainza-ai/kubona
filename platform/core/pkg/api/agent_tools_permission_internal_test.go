package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func TestToolCallDigestCanonicalizesArguments(t *testing.T) {
	h1, p1, _ := toolCallDigest("run_vqe", `{"b":2,"a":1}`)
	h2, p2, _ := toolCallDigest("run_vqe", `{ "a": 1, "b": 2 }`)
	if h1 != h2 || p1 != p2 {
		t.Fatalf("key order/whitespace changed the digest: %s/%s vs %s/%s", h1, p1, h2, p2)
	}
	h3, _, _ := toolCallDigest("run_vqe", `{"a":1,"b":3}`)
	if h1 == h3 {
		t.Fatal("different arguments share a digest")
	}
	h4, _, _ := toolCallDigest("other_tool", `{"a":1,"b":2}`)
	if h1 == h4 {
		t.Fatal("tool name is not bound into the digest")
	}
	_, preview, truncated := toolCallDigest("chatty", `{"blob":"`+strings.Repeat("x", 5000)+`"}`)
	if !truncated || len(preview) > 4200 || !strings.Contains(preview, "(truncated)") {
		t.Fatalf("large preview must be marked truncated, got len=%d truncated=%v", len(preview), truncated)
	}
}

func TestAskGateRequestsThenSpendsOneApproval(t *testing.T) {
	srv, l, cleanup := loopTestServer(t, "test_tool_gate.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("gate drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	policy := autonomy.DefaultPolicy()
	policy.ToolPermissions = map[string]string{"write_tool": "ask"}

	args := `{"x":1}`
	if err := srv.gateToolDispatch(sess.ID, policy, nil, "write_tool", args); err == nil ||
		!strings.Contains(err.Error(), "one-time approval") {
		t.Fatalf("ask must hold the call, got %v", err)
	}
	if n := countEvents(t, l, sess.ID, "tool_approval_requested"); n != 1 {
		t.Fatalf("one request event, got %d", n)
	}
	items, _ := l.ListToolApprovals(sess.ID, 10)
	if len(items) != 1 || items[0].Status != "pending" {
		t.Fatalf("pending request missing: %+v", items)
	}

	// A retry with identical arguments reuses the pending row: no inbox flood.
	if err := srv.gateToolDispatch(sess.ID, policy, nil, "write_tool", `{ "x": 1 }`); err == nil {
		t.Fatal("unapproved retry must refuse")
	}
	if items, _ := l.ListToolApprovals(sess.ID, 10); len(items) != 1 {
		t.Fatalf("duplicate request flood: %+v", items)
	}
	if n := countEvents(t, l, sess.ID, "tool_approval_requested"); n != 1 {
		t.Fatalf("duplicate request event, got %d", n)
	}

	if _, err := l.DecideToolApproval(sess.ID, items[0].ID, "approve", "reviewed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := srv.gateToolDispatch(sess.ID, policy, nil, "write_tool", args); err != nil {
		t.Fatalf("approved call refused: %v", err)
	}
	if n := countEvents(t, l, sess.ID, "tool_approval_consumed"); n != 1 {
		t.Fatalf("one consumption event, got %d", n)
	}
	// The grant is one-shot: the identical call now needs its own approval,
	// and a different argument shape was never approved.
	if err := srv.gateToolDispatch(sess.ID, policy, nil, "write_tool", args); err == nil {
		t.Fatal("a consumed approval was reused")
	}
	if err := srv.gateToolDispatch(sess.ID, policy, nil, "write_tool", `{"x":2}`); err == nil {
		t.Fatal("an unapproved argument shape passed")
	}

	// Denial is durable and unspendable.
	items, _ = l.ListToolApprovals(sess.ID, 10)
	var deniedID string
	for _, item := range items {
		if item.Status == "pending" {
			if _, err := l.DecideToolApproval(sess.ID, item.ID, "deny", "no", "test"); err != nil {
				t.Fatal(err)
			}
			deniedID = item.ID
			break
		}
	}
	if deniedID == "" {
		t.Fatal("no pending request to deny")
	}
	denied, _ := l.GetToolApproval(sess.ID, deniedID)
	if denied.Status != "denied" || denied.Reason != "no" {
		t.Fatalf("denial not durable: %+v", denied)
	}
	if _, ok, _ := l.ConsumeToolApproval(sess.ID, "write_tool", denied.ArgsHash); ok {
		t.Fatal("a denied request was spendable")
	}
	if _, err := l.DecideToolApproval(sess.ID, deniedID, "approve", "", "test"); err == nil {
		t.Fatal("a denied request was reopened")
	}
}

func TestAskPermissionHoldsThenDispatchesAfterApproval(t *testing.T) {
	srv, l, cleanup := loopTestServer(t, "test_tool_ask_loop.db")
	defer cleanup()

	cli, closeStub := stubToolServer(t, "ask_tool", nil)
	srv.llmClient = cli

	dispatches := 0
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name: "ask_tool", Description: "needs approval",
		Parameters: map[string]any{"type": "object"},
		Capability: "test:write", ReadOnly: false,
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			dispatches++
			return map[string]any{"ok": true}, nil
		},
	})
	_ = reg.Allow(CopilotRole, "ask_tool")
	srv.toolRegistry = reg

	sess, err := srv.sessionMgr.CreateSession("ask drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	policy := autonomy.DefaultPolicy()
	policy.ToolPermissions = map[string]string{"ask_tool": "ask"}
	raw, _ := json.Marshal(policy)
	if err := l.SetAutonomyPolicy(sess.ID, string(raw)); err != nil {
		t.Fatal(err)
	}

	srv.resolveWithTools(context.Background(), sess.ID,
		[]llm.ChatMessage{{Role: "user", Content: "go"}})
	closeStub()
	if dispatches != 0 {
		t.Fatalf("ask dispatch before approval: %d", dispatches)
	}
	items, _ := l.ListToolApprovals(sess.ID, 10)
	if len(items) != 1 || items[0].Tool != "ask_tool" || items[0].Status != "pending" {
		t.Fatalf("request not recorded: %+v", items)
	}
	if _, err := l.DecideToolApproval(sess.ID, items[0].ID, "approve", "", "test"); err != nil {
		t.Fatal(err)
	}

	cli2, closeStub2 := stubToolServer(t, "ask_tool", nil)
	defer closeStub2()
	srv.llmClient = cli2
	srv.resolveWithTools(context.Background(), sess.ID,
		[]llm.ChatMessage{{Role: "user", Content: "go"}})
	if dispatches != 1 {
		t.Fatalf("approved call must dispatch exactly once, got %d", dispatches)
	}
	if n := countEvents(t, l, sess.ID, "tool_approval_consumed"); n != 1 {
		t.Fatalf("consumption event = %d", n)
	}
	// The stub asks again on its second round, but that round has already
	// consumed untrusted output: the taint gate refuses the effectful tool
	// before it can even ask for a second approval. One grant, one dispatch.
	items, _ = l.ListToolApprovals(sess.ID, 10)
	pending := 0
	for _, item := range items {
		if item.Status == "pending" {
			pending++
		}
	}
	if pending != 0 {
		t.Fatalf("no second approval should be requested within a tainted turn, pending=%d %+v", pending, items)
	}
	if dispatches != 1 {
		t.Fatalf("dispatches = %d, want 1", dispatches)
	}
}
