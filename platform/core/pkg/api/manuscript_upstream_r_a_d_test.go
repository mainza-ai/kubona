package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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

	// H3 (C-B): stageCountsArtifact writes .json when <= cap and auto-gzips to .json.gz when > cap
	arts := map[string][]byte{}
	pSmall, okSmall := stageCountsArtifact("supplement/counts/j1", map[string]int{"00": 512, "11": 512}, 256*1024, arts)
	if !okSmall || pSmall != "supplement/counts/j1.json" || len(arts[pSmall]) == 0 {
		t.Fatalf("expected plain .json staging, got %q ok=%v", pSmall, okSmall)
	}
	big := map[string]map[string]int{}
	for i := 0; i < 60; i++ {
		term := map[string]int{}
		for j := 0; j < 200; j++ {
			term[string(rune('a'+(j%26)))+string(rune('0'+(j%10)))+string(rune('A'+(i%26)))+string(rune('0'+(i%10)))] = j + 1
		}
		big[string(rune('A'+(i%26)))+string(rune('0'+(i%10)))] = term
	}
	pBig, okBig := stageCountsArtifact("supplement/counts/j2", big, 1024, arts)
	if !okBig || pBig != "supplement/counts/j2.json.gz" || len(arts[pBig]) == 0 {
		t.Fatalf("H3 violated: expected .json.gz fallback for large histogram, got %q ok=%v", pBig, okBig)
	}
	gz, err := gzip.NewReader(bytes.NewReader(arts[pBig]))
	if err != nil {
		t.Fatalf("staged histogram is not valid gzip: %v", err)
	}
	plain, err := io.ReadAll(gz)
	if err != nil || !json.Valid(plain) {
		t.Fatalf("gzip payload does not decompress to JSON: %v", err)
	}
	// C-C: countsArtifactMeta distinguishes plain vs gzip encoding and hashes the staged bytes
	encSmall, shaSmall := countsArtifactMeta(pSmall, arts)
	wantSmall := sha256.Sum256(arts[pSmall])
	if encSmall != "json" || shaSmall != hex.EncodeToString(wantSmall[:]) {
		t.Fatalf("C-C violated for plain json: enc=%q sha=%q", encSmall, shaSmall)
	}
	encBig, shaBig := countsArtifactMeta(pBig, arts)
	wantBig := sha256.Sum256(arts[pBig])
	plainSum := sha256.Sum256(plain)
	if encBig != "gzip" || shaBig != hex.EncodeToString(wantBig[:]) || shaBig == hex.EncodeToString(plainSum[:]) {
		t.Fatalf("C-C violated for gzip json: enc=%q sha=%q", encBig, shaBig)
	}
	// C-D: HasSettledHardware distinguishes settled hardware from simulator or failed rows
	if verify.HasSettledHardware(`{"experiments":[{"job_id":"j1","kind":"vqe","hardware":false,"measured":true}]}`) {
		t.Fatal("HasSettledHardware must be false for simulator-only campaign")
	}
	if !verify.HasSettledHardware(`{"experiments":[{"job_id":"j2","kind":"vqe_hardware","hardware":true,"measured":true}]}`) {
		t.Fatal("HasSettledHardware must be true for settled hardware campaign")
	}
}
