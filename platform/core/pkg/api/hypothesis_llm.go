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

func (a llmHypothesisCompleter) Complete(ctx context.Context, role string, messages []llm.ChatMessage) (string, error) {
	if a.client == nil {
		return "", errNoLLMClient
	}
	out, _, _, err := a.client.Complete(ctx, role, messages)
	return out, err
}

// SetHypothesisCompleter replaces the proposer's completion seam. Passing nil
// disables LLM proposals (the diagnostics remain). Used by tests and by
// embedders that supply their own client.
func (s *Server) SetHypothesisCompleter(c HypothesisCompleter) {
	s.hypothesisCompleter = c
}
