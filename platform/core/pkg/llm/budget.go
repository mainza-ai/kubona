package llm

// BudgetConfig is the single source for every context/money-adjacent bound
// in the agent loop. Previously these lived as scattered literals (40 msgs
// here, 250 recall tokens there, an uncounted tool tail everywhere), which
// meant no two paths agreed and the tool loop escaped budgeting entirely.
// Defaults preserve long-standing behavior exactly; tightening them is one
// edit here, with tests pinning every value.
type BudgetConfig struct {
	// HistoryMessages/HistoryChars bound the base transcript per call
	// (BudgetHistory: system survives, newest wins, cuts announced).
	HistoryMessages int
	HistoryChars    int
	// RecallTokens/RecallMinSimilarity bound associative-memory injection
	// (recall_memory tool, tribunal recall, HTTP recall).
	RecallTokens        int
	RecallMinSimilarity float64
	// MaxObservationChars caps a single tool observation on the wire. Tool
	// outputs embed whole payloads (a campaign dump is kilobytes); without
	// this, one chatty tool eats the turn. Truncation is marked, never
	// silent (see TruncateObservation).
	MaxObservationChars int
	// MaxToolCharsPerTurn caps total tool-added text per turn across all
	// rounds. The loop stops (with a ledger event) instead of growing the
	// prompt without bound after budgeting already ran.
	MaxToolCharsPerTurn int
	// MaxToolRounds bounds model→tool→observation cycles per turn.
	MaxToolRounds int
	// MaxTurnTokens accumulates provider-reported usage across a turn's
	// tool rounds. A heuristic guardrail (provider accounting varies),
	// not a bill: normal turns land far below it, pathological loops hit
	// it instead of burning minutes.
	MaxTurnTokens int
}

// DefaultRecallTokens mirrors the long-standing recall injection budget.
const DefaultRecallTokens = 250

// DefaultRecallMinSimilarity mirrors the long-standing recall floor.
const DefaultRecallMinSimilarity = 0.35

// DefaultBudgetConfig returns the production budgets. Every value is
// pinned by tests; change deliberately, with the suite green.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		HistoryMessages:     DefaultMaxHistoryMessages,
		HistoryChars:        DefaultMaxHistoryChars,
		RecallTokens:        DefaultRecallTokens,
		RecallMinSimilarity: DefaultRecallMinSimilarity,
		MaxObservationChars: 6000,
		MaxToolCharsPerTurn: 24000,
		MaxToolRounds:       3,
		MaxTurnTokens:       150000,
	}
}

// TruncateObservation bounds one observation, marking the cut so the model
// sees a budgeted value instead of a mysteriously short payload.
func TruncateObservation(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars]) + "…[observation truncated to budget]"
}
