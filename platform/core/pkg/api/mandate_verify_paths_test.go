package api_test

// Verify-path pins (R-fix 2026-10-17): /api/mandates/verify accepts exactly
// two attributable keys — the tenant's current admin key and the server
// reviewer key — and refuses a validly-signed mandate from any other key
// (the W0.12 threat: unattributed signatures spending a tenant's ceiling).

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

func postMandateVerify(t *testing.T, handler http.Handler, m *mandates.SpendMandate) (bool, string) {
	t.Helper()
	body, _ := json.Marshal(m)
	req := httptest.NewRequest(http.MethodPost, "/api/mandates/verify", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return resp.Valid, resp.Error
}

func TestMandateVerifyTwoPaths(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	handler := srv.Handler()

	tenantBody, _ := json.Marshal(map[string]any{"id": "tenant-verify-paths", "display_name": "Paths"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(tenantBody))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for /api/tenants, got %d", w.Code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	if _, err := srv.TenantManager().SetAdminKey("tenant-verify-paths", hex.EncodeToString(adminPub), nil); err != nil {
		t.Fatalf("admin bootstrap: %v", err)
	}
	expiry := time.Now().UTC().Add(time.Hour)

	// 1. Tenant-admin path: client-signed under the CURRENT admin key.
	adminMandate, err := mandates.SignMandate(adminPriv, "tenant-verify-paths", "job-paths", 10.0, expiry)
	if err != nil {
		t.Fatalf("sign admin mandate: %v", err)
	}
	if valid, verr := postMandateVerify(t, handler, adminMandate); !valid {
		t.Fatalf("admin-key mandate must verify valid: %s", verr)
	}

	// 2. Attacker path: valid signature, unattributed key — must refuse.
	_, attackerPriv, _ := ed25519.GenerateKey(nil)
	attackerMandate, err := mandates.SignMandate(attackerPriv, "tenant-verify-paths", "job-paths", 10.0, expiry)
	if err != nil {
		t.Fatalf("sign attacker mandate: %v", err)
	}
	if valid, verr := postMandateVerify(t, handler, attackerMandate); valid {
		t.Fatal("attacker-key mandate must NOT verify valid")
	} else if verr == "" {
		t.Fatal("attacker refusal must carry a reason")
	}

	// 3. Rotation invalidates the old admin key on the admin path.
	rotPub, rotPriv, _ := ed25519.GenerateKey(nil)
	rotPubHex := hex.EncodeToString(rotPub)
	rotParams := map[string]string{"new_admin_pubkey_hex": rotPubHex}
	rotHash, err := tenants.ParamsHashOf(rotParams)
	if err != nil {
		t.Fatalf("rotation params hash: %v", err)
	}
	rotAct := &tenants.AdminAction{
		TenantID: "tenant-verify-paths", Action: "set_admin_key", ParamsHash: rotHash,
		Expiry: time.Now().UTC().Add(time.Hour), PubkeyHex: hex.EncodeToString(adminPub),
	}
	rotSig := ed25519.Sign(adminPriv, rotAct.CanonicalPayload())
	rotAct.SignatureHex = hex.EncodeToString(rotSig)
	if _, err := srv.TenantManager().SetAdminKey("tenant-verify-paths", rotPubHex, rotAct); err != nil {
		t.Fatalf("admin rotation: %v", err)
	}
	if valid, _ := postMandateVerify(t, handler, adminMandate); valid {
		t.Fatal("pre-rotation admin mandate must NOT verify after rotation")
	}
	rotMandate, err := mandates.SignMandate(rotPriv, "tenant-verify-paths", "job-paths", 10.0, expiry)
	if err != nil {
		t.Fatalf("sign rotated mandate: %v", err)
	}
	if valid, verr := postMandateVerify(t, handler, rotMandate); !valid {
		t.Fatalf("rotated admin-key mandate must verify valid: %s", verr)
	}
}
