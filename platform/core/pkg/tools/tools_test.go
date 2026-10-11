package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func testRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register(Definition{
		Name:        "get_anchor",
		Description: "Fetch the classical anchor for a molecule.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"molecule": map[string]any{"type": "string"}},
			"required":   []any{"molecule"},
		},
		Capability: "anchor:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct {
				Molecule string `json:"molecule"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			return map[string]any{"molecule": a.Molecule, "fci": -1.13727}, nil
		},
	})
	_ = r.Allow("copilot", "get_anchor")
	return r
}

func TestDispatchHappyPath(t *testing.T) {
	r := testRegistry()
	out, err := r.Dispatch(context.Background(), "copilot", Call{ID: "1", Name: "get_anchor", Arguments: `{"molecule":"H2"}`})
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || m["molecule"] != "H2" {
		t.Errorf("unexpected observation: %v", out)
	}
}

func TestDispatchRefusals(t *testing.T) {
	r := testRegistry()
	cases := []struct {
		name string
		role string
		call Call
		want string
	}{
		{"unknown tool", "copilot", Call{Name: "nope", Arguments: `{}`}, "unknown tool"},
		{"role not allowed", "stranger", Call{Name: "get_anchor", Arguments: `{"molecule":"H2"}`}, "may not call"},
		{"missing required", "copilot", Call{Name: "get_anchor", Arguments: `{}`}, "missing required"},
		{"bad type", "copilot", Call{Name: "get_anchor", Arguments: `{"molecule":42}`}, "must be a string"},
		{"not json", "copilot", Call{Name: "get_anchor", Arguments: `{{{`}, "not valid JSON"},
		{"not object", "copilot", Call{Name: "get_anchor", Arguments: `[1]`}, "must be a JSON object"},
	}
	for _, tc := range cases {
		if _, err := r.Dispatch(context.Background(), tc.role, tc.call); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want refusal containing %q", tc.name, err, tc.want)
		}
	}
}

func TestAllowUnknownToolRefuses(t *testing.T) {
	r := NewRegistry()
	if err := r.Allow("copilot", "ghost"); err == nil {
		t.Error("allowing an unknown tool must refuse")
	}
}

func TestDefinitionsForLLMScoped(t *testing.T) {
	r := testRegistry()
	defs := r.DefinitionsForLLM("copilot")
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool for copilot, got %d", len(defs))
	}
	fn, _ := defs[0]["function"].(map[string]any)
	if fn["name"] != "get_anchor" {
		t.Errorf("unexpected tool name: %v", fn["name"])
	}
	if len(r.DefinitionsForLLM("stranger")) != 0 {
		t.Error("strangers must see zero tools (deny by default)")
	}
}

func TestDuplicateRegisterRefuses(t *testing.T) {
	r := testRegistry()
	err := r.Register(Definition{Name: "get_anchor", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
	if err == nil {
		t.Error("duplicate registration must refuse")
	}
}
