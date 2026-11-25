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

// deriveServerSideDiscoveryClaim computes artifact_sha256, gates_digest, and
// gates_0_to_6_passed strictly from settled ledger jobs owned by sessionID (N1).
// Never trusts client-supplied gate flags or hashes.
func (s *Server) deriveServerSideDiscoveryClaim(
	sessionID, claimID, codeName, evalJobID, simJobID, heldOutJobID string,
) (qec.DiscoveryClaimRecord, error) {
	if s.jobRunner == nil {
		return qec.DiscoveryClaimRecord{}, errors.New("job runner unavailable")
	}
	evalJob, err := s.jobRunner.GetJob(strings.TrimSpace(evalJobID))
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

	g0 := okN && okK && nVal > 0 && kVal > 0
	g1 := okD && dVal > 0 && dKind == "proved" && proofScope == "full_logical_group" && witnessSha != ""
	g2 := okDecomp && !decomposable
	g3 := okCat && !catalogKnown

	g4 := false
	g5 := false
	g6 := false
	var simRes, heldOutRes map[string]any

	if strings.TrimSpace(simJobID) != "" {
		simJob, sErr := s.jobRunner.GetJob(strings.TrimSpace(simJobID))
		if sErr != nil || simJob == nil || simJob.SessionID != sessionID || !isCompletedJobStatus(simJob.Status.String()) || simJob.JobType != "qec_memory_sim" {
			return qec.DiscoveryClaimRecord{}, fmt.Errorf("sim_job_id %q must be a completed qec_memory_sim job in session %q", simJobID, sessionID)
		}
		simRes = parseJSONObject(simJob.ResultJSON)
		_, okLer := asFloatField(simRes, "ler")
		_, okLo := asFloatField(simRes, "ci_low")
		_, okHi := asFloatField(simRes, "ci_high")
		dCirc, okDC := asFloatField(simRes, "d_circ")
		g4 = okLer && okLo && okHi
		g6 = okDC && dCirc > 0
	}

	if strings.TrimSpace(heldOutJobID) != "" {
		hJob, hErr := s.jobRunner.GetJob(strings.TrimSpace(heldOutJobID))
		if hErr != nil || hJob == nil || hJob.SessionID != sessionID || !isCompletedJobStatus(hJob.Status.String()) || hJob.JobType != "qec_memory_sim" {
			return qec.DiscoveryClaimRecord{}, fmt.Errorf("held_out_sim_job_id %q must be a completed qec_memory_sim job in session %q", heldOutJobID, sessionID)
		}
		heldOutRes = parseJSONObject(hJob.ResultJSON)
		noiseObj, _ := heldOutRes["noise"].(map[string]any)
		isHeldOut, _ := asBoolField(noiseObj, "is_held_out")
		snapID := asStringField(noiseObj, "calibration_snapshot_id")
		_, okLer := asFloatField(heldOutRes, "ler")
		g5 = isHeldOut && snapID != "" && okLer
	}

	allPassed := g0 && g1 && g2 && g3 && g4 && g5 && g6
	gatesObj := map[string]any{
		"gate0_valid":          map[string]any{"passed": g0, "eval_job_id": evalJobID},
		"gate1_exact_params":   map[string]any{"passed": g1, "d_kind": dKind, "proof_scope": proofScope, "witness_sha256": witnessSha},
		"gate2_indecomposable": map[string]any{"passed": g2},
		"gate3_novel":          map[string]any{"passed": g3},
		"gate4_frontier":       map[string]any{"passed": g4, "sim_job_id": simJobID},
		"gate5_held_out":       map[string]any{"passed": g5, "held_out_sim_job_id": heldOutJobID},
		"gate6_reproduced":     map[string]any{"passed": g6},
	}

	artifactPayload := map[string]any{
		"claim_id":            claimID,
		"session_id":          sessionID,
		"code_name":           codeName,
		"eval_job_id":         evalJobID,
		"sim_job_id":          simJobID,
		"held_out_sim_job_id": heldOutJobID,
		"witness_sha256":      witnessSha,
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

// handleDiscoveryClaimsRoute exposes durable, session-scoped (N8),
// server-verified (N1) Schema-17 QEC discovery_claims lifecycle operations:
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
			var req struct {
				ClaimID         string `json:"claim_id"`
				CodeName        string `json:"code_name"`
				EvalJobID       string `json:"eval_job_id"`
				SimJobID        string `json:"sim_job_id"`
				HeldOutSimJobID string `json:"held_out_sim_job_id"`
			}
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
			if err := dec.Decode(&req); err != nil {
				WriteAPIError(w, "bad_request", "invalid discovery claim JSON", nil)
				return
			}
			if strings.TrimSpace(req.ClaimID) == "" || strings.TrimSpace(req.CodeName) == "" || strings.TrimSpace(req.EvalJobID) == "" {
				WriteAPIError(w, "bad_request", "claim_id, code_name, and settled eval_job_id are required (client-supplied gate flags and artifact hashes are not accepted)", nil)
				return
			}
			rec, err := s.deriveServerSideDiscoveryClaim(
				sessionID,
				strings.TrimSpace(req.ClaimID),
				strings.TrimSpace(req.CodeName),
				strings.TrimSpace(req.EvalJobID),
				strings.TrimSpace(req.SimJobID),
				strings.TrimSpace(req.HeldOutSimJobID),
			)
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
