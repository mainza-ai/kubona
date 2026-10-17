package api

import (
	"encoding/json"
	"testing"
)

// Adapter contract: the A2A edge maps advertised capabilities onto ledger
// jobs with strict parameter validation. Anything unmapped or underspecified
// is refused here — enforcement lives in the core rail, never in the adapter.
func TestMapA2ATaskToJobContract(t *testing.T) {
	// Unknown task types refuse (no invented execution paths).
	if _, _, _, err := mapA2ATaskToJob("teleportation_teleport", json.RawMessage(`{}`)); err == nil {
		t.Fatalf("unmapped capability must refuse")
	}

	// Underspecified physics inputs refuse rather than default-guess.
	for _, tc := range []struct {
		taskType string
		payload  string
	}{
		{"molecular_ground_state_pyscf", `{"bond_distance":0.7414}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0}`},
		{"circuit_cutting_recombination", `{"qasm":"q;","max_subcircuit_qubits":0}`},
		{"zne_error_mitigation", `{}`},
		{"krylov_subspace_ground_state", `{"molecule":"H2"}`},
		{"molecular_ground_state_pyscf", `not-json`},
		{"epistemic_claim_evaluation", `{}`},
		{"epistemic_claim_evaluation", `{"claim_id":"c1","statement":"s"}`},
		{"epistemic_claim_evaluation", `{"claim_id":"c1","statement":"s","quantum_value":-1.1,"classical_anchor_value":-1.13}`},
		{"potential_energy_scan", `{}`},
		{"potential_energy_scan", `{"molecule":"H2"}`},
		{"potential_energy_scan", `{"molecule":"H2","r_min":0.5,"r_max":1.5,"steps":1}`},
		{"potential_energy_scan", `{"molecule":"H2","r_min":1.5,"r_max":0.5}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":0}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":-5}`},
		{"zne_error_mitigation", `{"qasm":"q;","max_cost_usd":"a-lot"}`},
	} {
		if _, _, _, err := mapA2ATaskToJob(tc.taskType, json.RawMessage(tc.payload)); err == nil {
			t.Fatalf("%s %s must refuse", tc.taskType, tc.payload)
		}
	}

	// Mapped capabilities bind the right ledger job types.
	mapped := map[string]string{
		"molecular_ground_state_pyscf":  "classical_anchor",
		"circuit_cutting_recombination": "circuit_cut",
		"zne_error_mitigation":          "mitigate",
		"krylov_subspace_ground_state":  "krylov_solve",
		"epistemic_claim_evaluation":    "claim_eval",
		"potential_energy_scan":         "pes_sweep",
	}
	payloads := map[string]string{
		"molecular_ground_state_pyscf":  `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5}`,
		"circuit_cutting_recombination": `{"qasm":"q;","max_subcircuit_qubits":2,"max_cost_usd":1.0}`,
		"zne_error_mitigation":          `{"qasm":"q;","max_cost_usd":1.0}`,
		"krylov_subspace_ground_state":  `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5}`,
		"epistemic_claim_evaluation":    `{"claim_id":"claim-h2-ground","statement":"H2 within chemical accuracy","quantum_value":-1.1368,"classical_anchor_value":-1.13727,"source_platforms":["pyscf","aer"],"total_engagement":12,"max_cost_usd":0.5}`,
		"potential_energy_scan":         `{"molecule":"H2","r_min":0.5,"r_max":1.5,"max_cost_usd":3.0}`,
	}
	for taskType, wantJob := range mapped {
		jobType, _, _, err := mapA2ATaskToJob(taskType, json.RawMessage(payloads[taskType]))
		if err != nil {
			t.Fatalf("%s: %v", taskType, err)
		}
		if jobType != wantJob {
			t.Fatalf("%s must map to %s, got %s", taskType, wantJob, jobType)
		}
	}
}
