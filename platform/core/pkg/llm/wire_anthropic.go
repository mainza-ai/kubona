package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicAPIVersion = "2023-06-01"

func anthropicEndpoint(baseURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + path
	}
	return base + "/v1" + path
}

func supportsAdaptiveThinking(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "claude-opus-4-6") ||
		strings.Contains(lower, "claude-sonnet-4-6") ||
		strings.Contains(lower, "claude-4-6") ||
		strings.Contains(lower, "4.6")
}

func anthropicThinkingBudget(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	default:
		return 0
	}
}

func buildAnthropicPayload(wr wireRequest, stream bool) ([]byte, error) {
	systemTexts, nonSystem := extractSystemAndNonSystem(wr.Messages)

	maxTokens := wr.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	var thinkingCfg map[string]any
	if effort := strings.ToLower(strings.TrimSpace(wr.ReasoningEffort)); effort == "low" || effort == "medium" || effort == "high" {
		if supportsAdaptiveThinking(wr.Model) {
			thinkingCfg = map[string]any{"type": "adaptive"}
		} else {
			budget := anthropicThinkingBudget(effort)
			if maxTokens <= budget {
				maxTokens = budget + 1024
			}
			thinkingCfg = map[string]any{"type": "enabled", "budget_tokens": budget}
		}
	}

	breakpointsUsed := 0
	const maxCacheBreakpoints = 4

	var systemBlocks []map[string]any
	if len(systemTexts) > 0 {
		joined := strings.Join(systemTexts, "\n\n")
		blk := map[string]any{
			"type": "text",
			"text": joined,
		}
		if len(joined) >= 256 && breakpointsUsed < maxCacheBreakpoints {
			blk["cache_control"] = map[string]any{"type": "ephemeral"}
			breakpointsUsed++
		}
		systemBlocks = append(systemBlocks, blk)
	}

	var anthropicTools []map[string]any
	for idx, t := range wr.Tools {
		name, desc, schema, ok := normalizeToolDef(t)
		if !ok {
			continue
		}
		td := map[string]any{
			"name":         name,
			"description":  desc,
			"input_schema": schema,
		}
		if idx == len(wr.Tools)-1 && breakpointsUsed < maxCacheBreakpoints {
			td["cache_control"] = map[string]any{"type": "ephemeral"}
			breakpointsUsed++
		}
		anthropicTools = append(anthropicTools, td)
	}

	msgObjs := make([]map[string]any, 0, len(nonSystem))
	for _, m := range nonSystem {
		role := m.Role
		if role == "tool" {
			role = "user"
		}
		parts := effectiveParts(m)
		var blocks []map[string]any
		for _, p := range parts {
			switch p.Type {
			case "thinking":
				if p.Signature != "" {
					blocks = append(blocks, map[string]any{
						"type":      "thinking",
						"thinking":  p.Text,
						"signature": p.Signature,
					})
				}
			case "text":
				if p.Text != "" {
					blocks = append(blocks, map[string]any{
						"type": "text",
						"text": p.Text,
					})
				}
			case "image":
				if p.DataBase64 != "" {
					mime := p.MimeType
					if mime == "" {
						mime = "image/png"
					}
					blocks = append(blocks, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": mime,
							"data":       p.DataBase64,
						},
					})
				} else if p.ImageURL != "" {
					blocks = append(blocks, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type": "url",
							"url":  p.ImageURL,
						},
					})
				}
			case "tool_call":
				var inputObj any = map[string]any{}
				if p.ToolArgs != "" {
					var tmp any
					if err := json.Unmarshal([]byte(p.ToolArgs), &tmp); err == nil {
						inputObj = tmp
					}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    p.ToolCallID,
					"name":  p.ToolName,
					"input": inputObj,
				})
			case "tool_result":
				blk := map[string]any{
					"type":        "tool_result",
					"tool_use_id": p.ToolCallID,
					"content":     p.Text,
				}
				if p.IsError {
					blk["is_error"] = true
				}
				blocks = append(blocks, blk)
			}
		}
		if len(blocks) == 0 {
			continue
		}
		// Coalesce consecutive turns with same role (required by Anthropic Messages API).
		if n := len(msgObjs); n > 0 && msgObjs[n-1]["role"] == role {
			existing := msgObjs[n-1]["content"].([]map[string]any)
			msgObjs[n-1]["content"] = append(existing, blocks...)
			continue
		}
		msgObjs = append(msgObjs, map[string]any{
			"role":    role,
			"content": blocks,
		})
	}
	// Mark the second-to-last non-system message as a sliding prompt-cache prefix when multi-turn.
	if len(msgObjs) >= 2 && breakpointsUsed < maxCacheBreakpoints {
		prevBlocks := msgObjs[len(msgObjs)-2]["content"].([]map[string]any)
		if len(prevBlocks) > 0 {
			lastBlk := prevBlocks[len(prevBlocks)-1]
			if lastBlk["type"] == "text" || lastBlk["type"] == "tool_result" {
				lastBlk["cache_control"] = map[string]any{"type": "ephemeral"}
			}
		}
	}

	payload := map[string]any{
		"model":      wr.Model,
		"max_tokens": maxTokens,
		"messages":   msgObjs,
	}
	if stream {
		payload["stream"] = true
	}
	if len(systemBlocks) > 0 {
		payload["system"] = systemBlocks
	}
	if len(anthropicTools) > 0 {
		payload["tools"] = anthropicTools
		payload["tool_choice"] = map[string]any{"type": "auto"}
	}
	if thinkingCfg != nil {
		payload["thinking"] = thinkingCfg
		// Anthropic rejects non-default temperature/top_p when extended thinking is enabled.
	} else {
		if wr.Temperature != nil {
			payload["temperature"] = *wr.Temperature
		}
		if wr.TopP != nil {
			payload["top_p"] = *wr.TopP
		}
	}
	return json.Marshal(payload)
}

func applyAnthropicHeaders(req *http.Request, wr wireRequest, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if wr.ProvConfig.APIKey != "" {
		req.Header.Set("x-api-key", wr.ProvConfig.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
}

func (c *Client) execAnthropicMessages(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	bodyBytes, err := buildAnthropicPayload(wr, false)
	if err != nil {
		return nil, err
	}
	endpoint := anthropicEndpoint(wr.ProvConfig.BaseURL, "/messages")
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyAnthropicHeaders(req, wr, false)
		return req, nil
	}, 2)
	if err != nil {
		return nil, &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	respBytes, err := readBoundedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fault, retryable, authInvalid := classifyHTTPFault(resp.StatusCode, respBytes)
		return nil, &WireError{Fault: fault, Status: resp.StatusCode, Endpoint: endpoint, Message: string(respBytes), Retryable: retryable, AuthInvalid: authInvalid}
	}

	var parsed struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  string          `json:"thinking"`
			Signature string          `json:"signature"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			OutputTokensDetails      *struct {
				ThinkingTokens int `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	var textBuf, thinkBuf strings.Builder
	var sig string
	var parts []MessagePart
	var toolCalls []ToolCall
	for _, b := range parsed.Content {
		switch b.Type {
		case "thinking":
			if b.Thinking != "" {
				thinkBuf.WriteString(b.Thinking)
			}
			if b.Signature != "" {
				sig = b.Signature
			}
			parts = append(parts, MessagePart{Type: "thinking", Text: b.Thinking, Signature: b.Signature})
		case "text":
			textBuf.WriteString(b.Text)
			parts = append(parts, MessagePart{Type: "text", Text: b.Text})
		case "tool_use":
			rawArgs := strings.TrimSpace(string(b.Input))
			if rawArgs == "" {
				rawArgs = "{}"
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        b.ID,
				Name:      b.Name,
				Arguments: rawArgs,
			})
			parts = append(parts, MessagePart{
				Type:       "tool_call",
				ToolCallID: b.ID,
				ToolName:   b.Name,
				ToolArgs:   rawArgs,
			})
		}
	}

	finish := parsed.StopReason
	if finish == "max_tokens" {
		finish = "length"
	} else if finish == "end_turn" {
		finish = "stop"
	}
	totalInput := parsed.Usage.InputTokens + parsed.Usage.CacheReadInputTokens + parsed.Usage.CacheCreationInputTokens
	thinkingToks := 0
	if parsed.Usage.OutputTokensDetails != nil {
		thinkingToks = parsed.Usage.OutputTokensDetails.ThinkingTokens
	}

	return &wireResponse{
		Content:            textBuf.String(),
		ReasoningContent:   thinkBuf.String(),
		ReasoningSignature: sig,
		Parts:              parts,
		ToolCalls:          toolCalls,
		Model:              parsed.Model,
		FinishReason:       finish,
		PromptTokens:       totalInput,
		CompletionTokens:   parsed.Usage.OutputTokens,
		TotalTokens:        totalInput + parsed.Usage.OutputTokens,
		CachedInputTokens:  parsed.Usage.CacheReadInputTokens,
		CacheWriteTokens:   parsed.Usage.CacheCreationInputTokens,
		ReasoningTokens:    thinkingToks,
		UsageReported:      (totalInput + parsed.Usage.OutputTokens) > 0,
	}, nil
}

func (c *Client) streamAnthropicMessages(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	bodyBytes, err := buildAnthropicPayload(wr, true)
	if err != nil {
		return nil, err
	}
	endpoint := anthropicEndpoint(wr.ProvConfig.BaseURL, "/messages")
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyAnthropicHeaders(req, wr, true)
		return req, nil
	}, 2)
	if err != nil {
		return nil, &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		fault, retryable, authInvalid := classifyHTTPFault(resp.StatusCode, slurp)
		return nil, &WireError{Fault: fault, Status: resp.StatusCode, Endpoint: endpoint, Message: string(slurp), Retryable: retryable, AuthInvalid: authInvalid}
	}

	var textBuf, thinkBuf strings.Builder
	var sig, finish string
	var inputToks, cacheReadToks, cacheCreateToks, outputToks int
	var streamErr error
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Message *struct {
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta *struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				Thinking   string `json:"thinking"`
				Signature  string `json:"signature"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage *struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		if ev.Type == "error" || ev.Error != nil {
			msg := payload
			if ev.Error != nil && strings.TrimSpace(ev.Error.Message) != "" {
				msg = strings.TrimSpace(ev.Error.Message)
			}
			streamErr = &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: msg}
			break
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				inputToks = ev.Message.Usage.InputTokens
				cacheReadToks = ev.Message.Usage.CacheReadInputTokens
				cacheCreateToks = ev.Message.Usage.CacheCreationInputTokens
			}
		case "content_block_delta":
			if ev.Delta != nil {
				switch ev.Delta.Type {
				case "text_delta":
					if ev.Delta.Text != "" {
						if textBuf.Len()+len(ev.Delta.Text) > maxLLMResponseBytes {
							return nil, fmt.Errorf("response exceeded %d byte ceiling during streaming", maxLLMResponseBytes)
						}
						textBuf.WriteString(ev.Delta.Text)
						if onToken != nil {
							onToken(ev.Delta.Text)
						}
					}
				case "thinking_delta":
					thinkBuf.WriteString(ev.Delta.Thinking)
				case "signature_delta":
					sig += ev.Delta.Signature
				}
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				if ev.Delta.StopReason == "max_tokens" {
					finish = "length"
				} else if ev.Delta.StopReason == "end_turn" {
					finish = "stop"
				} else {
					finish = ev.Delta.StopReason
				}
			}
			if ev.Usage != nil && ev.Usage.OutputTokens > 0 {
				outputToks = ev.Usage.OutputTokens
			}
		}
	}
	totalInput := inputToks + cacheReadToks + cacheCreateToks
	out := &wireResponse{
		Content:            textBuf.String(),
		ReasoningContent:   thinkBuf.String(),
		ReasoningSignature: sig,
		Model:              wr.Model,
		FinishReason:       finish,
		PromptTokens:       totalInput,
		CompletionTokens:   outputToks,
		TotalTokens:        totalInput + outputToks,
		CachedInputTokens:  cacheReadToks,
		CacheWriteTokens:   cacheCreateToks,
		UsageReported:      (totalInput + outputToks) > 0,
	}
	out.RawUsage = buildUsageMap(out)
	if streamErr != nil {
		return out, streamErr
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}
