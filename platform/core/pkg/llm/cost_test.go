package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// K4: pricing is heuristic and labeled; unknown/local never prices.
func TestEstimateCostUSD(t *testing.T) {
	// Local providers: no spend, never priced.
	for _, p := range []llm.ProviderType{llm.ProviderOMLX, llm.ProviderOllama, llm.ProviderLMStudio} {
		if cost, ok := llm.EstimateCostUSD(p, "anything", 10000, 5000); ok || cost != 0 {
			t.Errorf("local provider %s must not price, got %v,%v", p, cost, ok)
		}
	}
	// Unknown cloud model: refuses instead of inventing a rate.
	if cost, ok := llm.EstimateCostUSD(llm.ProviderOpenCode, "mystery-model-2099", 1000, 1000); ok || cost != 0 {
		t.Errorf("unknown model must not price, got %v,%v", cost, ok)
	}
	// Known model: exact arithmetic on the table (1000 in + 500 out).
	cost, ok := llm.EstimateCostUSD(llm.ProviderOpenCode, "deepseek-v4.1-flash", 1000, 500)
	if !ok {
		t.Fatal("known model must price")
	}
	want := 0.0001*1000.0/1000.0 + 0.0004*500.0/1000.0
	if diff := cost - want; diff > 1e-12 || diff < -1e-12 {
		t.Errorf("cost math wrong: got %v want %v", cost, want)
	}
	if cost <= 0 {
		t.Error("priced call must cost something")
	}
}

// K4: the hook fires once per cloud completion with measured usage, and
// never for local providers.
func TestOnSpendHookFiresCloudOnly(t *testing.T) {
	usage := map[string]any{"prompt_tokens": float64(120), "completion_tokens": float64(30), "total_tokens": float64(150)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": usage,
		})
	}))
	defer srv.Close()

	// Cloud path: hook fires with measured numbers.
	c := llm.NewClient(toolTestSettings(t, srv.URL))
	var got *llm.SpendRecord
	c.OnSpend = func(rec llm.SpendRecord) {
		r := rec
		got = &r
	}
	if _, _, _, err := c.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got == nil {
		t.Fatal("hook must fire for cloud completions")
	}
	if string(got.Provider) != "opencode" || got.PromptTokens != 120 || got.CompletionTokens != 30 || !got.UsageReported {
		t.Errorf("hook record wrong: %+v", got)
	}

	// Local path: same wire, no hook. Point omlx at the fake server and
	// assign the role to it.
	s := toolTestSettings(t, srv.URL)
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderOMLX]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOMLX] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Provider: llm.ProviderOMLX, Model: "local-model"}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	c2 := llm.NewClient(s)
	fired := false
	c2.OnSpend = func(llm.SpendRecord) { fired = true }
	if _, _, _, err := c2.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("local complete: %v", err)
	}
	if fired {
		t.Error("hook must never fire for local providers")
	}
}
