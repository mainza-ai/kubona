package api

import (
	"context"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func mustCall(name, args string) tools.Call {
	return tools.Call{ID: "t1", Name: name, Arguments: args}
}

func TestCopilotToolRegistration(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	defs := r.DefinitionsForLLM(CopilotRole)
	want := map[string]bool{
		"get_anchor": false, "get_campaign": false, "list_jobs": false,
		"get_job": false, "recent_events": false, "query_literature": false,
		"submit_local_sim": false,
	}
	for _, d := range defs {
		fn, _ := d["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if _, ok := want[name]; ok {
			want[name] = true
		} else {
			t.Errorf("unexpected tool exposed to copilot: %q", name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("copilot missing tool %q", name)
		}
	}
	if len(r.DefinitionsForLLM("stranger")) != 0 {
		t.Error("unlisted roles must see zero tools")
	}
}

func TestToolDispatchRefusalsNoDeps(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	ctx := context.Background()
	if _, err := r.Dispatch(ctx, CopilotRole, tools.Call{Name: "ghost", Arguments: `{}`}); err == nil {
		t.Error("unknown tool must refuse")
	}
	if _, err := r.Dispatch(ctx, "stranger", tools.Call{Name: "get_anchor", Arguments: `{"molecule":"H2"}`}); err == nil {
		t.Error("unauthorized role must refuse")
	}
	// Nil bridge: honest refusal, never invented numbers, never a panic.
	_, err := r.Dispatch(ctx, CopilotRole, tools.Call{Name: "get_anchor", Arguments: `{"molecule":"H2"}`})
	if err == nil || !strings.Contains(err.Error(), "bridge unavailable") {
		t.Errorf("expected bridge-unavailable refusal, got %v", err)
	}
}

func TestResolveWithToolsDegrades(t *testing.T) {
	base := []llm.ChatMessage{{Role: "user", Content: "hi"}}
	s := &Server{}
	if out := s.resolveWithTools(context.Background(), "sess", base); len(out) != len(base) {
		t.Error("nil registry must return base unchanged")
	}
	s.toolRegistry = s.registerAgentTools()
	if out := s.resolveWithTools(context.Background(), "sess", base); len(out) != len(base) {
		t.Error("nil LLM client must return base unchanged")
	}
}

func TestSubmitLocalSimTool(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	defs := r.DefinitionsForLLM(CopilotRole)
	found := false
	for _, d := range defs {
		fn, _ := d["function"].(map[string]any)
		if fn["name"] == "submit_local_sim" {
			found = true
		}
	}
	if !found {
		t.Fatal("copilot must see submit_local_sim")
	}
	for _, role := range []string{"ansatz-scout", "device-auditor"} {
		for _, d := range r.DefinitionsForLLM(role) {
			fn, _ := d["function"].(map[string]any)
			if fn["name"] == "submit_local_sim" {
				t.Errorf("turn role %q must never see the submit tool", role)
			}
		}
	}
	// Empty QASM refuses before any runner is touched (nil-safe: the
	// refusal happens on validation, so no jobRunner is needed).
	_, err := r.Dispatch(context.Background(), CopilotRole, mustCall("submit_local_sim", `{"session_id":"s","qasm":"  "}`))
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected empty-QASM refusal, got %v", err)
	}
	_, err = r.Dispatch(context.Background(), CopilotRole, mustCall("submit_local_sim", `{"session_id":"s","qasm":"OPENQASM 2.0;","shots":99999}`))
	if err == nil || !strings.Contains(err.Error(), "capped at 4096") {
		t.Errorf("expected shots-cap refusal, got %v", err)
	}
}
