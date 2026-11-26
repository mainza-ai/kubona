package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestAttachSupplementArchivesCDRCountsAndAblationDiagnostics(t *testing.T) {
	resultMap := map[string]any{
		"success":   true,
		"kind":      "vqe_hardware",
		"hardware":  true,
		"mitigated": true,
		"measured":  false,
		"energy":    -1.1368,
		"delta_mha": 0.506,
		"ci95_mha":  1.85,
		"shots":     4096,
		"counts_by_term": map[string]any{
			"IZ": map[string]any{"00": 64000, "11": 1536},
			"XX": map[string]any{"00": 8000, "11": 8384},
		},
		"cdr_training_counts": map[string]any{
			"train_00": map[string]any{"ZZ": map[string]any{"00": 4000, "11": 96}},
			"train_01": map[string]any{"ZZ": map[string]any{"00": 2000, "11": 2096}},
		},
		"readout_cal_counts": map[string]any{
			"00": map[string]any{"00": 8100, "01": 92},
			"01": map[string]any{"01": 8050, "00": 142},
			"10": map[string]any{"10": 8040, "00": 152},
			"11": map[string]any{"11": 7980, "01": 100, "10": 112},
		},
		"mitigation": map[string]any{
			"method":                    "CDR",
			"fit_method":                "cdr_eiv_deming_exact",
			"slope":                     1.02,
			"intercept":                 0.01,
			"r_squared":                 0.998,
			"chi2_reduced":              0.91,
			"dof":                       10,
			"loo_cv_mha":                1.7,
			"training_count":            12,
			"distinct_energies":         12,
			"target_in_training_domain": true,
			"model_adequate":            true,
			"difference_ha":             -0.018,
			"difference_ci95_mha":       3.1,
			"difference_significant":    true,
			"cdr_input_layer":           "symmetry_verified",
		},
		"ablation_layers": map[string]any{
			"raw_unmitigated":        map[string]any{"energy": -1.119, "delta_mha": 18.3, "ci95_mha": 2.4},
			"live_readout_mitigated": map[string]any{"energy": -1.131, "delta_mha": 6.3, "ci95_mha": 3.1},
			"symmetry_verified":      map[string]any{"energy": -1.133, "delta_mha": 4.3, "ci95_mha": 2.6},
			"cdr_mitigated":          map[string]any{"energy": -1.1368, "delta_mha": 0.506, "ci95_mha": 1.85},
		},
		"symmetry": map[string]any{"method": "bonet_monroig_exact_projector", "survival_probability": 0.96},
	}
	resJSON, _ := json.Marshal(resultMap)
	paramsJSON, _ := json.Marshal(map[string]any{
		"shots": 4096,
		"seed":  7,
		"qasm":  "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncx q[0],q[1];\n",
	})

	row := map[string]any{
		"job_id":    "job_cdr_1",
		"kind":      "vqe_hardware",
		"backend":   "ibm_fez",
		"hardware":  true,
		"mitigated": true,
		"energy":    -1.1368,
		"delta_mha": 0.506,
	}
	enrichJobFromParams(row, paramsJSON, resJSON)
	if row["mitigation_method"] != "CDR" {
		t.Fatalf("expected mitigation_method=CDR, got %v", row["mitigation_method"])
	}
	if row["ablation_layers"] == nil {
		t.Fatalf("expected ablation_layers in enriched campaign row")
	}

	// Verify stageCountsArtifact directly with gzip threshold override (also covers C-F gzip path).
	artifacts := map[string][]byte{}
	if p, ok := stageCountsArtifact("supplement/counts/job_cdr_1_cdr_train", resultMap["cdr_training_counts"], 256*1024, artifacts); !ok || p != "supplement/counts/job_cdr_1_cdr_train.json" {
		t.Fatalf("unexpected cdr_train staging result: %q ok=%v", p, ok)
	}
	if p, ok := stageCountsArtifact("supplement/counts/job_cdr_1_readout_cal", resultMap["readout_cal_counts"], 256*1024, artifacts); !ok || p != "supplement/counts/job_cdr_1_readout_cal.json" {
		t.Fatalf("unexpected readout_cal staging result: %q ok=%v", p, ok)
	}
	// Trigger gzip threshold cleanly
	bigMap := map[string]any{}
	for i := 0; i < 64; i++ {
		bigMap[strings.Repeat("01", 8)+string(rune('a'+i%26))+string(rune('A'+i/26))] = i + 1
	}
	if p, ok := stageCountsArtifact("supplement/counts/job_cdr_big", bigMap, 1024, artifacts); !ok || !strings.HasSuffix(p, ".json.gz") {
		t.Fatalf("expected .json.gz staging above threshold, got %q ok=%v", p, ok)
	}
	_ = jobs.HardwareExperimentParams{}
	_ = ledger.Manuscript{}
}
