package evals

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testSet() *Set {
	set := &Set{
		Version: 1,
		Role:    "copilot",
		Labels:  append([]string(nil), AllowedLabels...),
		Fixtures: []Fixture{
			{ID: "a", Prompt: "a", Label: "supported", Trust: TrustMeasured},
			{ID: "b", Prompt: "b", Label: "refuted", Trust: TrustUnscored},
			{ID: "c", Prompt: "c", Label: "unverified", Trust: TrustInferred},
		},
	}
	set.SHA256 = set.computeHash()
	return set
}

func TestEmbeddedFixturesArePinnedAndPartitioned(t *testing.T) {
	set, err := Load("copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Fixtures) != 12 {
		t.Fatalf("fixtures = %d, want 12", len(set.Fixtures))
	}
	if len(set.scoredFixtures()) != 10 {
		t.Fatalf("scored = %d, want 10", len(set.scoredFixtures()))
	}
	if set.SHA256 == "" || len(set.SHA256) != 64 {
		t.Fatalf("set sha = %q", set.SHA256)
	}
	for _, fixture := range set.Fixtures {
		if fixture.SHA256 != fixturePin(fixture) {
			t.Fatalf("fixture %s pin drifted", fixture.ID)
		}
	}
	// Altering one prompt without re-pinning must fail validation.
	tampered := *set
	tampered.Fixtures = append([]Fixture(nil), set.Fixtures...)
	tampered.Fixtures[0].Prompt += " tampered"
	if err := tampered.validate(); err == nil || !strings.Contains(err.Error(), "sha256 drifted") {
		t.Fatalf("tamper error = %v", err)
	}
}

func TestEvaluateComputesBrierAndECE(t *testing.T) {
	set := testSet()
	input := PredictionFile{
		Role:           "copilot",
		Provider:       "local",
		Model:          "test-model",
		FixturesSHA256: set.SHA256,
		Predictions: []Prediction{
			{ID: "a", Probabilities: map[string]float64{"supported": 1, "refuted": 0, "unverified": 0}},
			{ID: "c", Probabilities: map[string]float64{"supported": 0.3, "refuted": 0.2, "unverified": 0.5}},
		},
	}
	report, err := Evaluate(set, input, time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 || report.Unscored != 1 || report.EvaluatedAt != "2026-11-14T12:00:00Z" {
		t.Fatalf("report header = %+v", report)
	}
	if math.Abs(report.Brier-0.19) > 1e-9 {
		t.Fatalf("brier = %.9f, want 0.19", report.Brier)
	}
	if math.Abs(report.ECE-0.25) > 1e-9 {
		t.Fatalf("ece = %.9f, want 0.25", report.ECE)
	}
	if got := report.PerLabel["supported"]; got.Correct != 1 || got.Total != 1 {
		t.Fatalf("supported score = %+v", got)
	}
	if got := report.PerLabel["unverified"]; got.Correct != 1 || got.Total != 1 {
		t.Fatalf("unverified score = %+v", got)
	}
}

func TestEvaluateRefusesIncompleteOrMalformedInput(t *testing.T) {
	set := testSet()
	base := PredictionFile{
		Role: "copilot", Provider: "p", Model: "m", FixturesSHA256: set.SHA256,
		Predictions: []Prediction{
			{ID: "a", Probabilities: map[string]float64{"supported": 1, "refuted": 0, "unverified": 0}},
			{ID: "c", Probabilities: map[string]float64{"supported": 1, "refuted": 0, "unverified": 0}},
		},
	}
	now := time.Now()

	cases := []struct {
		name   string
		mutate func(*PredictionFile)
		want   string
	}{
		{"missing", func(in *PredictionFile) { in.Predictions = in.Predictions[:1] }, "missing predictions"},
		{"unscored", func(in *PredictionFile) {
			in.Predictions = append(in.Predictions, Prediction{ID: "b", Probabilities: map[string]float64{"supported": 1}})
		}, "not a scored fixture"},
		{"unknown", func(in *PredictionFile) {
			in.Predictions = append(in.Predictions, Prediction{ID: "ghost", Probabilities: map[string]float64{"supported": 1}})
		}, "not a scored fixture"},
		{"bad-sum", func(in *PredictionFile) {
			in.Predictions[0].Probabilities = map[string]float64{"supported": 0.5, "refuted": 0.1, "unverified": 0.1}
		}, "probabilities sum"},
		{"bad-key", func(in *PredictionFile) {
			in.Predictions[0].Probabilities = map[string]float64{"supported": 1, "refuted": 0}
		}, "exactly"},
		{"sha", func(in *PredictionFile) { in.FixturesSHA256 = "00" }, "fixtures_sha256 mismatch"},
		{"role", func(in *PredictionFile) { in.Role = "device-auditor" }, "does not match"},
		{"provider", func(in *PredictionFile) { in.Provider = "" }, "provider and model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.Predictions = append([]Prediction(nil), base.Predictions...)
			tc.mutate(&input)
			_, err := Evaluate(set, input, now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStoreRoundTripAndStatusTransitions(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	set := testSet()
	status := store.Status(set, "copilot", "local", "m")
	if status.Status != StatusUnevaluated || status.Total != 2 || status.Unscored != 1 || status.EvaluatedAt != nil {
		t.Fatalf("initial status = %+v", status)
	}
	report := Report{
		Role: "copilot", Provider: "local", Model: "m", FixturesSHA256: set.SHA256,
		EvaluatedAt: "2026-11-14T12:00:00Z", Total: 2, Unscored: 1,
		Brier: 0.19, ECE: 0.25, PerLabel: map[string]LabelScore{"supported": {1, 1}},
		Labels: set.Labels,
	}
	if err := store.Write(report); err != nil {
		t.Fatal(err)
	}
	status = store.Status(set, "copilot", "local", "m")
	if status.Status != StatusEvaluated || status.Evaluated != 2 || status.Brier == nil || *status.Brier != 0.19 {
		t.Fatalf("stored status = %+v", status)
	}
	if _, err := os.Stat(store.path("copilot", "local", "m")); err != nil {
		t.Fatalf("report file: %v", err)
	}
	// A changed fixture set makes the recorded eval stale, not silently green.
	changed := *set
	changed.Fixtures = append([]Fixture(nil), set.Fixtures...)
	changed.Fixtures[0].Prompt += " changed"
	changed.SHA256 = changed.computeHash()
	status = store.Status(&changed, "copilot", "local", "m")
	if status.Status != StatusStale {
		t.Fatalf("status = %q, want stale", status.Status)
	}
}

func TestStoreRejectsNonFiniteScoresAndSanitizesPaths(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	bad := Report{Role: "copilot", Provider: "p", Model: "m", Brier: math.NaN(), ECE: 0}
	if err := store.Write(bad); err == nil {
		t.Fatal("NaN score must not be stored")
	}
	path := store.path("copilot", "openai/gpt 4", "../../etc/passwd")
	if strings.Contains(path, "..") || strings.Contains(filepath.Base(path), "/") {
		t.Fatalf("path escaped: %s", path)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Fatalf("path = %s", path)
	}
}
