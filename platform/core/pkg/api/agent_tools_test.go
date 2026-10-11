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
		"submit_local_sim": false, "recall_memory": false,
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

func TestRecallMemoryTool(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	// Unknown session: tenant falls back to default, empty store reads
	// return empty snippets — never an error, never invented memories.
	out, err := r.Dispatch(context.Background(), CopilotRole,
		mustCall("recall_memory", `{"session_id":"missing","query":"H2 anchor"}`))
	if err != nil {
		t.Fatalf("recall on empty store must succeed empty, got %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("unexpected shape: %T", out)
	}
	if snips, ok := m["snippets"].([]any); !ok || len(snips) != 0 {
		t.Errorf("expected empty snippets, got %v", m["snippets"])
	}
	// Missing query refuses (required field).
	if _, err := r.Dispatch(context.Background(), CopilotRole,
		mustCall("recall_memory", `{"session_id":"s"}`)); err == nil {
		t.Error("missing query must refuse")
	}
	// Turn roles may recall; nobody unlisted may.
	for _, role := range []string{"ansatz-scout", "epistemic-scribe"} {
		seen := false
		for _, d := range r.DefinitionsForLLM(role) {
			fn, _ := d["function"].(map[string]any)
			if fn["name"] == "recall_memory" {
				seen = true
			}
		}
		if !seen {
			t.Errorf("role %q must see recall_memory", role)
		}
	}
}

// The registry is the only enforcement plane: tribunal roles observe
// read-only (submit_local_sim is copilot-only), unknown tools and unknown
// roles are denied before any handler runs.
func TestRegistryReplacesAllowLists(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	ctx := context.Background()

	defNames := func(role string) map[string]bool {
		out := map[string]bool{}
		for _, d := range r.DefinitionsForLLM(role) {
			fn, _ := d["function"].(map[string]any)
			name, _ := fn["name"].(string)
			out[name] = true
		}
		return out
	}
	copilot := defNames(CopilotRole)
	if !copilot["submit_local_sim"] {
		t.Error("copilot must be granted submit_local_sim")
	}
	for _, role := range []string{"ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe"} {
		defs := defNames(role)
		if defs["submit_local_sim"] {
			t.Errorf("tribunal role %s must not be granted submit_local_sim", role)
		}
		if !defs["recall_memory"] {
			t.Errorf("tribunal role %s must be granted recall_memory", role)
		}
		if _, err := r.Dispatch(ctx, role, mustCall("submit_local_sim", `{}`)); err == nil ||
			!strings.Contains(err.Error(), "may not call") {
			t.Errorf("tribunal role %s submit must be denied at the gate, got: %v", role, err)
		}
	}

	if _, err := r.Dispatch(ctx, CopilotRole, mustCall("sign_spend_mandate", `{}`)); err == nil ||
		!strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("unknown tool must be denied even for copilot, got: %v", err)
	}
	if _, err := r.Dispatch(ctx, "ghost-role", mustCall("get_anchor", `{}`)); err == nil {
		t.Error("unknown role must be denied everything")
	}
}
