package api

// B3 pins: a metered call gets an affordable ceiling, concurrent calls cannot
// reserve past the cap, and blind/unpriced spend refuses.

import (
	"math"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func guardMessages() []llm.ChatMessage {
	return []llm.ChatMessage{{Role: "user", Content: "measure the H2 ground state"}}
}

func TestLLMBudgetGuardBoundsAndReleases(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/guard1.db")
	defer cleanup()
	t.Setenv("KUBONA_LLM_BUDGET_USD", "0.001")
	guard := &llmBudgetGuard{s: srv}

	ceiling, release, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 1024)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if ceiling <= 0 || ceiling > 1024 {
		t.Fatalf("ceiling must be a positive bound, got %d", ceiling)
	}
	if release == nil {
		t.Fatal("a reservation must return a release")
	}
	// While the reservation is held, the same cap cannot fund another call.
	if _, _, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 1024); err == nil {
		t.Fatal("concurrent reservation past the cap must refuse")
	}
	release()
	// Released capacity is usable again; double-release is safe.
	release()
	if _, release2, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 512); err != nil {
		t.Fatalf("released capacity must reserve: %v", err)
	} else if release2 != nil {
		release2()
	}
}

func TestLLMBudgetGuardRefusesUnboundedInputs(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/guard2.db")
	defer cleanup()
	t.Setenv("KUBONA_LLM_BUDGET_USD", "0.01")
	guard := &llmBudgetGuard{s: srv}

	// Unknown model: cannot be priced, so it cannot be bounded.
	if _, _, err := guard.Reserve(llm.ProviderOpenCode, "mystery-model-2099", guardMessages(), 512); err == nil {
		t.Fatal("unpriced metered model must refuse")
	}
	// Local providers are free by construction: no reservation.
	if ceiling, release, err := guard.Reserve(llm.ProviderOllama, "whatever", guardMessages(), 0); err != nil || ceiling != 0 || release != nil {
		t.Fatalf("local provider must not reserve: ceiling=%d release=%v err=%v", ceiling, release != nil, err)
	}
	// Blind spend trips the capped gate.
	if err := srv.Ledger().AppendEvent("global", "llm_spend_estimated", map[string]any{"unmetered": true, "provider": "opencode"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 512); err == nil {
		t.Fatal("unmetered spend must refuse new capped calls")
	}
}

func TestLLMBudgetGuardExhaustedAndCeilingMath(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/guard3.db")
	defer cleanup()
	t.Setenv("KUBONA_LLM_BUDGET_USD", "0.0005")
	guard := &llmBudgetGuard{s: srv}

	// $0.0005 at $0.0012/1k output ≈ 416 tokens; prompt cost leaves less.
	ceiling, release, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 0)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if ceiling < llmMinCallCeiling || ceiling > 416 {
		t.Fatalf("ceiling must fit the remaining dollars, got %d", ceiling)
	}
	release()

	// Recorded spend at/above the cap refuses before any call.
	if err := srv.Ledger().AppendEvent("global", "llm_spend_estimated", map[string]any{"estimated_usd": 0.001, "provider": "opencode"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := guard.Reserve(llm.ProviderOpenCode, "deepseek-v4.1-flash", guardMessages(), 512); err == nil {
		t.Fatal("exhausted budget must refuse")
	}

	// The guard never reports a negative reservation.
	guard.mu.Lock()
	reserved := guard.reservedUSD
	guard.mu.Unlock()
	if reserved < 0 || math.Abs(reserved) > 1e-9 {
		t.Fatalf("reservations must be released to zero, got %f", reserved)
	}
}
