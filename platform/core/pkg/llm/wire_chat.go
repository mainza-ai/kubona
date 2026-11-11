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
	"time"
)

func applyChatHeaders(req *http.Request, wr wireRequest, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if wr.ProvConfig.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", wr.ProvConfig.APIKey))
	}
	if wr.Provider == ProviderOpenRouter {
		req.Header.Set("HTTP-Referer", "https://github.com/mainza-ai/kubona")
		req.Header.Set("X-Title", "Kubona Observatory")
	}
}

func toOpenAIMessages(messages []ChatMessage) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		if m.Role == "tool" {
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"name":         m.ToolName,
				"content":      m.Content,
			})
			continue
		}
		if len(m.ToolCalls) > 0 {
			out = append(out, map[string]any{
				"role":       m.Role,
				"content":    m.Content,
				"tool_calls": toWireCalls(m.ToolCalls),
			})
			continue
		}
		parts := effectiveParts(m)
		hasImage := false
		for _, p := range parts {
			if p.Type == "image" {
				hasImage = true
				break
			}
		}
		if hasImage {
			contentArr := make([]map[string]any, 0, len(parts))
			for _, p := range parts {
				switch p.Type {
				case "text":
					contentArr = append(contentArr, map[string]any{"type": "text", "text": p.Text})
				case "image":
					imgURL := p.ImageURL
					if imgURL == "" && p.DataBase64 != "" {
						mime := p.MimeType
						if mime == "" {
							mime = "image/png"
						}
						imgURL = "data:" + mime + ";base64," + p.DataBase64
					}
					if imgURL != "" {
						contentArr = append(contentArr, map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": imgURL},
						})
					}
				}
			}
			out = append(out, map[string]any{
				"role":    m.Role,
				"content": contentArr,
			})
			continue
		}
		out = append(out, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return out
}

func (c *Client) execChatCompletions(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	body := map[string]any{
		"model":    wr.Model,
		"messages": toOpenAIMessages(wr.Messages),
	}
	if wr.Temperature != nil {
		body["temperature"] = *wr.Temperature
	}
	if wr.TopP != nil {
		body["top_p"] = *wr.TopP
	}
	if wr.Seed != nil && wr.Provider != ProviderNVIDIA {
		body["seed"] = *wr.Seed
	}
	if wr.MaxTokens > 0 {
		body["max_tokens"] = wr.MaxTokens
	}
	if wr.ReasoningEffort != "" && (wr.Provider == ProviderOpenAI || wr.Provider == ProviderOpenRouter || wr.Provider == ProviderXAI || wr.Provider == ProviderGroq) {
		body["reasoning_effort"] = wr.ReasoningEffort
	}
	if wr.ResponseFormat != nil {
		body["response_format"] = wr.ResponseFormat
	}
	if len(wr.Tools) > 0 {
		body["tools"] = wr.Tools
		body["tool_choice"] = "auto"
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/chat/completions", strings.TrimRight(wr.ProvConfig.BaseURL, "/"))
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
		slurp := string(respBytes)
		if len(slurp) > 1024 {
			slurp = slurp[:1024]
		}
		return nil, &WireError{
			Fault:       fault,
			Status:      resp.StatusCode,
			Endpoint:    endpoint,
			Message:     slurp,
			Retryable:   retryable,
			AuthInvalid: authInvalid,
		}
	}

	var parsed struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role             string         `json:"role"`
				Content          string         `json:"content"`
				ReasoningContent string         `json:"reasoning_content"`
				Reasoning        string         `json:"reasoning"`
				ToolCalls        []toolCallWire `json:"tool_calls"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int     `json:"prompt_tokens"`
			CompletionTokens    int     `json:"completion_tokens"`
			TotalTokens         int     `json:"total_tokens"`
			PromptTokPerSec     float64 `json:"prompt_tokens_per_second"`
			GenTokPerSec        float64 `json:"generation_tokens_per_second"`
			TotalTimeSec        float64 `json:"total_time"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned by model")
	}

	ch := parsed.Choices[0]
	reasoning := ch.Message.ReasoningContent
	if reasoning == "" {
		reasoning = ch.Message.Reasoning
	}
	usageOK := (parsed.Usage.PromptTokens >= 0 && parsed.Usage.CompletionTokens >= 0 && (parsed.Usage.PromptTokens+parsed.Usage.CompletionTokens) > 0) || parsed.Usage.TotalTokens > 0
	pt, ct := parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens
	if pt == 0 && ct == 0 && parsed.Usage.TotalTokens > 0 {
		pt = parsed.Usage.TotalTokens
	}
	cachedIn := 0
	if parsed.Usage.PromptTokensDetails != nil {
		cachedIn = parsed.Usage.PromptTokensDetails.CachedTokens
	}
	reasonTok := 0
	if parsed.Usage.CompletionTokensDetails != nil {
		reasonTok = parsed.Usage.CompletionTokensDetails.ReasoningTokens
	}
	return &wireResponse{
		Content:           ch.Message.Content,
		ReasoningContent:  reasoning,
		ToolCalls:         fromWireCalls(ch.Message.ToolCalls),
		Model:             parsed.Model,
		FinishReason:      ch.Finish,
		PromptTokens:      pt,
		CompletionTokens:  ct,
		TotalTokens:       parsed.Usage.TotalTokens,
		CachedInputTokens: cachedIn,
		ReasoningTokens:   reasonTok,
		PromptTokPerSec:   parsed.Usage.PromptTokPerSec,
		GenTokPerSec:      parsed.Usage.GenTokPerSec,
		TotalTimeSec:      parsed.Usage.TotalTimeSec,
		UsageReported:     usageOK,
	}, nil
}

func (c *Client) streamChatCompletions(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	body := map[string]any{
		"model":          wr.Model,
		"messages":       toOpenAIMessages(wr.Messages),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if wr.Temperature != nil {
		body["temperature"] = *wr.Temperature
	}
	if wr.TopP != nil {
		body["top_p"] = *wr.TopP
	}
	if wr.Seed != nil && wr.Provider != ProviderNVIDIA {
		body["seed"] = *wr.Seed
	}
	if wr.MaxTokens > 0 {
		body["max_tokens"] = wr.MaxTokens
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/chat/completions", strings.TrimRight(wr.ProvConfig.BaseURL, "/"))
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
		return nil, &WireError{
			Fault:       fault,
			Status:      resp.StatusCode,
			Endpoint:    endpoint,
			Message:     string(slurp),
			Retryable:   retryable,
			AuthInvalid: authInvalid,
		}
	}

	var full strings.Builder
	finish := ""
	var rawUsage map[string]any
	var streamErr error
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage map[string]any `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			rawUsage = chunk.Usage
		}
		if chunk.Error != nil {
			msg := strings.TrimSpace(chunk.Error.Message)
			if msg == "" {
				msg = payload
			}
			streamErr = &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: msg}
			break
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta.Content != "" {
			if full.Len()+len(ch.Delta.Content) > maxLLMResponseBytes {
				return nil, fmt.Errorf("response exceeded %d byte ceiling during streaming", maxLLMResponseBytes)
			}
			full.WriteString(ch.Delta.Content)
			if onToken != nil {
				onToken(ch.Delta.Content)
			}
		}
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
	}
	pt, ct, tt := extractRawUsageInts(rawUsage)
	out := &wireResponse{
		Content:          full.String(),
		Model:            wr.Model,
		FinishReason:     finish,
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      tt,
		UsageReported:    rawUsage != nil && (pt+ct > 0 || tt > 0),
		RawUsage:         rawUsage,
	}
	if streamErr != nil {
		return out, streamErr
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func extractRawUsageInts(usage map[string]any) (pt, ct, tt int) {
	if usage == nil {
		return 0, 0, 0
	}
	num := func(k string) int {
		switch v := usage[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		default:
			return 0
		}
	}
	pt, ct, tt = num("prompt_tokens"), num("completion_tokens"), num("total_tokens")
	if pt == 0 && ct == 0 && tt > 0 {
		pt = tt
	}
	return pt, ct, tt
}

func ollamaRootURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u = strings.TrimSuffix(u, "/v1")
	return strings.TrimRight(u, "/")
}

func toOllamaMessages(messages []ChatMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		if m.Role == "tool" {
			msg := map[string]any{
				"role":    "tool",
				"content": m.Content,
			}
			if m.ToolName != "" {
				msg["tool_name"] = m.ToolName
			}
			out = append(out, msg)
			continue
		}
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				var argsObj map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &argsObj); err != nil {
					argsObj = map[string]any{"raw": tc.Arguments}
				}
				calls = append(calls, map[string]any{
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": argsObj,
					},
				})
			}
			msg["tool_calls"] = calls
		}
		var imgs []string
		for _, p := range effectiveParts(m) {
			if p.Type == "image" && p.DataBase64 != "" {
				imgs = append(imgs, p.DataBase64)
			}
		}
		if len(imgs) > 0 {
			msg["images"] = imgs
		}
		out = append(out, msg)
	}
	return out
}

func (c *Client) buildOllamaChatPayload(wr wireRequest, stream bool) ([]byte, error) {
	options := map[string]any{}
	if wr.Temperature != nil {
		options["temperature"] = *wr.Temperature
	}
	if wr.TopP != nil {
		options["top_p"] = *wr.TopP
	}
	if wr.Seed != nil {
		options["seed"] = *wr.Seed
	}
	if wr.MaxTokens > 0 {
		options["num_predict"] = wr.MaxTokens
	}
	if wr.NumCtx > 0 {
		options["num_ctx"] = wr.NumCtx
	}
	body := map[string]any{
		"model":    wr.Model,
		"messages": toOllamaMessages(wr.Messages),
		"stream":   stream,
	}
	if len(options) > 0 {
		body["options"] = options
	}
	if len(wr.Tools) > 0 {
		body["tools"] = wr.Tools
	}
	return json.Marshal(body)
}

func decodeOllamaToolCalls(rawCalls []struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}) []ToolCall {
	if len(rawCalls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(rawCalls))
	for idx, tc := range rawCalls {
		args := strings.TrimSpace(string(tc.Function.Arguments))
		if args == "" {
			args = "{}"
		}
		out = append(out, ToolCall{
			ID:        fmt.Sprintf("ollama-call-%d", idx+1),
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return out
}

func (c *Client) execOllamaChat(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	bodyBytes, err := c.buildOllamaChatPayload(wr, false)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/chat", ollamaRootURL(wr.ProvConfig.BaseURL))
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyChatHeaders(req, wr, false)
		return req, nil
	}, 1)
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
		Model      string `json:"model"`
		DoneReason string `json:"done_reason"`
		Message    struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Thinking  string `json:"thinking"`
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		PromptEvalCount    int   `json:"prompt_eval_count"`
		EvalCount          int   `json:"eval_count"`
		PromptEvalDuration int64 `json:"prompt_eval_duration"`
		EvalDuration       int64 `json:"eval_duration"`
		TotalDuration      int64 `json:"total_duration"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}
	var genRate, promptRate float64
	if parsed.EvalDuration > 0 && parsed.EvalCount > 0 {
		genRate = float64(parsed.EvalCount) / (float64(parsed.EvalDuration) / float64(time.Second))
	}
	if parsed.PromptEvalDuration > 0 && parsed.PromptEvalCount > 0 {
		promptRate = float64(parsed.PromptEvalCount) / (float64(parsed.PromptEvalDuration) / float64(time.Second))
	}
	return &wireResponse{
		Content:          parsed.Message.Content,
		ReasoningContent: parsed.Message.Thinking,
		ToolCalls:        decodeOllamaToolCalls(parsed.Message.ToolCalls),
		Model:            parsed.Model,
		FinishReason:     parsed.DoneReason,
		PromptTokens:     parsed.PromptEvalCount,
		CompletionTokens: parsed.EvalCount,
		TotalTokens:      parsed.PromptEvalCount + parsed.EvalCount,
		PromptTokPerSec:  promptRate,
		GenTokPerSec:     genRate,
		TotalTimeSec:     float64(parsed.TotalDuration) / float64(time.Second),
		UsageReported:    parsed.PromptEvalCount+parsed.EvalCount > 0,
	}, nil
}

func (c *Client) streamOllamaChat(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	bodyBytes, err := c.buildOllamaChatPayload(wr, true)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/chat", ollamaRootURL(wr.ProvConfig.BaseURL))
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyChatHeaders(req, wr, false)
		return req, nil
	}, 1)
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
	finish := ""
	var pt, ct int
	var genRate, promptRate float64
	var streamErr error
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var chunk struct {
			Model      string `json:"model"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Error      string `json:"error"`
			Message    struct {
				Content string `json:"content"`
			} `json:"message"`
			PromptEvalCount    int   `json:"prompt_eval_count"`
			EvalCount          int   `json:"eval_count"`
			PromptEvalDuration int64 `json:"prompt_eval_duration"`
			EvalDuration       int64 `json:"eval_duration"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if strings.TrimSpace(chunk.Error) != "" {
			streamErr = &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: strings.TrimSpace(chunk.Error)}
			break
		}
		if chunk.Message.Content != "" {
			if full.Len()+len(chunk.Message.Content) > maxLLMResponseBytes {
				return nil, fmt.Errorf("response exceeded %d byte ceiling during streaming", maxLLMResponseBytes)
			}
			full.WriteString(chunk.Message.Content)
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}
		if chunk.Done {
			finish = chunk.DoneReason
			pt = chunk.PromptEvalCount
			ct = chunk.EvalCount
			if chunk.EvalDuration > 0 && ct > 0 {
				genRate = float64(ct) / (float64(chunk.EvalDuration) / float64(time.Second))
			}
			if chunk.PromptEvalDuration > 0 && pt > 0 {
				promptRate = float64(pt) / (float64(chunk.PromptEvalDuration) / float64(time.Second))
			}
		}
	}
	rawUsage := map[string]any{}
	if pt+ct > 0 {
		rawUsage["prompt_tokens"] = pt
		rawUsage["completion_tokens"] = ct
		rawUsage["total_tokens"] = pt + ct
	}
	var usageArg map[string]any
	if len(rawUsage) > 0 {
		usageArg = rawUsage
	}
	out := &wireResponse{
		Content:          full.String(),
		Model:            wr.Model,
		FinishReason:     finish,
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      pt + ct,
		PromptTokPerSec:  promptRate,
		GenTokPerSec:     genRate,
		UsageReported:    pt+ct > 0,
		RawUsage:         usageArg,
	}
	if streamErr != nil {
		return out, streamErr
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}
