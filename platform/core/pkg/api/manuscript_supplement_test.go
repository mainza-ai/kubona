package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/version"
)

func TestEnrichJobFromParamsIsolatesRawQASMAndHashesCircuit(t *testing.T) {
	row := map[string]any{"job_id": "job_hw_99", "hardware": true}
	params := []byte(`{"qasm":"OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\nrz(4.0934) q[0];\ncx q[0],q[1];\n","seed":42,"shots":1024,"theta":[0.1,0.2,0.3]}`)
	res := []byte(`{"counts":{"00":512,"11":512},"energy":-1.137}`)

	enrichJobFromParams(row, params, res)

	if _, leaked := row["qasm"]; leaked {
		t.Fatalf("Phase 0 invariant violated: raw qasm must NEVER be stored in campaign row (breaks verification-v1)")
	}
	if _, leaked := row["counts"]; leaked {
		t.Fatalf("Phase 0 invariant violated: full counts map must not be duplicated in campaign row")
	}
	if h, _ := row["circuit_sha256"].(string); len(h) != 64 {
		t.Fatalf("expected 64-hex circuit_sha256, got %q", h)
	}
	if h, _ := row["counts_sha256"].(string); len(h) != 64 {
		t.Fatalf("expected 64-hex counts_sha256, got %q", h)
	}
	if row["seed"] != float64(42) {
		t.Fatalf("expected seed=42, got %#v", row["seed"])
	}
	if row["two_qubit_gate_count"] != 1 {
		t.Fatalf("expected two_qubit_gate_count=1, got %#v", row["two_qubit_gate_count"])
	}

	// Verify serialized campaign row never contains QASM rotation literal "4.0934"
	b, _ := json.Marshal(row)
	if strings.Contains(string(b), "4.0934") {
		t.Fatalf("raw QASM rotation literal leaked into campaign JSON: %s", string(b))
	}
}

func TestAttachSupplementArtifactsStagesDatasheetCSVEnvAndCompleteness(t *testing.T) {
	srv := &Server{}
	campBytes, _ := json.Marshal(map[string]any{
		"experiments": []map[string]any{
			{"job_id": "sim_1", "kind": "vqe", "hardware": false, "seed": 7, "shots": 1024, "energy": -1.137},
		},
	})
	ms := &ledger.Manuscript{
		ID:              "ms_sup_1",
		SessionID:       "sess_1",
		CampaignJSON:    string(campBytes),
		EvidenceJSON:    "{}",
		SourceDateEpoch: 1700000000,
	}
	oldCommit := version.Commit
	version.Commit = "0123456789abcdef0123456789abcdef01234567"
	defer func() { version.Commit = oldCommit }()
	arts := map[string][]byte{
		"main.pdf":                []byte("%PDF-1.4"),
		"supplement/tooling.json": []byte(`{"python":"3.11.9","qiskit":"2.6.2"}`),
	}
	srv.attachSupplementArtifacts("sess_1", ms, arts)

	for _, need := range []string{
		"supplement/datasheet.json",
		"supplement/jobs.csv",
		"supplement/environment.json",
		"supplement/completeness_report.json",
	} {
		if len(arts[need]) == 0 {
			t.Fatalf("expected %s to be staged in artifacts", need)
		}
	}
	var env map[string]any
	if err := json.Unmarshal(arts["supplement/environment.json"], &env); err != nil {
		t.Fatalf("environment.json is not decodable: %v", err)
	}
	tooling, _ := env["tooling"].(map[string]any)
	if tooling["qiskit"] != "2.6.2" {
		t.Fatalf("expected bridge tooling versions merged into environment.json, got %#v", env["tooling"])
	}
	if env["kubona_commit"] != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("expected injected commit in environment.json, got %#v", env["kubona_commit"])
	}
}

func TestEnrichJobFromParamsHashesPerPointCircuits(t *testing.T) {
	row := map[string]any{"job_id": "job_pes_1", "hardware": true}
	params := []byte(`{"kind":"hardware_pes","points":[{"bond_distance":0.5,"qasm":"OPENQASM 2.0;\nqreg q[2];\nry(0.5) q[0];\n"},{"bond_distance":0.7,"qasm":"OPENQASM 2.0;\nqreg q[2];\nry(0.7) q[0];\n"}]}`)
	enrichJobFromParams(row, params, nil)
	if row["qasm_points"] != 2 {
		t.Fatalf("expected qasm_points=2, got %#v", row["qasm_points"])
	}
	shas, _ := row["point_circuits_sha256"].([]string)
	if len(shas) != 2 || len(shas[0]) != 64 {
		t.Fatalf("expected two 64-hex point hashes, got %#v", row["point_circuits_sha256"])
	}
	if _, leaked := row["qasm"]; leaked {
		t.Fatalf("raw per-point qasm must not be stored in the campaign row")
	}
}

func TestRatioCIFromCountsUsesCutDistribution(t *testing.T) {
	row := map[string]any{"job_id": "job_qaoa_1", "hardware": true}
	params := []byte(`{"kind":"qaoa_hardware","edges":[[0,1]],"optimal_cut":1}`)
	result := []byte(`{"counts":{"00":100,"01":100,"10":100,"11":100},"optimal_cut":1,"approximation_ratio":0.5}`)

	enrichJobFromParams(row, params, result)

	ci, ok := row["ratio_ci95"].([]float64)
	if !ok || len(ci) != 2 {
		t.Fatalf("expected ratio_ci95 [lo,hi], got %#v", row["ratio_ci95"])
	}
	// cut values are 0,1,1,0 over 400 shots: mean 0.5, 95% normal interval
	// around 0.5 with se≈0.0250 => about [0.451, 0.549].
	if !(ci[0] > 0.44 && ci[0] < 0.46 && ci[1] > 0.54 && ci[1] < 0.56) {
		t.Fatalf("unexpected CI %#v for the known cut distribution", ci)
	}
	if row["ratio_ci_method"] != "normal_approx_95" {
		t.Fatalf("expected the CI method to be recorded, got %#v", row["ratio_ci_method"])
	}
}

func TestRatioCIFromCountsRefusesWithoutGraph(t *testing.T) {
	row := map[string]any{"job_id": "job_qaoa_2", "hardware": true}
	params := []byte(`{"kind":"qaoa_hardware","optimal_cut":1}`)
	result := []byte(`{"counts":{"00":100,"11":100},"optimal_cut":1}`)
	enrichJobFromParams(row, params, result)
	if _, ok := row["ratio_ci95"]; ok {
		t.Fatalf("must not fabricate a ratio interval without graph edges: %#v", row["ratio_ci95"])
	}
}

func TestUsedQubitsFromNestedLayout(t *testing.T) {
	layout := map[string]any{
		"initial": map[string]any{"0": float64(58), "1": float64(71), "2": float64(70)},
		"final":   map[string]any{},
	}
	got := extractUsedQubits(layout, nil)
	want := []int{58, 70, 71}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
	flat := extractUsedQubits([]any{float64(3), float64(1)}, nil)
	if len(flat) != 2 || flat[0] != 1 || flat[1] != 3 {
		t.Fatalf("expected sorted [1 3], got %v", flat)
	}
}
