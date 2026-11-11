package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ToolCall is one model-requested function invocation (OpenAI-compatible
// function-calling dialect, spoken by oMLX, Ollama, and cloud endpoints).
type ToolCall struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
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

// CompleteWithTools executes a chat completion advertising function tools,
// applying per-role parameter overrides and ordered failover hops.
// A nil/empty tools slice behaves identically to Complete.
func (c *Client) CompleteWithTools(ctx context.Context, role string, messages []ChatMessage, tools []map[string]any) (string, []ToolCall, *ChatCompletionResponse, time.Duration, error) {
	return c.executeRoleHops(ctx, role, messages, tools, -1)
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
//
// Single grammar (W0.19): invokes are valid ONLY inside calls blocks. Stray
// invokes in prose are ignored (never executed) — the old parser executed
// any invoke anywhere, so prompt-injected markup ran spend-capable tools.
// Strip removes whole calls blocks (with contents); stray tags are cleaned
// after. Parse and strip share these patterns: one dialect, never two.
var (
	dsmlInvokeRe = regexp.MustCompile(`(?s)<｜DSML｜\s+invoke\s+name="([^"]+)"\s*>(.*?)</｜DSML｜\s*invoke\s*>`)
	dsmlParamRe  = regexp.MustCompile(`(?s)<｜DSML｜\s+parameter\s+name="([^"]+)"[^>]*>(.*?)</｜DSML｜\s*parameter\s*>`)
	dsmlCallsRe  = regexp.MustCompile(`(?s)<｜DSML｜\s+calls\s*>.*?</｜DSML｜\s*calls\s*>`)
	dsmlTagRe    = regexp.MustCompile(`</?｜DSML｜[^>]*>`)
)

// ParseDSMLInvokes extracts tool invocations from DSML-marked text — only
// invokes nested inside calls blocks execute. Stray invokes (prose
// injection) are ignored, never parsed.
func ParseDSMLInvokes(text string) []ToolCall {
	var out []ToolCall
	for _, block := range dsmlCallsRe.FindAllString(text, -1) {
		for _, m := range dsmlInvokeRe.FindAllStringSubmatch(block, -1) {
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
