package experimentresult

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDecodeAcceptsEverySupportedVariant(t *testing.T) {
	cases := []struct {
		jobType string
		body    map[string]any
	}{
		{"vqe_run", map[string]any{"kind": "vqe", "energy": -1.1, "fci_exact_energy": -1.137}},
		{"geometry_optimization", map[string]any{"kind": "geometry_optimization", "energy": -1.1, "fci_exact_energy": -1.137, "optimized_bond_distance": 0.74}},
		{"vqe_repeats", map[string]any{"kind": "vqe_repeats", "energy": -1.1, "fci_exact_energy": -1.137, "repeats": 3}},
		{"vqe_pes", map[string]any{"kind": "pes", "points": []map[string]any{{"energy": -1.1, "fci_exact_energy": -1.137}}}},
		{"qaoa_run", map[string]any{"kind": "qaoa", "energy": 7.5, "optimal_cut": 10}},
		{"excited_state", map[string]any{"kind": "excited_state", "ground_energy": -1.1, "excited_energy": -0.5, "gap": 0.6}},
		{"reaction", map[string]any{"kind": "reaction", "reaction_energy": 0.2, "reaction_energy_exact": 0.19}},
		{"spin_chain", map[string]any{"kind": "spin_chain", "energy": -3.1, "energy_exact": -3.2}},
		{"spin_chain_hardware", map[string]any{"kind": "spin_chain_hardware", "energy": -3.1, "energy_exact": -3.2, "hardware": true, "hardware_job_id": "vendor-1"}},
		{"hardware_experiment", map[string]any{"kind": "vqe_hardware", "energy": -1.1, "fci_exact_energy": -1.137, "hardware": true, "hardware_job_id": "vendor-vqe"}},
		{"hardware_experiment", map[string]any{"kind": "vqe_hardware_loop", "energy": -1.1, "fci_exact_energy": -1.137, "hardware": true, "hardware_job_ids": []string{"v1"}}},
		{"qaoa_hardware", map[string]any{"kind": "qaoa_hardware", "measured_cut": 7.5, "optimal_cut": 10, "hardware": true, "hardware_job_id": "vendor-qaoa"}},
		{"qaoa_hardware", map[string]any{"kind": "qaoa_hardware_loop", "measured_cut": 7.5, "optimal_cut": 10, "hardware": true, "hardware_job_id": "vendor-qaoa-loop"}},
		{"conjecture_run", map[string]any{"kind": "conjecture", "candidate_count": 1}},
		{"lattice_dynamics", map[string]any{"kind": "lattice_dynamics", "times": []map[string]any{{"t": 0.0, "energy": -4.0}, {"t": 0.5, "energy": -3.0}}}},
		{"thermodynamic_limit", map[string]any{"kind": "thermodynamic_limit", "finite_size": []map[string]any{{"n_sites": 8, "energy": -9.8, "energy_per_site": -1.23}, {"n_sites": 12, "energy": -15.0, "energy_per_site": -1.25}}}},
		{"hardware_pes", map[string]any{"kind": "hardware_pes", "hardware": true, "hardware_job_id": "vendor-pes", "points": []map[string]any{{"energy": -1.0, "fci_exact_energy": -1.0}, {"energy": -1.1, "fci_exact_energy": -1.1}}}},
	}
	for _, tc := range cases {
		tc.body["result_version"] = Version
		tc.body["success"] = true
		raw, _ := json.Marshal(tc.body)
		if _, err := Decode(tc.jobType, raw); err != nil {
			t.Fatalf("%s/%s: %v", tc.jobType, tc.body["kind"], err)
		}
	}
}

func TestDecodeRejectsTruthViolations(t *testing.T) {
	base := func(extra map[string]any) []byte {
		body := map[string]any{"result_version": Version, "success": true, "kind": "vqe", "energy": -1.1, "fci_exact_energy": -1.137}
		for k, v := range extra {
			body[k] = v
		}
		raw, _ := json.Marshal(body)
		return raw
	}
	cases := []struct {
		name    string
		jobType string
		raw     []byte
		want    string
	}{
		{"missing version", "vqe_run", []byte(`{"success":true,"kind":"vqe","energy":-1,"fci_exact_energy":-1}`), "result_version"},
		{"success false", "vqe_run", base(map[string]any{"success": false}), "success=false"},
		{"kind mismatch", "vqe_run", base(map[string]any{"kind": "spin_chain"}), "does not match"},
		{"missing anchor", "vqe_run", base(map[string]any{"fci_exact_energy": nil}), "fci_exact_energy"},
		{"nan", "vqe_run", []byte(`{"result_version":"experiment-result-v1","success":true,"kind":"vqe","energy":NaN,"fci_exact_energy":-1}`), "not valid JSON"},
		{"hardware no vendor id", "spin_chain_hardware", []byte(`{"result_version":"experiment-result-v1","success":true,"kind":"spin_chain_hardware","energy":-3,"energy_exact":-3.2,"hardware":true}`), "hardware_job_id"},
		{"qaoa hardware loop no vendor id", "qaoa_hardware", []byte(`{"result_version":"experiment-result-v1","success":true,"kind":"qaoa_hardware_loop","measured_cut":7.5,"optimal_cut":10,"hardware":true}`), "hardware_job_id"},
		{"receipt unsigned claim", "vqe_run", base(map[string]any{"result_receipt": map[string]any{"algorithm": "ed25519", "ledger_entry_hash": "h", "result_sha256": "r", "signed": true}}), "without a signature"},
	}
	for _, tc := range cases {
		if _, err := Decode(tc.jobType, tc.raw); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestSchemaKindEnumMatchesDecoder(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "contracts", "experiment-result-v1.schema.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaPath, err)
	}
	var doc struct {
		Properties struct {
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	schemaSet := make(map[string]bool, len(doc.Properties.Kind.Enum))
	for _, k := range doc.Properties.Kind.Enum {
		schemaSet[k] = true
	}
	decoderSet := make(map[string]bool)
	for _, kinds := range ExpectedJobKinds() {
		for _, k := range kinds {
			decoderSet[k] = true
		}
	}
	var missingInSchema, missingInDecoder []string
	for k := range decoderSet {
		if !schemaSet[k] {
			missingInSchema = append(missingInSchema, k)
		}
	}
	for k := range schemaSet {
		if !decoderSet[k] {
			missingInDecoder = append(missingInDecoder, k)
		}
	}
	sort.Strings(missingInSchema)
	sort.Strings(missingInDecoder)
	if len(missingInSchema) > 0 || len(missingInDecoder) > 0 {
		t.Fatalf("experiment-result-v1.schema.json kind enum drift: missingInSchema=%v missingInDecoder=%v", missingInSchema, missingInDecoder)
	}
}

func TestReceiptFromJSONIsOptional(t *testing.T) {
	if got := ReceiptFromJSON([]byte(`{"result_receipt":{"signed":true,"signature":"s","ledger_entry_hash":"h","result_sha256":"r"}}`)); got == nil || !got.Signed {
		t.Fatalf("receipt not extracted: %+v", got)
	}
	if got := ReceiptFromJSON([]byte(`{"success":true}`)); got != nil {
		t.Fatalf("absent receipt must stay absent: %+v", got)
	}
}
