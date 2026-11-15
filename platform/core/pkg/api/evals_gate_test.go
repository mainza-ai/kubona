package api

import (
	"errors"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/evals"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func gateServer(t *testing.T) (*Server, *evals.Set, evals.Store) {
	t.Helper()
	set, err := evals.Load("copilot")
	if err != nil {
		t.Fatal(err)
	}
	settings := llm.DefaultSettings()
	settings.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role: "copilot", Provider: llm.ProviderType("strata"), Model: "iq3_s",
	}
	store := evals.Store{Dir: t.TempDir()}
	return &Server{llmSettings: settings, evalStore: store}, set, store
}

func copilotChange(provider, model string) *llm.Settings {
	settings := llm.DefaultSettings()
	settings.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role: "copilot", Provider: llm.ProviderType(provider), Model: model,
	}
	return settings
}

func TestEvalGateRefusesUnevaluatedModelWhenRequired(t *testing.T) {
	t.Setenv("KUBONA_REQUIRE_LLM_EVAL", "1")
	s, _, _ := gateServer(t)
	_, err := s.enforceLLMEvalGate(copilotChange("openai", "gpt-4.1"), false)
	var refusal *llmEvalRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want model_unevaluated refusal", err)
	}
	if refusal.Status.Status != evals.StatusUnevaluated || refusal.Status.Model != "gpt-4.1" {
		t.Fatalf("refusal status = %+v", refusal.Status)
	}
}

func TestEvalGateAdmitsEvaluatedModelAndAcknowledgedStaleModel(t *testing.T) {
	t.Setenv("KUBONA_REQUIRE_LLM_EVAL", "1")
	s, set, store := gateServer(t)
	report := evals.Report{
		Role: "copilot", Provider: "openai", Model: "gpt-4.1", FixturesSHA256: set.SHA256,
		EvaluatedAt: "2026-11-14T12:00:00Z", Total: set.ScoredCount(), Unscored: set.UnscoredCount(),
		Brier: 0.1, ECE: 0.05, PerLabel: map[string]evals.LabelScore{}, Labels: set.Labels,
	}
	if err := store.Write(report); err != nil {
		t.Fatal(err)
	}
	if ack, err := s.enforceLLMEvalGate(copilotChange("openai", "gpt-4.1"), false); err != nil || ack != nil {
		t.Fatalf("evaluated model gate = ack %+v err %v", ack, err)
	}
	// A different (stale/unevaluated) model still refuses, but explicit
	// acknowledgement passes and is returned for the audit event.
	if _, err := s.enforceLLMEvalGate(copilotChange("openai", "other-model"), false); err == nil {
		t.Fatal("unevaluated model must refuse")
	}
	ack, err := s.enforceLLMEvalGate(copilotChange("openai", "other-model"), true)
	if err != nil || ack == nil || ack.Model != "other-model" || ack.Status != evals.StatusUnevaluated {
		t.Fatalf("acknowledged gate = ack %+v err %v", ack, err)
	}
}

func TestEvalGateIgnoresUnrelatedOrUnchangedWritesAndIsOptIn(t *testing.T) {
	s, _, _ := gateServer(t)

	// Flag off: even an unevaluated change passes.
	if _, err := s.enforceLLMEvalGate(copilotChange("openai", "gpt-4.1"), false); err != nil {
		t.Fatalf("gate must be opt-in: %v", err)
	}

	t.Setenv("KUBONA_REQUIRE_LLM_EVAL", "1")
	// Unchanged assignment passes.
	if _, err := s.enforceLLMEvalGate(copilotChange("strata", "iq3_s"), false); err != nil {
		t.Fatalf("unchanged assignment: %v", err)
	}
	// A write that does not mention copilot passes.
	other := llm.DefaultSettings()
	other.RoleAssignments["coder"] = llm.RoleModelAssignment{Role: "coder", Provider: "openai", Model: "x"}
	delete(other.RoleAssignments, "copilot")
	if _, err := s.enforceLLMEvalGate(other, false); err != nil {
		t.Fatalf("unrelated write: %v", err)
	}
}
