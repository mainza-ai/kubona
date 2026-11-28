package api_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func TestDiscoveryClaimsHTTPFlowAndEd25519Signoff(t *testing.T) {
	// Commission a real tenant admin Ed25519 keypair for the default tenant
	adminPub, adminPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen admin: %v", err)
	}
	t.Setenv("KUBONA_TENANT_ADMIN_PUBKEY_HEX", hex.EncodeToString(adminPub))

	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sidA := createSession(t, srv, "default")
	sidB := createSession(t, srv, "default")

	// N1: Forged claim without settled session jobs MUST be refused (400)
	wForge := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":            "forge_demo_001",
		"code_name":           "forged_code",
		"artifact_sha256":     "0000000000000000000000000000000000000000000000000000000000000000",
		"gates_digest":        "1111111111111111111111111111111111111111111111111111111111111111",
		"gates_0_to_6_passed": true,
	})
	if wForge.Code != http.StatusBadRequest {
		t.Fatalf("forged claim status = %d, want 400 (body=%s)", wForge.Code, wForge.Body.String())
	}

	// Seed real settled QEC jobs in session A
	db := srv.Ledger().DB()
	evalNovel, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96",
		"n":         96, "k": 12, "d": 8, "d_kind": "proved",
		"decomposable": false, "catalog_known": false,
		"catalog_relation": "novel_to_catalog",
		"canonical_hash":   "canon_96_abcdef",
		"certificate": map[string]any{
			"proof_scope":    "full_logical_group",
			"witness_sha256": "a41ac5bbc9a4221e3e2bf23863a5101949be0cbd7a6478fec1330703bae11d0f",
		},
	})
	evalUnchecked, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96",
		"n":         96, "k": 12, "d": 8, "d_kind": "proved",
		"decomposable": false, "catalog_known": false,
		"catalog_relation": "unchecked",
		"canonical_hash":   "",
		"certificate": map[string]any{
			"proof_scope":    "full_logical_group",
			"witness_sha256": "a41ac5bbc9a4221e3e2bf23863a5101949be0cbd7a6478fec1330703bae11d0f",
		},
	})
	simCand, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96", "total_qubits": 192, "rounds": 6, "basis": "z", "metric_kind": "ler_per_round",
		"noise": map[string]any{"model": "si1000", "p_2q": 0.001},
		"ler": 0.0020, "ci_low": 0.0014, "ci_high": 0.0028, "d_circ": 6, "seed": 7,
	})
	simBase, _ := json.Marshal(map[string]any{
		"code_name": "surface_d5", "total_qubits": 200, "rounds": 6, "basis": "z", "metric_kind": "ler_per_round",
		"noise": map[string]any{"model": "si1000", "p_2q": 0.001},
		"ler": 0.0080, "ci_low": 0.0065, "ci_high": 0.0098, "d_circ": 5, "seed": 7,
	})
	simBaseWrongP, _ := json.Marshal(map[string]any{
		"code_name": "surface_d5", "total_qubits": 200, "rounds": 6, "basis": "z", "metric_kind": "ler_per_round",
		"noise": map[string]any{"model": "si1000", "p_2q": 0.005},
		"ler": 0.0800, "ci_low": 0.0650, "ci_high": 0.0980, "d_circ": 5, "seed": 7,
	})
	simRepro, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96", "total_qubits": 192, "rounds": 6, "basis": "z", "metric_kind": "ler_per_round",
		"noise": map[string]any{"model": "si1000", "p_2q": 0.001},
		"ler": 0.0022, "ci_low": 0.0015, "ci_high": 0.0030, "d_circ": 6, "seed": 11,
	})
	simReproNoSeed, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96", "total_qubits": 192, "rounds": 6, "basis": "z", "metric_kind": "ler_per_round",
		"noise": map[string]any{"model": "si1000", "p_2q": 0.001},
		"ler": 0.0022, "ci_low": 0.0015, "ci_high": 0.0030, "d_circ": 6,
	})
	hoCand, _ := json.Marshal(map[string]any{
		"code_name": "novel_bb_96", "total_qubits": 192, "rounds": 6,
		"ler": 0.0040, "ci_low": 0.0030, "ci_high": 0.0052, "d_circ": 6,
		"noise": map[string]any{"is_held_out": true, "calibration_snapshot_id": "cal_8f65e57d6f49", "model": "heron", "p_2q": 0.002},
	})
	hoBase, _ := json.Marshal(map[string]any{
		"code_name": "surface_d5", "total_qubits": 200, "rounds": 6,
		"ler": 0.0135, "ci_low": 0.0110, "ci_high": 0.0165, "d_circ": 5,
		"noise": map[string]any{"is_held_out": true, "calibration_snapshot_id": "cal_8f65e57d6f49", "model": "heron", "p_2q": 0.002},
	})
	for _, row := range []struct {
		id, jtype string
		res       []byte
	}{
		{"job_eval_a", "qec_code_eval", evalNovel},
		{"job_eval_unchecked", "qec_code_eval", evalUnchecked},
		{"job_sim_cand", "qec_memory_sim", simCand},
		{"job_sim_base", "qec_memory_sim", simBase},
		{"job_sim_base_wrong_p", "qec_memory_sim", simBaseWrongP},
		{"job_sim_repro", "qec_memory_sim", simRepro},
		{"job_sim_repro_noseed", "qec_memory_sim", simReproNoSeed},
		{"job_ho_cand", "qec_memory_sim", hoCand},
		{"job_ho_base", "qec_memory_sim", hoBase},
	} {
		if _, err := db.Exec(
			`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, 'JOB_STATUS_COMPLETED', ?, '{}', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			row.id, sidA, row.jtype, string(row.res),
		); err != nil {
			t.Fatalf("seed job %s: %v", row.id, err)
		}
	}

	// N10: check_catalog=false (relation="unchecked") MUST NOT pass Gate 3
	wUnchecked := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":                     "dc_unchecked",
		"code_name":                    "novel_bb_96",
		"eval_job_id":                  "job_eval_unchecked",
		"sim_job_id":                   "job_sim_cand",
		"baseline_sim_job_id":          "job_sim_base",
		"held_out_sim_job_id":          "job_ho_cand",
		"held_out_baseline_sim_job_id": "job_ho_base",
		"repro_sim_job_id":             "job_sim_repro",
	})
	var uncheckedResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wUnchecked.Body.Bytes(), &uncheckedResp)
	if uncheckedResp.Claim.Gates0To6Passed {
		t.Fatalf("N10 violated: unchecked catalog passed gates 0-6: %+v", uncheckedResp.Claim)
	}

	// N9: Attaching surface_d5 sim_job_id to a novel_bb_96 claim MUST fail Gate 4 (code_name_mismatch)
	wMismatch := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":                     "dc_mismatch",
		"code_name":                    "novel_bb_96",
		"eval_job_id":                  "job_eval_a",
		"sim_job_id":                   "job_sim_base",
		"baseline_sim_job_id":          "job_sim_cand",
		"held_out_sim_job_id":          "job_ho_cand",
		"held_out_baseline_sim_job_id": "job_ho_base",
		"repro_sim_job_id":             "job_sim_repro",
	})
	var mismatchResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wMismatch.Body.Bytes(), &mismatchResp)
	if mismatchResp.Claim.Gates0To6Passed {
		t.Fatalf("N9 violated: mismatched code_name sim passed gates 0-6: %+v", mismatchResp.Claim)
	}

	// M0 (Q-A1): Mismatched p_physical between candidate and baseline MUST fail Gate 4
	wWrongP := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":                     "dc_wrong_p",
		"code_name":                    "novel_bb_96",
		"eval_job_id":                  "job_eval_a",
		"sim_job_id":                   "job_sim_cand",
		"baseline_sim_job_id":          "job_sim_base_wrong_p",
		"held_out_sim_job_id":          "job_ho_cand",
		"held_out_baseline_sim_job_id": "job_ho_base",
		"repro_sim_job_id":             "job_sim_repro",
	})
	var wrongPResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wWrongP.Body.Bytes(), &wrongPResp)
	if wrongPResp.Claim.Gates0To6Passed {
		t.Fatalf("M0 Q-A1 violated: mismatched p_physical baseline passed Gate 4: %+v", wrongPResp.Claim)
	}

	// M0 (Q-A2): Missing seed on reproduction sim MUST fail Gate 6 (fail-closed)
	wNoSeed := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":                     "dc_noseed",
		"code_name":                    "novel_bb_96",
		"eval_job_id":                  "job_eval_a",
		"sim_job_id":                   "job_sim_cand",
		"baseline_sim_job_id":          "job_sim_base",
		"held_out_sim_job_id":          "job_ho_cand",
		"held_out_baseline_sim_job_id": "job_ho_base",
		"repro_sim_job_id":             "job_sim_repro_noseed",
	})
	var noSeedResp struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	_ = json.Unmarshal(wNoSeed.Body.Bytes(), &noSeedResp)
	if noSeedResp.Claim.Gates0To6Passed {
		t.Fatalf("M0 Q-A2 violated: missing seed repro passed Gate 6: %+v", noSeedResp.Claim)
	}

	// 1. Valid server-derived candidate claim with bound jobs, baselines, and distinct-seed repro
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":                     "dc_http_1",
		"code_name":                    "novel_bb_96",
		"eval_job_id":                  "job_eval_a",
		"sim_job_id":                   "job_sim_cand",
		"baseline_sim_job_id":          "job_sim_base",
		"held_out_sim_job_id":          "job_ho_cand",
		"held_out_baseline_sim_job_id": "job_ho_base",
		"repro_sim_job_id":             "job_sim_repro",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create claim status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	var created struct {
		Claim qec.DiscoveryClaimRecord `json:"claim"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if !created.Claim.Gates0To6Passed || len(created.Claim.ArtifactSHA256) != 64 || len(created.Claim.GatesDigest) != 64 {
		t.Fatalf("expected server-derived gates + sha256 digests, got %+v", created.Claim)
	}

	// N8: Cross-session IDOR transition from session B MUST be refused (404)
	wIDOR := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidB+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status": "verified",
	})
	if wIDOR.Code != http.StatusNotFound {
		t.Fatalf("cross-session IDOR transition status = %d, want 404 (body=%s)", wIDOR.Code, wIDOR.Body.String())
	}

	// 2. Promote candidate -> verified inside owning session A
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status": "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// N1: Fresh untrusted Ed25519 keypair signing the real digests MUST be refused (403)
	_, freshPriv, _ := ed25519.GenerateKey(rand.Reader)
	freshPubHex, freshSigHex, _ := qec.SignDiscoveryClaim(created.Claim.ClaimID, created.Claim.ArtifactSHA256, created.Claim.GatesDigest, freshPriv)
	wUntrusted := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status":                 "claimed",
		"operator_pubkey_hex":    freshPubHex,
		"operator_signature_hex": freshSigHex,
	})
	if wUntrusted.Code != http.StatusForbidden {
		t.Fatalf("untrusted fresh keypair status = %d, want 403 (body=%s)", wUntrusted.Code, wUntrusted.Body.String())
	}

	// 3. Commissioned tenant admin Ed25519 signature promotes verified -> claimed (200)
	pubHex, sigHex, err := qec.SignDiscoveryClaim(created.Claim.ClaimID, created.Claim.ArtifactSHA256, created.Claim.GatesDigest, adminPriv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status":                 "claimed",
		"operator_pubkey_hex":    pubHex,
		"operator_signature_hex": sigHex,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("commissioned admin signed claim status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}
