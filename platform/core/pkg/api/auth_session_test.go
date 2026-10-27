package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	_ "github.com/mattn/go-sqlite3"
)

// authTestSetup creates tenant lab-auth and bootstraps the given admin key.
func authTestSetup(t *testing.T, srv *api.Server, adminPubHex string) {
	t.Helper()
	// create tenant (public, rate-limited)
	body, _ := json.Marshal(map[string]any{"id": "lab-auth", "display_name": "Auth Lab"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Fatalf("tenant create: got %d", w.Code)
	}
	// bootstrap admin key (first write wins; bearer proves operator)
	boot, _ := json.Marshal(map[string]any{"tenant_id": "lab-auth", "new_admin_pubkey_hex": adminPubHex})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(boot))
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin bootstrap: got %d: %s", w.Code, w.Body.String())
	}
}

func authChallenge(t *testing.T, srv *api.Server) (nonce, message string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/challenge", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("challenge: got %d", w.Code)
	}
	var out struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Nonce == "" {
		t.Fatalf("challenge malformed: %v", err)
	}
	return out.Nonce, out.Message
}

func authLogin(t *testing.T, srv *api.Server, tenant, pubHex string, priv ed25519.PrivateKey, nonce string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	sig := ed25519.Sign(priv, []byte("kubona-auth/v1 login "+nonce))
	body, _ := json.Marshal(map[string]any{
		"tenant_id": tenant, "pubkey_hex": pubHex, "nonce": nonce,
		"signature_hex": hex.EncodeToString(sig),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	cookie := ""
	for _, c := range w.Result().Cookies() {
		if c.Name == "kubona_session" {
			cookie = c.Value
		}
	}
	return w, cookie
}

func TestAuthLoginFlow(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	authTestSetup(t, srv, pubHex)

	nonce, _ := authChallenge(t, srv)
	w, cookie := authLogin(t, srv, "lab-auth", pubHex, priv, nonce)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d: %s", w.Code, w.Body.String())
	}
	if cookie == "" {
		t.Fatal("login must set the session cookie")
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "kubona_session" && !c.HttpOnly {
			t.Fatal("session cookie must be httpOnly")
		}
	}

	// whoami reflects the session.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/whoami", nil)
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var who struct {
		Authenticated bool   `json:"authenticated"`
		TenantID      string `json:"tenant_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &who); err != nil || !who.Authenticated || who.TenantID != "lab-auth" {
		t.Fatalf("whoami must show the lab: %v %s", err, w.Body.String())
	}

	// A bearer-gated route accepts the cookie (no bearer header at all).
	abort, _ := json.Marshal(map[string]any{"session_id": "ghost"})
	req = httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abort))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusServiceUnavailable {
		t.Fatalf("session cookie must authorize gated routes, got %d", w.Code)
	}

	// Logout revokes: whoami goes false and the cookie stops working.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: got %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/auth/whoami", nil)
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var who2 struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &who2); err != nil || who2.Authenticated {
		t.Fatal("revoked session must not authenticate")
	}
}

func TestAuthLoginRefusals(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	authTestSetup(t, srv, pubHex)
	evilPub, evilPriv, _ := ed25519.GenerateKey(rand.Reader)
	evilHex := hex.EncodeToString(evilPub)

	login := func(tenant, key string, signer ed25519.PrivateKey, nonce string, sigOverride string) int {
		sig := sigOverride
		if sig == "" {
			sig = hex.EncodeToString(ed25519.Sign(signer, []byte("kubona-auth/v1 login "+nonce)))
		}
		body, _ := json.Marshal(map[string]any{
			"tenant_id": tenant, "pubkey_hex": key, "nonce": nonce, "signature_hex": sig,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code
	}

	// Unknown tenant 404s.
	nonce, _ := authChallenge(t, srv)
	if code := login("no-such-lab", pubHex, priv, nonce, ""); code != http.StatusNotFound {
		t.Fatalf("unknown tenant must 404, got %d", code)
	}
	// Wrong key for the lab refuses.
	nonce, _ = authChallenge(t, srv)
	if code := login("lab-auth", evilHex, evilPriv, nonce, ""); code != http.StatusUnauthorized {
		t.Fatalf("wrong key must 401, got %d", code)
	}
	// Tampered signature refuses.
	nonce, _ = authChallenge(t, srv)
	sig := ed25519.Sign(priv, []byte("kubona-auth/v1 login "+nonce))
	sig[0] ^= 0xff
	if code := login("lab-auth", pubHex, priv, nonce, hex.EncodeToString(sig)); code != http.StatusUnauthorized {
		t.Fatalf("tampered signature must 401, got %d", code)
	}
	// Replayed nonce refuses (single-use even after failure above — each
	// login call consumes its challenge).
	nonce, _ = authChallenge(t, srv)
	if code := login("lab-auth", pubHex, priv, nonce, ""); code != http.StatusOK {
		t.Fatalf("first use must succeed, got %d", code)
	}
	if code := login("lab-auth", pubHex, priv, nonce, ""); code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce must 401, got %d", code)
	}
	// Garbage nonce refuses.
	if code := login("lab-auth", pubHex, priv, "not-a-real-nonce", ""); code != http.StatusUnauthorized {
		t.Fatalf("unknown nonce must 401, got %d", code)
	}
}

func TestAuthSessionsListAndRevoke(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	authTestSetup(t, srv, hex.EncodeToString(pub))

	var cookies []string
	for i := 0; i < 2; i++ {
		nonce, _ := authChallenge(t, srv)
		w, cookie := authLogin(t, srv, "lab-auth", hex.EncodeToString(pub), priv, nonce)
		if w.Code != http.StatusOK || cookie == "" {
			t.Fatalf("login %d failed: %d", i, w.Code)
		}
		cookies = append(cookies, cookie)
	}
	get := func(cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	// Bearer alone cannot list sessions (no tenant to scope to).
	req := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bearer must not list sessions, got %d", w.Code)
	}
	w = get(cookies[0])
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %v %s", err, w.Body.String())
	}
	id, _ := list.Sessions[0]["id"].(string)
	revoke, _ := json.Marshal(map[string]any{"id": id})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/revoke", bytes.NewReader(revoke))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookies[1]})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: got %d: %s", w.Code, w.Body.String())
	}
	w = get(cookies[1])
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session after revoke, got %v %s", err, w.Body.String())
	}
	// The revoked token no longer authorizes.
	abort, _ := json.Marshal(map[string]any{"session_id": "ghost"})
	req = httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abort))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookies[0]})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("revoked session must not authorize gated routes")
	}
}

func TestAuthCSRFOriginRejected(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	authTestSetup(t, srv, hex.EncodeToString(pub))
	nonce, _ := authChallenge(t, srv)
	_, cookie := authLogin(t, srv, "lab-auth", hex.EncodeToString(pub), priv, nonce)
	if cookie == "" {
		t.Fatal("login failed")
	}
	abort, _ := json.Marshal(map[string]any{"session_id": "ghost"})
	mkreq := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abort))
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	if w := mkreq("https://evil.example.com"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-site cookie use must 403, got %d", w.Code)
	}
	if w := mkreq(""); w.Result().StatusCode == http.StatusForbidden || w.Result().StatusCode == http.StatusUnauthorized {
		t.Fatalf("same-context use without Origin must pass auth, got %d", w.Result().StatusCode)
	}
	// Regression: handleLLMSettings (inline s.checkAuth switch) and
	// handleAuthLogout / handleAuthRevoke must also refuse cross-site Origin
	// with 403 Forbidden without mutating state or revoking the session.
	for _, tc := range []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPut, "/api/llm/settings", []byte(`{"active_provider":"nvidia"}`)},
		{http.MethodPost, "/api/auth/logout", nil},
		{http.MethodPost, "/api/auth/revoke", []byte(`{"id":"any"}`)},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		req.Header.Set("Origin", "https://evil.example.com")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s with evil Origin must 403, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// Verify session is still alive after refused cross-site logout/revoke.
	whoReq := httptest.NewRequest(http.MethodGet, "/api/auth/whoami", nil)
	whoReq.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	whoW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(whoW, whoReq)
	if !bytes.Contains(whoW.Body.Bytes(), []byte(`"authenticated":true`)) {
		t.Fatalf("session must survive cross-site logout/revoke attempt, got %s", whoW.Body.String())
	}
}

func TestAuthBearerStillWorks(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	// Machines and CI keep working with zero session involvement.
	abort, _ := json.Marshal(map[string]any{"session_id": "ghost"})
	req := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abort))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusServiceUnavailable {
		t.Fatalf("bearer must still authorize, got %d", w.Code)
	}
	// Sessions work even when no bearer token is configured at all.
	// Bootstrap first (needs the bearer), then drop the token.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	authTestSetup(t, srv, hex.EncodeToString(pub))
	t.Setenv("KUBONA_API_TOKEN", "")
	nonce, _ := authChallenge(t, srv)
	w, cookie := authLogin(t, srv, "lab-auth", hex.EncodeToString(pub), priv, nonce)
	if w.Code != http.StatusOK || cookie == "" {
		t.Fatalf("session login must work without a configured bearer, got %d", w.Code)
	}
	// ...and the cookie alone authorizes gated routes with no token set.
	abort2, _ := json.Marshal(map[string]any{"session_id": "ghost"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/supervisor/abort", bytes.NewReader(abort2))
	req2.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code == http.StatusUnauthorized || w2.Code == http.StatusServiceUnavailable {
		t.Fatalf("session cookie must authorize with no bearer configured, got %d", w2.Code)
	}
}

// TestAuthLifecycleSealedInLedger proves the A2 claim through the same
// SQLite file the daemon writes: every login, refusal, and revocation
// lands as a tenant-scoped event carrying the acting key fingerprint.
// Read-only handle: the test never writes outside the HTTP surface.
func TestAuthLifecycleSealedInLedger(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	authTestSetup(t, srv, pubHex)

	nonce, _ := authChallenge(t, srv)
	w, cookie := authLogin(t, srv, "lab-auth", pubHex, priv, nonce)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d", w.Code)
	}
	// One refusal for the trail (wrong key).
	evilPub, evilPriv, _ := ed25519.GenerateKey(rand.Reader)
	nonce2, _ := authChallenge(t, srv)
	authLogin(t, srv, "lab-auth", hex.EncodeToString(evilPub), evilPriv, nonce2)
	// One revocation.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)

	db, err := sql.Open("sqlite3", "file:test_api.db?mode=ro")
	if err != nil {
		t.Fatalf("open ledger read-only: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT event_type, payload FROM ledger_events WHERE session_id = 'lab-auth' AND event_type LIKE 'auth_%' ORDER BY id`)
	if err != nil {
		t.Fatalf("query auth events: %v", err)
	}
	defer rows.Close()
	seen := map[string]int{}
	fpOK := true
	for rows.Next() {
		var typ, payload string
		if err := rows.Scan(&typ, &payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[typ]++
		var p map[string]any
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			t.Fatalf("payload must parse: %v", err)
		}
		if f, _ := p["pubkey_fp"].(string); typ == "auth_login" && f == "" {
			fpOK = false
		}
	}
	if seen["auth_login"] < 1 || seen["auth_login_refused"] < 1 || seen["auth_logout"] < 1 {
		t.Fatalf("login, refusal, and logout must all be sealed, got %v", seen)
	}
	if !fpOK {
		t.Fatal("auth_login must carry the acting key fingerprint")
	}
}

func TestSessionAdminAuthorizesWritesWithoutEnvelope(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	authTestSetup(t, srv, pubHex)
	nonce, _ := authChallenge(t, srv)
	_, cookie := authLogin(t, srv, "lab-auth", pubHex, priv, nonce)
	if cookie == "" {
		t.Fatal("login must issue a session cookie")
	}

	// No client Ed25519 envelope: the lab-admin session is the authority.
	body, _ := json.Marshal(map[string]any{"tenant_id": "lab-auth", "ceiling_usd": 250})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/ceiling", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session-admin ceiling must succeed, got %d: %s", w.Code, w.Body.String())
	}
	t2, _ := srv.TenantManager().GetTenant("lab-auth")
	if t2.SpendCeilingUSD != 250 {
		t.Fatalf("ceiling must be 250, got %v", t2.SpendCeilingUSD)
	}

	// The same cookie cannot mutate a different lab.
	other, _ := json.Marshal(map[string]any{"id": "lab-other", "display_name": "Other"})
	oreq := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(other))
	ow := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ow, oreq)
	if ow.Code != http.StatusCreated {
		t.Fatalf("create other lab: %d", ow.Code)
	}
	body, _ = json.Marshal(map[string]any{"tenant_id": "lab-other", "ceiling_usd": 300})
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/ceiling", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-lab session-admin write must 403, got %d: %s", w.Code, w.Body.String())
	}
}
