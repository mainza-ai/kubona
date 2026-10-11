package api

import (
	"context"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func TestCopilotToolRegistration(t *testing.T) {
	s := &Server{}
	r := s.registerAgentTools()
	defs := r.DefinitionsForLLM(CopilotRole)
	want := map[string]bool{
		"get_anchor": false, "get_campaign": false, "list_jobs": false,
		"get_job": false, "recent_events": false, "query_literature": false,
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
