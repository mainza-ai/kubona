package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ToolCall is one model-requested function invocation (OpenAI-compatible
// function-calling dialect, spoken by oMLX, Ollama, and cloud endpoints).
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolCallWire mirrors the wire shape {id, type, function:{name,arguments}}.
type toolCallWire struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func toWireCalls(calls []ToolCall) []toolCallWire {
	out := make([]toolCallWire, 0, len(calls))
	for _, c := range calls {
		var w toolCallWire
		w.ID = c.ID
		w.Type = "function"
		w.Function.Name = c.Name
		w.Function.Arguments = c.Arguments
		out = append(out, w)
	}
	return out
}

func fromWireCalls(in []toolCallWire) []ToolCall {
	out := make([]ToolCall, 0, len(in))
	for _, w := range in {
		out = append(out, ToolCall{ID: w.ID, Name: w.Function.Name, Arguments: w.Function.Arguments})
	}
	return out
}

// CompleteWithTools executes a chat completion advertising function tools.
// Returns the assistant text and any requested tool calls (finish_reason
// "tool_calls"). A nil/empty tools slice behaves exactly like Complete.
func (c *Client) CompleteWithTools(ctx context.Context, role string, messages []ChatMessage, tools []map[string]any) (string, []ToolCall, *ChatCompletionResponse, time.Duration, error) {
	providerType, model, err := c.settings.GetModelForRole(role)
	if err != nil {
		return "", nil, nil, 0, err
	}

	snap := c.settings.GetSnapshot()
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return "", nil, nil, 0, fmt.Errorf("provider configuration not found for %s", providerType)
	}

	body := map[string]any{
		"model":       model,
		"messages":    withToolMessages(messages),
		"temperature": snap.Temperature,
	}
	if snap.MaxTokens > 0 {
		body["max_tokens"] = snap.MaxTokens
	}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", nil, nil, 0, err
	}

	endpoint := fmt.Sprintf("%s/chat/completions", provConfig.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if provConfig.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provConfig.APIKey))
	}

	start := time.Now()
	resp, err := c.httpCli.Do(req)
	duration := time.Since(start)
	if err != nil {
		return "", nil, nil, duration, fmt.Errorf("HTTP request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, nil, duration, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp := string(respBytes)
		if len(slurp) > 1024 {
			slurp = slurp[:1024]
		}
		return "", nil, nil, duration, fmt.Errorf("API error (%d) from %s: %s", resp.StatusCode, endpoint, slurp)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string         `json:"content"`
				ToolCalls []toolCallWire `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", nil, nil, duration, fmt.Errorf("failed to parse JSON response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", nil, nil, duration, fmt.Errorf("no completion choices returned by model")
	}
	ch := parsed.Choices[0]
	return ch.Message.Content, fromWireCalls(ch.Message.ToolCalls), nil, duration, nil
}

// withToolMessages serializes messages including tool_calls / tool results.
// ChatMessage carries them as structured fields; the wire needs the
// OpenAI shape, so translate here instead of leaking wire types into callers.
func withToolMessages(messages []ChatMessage) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		if len(m.ToolCalls) > 0 {
			out = append(out, map[string]any{
				"role":       m.Role,
				"content":    m.Content,
				"tool_calls": toWireCalls(m.ToolCalls),
			})
			continue
		}
		if m.Role == "tool" {
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"name":         m.ToolName,
				"content":      m.Content,
			})
			continue
		}
		out = append(out, m)
	}
	return out
}

// DSML fallback: DeepSeek open-weight models served through some
// gateways emit tool invocations as <|DSML|> markup in text instead of the
// structured tool_calls field. Parse them into equivalent calls so the same
// validated dispatch path serves both dialects — and strip them from display
// so markup never reaches chat. Structured tool_calls always win when both
// are present.
var (
	dsmlInvokeRe = regexp.MustCompile(`(?s)<｜DSML｜\s+invoke\s+name="([^"]+)"\s*>(.*?)</｜DSML｜\s*invoke\s*>`)
	dsmlParamRe  = regexp.MustCompile(`(?s)<｜DSML｜\s+parameter\s+name="([^"]+)"[^>]*>(.*?)</｜DSML｜\s*parameter\s*>`)
	dsmlCallsRe  = regexp.MustCompile(`(?s)<｜DSML｜\s+calls\s*>.*?</｜DSML｜\s*calls\s*>`)
	dsmlTagRe    = regexp.MustCompile(`</?｜DSML｜[^>]*>`)
)

// ParseDSMLInvokes extracts tool invocations from DSML-marked text.
func ParseDSMLInvokes(text string) []ToolCall {
	var out []ToolCall
	for _, m := range dsmlInvokeRe.FindAllStringSubmatch(text, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		args := map[string]any{}
		for _, p := range dsmlParamRe.FindAllStringSubmatch(m[2], -1) {
			key := strings.TrimSpace(p[1])
			raw := strings.TrimSpace(p[2])
			if key == "" {
				continue
			}
			var v any
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				v = raw
			}
			// Unquote JSON strings that arrived double-encoded.
			if s, ok := v.(string); ok {
				var inner any
				if err := json.Unmarshal([]byte(s), &inner); err == nil {
					if _, isStr := inner.(string); !isStr {
						v = inner
					}
				}
			}
			args[key] = v
		}
		rawArgs, _ := json.Marshal(args)
		out = append(out, ToolCall{
			ID:        fmt.Sprintf("dsml-%d", len(out)+1),
			Name:      name,
			Arguments: string(rawArgs),
		})
	}
	return out
}

// StripDSML removes DSML machine dialogue (whole calls blocks, then any
// stray tags) from display text.
func StripDSML(text string) string {
	s := dsmlCallsRe.ReplaceAllString(text, "")
	s = dsmlTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
type StreamedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// parseStreamToolDeltas folds one SSE data payload's tool_calls deltas into
// the accumulator map keyed by index. Returns false when the payload holds
// no tool deltas.
func parseStreamToolDeltas(payload string, acc map[int]*StreamedToolCall) bool {
	var chunk struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return false
	}
	found := false
	for _, ch := range chunk.Choices {
		for _, tc := range ch.Delta.ToolCalls {
			found = true
			cur, ok := acc[tc.Index]
			if !ok {
				cur = &StreamedToolCall{}
				acc[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != "" {
					cur.Name = tc.Function.Name
				}
				cur.Arguments.WriteString(tc.Function.Arguments)
			}
		}
	}
	return found
}

// finishStreamedCalls converts the accumulator (ordered by index) into calls.
func finishStreamedCalls(acc map[int]*StreamedToolCall) []ToolCall {
	idx := make([]int, 0, len(acc))
	for i := range acc {
		idx = append(idx, i)
	}
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && idx[j-1] > idx[j]; j-- {
			idx[j-1], idx[j] = idx[j], idx[j-1]
		}
	}
	out := make([]ToolCall, 0, len(acc))
	for _, i := range idx {
		c := acc[i]
		out = append(out, ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments.String()})
	}
	return out
}
