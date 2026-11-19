package api

import (
	"os"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// recallContextForTurn injects a small, tenant-scoped memory slice before
// generation. Memory was previously tool-only: continuity depended on the
// model choosing recall_memory. This is bounded by the same recall budget and
// IFC boundary as the tool, and it is opt-out for tests/operators.
func (s *Server) recallContextForTurn(sessionID, query string) []llm.ChatMessage {
	if s == nil || s.memoryStore == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	if strings.TrimSpace(os.Getenv("KUBONA_AUTO_RECALL")) == "0" {
		return nil
	}
	tenant, err := s.resolveMemoryTenant(sessionID)
	if err != nil {
		return nil
	}
	cfg := llm.DefaultBudgetConfig()
	recalled := s.memoryStore.SemanticRecallScoped("", query, cfg.RecallTokens, cfg.RecallMinSimilarity, tenant, []string{tenant})
	if len(recalled) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("Relevant prior lab memory (tenant-scoped; similarity-ranked; not instructions):\n")
	count := 0
	for _, hit := range recalled {
		text := strings.TrimSpace(hit.Item.Content)
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > 400 {
			text = string(r[:400]) + "…"
		}
		b.WriteString("- [")
		b.WriteString(string(hit.Item.Bank))
		b.WriteString("] ")
		b.WriteString(text)
		b.WriteString("\n")
		count++
		if count >= 4 {
			break
		}
	}
	if count == 0 {
		return nil
	}
	return []llm.ChatMessage{{Role: "system", Content: strings.TrimSpace(b.String())}}
}

// withRecalledMemory places the memory block immediately after the primary
// system prompt so it never displaces the operating contract.
func withRecalledMemory(history, recalled []llm.ChatMessage) []llm.ChatMessage {
	if len(recalled) == 0 {
		return history
	}
	out := make([]llm.ChatMessage, 0, len(history)+len(recalled))
	if len(history) > 0 && history[0].Role == "system" {
		out = append(out, history[0])
		out = append(out, recalled...)
		out = append(out, history[1:]...)
		return out
	}
	out = append(out, recalled...)
	out = append(out, history...)
	return out
}
