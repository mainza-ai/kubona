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
