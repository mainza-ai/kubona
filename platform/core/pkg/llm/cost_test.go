package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// K4: pricing is heuristic and labeled; unknown/local never prices.
func TestEstimateCostUSD(t *testing.T) {
	// Local providers: no spend, never priced.
	for _, p := range []llm.ProviderType{llm.ProviderOMLX, llm.ProviderOllama, llm.ProviderLMStudio, llm.ProviderStrata} {
		if cost, ok := llm.EstimateCostUSD(p, "anything", 10000, 5000); ok || cost != 0 {
			t.Errorf("local provider %s must not price, got %v,%v", p, cost, ok)
		}
		if !llm.IsLocalProvider(p) {
			t.Errorf("IsLocalProvider(%s) must be true", p)
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

// Audit 2026-10-21: the spend-silence paths carried inline oMLX/Ollama/
// LMStudio triples that missed ProviderStrata (the default). Strata
// completions filed spend records, and usage-omitted Strata calls with
// MaxTokens=0 refused as "blind cloud spend" for a free local engine.
// Pins: hook silence + no-refusal without usage, same as oMLX above.
func TestStrataSpendSilence(t *testing.T) {
	// Same wire as the cloud test, but the role resolves to strata and
	// the endpoint reports NO usage at all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "hi"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	s := toolTestSettings(t, srv.URL)
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderStrata]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderStrata] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Provider: llm.ProviderStrata, Model: "iq3_s"}
	snap.MaxTokens = 0 // default: no ceiling configured
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	c := llm.NewClient(s)
	fired := false
	c.OnSpend = func(llm.SpendRecord) { fired = true }
	if _, _, _, err := c.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("strata complete without usage must not refuse: %v", err)
	}
	if fired {
		t.Error("hook must never fire for strata (local provider)")
	}
}

// I3: loop iteration bounds price worst-case ceilings per role.
func TestLoopIterationBound(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "test-key")
	t.Setenv("OPENCODE_MODEL", "deepseek-v4.1-flash")
	t.Setenv("KUBONA_LLM_PROVIDER", "opencode")
	// Defaults: coder/critic unassigned and the active provider force-
	// disabled — nothing billable, bound zero, no fallbacks. Cloud math
	// needs cloud: opt out of the air-gap default like an operator would.
	s := llm.DefaultSettings()
	off := s.GetSnapshot()
	off.AirGapped = false
	offPc := off.Providers[llm.ProviderOpenCode]
	offPc.Enabled = false
	off.Providers[llm.ProviderOpenCode] = offPc
	if err := s.Update(&off, nil); err != nil {
		t.Fatal(err)
	}
	bound, fallbacks, err := llm.LoopIterationBound(s, []string{"coder", "critic"})
	if err != nil {
		t.Fatalf("unassigned roles must not error, got: %v", err)
	}
	if bound != 0 || len(fallbacks) != 0 {
		t.Fatalf("unassigned roles must bind zero, got %v %v", bound, fallbacks)
	}
	// Priced roles sum at ceiling rate (150k tokens, output rate).
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.Enabled = true
	snap.Providers[llm.ProviderOpenCode] = pc
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{Role: "coder", Provider: llm.ProviderOpenCode, Model: "deepseek-v4.1-flash"}
	snap.RoleAssignments["critic"] = llm.RoleModelAssignment{Role: "critic", Provider: llm.ProviderOpenCode, Model: "deepseek-v4.1-flash"}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	bound, fallbacks, err = llm.LoopIterationBound(s, []string{"coder", "critic"})
	if err != nil {
		t.Fatalf("priced roles must bind, got: %v", err)
	}
	want := 2 * 0.0004 * 150000.0 / 1000.0
	if bound != want || len(fallbacks) != 0 {
		t.Fatalf("priced roles must sum ceiling cost, got %v %v (want %v)", bound, fallbacks, want)
	}
	// Unknown metered model fails closed: no bound, no over-reserve —
	// the refusal names the model and the fix.
	snap.RoleAssignments["critic"] = llm.RoleModelAssignment{Role: "critic", Provider: llm.ProviderOpenCode, Model: "mystery-9000"}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := llm.LoopIterationBound(s, []string{"coder", "critic"}); err == nil {
		t.Fatal("unknown model must refuse admission, not over-reserve")
	} else if got := err.Error(); !strings.Contains(got, "mystery-9000") {
		t.Fatalf("refusal must name the model, got: %v", err)
	}
	// Local providers contribute zero even when assigned.
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{Role: "coder", Provider: llm.ProviderOllama, Model: "llama3"}
	snap.RoleAssignments["critic"] = llm.RoleModelAssignment{Role: "critic", Provider: llm.ProviderOllama, Model: "llama3"}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	bound3, fallbacks, err := llm.LoopIterationBound(s, []string{"coder", "critic"})
	if err != nil {
		t.Fatalf("local roles must not error, got: %v", err)
	}
	if bound3 != 0 || len(fallbacks) != 0 {
		t.Fatalf("local roles must bind zero, got %v %v", bound3, fallbacks)
	}
}

// Audit 2026-10-21: uncapped tribunal turns rambled past every context
// timeout on the default Strata stack (~36 tok/s, unbounded generation).
// CompleteCapped must send an explicit max_tokens; plain Complete must
// keep omitting it (the deliberate no-clamp default for chat).
func TestCompleteCappedSendsMaxTokens(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = nil // fresh per request: decode merges into maps, never clears
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	capped := func(maxTokens int) map[string]any {
		t.Helper()
		s := toolTestSettings(t, srv.URL)
		c := llm.NewClient(s)
		if _, _, _, err := c.CompleteCapped(context.Background(), "copilot",
			[]llm.ChatMessage{{Role: "user", Content: "hi"}}, maxTokens); err != nil {
			t.Fatalf("complete: %v", err)
		}
		return gotBody
	}

	if v, present := capped(1500)["max_tokens"]; !present || v != float64(1500) {
		t.Errorf("CompleteCapped(1500) must send max_tokens=1500, got body %v", capped(1500))
	}
	// Plain Complete honors the settings default (0 in tests = omit).
	s := toolTestSettings(t, srv.URL)
	c := llm.NewClient(s)
	if _, _, _, err := c.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, present := gotBody["max_tokens"]; present {
		t.Errorf("plain Complete with MaxTokens=0 must omit max_tokens, got body %v", gotBody)
	}
}
