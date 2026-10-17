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

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls carries model-requested function calls on assistant
	// messages; ToolCallID/ToolName mark tool-result messages (role tool).
	// Empty on plain chat; serialized to the OpenAI wire shape by the
	// tools path, never by callers.
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

type ChatCompletionRequest struct {
	Model string `json:"model"`
	// Messages is the wire-ready payload: plain chat marshals identically,
	// while tool_calls / tool results take the OpenAI function shape via
	// withToolMessages. Callers pass ChatMessage slices through it.
	Messages    any     `json:"messages"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
		Finish  string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		TotalTokens      int     `json:"total_tokens"`
		PromptTokPerSec  float64 `json:"prompt_tokens_per_second"`
		GenTokPerSec     float64 `json:"generation_tokens_per_second"`
		TotalTimeSec     float64 `json:"total_time"`
	} `json:"usage"`
}

type ModelInfo struct {
	ID        string `json:"id"`
	OwnedBy   string `json:"owned_by,omitempty"`
	MaxLen    int    `json:"max_model_len,omitempty"`
	Provider  string `json:"provider"`
	IsDefault bool   `json:"is_default"`
}

type Client struct {
	settings *Settings
	httpCli  *http.Client
	// OnSpend, when set, fires after every non-local completion with the
	// provider-reported usage. Fire-and-forget by contract: it runs
	// synchronously but must never fail the call (accounting observes,
	// never gates — gating happens pre-call at the edge).
	OnSpend func(SpendRecord)
}

// maxLLMResponseBytes bounds a single provider response body (W2.20). A
// malicious or broken endpoint must not OOM the daemon with an unbounded
// read: bodies past the cap refuse instead of buffering forever. 8MB fits
// any real chat completion (long turns are streamed, not buffered).
const maxLLMResponseBytes = 8 * 1024 * 1024

// readBoundedBody reads at most maxLLMResponseBytes+1 and refuses past the
// cap — the +1 distinguishes "exactly full" from "overflowing".
func readBoundedBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxLLMResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxLLMResponseBytes {
		return nil, fmt.Errorf("provider response exceeds %d-byte cap: refusing unbounded body", maxLLMResponseBytes)
	}
	return raw, nil
}

// reportSpend emits one accounting record for a completed call when a
// hook is set and the provider is metered (non-local).
func (c *Client) reportSpend(providerType ProviderType, model string, promptTokens, completionTokens int, usageReported bool) {
	if c.OnSpend == nil {
		return
	}
	if providerType == ProviderOMLX || providerType == ProviderOllama || providerType == ProviderLMStudio {
		return
	}
	c.OnSpend(SpendRecord{
		Provider:         string(providerType),
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		UsageReported:    usageReported,
	})
}

// reportSpendCeiling records a ceiling estimate when a non-local provider
// omits usage (W0.19): tokens unknown, so the role's token ceiling bills at
// the output rate (upper bound) instead of the call billing zero. Never
// silent — CeilingApplied marks the record.
func (c *Client) reportSpendCeiling(providerType ProviderType, model string, ceilingTokens int) {
	if c.OnSpend == nil {
		return
	}
	if providerType == ProviderOMLX || providerType == ProviderOllama || providerType == ProviderLMStudio {
		return
	}
	c.OnSpend(SpendRecord{
		Provider:         string(providerType),
		Model:            model,
		PromptTokens:     ceilingTokens,
		CompletionTokens: 0,
		UsageReported:    false,
		CeilingApplied:   true,
		CeilingTokens:    ceilingTokens,
	})
}

// requireMeteredUsage enforces the W0.19 rule for non-local completions:
// reported usage bills exact; unreported usage bills the token ceiling when
// one is configured, and refuses when it isn't (blind cloud spend with no
// ceiling is un-meterable — the call fails instead of billing zero).
// Local providers are exempt (no external spend). Returns nil to proceed.
func (c *Client) requireMeteredUsage(providerType ProviderType, model string, usageReported bool) error {
	if usageReported {
		return nil
	}
	if providerType == ProviderOMLX || providerType == ProviderOllama || providerType == ProviderLMStudio {
		return nil
	}
	ceiling := c.settings.GetSnapshot().MaxTokens
	if ceiling <= 0 {
		return fmt.Errorf("provider %s omitted usage and no token ceiling is configured (max_tokens=0): refusing blind cloud spend — set max_tokens or use a reporting provider", providerType)
	}
	c.reportSpendCeiling(providerType, model, ceiling)
	return nil
}

func NewClient(settings *Settings) *Client {
	return &Client{
		settings: settings,
		httpCli: &http.Client{
			// Generous: uncapped local generations run ~6 tok/s, so a full
			// answer can take minutes. The UI shows elapsed + overdue states;
			// this is a backstop, not a budget (2026-10-11).
			Timeout: 600 * time.Second,
		},
	}
}

// Complete executes a chat completion request with the configured provider and model
func (c *Client) Complete(ctx context.Context, role string, messages []ChatMessage) (string, *ChatCompletionResponse, time.Duration, error) {
	providerType, model, err := c.settings.GetModelForRole(role)
	if err != nil {
		return "", nil, 0, err
	}

	snap := c.settings.GetSnapshot()
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return "", nil, 0, fmt.Errorf("provider configuration not found for %s", providerType)
	}

	// MaxTokens passes straight through: 0 omits the field (omitempty) and
	// the model generates to EOS / its own context window. There is
	// deliberately no client-side clamp — a 250-token ceiling once
	// decapitated every reply mid-sentence (2026-10-11). Long turns are
	// governed by the result contract (finish_reason) and the UI run-state,
	// not by truncation here.
	reqBody := ChatCompletionRequest{
		Model:       model,
		Messages:    withToolMessages(messages),
		Temperature: snap.Temperature,
		MaxTokens:   snap.MaxTokens,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, 0, err
	}

	endpoint := fmt.Sprintf("%s/chat/completions", provConfig.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	if provConfig.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provConfig.APIKey))
	}

	start := time.Now()
	resp, err := c.httpCli.Do(req)
	duration := time.Since(start)
	if err != nil {
		return "", nil, duration, fmt.Errorf("HTTP request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, err := readBoundedBody(resp.Body)
	if err != nil {
		return "", nil, duration, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, duration, fmt.Errorf("API error (%d) from %s: %s", resp.StatusCode, endpoint, string(respBytes))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", nil, duration, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", nil, duration, fmt.Errorf("no completion choices returned by model")
	}

	usageOK := chatResp.Usage.TotalTokens > 0
	if err := c.requireMeteredUsage(providerType, model, usageOK); err != nil {
		return "", nil, duration, err
	}
	if usageOK {
		c.reportSpend(providerType, model,
			chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens, true)
	}
	return chatResp.Choices[0].Message.Content, &chatResp, duration, nil
}

// StreamChunk is one SSE delta from a streaming completion.
type StreamChunk struct {
	Content      string
	FinishReason string
	Done         bool
}

// StreamResult is the accumulated outcome of a streaming completion.
type StreamResult struct {
	Text     string
	Finish   string
	Duration time.Duration
	// Usage is provider-reported token accounting when the endpoint sends
	// it (final usage chunk); nil when absent — never estimated.
	Usage map[string]any
}

// CompleteStream executes a streaming chat completion, invoking onToken for
// each content delta in arrival order. Both oMLX and OpenAI-compatible cloud
// endpoints speak this SSE dialect.
func (c *Client) CompleteStream(ctx context.Context, role string, messages []ChatMessage, onToken func(string)) (*StreamResult, error) {
	providerType, model, err := c.settings.GetModelForRole(role)
	if err != nil {
		return nil, err
	}

	snap := c.settings.GetSnapshot()
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return nil, fmt.Errorf("provider configuration not found for %s", providerType)
	}

	body := map[string]any{
		"model":          model,
		"messages":       withToolMessages(messages),
		"temperature":    snap.Temperature,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if snap.MaxTokens > 0 {
		body["max_tokens"] = snap.MaxTokens
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/chat/completions", provConfig.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if provConfig.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provConfig.APIKey))
	}

	start := time.Now()
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("API error (%d) from %s: %s", resp.StatusCode, endpoint, string(slurp))
	}

	var full strings.Builder
	finish := ""
	var usage map[string]any
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
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta.Content != "" {
			full.WriteString(ch.Delta.Content)
			if onToken != nil {
				onToken(ch.Delta.Content)
			}
		}
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
	}
	if err := sc.Err(); err != nil {
		return &StreamResult{Text: full.String(), Finish: finish, Duration: time.Since(start), Usage: usage}, err
	}
	c.reportSpendMap(providerType, model, usage)
	return &StreamResult{Text: full.String(), Finish: finish, Duration: time.Since(start), Usage: usage}, nil
}

// reportSpendMap adapts provider-reported usage maps (float64 JSON
// numbers) to reportSpend. Absent keys mean unreported, never zero-priced:
// non-local streams without usage take the ceiling-or-refuse rule (W0.19);
// without a ceiling the unmetered record is flagged, never priced zero.
func (c *Client) reportSpendMap(providerType ProviderType, model string, usage map[string]any) {
	if usage == nil {
		if err := c.requireMeteredUsage(providerType, model, false); err != nil {
			c.reportUnmetered(providerType, model)
		}
		return
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
	pt, ct := num("prompt_tokens"), num("completion_tokens")
	if pt+ct > 0 {
		c.reportSpend(providerType, model, pt, ct, true)
		return
	}
	if err := c.requireMeteredUsage(providerType, model, false); err != nil {
		c.reportUnmetered(providerType, model)
	}
}

// reportUnmetered records a blind-spend flag when neither usage nor ceiling
// exists (streams only — Complete refuses instead). The budget gate trips
// on unmetered records when a cap is set; track-only mode keeps them visible.
func (c *Client) reportUnmetered(providerType ProviderType, model string) {
	if c.OnSpend == nil {
		return
	}
	if providerType == ProviderOMLX || providerType == ProviderOllama || providerType == ProviderLMStudio {
		return
	}
	c.OnSpend(SpendRecord{
		Provider:      string(providerType),
		Model:         model,
		UsageReported: false,
		Unmetered:     true,
	})
}

// FetchModels queries the provider's /models endpoint to discover live available models
func (c *Client) FetchModels(ctx context.Context, providerType ProviderType) ([]ModelInfo, error) {
	snap := c.settings.GetSnapshot()
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", providerType)
	}

	endpoint := fmt.Sprintf("%s/models", provConfig.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if provConfig.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provConfig.APIKey))
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, endpoint)
	}

	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
			MaxLen  int    `json:"max_model_len"`
		} `json:"data"`
	}

	// W2.20: bounded decode — the /models body is small; cap it like
	// completions instead of decoding an unbounded stream.
	bounded, err := readBoundedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bounded, &raw); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range raw.Data {
		models = append(models, ModelInfo{
			ID:        m.ID,
			OwnedBy:   m.OwnedBy,
			MaxLen:    m.MaxLen,
			Provider:  string(providerType),
			IsDefault: m.ID == provConfig.DefaultModel,
		})
	}

	return models, nil
}
