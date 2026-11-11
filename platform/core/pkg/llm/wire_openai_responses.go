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

func buildOpenAIResponsesPayload(wr wireRequest, stream bool) ([]byte, error) {
	systemTexts, nonSystem := extractSystemAndNonSystem(wr.Messages)

	inputItems := make([]any, 0, len(nonSystem))
	for _, m := range nonSystem {
		parts := effectiveParts(m)
		if m.Role == "tool" {
			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
			continue
		}
		if m.Role == "assistant" {
			for _, p := range parts {
				if p.Type == "thinking" && p.EncryptedContent != "" {
					inputItems = append(inputItems, map[string]any{
						"type":              "reasoning",
						"encrypted_content": p.EncryptedContent,
					})
				}
			}
			if m.Content != "" {
				inputItems = append(inputItems, map[string]any{
					"role":    "assistant",
					"content": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Name,
					"arguments": tc.Arguments,
				})
			}
			continue
		}
		var contentParts []map[string]any
		hasImage := false
		for _, p := range parts {
			switch p.Type {
			case "text":
				contentParts = append(contentParts, map[string]any{"type": "input_text", "text": p.Text})
			case "image":
				hasImage = true
				imgURL := p.ImageURL
				if imgURL == "" && p.DataBase64 != "" {
					mime := p.MimeType
					if mime == "" {
						mime = "image/png"
					}
					imgURL = "data:" + mime + ";base64," + p.DataBase64
				}
				if imgURL != "" {
					contentParts = append(contentParts, map[string]any{"type": "input_image", "image_url": imgURL})
				}
			}
		}
		if hasImage {
			inputItems = append(inputItems, map[string]any{"role": "user", "content": contentParts})
		} else {
			inputItems = append(inputItems, map[string]any{"role": "user", "content": m.Content})
		}
	}

	payload := map[string]any{
		"model": wr.Model,
		"input": inputItems,
	}
	if len(systemTexts) > 0 {
		payload["instructions"] = strings.Join(systemTexts, "\n\n")
	}
	if stream {
		payload["stream"] = true
	}
	if wr.MaxTokens > 0 {
		payload["max_output_tokens"] = wr.MaxTokens
	}
	if wr.PromptCacheKey != "" {
		payload["prompt_cache_key"] = wr.PromptCacheKey
	}
	if effort := strings.ToLower(strings.TrimSpace(wr.ReasoningEffort)); effort == "low" || effort == "medium" || effort == "high" {
		payload["reasoning"] = map[string]any{"effort": effort}
	} else {
		if wr.Temperature != nil {
			payload["temperature"] = *wr.Temperature
		}
		if wr.TopP != nil {
			payload["top_p"] = *wr.TopP
		}
	}
	if wr.ResponseFormat != nil {
		payload["text"] = map[string]any{"format": wr.ResponseFormat}
	}
	if len(wr.Tools) > 0 {
		respTools := make([]map[string]any, 0, len(wr.Tools))
		for _, t := range wr.Tools {
			name, desc, schema, ok := normalizeToolDef(t)
			if !ok {
				continue
			}
			respTools = append(respTools, map[string]any{
				"type":        "function",
				"name":        name,
				"description": desc,
				"parameters":  schema,
			})
		}
		if len(respTools) > 0 {
			payload["tools"] = respTools
			payload["tool_choice"] = "auto"
		}
	}
	return json.Marshal(payload)
}

func (c *Client) execOpenAIResponses(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	bodyBytes, err := buildOpenAIResponsesPayload(wr, false)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/responses", strings.TrimRight(wr.ProvConfig.BaseURL, "/"))
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyChatHeaders(req, wr, false)
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

	var parsed openAIResponseEnvelope
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI Responses payload: %w", err)
	}
	return decodeOpenAIResponseEnvelope(&parsed, wr.Model), nil
}

type openAIResponseEnvelope struct {
	ID                string `json:"id"`
	Model             string `json:"model"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type             string `json:"type"`
		CallID           string `json:"call_id"`
		Name             string `json:"name"`
		Arguments        string `json:"arguments"`
		EncryptedContent string `json:"encrypted_content"`
		Content          []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Summary []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"summary"`
	} `json:"output"`
	Usage struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func decodeOpenAIResponseEnvelope(parsed *openAIResponseEnvelope, fallbackModel string) *wireResponse {
	var textBuf, reasonBuf strings.Builder
	var encReason string
	var parts []MessagePart
	var toolCalls []ToolCall
	for _, item := range parsed.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Text != "" {
					textBuf.WriteString(c.Text)
					parts = append(parts, MessagePart{Type: "text", Text: c.Text})
				}
			}
		case "reasoning":
			if item.EncryptedContent != "" {
				encReason = item.EncryptedContent
			}
			for _, s := range item.Summary {
				reasonBuf.WriteString(s.Text)
			}
			parts = append(parts, MessagePart{
				Type:             "thinking",
				Text:             reasonBuf.String(),
				EncryptedContent: item.EncryptedContent,
			})
		case "function_call":
			toolCalls = append(toolCalls, ToolCall{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: item.Arguments,
			})
			parts = append(parts, MessagePart{
				Type:       "tool_call",
				ToolCallID: item.CallID,
				ToolName:   item.Name,
				ToolArgs:   item.Arguments,
			})
		}
	}
	finish := "stop"
	if parsed.IncompleteDetails != nil && parsed.IncompleteDetails.Reason == "max_output_tokens" {
		finish = "length"
	} else if len(toolCalls) > 0 {
		finish = "tool_calls"
	}
	cachedIn, cacheWrite, reasonTok := 0, 0, 0
	if parsed.Usage.InputTokensDetails != nil {
		cachedIn = parsed.Usage.InputTokensDetails.CachedTokens
		cacheWrite = parsed.Usage.InputTokensDetails.CacheWriteTokens
	}
	if parsed.Usage.OutputTokensDetails != nil {
		reasonTok = parsed.Usage.OutputTokensDetails.ReasoningTokens
	}
	model := parsed.Model
	if model == "" {
		model = fallbackModel
	}
	return &wireResponse{
		Content:            textBuf.String(),
		ReasoningContent:   reasonBuf.String(),
		EncryptedReasoning: encReason,
		Parts:              parts,
		ToolCalls:          toolCalls,
		Model:              model,
		FinishReason:       finish,
		PromptTokens:       parsed.Usage.InputTokens,
		CompletionTokens:   parsed.Usage.OutputTokens,
		TotalTokens:        parsed.Usage.TotalTokens,
		CachedInputTokens:  cachedIn,
		CacheWriteTokens:   cacheWrite,
		ReasoningTokens:    reasonTok,
		UsageReported:      parsed.Usage.InputTokens+parsed.Usage.OutputTokens+parsed.Usage.TotalTokens > 0,
	}
}

func (c *Client) streamOpenAIResponses(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	bodyBytes, err := buildOpenAIResponsesPayload(wr, true)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/responses", strings.TrimRight(wr.ProvConfig.BaseURL, "/"))
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyChatHeaders(req, wr, true)
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

	var full strings.Builder
	var completed *wireResponse
	var streamErr error
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ev struct {
			Type     string                  `json:"type"`
			Delta    string                  `json:"delta"`
			Response *openAIResponseEnvelope `json:"response"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		if ev.Error != nil || ev.Type == "error" || ev.Type == "response.failed" {
			msg := payload
			if ev.Error != nil && strings.TrimSpace(ev.Error.Message) != "" {
				msg = strings.TrimSpace(ev.Error.Message)
			}
			streamErr = &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: msg}
			break
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				if full.Len()+len(ev.Delta) > maxLLMResponseBytes {
					return nil, fmt.Errorf("response exceeded %d byte ceiling during streaming", maxLLMResponseBytes)
				}
				full.WriteString(ev.Delta)
				if onToken != nil {
					onToken(ev.Delta)
				}
			}
		case "response.completed":
			if ev.Response != nil {
				completed = decodeOpenAIResponseEnvelope(ev.Response, wr.Model)
			}
		}
	}
	if completed == nil {
		completed = &wireResponse{Content: full.String(), Model: wr.Model, FinishReason: "stop"}
	} else if completed.Content == "" && full.Len() > 0 {
		completed.Content = full.String()
	}
	completed.RawUsage = buildUsageMap(completed)
	if streamErr != nil {
		return completed, streamErr
	}
	if err := sc.Err(); err != nil {
		return completed, err
	}
	return completed, nil
}
