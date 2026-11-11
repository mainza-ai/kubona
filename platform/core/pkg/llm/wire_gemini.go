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

func geminiEndpoint(baseURL, model, action string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	cleanModel := strings.TrimPrefix(strings.TrimSpace(model), "models/")
	return fmt.Sprintf("%s/models/%s:%s", base, cleanModel, action)
}

func applyGeminiHeaders(req *http.Request, wr wireRequest, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if wr.ProvConfig.APIKey != "" {
		req.Header.Set("x-goog-api-key", wr.ProvConfig.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
}

func buildGeminiPayload(wr wireRequest) ([]byte, error) {
	systemTexts, nonSystem := extractSystemAndNonSystem(wr.Messages)

	contents := make([]map[string]any, 0, len(nonSystem))
	for _, m := range nonSystem {
		role := "user"
		if m.Role == "assistant" || m.Role == "model" {
			role = "model"
		}
		var parts []map[string]any
		for _, p := range effectiveParts(m) {
			switch p.Type {
			case "thinking":
				if p.ThoughtSignature != "" {
					part := map[string]any{
						"thought":          true,
						"text":             p.Text,
						"thoughtSignature": p.ThoughtSignature,
					}
					parts = append(parts, part)
				}
			case "text":
				if p.Text != "" {
					parts = append(parts, map[string]any{"text": p.Text})
				}
			case "image":
				if p.DataBase64 != "" {
					mime := p.MimeType
					if mime == "" {
						mime = "image/png"
					}
					parts = append(parts, map[string]any{
						"inlineData": map[string]any{
							"mimeType": mime,
							"data":     p.DataBase64,
						},
					})
				}
			case "tool_call":
				var argsObj map[string]any
				if err := json.Unmarshal([]byte(p.ToolArgs), &argsObj); err != nil {
					argsObj = map[string]any{}
				}
				part := map[string]any{
					"functionCall": map[string]any{
						"id":   p.ToolCallID,
						"name": p.ToolName,
						"args": argsObj,
					},
				}
				if p.ThoughtSignature != "" {
					part["thoughtSignature"] = p.ThoughtSignature
				}
				parts = append(parts, part)
			case "tool_result":
				var respObj map[string]any
				if err := json.Unmarshal([]byte(p.Text), &respObj); err != nil {
					respObj = map[string]any{"result": p.Text}
				}
				parts = append(parts, map[string]any{
					"functionResponse": map[string]any{
						"id":       p.ToolCallID,
						"name":     p.ToolName,
						"response": respObj,
					},
				})
			}
		}
		if len(parts) == 0 {
			continue
		}
		if n := len(contents); n > 0 && contents[n-1]["role"] == role {
			existing := contents[n-1]["parts"].([]map[string]any)
			contents[n-1]["parts"] = append(existing, parts...)
			continue
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}

	genCfg := map[string]any{}
	if wr.Temperature != nil {
		genCfg["temperature"] = *wr.Temperature
	}
	if wr.TopP != nil {
		genCfg["topP"] = *wr.TopP
	}
	if wr.Seed != nil {
		genCfg["seed"] = *wr.Seed
	}
	if wr.MaxTokens > 0 {
		genCfg["maxOutputTokens"] = wr.MaxTokens
	}
	if effort := strings.ToLower(strings.TrimSpace(wr.ReasoningEffort)); effort != "" {
		budget := anthropicThinkingBudget(effort)
		if budget > 0 {
			genCfg["thinkingConfig"] = map[string]any{
				"includeThoughts": true,
				"thinkingBudget":  budget,
			}
		}
	}

	payload := map[string]any{
		"contents": contents,
	}
	if len(systemTexts) > 0 {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": strings.Join(systemTexts, "\n\n")}},
		}
	}
	if len(genCfg) > 0 {
		payload["generationConfig"] = genCfg
	}
	if len(wr.Tools) > 0 {
		decls := make([]map[string]any, 0, len(wr.Tools))
		for _, t := range wr.Tools {
			name, desc, schema, ok := normalizeToolDef(t)
			if !ok {
				continue
			}
			decls = append(decls, map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  schema,
			})
		}
		if len(decls) > 0 {
			payload["tools"] = []map[string]any{{"functionDeclarations": decls}}
		}
	}
	return json.Marshal(payload)
}

type geminiResponseEnvelope struct {
	ModelVersion string `json:"modelVersion"`
	Candidates   []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text             string `json:"text"`
				Thought          bool   `json:"thought"`
				ThoughtSignature string `json:"thoughtSignature"`
				FunctionCall     *struct {
					ID   string         `json:"id"`
					Name string         `json:"name"`
					Args map[string]any `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}

func (c *Client) execGemini(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	bodyBytes, err := buildGeminiPayload(wr)
	if err != nil {
		return nil, err
	}
	endpoint := geminiEndpoint(wr.ProvConfig.BaseURL, wr.Model, "generateContent")
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyGeminiHeaders(req, wr, false)
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

	var parsed geminiResponseEnvelope
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}
	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("no completion candidates returned by Gemini")
	}

	var textBuf, thinkBuf strings.Builder
	var lastSig string
	var parts []MessagePart
	var toolCalls []ToolCall
	cand := parsed.Candidates[0]
	for idx, p := range cand.Content.Parts {
		if p.ThoughtSignature != "" {
			lastSig = p.ThoughtSignature
		}
		if p.FunctionCall != nil {
			callID := p.FunctionCall.ID
			if callID == "" {
				callID = fmt.Sprintf("gemini-call-%d", idx+1)
			}
			argsRaw, _ := json.Marshal(p.FunctionCall.Args)
			toolCalls = append(toolCalls, ToolCall{
				ID:               callID,
				Name:             p.FunctionCall.Name,
				Arguments:        string(argsRaw),
				ThoughtSignature: p.ThoughtSignature,
			})
			parts = append(parts, MessagePart{
				Type:             "tool_call",
				ToolCallID:       callID,
				ToolName:         p.FunctionCall.Name,
				ToolArgs:         string(argsRaw),
				ThoughtSignature: p.ThoughtSignature,
			})
			continue
		}
		if p.Thought {
			thinkBuf.WriteString(p.Text)
			parts = append(parts, MessagePart{
				Type:             "thinking",
				Text:             p.Text,
				ThoughtSignature: p.ThoughtSignature,
			})
			continue
		}
		if p.Text != "" {
			textBuf.WriteString(p.Text)
			parts = append(parts, MessagePart{Type: "text", Text: p.Text})
		}
	}

	finish := strings.ToLower(cand.FinishReason)
	if finish == "max_tokens" {
		finish = "length"
	}
	var pt, ct, tt, cachedIn, reasonTok int
	usageOK := false
	if parsed.UsageMetadata != nil {
		pt = parsed.UsageMetadata.PromptTokenCount
		ct = parsed.UsageMetadata.CandidatesTokenCount
		tt = parsed.UsageMetadata.TotalTokenCount
		cachedIn = parsed.UsageMetadata.CachedContentTokenCount
		reasonTok = parsed.UsageMetadata.ThoughtsTokenCount
		usageOK = (pt + ct + tt) > 0
	}
	model := parsed.ModelVersion
	if model == "" {
		model = wr.Model
	}
	return &wireResponse{
		Content:            textBuf.String(),
		ReasoningContent:   thinkBuf.String(),
		ReasoningSignature: lastSig,
		Parts:              parts,
		ToolCalls:          toolCalls,
		Model:              model,
		FinishReason:       finish,
		PromptTokens:       pt,
		CompletionTokens:   ct,
		TotalTokens:        tt,
		CachedInputTokens:  cachedIn,
		ReasoningTokens:    reasonTok,
		UsageReported:      usageOK,
	}, nil
}

func (c *Client) streamGemini(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	bodyBytes, err := buildGeminiPayload(wr)
	if err != nil {
		return nil, err
	}
	endpoint := geminiEndpoint(wr.ProvConfig.BaseURL, wr.Model, "streamGenerateContent?alt=sse")
	resp, err := doWithRetry(ctx, c.httpCli, func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return nil, rerr
		}
		applyGeminiHeaders(req, wr, true)
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
	var finish string
	var pt, ct, tt, cachedIn, reasonTok int
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
		var env geminiResponseEnvelope
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			continue
		}
		if len(env.Candidates) > 0 {
			cand := env.Candidates[0]
			for _, p := range cand.Content.Parts {
				if p.Thought {
					thinkBuf.WriteString(p.Text)
				} else if p.Text != "" {
					if textBuf.Len()+len(p.Text) > maxLLMResponseBytes {
						return nil, fmt.Errorf("response exceeded %d byte ceiling during streaming", maxLLMResponseBytes)
					}
					textBuf.WriteString(p.Text)
					if onToken != nil {
						onToken(p.Text)
					}
				}
			}
			if cand.FinishReason != "" {
				finish = strings.ToLower(cand.FinishReason)
				if finish == "max_tokens" {
					finish = "length"
				}
			}
		}
		if env.UsageMetadata != nil {
			pt = env.UsageMetadata.PromptTokenCount
			ct = env.UsageMetadata.CandidatesTokenCount
			tt = env.UsageMetadata.TotalTokenCount
			cachedIn = env.UsageMetadata.CachedContentTokenCount
			reasonTok = env.UsageMetadata.ThoughtsTokenCount
		}
	}
	out := &wireResponse{
		Content:           textBuf.String(),
		ReasoningContent:  thinkBuf.String(),
		Model:             wr.Model,
		FinishReason:      finish,
		PromptTokens:      pt,
		CompletionTokens:  ct,
		TotalTokens:       tt,
		CachedInputTokens: cachedIn,
		ReasoningTokens:   reasonTok,
		UsageReported:     (pt + ct + tt) > 0,
	}
	out.RawUsage = buildUsageMap(out)
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}
