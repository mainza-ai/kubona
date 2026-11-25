package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func discoveryClaimsSubpath(id string) (sessionID, rest string, ok bool) {
	const marker = "/discovery-claims"
	idx := strings.Index(id, marker)
	if idx <= 0 {
		return "", "", false
	}
	after := id[idx+len(marker):]
	if after != "" && !strings.HasPrefix(after, "/") {
		return "", "", false
	}
	return id[:idx], strings.TrimPrefix(after, "/"), true
}

// isTrustedDiscoverySignoffKey enforces N1 (P0): Gate-7 Ed25519 sign-off MUST
// match either the tenant's commissioned AdminPubkeyHex or the host's
// commissioned reviewer/operator Ed25519 public key — never an arbitrary
// caller-generated keypair.
func (s *Server) isTrustedDiscoverySignoffKey(tenantID, pubkeyHex string) (bool, int, string) {
	pk := strings.TrimSpace(pubkeyHex)
	if pk == "" {
		return false, http.StatusBadRequest, "operator_pubkey_hex is required"
	}
	hasTrustAnchor := false
	if s.tenantMgr != nil {
		if lab, err := s.tenantMgr.GetTenant(tenantID); err == nil && strings.TrimSpace(lab.AdminPubkeyHex) != "" {
			hasTrustAnchor = true
			if strings.EqualFold(strings.TrimSpace(lab.AdminPubkeyHex), pk) {
				return true, http.StatusOK, ""
			}
		}
	}
	if len(s.signerPubKey) > 0 && (s.reviewerKeyConfigured || ephemeralAllowed()) {
		hasTrustAnchor = true
		if strings.EqualFold(hex.EncodeToString(s.signerPubKey), pk) {
			return true, http.StatusOK, ""
		}
	}
	if !hasTrustAnchor {
		return false, http.StatusServiceUnavailable, "no commissioned tenant admin or reviewer Ed25519 key is configured on this host"
	}
	return false, http.StatusForbidden, "operator_pubkey_hex is not a commissioned tenant admin or reviewer key"
}

func parseJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func asBoolField(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func asFloatField(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

func asStringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func isCompletedJobStatus(st string) bool {
	s := strings.TrimSpace(st)
	return strings.EqualFold(s, "completed") || strings.EqualFold(s, "JOB_STATUS_COMPLETED")
}

type discoveryClaimCreateReq struct {
	ClaimID               string `json:"claim_id"`
	Kind                  string `json:"kind"` // "qec_code" (default) | "circuit"
	CodeName              string `json:"code_name"`
	EvalJobID             string `json:"eval_job_id"`
	SimJobID              string `json:"sim_job_id"`
	BaselineSimJobID      string `json:"baseline_sim_job_id"`
	HeldOutSimJobID       string `json:"held_out_sim_job_id"`
	HeldOutBaselineSimJob string `json:"held_out_baseline_sim_job_id"`
	ReproSimJobID         string `json:"repro_sim_job_id"`
	// Phase 1 CircuitClaim fields (kind="circuit")
	EvalRunID     string `json:"eval_run_id"`
	BaselineRunID string `json:"baseline_run_id"`
	ReproRunID    string `json:"repro_run_id"`
	HeldOutRunID  string `json:"held_out_run_id"`
}

// knownPublishedCircuits mirrors kubona/codesign/claims.py BUILTIN_PUBLISHED_CIRCUITS
// for server-side C6 novelty enforcement (preventing textbook circuits from claiming T1_VERIFIED).
type knownCircuitEntry struct {
	Name       string
	SpecKind   string
	CanonJSON  string
	ArtifactID string
}

func knownPublishedCircuits() []knownCircuitEntry {
	entries := []knownCircuitEntry{
		{Name: "and2_single_ccx", SpecKind: "and2", CanonJSON: `[["ccx",[0,1,2],[]]]`},
		{Name: "copy1_single_cx", SpecKind: "copy1", CanonJSON: `[["cx",[0,1],[]]]`},
		{Name: "xor2_two_cx", SpecKind: "xor2", CanonJSON: `[["cx",[0,2],[]],["cx",[1,2],[]]]`},
		{Name: "cz_phase_single_cz", SpecKind: "cz-phase", CanonJSON: `[["cz",[0,1],[]]]`},
		{Name: "steane_7_cnot", SpecKind: "steane-cnot", CanonJSON: `[["cx",[0,7],[]],["cx",[1,8],[]],["cx",[2,9],[]],["cx",[3,10],[]],["cx",[4,11],[]],["cx",[5,12],[]],["cx",[6,13],[]]]`},
	}
	for i := range entries {
		sum := sha256.Sum256([]byte(entries[i].CanonJSON))
		entries[i].ArtifactID = hex.EncodeToString(sum[:])
	}
	return entries
}

func intPow(base, exp int) int {
	res := 1
	for i := 0; i < exp; i++ {
		res *= base
	}
	return res
}

// deriveServerSideCircuitClaim enforces ALL CircuitClaim gates C0-C6 strictly
// from durable, session-scoped codesign_runs rows (Phase 1 + C1/C8 hardening:
// unified discovery_claims table, exhaustive eps=0, re-derived TQ, held-out C5,
// and known-circuit novelty C6).
func (s *Server) deriveServerSideCircuitClaim(
	sessionID string,
	req discoveryClaimCreateReq,
) (qec.DiscoveryClaimRecord, error) {
	claimID := strings.TrimSpace(req.ClaimID)
	codeName := strings.TrimSpace(req.CodeName)
	evalRunID := strings.TrimSpace(req.EvalRunID)
	baseRunID := strings.TrimSpace(req.BaselineRunID)
	reproRunID := strings.TrimSpace(req.ReproRunID)
	heldOutRunID := strings.TrimSpace(req.HeldOutRunID)

	evalRun, err := s.ledger.GetCodesignRun(sessionID, evalRunID)
	if err != nil {
		return qec.DiscoveryClaimRecord{}, fmt.Errorf("eval_run_id %q not found in session %q", evalRunID, sessionID)
	}
	c0 := evalRun.ArtifactSHA256 != "" && evalRun.SpecKind != ""
	// Decision A2: exhaustive equivalence only (verification_method == "exhaustive" and eps == 0)
	c1 := evalRun.Valid && evalRun.VerificationMethod == "exhaustive" && evalRun.EpsBound == 0
	c1Reason := "ok"
	if !evalRun.Valid {
		c1Reason = "not_equivalent"
	} else if !c1 {
		c1Reason = "non_exhaustive_not_claimable"
	}

	// C2 (C8 fix): non-vacuous TQ re-derivation (TQ == toffoli * peak_qubits^kappa)
	c2 := false
	c2Reason := "invalid_cost_parameters"
	expectedTQ := -1
	if evalRun.Kappa >= 1 && evalRun.Kappa <= 3 && evalRun.PeakQubits > 0 && evalRun.Toffoli >= 0 {
		expectedTQ = evalRun.Toffoli * intPow(evalRun.PeakQubits, evalRun.Kappa)
		if evalRun.TQ == expectedTQ {
			c2 = true
			c2Reason = "ok"
		} else {
			c2Reason = fmt.Sprintf("tq_mismatch (got=%d, expected=%d)", evalRun.TQ, expectedTQ)
		}
	}

	c3 := false
	c3Reason := "missing_baseline_run"
	if baseRunID != "" && baseRunID != evalRunID {
		baseRun, bErr := s.ledger.GetCodesignRun(sessionID, baseRunID)
		if bErr != nil {
			return qec.DiscoveryClaimRecord{}, fmt.Errorf("baseline_run_id %q not found in session %q", baseRunID, sessionID)
		}
		if baseRun.SpecKind != evalRun.SpecKind || baseRun.Kappa != evalRun.Kappa {
			c3Reason = "baseline_spec_or_kappa_mismatch"
		} else if !baseRun.Valid {
			c3Reason = "baseline_invalid"
		} else {
			betterTQ := evalRun.TQ < baseRun.TQ
			betterTDepth := evalRun.ToffoliDepth < baseRun.ToffoliDepth && evalRun.TQ <= baseRun.TQ
			betterQ := evalRun.PeakQubits < baseRun.PeakQubits && evalRun.TQ <= baseRun.TQ
			if betterTQ || betterTDepth || betterQ {
				c3 = true
				c3Reason = "ok"
			} else {
				c3Reason = "no_frontier_advantage"
			}
		}
	}

	c4 := false
	c4Reason := "missing_repro_run"
	var reproSeed int
	hasRepro := false
	if reproRunID != "" && reproRunID != evalRunID {
		reproRun, rErr := s.ledger.GetCodesignRun(sessionID, reproRunID)
		if rErr != nil {
			return qec.DiscoveryClaimRecord{}, fmt.Errorf("repro_run_id %q not found in session %q", reproRunID, sessionID)
		}
		reproSeed = reproRun.Seed
		hasRepro = true
		if reproRun.ArtifactSHA256 != evalRun.ArtifactSHA256 {
			c4Reason = "artifact_mismatch"
		} else if reproRun.Seed == evalRun.Seed {
			c4Reason = "repro_seed_not_distinct"
		} else if !reproRun.Valid || reproRun.VerificationMethod != "exhaustive" || reproRun.EpsBound != 0 {
			c4Reason = "repro_not_exhaustive_valid"
		} else {
			c4 = true
			c4Reason = "ok"
		}
	}

	// C5 (C1 fix): held-out verification run required (distinct run & seed, exhaustive, matching artifact)
	c5 := false
	c5Reason := "missing_held_out_run"
	if heldOutRunID != "" && heldOutRunID != evalRunID && heldOutRunID != reproRunID {
		heldRun, hErr := s.ledger.GetCodesignRun(sessionID, heldOutRunID)
		if hErr != nil {
			return qec.DiscoveryClaimRecord{}, fmt.Errorf("held_out_run_id %q not found in session %q", heldOutRunID, sessionID)
		}
		if heldRun.ArtifactSHA256 != evalRun.ArtifactSHA256 {
			c5Reason = "held_out_artifact_mismatch"
		} else if heldRun.SpecKind != evalRun.SpecKind {
			c5Reason = "held_out_spec_mismatch"
		} else if heldRun.Seed == evalRun.Seed || (hasRepro && heldRun.Seed == reproSeed) {
			c5Reason = "held_out_seed_not_distinct"
		} else if !heldRun.Valid || heldRun.VerificationMethod != "exhaustive" || heldRun.EpsBound != 0 {
			c5Reason = "held_out_not_exhaustive_valid"
		} else {
			c5 = true
			c5Reason = "ok"
		}
	}

	// C6 (C1 fix): novelty vs known published circuits (exact canonical hash)
	c6 := true
	c6Reason := "novel"
	matchedKnown := ""
	for _, kc := range knownPublishedCircuits() {
		if kc.SpecKind == evalRun.SpecKind && kc.ArtifactID == evalRun.ArtifactSHA256 {
			c6 = false
			matchedKnown = kc.Name
			c6Reason = "known_published_circuit:" + kc.Name
			break
		}
	}

	allPassed := c0 && c1 && c2 && c3 && c4 && c5 && c6
	gatesObj := map[string]any{
		"gate0_valid":       map[string]any{"passed": c0, "eval_run_id": evalRunID, "artifact_sha256": evalRun.ArtifactSHA256},
		"gate1_equivalence": map[string]any{"passed": c1, "reason": c1Reason, "verification_method": evalRun.VerificationMethod, "eps_bound": evalRun.EpsBound},
		"gate2_cost":        map[string]any{"passed": c2, "reason": c2Reason, "toffoli": evalRun.Toffoli, "peak_qubits": evalRun.PeakQubits, "tq": evalRun.TQ, "expected_tq": expectedTQ, "kappa": evalRun.Kappa},
		"gate3_frontier":    map[string]any{"passed": c3, "reason": c3Reason, "baseline_run_id": baseRunID},
		"gate4_reproduced":  map[string]any{"passed": c4, "reason": c4Reason, "repro_run_id": reproRunID},
		"gate5_held_out":    map[string]any{"passed": c5, "reason": c5Reason, "held_out_run_id": heldOutRunID},
		"gate6_novelty":     map[string]any{"passed": c6, "reason": c6Reason, "matched_known": matchedKnown},
	}

	artPayload := map[string]any{
		"claim_id":        claimID,
		"session_id":      sessionID,
		"kind":            "circuit",
		"code_name":       codeName,
		"spec_kind":       evalRun.SpecKind,
		"eval_run_id":     evalRunID,
		"baseline_run_id": baseRunID,
		"repro_run_id":    reproRunID,
		"held_out_run_id": heldOutRunID,
		"artifact_sha256": evalRun.ArtifactSHA256,
	}
	artBytes, _ := json.Marshal(artPayload)
	artSum := sha256.Sum256(artBytes)
	artHex := hex.EncodeToString(artSum[:])

	gatesBytes, _ := json.Marshal(gatesObj)
	gatesSum := sha256.Sum256(gatesBytes)
	gatesHex := hex.EncodeToString(gatesSum[:])

	tier := "T3_SAMPLED"
	cred := "L2"
	if allPassed {
		tier = "T1_VERIFIED"
		cred = "L4"
	}

	return qec.DiscoveryClaimRecord{
		ClaimID:         claimID,
		SessionID:       sessionID,
		Kind:            "circuit",
		CodeName:        codeName,
		Status:          qec.ClaimCandidate,
		EvidenceTier:    tier,
		CredibilityTier: cred,
		ArtifactSHA256:  artHex,
		GatesDigest:     gatesHex,
		Gates0To6Passed: allPassed,
		GateReportJSON:  string(gatesBytes),
	}, nil
}

func (s *Server) loadCompletedSimJob(sessionID, jobID, label string) (map[string]any, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return nil, nil
	}
	job, err := s.jobRunner.GetJob(id)
	if err != nil || job == nil || job.SessionID != sessionID || !isCompletedJobStatus(job.Status.String()) || job.JobType != "qec_memory_sim" {
		return nil, fmt.Errorf("%s %q must be a completed qec_memory_sim job in session %q", label, id, sessionID)
	}
	res := parseJSONObject(job.ResultJSON)
	if res == nil {
		return nil, fmt.Errorf("%s %q has empty or invalid result_json", label, id)
	}
	return res, nil
}

// deriveServerSideDiscoveryClaim computes artifact_sha256, gates_digest, and
// gates_0_to_6_passed strictly from settled ledger jobs owned by sessionID
// (N1 + N9 + N10). Never trusts client-supplied gate flags, hashes, unchecked
// novelty, or unbound simulation jobs.
func (s *Server) deriveServerSideDiscoveryClaim(
	sessionID string,
	req discoveryClaimCreateReq,
) (qec.DiscoveryClaimRecord, error) {
	if s.jobRunner == nil {
		return qec.DiscoveryClaimRecord{}, errors.New("job runner unavailable")
	}
	claimID := strings.TrimSpace(req.ClaimID)
	codeName := strings.TrimSpace(req.CodeName)
	evalJobID := strings.TrimSpace(req.EvalJobID)
	simJobID := strings.TrimSpace(req.SimJobID)
	baseSimJobID := strings.TrimSpace(req.BaselineSimJobID)
	heldOutJobID := strings.TrimSpace(req.HeldOutSimJobID)
	heldOutBaseJobID := strings.TrimSpace(req.HeldOutBaselineSimJob)
	reproJobID := strings.TrimSpace(req.ReproSimJobID)

	evalJob, err := s.jobRunner.GetJob(evalJobID)
	if err != nil || evalJob == nil || evalJob.SessionID != sessionID {
		return qec.DiscoveryClaimRecord{}, fmt.Errorf("eval_job_id %q not found in session %q", evalJobID, sessionID)
	}
	if !isCompletedJobStatus(evalJob.Status.String()) {
		return qec.DiscoveryClaimRecord{}, fmt.Errorf("eval_job_id %q is not completed (status=%s)", evalJobID, evalJob.Status.String())
	}
	if evalJob.JobType != "qec_code_eval" && evalJob.JobType != "qec_distance" {
		return qec.DiscoveryClaimRecord{}, fmt.Errorf("eval_job_id %q must be qec_code_eval or qec_distance (got %s)", evalJobID, evalJob.JobType)
	}
	evalRes := parseJSONObject(evalJob.ResultJSON)
	if evalRes == nil {
		return qec.DiscoveryClaimRecord{}, errors.New("eval_job result_json is empty or invalid")
	}

	evalCodeName := asStringField(evalRes, "code_name")
	if evalCodeName == "" {
		evalCodeName = codeName
	}

	nVal, okN := asFloatField(evalRes, "n")
	kVal, okK := asFloatField(evalRes, "k")
	dVal, okD := asFloatField(evalRes, "d")
	dKind := asStringField(evalRes, "d_kind")
	certObj, _ := evalRes["certificate"].(map[string]any)
	if certObj == nil {
		certObj, _ = evalRes["distance_certificate"].(map[string]any)
	}
	proofScope := asStringField(certObj, "proof_scope")
	witnessSha := asStringField(certObj, "witness_sha256")
	decomposable, okDecomp := asBoolField(evalRes, "decomposable")
	catalogKnown, okCat := asBoolField(evalRes, "catalog_known")
	catalogRel := asStringField(evalRes, "catalog_relation")
	canonicalHash := asStringField(evalRes, "canonical_hash")

	g0 := okN && okK && nVal > 0 && kVal > 0
	g1 := okD && dVal > 0 && dKind == "proved" && proofScope == "full_logical_group" && witnessSha != ""
	g2 := okDecomp && !decomposable
	// N10: reject check_catalog=false (relation="unchecked") and require canonical_hash
	g3 := okCat && !catalogKnown && catalogRel == "novel_to_catalog" && canonicalHash != ""
	g3Reason := "ok"
	if !g3 {
		if catalogRel == "unchecked" || catalogRel == "" {
			g3Reason = "catalog_unchecked"
		} else if catalogKnown {
			g3Reason = "known_in_catalog"
		} else {
			g3Reason = "missing_canonical_hash"
		}
	}

	simRes, err := s.loadCompletedSimJob(sessionID, simJobID, "sim_job_id")
	if err != nil {
		return qec.DiscoveryClaimRecord{}, err
	}
	baseRes, err := s.loadCompletedSimJob(sessionID, baseSimJobID, "baseline_sim_job_id")
	if err != nil {
		return qec.DiscoveryClaimRecord{}, err
	}
	heldOutRes, err := s.loadCompletedSimJob(sessionID, heldOutJobID, "held_out_sim_job_id")
	if err != nil {
		return qec.DiscoveryClaimRecord{}, err
	}
	heldOutBaseRes, err := s.loadCompletedSimJob(sessionID, heldOutBaseJobID, "held_out_baseline_sim_job_id")
	if err != nil {
		return qec.DiscoveryClaimRecord{}, err
	}
	reproRes, err := s.loadCompletedSimJob(sessionID, reproJobID, "repro_sim_job_id")
	if err != nil {
		return qec.DiscoveryClaimRecord{}, err
	}

	// N9 Gate 4: sim_job bound to evalCodeName, matched qubit budget, non-overlapping 95% CI vs baseline_sim_job
	g4 := false
	g4Reason := "missing_sim_or_baseline"
	baseLo := 0.0
	hasBaseLo := false
	if simRes != nil && baseRes != nil && simJobID != baseSimJobID {
		simCode := asStringField(simRes, "code_name")
		_, okLer := asFloatField(simRes, "ler")
		simHi, okSimHi := asFloatField(simRes, "ci_high")
		bLo, okBaseLo := asFloatField(baseRes, "ci_low")
		simQ, okSimQ := asFloatField(simRes, "total_qubits")
		baseQ, okBaseQ := asFloatField(baseRes, "total_qubits")
		budgetOK := !okSimQ || !okBaseQ || simQ <= baseQ
		if simCode != evalCodeName {
			g4Reason = fmt.Sprintf("code_name_mismatch (sim=%q, eval=%q)", simCode, evalCodeName)
		} else if !budgetOK {
			g4Reason = "qubit_budget_exceeded"
		} else if !okLer || !okSimHi || !okBaseLo {
			g4Reason = "missing_ci_fields"
		} else if simHi >= bLo {
			g4Reason = "ci_overlap_with_baseline"
		} else {
			g4 = true
			g4Reason = "ok"
			baseLo = bLo
			hasBaseLo = true
		}
	}

	// N9 Gate 5: held-out sim bound to evalCodeName, matching held-out snapshot, non-overlapping 95% CI vs held-out baseline
	g5 := false
	g5Reason := "missing_held_out_or_baseline"
	if heldOutRes != nil && heldOutBaseRes != nil && heldOutJobID != heldOutBaseJobID {
		hoCode := asStringField(heldOutRes, "code_name")
		hoNoise, _ := heldOutRes["noise"].(map[string]any)
		hoBaseNoise, _ := heldOutBaseRes["noise"].(map[string]any)
		hoIsHeldOut, _ := asBoolField(hoNoise, "is_held_out")
		hoBaseIsHeldOut, _ := asBoolField(hoBaseNoise, "is_held_out")
		hoSnap := asStringField(hoNoise, "calibration_snapshot_id")
		hoBaseSnap := asStringField(hoBaseNoise, "calibration_snapshot_id")
		hoHi, okHoHi := asFloatField(heldOutRes, "ci_high")
		hoBaseLo, okHoBaseLo := asFloatField(heldOutBaseRes, "ci_low")
		if hoCode != evalCodeName {
			g5Reason = fmt.Sprintf("code_name_mismatch (held_out=%q, eval=%q)", hoCode, evalCodeName)
		} else if !hoIsHeldOut || !hoBaseIsHeldOut || hoSnap == "" || hoSnap != hoBaseSnap {
			g5Reason = "held_out_snapshot_mismatch"
		} else if !okHoHi || !okHoBaseLo {
			g5Reason = "missing_ci_fields"
		} else if hoHi >= hoBaseLo {
			g5Reason = "held_out_ci_overlap"
		} else {
			g5 = true
			g5Reason = "ok"
		}
	}

	// N9 Gate 6: distinct second-seed reproduction sim bound to evalCodeName with d_circ > 0 and CI below baseline
	g6 := false
	g6Reason := "missing_repro_sim"
	if reproRes != nil && reproJobID != simJobID {
		reproCode := asStringField(reproRes, "code_name")
		reproDCirc, okDC := asFloatField(reproRes, "d_circ")
		reproHi, okReproHi := asFloatField(reproRes, "ci_high")
		simSeed, okSimSeed := asFloatField(simRes, "seed")
		reproSeed, okReproSeed := asFloatField(reproRes, "seed")
		distinctSeed := !okSimSeed || !okReproSeed || simSeed != reproSeed
		if reproCode != evalCodeName {
			g6Reason = fmt.Sprintf("code_name_mismatch (repro=%q, eval=%q)", reproCode, evalCodeName)
		} else if !distinctSeed {
			g6Reason = "repro_seed_not_distinct"
		} else if !okDC || reproDCirc <= 0 {
			g6Reason = "d_circ_not_verified"
		} else if !hasBaseLo || !okReproHi || reproHi >= baseLo {
			g6Reason = "repro_ci_not_below_baseline"
		} else {
			g6 = true
			g6Reason = "ok"
		}
	}

	allPassed := g0 && g1 && g2 && g3 && g4 && g5 && g6
	gatesObj := map[string]any{
		"gate0_valid":          map[string]any{"passed": g0, "eval_job_id": evalJobID},
		"gate1_exact_params":   map[string]any{"passed": g1, "d_kind": dKind, "proof_scope": proofScope, "witness_sha256": witnessSha},
		"gate2_indecomposable": map[string]any{"passed": g2},
		"gate3_novel":          map[string]any{"passed": g3, "reason": g3Reason, "catalog_relation": catalogRel, "canonical_hash": canonicalHash},
		"gate4_frontier":       map[string]any{"passed": g4, "reason": g4Reason, "sim_job_id": simJobID, "baseline_sim_job_id": baseSimJobID},
		"gate5_held_out":       map[string]any{"passed": g5, "reason": g5Reason, "held_out_sim_job_id": heldOutJobID, "held_out_baseline_sim_job_id": heldOutBaseJobID},
		"gate6_reproduced":     map[string]any{"passed": g6, "reason": g6Reason, "repro_sim_job_id": reproJobID},
	}

	artifactPayload := map[string]any{
		"claim_id":                     claimID,
		"session_id":                   sessionID,
		"code_name":                    evalCodeName,
		"eval_job_id":                  evalJobID,
		"sim_job_id":                   simJobID,
		"baseline_sim_job_id":          baseSimJobID,
		"held_out_sim_job_id":          heldOutJobID,
		"held_out_baseline_sim_job_id": heldOutBaseJobID,
		"repro_sim_job_id":             reproJobID,
		"witness_sha256":               witnessSha,
		"canonical_hash":               canonicalHash,
	}
	artBytes, _ := json.Marshal(artifactPayload)
	artSum := sha256.Sum256(artBytes)
	artHex := hex.EncodeToString(artSum[:])

	gatesBytes, _ := json.Marshal(gatesObj)
	gatesSum := sha256.Sum256(gatesBytes)
	gatesHex := hex.EncodeToString(gatesSum[:])

	tier := "T3_SAMPLED"
	cred := "L2"
	if allPassed {
		tier = "T1_VERIFIED"
		cred = "L4"
	}

	return qec.DiscoveryClaimRecord{
		ClaimID:         claimID,
		SessionID:       sessionID,
		CodeName:        evalCodeName,
		Status:          qec.ClaimCandidate,
		EvidenceTier:    tier,
		CredibilityTier: cred,
		ArtifactSHA256:  artHex,
		GatesDigest:     gatesHex,
		Gates0To6Passed: allPassed,
		GateReportJSON:  string(gatesBytes),
	}, nil
}

// handleDiscoveryClaimsRoute exposes durable, session-scoped (N8),
// server-verified (N1/N9/N10) Schema-17 QEC discovery_claims lifecycle operations:
//
//	GET  /api/sessions/{id}/discovery-claims
//	POST /api/sessions/{id}/discovery-claims                      (create from settled session jobs only)
//	POST /api/sessions/{id}/discovery-claims/{claimId}/transition (verified / trust-anchored Ed25519 claimed / retracted)
func (s *Server) handleDiscoveryClaimsRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}

	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			claims, err := s.ledger.ListDiscoveryClaims(sessionID)
			if err != nil {
				WriteAPIError(w, "internal", "failed to list discovery claims", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"session_id": sessionID,
				"claims":     claims,
			})
			return

		case http.MethodPost:
			var req discoveryClaimCreateReq
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
			if err := dec.Decode(&req); err != nil {
				WriteAPIError(w, "bad_request", "invalid discovery claim JSON", nil)
				return
			}
			kind := strings.TrimSpace(req.Kind)
			if kind == "" {
				kind = "qec_code"
			}
			var (
				rec qec.DiscoveryClaimRecord
				err error
			)
			if kind == "circuit" {
				if strings.TrimSpace(req.ClaimID) == "" || strings.TrimSpace(req.CodeName) == "" || strings.TrimSpace(req.EvalRunID) == "" {
					WriteAPIError(w, "bad_request", "claim_id, code_name, and settled eval_run_id are required for kind=circuit", nil)
					return
				}
				rec, err = s.deriveServerSideCircuitClaim(sessionID, req)
			} else if kind == "qec_code" {
				if strings.TrimSpace(req.ClaimID) == "" || strings.TrimSpace(req.CodeName) == "" || strings.TrimSpace(req.EvalJobID) == "" {
					WriteAPIError(w, "bad_request", "claim_id, code_name, and settled eval_job_id are required (client-supplied gate flags and artifact hashes are not accepted)", nil)
					return
				}
				rec, err = s.deriveServerSideDiscoveryClaim(sessionID, req)
			} else {
				WriteAPIError(w, "bad_request", fmt.Sprintf("unsupported discovery claim kind %q (allowed: qec_code, circuit)", kind), nil)
				return
			}
			if err != nil {
				WriteAPIError(w, "bad_request", err.Error(), nil)
				return
			}
			if err := s.ledger.SaveDiscoveryClaim(rec); err != nil {
				WriteAPIError(w, "bad_request", err.Error(), nil)
				return
			}
			saved, err := s.ledger.GetDiscoveryClaim(sessionID, rec.ClaimID)
			if err != nil {
				WriteAPIError(w, "internal", "failed to reload saved discovery claim", nil)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"claim": saved})
			return
		default:
			WriteAPIError(w, "method_not_allowed", "GET or POST required", nil)
			return
		}
	}

	// POST /api/sessions/{id}/discovery-claims/{claimId}/transition
	if strings.HasSuffix(rest, "/transition") {
		if r.Method != http.MethodPost {
			WriteAPIError(w, "method_not_allowed", "POST required", nil)
			return
		}
		claimID := strings.TrimSuffix(rest, "/transition")
		var req struct {
			Status               string `json:"status"`
			OperatorPubkeyHex    string `json:"operator_pubkey_hex"`
			OperatorSignatureHex string `json:"operator_signature_hex"`
			RetractionReason     string `json:"retraction_reason"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			WriteAPIError(w, "bad_request", "invalid transition payload", nil)
			return
		}
		nextStatus := qec.DiscoveryClaimStatus(strings.TrimSpace(req.Status))

		// N1 (P0): Gate-7 promotion to claimed MUST be signed by a commissioned
		// tenant admin or host reviewer Ed25519 key (never a fresh caller keypair).
		if nextStatus == qec.ClaimClaimed {
			trusted, code, reason := s.isTrustedDiscoverySignoffKey(sess.TenantID, req.OperatorPubkeyHex)
			if !trusted {
				errCode := "forbidden"
				if code == http.StatusServiceUnavailable {
					errCode = "service_unavailable"
				} else if code == http.StatusBadRequest {
					errCode = "bad_request"
				}
				WriteAPIError(w, errCode, reason, nil)
				return
			}
		}

		updated, err := s.ledger.TransitionAndPersistDiscoveryClaim(
			sessionID,
			claimID,
			nextStatus,
			req.OperatorPubkeyHex,
			req.OperatorSignatureHex,
			req.RetractionReason,
		)
		if err != nil {
			if errors.Is(err, ledger.ErrDiscoveryClaimNotFound) {
				WriteAPIError(w, "not_found", err.Error(), nil)
				return
			}
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"claim": updated})
		return
	}

	WriteAPIError(w, "not_found", "unknown discovery-claims subpath", nil)
}
