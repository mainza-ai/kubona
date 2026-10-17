package llm

// History budgeting: every LLM call carries a bounded transcript. Unbounded
// GetMessages growth otherwise becomes unbounded spend and latency, and
// drowns the live turn in stale context. System messages always survive;
// the newest turns win; truncation is announced, never silent.
const (
	// DefaultMaxHistoryMessages caps user/assistant/tool turns per call.
	DefaultMaxHistoryMessages = 40
	// DefaultMaxHistoryChars caps total turn characters (~4 chars/token).
	DefaultMaxHistoryChars = 48000
)

// BudgetHistory returns msgs with system rows preserved and the most recent
// turns kept within caps. When turns are dropped, a boundary marker records
// how many — the model sees the cut instead of a mysteriously short past.
func BudgetHistory(msgs []ChatMessage, maxMsgs, maxChars int) []ChatMessage {
	if maxMsgs <= 0 {
		maxMsgs = DefaultMaxHistoryMessages
	}
	if maxChars <= 0 {
		maxChars = DefaultMaxHistoryChars
	}
	var system []ChatMessage
	var turns []ChatMessage
	for _, m := range msgs {
		if m.Role == "system" {
			system = append(system, m)
		} else {
			turns = append(turns, m)
		}
	}
	dropped := 0
	for len(turns) > maxMsgs || historyChars(turns) > maxChars {
		turns = turns[1:]
		dropped++
	}
	out := append([]ChatMessage(nil), system...)
	if dropped > 0 {
		out = append(out, ChatMessage{
			Role:    "system",
			Content: "Context budget: the " + itoa(dropped) + " oldest turns were omitted; what follows is the most recent history.",
		})
	}
	return append(out, turns...)
}

func historyChars(msgs []ChatMessage) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, c := range m.ToolCalls {
			total += len(c.Arguments) + len(c.Name)
		}
	}
	return total
}

// HistoryPressure reports what BudgetHistory would do to msgs WITHOUT
// mutating: turn count, char count, and how many oldest turns would be
// dropped under the caps. S8a serves this as the composer's context
// readout — measured transcript against the real truncation rule, never a
// vendor window we don't own.
func HistoryPressure(msgs []ChatMessage, maxMsgs, maxChars int) (turns, chars, dropped int) {
	if maxMsgs <= 0 {
		maxMsgs = DefaultMaxHistoryMessages
	}
	if maxChars <= 0 {
		maxChars = DefaultMaxHistoryChars
	}
	var live []ChatMessage
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		live = append(live, m)
	}
	chars = historyChars(live)
	for len(live)-dropped > maxMsgs || historyChars(live[dropped:]) > maxChars {
		dropped++
	}
	return len(live), chars, dropped
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
