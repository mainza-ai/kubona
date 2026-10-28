package api_test

// R2b HTTP test: verifying a mandate opens its closure obligation
// (receipt-scoped, admin-owned); re-verifying the SAME signed mandate is
// an idempotent replay (one receipt, one obligation — public spam cannot
// grow the ledger).

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupMandateServer(t *testing.T) (http.Handler, *ledger.Ledger, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	l, err := ledger.OpenLedger(t.TempDir() + "/mandate.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	return srv.Handler(), l, func() { l.Close() }
}

func verifyMandate(t *testing.T, h http.Handler, tenantID, pubHex string, priv ed25519.PrivateKey) map[string]any {
	t.Helper()
	// Client-signed under the tenant admin key (admin path — the
	// obligation owner is the admin, not the reviewer).
	adminMandate, err := mandates.SignMandate(priv, tenantID, "job-sha256-r2b", 25.0, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, _ := json.Marshal(adminMandate)
	req := httptest.NewRequest(http.MethodPost, "/api/mandates/verify", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("verify decode: %v", err)
	}
	if out["valid"] != true {
		t.Fatalf("mandate must verify valid: %v", out)
	}
	return out
}

func TestVerifyOpensMandateObligation(t *testing.T) {
	h, l, close := setupMandateServer(t)
	defer close()

	code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-r2b", "display_name": "R2B"})
	if code != http.StatusCreated {
		t.Fatalf("tenant create: %d", code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := hex.EncodeToString(adminPub)
	mgr := tenants.NewManager(l)
	if _, err := mgr.SetAdminKey("lab-r2b", adminHex, nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	out := verifyMandate(t, h, "lab-r2b", adminHex, adminPriv)
	receipt, _ := out["receipt"].(string)
	if receipt == "" {
		t.Fatalf("verify response must carry the receipt hash (clients cite it as mandate_ref): %v", out)
	}
	ob, err := l.GetObligationBySubject("mandate", receipt)
	if err != nil || ob == nil {
		t.Fatalf("verify must open the receipt obligation: %+v %v", ob, err)
	}
	if ob.Owner != "admin:"+adminHex {
		t.Fatalf("obligation must be admin-owned, got %q", ob.Owner)
	}
	if ob.Disposition != "open" {
		t.Fatalf("fresh obligation must be open, got %q", ob.Disposition)
	}
	// Re-verify: identical signed bytes replay the same receipt/obligation.
	out2 := verifyMandate(t, h, "lab-r2b", adminHex, adminPriv)
	receipt2, _ := out2["receipt"].(string)
	if receipt2 != receipt {
		t.Fatalf("re-verify must replay the same receipt: %q vs %q", receipt2, receipt)
	}
	if out2["replayed"] != true {
		t.Fatalf("re-verify must mark the replay: %v", out2)
	}
	var n int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE event_type = 'mandate_verified_client'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("replay must not append a second receipt, got %d", n)
	}
}
