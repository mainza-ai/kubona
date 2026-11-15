package llm

// B13 — capabilities come from declared data, not name substrings. Vision and
// reasoning are populated from the reviewed rate card and provider metadata
// (OpenRouter architecture/supported_parameters, Ollama /api/show). Unknown
// models claim nothing.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// reasoningEffortProviders are the wires that accept a reasoning_effort-like
// control. Anthropic maps it to a thinking budget; the OpenAI-compatible
// families accept low|medium|high.
var reasoningEffortProviders = map[ProviderType]bool{
	ProviderOpenAI:     true,
	ProviderOpenRouter: true,
	ProviderXAI:        true,
	ProviderGroq:       true,
	ProviderAnthropic:  true,
}

var reasoningEffortRank = map[string]int{"low": 0, "medium": 1, "high": 2}

// reasoningEffortsFor exposes the levels a provider accepts when a model is
// declared reasoning-capable.
func reasoningEffortsFor(pt ProviderType) []string {
	if !reasoningEffortProviders[pt] {
		return nil
	}
	return []string{"low", "medium", "high"}
}

// normalizeReasoningEffort maps a requested level to one the model actually
// accepts, and returns "" when no level may be sent:
//   - unknown models claim no reasoning capability (no guess);
//   - a model whose rate-card capabilities exclude reasoning gets nothing;
//   - "none" is never sent (we do not advertise it as supported);
//   - aliases map to the nearest level (minimal→low, xhigh→high).
func normalizeReasoningEffort(pt ProviderType, model, effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "none" {
		return ""
	}
	if !reasoningEffortProviders[pt] {
		return ""
	}
	rate, ok := rates.LookupModelRate(string(pt), model)
	if !ok {
		return ""
	}
	reasoning := false
	for _, cap := range rate.Capabilities {
		if strings.EqualFold(cap, "reasoning") {
			reasoning = true
			break
		}
	}
	if !reasoning {
		return ""
	}
	switch effort {
	case "minimal":
		effort = "low"
	case "xhigh", "highest", "max":
		effort = "high"
	}
	if _, ok := reasoningEffortRank[effort]; ok {
		return effort
	}
	// Unknown level: nearest supported level, never a value outside the set.
	nearest := "low"
	if strings.Contains(effort, "high") || strings.Contains(effort, "max") {
		nearest = "high"
	} else if strings.Contains(effort, "med") {
		nearest = "medium"
	}
	return nearest
}

// mergeCapabilities unions declared capabilities; it never clears one.
func mergeCapabilities(dst *ModelCapabilities, src ModelCapabilities) {
	if dst == nil {
		return
	}
	if src.Vision {
		dst.Vision = true
	}
	if src.Tools {
		dst.Tools = true
	}
	if src.Reasoning {
		dst.Reasoning = true
	}
	if src.StructuredOutput {
		dst.StructuredOutput = true
	}
	if src.Streaming {
		dst.Streaming = true
	}
}

// ollamaShowCapabilities asks the local daemon what one model supports. A
// failed/slow lookup returns nil: unknown capabilities stay unknown, never
// guessed from the model name.
func (c *Client) ollamaShowCapabilities(ctx context.Context, provConfig ProviderConfig, model string) *ModelCapabilities {
	endpoint := ollamaRootURL(provConfig.BaseURL) + "/api/show"
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	payload, err := json.Marshal(map[string]string{"name": model})
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	raw, err := readBoundedBody(resp.Body)
	if err != nil {
		return nil
	}
	var out struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out.Capabilities) == 0 {
		return nil
	}
	caps := &ModelCapabilities{}
	for _, capability := range out.Capabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "vision":
			caps.Vision = true
		case "tools":
			caps.Tools = true
		case "thinking", "reasoning":
			caps.Reasoning = true
		case "completion", "chat":
			caps.Streaming = true
		}
	}
	return caps
}
