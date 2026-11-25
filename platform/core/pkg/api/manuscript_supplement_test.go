package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
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
	arts := map[string][]byte{"main.pdf": []byte("%PDF-1.4")}
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
}
