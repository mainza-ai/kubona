package llm_test

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// Every budget value is pinned: tightening or loosening is a deliberate,
// reviewed edit, not drift.
func TestDefaultBudgetConfig(t *testing.T) {
	cfg := llm.DefaultBudgetConfig()
	if cfg.HistoryMessages != 40 || cfg.HistoryChars != 48000 {
		t.Errorf("history bounds must preserve long-standing behavior: %+v", cfg)
	}
	if cfg.RecallTokens != 250 || cfg.RecallMinSimilarity != 0.35 {
		t.Errorf("recall bounds must match the tool contract: %+v", cfg)
	}
	if cfg.MaxObservationChars <= 0 || cfg.MaxToolCharsPerTurn <= cfg.MaxObservationChars {
		t.Errorf("observation/turn caps must nest sanely: %+v", cfg)
	}
	if cfg.MaxToolRounds != 3 || cfg.MaxTurnTokens <= 0 {
		t.Errorf("loop guards must be positive: %+v", cfg)
	}
}

func TestTruncateObservation(t *testing.T) {
	if got := llm.TruncateObservation("short", 6000); got != "short" {
		t.Errorf("short values pass through, got %q", got)
	}
	long := strings.Repeat("x", 7000)
	got := llm.TruncateObservation(long, 6000)
	if len([]rune(got)) <= 6000 || !strings.HasSuffix(got, "[observation truncated to budget]") {
		t.Errorf("cut must be marked, got len %d suffix %q", len([]rune(got)), got[len(got)-20:])
	}
	if got := llm.TruncateObservation(long, 0); got != long {
		t.Error("non-positive cap disables truncation")
	}
}
