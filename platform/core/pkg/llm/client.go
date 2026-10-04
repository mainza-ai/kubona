package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
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
}

func NewClient(settings *Settings) *Client {
	return &Client{
		settings: settings,
		httpCli: &http.Client{
			Timeout: 180 * time.Second,
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

	maxTokens := snap.MaxTokens
	if maxTokens > 250 {
		maxTokens = 250
	}

	reqBody := ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: snap.Temperature,
		MaxTokens:   maxTokens,
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

	respBytes, err := io.ReadAll(resp.Body)
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
		return "", &chatResp, duration, fmt.Errorf("no completion choices returned by model")
	}

	return chatResp.Choices[0].Message.Content, &chatResp, duration, nil
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

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
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
