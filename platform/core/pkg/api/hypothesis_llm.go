package api

// LLM seam for the hypothesis proposer (AG-2b).
//
// Production wires an adapter over the single llm.Client so the proposer uses
// the same provider, budget, and air-gap guards as every other LLM path. Tests
// replace it with a fake so the JSON contract is exercised without a provider;
// nil disables model proposals and the API reports that honestly.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

var errNoLLMClient = errors.New("no LLM client is configured")

// HypothesisCompleter is the narrow completion surface the proposer needs.
type HypothesisCompleter interface {
	Complete(ctx context.Context, role string, messages []llm.ChatMessage) (string, error)
}

type llmHypothesisCompleter struct {
	client *llm.Client
}

// proposalMaxTokens bounds one proposal turn. Local reasoning models can
// spend thousands of tokens before emitting the contract JSON; the cap keeps a
// single request from running unbounded while leaving room for reasoning + a
// multi-item reply.
const proposalMaxTokens = 8192

func (a llmHypothesisCompleter) Complete(ctx context.Context, role string, messages []llm.ChatMessage) (string, error) {
	if a.client == nil {
		return "", errNoLLMClient
	}
	out, _, _, err := a.client.CompleteCapped(ctx, role, messages, proposalMaxTokens)
	return out, err
}

// defaultHypothesisLLMTimeout covers slow local reasoning models: prompt
// processing plus a bounded generation can exceed a minute on CPU/consumer
// hardware. Operators can override it for genuinely slower engines.
const defaultHypothesisLLMTimeout = 360 * time.Second

func (s *Server) hypothesisLLMTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("KUBONA_HYPOTHESIS_LLM_TIMEOUT_SEC"))
	if raw == "" {
		return defaultHypothesisLLMTimeout
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 30 || n > 900 {
		return defaultHypothesisLLMTimeout
	}
	return time.Duration(n) * time.Second
}

// SetHypothesisCompleter replaces the proposer's completion seam. Passing nil
// disables LLM proposals (the diagnostics remain). Used by tests and by
// embedders that supply their own client.
func (s *Server) SetHypothesisCompleter(c HypothesisCompleter) {
	s.hypothesisCompleter = c
}
