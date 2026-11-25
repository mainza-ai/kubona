package api

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/verify"
)

func TestExtractUsedQubitCalibrationSlicesPerQubitMap(t *testing.T) {
	cal := map[string]any{
		"qubits": 3,
		"t1_us":  250.0,
		"per_qubit": map[string]any{
			"0": map[string]any{"t1_us": 250.0, "readout_error": 0.011},
			"1": map[string]any{"t1_us": 280.0, "readout_error": 0.014},
			"2": map[string]any{"t1_us": 220.0, "readout_error": 0.009},
		},
	}
	sliced := extractUsedQubitCalibration(cal, []int{0, 2})
	if len(sliced) != 2 || sliced["0"] == nil || sliced["2"] == nil || sliced["1"] != nil {
		t.Fatalf("unexpected per-qubit slice: %+v", sliced)
	}
	// Legacy snapshot without per_qubit returns nil cleanly
	if got := extractUsedQubitCalibration(map[string]any{"qubits": 3, "t1_us": 250.0}, []int{0}); got != nil {
		t.Fatalf("expected nil for legacy snapshot without per_qubit, got %+v", got)
	}
	// Verify scholarly metadata check clears once all 5 fields are populated
	ev := `{"_scholarly":{"affiliation":"Kubona","corresponding_author":"A <a@b.org>","competing_interests":"None","funding":"OSS","license":"CC-BY-4.0"}}`
	if miss := verify.CheckScholarlyMetadata(ev); len(miss) != 0 {
		t.Fatalf("expected 0 missing scholarly fields, got %+v", miss)
	}
}
