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

func toolTestSettings(t *testing.T, url string) *llm.Settings {
	t.Helper()
	t.Setenv("OPENCODE_API_KEY", "test-key")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = url
	snap.Providers[llm.ProviderOpenCode] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	return s
}

func TestCompleteWithToolsParsesToolCalls(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"content": "",
					"tool_calls": []any{map[string]any{
						"id":   "call_1",
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

	c := llm.NewClient(toolTestSettings(t, srv.URL))
	text, calls, _, _, err := c.CompleteWithTools(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "anchor for H2?"}},
		[]map[string]any{{"type": "function", "function": map[string]any{"name": "get_anchor"}}})
	if err != nil {
		t.Fatalf("CompleteWithTools failed: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty text with tool calls, got %q", text)
	}
	if len(calls) != 1 || calls[0].Name != "get_anchor" || calls[0].ID != "call_1" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Arguments), &args); err != nil || args["molecule"] != "H2" {
		t.Errorf("arguments did not survive: %q", calls[0].Arguments)
	}
	if gotBody["tool_choice"] != "auto" {
		t.Errorf("expected tool_choice auto, got %v", gotBody["tool_choice"])
	}
}

func TestCompleteWithToolsNoToolsBehavesLikeComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "plain answer"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	c := llm.NewClient(toolTestSettings(t, srv.URL))
	text, calls, _, _, err := c.CompleteWithTools(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if text != "plain answer" || len(calls) != 0 {
		t.Errorf("got text=%q calls=%v", text, calls)
	}
}

func TestCompleteWithToolsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		_, _ = w.Write([]byte(`{"error":"insufficient funds"}`))
	}))
	defer srv.Close()

	c := llm.NewClient(toolTestSettings(t, srv.URL))
	_, _, _, _, err := c.CompleteWithTools(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}},
		[]map[string]any{{"type": "function"}})
	if err == nil || !strings.Contains(err.Error(), "402") {
		t.Errorf("expected 402 error, got %v", err)
	}
}

func TestToolMessageSerialization(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "anchored: -1.13727"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	c := llm.NewClient(toolTestSettings(t, srv.URL))
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "anchor?"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "get_anchor", Arguments: `{"molecule":"H2"}`}}},
		{Role: "tool", ToolCallID: "call_1", ToolName: "get_anchor", Content: `{"fci":-1.13727}`},
	}
	text, _, _, _, err := c.CompleteWithTools(context.Background(), "copilot", msgs, nil)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if text != "anchored: -1.13727" {
		t.Errorf("unexpected text: %q", text)
	}
	raw, _ := json.Marshal(gotBody["messages"])
	if !strings.Contains(string(raw), "call_1") {
		t.Errorf("tool_call_id missing from wire messages: %s", raw)
	}
}

func TestParseDSMLInvokes(t *testing.T) {
	text := "<｜DSML｜ calls>\n<｜DSML｜ invoke name=\"query_literature\">\n<｜DSML｜ parameter name=\"query\" string=\"true\">H2 anchor</｜DSML｜ parameter>\n</｜DSML｜ invoke>\n</｜DSML｜ calls>"
	calls := llm.ParseDSMLInvokes(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "query_literature" {
		t.Errorf("unexpected name: %q", calls[0].Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Arguments), &args); err != nil || args["query"] != "H2 anchor" {
		t.Errorf("arguments did not survive: %q", calls[0].Arguments)
	}
	if llm.StripDSML(text) != "" {
		t.Errorf("strip left residue: %q", llm.StripDSML(text))
	}
	if len(llm.ParseDSMLInvokes("plain answer, no markup")) != 0 {
		t.Error("plain text must yield zero calls")
	}
}
