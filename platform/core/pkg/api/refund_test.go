package api_test

// Refund route tests: bearer + admin envelope + typed refusals; money
// moves only with a who and a why.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

func refundAction(t *testing.T, tenantID, pubHex string, priv ed25519.PrivateKey, amount float64, reason string) *tenants.AdminAction {
	t.Helper()
	params := map[string]any{"tenant_id": tenantID, "amount_usd": amount, "reason": reason}
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatal(err)
	}
	act := &tenants.AdminAction{
		TenantID: tenantID, Action: "refund_spend", ParamsHash: hash,
		Expiry: time.Now().UTC().Add(time.Hour), PubkeyHex: pubHex,
	}
	sig := ed25519.Sign(priv, act.CanonicalPayload())
	act.SignatureHex = hex.EncodeToString(sig)
	return act
}

func postRefund(t *testing.T, h http.Handler, bearer bool, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/refund", bytes.NewReader(raw))
	if bearer {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestTenantRefundRoute(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()

	// Register + fund a lab: create, bootstrap admin, burn $40.
	code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-cash", "display_name": "Cash"})
	if code != http.StatusCreated {
		t.Fatalf("tenant create: %d", code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := hex.EncodeToString(adminPub)
	if _, err := srv.TenantManager().SetAdminKey("lab-cash", adminHex, nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Burn spend via a ceiling-capped reservation path: use the manager
	// directly (routes never mint bare spend).
	if err := srv.TenantManager().ReserveSpend("lab-cash", 40); err != nil {
		t.Fatalf("spend: %v", err)
	}

	good := map[string]any{
		"tenant_id": "lab-cash", "amount_usd": 15.0, "reason": "failed job credit",
		"action": refundAction(t, "lab-cash", adminHex, adminPriv, 15.0, "failed job credit"),
	}
	// No bearer → 401 typed.
	if code, out := postRefund(t, h, false, good); code != http.StatusUnauthorized || out["code"] != "unauthorized" {
		t.Fatalf("bearerless refund must 401 typed, got %d %v", code, out)
	}
	// Happy path.
	code, out := postRefund(t, h, true, good)
	if code != http.StatusOK {
		t.Fatalf("refund: %d %v", code, out)
	}
	if out["spent_usd"] != 25.0 {
		t.Fatalf("spent must drop to 25, got %v", out)
	}
	// Tampered amount (envelope binds 15, body says 5) → 403.
	tampered := map[string]any{
		"tenant_id": "lab-cash", "amount_usd": 5.0, "reason": "failed job credit",
		"action": refundAction(t, "lab-cash", adminHex, adminPriv, 15.0, "failed job credit"),
	}
	if code, out := postRefund(t, h, true, tampered); code != http.StatusForbidden {
		t.Fatalf("param-swapped refund must 403, got %d %v", code, out)
	}
	// Over-refund → 403, books unmoved.
	over := map[string]any{
		"tenant_id": "lab-cash", "amount_usd": 1000.0, "reason": "greedy",
		"action": refundAction(t, "lab-cash", adminHex, adminPriv, 1000.0, "greedy"),
	}
	if code, _ := postRefund(t, h, true, over); code != http.StatusForbidden {
		t.Fatalf("over-refund must 403, got %d", code)
	}
	got, _ := srv.TenantManager().GetTenant("lab-cash")
	if got.SpentUSD != 25.0 {
		t.Fatalf("refusals must not move books, spent=%v", got.SpentUSD)
	}
}

func refundSessionAction(t *testing.T, tenantID, sessionID, pubHex string, priv ed25519.PrivateKey, amount float64, reason string) *tenants.AdminAction {
	t.Helper()
	params := map[string]any{"tenant_id": tenantID, "session_id": sessionID, "amount_usd": amount, "reason": reason}
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatal(err)
	}
	act := &tenants.AdminAction{
		TenantID: tenantID, Action: "session_refund_spend", ParamsHash: hash,
		Expiry: time.Now().UTC().Add(time.Hour), PubkeyHex: pubHex,
	}
	sig := ed25519.Sign(priv, act.CanonicalPayload())
	act.SignatureHex = hex.EncodeToString(sig)
	return act
}

func postSessionRefund(t *testing.T, h http.Handler, bearer bool, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/refund", bytes.NewReader(raw))
	if bearer {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestSessionRefundRoute(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()

	// Register + fund a lab session: tenant, admin, session, $40 spend.
	if code, _, _ := idemPost(t, h, "/api/tenants", map[string]any{"id": "lab-sess", "display_name": "Sess"}); code != http.StatusCreated {
		t.Fatalf("tenant create: %d", code)
	}
	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminHex := hex.EncodeToString(adminPub)
	if _, err := srv.TenantManager().SetAdminKey("lab-sess", adminHex, nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	sessRaw, _ := json.Marshal(map[string]any{"title": "refund drill", "tenant_id": "lab-sess"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessRaw))
	sessReq.Header.Set("Authorization", "Bearer test-token")
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sess struct {
		ID              string  `json:"id"`
		CurrentSpendUSD float64 `json:"current_spend_usd"`
	}
	if err := json.Unmarshal(sessW.Body.Bytes(), &sess); err != nil || sess.ID == "" {
		t.Fatalf("session create: %s", sessW.Body.String())
	}
	if err := srv.SessionManager().ReserveSpend(sess.ID, 40); err != nil {
		t.Fatalf("spend: %v", err)
	}

	good := map[string]any{
		"session_id": sess.ID, "amount_usd": 15.0, "reason": "vendor error, job never ran",
		"action": refundSessionAction(t, "lab-sess", sess.ID, adminHex, adminPriv, 15.0, "vendor error, job never ran"),
	}
	// No bearer → 401 typed.
	if code, out := postSessionRefund(t, h, false, good); code != http.StatusUnauthorized || out["code"] != "unauthorized" {
		t.Fatalf("bearerless session refund must 401 typed, got %d %v", code, out)
	}
	// Happy path.
	code, out := postSessionRefund(t, h, true, good)
	if code != http.StatusOK {
		t.Fatalf("session refund: %d %v", code, out)
	}
	if out["current_spend_usd"] != 25.0 {
		t.Fatalf("spent must drop to 25, got %v", out)
	}
	// Tampered amount (envelope binds 15, body says 5) → 403.
	tampered := map[string]any{
		"session_id": sess.ID, "amount_usd": 5.0, "reason": "vendor error, job never ran",
		"action": refundSessionAction(t, "lab-sess", sess.ID, adminHex, adminPriv, 15.0, "vendor error, job never ran"),
	}
	if code, _ := postSessionRefund(t, h, true, tampered); code != http.StatusForbidden {
		t.Fatalf("param-swapped session refund must 403, got %d", code)
	}
	// Over-refund → 403, books unmoved.
	over := map[string]any{
		"session_id": sess.ID, "amount_usd": 1000.0, "reason": "greedy",
		"action": refundSessionAction(t, "lab-sess", sess.ID, adminHex, adminPriv, 1000.0, "greedy"),
	}
	if code, _ := postSessionRefund(t, h, true, over); code != http.StatusForbidden {
		t.Fatalf("over-refund must 403, got %d", code)
	}
	got, _ := srv.SessionManager().GetSession(sess.ID)
	if got.CurrentSpendUSD != 25.0 {
		t.Fatalf("refusals must not move books, spent=%v", got.CurrentSpendUSD)
	}
	// F-NEW-07: both session_spend_refunded and session_spend_refund_refused
	// must be registered in KnownEventTypes and sealed in the session ledger
	// so QuantumDispatch's refund history table sees them.
	events, err := srv.Ledger().GetRecentEvents(sess.ID, 20)
	if err != nil {
		t.Fatalf("GetRecentEvents: %v", err)
	}
	var sawRefunded, sawRefused bool
	for _, ev := range events {
		switch ev.EventType {
		case "session_spend_refunded":
			sawRefunded = true
		case "session_spend_refund_refused":
			sawRefused = true
		}
	}
	if !sawRefunded || !sawRefused {
		t.Fatalf("expected both session_spend_refunded (%v) and session_spend_refund_refused (%v) in ledger events", sawRefunded, sawRefused)
	}
}
