package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

func replanTestState(t *testing.T, prompt string) []byte {
	t.Helper()
	state, err := BuildCampaignProposal(context.Background(), "sess-replan-test", prompt, "", nil)
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
		t.Error("MW unchanged from the unmeasured proposal")
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

	updated, changed, _ := ReplanWithJobResult(raw, "job-anchor1", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H2","bond_distance":0.7414,"rhf_energy":-1.11670,"fci_exact_energy":-1.13730}`), "")
	if !changed {
		t.Fatal("recomputed anchor must fold")
	}
	after := decodeReplan(t, updated)
	if after.ClassicalAnchor.FciExactEnergy != -1.13730 {
		t.Errorf("FCI not replaced: %v", after.ClassicalAnchor.FciExactEnergy)
	}
	if after.Notebook == nil || after.Notebook.ProvenanceHash == "" {
		t.Error("anchor fold must create a notebook with provenance")
	}
	if after.ClassicalAnchor.ChemicalAccuracyPassed {
		t.Error("anchor alone must not pass chemical accuracy: no VQE measurement exists")
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
	// The proposal has no computed anchor, so a pass withholds collapse —
	// amplitudes fold, status contests loudly.
	if after.Epistemics.Status != "CONTESTED" || after.Epistemics.AlphaCorroborated != 0.97 {
		t.Errorf("pass on an uncomputed anchor must withhold collapse: %+v", after.Epistemics)
	}
	if !strings.Contains(after.Epistemics.Statement, "UNCOMPUTED") {
		t.Errorf("withholding must name the cause: %q", after.Epistemics.Statement)
	}

	// Promote the anchor first, then the same pass verifies for real.
	updated, changed, _ = ReplanWithJobResult(raw, "job-anchor0", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H2","bond_distance":0.7414,"rhf_energy":-1.11668,"fci_exact_energy":-1.13727}`), "")
	if !changed {
		t.Fatal("anchor must fold")
	}
	updated, changed, _ = ReplanWithJobResult(updated, "job-claim1b", "claim_eval",
		[]byte(`{"claim_id":"c1","corroborated_amplitude":0.97,"contested_amplitude":0.02,"unverified_amplitude":0.05,"passes_chemical_accuracy":true,"verdict_reason":"within 0.4 mHa"}`), "")
	if !changed {
		t.Fatal("verdict must fold")
	}
	after = decodeReplan(t, updated)
	if after.Epistemics.Status != "COLLAPSED_VERIFIED" || after.Epistemics.AlphaCorroborated != 0.97 {
		t.Errorf("pass on computed anchor must verify: %+v", after.Epistemics)
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
	state := decodeReplan(t, raw)
	ensureNotebook(&state)
	state.Notebook.Sections = append(state.Notebook.Sections, "Narrative: campaign objective")
	narrativeSections := append([]string{}, state.Notebook.Sections...)
	narrative := len(narrativeSections)
	for i := 0; i < 60; i++ {
		state.Notebook.Sections = append(state.Notebook.Sections, "Job filler: old measurement")
	}
	full, _ := json.Marshal(state)

	updated, changed, _ := ReplanWithJobResult(full, "job-cap1", "circuit_run", []byte(`{"success":true,"meyer_wallach_entanglement":0.5,"resolved_backend":"aer","counts":{"00":50,"11":50}}`), "")
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
		for _, orig := range narrativeSections {
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

// A proposal carries no measured fields, even when a live anchor exists.
func TestBuildProposalCarriesNoMeasuredFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bridge pb.QuantumBridgeServiceClient
	}{
		{name: "uncomputed", bridge: nil},
		{name: "computed", bridge: stubAnchorBridge{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, err := BuildCampaignProposal(context.Background(), "sess-proposal", "H2 ground state VQE", "", tc.bridge)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Trajectory) != 0 || len(state.Branches) != 0 {
				t.Fatalf("proposal must not carry trajectory/branches: %+v", state)
			}
			if state.Notebook != nil || state.Epistemics != nil {
				t.Fatalf("proposal must not carry notebook/epistemics: %+v", state)
			}
			if state.ClassicalAnchor.VqeExpectationEnergy != 0 || state.ClassicalAnchor.ChemicalAccuracyPassed {
				t.Fatalf("proposal must not claim an energy/verdict: %+v", state.ClassicalAnchor)
			}
			if state.QuantumFactors.CliffordFidelity != 0 {
				t.Fatalf("proposal must not claim canary fidelity: %+v", state.QuantumFactors)
			}
			if !state.QuantumFactors.Synthesized {
				t.Error("proposal circuit must be marked synthesized")
			}
			if len(state.QuantumFactors.WireCuts) != 0 {
				t.Error("proposal must not carry planned cut overhead")
			}
		})
	}
}

// A recomputed anchor promotes the campaign to computed-grade without
// manufacturing epistemics: only a claim evaluation may create those.
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
	if after.Epistemics != nil {
		t.Errorf("anchor fold must not invent epistemics: %+v", after.Epistemics)
	}
	if after.Notebook == nil || after.Notebook.ProvenanceHash == "" {
		t.Error("anchor fold must record notebook provenance")
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
		t.Error("cut that produced nothing must not fold")
	}
	cutReal, cutChanged, _ := ReplanWithJobResult(raw, "job-v3b", "circuit_cut",
		[]byte(`{"success":true,"fragments":[{"fragment_id":1},{"fragment_id":2}]}`), "")
	if !cutChanged {
		t.Error("real bridge shape (fragments, no total_cuts) must fold with derived count")
	} else {
		afterCut := decodeReplan(t, cutReal)
		entry, _ := afterCut.QuantumFactors.WireCuts[0].(map[string]any)
		if entry["total_cuts"] != float64(2) {
			t.Errorf("count must derive from fragments: %+v", entry)
		}
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

func TestReplanCutPersistsMeasuredWireCuts(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, _ := ReplanWithJobResult(raw, "job-cut9", "circuit_cut",
		[]byte(`{"success":true,"total_cuts":2,"fragments":[{"fragment_id":0,"cut_qubits":[1]},{"fragment_id":1,"cut_qubits":[1]}]}`), "")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	if len(after.QuantumFactors.WireCuts) != 1 {
		t.Fatalf("expected 1 measured entry, got %+v", after.QuantumFactors.WireCuts)
	}
	entry, ok := after.QuantumFactors.WireCuts[0].(map[string]any)
	if !ok {
		t.Fatalf("entry wrong shape: %T", after.QuantumFactors.WireCuts[0])
	}
	if entry["measured"] != true || entry["job_id"] != "job-cut9" {
		t.Errorf("entry must be marked measured with job: %+v", entry)
	}
	if entry["total_cuts"] != float64(2) {
		t.Errorf("cut count wrong: %+v", entry)
	}
	frags, _ := entry["fragments"].([]any)
	if len(frags) != 2 {
		t.Errorf("fragments wrong: %+v", entry)
	}
	// A proposal carries no planned cut overhead at all.
	proposal := decodeReplan(t, raw)
	if len(proposal.QuantumFactors.WireCuts) != 0 {
		t.Errorf("proposal must not carry planned cut overhead: %+v", proposal.QuantumFactors.WireCuts)
	}
}

// A live anchor is a proposal fact, not a projection of results.
func TestBuildProposalLiveAnchorIsUnmeasured(t *testing.T) {
	state, err := BuildCampaignProposal(context.Background(), "sess-proj", "H2 ground state VQE", "", stubAnchorBridge{})
	if err != nil {
		t.Fatal(err)
	}
	if !state.ClassicalAnchor.AnchorComputed {
		t.Fatal("stub bridge must yield a computed anchor")
	}
	if state.ClassicalAnchor.VqeExpectationEnergy != 0 || state.ClassicalAnchor.DiscrepancyMHa != 0 {
		t.Errorf("proposal must not derive a VQE number: %+v", state.ClassicalAnchor)
	}
	if state.Epistemics != nil || state.Notebook != nil || len(state.Trajectory) != 0 {
		t.Errorf("proposal must not carry result fields: %+v", state)
	}
}

// stubAnchorBridge answers ComputeClassicalAnchor with fixed H2 numbers.
// All other RPCs nil-panic if touched — proposal construction must not touch them.
type stubAnchorBridge struct {
	pb.QuantumBridgeServiceClient
}

func (s stubAnchorBridge) ComputeClassicalAnchor(ctx context.Context, in *pb.ClassicalAnchorRequest, _ ...grpc.CallOption) (*pb.ClassicalAnchorResponse, error) {
	return &pb.ClassicalAnchorResponse{
		Success: true, Molecule: in.Molecule,
		RhfEnergy: -1.11668, FciExactEnergy: -1.13727,
	}, nil
}

func TestReplanContradictionCheck(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	// Pass flag with failing delta: evaluator contradicts its evidence.
	updated, changed, _ := ReplanWithJobResult(raw, "job-contra", "claim_eval",
		[]byte(`{"claim_id":"c","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"passes_chemical_accuracy":true,"delta_mha":3.2,"verdict_reason":"fine"}`), "")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "CONTRADICTION") || !strings.Contains(last, "3.20") {
		t.Errorf("mismatch must record loudly: %q", last)
	}
	// Consistent verdicts record no contradiction line.
	updated, _, _ = ReplanWithJobResult(raw, "job-ok", "claim_eval",
		[]byte(`{"claim_id":"c","corroborated_amplitude":0.9,"contested_amplitude":0.05,"unverified_amplitude":0.1,"passes_chemical_accuracy":true,"delta_mha":0.4,"verdict_reason":"fine"}`), "")
	after = decodeReplan(t, updated)
	for _, sec := range after.Notebook.Sections {
		if strings.Contains(sec, "CONTRADICTION") {
			t.Errorf("consistent verdict must not flag: %q", sec)
		}
	}
}

func TestReplanKrylovLabelsEmulation(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, _ := ReplanWithJobResult(raw, "job-k2", "krylov_solve",
		[]byte(`{"success":true,"ground_state_energy":-1.13699,"classical_fci_energy":-1.13727}`), "")
	if !changed {
		t.Fatal("expected fold")
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "classical emulation") {
		t.Errorf("krylov line must name the emulation: %q", last)
	}
}

// The molecule table supplies target geometry, never a borrowed energy.
func TestBuildProposalDoesNotCopyTableAnchor(t *testing.T) {
	for _, prompt := range []string{"H2 ground state", "LiH dissociation", "Benzene", "Caffeine"} {
		state, err := BuildCampaignProposal(context.Background(), "sess-defaults", prompt, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if state.ClassicalAnchor.AnchorComputed {
			t.Errorf("%q: nil bridge must stay uncomputed", prompt)
		}
		if state.ClassicalAnchor.RhfMeanFieldEnergy != 0 || state.ClassicalAnchor.FciExactEnergy != 0 {
			t.Errorf("%q: proposal borrowed table energies: %+v", prompt, state.ClassicalAnchor)
		}
	}
}

// J5: Meyer-Wallach folds only for real numerical solves. The analytical
// corridor path reports a placeholder 0.85 that must never overwrite a
// measured value; unattributed results (no solver field) are treated the
// same — a number without provenance is not a measurement.
func TestReplanKrylovMWFoldsOnlyWhenNumerical(t *testing.T) {
	fold := func(payload string) CampaignState {
		t.Helper()
		raw := replanTestState(t, "H2 ground state VQE")
		updated, changed, _ := ReplanWithJobResult(raw, "job-kmw", "krylov_solve", []byte(payload), "")
		if !changed {
			t.Fatal("krylov energy must fold")
		}
		return decodeReplan(t, updated)
	}
	before := decodeReplan(t, replanTestState(t, "H2 ground state VQE"))

	// Numerical solve folds MW.
	after := fold(`{"success":true,"ground_state_energy":-1.13699,"classical_fci_energy":-1.13727,"meyer_wallach_entanglement":0.81,"solver":"numerical-krylov"}`)
	if after.QuantumFactors.MeyerWallachQ != 0.81 {
		t.Errorf("numerical MW must fold, got %v", after.QuantumFactors.MeyerWallachQ)
	}
	// Analytical corridor never overwrites, energy still folds.
	after = fold(`{"success":true,"ground_state_energy":-7.83239,"classical_fci_energy":-7.88239,"meyer_wallach_entanglement":0.85,"solver":"analytical-sequence"}`)
	if after.QuantumFactors.MeyerWallachQ != before.QuantumFactors.MeyerWallachQ {
		t.Errorf("analytical placeholder must not fold, MW moved %v -> %v",
			before.QuantumFactors.MeyerWallachQ, after.QuantumFactors.MeyerWallachQ)
	}
	if n := len(after.Trajectory); n != len(before.Trajectory)+1 {
		t.Errorf("analytical energy must still fold to trajectory, len %d", n)
	}
	// Missing solver field: unattributed, MW skipped.
	after = fold(`{"success":true,"ground_state_energy":-1.13699,"classical_fci_energy":-1.13727,"meyer_wallach_entanglement":0.81}`)
	if after.QuantumFactors.MeyerWallachQ != before.QuantumFactors.MeyerWallachQ {
		t.Errorf("unattributed MW must not fold, MW moved %v -> %v",
			before.QuantumFactors.MeyerWallachQ, after.QuantumFactors.MeyerWallachQ)
	}
}

// W1.14: no-measurement folds nothing (no version spam); molecule matching
// is token-bound (LI never matches LIH); mitigation needs all three inputs;
// provenance hashes are full-precision.
func TestReplanW114Honesty(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")

	// circuit_run with no MW: no fold, no version.
	if _, changed, _ := ReplanWithJobResult(raw, "job-nm", "circuit_run",
		[]byte(`{"success":true,"resolved_backend":"aer","counts":{"00":50}}`), ""); changed {
		t.Error("measurement-less run must not fold (no version spam)")
	}
	// Mitigation missing unmitigated/error-bound refuses (no 0.0 printing).
	if _, changed, _ := ReplanWithJobResult(raw, "job-m2", "mitigate",
		[]byte(`{"success":true,"method_used":"ZNE","mitigated_value":-1.1371}`), ""); changed {
		t.Error("mitigation without unmitigated value must not fold")
	}
	if _, changed, _ := ReplanWithJobResult(raw, "job-m3", "mitigate",
		[]byte(`{"success":true,"method_used":"ZNE","unmitigated_value":-1.1,"mitigated_value":-1.1371}`), ""); changed {
		t.Error("mitigation without error bound must not fold")
	}
	// Token match: LIH target matches LIH, never LI.
	if !molTokenMatch("LIH CLUSTER", "LIH") {
		t.Error("LIH must token-match")
	}
	if molTokenMatch("LIH CLUSTER", "LI") {
		t.Error("LI must never token-match LIH")
	}
	// Full-precision provenance: bond distances 1e-6 apart hash apart.
	h1 := provenanceHash("s", "H2", 0.741400, -1.13727, "q;")
	h2 := provenanceHash("s", "H2", 0.741401, -1.13727, "q;")
	if h1 == h2 {
		t.Error("provenance must distinguish 1e-6 bond differences")
	}
}

func TestReplanVQERecordsMeasuredPointAndNotebook(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-vqe-1", "vqe_run",
		[]byte(`{"success":true,"energy":-1.1368,"energy_exact":-1.1368,"fci_exact_energy":-1.137270174660903,"delta_mha":0.470174660903,"uncertainty":0.002,"theta":[0.1,0.2],"chemical_accuracy":true,"trajectory":[{"iteration":1,"energy":-1.1},{"iteration":2,"energy":-1.1368}]}`),
		"clm-vqe-1")
	if !changed {
		t.Fatalf("VQE result must change state: %s", note)
	}
	var state CampaignState
	if err := json.Unmarshal(updated, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Trajectory) != 1 || state.Trajectory[0].Source != "measured" ||
		state.Trajectory[0].Watchdog != "measured_vqe" || state.Trajectory[0].Energy != -1.1368 {
		t.Fatalf("measured VQE trajectory wrong: %+v", state.Trajectory)
	}
	if state.ClassicalAnchor.VqeExpectationEnergy != -1.1368 ||
		!state.ClassicalAnchor.ChemicalAccuracyPassed {
		t.Fatalf("VQE anchor fields wrong: %+v", state.ClassicalAnchor)
	}
	if state.Measurement == nil || len(state.Measurement.OptimizerHistory) != 2 ||
		state.Measurement.OptimizerHistory[1].Energy != -1.1368 {
		t.Fatalf("optimizer history not retained: %+v", state.Measurement)
	}
	if state.Notebook == nil || state.Notebook.ClassicalAnchor == "not computed" ||
		state.Notebook.ProvenanceHash == "" || state.Notebook.QuantumEnergy != -1.1368 {
		t.Fatalf("notebook did not cite the measured VQE result: %+v", state.Notebook)
	}
}

func TestReplanVQERejectsCrossMoleculeAndIncompatibleFCI(t *testing.T) {
	anchored, err := BuildCampaignProposal(context.Background(), "sess-h2-anchored", "H2 ground state VQE", "", stubAnchorBridge{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(anchored)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Explicit wrong-molecule payload must be rejected cleanly.
	if _, changed, note := ReplanWithJobResult(raw, "job-lih-1", "vqe_run",
		[]byte(`{"success":true,"molecule":"LiH","energy":-7.86,"fci_exact_energy":-7.88236,"trajectory":[{"iteration":1,"energy":-7.86}]}`),
		"clm-lih-1"); changed || note != "vqe molecule mismatch" {
		t.Fatalf("expected cross-molecule VQE rejection, got changed=%v note=%q", changed, note)
	}

	// 2. Unattributed cross-molecule energy (LiH -7.88236 Ha on H2 -1.13727 Ha)
	// must be rejected via FCI compatibility check.
	if _, changed, note := ReplanWithJobResult(raw, "job-unattributed-lih", "vqe_repeats",
		[]byte(`{"success":true,"energy":-7.85,"fci_exact_energy":-7.88236,"trajectory":[{"iteration":1,"energy":-7.85}]}`),
		"clm-unattr-1"); changed || note != "vqe fci anchor mismatch" {
		t.Fatalf("expected incompatible FCI VQE rejection, got changed=%v note=%q", changed, note)
	}

	// 3. Even on a bridge-less proposal whose first VQE sets the campaign FCI
	// anchor, a later incompatible FCI trajectory must still be rejected.
	unanchored := replanTestState(t, "H2 ground state VQE")
	firstUpdated, changed, note := ReplanWithJobResult(unanchored, "job-h2-first", "vqe_run",
		[]byte(`{"success":true,"energy":-1.1368,"fci_exact_energy":-1.13727,"trajectory":[{"iteration":1,"energy":-1.1368}]}`),
		"clm-h2-1")
	if !changed {
		t.Fatalf("first unanchored VQE must fold cleanly: %s", note)
	}
	if _, changed, note := ReplanWithJobResult(firstUpdated, "job-unattributed-later", "vqe_repeats",
		[]byte(`{"success":true,"energy":-7.85,"fci_exact_energy":-7.88236,"trajectory":[{"iteration":1,"energy":-7.85}]}`),
		"clm-unattr-2"); changed || note != "vqe fci anchor mismatch" {
		t.Fatalf("expected subsequent incompatible FCI VQE rejection, got changed=%v note=%q", changed, note)
	}

	// 4. Legitimate stretched H2 geometry (-0.95 Ha vs -1.13727 Ha) stays within
	// tolerance and folds cleanly.
	updated, changed, note := ReplanWithJobResult(raw, "job-stretched-h2", "geometry_optimization",
		[]byte(`{"success":true,"molecule":"H2","energy":-0.945,"fci_exact_energy":-0.95,"trajectory":[{"iteration":1,"energy":-0.945}]}`),
		"clm-h2-stretch")
	if !changed {
		t.Fatalf("expected stretched H2 geometry_optimization to fold, got note=%q", note)
	}
	var state CampaignState
	if err := json.Unmarshal(updated, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Trajectory) != 1 || state.Trajectory[0].Energy != -0.945 {
		t.Fatalf("expected stretched H2 point folded cleanly, got %+v", state.Trajectory)
	}
}

func TestReplanPesHonorsExplicitUnclaimablePoint(t *testing.T) {
	raw := replanTestState(t, "LiH ground state VQE")
	updated, changed, _ := ReplanWithJobResult(raw, "job-sweep-claim", "vqe_pes",
		[]byte(`{"success":true,"molecule":"LiH","points":[
			{"bond_distance":1.5,"energy":-7.8816,"energy_exact":-7.8821,"fci_exact_energy":-7.88236,"delta_mha":0.7,"uncertainty":0.0005,"claimable":false,"theta":[0.1]}
		]}`), "")
	if !changed {
		t.Fatal("PES with explicit claimability must fold")
	}
	state := decodeReplan(t, updated)
	if len(state.PesPoints) != 1 {
		t.Fatalf("expected one point, got %+v", state.PesPoints)
	}
	if state.PesPoints[0].ChemicalAccuracy {
		t.Fatal("claimable:false must not become chemical accuracy from delta alone")
	}
}

func TestReplanMolecularJobsRejectCrossMoleculeAndIncompatibleFCI(t *testing.T) {
	anchored, err := BuildCampaignProposal(context.Background(), "sess-h2-scope", "H2 ground state VQE", "", stubAnchorBridge{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(anchored)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		jobType string
		payload string
		want    string
	}{
		{
			name:    "vqe_pes explicit wrong molecule",
			jobType: "vqe_pes",
			payload: `{"success":true,"molecule":"LiH","points":[{"bond_distance":1.5,"energy":-7.8816,"fci_exact_energy":-7.88236,"delta_mha":0.7}]}`,
			want:    "pes molecule mismatch",
		},
		{
			name:    "vqe_pes unattributed incompatible FCI",
			jobType: "vqe_pes",
			payload: `{"success":true,"points":[{"bond_distance":1.5,"energy":-7.8816,"fci_exact_energy":-7.88236,"delta_mha":0.7}]}`,
			want:    "pes fci anchor mismatch",
		},
		{
			name:    "hardware_pes explicit wrong molecule",
			jobType: "hardware_pes",
			payload: `{"success":true,"molecule":"LiH","backend":"ibm_fez","points":[{"bond_distance":1.5,"energy":-7.85,"fci_exact_energy":-7.88236,"delta_mha":32.0,"claimable":false}]}`,
			want:    "hardware_pes molecule mismatch",
		},
		{
			name:    "hardware_pes unattributed incompatible FCI",
			jobType: "hardware_pes",
			payload: `{"success":true,"backend":"ibm_fez","points":[{"bond_distance":1.5,"energy":-7.85,"fci_exact_energy":-7.88236,"delta_mha":32.0,"claimable":false}]}`,
			want:    "hardware_pes fci anchor mismatch",
		},
		{
			name:    "krylov explicit wrong molecule",
			jobType: "krylov_solve",
			payload: `{"success":true,"molecule":"LiH","ground_state_energy":-7.881,"classical_fci_energy":-7.88236}`,
			want:    "krylov molecule mismatch",
		},
		{
			name:    "krylov unattributed incompatible FCI",
			jobType: "krylov_solve",
			payload: `{"success":true,"ground_state_energy":-7.881,"classical_fci_energy":-7.88236}`,
			want:    "krylov fci anchor mismatch",
		},
		{
			name:    "reaction explicit wrong molecule",
			jobType: "reaction",
			payload: `{"success":true,"molecule":"LiH","reaction_energy":0.1971,"reaction_energy_exact":0.2041}`,
			want:    "reaction molecule mismatch",
		},
		{
			name:    "excited_state explicit wrong molecule",
			jobType: "excited_state",
			payload: `{"success":true,"molecule":"LiH","ground_energy":-7.85,"ground_energy_exact":-7.88236,"excited_energy":-7.41,"gap":0.44}`,
			want:    "excited_state molecule mismatch",
		},
		{
			name:    "excited_state unattributed incompatible FCI",
			jobType: "excited_state",
			payload: `{"success":true,"ground_energy":-7.85,"ground_energy_exact":-7.88236,"excited_energy":-7.41,"gap":0.44}`,
			want:    "excited_state fci anchor mismatch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, changed, note := ReplanWithJobResult(raw, "job-scope-"+tc.jobType, tc.jobType, []byte(tc.payload), ""); changed || note != tc.want {
				t.Fatalf("expected changed=false note=%q, got changed=%v note=%q", tc.want, changed, note)
			}
		})
	}
}

func TestReplanSpinChainFoldsMeasuredEnergyWithoutChemicalClaim(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-spin-1", "spin_chain",
		[]byte(`{"success":true,"kind":"spin_chain","model":"tfim","n_sites":4,"energy":-3.2,"energy_exact":-3.21,"uncertainty":0.004,"gap":0.8,"claimable":false,"backend":"qiskit-aer-simulator"}`), "")
	if !changed {
		t.Fatalf("spin-chain measurement must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	if after.Notebook == nil || !strings.Contains(after.Notebook.Sections[len(after.Notebook.Sections)-1], "spin_chain") {
		t.Error("spin-chain result must be recorded in the notebook")
	}
	if len(after.Experiments) != 1 || after.Experiments[0].Kind != "spin_chain" || after.Experiments[0].Model != "tfim" {
		t.Errorf("spin-chain typed projection wrong: %+v", after.Experiments)
	}
	for _, step := range after.Trajectory {
		if step.Watchdog == "measured_spin_chain" {
			t.Error("spin-chain energy must not be projected as a molecular VQE trajectory")
		}
	}
}

func TestReplanThermodynamicLimitKeepsDMRGClassical(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-thermo-1", "thermodynamic_limit",
		[]byte(`{"success":true,"kind":"thermodynamic_limit","model":"tfim","bond_dim":32,"finite_size":[{"n_sites":8,"energy":-9.8,"energy_per_site":-1.23},{"n_sites":12,"energy":-15.0,"energy_per_site":-1.25}],"thermodynamic_limit":{"available":true,"energy_per_site":-1.27324}}`), "")
	if !changed {
		t.Fatalf("thermodynamic limit must be recorded: %s", note)
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "thermodynamic_limit") || !strings.Contains(last, "measured=false") {
		t.Errorf("DMRG note must keep its classical/measurement boundary: %q", last)
	}
	if len(after.Experiments) != 1 || after.Experiments[0].Kind != "thermodynamic_limit" || after.Experiments[0].Measured {
		t.Errorf("DMRG typed projection wrong: %+v", after.Experiments)
	}
}

func TestReplanConjectureRecordsCandidatesWithoutClaim(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-conj-1", "conjecture_run",
		[]byte(`{"success":true,"kind":"conjecture","candidate_count":1,"status":"conjecture","candidates":[{"law":"a(n) = a(n-1) + a(n-2)","status":"conjecture"}]}`), "")
	if !changed {
		t.Fatalf("conjecture must be recorded: %s", note)
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "conjecture") || !strings.Contains(last, "not a theorem") {
		t.Errorf("conjecture note must keep its epistemic limit: %q", last)
	}
	if after.Epistemics != nil && after.Epistemics.Status == "COLLAPSED_VERIFIED" {
		t.Error("a finite conjecture must never collapse to verified")
	}
	if len(after.Experiments) != 1 || after.Experiments[0].CandidateCount != 1 {
		t.Errorf("conjecture typed projection wrong: %+v", after.Experiments)
	}
}

func TestReplanSpinChainHardwareRecordsVendorReceipt(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-spin-hw-1", "spin_chain_hardware",
		[]byte(`{"success":true,"kind":"spin_chain_hardware","model":"tfim","n_sites":4,"energy":-3.1,"energy_exact":-3.2,"uncertainty":0.02,"delta":0.1,"claimable":false,"measured":true,"backend":"ibm_fez","hardware_job_id":"vendor-1"}`), "")
	if !changed {
		t.Fatalf("hardware result must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "spin_chain_hardware") || !strings.Contains(last, "vendor-1") {
		t.Errorf("hardware receipt not recorded: %q", last)
	}
}

func TestReplanExcitedStateRecordsGapWithoutChemicalClaim(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-exc-1", "excited_state",
		[]byte(`{"success":true,"kind":"excited_state","ground_energy":-1.1346,"ground_energy_exact":-1.13727,"excited_energy":-0.5312,"excited_energy_exact":-0.53248,"gap":0.6034,"gap_exact":0.60479,"uncertainty":0.0053,"claimable":false}`), "")
	if !changed {
		t.Fatalf("excited state must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "excited_state") || !strings.Contains(last, "gap") {
		t.Errorf("excited-state note wrong: %q", last)
	}
}

func TestReplanReactionEnergyRecordsDifference(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-rxn-1", "reaction",
		[]byte(`{"success":true,"kind":"reaction","reaction_energy":0.1971,"reaction_energy_exact":0.2041,"uncertainty":0.0048,"claimable":false}`), "")
	if !changed {
		t.Fatalf("reaction must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	last := after.Notebook.Sections[len(after.Notebook.Sections)-1]
	if !strings.Contains(last, "reaction") || !strings.Contains(last, "0.197100") {
		t.Errorf("reaction note wrong: %q", last)
	}
}

func TestReplanPreservesCanonicalReceiptReference(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-receipt-1", "spin_chain",
		[]byte(`{"success":true,"kind":"spin_chain","result_version":"experiment-result-v1","model":"tfim","n_sites":4,"energy":-3.2,"energy_exact":-3.21,"uncertainty":0.004,"gap":0.8,"claimable":false,"result_receipt":{"algorithm":"ed25519","ledger_entry_hash":"ledger-1","result_sha256":"result-1","signature":"sig","signed":true}}`), "")
	if !changed {
		t.Fatalf("receipt-bearing result must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	if len(after.ResultReceipts) != 1 {
		t.Fatalf("expected one canonical receipt ref, got %+v", after.ResultReceipts)
	}
	ref := after.ResultReceipts[0]
	if ref.JobID != "job-receipt-1" || ref.ResultVersion != "experiment-result-v1" ||
		ref.Receipt == nil || !ref.Receipt.Signed || ref.Receipt.LedgerEntryHash != "ledger-1" {
		t.Fatalf("receipt projection wrong: %+v", ref)
	}
	if after.Experiments[0].ResultVersion != "experiment-result-v1" {
		t.Fatalf("experiment version not preserved: %+v", after.Experiments[0])
	}
}

func TestRecordJobFailureCreatesNegativeResultWithoutClaim(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := RecordJobFailure(raw, "job-failed-1", "spin_chain", "bridge unavailable")
	if !changed {
		t.Fatalf("failure must be recorded: %s", note)
	}
	after := decodeReplan(t, updated)
	if len(after.Experiments) != 1 {
		t.Fatalf("expected one negative result, got %+v", after.Experiments)
	}
	exp := after.Experiments[0]
	if exp.JobID != "job-failed-1" || exp.Status != "failed" || exp.Error != "bridge unavailable" ||
		exp.Measured || exp.Claimable {
		t.Fatalf("negative result projection wrong: %+v", exp)
	}
	if after.Notebook == nil || !strings.Contains(after.Notebook.Sections[len(after.Notebook.Sections)-1], "FAILED") {
		t.Fatalf("failure must be visible in the notebook: %+v", after.Notebook)
	}
	if _, again, _ := RecordJobFailure(updated, "job-failed-1", "spin_chain", "duplicate"); again {
		t.Fatal("duplicate failure must be idempotent")
	}
}

func TestReplanKrylovJsonMitigateJsonAndQAOAExperimentFold(t *testing.T) {
	raw := replanTestState(t, "H2 ground state VQE")
	updated, changed, note := ReplanWithJobResult(raw, "job-kjson-1", "krylov_json",
		[]byte(`{"success":true,"ground_state_energy":-1.13701,"classical_fci_energy":-1.13727,"meyer_wallach_entanglement":0.82,"solver":"numerical-krylov"}`), "")
	if !changed {
		t.Fatalf("krylov_json must fold: %s", note)
	}
	updated, changed, note = ReplanWithJobResult(updated, "job-mjson-1", "mitigate_json",
		[]byte(`{"success":true,"method_used":"ZNE","unmitigated_value":-1.110,"mitigated_value":-1.1369,"error_bound":0.002}`), "")
	if !changed {
		t.Fatalf("mitigate_json must fold: %s", note)
	}
	updated, changed, note = ReplanWithJobResult(updated, "job-qaoa-1", "qaoa_run",
		[]byte(`{"success":true,"kind":"qaoa","measured_cut":3.85,"optimal_cut":4,"approximation_ratio":0.9625,"claimable":true,"backend":"qiskit-aer-simulator"}`), "")
	if !changed {
		t.Fatalf("qaoa_run must fold: %s", note)
	}
	after := decodeReplan(t, updated)
	if len(after.Experiments) != 1 || after.Experiments[0].Kind != "qaoa_run" ||
		after.Experiments[0].Energy == nil || *after.Experiments[0].Energy != 3.85 ||
		after.Experiments[0].EnergyExact == nil || *after.Experiments[0].EnergyExact != 4.0 {
		t.Fatalf("qaoa_run experiment projection wrong: %+v", after.Experiments)
	}
}
