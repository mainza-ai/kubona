package llm

// B3 — reserve before the wire request. The guard runs inside the client so
// every call path (copilot tool loops, unary, streams, tribunal, agents) is
// covered by the same rule: no metered call starts without a bound that the
// remaining budget can cover. The guard returns an output-token ceiling and a
// release callback; the client applies the ceiling and releases after the
// call has been accounted for.

import "fmt"

type SpendGuard interface {
	// Reserve bounds one call. A ceiling <= 0 means "leave the planned
	// max_tokens unchanged". A non-nil error refuses the call.
	Reserve(provider ProviderType, model string, messages []ChatMessage, plannedMaxTokens int) (ceiling int, release func(), err error)
}

// SetSpendGuard installs the process budget guard. Set once at construction
// (before serving); nil disables reservation (track-only mode).
func (c *Client) SetSpendGuard(guard SpendGuard) {
	c.spendGuard = guard
}

func (c *Client) reserveCall(provider ProviderType, model string, messages []ChatMessage, planned int) (int, func(), error) {
	if c.spendGuard == nil {
		return 0, nil, nil
	}
	ceiling, release, err := c.spendGuard.Reserve(provider, model, messages, planned)
	if err != nil {
		return 0, nil, fmt.Errorf("cloud LLM budget refused this call: %w", err)
	}
	return ceiling, release, nil
}
