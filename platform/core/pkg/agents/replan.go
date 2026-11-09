package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Planner replan: campaigns start as unmeasured proposals. This file folds
// settled job results into stored state, so the plan learns what actually
// happened instead of carrying a template result.
//
// Purity is the point. Replan runs on a job-worker goroutine, so it performs
// no LLM calls, no bridge calls, no ledger access. It decodes, it folds only
// what was actually measured, and it reports whether anything changed. The
// caller saves a version only when changed=true — a version with no change
// is audit spam against the 50-version cap.
//
// Honesty rules, per job type (all keys snake_case, matching the marshaled
// proto responses; omitempty means absent keys are unknown, never zero):
//   - circuit_run: only the measured Meyer-Wallach entanglement folds into
//     state. Counts have no state field; energy is NOT derivable from counts
//     and is never fabricated into the trajectory.
//   - krylov_solve: ground_state_energy is a real computed energy, so it
//     appends a trajectory point — watchdog-tagged "measured_krylov" so no
//     consumer mistakes it for an optimizer step. Theta stays empty (those
//     parameters were never run); FlightRecorder renders empty theta.
//   - classical_anchor: recomputed RHF/FCI replace the anchor; discrepancy
//     and chemical-accuracy re-derive mechanically; provenance re-hashes.
//     Skipped on molecule mismatch — a wrong-molecule anchor corrupts.
//   - claim_eval: evaluated amplitudes create the epistemic state (none is
//     fabricated at proposal time); verdict pass/fail drives status. No
//     success field exists on this response; presence of amplitude keys is
//     the validity signal.
//   - mitigate / circuit_cut: notebook provenance lines only; no state
//     field honestly holds them.
// Failed jobs never reach the fold (the hook fires on COMPLETED only).

// maxNotebookSections bounds live-row growth: replans append measurement
// lines, and an unbounded campaign row would bloat every fetch. Narrative
// sections are never dropped; oldest measurement lines fall off first.
const maxNotebookSections = 40

// ClaimDraft is an evaluated verdict awaiting a record ID. The caller mints
// the record, then hands the ID back to the fold as cite so the notebook
// line cites the record that backs it.
type ClaimDraft struct {
	Statement  string
	Alpha      float64
	Beta       float64
	Gamma      float64
	Verdict    string // "verified" or "contested"
	ExternalID string // the submitter's claim_id, echoed back by the bridge
	Reason     string
}

// EvalClaimRecord parses a claim_eval result into a record draft. It applies
// the same validity rule as the fold (amplitude keys present) so a record is
// never minted for a verdict the fold would refuse.
func EvalClaimRecord(resultJSON []byte) (*ClaimDraft, bool) {
	var res map[string]json.RawMessage
	if err := json.Unmarshal(resultJSON, &res); err != nil {
		return nil, false
	}
	get := func(key string) (float64, bool) {
		raw, ok := res[key]
		if !ok {
			return -1, false
		}
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return -1, false
		}
		return v, true
	}
	alpha, okA := get("corroborated_amplitude")
	beta, okB := get("contested_amplitude")
	gamma, okG := get("unverified_amplitude")
	if !okA && !okB && !okG {
		return nil, false
	}
	if !okA || alpha < 0 {
		alpha = 0.0
	}
	if !okB || beta < 0 {
		beta = 0.0
	}
	if !okG || gamma < 0 {
		gamma = 0.0
	}
	var passes bool
	rawPass, ok := res["passes_chemical_accuracy"]
	if !ok {
		return nil, false
	}
	if err := json.Unmarshal(rawPass, &passes); err != nil {
		return nil, false
	}
	if rawSuccess, ok := res["success"]; ok {
		var succ bool
		if err := json.Unmarshal(rawSuccess, &succ); err == nil && !succ {
			passes = false
		}
	}
	var reason, external string
	if raw, ok := res["verdict_reason"]; ok {
		_ = json.Unmarshal(raw, &reason)
	}
	if raw, ok := res["claim_id"]; ok {
		_ = json.Unmarshal(raw, &external)
	}
	verdict := "contested"
	if passes {
		verdict = "verified"
	}
	return &ClaimDraft{Statement: boundClaim(reason), Alpha: alpha, Beta: beta, Gamma: gamma,
		Verdict: verdict, ExternalID: external, Reason: boundClaim(reason)}, true
}

// molTokenMatch reports whether mol appears as a whole token in target
// (split on non-alphanumerics): "LIH" matches "LIH cluster" but "LI"
// never matches "LIH".
func molTokenMatch(targetUpper, molNorm string) bool {
	for _, tok := range strings.FieldsFunc(targetUpper, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		if tok == molNorm {
			return true
		}
	}
	return false
}

// ReplanWithJobResult folds one settled job into stored campaign JSON.
// cite carries a claim record ID for the notebook line ("" = no citation).
func ReplanWithJobResult(stateJSON []byte, jobID, jobType string, resultJSON []byte, cite string) (updated []byte, changed bool, note string) {
	var state CampaignState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, false, "campaign undecodable"
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(resultJSON, &res); err != nil {
		return nil, false, "result undecodable"
	}
	shortID := jobID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	present := func(key string) ([]byte, bool) {
		raw, ok := res[key]
		return raw, ok
	}
	successTrue := func() bool {
		raw, ok := present("success")
		if !ok {
			return false
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return false
		}
		return b
	}
	measureLine := func(format string, args ...any) {
		nb := ensureNotebook(&state)
		nb.Sections = append(nb.Sections,
			"Job "+shortID+" ("+jobType+"): "+fmt.Sprintf(format, args...))
		trimMeasurementLines(&state)
	}

	switch jobType {
	case "circuit_run":
		if !successTrue() {
			return nil, false, "run reported success=false"
		}
		changedAny := false
		if raw, ok := present("meyer_wallach_entanglement"); ok {
			var mw float64
			if err := json.Unmarshal(raw, &mw); err == nil && mw > 0 {
				state.QuantumFactors.MeyerWallachQ = mw
				changedAny = true
			}
		}
		if !changedAny {
			// W1.14: nothing measured folded, nothing changed. The old
			// code forced changed=true for the provenance line alone —
			// version spam against the 50-version cap (the file's own
			// purity rule: no change, no version). The run itself stays
			// recorded on the job row and events.
			return marshalReplan(&state, false, "circuit measurements folded (no state change)")
		}
		backend := "unknown backend"
		if raw, ok := present("resolved_backend"); ok {
			var b string
			if err := json.Unmarshal(raw, &b); err == nil && b != "" {
				backend = b
			}
		}
		shots := int64(0)
		if raw, ok := present("counts"); ok {
			var counts map[string]int64
			if err := json.Unmarshal(raw, &counts); err == nil {
				for _, c := range counts {
					shots += c
				}
			}
		}
		measureLine("executed on %s, %d shots, measured MW=%.4f.", backend, shots, state.QuantumFactors.MeyerWallachQ)
		return marshalReplan(&state, true, "circuit measurements folded")

	case "krylov_solve":
		if !successTrue() {
			return nil, false, "krylov reported success=false"
		}
		rawE, ok := present("ground_state_energy")
		if !ok {
			return nil, false, "no ground_state_energy measured"
		}
		var energy float64
		if err := json.Unmarshal(rawE, &energy); err != nil || energy == 0 {
			return nil, false, "ground_state_energy unusable"
		}
		fci := state.ClassicalAnchor.FciExactEnergy
		state.Trajectory = append(state.Trajectory, TrajectoryStep{
			Step:      len(state.Trajectory) + 1,
			Energy:    energy,
			Loss:      math.Abs(energy - fci),
			Theta:     []float64{},
			GradNorm:  0,
			Watchdog:  "measured_krylov",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Source:    "measured",
		})
		if raw, ok := present("meyer_wallach_entanglement"); ok {
			var mw float64
			// J5: MW folds only for real numerical solves. The analytical
			// corridor path reports a placeholder 0.85 that must never
			// overwrite a measured value; unattributed results (no solver
			// field, e.g. pre-J5 rows) are treated the same way — a number
			// without provenance is not a measurement.
			var solver string
			if rawS, ok := present("solver"); ok {
				_ = json.Unmarshal(rawS, &solver)
			}
			if err := json.Unmarshal(raw, &mw); err == nil && mw > 0 && solver == "numerical-krylov" {
				state.QuantumFactors.MeyerWallachQ = mw
			}
		}
		measureLine("Krylov ground state E=%.5f Ha (|E-FCI|=%.5f Ha; bridge-computed classical emulation, no quantum execution).", energy, math.Abs(energy-fci))
		return marshalReplan(&state, true, "krylov energy folded into trajectory")

	case "classical_anchor":
		if !successTrue() {
			return nil, false, "anchor reported success=false"
		}
		var mol string
		if raw, ok := present("molecule"); ok {
			_ = json.Unmarshal(raw, &mol)
		}
		// Molecule identity is exact against the campaign formula
		// ("H2" == "H2"), with token-boundary match against the display
		// target only as a fallback for formula-less states (W1.14: the old
		// substring fallback let "LI" match "LIH" — a wrong-molecule anchor
		// corrupts everything downstream). Codes shorter than two
		// characters never match at all.
		molNorm := strings.ToUpper(strings.TrimSpace(mol))
		matched := false
		if len(molNorm) >= 2 {
			matched = strings.EqualFold(
				strings.TrimSpace(state.QuantumFactors.MolecularFormula), mol)
			if !matched {
				targetUpper := strings.ToUpper(state.QuantumFactors.TargetMolecule)
				matched = molTokenMatch(targetUpper, molNorm)
			}
		}
		if !matched {
			return nil, false, "anchor molecule mismatch"
		}
		rawRHF, okR := present("rhf_energy")
		rawFCI, okF := present("fci_exact_energy")
		if !okR || !okF {
			return nil, false, "anchor energies absent"
		}
		var rhf, fci float64
		if err := json.Unmarshal(rawRHF, &rhf); err != nil {
			return nil, false, "rhf undecodable"
		}
		if err := json.Unmarshal(rawFCI, &fci); err != nil || fci == 0 {
			return nil, false, "fci unusable"
		}
		oldFCI := state.ClassicalAnchor.FciExactEnergy
		state.ClassicalAnchor.RhfMeanFieldEnergy = rhf
		state.ClassicalAnchor.FciExactEnergy = fci
		// A recomputed anchor is measured evidence: the campaign's anchor
		// becomes computed-grade here, even if synthesis ran on fallback.
		state.ClassicalAnchor.AnchorComputed = true
		state.ClassicalAnchor.AnchorSource = "pyscf-live"
		state.ClassicalAnchor.Method = "PySCF STO-3G Full Configuration Interaction"
		delta := 0.0
		if state.ClassicalAnchor.VqeExpectationEnergy != 0 {
			delta = math.Abs(state.ClassicalAnchor.VqeExpectationEnergy-fci) * 1000.0
			state.ClassicalAnchor.DiscrepancyMHa = delta
			state.ClassicalAnchor.ChemicalAccuracyPassed = delta <= 1.6
		} else {
			state.ClassicalAnchor.DiscrepancyMHa = 0
			state.ClassicalAnchor.ChemicalAccuracyPassed = false
		}
		nb := ensureNotebook(&state)
		nb.ClassicalAnchor = fmt.Sprintf("PySCF STO-3G Full Configuration Interaction (%.5f Ha)", fci)
		nb.ProvenanceHash = provenanceHash(state.SessionID,
			state.QuantumFactors.TargetMolecule, state.QuantumFactors.BondDistance, fci,
			state.QuantumFactors.QASM)
		// Legacy fallback repair remains for already-stored pre-Phase-1
		// campaigns. New campaigns start with no epistemics at all, so a
		// computed anchor alone never manufactures projection weights.
		if ep := state.Epistemics; ep != nil && strings.Contains(ep.Statement, strings.TrimSpace(fallbackCaveat)) {
			ep.Statement = strings.ReplaceAll(ep.Statement, fallbackCaveat, "")
			ep.Sources = synthSources(true)
			if ep.Status == "UNVERIFIED" {
				ep.Status = "PROJECTED"
				ep.AlphaCorroborated = liveAlpha
				ep.BetaContested = liveBeta
				ep.GammaUnverified = liveGamma
				ep.PCorrelated = livePCorrelated
			}
		}
		measureLine("anchor recomputed: FCI %.5f -> %.5f Ha, discrepancy %.2f mHa.", oldFCI, fci, delta)
		return marshalReplan(&state, true, "classical anchor recomputed")

	case "vqe_run", "hardware_experiment":
		if !successTrue() {
			return nil, false, "vqe reported success=false"
		}
		var energy, fci, unc, delta, exact, activeFci, activeDelta float64
		var theta []float64
		var shots, seed int
		var method, ansatz, backend, resultVersion, provider, optimizerBackend, measurementBackend string
		var hardware bool
		measuredFlag := true
		claimableFlag := false
		claimablePresent := false
		activeClaimable := false
		activeClaimablePresent := false
		var ci95, usageSeconds float64
		var actualUSD *float64
		var layout, calibration map[string]any
		var counts map[string]int
		var evidenceRefs []string
		for key, dst := range map[string]*float64{
			"energy": &energy, "fci_exact_energy": &fci,
			"uncertainty": &unc, "delta_mha": &delta,
			"energy_exact": &exact, "active_space_fci_energy": &activeFci,
			"active_space_delta_mha": &activeDelta,
		} {
			if raw, ok := present(key); ok {
				var v float64
				if err := json.Unmarshal(raw, &v); err == nil {
					*dst = v
				}
			}
		}
		if energy == 0 || fci == 0 {
			return nil, false, "vqe energy/anchor absent"
		}
		if raw, ok := present("theta"); ok {
			_ = json.Unmarshal(raw, &theta)
		}
		if raw, ok := present("shots"); ok {
			_ = json.Unmarshal(raw, &shots)
		}
		if raw, ok := present("seed"); ok {
			_ = json.Unmarshal(raw, &seed)
		}
		if raw, ok := present("method"); ok {
			_ = json.Unmarshal(raw, &method)
		}
		if raw, ok := present("ansatz"); ok {
			_ = json.Unmarshal(raw, &ansatz)
		}
		if raw, ok := present("backend"); ok {
			_ = json.Unmarshal(raw, &backend)
		}
		for key, dst := range map[string]*string{
			"result_version": &resultVersion, "provider": &provider,
			"optimizer_backend": &optimizerBackend, "measurement_backend": &measurementBackend,
		} {
			if raw, ok := present(key); ok {
				_ = json.Unmarshal(raw, dst)
			}
		}
		if raw, ok := present("hardware"); ok {
			_ = json.Unmarshal(raw, &hardware)
		}
		if raw, ok := present("measured"); ok {
			_ = json.Unmarshal(raw, &measuredFlag)
		}
		if raw, ok := present("claimable"); ok {
			if json.Unmarshal(raw, &claimableFlag) == nil {
				claimablePresent = true
			}
		}
		if raw, ok := present("active_space_claimable"); ok {
			if json.Unmarshal(raw, &activeClaimable) == nil {
				activeClaimablePresent = true
			}
		} else if raw, ok := present("active_space_chemical_accuracy"); ok {
			if json.Unmarshal(raw, &activeClaimable) == nil {
				activeClaimablePresent = true
			}
		}
		if raw, ok := present("ci95_mha"); ok {
			_ = json.Unmarshal(raw, &ci95)
		}
		if raw, ok := present("usage_seconds"); ok {
			_ = json.Unmarshal(raw, &usageSeconds)
		}
		if raw, ok := present("actual_usd"); ok {
			_ = json.Unmarshal(raw, &actualUSD)
		}
		if raw, ok := present("layout"); ok {
			_ = json.Unmarshal(raw, &layout)
		}
		if raw, ok := present("calibration"); ok {
			_ = json.Unmarshal(raw, &calibration)
		}
		if raw, ok := present("counts"); ok {
			_ = json.Unmarshal(raw, &counts)
		}
		if raw, ok := present("evidence_refs"); ok {
			_ = json.Unmarshal(raw, &evidenceRefs)
		}
		measuredDelta := math.Abs(energy-fci) * 1000.0
		if delta == 0 {
			delta = measuredDelta
		}
		if !state.ClassicalAnchor.AnchorComputed && fci != 0 {
			// A VQE runner returns the anchor it used. Record it as computed
			// evidence only when the result actually carried it.
			state.ClassicalAnchor.FciExactEnergy = fci
			state.ClassicalAnchor.AnchorComputed = true
			state.ClassicalAnchor.AnchorSource = "vqe-run"
			state.ClassicalAnchor.Method = "PySCF STO-3G Full Configuration Interaction"
		}
		chemicalAccuracy := delta <= 1.6
		if claimablePresent {
			chemicalAccuracy = claimableFlag
		}
		state.ClassicalAnchor.VqeExpectationEnergy = energy
		state.ClassicalAnchor.DiscrepancyMHa = delta
		state.ClassicalAnchor.ChemicalAccuracyPassed = chemicalAccuracy
		state.Trajectory = append(state.Trajectory, TrajectoryStep{
			Step: len(state.Trajectory) + 1, Energy: energy, Loss: math.Abs(energy - fci),
			Theta: theta, GradNorm: 0, Watchdog: "measured_vqe",
			Timestamp: time.Now().UTC().Format(time.RFC3339), Source: "measured",
		})
		nb := ensureNotebook(&state)
		if state.ClassicalAnchor.AnchorComputed {
			nb.ClassicalAnchor = fmt.Sprintf("PySCF STO-3G Full Configuration Interaction (%.5f Ha)", fci)
			nb.ProvenanceHash = provenanceHash(state.SessionID,
				state.QuantumFactors.TargetMolecule, state.QuantumFactors.BondDistance,
				fci, state.QuantumFactors.QASM)
		}
		nb.QuantumEnergy = energy
		nb.ClassicalEnergy = fci
		nb.DeltaMHa = delta
		nb.ChemicalAccuracy = chemicalAccuracy
		if nb.ChemicalAccuracy {
			nb.EpistemicVerdict = "VERIFIED"
		} else {
			nb.EpistemicVerdict = "CONTESTED"
		}
		// Keep the campaign's proof projection in step with the measured
		// verdict. Amplitudes stay unknown (no invented alpha/beta/gamma);
		// the claim record and the measured statement carry the evidence.
		ep := ensureEpistemics(&state)
		ep.Status = nb.EpistemicVerdict
		if nb.ChemicalAccuracy {
			ep.Status = "COLLAPSED_VERIFIED"
		}
		ep.Statement = fmt.Sprintf("VQE measured %.6f Ha vs FCI %.6f Ha (Δ=%.3f mHa).",
			energy, fci, delta)
		sourceKind := jobType
		if sourceKind == "" {
			sourceKind = "vqe_run"
		}
		ep.Sources = []string{"bridge:" + sourceKind + ":" + jobID}
		// Amplitudes were not evaluated by this VQE; use the explicit
		// not-evaluated sentinel rather than zeroes that render as certainty.
		ep.AlphaCorroborated = -1
		ep.BetaContested = -1
		ep.GammaUnverified = -1
		ep.PCorrelated = -1
		ep.LiteratureDrift = -1
		var optimizerHistory []OptimizerPoint
		if raw, ok := present("trajectory"); ok {
			var path []struct {
				Iteration int     `json:"iteration"`
				Energy    float64 `json:"energy"`
			}
			if json.Unmarshal(raw, &path) == nil {
				for _, p := range path {
					optimizerHistory = append(optimizerHistory, OptimizerPoint{Iteration: p.Iteration, Energy: p.Energy})
				}
			}
		}
		state.Measurement = &MeasurementSummary{
			JobID: jobID, ClaimID: cite, Method: method, Ansatz: ansatz,
			Backend: backend, Energy: energy, EnergyExact: exact,
			FCIEnergy: fci, ActiveSpaceFCIEnergy: activeFci,
			UncertaintyMHa: unc * 1000.0, DeltaMHa: delta,
			ActiveSpaceDeltaMHa: activeDelta, Shots: shots, Seed: seed,
			ChemicalAccuracy: chemicalAccuracy, Claimable: chemicalAccuracy,
			ActiveSpaceChemicalAcc: func() bool {
				if activeClaimablePresent {
					return activeClaimable
				}
				return activeDelta > 0 && activeDelta <= 1.6
			}(),
			Converged: false, Measured: measuredFlag, BelowFCI: energy < fci,
			Source: "shot-sampled Pauli expectation", OptimizerHistory: optimizerHistory,
			ResultVersion: resultVersion, Provider: provider, Hardware: hardware,
			OptimizerBackend: optimizerBackend, MeasurementBackend: measurementBackend,
			CI95MHa: ci95, UsageSeconds: usageSeconds, ActualUSD: actualUSD,
			Layout: layout, Calibration: calibration, Counts: counts, EvidenceRefs: evidenceRefs,
		}
		if raw, ok := present("converged"); ok {
			_ = json.Unmarshal(raw, &state.Measurement.Converged)
		}
		measureLine("VQE measured E=%.6f Ha ± %.6f Ha vs FCI %.6f Ha (Δ=%.3f mHa; %s).%s",
			energy, unc, fci, delta, map[bool]string{true: "within", false: "outside"}[nb.ChemicalAccuracy]+" chemical accuracy", citeSuffix(cite))
		return marshalReplan(&state, true, "VQE energy folded into trajectory")

	case "vqe_pes":
		if !successTrue() {
			return nil, false, "PES reported success=false"
		}
		var points []struct {
			BondDistance    float64   `json:"bond_distance"`
			Energy          float64   `json:"energy"`
			EnergyExact     float64   `json:"energy_exact"`
			Fci             float64   `json:"fci_exact_energy"`
			ActiveSpaceFCI  float64   `json:"active_space_fci_energy"`
			DeltaMHa        float64   `json:"delta_mha"`
			ExactDeltaMHa   float64   `json:"exact_delta_mha"`
			Uncertainty     float64   `json:"uncertainty"`
			CI95MHa         float64   `json:"ci95_mha"`
			Theta           []float64 `json:"theta"`
			Claimable       *bool     `json:"claimable,omitempty"`
			FullSpaceStatus string    `json:"full_space_status,omitempty"`
			Method          string    `json:"method,omitempty"`
		}
		rawPoints, ok := present("points")
		if !ok || json.Unmarshal(rawPoints, &points) != nil || len(points) == 0 {
			return nil, false, "PES points absent"
		}
		best := points[0]
		bestClaimable := false
		bestClaimableDelta := math.Inf(1)
		for _, p := range points {
			if math.Abs(p.DeltaMHa) < math.Abs(best.DeltaMHa) {
				best = p
			}
			chemicalAccuracy := math.Abs(p.DeltaMHa) <= 1.6
			if p.Claimable != nil {
				// Full-space/uncertainty-gated points may report a measured
				// delta inside 1.6 mHa while the exact diagnostic or shot
				// uncertainty still refuses the claim. Never override that.
				chemicalAccuracy = *p.Claimable
				if chemicalAccuracy && math.Abs(p.DeltaMHa) < bestClaimableDelta {
					bestClaimable = true
					bestClaimableDelta = math.Abs(p.DeltaMHa)
				}
			} else if chemicalAccuracy && math.Abs(p.DeltaMHa) < bestClaimableDelta {
				bestClaimable = true
				bestClaimableDelta = math.Abs(p.DeltaMHa)
			}
			state.PesPoints = append(state.PesPoints, PesPoint{
				BondDistance: p.BondDistance, Energy: p.Energy, EnergyExact: p.EnergyExact,
				FCIEnergy: p.Fci, ActiveSpaceFCI: p.ActiveSpaceFCI,
				DeltaMHa: p.DeltaMHa, ExactDeltaMHa: p.ExactDeltaMHa,
				UncertaintyMHa: p.Uncertainty * 1000.0, CI95MHa: p.CI95MHa,
				ChemicalAccuracy: chemicalAccuracy, Claimable: chemicalAccuracy,
				FullSpaceStatus: p.FullSpaceStatus, Method: p.Method,
			})
			state.Trajectory = append(state.Trajectory, TrajectoryStep{
				Step: len(state.Trajectory) + 1, Energy: p.Energy,
				Loss: math.Abs(p.Energy - p.Fci), Theta: p.Theta, GradNorm: 0,
				Watchdog: "measured_vqe_pes", Timestamp: time.Now().UTC().Format(time.RFC3339),
				Source: "measured",
			})
		}
		// PES is a curve, not a replacement for a better primary VQE point.
		// Keep the best measured discrepancy so a noisy scan cannot demote a
		// verified VQE result to CONTESTED.
		if state.ClassicalAnchor.VqeExpectationEnergy == 0 ||
			best.DeltaMHa < state.ClassicalAnchor.DiscrepancyMHa {
			state.ClassicalAnchor.VqeExpectationEnergy = best.Energy
			state.ClassicalAnchor.DiscrepancyMHa = best.DeltaMHa
			state.ClassicalAnchor.ChemicalAccuracyPassed = bestClaimable
		}
		if state.ClassicalAnchor.FciExactEnergy == 0 {
			state.ClassicalAnchor.FciExactEnergy = best.Fci
			state.ClassicalAnchor.AnchorComputed = best.Fci != 0
			state.ClassicalAnchor.AnchorSource = "pyscf-live"
			state.ClassicalAnchor.Method = "PySCF STO-3G Full Configuration Interaction"
		}
		nb := ensureNotebook(&state)
		if nb.QuantumEnergy == 0 || best.DeltaMHa < nb.DeltaMHa {
			nb.QuantumEnergy = best.Energy
			nb.ClassicalEnergy = best.Fci
			nb.DeltaMHa = best.DeltaMHa
			nb.ChemicalAccuracy = bestClaimable
			if nb.ChemicalAccuracy {
				nb.EpistemicVerdict = "VERIFIED"
			} else {
				nb.EpistemicVerdict = "CONTESTED"
			}
		}
		if state.ClassicalAnchor.AnchorComputed && nb.ProvenanceHash == "" {
			nb.ClassicalAnchor = fmt.Sprintf("PySCF STO-3G Full Configuration Interaction (%.5f Ha)", best.Fci)
			nb.ProvenanceHash = provenanceHash(state.SessionID,
				state.QuantumFactors.TargetMolecule, state.QuantumFactors.BondDistance,
				best.Fci, state.QuantumFactors.QASM)
		}
		minPoint := points[0]
		for _, p := range points {
			if p.Energy < minPoint.Energy {
				minPoint = p
			}
		}
		measureLine("VQE PES: %d points; minimum E=%.6f Ha at R=%.4f Å; closest to FCI at R=%.4f Å (Δ=%.3f mHa).%s",
			len(points), minPoint.Energy, minPoint.BondDistance, best.BondDistance, best.DeltaMHa, citeSuffix(cite))
		return marshalReplan(&state, true, "VQE PES folded into trajectory")

	case "spin_chain":
		if !successTrue() {
			return nil, false, "spin-chain experiment reported success=false"
		}
		var energy, exact, uncertainty, delta, gap float64
		var nSites int
		var model, backend, resultVersion string
		var claimable bool
		for key, dst := range map[string]*float64{
			"energy": &energy, "energy_exact": &exact, "uncertainty": &uncertainty,
			"delta": &delta, "gap": &gap,
		} {
			if raw, ok := present(key); ok {
				_ = json.Unmarshal(raw, dst)
			}
		}
		for key, dst := range map[string]*string{
			"model": &model, "backend": &backend, "result_version": &resultVersion,
		} {
			if raw, ok := present(key); ok {
				_ = json.Unmarshal(raw, dst)
			}
		}
		if raw, ok := present("n_sites"); ok {
			_ = json.Unmarshal(raw, &nSites)
		}
		if raw, ok := present("claimable"); ok {
			_ = json.Unmarshal(raw, &claimable)
		}
		if energy == 0 || exact == 0 {
			return nil, false, "spin-chain energy/anchor absent"
		}
		// Do not append this to the chemistry Trajectory: FlightRecorder and
		// the workbench chart label trajectory energy as VQE, and a spin-chain
		// energy in coupling units is not a molecular energy. The notebook
		// record is the correct projection until ResultV1 variants reach the UI.
		nb := ensureNotebook(&state)
		nb.Sections = append(nb.Sections, fmt.Sprintf(
			"Job %s (spin_chain): %s n=%d measured E=%.6f vs exact %.6f (%.3g gap, uncertainty %.3g, claimable=%v, backend=%s).",
			shortID, model, nSites, energy, exact, gap, uncertainty, claimable, backend))
		trimMeasurementLines(&state)
		return marshalReplan(&state, true, "spin-chain measurement folded")

	case "conjecture_run":
		if !successTrue() {
			return nil, false, "conjecture experiment reported success=false"
		}
		var count int
		if raw, ok := present("candidate_count"); ok {
			_ = json.Unmarshal(raw, &count)
		}
		rawCandidates, ok := present("candidates")
		var laws []string
		if ok {
			var candidates []struct {
				Law    string `json:"law"`
				Status string `json:"status"`
			}
			if json.Unmarshal(rawCandidates, &candidates) == nil {
				for _, c := range candidates {
					if c.Law != "" {
						laws = append(laws, c.Law+" ["+c.Status+"]")
					}
				}
			}
		}
		nb := ensureNotebook(&state)
		line := fmt.Sprintf("Job %s (conjecture): %d exact finite-scope candidate(s) verified on holdout; not a theorem and not a claim.", shortID, count)
		if len(laws) > 0 {
			line += " " + strings.Join(laws, "; ")
		}
		nb.Sections = append(nb.Sections, line)
		trimMeasurementLines(&state)
		return marshalReplan(&state, true, "conjecture candidates recorded")

	case "qaoa_run", "qaoa_hardware":
		if !successTrue() {
			return nil, false, "QAOA reported success=false"
		}
		var value, exact, ratio float64
		var optimal int
		for key, dst := range map[string]*float64{
			"energy": &value, "energy_exact": &exact, "approximation_ratio": &ratio,
		} {
			if raw, ok := present(key); ok {
				var v float64
				if err := json.Unmarshal(raw, &v); err == nil {
					*dst = v
				}
			}
		}
		if raw, ok := present("optimal_cut"); ok {
			_ = json.Unmarshal(raw, &optimal)
		}
		if value == 0 && exact == 0 {
			return nil, false, "QAOA value absent"
		}
		ensureNotebook(&state)
		measureLine("QAOA MaxCut measured %.3f (exact optimizer %.3f) vs classical optimum %d; ratio %.4f.%s",
			value, exact, optimal, ratio, citeSuffix(cite))
		return marshalReplan(&state, true, "QAOA result recorded")

	case "claim_eval":
		var alpha, beta, gamma *float64
		for key, dst := range map[string]**float64{
			"corroborated_amplitude": &alpha,
			"contested_amplitude":    &beta,
			"unverified_amplitude":   &gamma,
		} {
			if raw, ok := present(key); ok {
				var v float64
				if err := json.Unmarshal(raw, &v); err == nil {
					*dst = &v
				}
			}
		}
		if alpha == nil && beta == nil && gamma == nil {
			return nil, false, "no evaluated amplitudes present"
		}
		ep := ensureEpistemics(&state)
		if alpha != nil {
			ep.AlphaCorroborated = *alpha
		}
		if beta != nil {
			ep.BetaContested = *beta
		}
		if gamma != nil {
			ep.GammaUnverified = *gamma
		}
		ep.PCorrelated = ep.AlphaCorroborated * ep.AlphaCorroborated * 100.0
		passes := false
		rawPass, ok := present("passes_chemical_accuracy")
		if !ok {
			// Absent verdict is not a failing verdict: folding
			// "contested" here would invent an evaluation outcome.
			return nil, false, "verdict unknown"
		}
		_ = json.Unmarshal(rawPass, &passes)
		var delta float64
		deltaKnown := false
		if rawDelta, ok := present("delta_mha"); ok {
			if json.Unmarshal(rawDelta, &delta) == nil {
				deltaKnown = true
			}
		}
		reason := ""
		if raw, ok := present("verdict_reason"); ok {
			_ = json.Unmarshal(raw, &reason)
		}
		if passes {
			if !state.ClassicalAnchor.AnchorComputed {
				// A pass against an uncomputed anchor verifies nothing:
				// the evaluator blessed fallback numbers. Amplitudes fold
				// (real tribunal output — the record is transcribed, not
				// rewritten), but collapse is withheld loudly (R3).
				ep.Status = "CONTESTED"
				ep.Statement = "Evaluated claim passed against an UNCOMPUTED anchor — collapse withheld: " + boundClaim(reason)
				measureLine("claim evaluated: PASS WITHHELD (anchor uncomputed). %s%s", boundClaim(reason), citeSuffix(cite))
			} else {
				ep.Status = "COLLAPSED_VERIFIED"
				measureLine("claim evaluated: chemical accuracy PASSED. %s%s", boundClaim(reason), citeSuffix(cite))
			}
		} else {
			ep.Status = "CONTESTED"
			ep.Statement = "Evaluated claim failed chemical accuracy: " + boundClaim(reason)
			measureLine("claim evaluated: chemical accuracy FAILED. %s%s", boundClaim(reason), citeSuffix(cite))
		}
		// Mechanical contradiction check: the verdict flag and the measured
		// delta must agree (pass ⟺ Δ ≤ 1.6). A mismatch means the evaluator
		// contradicts its own evidence — recorded loudly, never smoothed
		// over. This is the thinnest real contradiction detector: no LLM,
		// just arithmetic over the verdict just folded.
		if deltaKnown && ((passes && delta > 1.6) || (!passes && delta <= 1.6)) {
			measureLine("CONTRADICTION: verdict flag disagrees with measured Δ=%.2f mHa (passes=%v).", delta, passes)
		}
		return marshalReplan(&state, true, "epistemics set from evaluated verdict")

	case "mitigate":
		if !successTrue() {
			return nil, false, "mitigation reported success=false"
		}
		var method string
		if raw, ok := present("method_used"); ok {
			_ = json.Unmarshal(raw, &method)
		}
		if method == "" {
			return nil, false, "no mitigation method recorded"
		}
		var unmit, mit, bound float64
		// W1.14: all three values are the measurement — absent inputs
		// refuse instead of printing 0.0 defaults as measured results.
		rawUnmit, okU := present("unmitigated_value")
		if !okU {
			return nil, false, "unmitigated value absent"
		}
		if err := json.Unmarshal(rawUnmit, &unmit); err != nil {
			return nil, false, "unmitigated value undecodable"
		}
		rawMit, ok := present("mitigated_value")
		if !ok {
			return nil, false, "mitigated value absent"
		}
		if err := json.Unmarshal(rawMit, &mit); err != nil {
			return nil, false, "mitigated value undecodable"
		}
		rawBound, okB := present("error_bound")
		if !okB {
			return nil, false, "error bound absent"
		}
		if err := json.Unmarshal(rawBound, &bound); err != nil {
			return nil, false, "error bound undecodable"
		}
		measureLine("%s mitigation: %.6f -> %.6f (bound %.2e).", method, unmit, mit, bound)
		return marshalReplan(&state, true, "mitigation recorded")

	case "circuit_cut":
		if !successTrue() {
			return nil, false, "cut reported success=false"
		}
		// The bridge reports fragments (each with fragment_id and QASM);
		// total_cuts is optional. The count derives from the array when
		// absent — arithmetic over the payload, never invention. Fragment
		// QASM is not stored (kilobytes each); ids and cut qubits are.
		frags := []any{}
		if rawFrags, ok := present("fragments"); ok {
			var decoded []map[string]json.RawMessage
			if err := json.Unmarshal(rawFrags, &decoded); err == nil {
				for _, f := range decoded {
					var idx int
					_ = json.Unmarshal(f["fragment_id"], &idx)
					entry := map[string]any{"fragment": idx}
					var cutQubits []int32
					if rawQ, ok := f["cut_qubits"]; ok {
						if json.Unmarshal(rawQ, &cutQubits) == nil && len(cutQubits) > 0 {
							entry["cut_qubits"] = cutQubits
						}
					}
					frags = append(frags, entry)
				}
			}
		}
		cuts := len(frags)
		if rawCuts, ok := present("total_cuts"); ok {
			_ = json.Unmarshal(rawCuts, &cuts)
		}
		if cuts == 0 && len(frags) == 0 {
			return nil, false, "cut produced nothing"
		}
		state.QuantumFactors.WireCuts = []any{map[string]any{
			"measured": true, "job_id": shortID,
			"total_cuts": cuts, "fragments": frags,
		}}
		measureLine("cut into fragments with %d wire cuts.", cuts)
		return marshalReplan(&state, true, "cut recorded")

	case "pes_sweep":
		// A sweep changes no reference state: the campaign anchor stays
		// the equilibrium value, the trajectory stays optimizer output.
		// What folds is the computed table, as one bounded section, plus
		// the coarse FCI minimum (argmin over successful points only —
		// a single point has no minimum to claim).
		if !successTrue() {
			return nil, false, "sweep reported success=false"
		}
		rawPts, ok := present("points")
		if !ok {
			return nil, false, "sweep has no points"
		}
		var pts []map[string]json.RawMessage
		if err := json.Unmarshal(rawPts, &pts); err != nil || len(pts) == 0 {
			return nil, false, "sweep points undecodable"
		}
		var mol string
		if raw, ok := present("molecule"); ok {
			_ = json.Unmarshal(raw, &mol)
		}
		var lines []string
		type okPt struct {
			r, fci float64
		}
		var good []okPt
		rMin, rMax := 0.0, 0.0
		for i, pm := range pts {
			rawR, ok := pm["r"]
			if !ok {
				return nil, false, "sweep point missing geometry"
			}
			var r float64
			if err := json.Unmarshal(rawR, &r); err != nil {
				return nil, false, "sweep geometry undecodable"
			}
			if i == 0 {
				rMin, rMax = r, r
			} else {
				if r < rMin {
					rMin = r
				}
				if r > rMax {
					rMax = r
				}
			}
			var rhf, fci float64
			rOK, fOK := false, false
			if raw, ok := pm["rhf_energy"]; ok {
				rOK = json.Unmarshal(raw, &rhf) == nil
			}
			if raw, ok := pm["fci_energy"]; ok {
				fOK = json.Unmarshal(raw, &fci) == nil
			}
			if rOK && fOK {
				good = append(good, okPt{r, fci})
				lines = append(lines, fmt.Sprintf("  R=%.4f A  RHF=%.5f  FCI=%.5f Ha", r, rhf, fci))
				continue
			}
			reason := "energies absent"
			if raw, ok := pm["error"]; ok {
				var e string
				if json.Unmarshal(raw, &e) == nil && e != "" {
					reason = boundClaim(e)
				}
			}
			lines = append(lines, fmt.Sprintf("  R=%.4f A  anchor failed: %s", r, reason))
		}
		if len(good) == 0 {
			return nil, false, "sweep measured no geometry"
		}
		head := fmt.Sprintf("%s classical PES, %d points R=%.4f..%.4f A:", mol, len(pts), rMin, rMax)
		if len(good) >= 2 {
			best := good[0]
			for _, g := range good[1:] {
				if g.fci < best.fci {
					best = g
				}
			}
			lines = append(lines, fmt.Sprintf("  coarse FCI minimum near R=%.4f A", best.r))
		}
		nb := ensureNotebook(&state)
		nb.Sections = append(nb.Sections,
			"Job "+shortID+" (pes_sweep): "+head+"\n"+strings.Join(lines, "\n"))
		trimMeasurementLines(&state)
		return marshalReplan(&state, true, "PES table folded")

	default:
		return nil, false, "unknown job type " + jobType
	}
}

func marshalReplan(state *CampaignState, changed bool, note string) ([]byte, bool, string) {
	if !changed {
		return nil, false, note + " (no state change)"
	}
	state.UpdatedAt = time.Now().UTC()
	out, err := json.Marshal(state)
	if err != nil {
		return nil, false, "remarshal failed"
	}
	return out, true, note
}

// trimMeasurementLines drops the oldest "Job …" provenance lines past the
// cap. Narrative sections are never candidates for removal.
func ensureNotebook(state *CampaignState) *SystematicNotebook {
	if state.Notebook == nil {
		target := strings.TrimSpace(state.TargetSystem)
		if target == "" {
			target = strings.TrimSpace(state.QuantumFactors.TargetMolecule)
		}
		if target == "" {
			target = "campaign"
		}
		suffix := state.SessionID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		state.Notebook = &SystematicNotebook{
			ID:               "nb-" + strings.ToLower(strings.ReplaceAll(target, " ", "-")) + "-" + suffix,
			Title:            target + " Campaign Notebook",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
			ClassicalAnchor:  "not computed",
			EpistemicVerdict: "PENDING",
			Sections:         []string{},
		}
		// A campaign is created with a live classical anchor. The notebook
		// must cite it from the first measurement onward; the old placeholder
		// survived forever unless a separate classical_anchor job happened to
		// fold later (observed on the H2 audit campaign).
		if state.ClassicalAnchor.AnchorComputed {
			state.Notebook.ClassicalAnchor = fmt.Sprintf(
				"PySCF STO-3G Full Configuration Interaction (%.5f Ha)",
				state.ClassicalAnchor.FciExactEnergy)
			state.Notebook.ProvenanceHash = provenanceHash(state.SessionID,
				state.QuantumFactors.TargetMolecule, state.QuantumFactors.BondDistance,
				state.ClassicalAnchor.FciExactEnergy, state.QuantumFactors.QASM)
		}
	}
	return state.Notebook
}

func ensureEpistemics(state *CampaignState) *EpistemicState {
	if state.Epistemics == nil {
		state.Epistemics = &EpistemicState{Status: "UNVERIFIED", Sources: []string{}}
	}
	return state.Epistemics
}

func trimMeasurementLines(state *CampaignState) {
	if state.Notebook == nil {
		return
	}
	for len(state.Notebook.Sections) > maxNotebookSections {
		dropped := false
		for i, sec := range state.Notebook.Sections {
			if strings.HasPrefix(sec, "Job ") {
				state.Notebook.Sections = append(state.Notebook.Sections[:i], state.Notebook.Sections[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			return
		}
	}
}

// provenanceHash derives the campaign's Merkle identity from the inputs
// that define it. Synthesis and replan share it so a recomputed anchor
// re-hashes identically instead of forking the scheme.
func provenanceHash(sessionID, mol string, bondDist, fciVal float64, qasmCode string) string {
	// W1.14: full-precision floats — the old %.5f aliased distinct
	// (bond, energy) pairs to one provenance hash.
	hasher := sha256.New()
	hasher.Write([]byte(strings.Join([]string{
		sessionID, mol,
		strconv.FormatFloat(bondDist, 'g', -1, 64),
		strconv.FormatFloat(fciVal, 'g', -1, 64),
		qasmCode,
	}, ":")))
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(hasher.Sum(nil)))
}

// citeSuffix renders a resolvable record citation for a notebook line.
func citeSuffix(cite string) string {
	if strings.TrimSpace(cite) == "" {
		return ""
	}
	return " [" + strings.TrimSpace(cite) + "]"
}

func boundClaim(s string) string {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > 160 {
		t = string([]rune(t)[:160]) + "…"
	}
	if t == "" {
		t = "no reason recorded"
	}
	return t
}
