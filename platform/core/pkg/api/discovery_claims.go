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
	CodeName              string `json:"code_name"`
	EvalJobID             string `json:"eval_job_id"`
	SimJobID              string `json:"sim_job_id"`
	BaselineSimJobID      string `json:"baseline_sim_job_id"`
	HeldOutSimJobID       string `json:"held_out_sim_job_id"`
	HeldOutBaselineSimJob string `json:"held_out_baseline_sim_job_id"`
	ReproSimJobID         string `json:"repro_sim_job_id"`
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
			if strings.TrimSpace(req.ClaimID) == "" || strings.TrimSpace(req.CodeName) == "" || strings.TrimSpace(req.EvalJobID) == "" {
				WriteAPIError(w, "bad_request", "claim_id, code_name, and settled eval_job_id are required (client-supplied gate flags and artifact hashes are not accepted)", nil)
				return
			}
			rec, err := s.deriveServerSideDiscoveryClaim(sessionID, req)
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
