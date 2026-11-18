package api

import (
	"testing"
	"time"
)

func TestHypothesisLLMTimeout(t *testing.T) {
	s := &Server{}
	t.Setenv("KUBONA_HYPOTHESIS_LLM_TIMEOUT_SEC", "")
	if got := s.hypothesisLLMTimeout(); got != defaultHypothesisLLMTimeout {
		t.Fatalf("default timeout = %s", got)
	}
	t.Setenv("KUBONA_HYPOTHESIS_LLM_TIMEOUT_SEC", "240")
	if got := s.hypothesisLLMTimeout(); got != 240*time.Second {
		t.Fatalf("override = %s", got)
	}
	for _, bad := range []string{"nope", "10", "10000", "-1"} {
		t.Setenv("KUBONA_HYPOTHESIS_LLM_TIMEOUT_SEC", bad)
		if got := s.hypothesisLLMTimeout(); got != defaultHypothesisLLMTimeout {
			t.Fatalf("bad %q accepted: %s", bad, got)
		}
	}
}
