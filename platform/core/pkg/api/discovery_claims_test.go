package api_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func TestDiscoveryClaimsHTTPFlowAndEd25519Signoff(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	// 1. Create candidate claim
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/discovery-claims", "", map[string]any{
		"claim_id":            "dc_http_1",
		"code_name":           "steane_7_1_3",
		"evidence_tier":       "T1_VERIFIED",
		"credibility_tier":    "L4",
		"artifact_sha256":     "aaaa1111bbbb2222",
		"gates_digest":        "cccc3333dddd4444",
		"gates_0_to_6_passed": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create claim status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}

	// 2. Promote candidate -> verified
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status": "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// 3. Unsigned promotion to claimed must be refused (400)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status":              "claimed",
		"operator_pubkey_hex": "not-hex",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsigned claim status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}

	// 4. Signed Ed25519 promotion to claimed succeeds (200)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex, sigHex, err := qec.SignDiscoveryClaim("dc_http_1", "aaaa1111bbbb2222", "cccc3333dddd4444", priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/discovery-claims/dc_http_1/transition", "", map[string]any{
		"status":                 "claimed",
		"operator_pubkey_hex":    pubHex,
		"operator_signature_hex": sigHex,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("signed claim status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// 5. GET lists the durable claimed record
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/discovery-claims", "", nil)
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
