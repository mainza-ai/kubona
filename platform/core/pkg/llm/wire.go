package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Closed credential/transport fault categories surfaced by Execute/Verify.
const (
	FaultNone               = ""
	FaultWrongKey           = "wrong_key"
	FaultNoCredits          = "no_credits"
	FaultRateLimited        = "rate_limited"
	FaultServiceUnavailable = "service_unavailable"
	FaultModelNotFound      = "model_not_found"
	FaultBadRequest         = "bad_request"
	FaultUnreachable        = "unreachable"
	FaultAirGapBlocked      = "air_gap_blocked"
	FaultCloudOptInRequired = "cloud_opt_in_required"
	FaultInvalidEndpoint    = "invalid_endpoint"
)

// MessagePart represents one typed fragment inside a normalized conversation turn
// across OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, Gemini,
// and Ollama native wire adapters.
type MessagePart struct {
	Type             string `json:"type"` // "text" | "image" | "tool_call" | "tool_result" | "thinking"
	Text             string `json:"text,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	DataBase64       string `json:"data_base64,omitempty"`
	ImageURL         string `json:"image_url,omitempty"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	ToolArgs         string `json:"tool_args,omitempty"`
	IsError          bool   `json:"is_error,omitempty"`
	Signature        string `json:"signature,omitempty"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// WireError captures a structured upstream or policy failure without ever
// embedding secret key bytes.
type WireError struct {
	Fault       string
	Status      int
	Endpoint    string
	Message     string
	Retryable   bool
	AuthInvalid bool
}

func (e *WireError) Error() string {
	if e == nil {
		return ""
	}
	if e.Status > 0 {
		return fmt.Sprintf("API error (%d, %s) from %s: %s", e.Status, e.Fault, e.Endpoint, e.Message)
	}
	return fmt.Sprintf("%s (%s): %s", e.Endpoint, e.Fault, e.Message)
}

func classifyHTTPFault(status int, body []byte) (fault string, retryable bool, authInvalid bool) {
	lower := strings.ToLower(string(body))
	var parsedErr struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsedErr)
	msg := strings.ToLower(parsedErr.Error.Message + " " + parsedErr.Error.Code + " " + parsedErr.Error.Type + " " + parsedErr.Error.Status + " " + parsedErr.Message)
	combined := lower + " " + msg

	switch status {
	case http.StatusUnauthorized:
		return FaultWrongKey, false, true
	case http.StatusPaymentRequired:
		return FaultNoCredits, false, false
	case http.StatusForbidden:
		if strings.Contains(combined, "credit") || strings.Contains(combined, "billing") || strings.Contains(combined, "quota") {
			return FaultNoCredits, false, false
		}
		if strings.Contains(combined, "key") || strings.Contains(combined, "auth") || strings.Contains(combined, "permission_denied") || strings.Contains(combined, "api_key_invalid") {
			return FaultWrongKey, false, true
		}
		return FaultWrongKey, false, true
	case http.StatusNotFound:
		return FaultModelNotFound, false, false
	case http.StatusTooManyRequests:
		if strings.Contains(combined, "insufficient_quota") || strings.Contains(combined, "billing") || strings.Contains(combined, "credit balance is too low") {
			return FaultNoCredits, false, false
		}
		return FaultRateLimited, true, false
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return FaultServiceUnavailable, true, false
	}
	if status >= 500 {
		return FaultServiceUnavailable, true, false
	}
	if strings.Contains(combined, "invalid_api_key") || strings.Contains(combined, "api key not valid") || strings.Contains(combined, "authentication_error") {
		return FaultWrongKey, false, true
	}
	if strings.Contains(combined, "insufficient_quota") || strings.Contains(combined, "credit balance is too low") {
		return FaultNoCredits, false, false
	}
	if strings.Contains(combined, "model_not_found") || strings.Contains(combined, "does not exist") && strings.Contains(combined, "model") {
		return FaultModelNotFound, false, false
	}
	return FaultBadRequest, false, false
}

const maxRetryAfterCap = 20 * time.Second

func parseRetryAfter(h http.Header) (time.Duration, bool) {
	if h == nil {
		return 0, false
	}
	if msRaw := strings.TrimSpace(h.Get("retry-after-ms")); msRaw != "" {
		if ms, err := strconv.ParseInt(msRaw, 10, 64); err == nil && ms >= 0 {
			d := time.Duration(ms) * time.Millisecond
			if d > maxRetryAfterCap {
				d = maxRetryAfterCap
			}
			return d, true
		}
	}
	if ra := strings.TrimSpace(h.Get("Retry-After")); ra != "" {
		if secs, err := strconv.ParseFloat(ra, 64); err == nil && secs >= 0 {
			d := time.Duration(secs * float64(time.Second))
			if d > maxRetryAfterCap {
				d = maxRetryAfterCap
			}
			return d, true
		}
		if t, err := http.ParseTime(ra); err == nil {
			d := time.Until(t)
			if d < 0 {
				d = 0
			}
			if d > maxRetryAfterCap {
				d = maxRetryAfterCap
			}
			return d, true
		}
	}
	return 0, false
}

func boundedBackoffDelay(attempt int, h http.Header) time.Duration {
	if d, ok := parseRetryAfter(h); ok {
		return d
	}
	base := 350 * time.Millisecond * time.Duration(1<<attempt)
	if base > 4*time.Second {
		base = 4 * time.Second
	}
	jitter := time.Duration(rand.Int63n(int64(base/4 + 1)))
	delay := base + jitter
	if delay > maxRetryAfterCap {
		delay = maxRetryAfterCap
	}
	return delay
}

func doWithRetry(ctx context.Context, cli *http.Client, makeReq func() (*http.Request, error), maxRetries int) (*http.Response, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		req, err := makeReq()
		if err != nil {
			return nil, err
		}
		resp, err := cli.Do(req)
		if err != nil {
			lastErr = err
			if attempt >= maxRetries || !isRetryableNetError(ctx, err) {
				return nil, err
			}
			delay := boundedBackoffDelay(attempt, nil)
			if err := sleepWithContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == 529 {
			if attempt < maxRetries {
				// Peek bounded body to avoid retrying fatal no-credit 429s.
				peek, _ := readBoundedBody(resp.Body)
				_ = resp.Body.Close()
				_, retryable, _ := classifyHTTPFault(resp.StatusCode, peek)
				if !retryable {
					// Return reconstructed response so caller surfaces exact vendor body.
					resp.Body = newReadCloserBytes(peek)
					return resp, nil
				}
				delay := boundedBackoffDelay(attempt, resp.Header)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
		}
		return resp, nil
	}
	return nil, lastErr
}

func isRetryableNetError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var wireErr *WireError
	if errors.As(err, &wireErr) {
		return wireErr.Retryable
	}
	// Do not retry explicit dial/airgap policy refusals.
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "air-gap") || strings.Contains(lower, "kubona_llm_cloud") || strings.Contains(lower, "redirect refused") {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || strings.Contains(lower, "eof") || strings.Contains(lower, "connection reset")
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

type readCloserBytes struct {
	data []byte
	pos  int
}

func newReadCloserBytes(b []byte) *readCloserBytes {
	return &readCloserBytes{data: b}
}

func (r *readCloserBytes) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

func (r *readCloserBytes) Close() error { return nil }

// newGuardedHTTPClient creates the HTTP client with dual-layer air-gap/production
// dial blocking and cross-origin credential-stripping redirect refusal.
func newGuardedHTTPClient(settings *Settings) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			blockNonLocal := false
			reason := ""
			if settings != nil {
				snap := settings.GetSnapshot()
				if snap.AirGapped {
					blockNonLocal = true
					reason = "Sovereign Air-Gap isolation blocks non-local dial"
				} else if productionCloudWithoutOptIn(false) {
					blockNonLocal = true
					reason = "production KUBONA_LLM_CLOUD=1 opt-in missing for non-local dial"
				}
			}
			if blockNonLocal {
				if addrIP, perr := netip.ParseAddr(strings.Trim(host, "[]")); perr == nil {
					if !IsLocalHost(addrIP.String()) {
						return nil, &WireError{Fault: FaultAirGapBlocked, Endpoint: addr, Message: reason}
					}
				} else if !IsLocalHost(host) {
					ips, rerr := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
					if rerr != nil {
						return nil, &WireError{Fault: FaultAirGapBlocked, Endpoint: addr, Message: reason}
					}
					for _, ip := range ips {
						if !IsLocalHost(ip.String()) {
							return nil, &WireError{Fault: FaultAirGapBlocked, Endpoint: addr, Message: fmt.Sprintf("%s (%s resolved to non-local %s)", reason, host, ip)}
						}
					}
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{
		Timeout:   600 * time.Second,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if len(via) > 0 && !sameOriginURL(via[0].URL, req.URL) {
				return fmt.Errorf("cross-origin redirect refused to protect credentials")
			}
			return nil
		},
	}
}

func sameOriginURL(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

type wireRequest struct {
	Provider        ProviderType
	ProvConfig      ProviderConfig
	Wire            WireType
	Model           string
	Messages        []ChatMessage
	Tools           []map[string]any
	Temperature     *float64
	TopP            *float64
	Seed            *int64
	MaxTokens       int
	ReasoningEffort string
	NumCtx          int
	PromptCacheKey  string
	ResponseFormat  map[string]any
}

type wireResponse struct {
	Content            string
	ReasoningContent   string
	ReasoningSignature string
	EncryptedReasoning string
	Parts              []MessagePart
	ToolCalls          []ToolCall
	Model              string
	FinishReason       string
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	CachedInputTokens  int
	CacheWriteTokens   int
	ReasoningTokens    int
	PromptTokPerSec    float64
	GenTokPerSec       float64
	TotalTimeSec       float64
	UsageReported      bool
	RawUsage           map[string]any
}

func (wr *wireResponse) toChatCompletionResponse() *ChatCompletionResponse {
	if wr == nil {
		return nil
	}
	resp := &ChatCompletionResponse{
		Model: wr.Model,
		Choices: []struct {
			Index   int         `json:"index"`
			Message ChatMessage `json:"message"`
			Finish  string      `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: ChatMessage{
					Role:               "assistant",
					Content:            wr.Content,
					ToolCalls:          wr.ToolCalls,
					ReasoningContent:   wr.ReasoningContent,
					ReasoningSignature: wr.ReasoningSignature,
					EncryptedReasoning: wr.EncryptedReasoning,
					Parts:              wr.Parts,
				},
				Finish: wr.FinishReason,
			},
		},
	}
	resp.Usage.PromptTokens = wr.PromptTokens
	resp.Usage.CompletionTokens = wr.CompletionTokens
	resp.Usage.TotalTokens = wr.TotalTokens
	resp.Usage.CachedInputTokens = wr.CachedInputTokens
	resp.Usage.CacheWriteTokens = wr.CacheWriteTokens
	resp.Usage.ReasoningTokens = wr.ReasoningTokens
	resp.Usage.PromptTokPerSec = wr.PromptTokPerSec
	resp.Usage.GenTokPerSec = wr.GenTokPerSec
	resp.Usage.TotalTimeSec = wr.TotalTimeSec
	return resp
}

func effectiveParts(m ChatMessage) []MessagePart {
	if len(m.Parts) > 0 {
		return m.Parts
	}
	var out []MessagePart
	if m.Role == "tool" {
		out = append(out, MessagePart{
			Type:       "tool_result",
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
			Text:       m.Content,
		})
		return out
	}
	if m.ReasoningContent != "" || m.ReasoningSignature != "" || m.EncryptedReasoning != "" {
		out = append(out, MessagePart{
			Type:             "thinking",
			Text:             m.ReasoningContent,
			Signature:        m.ReasoningSignature,
			EncryptedContent: m.EncryptedReasoning,
		})
	}
	if m.Content != "" {
		out = append(out, MessagePart{Type: "text", Text: m.Content})
	}
	for _, tc := range m.ToolCalls {
		out = append(out, MessagePart{
			Type:             "tool_call",
			ToolCallID:       tc.ID,
			ToolName:         tc.Name,
			ToolArgs:         tc.Arguments,
			ThoughtSignature: tc.ThoughtSignature,
		})
	}
	return out
}

func extractSystemAndNonSystem(messages []ChatMessage) (systemParts []string, rest []ChatMessage) {
	for _, m := range messages {
		if strings.EqualFold(m.Role, "system") {
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
			continue
		}
		rest = append(rest, m)
	}
	return systemParts, rest
}

func normalizeToolDef(t map[string]any) (name, description string, schema map[string]any, ok bool) {
	if fn, hasFn := t["function"].(map[string]any); hasFn {
		name, _ = fn["name"].(string)
		description, _ = fn["description"].(string)
		schema, _ = fn["parameters"].(map[string]any)
	} else {
		name, _ = t["name"].(string)
		description, _ = t["description"].(string)
		schema, _ = t["parameters"].(map[string]any)
	}
	if strings.TrimSpace(name) == "" {
		return "", "", nil, false
	}
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return name, description, schema, true
}

func buildUsageMap(wr *wireResponse) map[string]any {
	if wr == nil {
		return nil
	}
	if wr.RawUsage != nil {
		out := make(map[string]any, len(wr.RawUsage)+3)
		for k, v := range wr.RawUsage {
			out[k] = v
		}
		if wr.PromptTokens > 0 || wr.CompletionTokens > 0 {
			out["prompt_tokens"] = wr.PromptTokens
			out["completion_tokens"] = wr.CompletionTokens
			if wr.TotalTokens > 0 {
				out["total_tokens"] = wr.TotalTokens
			}
		}
		if wr.CachedInputTokens > 0 {
			out["cached_input_tokens"] = wr.CachedInputTokens
		}
		if wr.CacheWriteTokens > 0 {
			out["cache_write_tokens"] = wr.CacheWriteTokens
		}
		if wr.ReasoningTokens > 0 {
			out["reasoning_tokens"] = wr.ReasoningTokens
		}
		return out
	}
	if !wr.UsageReported && wr.PromptTokens == 0 && wr.CompletionTokens == 0 && wr.TotalTokens == 0 {
		return nil
	}
	u := map[string]any{
		"prompt_tokens":     wr.PromptTokens,
		"completion_tokens": wr.CompletionTokens,
		"total_tokens":      wr.TotalTokens,
	}
	if wr.CachedInputTokens > 0 {
		u["cached_input_tokens"] = wr.CachedInputTokens
	}
	if wr.CacheWriteTokens > 0 {
		u["cache_write_tokens"] = wr.CacheWriteTokens
	}
	if wr.ReasoningTokens > 0 {
		u["reasoning_tokens"] = wr.ReasoningTokens
	}
	return u
}
