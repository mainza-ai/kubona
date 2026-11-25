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

	// Seed real settled QEC jobs directly in the ledger for session A
	db := srv.Ledger().DB()
	evalRes, _ := json.Marshal(map[string]any{
		"n": 7, "k": 1, "d": 3, "d_kind": "proved",
		"decomposable": false, "catalog_known": false,
		"certificate": map[string]any{
			"proof_scope":    "full_logical_group",
			"witness_sha256": "a41ac5bbc9a4221e3e2bf23863a5101949be0cbd7a6478fec1330703bae11d0f",
		},
	})
	simRes, _ := json.Marshal(map[string]any{
		"ler": 0.0046, "ci_low": 0.0037, "ci_high": 0.0056, "d_circ": 3,
	})
	heldOutRes, _ := json.Marshal(map[string]any{
		"ler": 0.0135, "ci_low": 0.0093, "ci_high": 0.0196, "d_circ": 3,
		"noise": map[string]any{"is_held_out": true, "calibration_snapshot_id": "cal_8f65e57d6f49"},
	})
	for _, row := range []struct {
		id, jtype string
		res       []byte
	}{
		{"job_eval_a", "qec_code_eval", evalRes},
		{"job_sim_a", "qec_memory_sim", simRes},
		{"job_ho_a", "qec_memory_sim", heldOutRes},
	} {
		if _, err := db.Exec(
			`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at) VALUES (?, ?, 'JOB_STATUS_COMPLETED', ?, '{}', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			row.id, sidA, row.jtype, string(row.res),
		); err != nil {
			t.Fatalf("seed job %s: %v", row.id, err)
		}
	}

	// 1. Create server-derived candidate claim from real settled session A jobs
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sidA+"/discovery-claims", "", map[string]any{
		"claim_id":            "dc_http_1",
		"code_name":           "steane_7_1_3",
		"eval_job_id":         "job_eval_a",
		"sim_job_id":          "job_sim_a",
		"held_out_sim_job_id": "job_ho_a",
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

	// 4. GET lists the durable claimed record in session A and empty in session B
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sidA+"/discovery-claims", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list claims status = %d, want 200", w.Code)
	}
	var payload struct {
		Claims []qec.DiscoveryClaimRecord `json:"claims"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(payload.Claims) != 1 || payload.Claims[0].Status != qec.ClaimClaimed {
		t.Fatalf("unexpected listed claims: %+v", payload.Claims)
	}
}
