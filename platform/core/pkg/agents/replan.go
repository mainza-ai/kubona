package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Planner replan: campaign synthesis runs on every chat turn, but until now
// nothing folded settled job results back into the campaign — the plan could
// describe a run that already happened without knowing its outcome. This
// file closes that loop: a pure fold of measured results into stored state.
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
//   - claim_eval: evaluated amplitudes replace the synthesis placeholders;
//     verdict pass/fail drives status. No success field exists on this
//     response; presence of amplitude keys is the validity signal.
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
	var passes bool
	rawPass, ok := res["passes_chemical_accuracy"]
	if !ok {
		return nil, false
	}
	if err := json.Unmarshal(rawPass, &passes); err != nil {
		return nil, false
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
		state.Notebook.Sections = append(state.Notebook.Sections,
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
		changedAny = true // the provenance line itself is new information
		return marshalReplan(&state, changedAny, "circuit measurements folded")

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
		// ("H2" == "H2"), with substring against the display target only
		// as a fallback for formula-less states. Substring alone
		// mismatches ("H" would satisfy "H2") and would corrupt the
		// anchor with the wrong molecule's physics — so codes shorter
		// than two characters never match at all.
		molNorm := strings.ToUpper(strings.TrimSpace(mol))
		matched := false
		if len(molNorm) >= 2 {
			matched = strings.EqualFold(
				strings.TrimSpace(state.QuantumFactors.MolecularFormula), mol)
			if !matched {
				targetUpper := strings.ToUpper(state.QuantumFactors.TargetMolecule)
				matched = strings.Contains(targetUpper, molNorm) ||
					strings.Contains(molNorm, targetUpper)
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
		delta := math.Abs(state.ClassicalAnchor.VqeExpectationEnergy-fci) * 1000.0
		state.ClassicalAnchor.DiscrepancyMHa = delta
		state.ClassicalAnchor.ChemicalAccuracyPassed = delta <= 1.6
		state.Notebook.ClassicalAnchor = fmt.Sprintf("PySCF STO-3G Full Configuration Interaction (%.5f Ha)", fci)
		state.Notebook.ProvenanceHash = provenanceHash(state.SessionID,
			state.QuantumFactors.TargetMolecule, state.QuantumFactors.BondDistance, fci,
			state.QuantumFactors.QASM)
		// A recomputed anchor repairs fallback epistemics (R3): strip the
		// hardcoded-fallback caveat and re-attest the PySCF sources. Only
		// pristine UNVERIFIED states regain projection weights — fresh
		// HYPOTHESIS_PENDING keeps its pending weights, and anything
		// already decided (COLLAPSED/CONTESTED/measured PROJECTED) is
		// never rewritten. The notebook sections stay historical; the
		// repair is recorded in the measure line below.
		if strings.Contains(state.Epistemics.Statement, strings.TrimSpace(fallbackCaveat)) {
			state.Epistemics.Statement = strings.ReplaceAll(state.Epistemics.Statement, fallbackCaveat, "")
			state.Epistemics.Sources = synthSources(true)
			if state.Epistemics.Status == "UNVERIFIED" {
				state.Epistemics.Status = "PROJECTED"
				state.Epistemics.AlphaCorroborated = liveAlpha
				state.Epistemics.BetaContested = liveBeta
				state.Epistemics.GammaUnverified = liveGamma
				state.Epistemics.PCorrelated = livePCorrelated
			}
		}
		measureLine("anchor recomputed: FCI %.5f -> %.5f Ha, discrepancy %.2f mHa.", oldFCI, fci, delta)
		return marshalReplan(&state, true, "classical anchor recomputed")

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
		if alpha != nil {
			state.Epistemics.AlphaCorroborated = *alpha
		}
		if beta != nil {
			state.Epistemics.BetaContested = *beta
		}
		if gamma != nil {
			state.Epistemics.GammaUnverified = *gamma
		}
		state.Epistemics.PCorrelated = state.Epistemics.AlphaCorroborated * state.Epistemics.AlphaCorroborated * 100.0
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
				state.Epistemics.Status = "CONTESTED"
				state.Epistemics.Statement = "Evaluated claim passed against an UNCOMPUTED anchor — collapse withheld: " + boundClaim(reason)
				measureLine("claim evaluated: PASS WITHHELD (anchor uncomputed). %s%s", boundClaim(reason), citeSuffix(cite))
			} else {
				state.Epistemics.Status = "COLLAPSED_VERIFIED"
				measureLine("claim evaluated: chemical accuracy PASSED. %s%s", boundClaim(reason), citeSuffix(cite))
			}
		} else {
			state.Epistemics.Status = "CONTESTED"
			state.Epistemics.Statement = "Evaluated claim failed chemical accuracy: " + boundClaim(reason)
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
		if raw, ok := present("unmitigated_value"); ok {
			_ = json.Unmarshal(raw, &unmit)
		}
		rawMit, ok := present("mitigated_value")
		if !ok {
			return nil, false, "mitigated value absent"
		}
		_ = json.Unmarshal(rawMit, &mit)
		if raw, ok := present("error_bound"); ok {
			_ = json.Unmarshal(raw, &bound)
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
		state.Notebook.Sections = append(state.Notebook.Sections,
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
func trimMeasurementLines(state *CampaignState) {
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
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s:%s:%.5f:%.5f:%s", sessionID, mol, bondDist, fciVal, qasmCode)))
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
