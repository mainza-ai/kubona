package api

// B3 — reserve cloud LLM budget before each call. The old gate ran once per
// chat entry; a tool loop could make several metered calls unchecked. This
// guard is installed on the LLM client, so every call path (copilot tool loop,
// unary, streams, tribunal, agents) is bounded by the same rule:
//
//   remaining = cap - measured spend - in-flight reservations
//
// A call gets an output-token ceiling it can afford; the reservation is
// released once the measured spend has been recorded. Unpriced metered models,
// unreadable ledgers, and blind (unmetered) spend all refuse.

import (
	"fmt"
	"math"
	"sync"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

const (
	llmDefaultCallCeiling = 4096
	llmMaxCallCeiling     = 8192
	llmMinCallCeiling     = 64
)

type llmBudgetGuard struct {
	s  *Server
	mu sync.Mutex
	// reservedUSD is the sum of in-flight worst-case bounds.
	reservedUSD float64
}

func estimatePromptTokens(messages []llm.ChatMessage) int {
	runes := 0
	for _, message := range messages {
		runes += len([]rune(message.Content)) + 8
	}
	// Headroom for tool schemas, role framing, and tokenizer variance.
	return runes/4 + 256
}

// Reserve bounds one call. Output-token ceilings are derived from the model's
// output rate and the remaining budget after the worst-case prompt cost.
func (g *llmBudgetGuard) Reserve(provider llm.ProviderType, model string, messages []llm.ChatMessage, plannedMaxTokens int) (int, func(), error) {
	capUSD := llmBudgetCapUSD()
	if capUSD <= 0 {
		return 0, nil, nil // track-only
	}
	snapshot := g.s.llmSettings.GetSnapshot()
	if snapshot.IsProviderLocal(provider) {
		return 0, nil, nil // free by construction
	}
	spent, _, unmetered, err := g.s.llmSpendSummary()
	if err != nil {
		return 0, nil, fmt.Errorf("cloud LLM budget cannot be verified (ledger read failed): %w", err)
	}
	if unmetered > 0 {
		return 0, nil, fmt.Errorf("blind cloud spend detected (%d unmetered record(s)): refusing until usage reporting or a token ceiling is configured", unmetered)
	}
	inputRate, ok := llm.EstimateCostUSDForSettings(&snapshot, provider, model, 1, 0, 0, 0)
	if !ok {
		return 0, nil, fmt.Errorf("model %s/%s is unpriced: refusing a capped call it cannot bound", provider, model)
	}
	outputRate, ok := llm.EstimateCostUSDForSettings(&snapshot, provider, model, 0, 1, 0, 0)
	if !ok || outputRate <= 0 {
		return 0, nil, fmt.Errorf("model %s/%s has no output rate: refusing a capped call it cannot bound", provider, model)
	}
	inputCost := inputRate * float64(estimatePromptTokens(messages))

	g.mu.Lock()
	defer g.mu.Unlock()
	remaining := capUSD - spent - g.reservedUSD
	if remaining <= 0 {
		return 0, nil, fmt.Errorf("cloud LLM budget exhausted: spent ~$%.4f of $%.4f cap (KUBONA_LLM_BUDGET_USD)", spent, capUSD)
	}
	affordable := int(math.Floor((remaining - inputCost) / outputRate))
	if affordable < llmMinCallCeiling {
		return 0, nil, fmt.Errorf("remaining cloud LLM budget $%.4f cannot cover one call (~$%.4f prompt + 64 output tokens): raise the cap or switch to a local provider", remaining, inputCost)
	}
	ceiling := plannedMaxTokens
	if ceiling <= 0 {
		ceiling = llmDefaultCallCeiling
	}
	if affordable < ceiling {
		ceiling = affordable
	}
	if ceiling > llmMaxCallCeiling {
		ceiling = llmMaxCallCeiling
	}
	reservation := inputCost + outputRate*float64(ceiling)
	g.reservedUSD += reservation
	released := false
	release := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if released {
			return
		}
		released = true
		g.reservedUSD -= reservation
		if g.reservedUSD < 0 {
			g.reservedUSD = 0
		}
	}
	return ceiling, release, nil
}
