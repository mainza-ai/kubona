package api_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func TestCodesignRunsDurabilityAndCircuitClaimLifecycle(t *testing.T) {
	adminPub, adminPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	t.Setenv("KUBONA_TENANT_ADMIN_PUBKEY_HEX", hex.EncodeToString(adminPub))

	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sidA := createSession(t, srv, "default")
	sidB := createSession(t, srv, "default")

	knownCCXSum := sha256.Sum256([]byte(`[["ccx",[0,1,2],[]]]`))
	knownCCXHash := hex.EncodeToString(knownCCXSum[:])

	// Seed durable codesign_runs in session A: novel candidate, heavier baseline, distinct-seed repro + held-out, non-exhaustive, known textbook CCX, tampered TQ
	for _, run := range []ledger.CodesignRunRecord{
		{
			RunID: "cs_eval_1", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_novel_001",
			Kappa: 1, Seed: 7, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_base_1", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_base",
			Kappa: 1, Seed: 7, NumTests: 16, Valid: true, Toffoli: 2, PeakQubits: 4, TQ: 8,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 2, ToffoliDepth: 2,
		},
		{
			RunID: "cs_repro_1", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_novel_001",
			Kappa: 1, Seed: 11, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_held_1", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_novel_001",
			Kappa: 1, Seed: 19, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_known_ccx", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: knownCCXHash,
			Kappa: 1, Seed: 7, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_known_repro", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: knownCCXHash,
			Kappa: 1, Seed: 11, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_known_held", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: knownCCXHash,
			Kappa: 1, Seed: 19, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_bad_tq", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_novel_001",
			Kappa: 1, Seed: 7, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 99,
			EpsBound: 0.0, VerificationMethod: "exhaustive", Depth: 1, ToffoliDepth: 1,
		},
		{
			RunID: "cs_stat_only", SessionID: sidA, SpecKind: "and2", ArtifactSHA256: "art_hash_stat",
			Kappa: 1, Seed: 7, NumTests: 16, Valid: true, Toffoli: 1, PeakQubits: 3, TQ: 3,
			EpsBound: 0.05, VerificationMethod: "statistical", Depth: 1, ToffoliDepth: 1,
		},
	} {
		if err := srv.Ledger().SaveCodesignRun(run); err != nil {
			t.Fatalf("save run %s: %v", run.RunID, err)
		}
	}

	// Phase 0: GET /api/codesign/runs/{id}?session_id=... returns run in owning session, 404 in another session
	wGet := doJSON(t, srv, http.MethodGet, "/api/codesign/runs/cs_eval_1?session_id="+sidA, "", nil)
	if wGet.Code != http.StatusOK {
		t.Fatalf("get run status = %d: %s", wGet.Code, wGet.Body.String())
	}
	wIDOR := doJSON(t, srv, http.MethodGet, "/api/codesign/runs/cs_eval_1?session_id="+sidB, "", nil)
	if wIDOR.Code != http.StatusNotFound {
		t.Fatalf("cross-session get run status = %d, want 404", wIDOR.Code)
	}

	// Phase 1 (Decision A2): Statistical-only run MUST NOT pass gates 0-6
	wStat := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":        "cc_stat_refuse",
		"kind":            "circuit",
		"code_name":       "and2_stat",
		"eval_run_id":     "cs_stat_only",
		"baseline_run_id": "cs_base_1",
		"repro_run_id":    "cs_repro_1",
	})
	var statResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wStat.Body.Bytes(), &statResp)
	if statResp.Claim.Gates0To6Passed {
		t.Fatalf("Decision A2 violated: statistical run passed circuit gates: %+v", statResp.Claim)
	}

	// C1 fix: Missing held_out_run_id (C5) MUST NOT emit T1_VERIFIED
	wNoHeld := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":        "cc_no_heldout",
		"kind":            "circuit",
		"code_name":       "and2_novel",
		"eval_run_id":     "cs_eval_1",
		"baseline_run_id": "cs_base_1",
		"repro_run_id":    "cs_repro_1",
	})
	var noHeldResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wNoHeld.Body.Bytes(), &noHeldResp)
	if noHeldResp.Claim.Gates0To6Passed || noHeldResp.Claim.EvidenceTier == "T1_VERIFIED" {
		t.Fatalf("C1 violated: missing held-out run emitted T1_VERIFIED: %+v", noHeldResp.Claim)
	}
	if !strings.Contains(noHeldResp.Claim.GateReportJSON, "missing_held_out_run") {
		t.Fatalf("expected missing_held_out_run in gate report, got: %s", noHeldResp.Claim.GateReportJSON)
	}

	// C1 fix: Known textbook CCX AND2 circuit MUST fail C6 novelty gate
	wKnown := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":        "cc_known_refuse",
		"kind":            "circuit",
		"code_name":       "and2_textbook",
		"eval_run_id":     "cs_known_ccx",
		"baseline_run_id": "cs_base_1",
		"repro_run_id":    "cs_known_repro",
		"held_out_run_id": "cs_known_held",
	})
	var knownResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wKnown.Body.Bytes(), &knownResp)
	if knownResp.Claim.Gates0To6Passed || knownResp.Claim.EvidenceTier == "T1_VERIFIED" {
		t.Fatalf("C1/C6 violated: textbook CCX AND2 passed novelty gate: %+v", knownResp.Claim)
	}
	if !strings.Contains(knownResp.Claim.GateReportJSON, "known_published_circuit:and2_single_ccx") {
		t.Fatalf("expected known_published_circuit:and2_single_ccx in gate report, got: %s", knownResp.Claim.GateReportJSON)
	}

	// C8 fix: Tampered TQ (99 != 1 * 3^1) MUST fail C2 cost gate
	wBadTQ := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":        "cc_bad_tq",
		"kind":            "circuit",
		"code_name":       "and2_bad_tq",
		"eval_run_id":     "cs_bad_tq",
		"baseline_run_id": "cs_base_1",
		"repro_run_id":    "cs_repro_1",
		"held_out_run_id": "cs_held_1",
	})
	var badTQResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wBadTQ.Body.Bytes(), &badTQResp)
	if badTQResp.Claim.Gates0To6Passed || !strings.Contains(badTQResp.Claim.GateReportJSON, "tq_mismatch") {
		t.Fatalf("C8 violated: tampered TQ passed C2: %+v", badTQResp.Claim)
	}

	// Full C0-C6 novel circuit claim passes, verifies, and signs with commissioned Ed25519 admin key
	wCreate := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":        "cc_and2_1",
		"kind":            "circuit",
		"code_name":       "and2_novel",
		"eval_run_id":     "cs_eval_1",
		"baseline_run_id": "cs_base_1",
		"repro_run_id":    "cs_repro_1",
		"held_out_run_id": "cs_held_1",
	})
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create circuit claim status = %d: %s", wCreate.Code, wCreate.Body.String())
	}
	var created struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wCreate.Body.Bytes(), &created)
	if created.Claim.Kind != "circuit" || !created.Claim.Gates0To6Passed {
		t.Fatalf("expected passing circuit claim, got %+v", created.Claim)
	}

	wVer := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims/cc_and2_1/transition", "", map[string]any{
		"status": "verified",
	})
	if wVer.Code != http.StatusOK {
		t.Fatalf("verify circuit claim = %d: %s", wVer.Code, wVer.Body.String())
	}

	pubHex, sigHex, _ := qec.SignDiscoveryClaim(created.Claim.ClaimID, created.Claim.ArtifactSHA256, created.Claim.GatesDigest, adminPriv)
	wClaim := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims/cc_and2_1/transition", "", map[string]any{
		"status":                 "claimed",
		"operator_pubkey_hex":    pubHex,
		"operator_signature_hex": sigHex,
	})
	if wClaim.Code != http.StatusOK {
		t.Fatalf("signed circuit claim = %d: %s", wClaim.Code, wClaim.Body.String())
	}
}
