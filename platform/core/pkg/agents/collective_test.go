package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func TestAgentCollectiveManifests(t *testing.T) {
	ac := NewAgentCollective()

	// Verify all 5 personas exist
	roles := ac.ListRoles()
	if len(roles) != 5 {
		t.Fatalf("expected 5 personas, got %d", len(roles))
	}

	// Epistemic scribe can evaluate wavefunctions but cannot sign mandates
	auth, msg := ac.AuthorizeTool(RoleEpistemicScribe, "evaluate_wavefunction")
	if !auth {
		t.Fatalf("expected epistemic-scribe to be authorized for evaluate_wavefunction, got: %s", msg)
	}

	auth, _ = ac.AuthorizeTool(RoleEpistemicScribe, "sign_spend_mandate")
	if auth {
		t.Fatalf("epistemic-scribe must NOT be authorized for sign_spend_mandate")
	}

	// Human reviewer holds sign_spend_mandate
	auth, _ = ac.AuthorizeTool(RoleHumanReviewer, "sign_spend_mandate")
	if !auth {
		t.Fatalf("expected human-reviewer to be authorized for sign_spend_mandate")
	}

	// Disconfirmation officer can compute classical anchors
	auth, _ = ac.AuthorizeTool(RoleDisconfirmationOfficer, "compute_classical_anchor")
	if !auth {
		t.Fatalf("expected disconfirmation-officer to be authorized for compute_classical_anchor")
	}
}

func TestExecuteTribunal(t *testing.T) {
	ac := NewAgentCollective()
	res, err := ac.ExecuteTribunal(t.Context(), nil, nil, "Investigate H2 ground state")
	if err != nil {
		t.Fatalf("ExecuteTribunal failed: %v", err)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(res.Turns))
	}
	if !res.ConsensusPassed {
		t.Fatalf("expected consensus to pass")
	}
}

func TestExecuteTribunalWithMemory(t *testing.T) {
	ac := NewAgentCollective()
	store := memory.NewHindsightStore()

	// Seed relevant past experience into memory
	store.Retain(memory.BankExperiences, "Past trial for LiH observed shallow 2-qubit ansatz converges within 1.2 mHa without barren plateaus", 0.95)

	res, err := ac.ExecuteTribunalWithMemory(t.Context(), nil, nil, store, "Investigate LiH molecule ground state")
	if err != nil {
		t.Fatalf("ExecuteTribunalWithMemory failed: %v", err)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(res.Turns))
	}
	if !res.ConsensusPassed {
		t.Fatalf("expected consensus to pass")
	}

	// Verify that the tribunal consensus was retained into Hindsight memory
	recalled := store.SemanticRecall(memory.BankExperiences, "LiH consensus", 200, 0.3)
	if len(recalled) == 0 {
		t.Fatalf("expected consensus to be retained in Hindsight memory")
	}
	foundConsensus := false
	for _, r := range recalled {
		if strings.Contains(r.Item.Content, "Tribunal consensus for LiH") {
			foundConsensus = true
			break
		}
	}
	if !foundConsensus {
		t.Fatalf("expected to find consensus memory record in store")
	}
}

func TestObserveTurnPassthrough(t *testing.T) {
	ac := NewAgentCollective()
	base := []llm.ChatMessage{{Role: "user", Content: "hi"}}
	if out := ac.observeTurn(context.Background(), nil, "ansatz-scout", base); len(out) != 1 {
		t.Error("nil client must return base unchanged")
	}
}

func TestObserveTurnDispatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		last, _ := msgs[len(msgs)-1].(map[string]any)
		if obs, ok := last["content"].(string); ok && strings.Contains(obs, "get_anchor") {
			// Observation round: answer from the observed value.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"message":       map[string]any{"content": "anchor observed: -1.13727"},
					"finish_reason": "stop",
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"content": "",
					"tool_calls": []any{map[string]any{
						"id":   "call_9",
						"type": "function",
						"function": map[string]any{
							"name":      "get_anchor",
							"arguments": `{"molecule":"H2"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		})
	}))
	defer srv.Close()

	t.Setenv("OPENCODE_API_KEY", "test-key")
	settings := llm.DefaultSettings()
	snap := settings.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOpenCode] = pc
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	// Point the scout role at the stub server.
	snap.RoleAssignments["ansatz-scout"] = llm.RoleModelAssignment{
		Role: "ansatz-scout", Provider: llm.ProviderOpenCode, Model: "stub",
	}
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	cli := llm.NewClient(settings)

	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name:        "get_anchor",
		Description: "test anchor",
		Parameters:  map[string]any{"type": "object"},
		Capability:  "anchor:read",
		ReadOnly:    true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"fci": -1.13727}, nil
		},
	})
	_ = reg.Allow("ansatz-scout", "get_anchor")
	ac := NewAgentCollective()
	ac.Tools = reg

	out := ac.observeTurn(context.Background(), cli, "ansatz-scout", []llm.ChatMessage{
		{Role: "system", Content: "s"},
		{Role: "user", Content: "anchor for H2?"},
	})
	// base(2) + assistant-tool-call + tool observation
	if len(out) != 4 {
		t.Fatalf("expected augmented history of 4, got %d", len(out))
	}
	if out[2].Role != "assistant" || len(out[2].ToolCalls) != 1 {
		t.Errorf("expected assistant tool-call message, got %+v", out[2])
	}
	if out[3].Role != "tool" || !strings.Contains(out[3].Content, "-1.13727") {
		t.Errorf("expected observation carrying the tool result, got %q", out[3].Content)
	}
}
