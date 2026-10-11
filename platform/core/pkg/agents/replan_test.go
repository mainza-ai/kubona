package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/epistemics"
)

func replanTestState(t *testing.T, prompt string) []byte {
	t.Helper()
	state, err := SynthesizeCampaignState(context.Background(), "sess-replan-test", prompt, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeReplan(t *testing.T, updated []byte) CampaignState {
	t.Helper()
	var state CampaignState
	if err := json.Unmarshal(updated, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestReplanCircuitRun(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	before := decodeReplan(t, raw)

	updated, changed, _ := ReplanWithJobResult(raw, "job-abcdef123456", "circuit_run",
		[]byte(`{"success":true,"resolved_backend":"local_simulator","meyer_wallach_entanglement":0.77,"counts":{"00":512,"11":512}}`), "")
	if !changed {
		t.Fatal("measured entanglement must fold")
	}
	after := decodeReplan(t, updated)
	if after.QuantumFactors.MeyerWallachQ != 0.77 {
		t.Errorf("MW not replaced: %v", after.QuantumFactors.MeyerWallachQ)
	}
	if after.QuantumFactors.MeyerWallachQ == before.QuantumFactors.MeyerWallachQ {
		t.Error("MW unchanged from synthesis estimate")
	}
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.HasPrefix(last, "Job job-abcd (circuit_run):") || !strings.Contains(last, "1024 shots") {
		t.Errorf("provenance line wrong: %q", last)
	}

	// success=false measures nothing: no fold, no version.
	if _, changed, _ := ReplanWithJobResult(raw, "job-x", "circuit_run",
		[]byte(`{"success":false,"error_message":"boom"}`), ""); changed {
		t.Error("failed run must not fold")
	}
}

func TestReplanKrylov(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	before := decodeReplan(t, raw)

	updated, changed, _ := ReplanWithJobResult(raw, "job-krylov1", "krylov_solve",
		[]byte(`{"success":true,"ground_state_energy":-1.13699,"classical_fci_energy":-1.13727,"meyer_wallach_entanglement":0.81}`), "")
	if !changed {
		t.Fatal("krylov energy must fold")
	}
	after := decodeReplan(t, updated)
	if len(after.Trajectory) != len(before.Trajectory)+1 {
		t.Fatalf("trajectory must grow by exactly one, was %d now %d", len(before.Trajectory), len(after.Trajectory))
	}
	pt := after.Trajectory[len(after.Trajectory)-1]
	if pt.Energy != -1.13699 {
		t.Errorf("measured energy must land exactly, got %v", pt.Energy)
	}
	if pt.Watchdog != "measured_krylov" {
		t.Errorf("measurement must be tagged, got %q", pt.Watchdog)
	}
	if len(pt.Theta) != 0 {
		t.Error("unrun parameters must stay empty, never carried forward")
	}
}

func TestReplanAnchor(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	before := decodeReplan(t, raw)

	updated, changed, _ := ReplanWithJobResult(raw, "job-anchor1", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H2","bond_distance":0.7414,"rhf_energy":-1.11670,"fci_exact_energy":-1.13730}`), "")
	if !changed {
		t.Fatal("recomputed anchor must fold")
	}
	after := decodeReplan(t, updated)
	if after.ClassicalAnchor.FciExactEnergy != -1.13730 {
		t.Errorf("FCI not replaced: %v", after.ClassicalAnchor.FciExactEnergy)
	}
	if after.Notebook.ProvenanceHash == before.Notebook.ProvenanceHash {
		t.Error("provenance must re-hash when FCI changes")
	}
	if !after.ClassicalAnchor.ChemicalAccuracyPassed {
		t.Error("0.31 mHa discrepancy must pass chemical accuracy")
	}

	// Wrong-molecule anchor corrupts: refuse the fold.
	if _, changed, note := ReplanWithJobResult(raw, "job-anchor2", "classical_anchor",
		[]byte(`{"success":true,"molecule":"LiH","bond_distance":1.5957,"rhf_energy":-7.86,"fci_exact_energy":-7.88}`), ""); changed {
		t.Error("mismatched molecule must not fold")
	} else if !strings.Contains(note, "mismatch") {
		t.Errorf("refusal must name the cause, got %q", note)
	}
}

func TestReplanClaimEval(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")

	updated, changed, _ := ReplanWithJobResult(raw, "job-claim1", "claim_eval",
		[]byte(`{"claim_id":"c1","corroborated_amplitude":0.97,"contested_amplitude":0.02,"unverified_amplitude":0.05,"passes_chemical_accuracy":true,"verdict_reason":"within 0.4 mHa"}`), "")
	if !changed {
		t.Fatal("verdict must fold")
	}
	after := decodeReplan(t, updated)
	if after.Epistemics.Status != "COLLAPSED_VERIFIED" || after.Epistemics.AlphaCorroborated != 0.97 {
		t.Errorf("verdict not applied: %+v", after.Epistemics)
	}

	updated, changed, _ = ReplanWithJobResult(raw, "job-claim2", "claim_eval",
		[]byte(`{"claim_id":"c2","corroborated_amplitude":0.40,"contested_amplitude":0.50,"unverified_amplitude":0.30,"passes_chemical_accuracy":false,"verdict_reason":"3.2 mHa off"}`), "")
	if !changed {
		t.Fatal("failing verdict must fold")
	}
	after = decodeReplan(t, updated)
	if after.Epistemics.Status != "CONTESTED" || !strings.Contains(after.Epistemics.Statement, "3.2 mHa") {
		t.Errorf("failure not recorded: %+v", after.Epistemics)
	}

	// Amplitude-free payload evaluates nothing.
	if _, changed, _ := ReplanWithJobResult(raw, "job-claim3", "claim_eval",
		[]byte(`{"claim_id":"c3","verdict_reason":"empty"}`), ""); changed {
		t.Error("verdict without amplitudes must not fold")
	}
}

func TestReplanNotebookOnlyAndUnknown(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")

	updated, changed, _ := ReplanWithJobResult(raw, "job-mit1", "mitigate",
		[]byte(`{"success":true,"unmitigated_value":-1.135,"mitigated_value":-1.1371,"error_bound":0.001,"method_used":"ZNE"}`), "")
	if !changed {
		t.Fatal("mitigation must record")
	}
	after := decodeReplan(t, updated)
	if !strings.Contains(after.Notebook.Sections[len(after.Notebook.Sections)-1], "ZNE mitigation") {
		t.Error("mitigation line missing")
	}
	// Mitigation measures an error bar, not the campaign: state untouched.
	if after.QuantumFactors.MeyerWallachQ != decodeReplan(t, raw).QuantumFactors.MeyerWallachQ {
		t.Error("mitigation must not touch state fields")
	}

	if _, changed, _ := ReplanWithJobResult(raw, "job-zzz", "teleport", []byte(`{"success":true}`), ""); changed {
		t.Error("unknown job type must not fold")
	}
	if _, changed, _ := ReplanWithJobResult(raw, "job-zzz", "circuit_run", []byte(`not json`), ""); changed {
		t.Error("undecodable result must not fold")
	}
}

func TestReplanNotebookCap(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	var state CampaignState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	narrative := len(state.Notebook.Sections)
	for i := 0; i < 60; i++ {
		state.Notebook.Sections = append(state.Notebook.Sections, "Job filler: old measurement")
	}
	full, _ := json.Marshal(state)

	updated, changed, _ := ReplanWithJobResult(full, "job-cap1", "circuit_run", []byte(`{"success":true}`), "")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	if len(after.Notebook.Sections) > maxNotebookSections {
		t.Errorf("notebook exceeds cap: %d", len(after.Notebook.Sections))
	}
	// Narrative sections survive; only measurement lines fall off.
	kept := 0
	for _, sec := range after.Notebook.Sections {
		for _, orig := range decodeReplan(t, raw).Notebook.Sections {
			if sec == orig {
				kept++
				break
			}
		}
	}
	if kept != narrative {
		t.Errorf("narrative must survive the cap: kept %d of %d", kept, narrative)
	}
}

func TestEvalClaimRecordDraft(t *testing.T) {
	draft, ok := EvalClaimRecord([]byte(`{"claim_id":"claim-h2-ground","corroborated_amplitude":0.97,"contested_amplitude":0.02,"unverified_amplitude":0.05,"passes_chemical_accuracy":true,"verdict_reason":"within 0.4 mHa"}`))
	if !ok {
		t.Fatal("evaluable verdict must draft")
	}
	if draft.Verdict != "verified" || draft.Alpha != 0.97 || draft.ExternalID != "claim-h2-ground" {
		t.Errorf("draft wrong: %+v", draft)
	}
	if _, ok := EvalClaimRecord([]byte(`{"claim_id":"c3","verdict_reason":"empty"}`)); ok {
		t.Error("amplitude-free verdict must not draft")
	}
	if _, ok := EvalClaimRecord([]byte(`not json`)); ok {
		t.Error("undecodable result must not draft")
	}
}

func TestReplanCitesClaimRecord(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, _ := ReplanWithJobResult(raw, "job-claim9", "claim_eval",
		[]byte(`{"claim_id":"c9","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"passes_chemical_accuracy":true,"verdict_reason":"fine"}`), "clm-sessrepl-0001")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "[clm-sessrepl-0001]") {
		t.Errorf("notebook line must cite the record, got %q", last)
	}

	// Empty cite: no dangling brackets.
	updated, _, _ = ReplanWithJobResult(raw, "job-claim9", "claim_eval",
		[]byte(`{"claim_id":"c9","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"passes_chemical_accuracy":true,"verdict_reason":"fine"}`), "")
	after = decodeReplan(t, updated)
	last = after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if strings.Contains(last, "[") {
		t.Errorf("empty cite must leave no brackets, got %q", last)
	}
}

// Every synthesized campaign ships Sources to the UI: they must resolve
// under the evidence vocabulary, never free text.
func TestSynthSourcesResolve(t *testing.T) {
	for _, prompt := range []string{"H2 ground state VQE", "LiH dissociation", "fresh water scan"} {
		state, err := SynthesizeCampaignState(context.Background(), "sess-sources", prompt, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Epistemics.Sources) < 2 {
			t.Errorf("%q: campaign must carry at least 2 evidence sources", prompt)
		}
		for _, src := range state.Epistemics.Sources {
			if !epistemics.ValidSourceLabel(src) {
				t.Errorf("%q: unresolvable source %q", prompt, src)
			}
		}
	}
}

func TestReplanPesSweep(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	before := decodeReplan(t, raw)

	updated, changed, _ := ReplanWithJobResult(raw, "job-sweep1", "pes_sweep",
		[]byte(`{"success":true,"molecule":"H2","points":[
			{"r":0.5,"rhf_energy":-1.06123,"fci_energy":-1.08345},
			{"r":1.0,"rhf_energy":-1.10011,"fci_energy":-1.12000},
			{"r":1.5,"error":"anchor reported success=false"}]}`), "")
	if !changed {
		t.Fatal("sweep table must fold")
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "Job job-swee (pes_sweep): H2 classical PES, 3 points") {
		t.Errorf("table header wrong: %q", last)
	}
	if !strings.Contains(last, "R=1.0000 A  RHF=-1.10011  FCI=-1.12000 Ha") {
		t.Errorf("point line wrong: %q", last)
	}
	if !strings.Contains(last, "R=1.5000 A  anchor failed:") {
		t.Errorf("failed point must stay transparent: %q", last)
	}
	if !strings.Contains(last, "coarse FCI minimum near R=1.0000 A") {
		t.Errorf("argmin missing: %q", last)
	}
	// Reference state untouched: anchor stays equilibrium, trajectory stays
	// optimizer output.
	if after.ClassicalAnchor.FciExactEnergy != before.ClassicalAnchor.FciExactEnergy {
		t.Error("sweep must not move the equilibrium anchor")
	}
	if len(after.Trajectory) != len(before.Trajectory) {
		t.Error("sweep must not append trajectory points")
	}

	// A sweep that measured nothing folds nothing.
	if _, changed, _ := ReplanWithJobResult(raw, "job-sweep2", "pes_sweep",
		[]byte(`{"success":true,"molecule":"H2","points":[{"r":0.5,"error":"boom"}]}`), ""); changed {
		t.Error("point-free sweep must not fold")
	}
}

// B0-1: an uncomputed anchor is never verification-grade. Nil bridge =
// fallback path: labeled everywhere, accuracy fails, no collapse.
func TestSynthFallbackAnchorNeverVerifies(t *testing.T) {
	state, err := SynthesizeCampaignState(context.Background(), "sess-fallback", "H2 ground state VQE", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.ClassicalAnchor.AnchorComputed {
		t.Error("nil bridge must not mark anchor computed")
	}
	if state.ClassicalAnchor.AnchorSource != "hardcoded-fallback" {
		t.Errorf("source must name the fallback, got %q", state.ClassicalAnchor.AnchorSource)
	}
	if state.ClassicalAnchor.ChemicalAccuracyPassed {
		t.Error("chemical accuracy cannot pass on a fallback anchor")
	}
	if state.Epistemics.Status != "UNVERIFIED" {
		t.Errorf("fallback epistemics must be UNVERIFIED, got %q", state.Epistemics.Status)
	}
	if state.Epistemics.AlphaCorroborated != 0.50 {
		t.Errorf("fallback amplitudes must drop to pending, got %v", state.Epistemics.AlphaCorroborated)
	}
	if !strings.Contains(state.Epistemics.Statement, "hardcoded fallback") {
		t.Errorf("statement must carry the caveat: %q", state.Epistemics.Statement)
	}
	if state.Notebook.EpistemicVerdict != "UNVERIFIED_FALLBACK" {
		t.Errorf("notebook verdict must name the fallback, got %q", state.Notebook.EpistemicVerdict)
	}
	if !strings.Contains(state.Notebook.Sections[1], "UNAVAILABLE") {
		t.Errorf("notebook section 2 must disclose the missing anchor: %q", state.Notebook.Sections[1])
	}
	if !strings.Contains(state.ClassicalAnchor.Method, "fallback") {
		t.Errorf("method must not read as PySCF: %q", state.ClassicalAnchor.Method)
	}

	// Fresh campaigns keep HYPOTHESIS_PENDING (already pending) + caveat.
	fresh, err := SynthesizeCampaignStateWithOpts(context.Background(), "sess-fresh-fb", "H2", "", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Epistemics.Status != "HYPOTHESIS_PENDING" {
		t.Errorf("fresh fallback keeps HYPOTHESIS_PENDING, got %q", fresh.Epistemics.Status)
	}
	if !strings.Contains(fresh.Epistemics.Statement, "hardcoded fallback") {
		t.Errorf("fresh statement must carry the caveat: %q", fresh.Epistemics.Statement)
	}
}

// A recomputed anchor promotes the campaign back to computed-grade.
func TestReplanAnchorPromotesComputed(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, _ := ReplanWithJobResult(raw, "job-anchor9", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H2","bond_distance":0.7414,"rhf_energy":-1.11668,"fci_exact_energy":-1.13727}`), "")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	if !after.ClassicalAnchor.AnchorComputed || after.ClassicalAnchor.AnchorSource != "pyscf-live" {
		t.Errorf("recomputed anchor must promote to computed-grade: %+v", after.ClassicalAnchor)
	}
}

// Robustness: absent verdict/values refuse the fold instead of inventing
// outcomes, and molecule identity is exact — "H" must never satisfy "H2".
func TestReplanPresenceRules(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")

	if _, changed, note := ReplanWithJobResult(raw, "job-v1", "claim_eval",
		[]byte(`{"claim_id":"c","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"verdict_reason":"x"}`), ""); changed {
		t.Error("verdict without passes key must not fold")
	} else if !strings.Contains(note, "unknown") {
		t.Errorf("refusal must name the cause, got %q", note)
	}
	if _, changed, _ := ReplanWithJobResult(raw, "job-v2", "mitigate",
		[]byte(`{"success":true,"method_used":"ZNE","unmitigated_value":-1.1}`), ""); changed {
		t.Error("mitigation without mitigated value must not fold")
	}
	if _, changed, _ := ReplanWithJobResult(raw, "job-v3", "circuit_cut",
		[]byte(`{"success":true}`), ""); changed {
		t.Error("cut without count must not fold")
	}
	if _, changed, _ := ReplanWithJobResult(raw, "job-v4", "pes_sweep",
		[]byte(`{"success":true,"molecule":"H2","points":[{"rhf_energy":-1.0,"fci_energy":-1.1}]}`), ""); changed {
		t.Error("point without geometry must not fold")
	}
	if _, changed, note := ReplanWithJobResult(raw, "job-v5", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H","bond_distance":0.5,"rhf_energy":-0.5,"fci_exact_energy":-0.6}`), ""); changed {
		t.Error("H anchor must never satisfy an H2 campaign")
	} else if !strings.Contains(note, "mismatch") {
		t.Errorf("refusal must name the cause, got %q", note)
	}
}

func TestEvalClaimRecordRequiresVerdict(t *testing.T) {
	if _, ok := EvalClaimRecord([]byte(`{"claim_id":"c","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"verdict_reason":"x"}`)); ok {
		t.Error("amplitudes without a verdict key must not draft")
	}
}
