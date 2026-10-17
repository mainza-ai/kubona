package llm_test

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestBudgetHistoryKeepsSystemAndNewest(t *testing.T) {
	var msgs []llm.ChatMessage
	msgs = append(msgs, llm.ChatMessage{Role: "system", Content: "sys"})
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.ChatMessage{Role: "user", Content: "old"})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: "newest"})
	out := llm.BudgetHistory(msgs, 3, 100000)
	if out[0].Content != "sys" {
		t.Error("system message must survive budgeting")
	}
	joined := ""
	for _, m := range out {
		joined += m.Content + "|"
	}
	if !strings.Contains(joined, "newest") {
		t.Error("newest turn must survive budgeting")
	}
	if !strings.Contains(joined, "omitted") {
		t.Error("truncation must be announced, never silent")
	}
}

func TestBudgetHistoryPassthrough(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "s"},
		{Role: "user", Content: "hi"},
	}
	out := llm.BudgetHistory(msgs, 40, 48000)
	if len(out) != 2 {
		t.Errorf("small history must pass through untouched, got %d", len(out))
	}
}

func TestBudgetHistoryCharCap(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "user", Content: strings.Repeat("x", 1000)},
		{Role: "user", Content: "short"},
	}
	out := llm.BudgetHistory(msgs, 40, 100)
	joined := ""
	for _, m := range out {
		joined += m.Content + "|"
	}
	if !strings.Contains(joined, "short") {
		t.Error("short recent turn must survive the char cap")
	}
	if strings.Contains(joined, strings.Repeat("x", 1000)) {
		t.Error("oversized old turn must be cut by the char cap")
	}
}

func TestHistoryPressureMirrorsBudget(t *testing.T) {
	mk := func(n int, size int) []llm.ChatMessage {
		var out []llm.ChatMessage
		for i := 0; i < n; i++ {
			out = append(out, llm.ChatMessage{Role: "user", Content: string(make([]byte, size))})
		}
		return out
	}
	turns, chars, dropped := llm.HistoryPressure(mk(3, 100), 40, 48000)
	if turns != 3 || chars != 300 || dropped != 0 {
		t.Fatalf("small transcript: got %d %d %d", turns, chars, dropped)
	}
	_, _, dropped = llm.HistoryPressure(mk(45, 10), 40, 48000)
	if dropped != 5 {
		t.Fatalf("45 turns over cap 40 must drop 5, got %d", dropped)
	}
	_, _, dropped = llm.HistoryPressure(mk(10, 10000), 40, 48000)
	if dropped == 0 {
		t.Fatalf("100KB over 48K cap must drop")
	}
	withSys := append([]llm.ChatMessage{{Role: "system", Content: "sys"}}, mk(2, 50)...)
	turns, _, _ = llm.HistoryPressure(withSys, 40, 48000)
	if turns != 2 {
		t.Fatalf("system excluded, got %d turns", turns)
	}
	msgs := mk(45, 10)
	budgeted := llm.BudgetHistory(msgs, 40, 48000)
	turns, _, dropped = llm.HistoryPressure(msgs, 40, 48000)
	// BudgetHistory appends a boundary marker when it drops (system row),
	// so survivors + marker == budgeted length.
	if turns-dropped+1 != len(budgeted) {
		t.Fatalf("pressure survivors %d + marker must equal budgeted %d", turns-dropped, len(budgeted))
	}
}
