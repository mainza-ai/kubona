package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/rates"
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
	// ReasoningContent is emitted by reasoning models (NVIDIA Nemotron,
	// DeepSeek, etc.). It is not assistant content: callers must never
	// render or parse it as the answer.
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// Optional multimodal parts and opaque replay state (Anthropic thinking
	// signature, OpenAI Responses encrypted_reasoning, etc.).
	Parts              []MessagePart `json:"parts,omitempty"`
	ReasoningSignature string        `json:"reasoning_signature,omitempty"`
	EncryptedReasoning string        `json:"encrypted_reasoning,omitempty"`
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
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Meta    *InvocationMeta `json:"meta,omitempty"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
		Finish  string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens      int     `json:"prompt_tokens"`
		CompletionTokens  int     `json:"completion_tokens"`
		TotalTokens       int     `json:"total_tokens"`
		CachedInputTokens int     `json:"cached_input_tokens,omitempty"`
		CacheWriteTokens  int     `json:"cache_write_tokens,omitempty"`
		ReasoningTokens   int     `json:"reasoning_tokens,omitempty"`
		PromptTokPerSec   float64 `json:"prompt_tokens_per_second"`
		GenTokPerSec      float64 `json:"generation_tokens_per_second"`
		TotalTimeSec      float64 `json:"total_time"`
	} `json:"usage"`
}

// InvocationMeta records authoritative provenance for one completed LLM call:
// which provider/wire/model actually served it (post-failover), the effective
// sampling parameters, token breakdown, latency, and estimated USD spend.
type InvocationMeta struct {
	Provider          string   `json:"provider"`
	Wire              WireType `json:"wire"`
	Model             string   `json:"model"`
	Role              string   `json:"role,omitempty"`
	Temperature       float64  `json:"temperature"`
	TopP              *float64 `json:"top_p,omitempty"`
	Seed              *int64   `json:"seed,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	ReasoningEffort   string   `json:"reasoning_effort,omitempty"`
	PromptTokens      int      `json:"prompt_tokens"`
	CompletionTokens  int      `json:"completion_tokens"`
	CachedInputTokens int      `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens  int      `json:"cache_write_tokens,omitempty"`
	ReasoningTokens   int      `json:"reasoning_tokens,omitempty"`
	DurationMS        int64    `json:"duration_ms"`
	CostUSD           float64  `json:"cost_usd,omitempty"`
	AttemptCount      int      `json:"attempt_count"`
}

type ModelCapabilities struct {
	Vision           bool `json:"vision"`
	Tools            bool `json:"tools"`
	Reasoning        bool `json:"reasoning"`
	StructuredOutput bool `json:"structured_output"`
	Streaming        bool `json:"streaming"`
}

type ModelInfo struct {
	ID                  string            `json:"id"`
	OwnedBy             string            `json:"owned_by,omitempty"`
	MaxLen              int               `json:"max_model_len,omitempty"`
	ContextWindow       int               `json:"context_window,omitempty"`
	MaxOutputTokens     int               `json:"max_output_tokens,omitempty"`
	Provider            string            `json:"provider"`
	IsDefault           bool              `json:"is_default"`
	Capabilities        ModelCapabilities `json:"capabilities"`
	Priced              bool              `json:"priced"`
	PriceSource         string            `json:"price_source"`
	InputPer1KUSD       float64           `json:"input_per_1k_usd,omitempty"`
	OutputPer1KUSD      float64           `json:"output_per_1k_usd,omitempty"`
	CachedInputPer1KUSD float64           `json:"cached_input_per_1k_usd,omitempty"`
	CacheWritePer1KUSD  float64           `json:"cache_write_per_1k_usd,omitempty"`
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

func (c *Client) isProviderLocal(providerType ProviderType) bool {
	if c != nil && c.settings != nil {
		return c.settings.IsProviderLocal(providerType)
	}
	return isLocalProvider(providerType)
}

// reportSpend emits one accounting record for a completed call when a
// hook is set and the provider is metered (non-local).
func (c *Client) reportSpend(providerType ProviderType, model string, promptTokens, completionTokens int, usageReported bool) {
	c.reportSpendWithCache(providerType, model, promptTokens, completionTokens, 0, 0, usageReported)
}

func (c *Client) reportSpendWithCache(providerType ProviderType, model string, promptTokens, completionTokens, cachedInputTokens, cacheWriteTokens int, usageReported bool) {
	if c.OnSpend == nil {
		return
	}
	if c.isProviderLocal(providerType) {
		return
	}
	c.OnSpend(SpendRecord{
		Provider:          string(providerType),
		Model:             model,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		CachedInputTokens: cachedInputTokens,
		CacheWriteTokens:  cacheWriteTokens,
		UsageReported:     usageReported,
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
	if c.isProviderLocal(providerType) {
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
func (c *Client) requireMeteredUsage(providerType ProviderType, model string, usageReported bool, ceilingOverride int) error {
	return c.requireMeteredUsageForProvider(providerType, model, usageReported, ceilingOverride)
}

func (c *Client) requireMeteredUsageForProvider(providerType ProviderType, model string, usageReported bool, ceilingOverride int) error {
	if usageReported {
		return nil
	}
	if c.isProviderLocal(providerType) {
		return nil
	}
	ceiling := ceilingOverride
	if ceiling <= 0 {
		ceiling = c.settings.GetSnapshot().MaxTokens
	}
	if ceiling <= 0 {
		return fmt.Errorf("provider %s omitted usage and no token ceiling is configured (max_tokens=0): refusing blind cloud spend — set max_tokens or use a reporting provider", providerType)
	}
	c.reportSpendCeiling(providerType, model, ceiling)
	return nil
}

func NewClient(settings *Settings) *Client {
	return &Client{
		settings: settings,
		httpCli:  newGuardedHTTPClient(settings),
	}
}

// SettingsMaxTokens exposes the operator's configured token ceiling
// (0 = uncapped) for callers that bound their own worst case (tribunal
// turns take min(operator ceiling, turn cap) when the operator set one).
func (c *Client) SettingsMaxTokens() int {
	if c == nil || c.settings == nil {
		return 0
	}
	return c.settings.GetSnapshot().MaxTokens
}

// Complete executes a chat completion request with the configured provider and model
func (c *Client) Complete(ctx context.Context, role string, messages []ChatMessage) (string, *ChatCompletionResponse, time.Duration, error) {
	return c.completeWithMaxTokens(ctx, role, messages, -1)
}

// CompleteCapped is Complete with an explicit per-call token ceiling.
// maxTokens < 0 keeps the settings default (0 = omit, generate to EOS);
// maxTokens >= 0 overrides it for this call only. Added 2026-10-21: the
// synchronous tribunal bounds worst-case turn latency on slow local
// models (uncapped turns rambled past every context timeout on the
// default Strata stack); the global default stays uncapped on purpose
// (a 250-token ceiling once decapitated every reply — see below).
func (c *Client) CompleteCapped(ctx context.Context, role string, messages []ChatMessage, maxTokens int) (string, *ChatCompletionResponse, time.Duration, error) {
	return c.completeWithMaxTokens(ctx, role, messages, maxTokens)
}

func (c *Client) executeWire(ctx context.Context, wr wireRequest) (*wireResponse, error) {
	switch wr.Wire {
	case WireAnthropicMessages:
		return c.execAnthropicMessages(ctx, wr)
	case WireOpenAIResponses:
		return c.execOpenAIResponses(ctx, wr)
	case WireGemini:
		return c.execGemini(ctx, wr)
	case WireOllamaNative:
		return c.execOllamaChat(ctx, wr)
	default:
		return c.execChatCompletions(ctx, wr)
	}
}

func (c *Client) streamWire(ctx context.Context, wr wireRequest, onToken func(string)) (*wireResponse, error) {
	switch wr.Wire {
	case WireAnthropicMessages:
		return c.streamAnthropicMessages(ctx, wr, onToken)
	case WireOpenAIResponses:
		return c.streamOpenAIResponses(ctx, wr, onToken)
	case WireGemini:
		return c.streamGemini(ctx, wr, onToken)
	case WireOllamaNative:
		return c.streamOllamaChat(ctx, wr, onToken)
	default:
		return c.streamChatCompletions(ctx, wr, onToken)
	}
}

func (c *Client) validateTargetPolicy(snap *Settings, providerType ProviderType, model string) (ProviderConfig, error) {
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return ProviderConfig{}, fmt.Errorf("unknown provider %q", providerType)
	}
	if !provConfig.Enabled {
		return ProviderConfig{}, fmt.Errorf("provider %s is disabled: refusing (enable it before calling)", providerType)
	}
	local := c.isProviderLocal(providerType)
	if snap.AirGapped && !local {
		return ProviderConfig{}, &WireError{Fault: FaultAirGapBlocked, Endpoint: provConfig.BaseURL, Message: fmt.Sprintf("provider %s is blocked by Sovereign Air-Gap isolation", providerType)}
	}
	if productionCloudWithoutOptIn(local) {
		return ProviderConfig{}, &WireError{Fault: FaultCloudOptInRequired, Endpoint: provConfig.BaseURL, Message: fmt.Sprintf("provider %s is non-local and KUBONA_LLM_CLOUD=1 is not set: refusing production cloud egress", providerType)}
	}
	if strings.TrimSpace(model) == "" {
		return ProviderConfig{}, fmt.Errorf("provider %s has no model configured", providerType)
	}
	return provConfig, nil
}

type completionAttemptOpts struct {
	Role            string
	Temperature     float64
	TopP            *float64
	Seed            *int64
	MaxTokens       int
	ReasoningEffort string
	AttemptCount    int
}

func resolveAttemptMaxTokens(plan ResolvedRolePlan, maxTokensOverride int) int {
	maxTokens := plan.MaxTokens
	if maxTokensOverride >= 0 {
		maxTokens = maxTokensOverride
		if plan.HasMaxTokens && plan.MaxTokens > 0 && plan.MaxTokens < maxTokens {
			maxTokens = plan.MaxTokens
		}
	}
	return maxTokens
}

// CompleteWithTarget executes a completion against an explicit provider+model
// while enforcing all air-gap, opt-in, metering, and spend-hook invariants.
func (c *Client) CompleteWithTarget(ctx context.Context, providerType ProviderType, model string, messages []ChatMessage, maxTokensOverride int) (string, *ChatCompletionResponse, time.Duration, error) {
	snap := c.settings.GetSnapshot()
	if strings.TrimSpace(model) == "" {
		if pc, ok := snap.Providers[providerType]; ok {
			model = pc.DefaultModel
		}
	}
	provConfig, err := c.validateTargetPolicy(&snap, providerType, model)
	if err != nil {
		return "", nil, 0, err
	}
	maxTokens := snap.MaxTokens
	if maxTokensOverride >= 0 {
		maxTokens = maxTokensOverride
	}
	content, _, chatResp, dur, err := c.runCompletionAttempt(ctx, providerType, provConfig, model, messages, nil, completionAttemptOpts{
		Temperature:  snap.Temperature,
		MaxTokens:    maxTokens,
		AttemptCount: 1,
	})
	return content, chatResp, dur, err
}

func (c *Client) completeWithMaxTokens(ctx context.Context, role string, messages []ChatMessage, maxTokensOverride int) (string, *ChatCompletionResponse, time.Duration, error) {
	content, _, chatResp, dur, err := c.executeRoleHops(ctx, role, messages, nil, maxTokensOverride)
	return content, chatResp, dur, err
}

func (c *Client) executeRoleHops(ctx context.Context, role string, messages []ChatMessage, tools []map[string]any, maxTokensOverride int) (string, []ToolCall, *ChatCompletionResponse, time.Duration, error) {
	snap := c.settings.GetSnapshot()
	plan := c.settings.ResolveRolePlan(role)
	if len(plan.Hops) == 0 {
		_, _, err := c.settings.GetModelForRole(role)
		if err != nil {
			return "", nil, nil, 0, err
		}
		return "", nil, nil, 0, fmt.Errorf("role %q has no model hops configured", role)
	}
	if len(plan.Hops) == 1 {
		if _, _, err := c.settings.GetModelForRole(role); err != nil {
			return "", nil, nil, 0, err
		}
	}

	maxTokens := resolveAttemptMaxTokens(plan, maxTokensOverride)
	var attemptErrors []string
	var lastErr error
	var totalDur time.Duration

	for idx, hop := range plan.Hops {
		if cerr := ctx.Err(); cerr != nil {
			if lastErr != nil {
				return "", nil, nil, totalDur, fmt.Errorf("%w (prior hop failed: %v)", cerr, lastErr)
			}
			return "", nil, nil, totalDur, cerr
		}
		provConfig, verr := c.validateTargetPolicy(&snap, hop.Provider, hop.Model)
		if verr != nil {
			lastErr = verr
			attemptErrors = append(attemptErrors, fmt.Sprintf("%s/%s: %v", hop.Provider, hop.Model, verr))
			continue
		}
		content, calls, chatResp, dur, err := c.runCompletionAttempt(ctx, hop.Provider, provConfig, hop.Model, messages, tools, completionAttemptOpts{
			Role:            role,
			Temperature:     plan.Temperature,
			TopP:            plan.TopP,
			Seed:            plan.Seed,
			MaxTokens:       maxTokens,
			ReasoningEffort: plan.ReasoningEffort,
			AttemptCount:    idx + 1,
		})
		totalDur += dur
		if err == nil {
			return content, calls, chatResp, totalDur, nil
		}
		lastErr = err
		attemptErrors = append(attemptErrors, fmt.Sprintf("%s/%s: %v", hop.Provider, hop.Model, err))
	}

	if len(attemptErrors) <= 1 && lastErr != nil {
		return "", nil, nil, totalDur, lastErr
	}
	return "", nil, nil, totalDur, fmt.Errorf("all %d model hops failed for role %q (%s): %w", len(plan.Hops), role, strings.Join(attemptErrors, "; "), lastErr)
}

func (c *Client) runCompletionAttempt(ctx context.Context, providerType ProviderType, provConfig ProviderConfig, model string, messages []ChatMessage, tools []map[string]any, opts completionAttemptOpts) (string, []ToolCall, *ChatCompletionResponse, time.Duration, error) {
	temp := opts.Temperature
	wire := ResolveWire(providerType, provConfig)
	wr := wireRequest{
		Provider:        providerType,
		ProvConfig:      provConfig,
		Wire:            wire,
		Model:           model,
		Messages:        messages,
		Tools:           tools,
		Temperature:     &temp,
		TopP:            opts.TopP,
		Seed:            opts.Seed,
		MaxTokens:       opts.MaxTokens,
		ReasoningEffort: opts.ReasoningEffort,
	}

	start := time.Now()
	wresp, err := c.executeWire(ctx, wr)
	duration := time.Since(start)
	if err != nil {
		var wireErr *WireError
		if errors.As(err, &wireErr) && wireErr.AuthInvalid {
			c.settings.RecordProviderCheck(providerType, KeyStatusInvalid, wireErr.Fault, time.Now().UTC().Format(time.RFC3339), nil)
		}
		return "", nil, nil, duration, err
	}

	chatResp := wresp.toChatCompletionResponse()
	if err := c.requireMeteredUsageForProvider(providerType, model, wresp.UsageReported, opts.MaxTokens); err != nil {
		return "", nil, nil, duration, err
	}
	if wresp.UsageReported {
		c.reportSpendWithCache(providerType, model, wresp.PromptTokens, wresp.CompletionTokens, wresp.CachedInputTokens, wresp.CacheWriteTokens, true)
	}
	c.settings.RecordSuccess(providerType)

	chatResp.Meta = c.buildInvocationMeta(providerType, wire, model, wresp, duration, opts)

	if len(tools) == 0 && strings.TrimSpace(wresp.Content) == "" && strings.TrimSpace(wresp.ReasoningContent) != "" {
		return "", nil, chatResp, duration, fmt.Errorf(
			"model returned reasoning_content but no assistant content (finish_reason=%q, completion_tokens=%d): raise the role's max output budget or use a non-reasoning structured model",
			wresp.FinishReason, wresp.CompletionTokens)
	}
	return wresp.Content, wresp.ToolCalls, chatResp, duration, nil
}

func (c *Client) buildInvocationMeta(pt ProviderType, wire WireType, model string, wresp *wireResponse, duration time.Duration, opts completionAttemptOpts) *InvocationMeta {
	servedModel := model
	if wresp != nil && strings.TrimSpace(wresp.Model) != "" {
		servedModel = wresp.Model
	}
	var ptTok, ctTok, cachedTok, cacheWriteTok, reasonTok int
	if wresp != nil {
		ptTok = wresp.PromptTokens
		ctTok = wresp.CompletionTokens
		cachedTok = wresp.CachedInputTokens
		cacheWriteTok = wresp.CacheWriteTokens
		reasonTok = wresp.ReasoningTokens
	}
	costUSD, _ := EstimateCostUSDForSettings(c.settings, pt, servedModel, ptTok, ctTok, cachedTok, cacheWriteTok)
	if costUSD == 0 && servedModel != model {
		costUSD, _ = EstimateCostUSDForSettings(c.settings, pt, model, ptTok, ctTok, cachedTok, cacheWriteTok)
	}
	attempts := opts.AttemptCount
	if attempts <= 0 {
		attempts = 1
	}
	return &InvocationMeta{
		Provider:          string(pt),
		Wire:              wire,
		Model:             servedModel,
		Role:              opts.Role,
		Temperature:       opts.Temperature,
		TopP:              opts.TopP,
		Seed:              opts.Seed,
		MaxTokens:         opts.MaxTokens,
		ReasoningEffort:   opts.ReasoningEffort,
		PromptTokens:      ptTok,
		CompletionTokens:  ctTok,
		CachedInputTokens: cachedTok,
		CacheWriteTokens:  cacheWriteTok,
		ReasoningTokens:   reasonTok,
		DurationMS:        duration.Milliseconds(),
		CostUSD:           costUSD,
		AttemptCount:      attempts,
	}
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
	Meta  *InvocationMeta
}

// CompleteStream executes a streaming chat completion, invoking onToken for
// each content delta in arrival order across the provider's native wire dialect.
// Ordered role fallback hops run when an upstream attempt fails before any
// token is emitted; once tokens have begun streaming, failures surface loudly.
func (c *Client) CompleteStream(ctx context.Context, role string, messages []ChatMessage, onToken func(string)) (*StreamResult, error) {
	snap := c.settings.GetSnapshot()
	plan := c.settings.ResolveRolePlan(role)
	if len(plan.Hops) == 0 {
		_, _, err := c.settings.GetModelForRole(role)
		return nil, err
	}
	if len(plan.Hops) == 1 {
		if _, _, err := c.settings.GetModelForRole(role); err != nil {
			return nil, err
		}
	}

	var attemptErrors []string
	var lastErr error
	var totalDur time.Duration

	for idx, hop := range plan.Hops {
		if cerr := ctx.Err(); cerr != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w (prior hop failed: %v)", cerr, lastErr)
			}
			return nil, cerr
		}
		provConfig, verr := c.validateTargetPolicy(&snap, hop.Provider, hop.Model)
		if verr != nil {
			lastErr = verr
			attemptErrors = append(attemptErrors, fmt.Sprintf("%s/%s: %v", hop.Provider, hop.Model, verr))
			continue
		}
		temp := plan.Temperature
		wire := ResolveWire(hop.Provider, provConfig)
		wr := wireRequest{
			Provider:        hop.Provider,
			ProvConfig:      provConfig,
			Wire:            wire,
			Model:           hop.Model,
			Messages:        messages,
			Temperature:     &temp,
			TopP:            plan.TopP,
			Seed:            plan.Seed,
			MaxTokens:       plan.MaxTokens,
			ReasoningEffort: plan.ReasoningEffort,
		}

		emitted := false
		wrappedOnToken := func(tok string) {
			if tok != "" {
				emitted = true
			}
			if onToken != nil {
				onToken(tok)
			}
		}

		start := time.Now()
		wresp, err := c.streamWire(ctx, wr, wrappedOnToken)
		dur := time.Since(start)
		totalDur += dur
		if err != nil {
			var wireErr *WireError
			if errors.As(err, &wireErr) && wireErr.AuthInvalid {
				c.settings.RecordProviderCheck(hop.Provider, KeyStatusInvalid, wireErr.Fault, time.Now().UTC().Format(time.RFC3339), nil)
			}
			if emitted {
				var usage map[string]any
				var text, finish string
				if wresp != nil {
					usage = buildUsageMap(wresp)
					text = wresp.Content
					finish = wresp.FinishReason
				}
				return &StreamResult{Text: text, Finish: finish, Duration: totalDur, Usage: usage}, err
			}
			lastErr = err
			attemptErrors = append(attemptErrors, fmt.Sprintf("%s/%s: %v", hop.Provider, hop.Model, err))
			continue
		}

		usage := buildUsageMap(wresp)
		c.reportSpendMap(hop.Provider, hop.Model, usage)
		c.settings.RecordSuccess(hop.Provider)
		meta := c.buildInvocationMeta(hop.Provider, wire, hop.Model, wresp, totalDur, completionAttemptOpts{
			Role:            role,
			Temperature:     temp,
			TopP:            plan.TopP,
			Seed:            plan.Seed,
			MaxTokens:       plan.MaxTokens,
			ReasoningEffort: plan.ReasoningEffort,
			AttemptCount:    idx + 1,
		})
		return &StreamResult{Text: wresp.Content, Finish: wresp.FinishReason, Duration: totalDur, Usage: usage, Meta: meta}, nil
	}

	if len(attemptErrors) <= 1 && lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("all %d streaming hops failed for role %q (%s): %w", len(plan.Hops), role, strings.Join(attemptErrors, "; "), lastErr)
}

// reportSpendMap adapts provider-reported usage maps (float64 JSON
// numbers) to reportSpend. Absent keys mean unreported, never zero-priced:
// non-local streams without usage take the ceiling-or-refuse rule (W0.19);
// without a ceiling the unmetered record is flagged, never priced zero.
func (c *Client) reportSpendMap(providerType ProviderType, model string, usage map[string]any) {
	if usage == nil {
		if err := c.requireMeteredUsage(providerType, model, false, 0); err != nil {
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
	cachedIn, cacheWrite := num("cached_input_tokens"), num("cache_write_tokens")
	if pt+ct > 0 {
		c.reportSpendWithCache(providerType, model, pt, ct, cachedIn, cacheWrite, true)
		return
	}
	if err := c.requireMeteredUsage(providerType, model, false, 0); err != nil {
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
	if c.isProviderLocal(providerType) {
		return
	}
	c.OnSpend(SpendRecord{
		Provider:      string(providerType),
		Model:         model,
		UsageReported: false,
		Unmetered:     true,
	})
}

// VerifyRequest allows validating either the persisted credential for a
// provider or an unsaved transient credential + endpoint override before saving.
type VerifyRequest struct {
	APIKey          string   `json:"api_key,omitempty"`
	BaseURL         string   `json:"base_url,omitempty"`
	Wire            WireType `json:"wire,omitempty"`
	CheckGeneration uint64   `json:"check_generation,omitempty"`
}

// VerifyResult reports the free metadata verification outcome without invoking
// any billed text-generation endpoint.
type VerifyResult struct {
	OK              bool        `json:"ok"`
	Provider        string      `json:"provider"`
	Wire            WireType    `json:"wire"`
	KeyStatus       string      `json:"key_status"`
	Fault           string      `json:"fault,omitempty"`
	Error           string      `json:"error,omitempty"`
	LatencyMS       int64       `json:"latency_ms"`
	Models          []ModelInfo `json:"models"`
	CheckGeneration uint64      `json:"check_generation"`
}

// VerifyProvider performs a free credential & reachability check against the
// provider's metadata endpoint (/models, /api/tags, OpenRouter /key).
func (c *Client) VerifyProvider(ctx context.Context, pt ProviderType, req VerifyRequest) (*VerifyResult, error) {
	snap := c.settings.GetSnapshot()
	pc, ok := snap.Providers[pt]
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", pt)
	}

	hasTransientKey := strings.TrimSpace(req.APIKey) != ""
	hasURLOverride := strings.TrimSpace(req.BaseURL) != ""
	if req.Wire != "" {
		if !validWireType(req.Wire) {
			return nil, fmt.Errorf("unsupported wire protocol %q", req.Wire)
		}
		pc.Wire = req.Wire
	}
	if hasURLOverride {
		cleaned, isLocal, uerr := ValidateEndpointURL(pt, req.BaseURL)
		if uerr != nil {
			return &VerifyResult{
				OK:              false,
				Provider:        string(pt),
				Wire:            ResolveWire(pt, pc),
				KeyStatus:       baselineKeyStatus(pc),
				Fault:           FaultInvalidEndpoint,
				Error:           uerr.Error(),
				Models:          []ModelInfo{},
				CheckGeneration: req.CheckGeneration,
			}, nil
		}
		pc.BaseURL = cleaned
		if pt == ProviderCustom || isLocalProvider(pt) {
			pc.IsLocal = isLocal
		}
	}

	wire := ResolveWire(pt, pc)
	local := pc.IsLocal
	if !local {
		if snap.AirGapped {
			return &VerifyResult{
				OK:              false,
				Provider:        string(pt),
				Wire:            wire,
				KeyStatus:       baselineKeyStatus(pc),
				Fault:           FaultAirGapBlocked,
				Error:           fmt.Sprintf("provider %s is blocked by Sovereign Air-Gap isolation", pt),
				Models:          []ModelInfo{},
				CheckGeneration: req.CheckGeneration,
			}, nil
		}
		if productionCloudWithoutOptIn(local) {
			return &VerifyResult{
				OK:              false,
				Provider:        string(pt),
				Wire:            wire,
				KeyStatus:       baselineKeyStatus(pc),
				Fault:           FaultCloudOptInRequired,
				Error:           fmt.Sprintf("provider %s is non-local and KUBONA_LLM_CLOUD=1 is not set", pt),
				Models:          []ModelInfo{},
				CheckGeneration: req.CheckGeneration,
			}, nil
		}
	}

	keyBytes := []byte(pc.APIKey)
	if hasTransientKey {
		wipeBytes(keyBytes)
		keyBytes = []byte(strings.TrimSpace(req.APIKey))
	}
	defer wipeBytes(keyBytes)

	if !local && len(keyBytes) == 0 {
		return &VerifyResult{
			OK:              false,
			Provider:        string(pt),
			Wire:            wire,
			KeyStatus:       KeyStatusMissing,
			Fault:           FaultWrongKey,
			Error:           fmt.Sprintf("provider %s has no API key configured", pt),
			Models:          []ModelInfo{},
			CheckGeneration: req.CheckGeneration,
		}, nil
	}

	start := time.Now()
	models, err := c.fetchModelsWithKey(ctx, pt, pc, string(keyBytes))
	latency := time.Since(start).Milliseconds()

	if err == nil && pt == ProviderOpenRouter && len(keyBytes) > 0 {
		if kerr := c.verifyOpenRouterKeyEndpoint(ctx, pc.BaseURL, string(keyBytes)); kerr != nil {
			err = kerr
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		fault := FaultUnreachable
		status := KeyStatusSet
		if len(keyBytes) == 0 && !local {
			status = KeyStatusMissing
		}
		var wireErr *WireError
		if errors.As(err, &wireErr) {
			if wireErr.Fault != "" {
				fault = wireErr.Fault
			}
			if wireErr.AuthInvalid {
				status = KeyStatusInvalid
			}
		}
		if !hasTransientKey && !hasURLOverride {
			c.settings.RecordProviderCheck(pt, status, fault, nowStr, nil)
		}
		return &VerifyResult{
			OK:              false,
			Provider:        string(pt),
			Wire:            wire,
			KeyStatus:       status,
			Fault:           fault,
			Error:           err.Error(),
			LatencyMS:       latency,
			Models:          []ModelInfo{},
			CheckGeneration: req.CheckGeneration,
		}, nil
	}

	if !hasTransientKey && !hasURLOverride {
		c.settings.RecordProviderCheck(pt, KeyStatusValid, FaultNone, nowStr, nil)
	}
	if models == nil {
		models = []ModelInfo{}
	}
	return &VerifyResult{
		OK:              true,
		Provider:        string(pt),
		Wire:            wire,
		KeyStatus:       KeyStatusValid,
		LatencyMS:       latency,
		Models:          models,
		CheckGeneration: req.CheckGeneration,
	}, nil
}

func (c *Client) verifyOpenRouterKeyEndpoint(ctx context.Context, baseURL, apiKey string) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/key"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	body, _ := readBoundedBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fault, retryable, authInvalid := classifyHTTPFault(resp.StatusCode, body)
		return &WireError{Fault: fault, Status: resp.StatusCode, Endpoint: endpoint, Message: string(body), Retryable: retryable, AuthInvalid: authInvalid}
	}
	return nil
}

// FetchModels queries the provider's metadata endpoint to discover live available models.
func (c *Client) FetchModels(ctx context.Context, providerType ProviderType) ([]ModelInfo, error) {
	snap := c.settings.GetSnapshot()
	provConfig, ok := snap.Providers[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", providerType)
	}
	return c.fetchModelsWithKey(ctx, providerType, provConfig, provConfig.APIKey)
}

func (c *Client) fetchModelsWithKey(ctx context.Context, providerType ProviderType, provConfig ProviderConfig, apiKey string) ([]ModelInfo, error) {
	wire := ResolveWire(providerType, provConfig)
	var endpoint string
	var req *http.Request
	var err error

	switch wire {
	case WireOllamaNative:
		endpoint = ollamaRootURL(provConfig.BaseURL) + "/api/tags"
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	case WireGemini:
		endpoint = strings.TrimRight(provConfig.BaseURL, "/") + "/models"
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil && apiKey != "" {
			req.Header.Set("x-goog-api-key", apiKey)
		}
	case WireAnthropicMessages:
		endpoint = anthropicEndpoint(provConfig.BaseURL, "/models")
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			req.Header.Set("anthropic-version", anthropicAPIVersion)
			if apiKey != "" {
				req.Header.Set("x-api-key", apiKey)
			}
		}
	default:
		endpoint = fmt.Sprintf("%s/models", strings.TrimRight(provConfig.BaseURL, "/"))
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil && apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		}
	}
	if err != nil {
		return nil, err
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, &WireError{Fault: FaultUnreachable, Endpoint: endpoint, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	bounded, err := readBoundedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		fault, retryable, authInvalid := classifyHTTPFault(resp.StatusCode, bounded)
		return nil, &WireError{
			Fault:       fault,
			Status:      resp.StatusCode,
			Endpoint:    endpoint,
			Message:     fmt.Sprintf("status %d from %s: %s", resp.StatusCode, endpoint, strings.TrimSpace(string(bounded))),
			Retryable:   retryable,
			AuthInvalid: authInvalid,
		}
	}

	isLocal := c.isProviderLocal(providerType)
	models := []ModelInfo{}
	switch wire {
	case WireOllamaNative:
		var ollamaRaw struct {
			Models []struct {
				Name  string `json:"name"`
				Model string `json:"model"`
			} `json:"models"`
		}
		if err := json.Unmarshal(bounded, &ollamaRaw); err != nil {
			return nil, err
		}
		for _, m := range ollamaRaw.Models {
			id := m.Name
			if id == "" {
				id = m.Model
			}
			if id == "" {
				continue
			}
			mi := ModelInfo{
				ID:           id,
				OwnedBy:      "ollama",
				Provider:     string(providerType),
				IsDefault:    id == provConfig.DefaultModel,
				Capabilities: ModelCapabilities{Streaming: true},
			}
			enrichModelInfo(&mi, providerType, isLocal, nil)
			models = append(models, mi)
		}
	case WireGemini:
		var geminiRaw struct {
			Models []struct {
				Name             string   `json:"name"`
				DisplayName      string   `json:"displayName"`
				InputTokenLimit  int      `json:"inputTokenLimit"`
				OutputTokenLimit int      `json:"outputTokenLimit"`
				SupportedMethods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		if err := json.Unmarshal(bounded, &geminiRaw); err != nil {
			return nil, err
		}
		for _, m := range geminiRaw.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if id == "" {
				continue
			}
			supportsGen := false
			for _, sm := range m.SupportedMethods {
				if sm == "generateContent" || sm == "streamGenerateContent" {
					supportsGen = true
				}
			}
			if !supportsGen && len(m.SupportedMethods) > 0 {
				continue
			}
			mi := ModelInfo{
				ID:              id,
				OwnedBy:         "google",
				MaxLen:          m.InputTokenLimit,
				ContextWindow:   m.InputTokenLimit,
				MaxOutputTokens: m.OutputTokenLimit,
				Provider:        string(providerType),
				IsDefault:       id == provConfig.DefaultModel,
				Capabilities: ModelCapabilities{
					Tools:            true,
					StructuredOutput: true,
					Streaming:        true,
				},
			}
			enrichModelInfo(&mi, providerType, isLocal, nil)
			models = append(models, mi)
		}
	default:
		var raw struct {
			Data []struct {
				ID            string `json:"id"`
				OwnedBy       string `json:"owned_by"`
				MaxLen        int    `json:"max_model_len"`
				ContextLength int    `json:"context_length"`
				MaxTokens     int    `json:"max_tokens"`
				TopProvider   *struct {
					ContextLength       int `json:"context_length"`
					MaxCompletionTokens int `json:"max_completion_tokens"`
				} `json:"top_provider"`
				Architecture *struct {
					InputModalities []string `json:"input_modalities"`
				} `json:"architecture"`
				SupportedParameters []string `json:"supported_parameters"`
				Pricing             *struct {
					Prompt     string `json:"prompt"`
					Completion string `json:"completion"`
				} `json:"pricing"`
			} `json:"data"`
		}
		if err := json.Unmarshal(bounded, &raw); err != nil {
			return nil, err
		}
		for _, m := range raw.Data {
			ctxLen := m.MaxLen
			if ctxLen == 0 {
				ctxLen = m.ContextLength
			}
			if ctxLen == 0 && m.TopProvider != nil {
				ctxLen = m.TopProvider.ContextLength
			}
			maxOut := m.MaxTokens
			if maxOut == 0 && m.TopProvider != nil {
				maxOut = m.TopProvider.MaxCompletionTokens
			}
			caps := ModelCapabilities{Streaming: true}
			if m.Architecture != nil {
				for _, mod := range m.Architecture.InputModalities {
					if strings.EqualFold(mod, "image") {
						caps.Vision = true
					}
				}
			}
			for _, sp := range m.SupportedParameters {
				switch strings.ToLower(sp) {
				case "tools":
					caps.Tools = true
				case "reasoning", "include_reasoning":
					caps.Reasoning = true
				case "structured_outputs", "response_format":
					caps.StructuredOutput = true
				}
			}
			var live *livePriceInfo
			if m.Pricing != nil {
				if inTok, err1 := strconv.ParseFloat(strings.TrimSpace(m.Pricing.Prompt), 64); err1 == nil {
					if outTok, err2 := strconv.ParseFloat(strings.TrimSpace(m.Pricing.Completion), 64); err2 == nil && inTok >= 0 && outTok >= 0 {
						live = &livePriceInfo{
							inputPer1K:  inTok * 1000.0,
							outputPer1K: outTok * 1000.0,
							source:      "openrouter_live",
						}
					}
				}
			}
			owned := m.OwnedBy
			if owned == "" && wire == WireAnthropicMessages {
				owned = "anthropic"
			}
			mi := ModelInfo{
				ID:              m.ID,
				OwnedBy:         owned,
				MaxLen:          ctxLen,
				ContextWindow:   ctxLen,
				MaxOutputTokens: maxOut,
				Provider:        string(providerType),
				IsDefault:       m.ID == provConfig.DefaultModel,
				Capabilities:    caps,
			}
			enrichModelInfo(&mi, providerType, isLocal, live)
			models = append(models, mi)
		}
	}

	return models, nil
}

type livePriceInfo struct {
	inputPer1K  float64
	outputPer1K float64
	source      string
}

func enrichModelInfo(mi *ModelInfo, pt ProviderType, isLocal bool, live *livePriceInfo) {
	if mi == nil {
		return
	}
	if mi.ContextWindow == 0 && mi.MaxLen > 0 {
		mi.ContextWindow = mi.MaxLen
	}
	if mi.MaxLen == 0 && mi.ContextWindow > 0 {
		mi.MaxLen = mi.ContextWindow
	}
	lowID := strings.ToLower(mi.ID)
	if strings.Contains(lowID, "vl") || strings.Contains(lowID, "vision") || strings.Contains(lowID, "4o") || strings.Contains(lowID, "claude-") || strings.Contains(lowID, "gemini-") {
		mi.Capabilities.Vision = true
	}
	if strings.Contains(lowID, "o1") || strings.Contains(lowID, "o3") || strings.Contains(lowID, "o4") || strings.Contains(lowID, "gpt-5") || strings.Contains(lowID, "gpt-6") || strings.Contains(lowID, "r1") || strings.Contains(lowID, "reason") || strings.Contains(lowID, "thinking") || strings.Contains(lowID, "nemotron") {
		mi.Capabilities.Reasoning = true
	}

	if r, ok := rates.LookupModelRate(string(pt), mi.ID); ok {
		mi.InputPer1KUSD = r.InputPer1KUSD
		mi.OutputPer1KUSD = r.OutputPer1KUSD
		mi.CachedInputPer1KUSD = r.CachedInputPer1KUSD
		mi.CacheWritePer1KUSD = r.CacheWritePer1KUSD
		if mi.ContextWindow == 0 && r.ContextWindow > 0 {
			mi.ContextWindow = r.ContextWindow
			mi.MaxLen = r.ContextWindow
		}
		if mi.MaxOutputTokens == 0 && r.MaxOutputTokens > 0 {
			mi.MaxOutputTokens = r.MaxOutputTokens
		}
		for _, capName := range r.Capabilities {
			switch strings.ToLower(capName) {
			case "vision":
				mi.Capabilities.Vision = true
			case "tools":
				mi.Capabilities.Tools = true
			case "reasoning":
				mi.Capabilities.Reasoning = true
			case "structured_output":
				mi.Capabilities.StructuredOutput = true
			case "streaming":
				mi.Capabilities.Streaming = true
			}
		}
		if isLocal {
			mi.PriceSource = "local_free"
			mi.Priced = false
			return
		}
		src := r.Confidence
		if src == "" {
			src = "vendor"
		}
		mi.Priced = true
		mi.PriceSource = src
		return
	}

	if isLocal {
		mi.PriceSource = "local_free"
		mi.Priced = false
		return
	}
	if live != nil {
		mi.InputPer1KUSD = live.inputPer1K
		mi.OutputPer1KUSD = live.outputPer1K
		mi.Priced = true
		mi.PriceSource = live.source
		return
	}
	mi.Priced = false
	mi.PriceSource = "unpriced"
}
