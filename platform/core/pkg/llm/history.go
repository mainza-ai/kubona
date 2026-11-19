package llm

import "strings"

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
	var droppedMsgs []ChatMessage
	for len(turns) > maxMsgs || historyChars(turns) > maxChars {
		droppedMsgs = append(droppedMsgs, turns[0])
		turns = turns[1:]
	}
	// Preserve assistant ToolCalls <-> tool result pairing: never leave orphaned
	// leading "tool" messages whose preceding assistant tool_calls turn was cut.
	for len(turns) > 0 && turns[0].Role == "tool" {
		droppedMsgs = append(droppedMsgs, turns[0])
		turns = turns[1:]
	}
	out := append([]ChatMessage(nil), system...)
	if len(droppedMsgs) > 0 {
		out = append(out, ChatMessage{
			Role:    "system",
			Content: compactionSummary(droppedMsgs),
		})
	}
	return append(out, turns...)
}

// compactionSummary is a deterministic extractive summary of the turns the
// budget drops. It is not an LLM summary: it preserves the user's stated asks
// and the tools that ran, and points at the full transcript (retained in the
// ledger), so compaction is reversible. The old marker said only "N turns
// omitted", which lost the task intent that makes a long session coherent.
func compactionSummary(dropped []ChatMessage) string {
	const maxAsks = 4
	const askRunes = 160
	var asks []string
	seenTools := map[string]struct{}{}
	var tools []string
	for _, m := range dropped {
		if m.Role == "user" {
			text := strings.TrimSpace(m.Content)
			if text != "" {
				r := []rune(text)
				if len(r) > askRunes {
					text = string(r[:askRunes]) + "…"
				}
				asks = append(asks, text)
				if len(asks) > maxAsks {
					asks = asks[len(asks)-maxAsks:]
				}
			}
		}
		for _, tc := range m.ToolCalls {
			if tc.Name == "" {
				continue
			}
			if _, ok := seenTools[tc.Name]; !ok {
				seenTools[tc.Name] = struct{}{}
				tools = append(tools, tc.Name)
			}
		}
	}
	var b strings.Builder
	b.WriteString("Context compaction: the ")
	b.WriteString(itoa(len(dropped)))
	b.WriteString(" oldest turns were omitted")
	if len(asks) > 0 {
		b.WriteString("; earlier user asks: ")
		for i, ask := range asks {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(ask)
		}
	}
	if len(tools) > 0 {
		b.WriteString("; tools used: ")
		b.WriteString(strings.Join(tools, ", "))
	}
	b.WriteString("; the full transcript remains in the session ledger and can be re-read.")
	out := b.String()
	if r := []rune(out); len(r) > 1600 {
		out = string(r[:1600]) + "…"
	}
	return out
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
