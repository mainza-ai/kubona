package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/evals"
)

func TestEvalLLMRecordsCompletePredictionsAndRefusesIncomplete(t *testing.T) {
	set, err := evals.Load("copilot")
	if err != nil {
		t.Fatal(err)
	}
	input := evals.PredictionFile{
		Role: "copilot", Provider: "local", Model: "test-model", FixturesSHA256: set.SHA256,
	}
	for _, fixture := range set.Fixtures {
		if fixture.Trust == evals.TrustUnscored {
			continue
		}
		probs := map[string]float64{}
		for _, label := range set.Labels {
			probs[label] = 0
		}
		probs[fixture.Label] = 1
		input.Predictions = append(input.Predictions, evals.Prediction{ID: fixture.ID, Probabilities: probs})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "predictions.json")
	body, _ := json.Marshal(input)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runEvalLLM([]string{"--role", "copilot", "--predictions", path, "--dir", filepath.Join(dir, "reports")}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	status := (evals.Store{Dir: filepath.Join(dir, "reports")}).Status(set, "copilot", "local", "test-model")
	if status.Status != evals.StatusEvaluated || status.Evaluated != 10 || status.Brier == nil || *status.Brier >= 1e-9 {
		t.Fatalf("status = %+v", status)
	}

	incomplete := input
	incomplete.Model = "incomplete"
	incomplete.Predictions = incomplete.Predictions[:1]
	body, _ = json.Marshal(incomplete)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runEvalLLM([]string{"--role", "copilot", "--predictions", path, "--dir", filepath.Join(dir, "reports")}); code == 0 {
		t.Fatal("incomplete predictions must refuse")
	}
}

func TestEvalLLMLiveRequiresExplicitTarget(t *testing.T) {
	if code := runEvalLLM([]string{"--role", "copilot", "--live"}); code != 2 {
		t.Fatalf("live without provider/model must be a usage error, got %d", code)
	}
}

func TestParseProbabilityObjectRefusesIncompleteJSON(t *testing.T) {
	got, err := parseProbabilityObject("```json\n{\"supported\":0.7,\"refuted\":0.2,\"unverified\":0.1}\n```")
	if err != nil || got["supported"] != 0.7 || got["unverified"] != 0.1 {
		t.Fatalf("parse = %+v err=%v", got, err)
	}
	if _, err := parseProbabilityObject(`{"supported":1}`); err == nil {
		t.Fatal("missing labels must refuse")
	}
	if _, err := parseProbabilityObject("no json here"); err == nil {
		t.Fatal("non-JSON must refuse")
	}
}
