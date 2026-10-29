package api_test

// Mandate-review queue: open obligations are the steady-state ops surface.
// Every pin is HTTP-level: list scoping (session vs bearer vs anonymous),
// close authority (owner session / owner-key signature / never bearer-alone),
// double-close idempotency, ghost handling without an oracle, and the
// ephemeral-signer prod refusal.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// queueDrill seeds two labs with sessions and one close-READY obligation
// each (owner + passing required case + authority). Returns server,
// cleanup, cookieA, cookieB, sessA id, obA id, obB id.
func queueDrill(t *testing.T) (*api.Server, func(), string, string, string, string, string) {
	t.Helper()
	srv, cleanup := setupTenantDrill(t)
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	cookieB := drillLogin(t, srv, "lab-b")
	sessA := drillSession(t, srv, "ops-a", "lab-a", "test-token")
	sessB := drillSession(t, srv, "ops-b", "lab-b", "test-token")
	obA := seedReadyObligation(t, srv, "tribunal-run", "run-a", "session:"+sessA)
	obB := seedReadyObligation(t, srv, "tribunal-run", "run-b", "session:"+sessB)
	_ = sessB
	return srv, cleanup, cookieA, cookieB, sessA, obA, obB
}

func seedReadyObligation(t *testing.T, srv *api.Server, kind, subject, owner string) string {
	t.Helper()
	ob, err := srv.Ledger().OpenObligation(kind, subject, []string{"evidence-ready"}, []string{"lab-recorder"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().SetObligationOwner(ob.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().RecordObligationCase(ob.ID, ledger.ObligationCase{
		CaseKind: "evidence-ready", Verdict: "pass", Authority: "lab-recorder",
	}); err != nil {
		t.Fatal(err)
	}
	return ob.ID
}

func queueGet(t *testing.T, srv *api.Server, cookie, token, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/obligations/open"+query, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("queue decode: %v: %s", err, w.Body.String())
	}
	return w.Code, out
}

func queueClose(t *testing.T, srv *api.Server, id, cookie, token, sigHex string) (int, map[string]any) {
	t.Helper()
	var body []byte
	if sigHex != "" {
		body, _ = json.Marshal(map[string]any{"signature_hex": sigHex})
	} else {
		body = []byte("{}")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/obligations/"+id, bytes.NewReader(body))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("close decode: %v: %s", err, w.Body.String())
	}
	return w.Code, out
}

func TestObligationsOpenScoping(t *testing.T) {
	srv, cleanup, cookieA, _, _, obA, _ := queueDrill(t)
	defer cleanup()

	// Lab session sees only its own lab's rows, with close-readiness.
	if code, out := queueGet(t, srv, cookieA, "", ""); code != http.StatusOK {
		t.Fatalf("lab-a list must 200, got %d", code)
	} else {
		obs := out["obligations"].([]any)
		if len(obs) != 1 {
			t.Fatalf("lab-a must see exactly 1 row, got %d", len(obs))
		}
		if obs[0].(map[string]any)["id"] != obA {
			t.Fatalf("lab-a must see obA, got %v", obs[0])
		}
		if ready, _ := obs[0].(map[string]any)["close_ready"].(bool); !ready {
			t.Fatalf("seeded obligation must report ready: %v", obs[0])
		}
	}

	// Bearer operator sees all.
	if code, out := queueGet(t, srv, "", "test-token", ""); code != http.StatusOK || int(out["count"].(float64)) != 2 {
		t.Fatalf("bearer must see 2 rows, got %d: %v", code, out)
	}

	// Owner filter narrows; ghost owner yields an empty list (no oracle).
	if code, out := queueGet(t, srv, "", "test-token", "?owner=session:ghost"); code != http.StatusOK || int(out["count"].(float64)) != 0 {
		t.Fatalf("ghost owner filter must 200-empty, got %d: %v", code, out)
	}

	// Subject-kind filter narrows.
	if code, out := queueGet(t, srv, "", "test-token", "?subject_kind=tribunal-run"); code != http.StatusOK || int(out["count"].(float64)) != 2 {
		t.Fatalf("kind filter must 200 with 2 rows, got %d: %v", code, out)
	}

	// Anonymous on a multi-lab store refuses (D-09 shaped).
	if code, _ := queueGet(t, srv, "", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anon multi-lab list must 401, got %d", code)
	}
}

func TestObligationCloseMatrix(t *testing.T) {
	srv, cleanup, cookieA, cookieB, sessA, obA, _ := queueDrill(t)
	defer cleanup()

	// Stranger lab cannot trigger.
	if code, _ := queueClose(t, srv, obA, cookieB, "", ""); code != http.StatusForbidden {
		t.Fatalf("stranger close must 403, got %d", code)
	}
	// Bearer alone is never sufficient.
	if code, _ := queueClose(t, srv, obA, "", "test-token", ""); code != http.StatusForbidden {
		t.Fatalf("bearer-alone close must 403, got %d", code)
	}
	// Ghost id refuses exactly like unowned (no oracle).
	if code, _ := queueClose(t, srv, "ob_ghost0000000000", cookieA, "", ""); code != http.StatusForbidden {
		t.Fatalf("ghost close must 403, got %d", code)
	}

	// Owner session closes a ready obligation.
	if code, out := queueClose(t, srv, obA, cookieA, "", ""); code != http.StatusOK || out["closed"] != true {
		t.Fatalf("owner close must 200-closed, got %d: %v", code, out)
	}
	// Double close is a 200 no-op with the sealed receipt posture.
	if code, out := queueClose(t, srv, obA, cookieA, "", ""); code != http.StatusOK || out["already_closed"] != true {
		t.Fatalf("re-close must 200 already_closed, got %d: %v", code, out)
	}

	// Unready obligation: 200 with reasons, no mutation.
	obU, err := srv.Ledger().OpenObligation("tribunal-run", "run-unready", []string{"evidence-ready"}, []string{"lab-recorder"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().SetObligationOwner(obU.ID, "session:"+sessA); err != nil {
		t.Fatal(err)
	}
	if code, out := queueClose(t, srv, obU.ID, cookieA, "", ""); code != http.StatusOK || out["closed"] != false {
		t.Fatalf("unready close must 200-open with reasons, got %d: %v", code, out)
	} else if rs, _ := out["reasons"].([]any); len(rs) == 0 {
		t.Fatalf("unready close must name reasons: %v", out)
	}
}

func TestObligationCloseByOwnerKey(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")

	// Key-owned obligation (admin pubkey owner, e.g. mandate obligations).
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	ob, err := srv.Ledger().OpenObligation("mandate", "receipt-1", []string{"settlement"}, []string{"ledger-receipt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().SetObligationOwner(ob.ID, pubHex); err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().RecordObligationCase(ob.ID, ledger.ObligationCase{
		CaseKind: "settlement", Verdict: "pass", Authority: "ledger-receipt",
	}); err != nil {
		t.Fatal(err)
	}

	// Wrong key refuses.
	_, evilPriv, _ := ed25519.GenerateKey(rand.Reader)
	evilSig := hex.EncodeToString(ed25519.Sign(evilPriv, []byte("obligation-close\n"+ob.ID)))
	if code, _ := queueClose(t, srv, ob.ID, "", "test-token", evilSig); code != http.StatusForbidden {
		t.Fatalf("wrong-key close must 403, got %d", code)
	}
	// Owner key signature closes (bearer transport, signature authority).
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte("obligation-close\n"+ob.ID)))
	if code, out := queueClose(t, srv, ob.ID, "", "test-token", sig); code != http.StatusOK || out["closed"] != true {
		t.Fatalf("owner-key close must 200-closed, got %d: %v", code, out)
	}
}

func TestEphemeralSignerProdRefusal(t *testing.T) {
	signWith := func(t *testing.T, srv *api.Server, admin *signTestAdmin) int {
		t.Helper()
		body := mandateSignBody(t, "default", "job-ephemeral-drill", 5.0, 3600, admin, "mandate_sign")
		return authedSignRequest(t, srv, body, "test-token").Code
	}
	bootstrap := func(t *testing.T, srv *api.Server) *signTestAdmin {
		t.Helper()
		admin := newSignTestAdmin(t)
		if _, err := srv.TenantManager().SetAdminKey("default", admin.pub, nil); err != nil {
			t.Fatal(err)
		}
		return admin
	}

	// Production + ephemeral flag: the flag's own message says local-dev
	// only — prod refuses regardless of the flag. (setupTenantDrill pins
	// KUBONA_API_TOKEN=test-token; the bearer value is transport only.)
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	if code := signWith(t, srv, bootstrap(t, srv)); code != http.StatusForbidden {
		t.Fatalf("prod ephemeral sign must 403, got %d", code)
	}

	// Commissioned reviewer key: prod signs fine (fail-closed only when
	// uncommissioned). Seed bytes -> NewServer reads KUBONA_REVIEWER_PRIVKEY_HEX.
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBONA_REVIEWER_PRIVKEY_HEX", hex.EncodeToString(seed))
	srv2, cleanup2 := setupTenantDrill(t)
	defer cleanup2()
	if code := signWith(t, srv2, bootstrap(t, srv2)); code != http.StatusOK {
		t.Fatalf("prod commissioned sign must 200, got %d", code)
	}
}
