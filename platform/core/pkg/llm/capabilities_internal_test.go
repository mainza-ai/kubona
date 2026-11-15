package llm

// B13 pins: capabilities are declared, not guessed; reasoning effort is
// normalized to a level the model actually supports, and never sent to a
// model/provider that does not.

import (
	"encoding/json"
	"testing"
)

func TestNormalizeReasoningEffort(t *testing.T) {
	cases := []struct {
		name     string
		provider ProviderType
		model    string
		effort   string
		want     string
	}{
		{"reasoning model exact", ProviderOpenAI, "gpt-5.4", "high", "high"},
		{"alias minimal", ProviderOpenAI, "gpt-5.4", "minimal", "low"},
		{"alias xhigh", ProviderOpenAI, "gpt-5.4", "xhigh", "high"},
		{"none is never sent", ProviderOpenAI, "gpt-5.4", "none", ""},
		{"non-reasoning model", ProviderOpenAI, "gpt-6-luna", "high", ""},
		{"unknown model", ProviderOpenAI, "gpt-mystery-2099", "high", ""},
		{"provider without effort control", ProviderOpenCode, "deepseek-v4.1-flash", "high", ""},
		{"anthropic maps", ProviderAnthropic, "claude-opus-5-5", "medium", "medium"},
		{"empty", ProviderOpenAI, "gpt-5.4", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeReasoningEffort(tc.provider, tc.model, tc.effort); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestReasoningEffortsFor(t *testing.T) {
	if got := reasoningEffortsFor(ProviderOpenAI); len(got) != 3 || got[0] != "low" || got[2] != "high" {
		t.Fatalf("openai efforts: %v", got)
	}
	if got := reasoningEffortsFor(ProviderOpenCode); got != nil {
		t.Fatalf("opencode accepts no effort control: %v", got)
	}
}

func TestAnthropicThinkingFollowsDeclaredCapabilities(t *testing.T) {
	build := func(model, effort string) map[string]any {
		raw, err := buildAnthropicPayload(wireRequest{
			Provider: ProviderAnthropic, Model: model, ReasoningEffort: effort, MaxTokens: 1024,
			Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	known := build("claude-opus-5-5", "xhigh")
	if _, ok := known["thinking"]; !ok {
		t.Fatalf("declared reasoning model must carry a thinking config: %v", known)
	}
	if cfg, ok := known["thinking"].(map[string]any); !ok || cfg["budget_tokens"] != float64(16384) {
		t.Fatalf("xhigh must map to the high budget: %v", known["thinking"])
	}
	unknown := build("claude-mystery-2099", "high")
	if _, ok := unknown["thinking"]; ok {
		t.Fatalf("unknown model must not claim reasoning: %v", unknown)
	}
}
