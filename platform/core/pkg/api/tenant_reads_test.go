package api_test

// T-09: tenant binding for session-scoped reads (D-09 single-lab open
// mode). Every pin is HTTP-level: two lab tenants, sessions in each,
// cookie/bearer/anonymous callers. Pre-fix the cross-lab reads all
// return 200 with the victim's data.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupTenantDrill(t *testing.T) (*api.Server, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	l, err := ledger.OpenLedger(t.TempDir() + "/tenant.db")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	return api.NewServer(l, sm, jr, sup, wd), func() { l.Close() }
}

func drillTenant(t *testing.T, srv *api.Server, id string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "display_name": id + " lab"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Fatalf("tenant %s create: got %d: %s", id, w.Code, w.Body.String())
	}
}

// drillLogin bootstraps the tenant admin key (operator bearer,
// first-write-wins) and completes the challenge/login flow, returning
// the session cookie bound to that lab.
func drillLogin(t *testing.T, srv *api.Server, tenant string) string {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	boot, _ := json.Marshal(map[string]any{"tenant_id": tenant, "new_admin_pubkey_hex": pubHex})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/admin-key", bytes.NewReader(boot))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin bootstrap %s: got %d: %s", tenant, w.Code, w.Body.String())
	}
	nonce, _ := authChallenge(t, srv)
	lw, cookie := authLogin(t, srv, tenant, pubHex, priv, nonce)
	if lw.Code != http.StatusOK || cookie == "" {
		t.Fatalf("login %s: got %d, cookie set=%v: %s", tenant, lw.Code, cookie != "", lw.Body.String())
	}
	return cookie
}

func drillSession(t *testing.T, srv *api.Server, title, tenant, token string) string {
	t.Helper()
	payload := map[string]any{"title": title}
	if tenant != "" {
		payload["tenant_id"] = tenant
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(raw))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("session create: got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		ID       string `json:"id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("session decode: %v: %s", err, w.Body.String())
	}
	return out.ID
}

func drillGet(t *testing.T, srv *api.Server, path, cookie, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestTenantReadsScopedByCookie(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	cookieB := drillLogin(t, srv, "lab-b")

	sessA := drillSession(t, srv, "a-work", "lab-a", "test-token")
	sessB := drillSession(t, srv, "b-work", "lab-b", "test-token")

	// Own-lab reads pass on every bound endpoint.
	for _, path := range []string{
		"/api/epistemics?session_id=" + sessA,
		"/api/sessions/" + sessA,
		"/api/sessions/" + sessA + "/claims",
		"/api/sessions/" + sessA + "/messages",
		"/api/events?session_id=" + sessA,
	} {
		if code, body := drillGet(t, srv, path, cookieA, ""); code != http.StatusOK {
			t.Errorf("own-lab read %s must 200, got %d: %s", path, code, body)
		}
	}
	// Cross-lab reads refuse with 403 and name the lab boundary.
	for _, path := range []string{
		"/api/epistemics?session_id=" + sessB,
		"/api/sessions/" + sessB,
		"/api/sessions/" + sessB + "/claims",
		"/api/sessions/" + sessB + "/messages",
		"/api/events?session_id=" + sessB,
	} {
		if code, body := drillGet(t, srv, path, cookieA, ""); code != http.StatusForbidden {
			t.Errorf("cross-lab read %s must 403, got %d: %s", path, code, body)
		} else if !strings.Contains(body, "another lab") {
			t.Errorf("cross-lab refusal must name the lab boundary: %s", body)
		}
	}
	// Lab B reads its own session fine (symmetry, not a blocklist).
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id="+sessB, cookieB, ""); code != http.StatusOK {
		t.Errorf("lab-b own read must 200, got %d", code)
	}
	// Bearer operator still reads across labs.
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id="+sessB, "", "test-token"); code != http.StatusOK {
		t.Errorf("bearer cross-lab read must 200, got %d", code)
	}
	// Ghosts keep legacy behavior (404, not 403).
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id=ghost", cookieA, ""); code != http.StatusNotFound {
		t.Errorf("ghost read must stay 404, got %d", code)
	}
}

// GET /api/tenants/:id binds the caller like the list (D-09): lab callers
// read only their own row, bearer reads all, anonymous reads only while the
// store holds a single lab. Pre-fix the path had no caller check at all.
func TestTenantByIDScopedByCaller(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()

	// Single-lab store (default only): anonymous reads work (local flow).
	if code, _ := drillGet(t, srv, "/api/tenants/default", "", ""); code != http.StatusOK {
		t.Errorf("single-lab anonymous tenant read must 200, got %d", code)
	}

	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")

	// Own-lab row reads fine.
	if code, body := drillGet(t, srv, "/api/tenants/lab-a", cookieA, ""); code != http.StatusOK {
		t.Errorf("own-lab tenant read must 200, got %d: %s", code, body)
	} else if !strings.Contains(body, "lab-a") {
		t.Errorf("own-lab read must return the lab row: %s", body)
	}
	// Cross-lab row refuses and names the boundary.
	if code, body := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab tenant read must 403, got %d: %s", code, body)
	} else if !strings.Contains(body, "another lab") {
		t.Errorf("cross-lab refusal must name the lab boundary: %s", body)
	}
	// Bearer operator reads across labs.
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", "", "test-token"); code != http.StatusOK {
		t.Errorf("bearer cross-lab tenant read must 200, got %d", code)
	}
	// Multi-lab store: anonymous reads close.
	if code, _ := drillGet(t, srv, "/api/tenants/lab-a", "", ""); code != http.StatusUnauthorized {
		t.Errorf("multi-lab anonymous tenant read must 401, got %d", code)
	}
	// Ghost tenants: no existence oracle — a lab caller guessing another
	// id gets the same 403 as an existing foreign row, never a 404 that
	// confirms absence (tenant ids are human-chosen, unlike session UUIDs).
	if code, _ := drillGet(t, srv, "/api/tenants/ghost-lab", cookieA, ""); code != http.StatusForbidden {
		t.Errorf("lab caller guessing a tenant id must 403 (no oracle), got %d", code)
	}
	// Bearer still distinguishes ghosts (operator sees the registry as-is).
	if code, _ := drillGet(t, srv, "/api/tenants/ghost-lab", "", "test-token"); code != http.StatusNotFound {
		t.Errorf("bearer ghost tenant read must stay 404, got %d", code)
	}
}

func TestAnonymousReadsSingleLabOpen(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	sess := drillSession(t, srv, "solo", "", "")

	// Single-lab store: anonymous reads work (zero-login local flow).
	if code, _ := drillGet(t, srv, "/api/sessions", "", ""); code != http.StatusOK {
		t.Fatalf("single-lab anonymous list must 200, got %d", code)
	}
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id="+sess, "", ""); code != http.StatusOK {
		t.Fatalf("single-lab anonymous read must 200, got %d", code)
	}
	// A second lab appears: anonymous reads close, bearer keeps working.
	drillTenant(t, srv, "lab-b")
	drillSession(t, srv, "b-work", "lab-b", "test-token")
	if code, body := drillGet(t, srv, "/api/sessions", "", ""); code != http.StatusUnauthorized {
		t.Errorf("multi-lab anonymous list must 401, got %d: %s", code, body)
	}
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id="+sess, "", ""); code != http.StatusUnauthorized {
		t.Errorf("multi-lab anonymous read must 401, got %d", code)
	}
	if code, _ := drillGet(t, srv, "/api/epistemics?session_id="+sess, "", "test-token"); code != http.StatusOK {
		t.Errorf("bearer read must survive multi-tenancy, got %d", code)
	}
}

func TestSessionListScopedByCaller(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	drillSession(t, srv, "a-work", "lab-a", "test-token")
	drillSession(t, srv, "b-work", "lab-b", "test-token")

	code, body := drillGet(t, srv, "/api/sessions", cookieA, "")
	if code != http.StatusOK {
		t.Fatalf("lab-a list must 200, got %d: %s", code, body)
	}
	if strings.Contains(body, "b-work") {
		t.Errorf("lab-a list must not contain lab-b sessions: %s", body)
	}
	if !strings.Contains(body, "a-work") {
		t.Errorf("lab-a list must contain its own session: %s", body)
	}
	code, body = drillGet(t, srv, "/api/sessions", "", "test-token")
	if code != http.StatusOK || !strings.Contains(body, "b-work") || !strings.Contains(body, "a-work") {
		t.Errorf("bearer list must 200 with both labs, got %d: %s", code, body)
	}
}

func TestSessionCreationSpoofGuard(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")

	post := func(body map[string]any, cookie, token string) (int, string) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(raw))
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	// Anonymous minting into another lab refuses.
	if code, body := post(map[string]any{"title": "spoof", "tenant_id": "lab-b"}, "", ""); code != http.StatusForbidden {
		t.Errorf("anonymous cross-lab create must 403, got %d: %s", code, body)
	}
	// Lab cookie minting into another lab refuses.
	if code, body := post(map[string]any{"title": "spoof", "tenant_id": "lab-b"}, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("lab-a cross-lab create must 403, got %d: %s", code, body)
	}
	// Lab cookie with empty tenant pins to its own lab.
	if code, body := post(map[string]any{"title": "mine"}, cookieA, ""); code != http.StatusCreated {
		t.Errorf("lab-a own create must 201, got %d: %s", code, body)
	} else if !strings.Contains(body, `"tenant_id":"lab-a"`) {
		t.Errorf("empty-tenant create must pin to the caller lab: %s", body)
	}
	// Anonymous default-lab creation still works (local flow).
	if code, _ := post(map[string]any{"title": "local"}, "", ""); code != http.StatusCreated {
		t.Errorf("anonymous default create must 201, got %d", code)
	}
	// Bearer operator mints anywhere.
	if code, _ := post(map[string]any{"title": "ops", "tenant_id": "lab-b"}, "", "test-token"); code != http.StatusCreated {
		t.Errorf("bearer cross-lab create must 201, got %d", code)
	}
}

func TestSessionDeleteBoundToTenant(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	sessB := drillSession(t, srv, "b-doomed", "lab-b", "test-token")
	sessA := drillSession(t, srv, "a-doomed", "lab-a", "test-token")

	del := func(id, cookie, token string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+id, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := del(sessB, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("lab-a deleting lab-b session must 403, got %d", code)
	}
	if code := del(sessA, cookieA, ""); code != http.StatusOK {
		t.Errorf("lab-a deleting own session must 200, got %d", code)
	}
	if code := del(sessB, "", "test-token"); code != http.StatusOK {
		t.Errorf("bearer deleting lab-b session must 200, got %d", code)
	}
}

// T-09c: session-keyed mutations bind caller to session tenant. Spend
// (jobs/tribunal/launch) and content (feedback/rewrite/regenerate)
// mutations refuse cross-lab cookies with 403; own-lab and bearer flows
// pass unchanged. Ghost behavior is untouched (404s preserved).
func TestMutationsBoundToTenant(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	drillTenant(t, srv, "lab-a")
	drillTenant(t, srv, "lab-b")
	cookieA := drillLogin(t, srv, "lab-a")
	sessA := drillSession(t, srv, "a-work", "lab-a", "test-token")
	sessB := drillSession(t, srv, "b-work", "lab-b", "test-token")

	post := func(path string, body any, cookie, token string) (int, string) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "kubona_session", Value: cookie})
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	qasm := "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n"

	// Spend class: jobs dispatch.
	jobBody := map[string]any{"session_id": sessB, "qasm": qasm, "backend": "local_simulator", "shots": 100}
	if code, body := post("/api/jobs", jobBody, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab jobs dispatch must 403, got %d: %s", code, body)
	}
	jobBody["session_id"] = sessA
	if code, body := post("/api/jobs", jobBody, cookieA, ""); code != http.StatusCreated {
		t.Errorf("own-lab jobs dispatch must 201, got %d: %s", code, body)
	}

	// Spend class: tribunal + launch refuse before any work.
	if code, _ := post("/api/chat/tribunal", map[string]any{"session_id": sessB, "prompt": "x"}, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab tribunal must 403, got %d", code)
	}
	if code, _ := post("/api/chat/launch", map[string]any{"session_id": sessB, "qasm": qasm, "backend": "local_simulator"}, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab launch must 403, got %d", code)
	}

	// Content class: feedback / rewrite / regenerate refuse before touching rows.
	fb := map[string]any{"session_id": sessB, "message_id": "ghost-msg", "vote": "up"}
	if code, _ := post("/api/chat/feedback", fb, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab feedback must 403, got %d", code)
	}
	rw := map[string]any{"session_id": sessB, "message_id": "ghost-msg", "content": "x"}
	if code, _ := post("/api/chat/rewrite", rw, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab rewrite must 403, got %d", code)
	}
	if code, _ := post("/api/chat/regenerate", map[string]any{"session_id": sessB}, cookieA, ""); code != http.StatusForbidden {
		t.Errorf("cross-lab regenerate must 403, got %d", code)
	}

	// Bearer operator still crosses labs (delegation-grade credential).
	if code, _ := post("/api/jobs", map[string]any{"session_id": sessB, "qasm": qasm, "backend": "local_simulator", "shots": 100}, "", "test-token"); code != http.StatusCreated {
		t.Errorf("bearer cross-lab jobs dispatch must 201, got %d", code)
	}

	// F-NEW-04: Observatory, Vault Predictions, Provenance, Quantum Jobs,
	// Jobs List/Cancel, Events Stream, and Vault Leases must all refuse
	// cross-lab cookie access to sessB with 403 Forbidden.
	crossLabPosts := []struct {
		name string
		path string
		body map[string]any
	}{
		{"observatory_lens", "/api/observatory/lens", map[string]any{"session_id": sessB, "prompt": "lab scene"}},
		{"observatory_assets", "/api/observatory/assets", map[string]any{"session_id": sessB, "data_base64": "data:image/png;base64,AAAA"}},
		{"observatory_resolve", "/api/observatory/lens/resolve", map[string]any{"session_id": sessB, "operation_id": "op-1"}},
		{"prediction_seal", "/api/vault/predictions/seal", map[string]any{"session_id": sessB, "target_system": "H2", "method": "VQE", "quantity": "energy_ha", "expected_mean": -1.137, "expected_std": 0.01, "author": "alice"}},
		{"prediction_evaluate", "/api/vault/predictions/evaluate", map[string]any{"session_id": sessB, "prediction_hash": "abc", "measured_value": -1.13}},
		{"prediction_assert", "/api/vault/predictions/assert", map[string]any{"session_id": sessB, "prediction_hash": "abc", "rho": 0.8, "evidence_refs": []string{"ev-1"}}},
		{"ledger_append", "/api/ledger/append", map[string]any{"session_id": sessB, "event_type": "annotation", "payload": map[string]any{"note": "spoof"}}},
		{"supervisor_abort", "/api/supervisor/abort", map[string]any{"session_id": sessB, "reason": "spoof abort"}},
		{"quantum_jobs", "/api/quantum/jobs", map[string]any{"session_id": sessB, "qasm": qasm, "provider": "local-simulator", "shots": 100, "idempotency_key": "cross-q-1"}},
		{"vault_leases", "/api/vault/leases", map[string]any{"session_id": sessB, "scope": "provider:session_token", "ttl_seconds": 60}},
	}
	for _, tc := range crossLabPosts {
		if code, body := post(tc.path, tc.body, cookieA, ""); code != http.StatusForbidden {
			t.Errorf("%s: cross-lab POST must 403, got %d: %s", tc.name, code, body)
		}
	}

	crossLabGets := []struct {
		name string
		path string
	}{
		{"observatory_history", "/api/observatory/lens/history?session_id=" + sessB},
		{"prediction_latest", "/api/vault/predictions/latest?session_id=" + sessB},
		{"prediction_calibration", "/api/vault/predictions/calibration?session_id=" + sessB},
		{"prediction_history", "/api/vault/predictions/history?session_id=" + sessB},
		{"jobs_list", "/api/jobs?session_id=" + sessB},
		{"events_stream", "/api/events/stream?session_id=" + sessB},
	}
	for _, tc := range crossLabGets {
		if code, body := drillGet(t, srv, tc.path, cookieA, ""); code != http.StatusForbidden {
			t.Errorf("%s: cross-lab GET must 403, got %d: %s", tc.name, code, body)
		}
	}
}

// Phase-2 grants on the tenant read paths (same allow/deny matrix as the
// tenants-package suite, exercised over HTTP): cross-lab reads refuse by
// default, allow with a live read grant, and refuse again after revoke,
// after expiry, or when the grant covers a different scope.

// grantDrillLab creates a tenant and bootstraps its admin via the manager
// (so the test holds the private key for grant issuance), then completes
// the cookie login flow for that lab.
func grantDrillLab(t *testing.T, srv *api.Server, tenant string) (ed25519.PrivateKey, string, string) {
	t.Helper()
	drillTenant(t, srv, tenant)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	if _, err := srv.TenantManager().SetAdminKey(tenant, pubHex, nil); err != nil {
		t.Fatalf("bootstrap %s: %v", tenant, err)
	}
	nonce, _ := authChallenge(t, srv)
	lw, cookie := authLogin(t, srv, tenant, pubHex, priv, nonce)
	if lw.Code != http.StatusOK || cookie == "" {
		t.Fatalf("login %s: got %d, cookie set=%v: %s", tenant, lw.Code, cookie != "", lw.Body.String())
	}
	return priv, pubHex, cookie
}

func grantDrillIssue(t *testing.T, srv *api.Server, priv ed25519.PrivateKey, pubHex, source, target string, scopes []string, expiry time.Time) *tenants.Grant {
	t.Helper()
	canonical := tenants.GrantCanonicalPayload(source, target, scopes, expiry)
	sig := ed25519.Sign(priv, canonical)
	g, err := srv.TenantManager().IssueGrant(source, target, scopes, expiry, pubHex, hex.EncodeToString(sig))
	if err != nil {
		t.Fatalf("issue %s -> %s: %v", source, target, err)
	}
	return g
}

func TestTenantReadsHonorGrants(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	_, _, cookieA := grantDrillLab(t, srv, "lab-a")
	privB, pubB, _ := grantDrillLab(t, srv, "lab-b")

	// No grant: cross-lab row read refuses, list shows only the own row.
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusForbidden {
		t.Fatalf("ungranted cross-lab tenant read must 403, got %d", code)
	}
	if code, body := drillGet(t, srv, "/api/tenants", cookieA, ""); code != http.StatusOK || strings.Contains(body, "lab-b") {
		t.Fatalf("ungranted list must 200 without lab-b, got %d: %s", code, body)
	}

	// Out-of-scope grant (write, not read): read still refuses.
	grantDrillIssue(t, srv, privB, pubB, "lab-b", "lab-a", []string{"write"}, time.Now().UTC().Add(time.Hour))
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusForbidden {
		t.Fatalf("out-of-scope grant must still 403 the read, got %d", code)
	}

	// Live read grant: row read allows, list includes the granting lab.
	gRead := grantDrillIssue(t, srv, privB, pubB, "lab-b", "lab-a", []string{"read"}, time.Now().UTC().Add(time.Hour))
	if code, body := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusOK {
		t.Fatalf("granted cross-lab tenant read must 200, got %d: %s", code, body)
	} else if !strings.Contains(body, "lab-b") {
		t.Fatalf("granted read must return the lab row: %s", body)
	}
	if code, body := drillGet(t, srv, "/api/tenants", cookieA, ""); code != http.StatusOK {
		t.Fatalf("granted list must 200, got %d", code)
	} else if !strings.Contains(body, "lab-b") || !strings.Contains(body, "lab-a") {
		t.Fatalf("granted list must show own + granting labs: %s", body)
	}

	// Revoke: the very next read refuses (immediate, no TTL cache).
	grantDrillRevoke(t, srv, privB, pubB, "lab-b", gRead.GrantID, "drill done")
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusForbidden {
		t.Fatalf("revoked grant must 403 the read again, got %d", code)
	}
	if code, body := drillGet(t, srv, "/api/tenants", cookieA, ""); code != http.StatusOK || strings.Contains(body, "lab-b") {
		t.Fatalf("revoked list must drop lab-b, got %d: %s", code, body)
	}

	// Ghost oracle preserved under grants: guessing an id still 403s.
	if code, _ := drillGet(t, srv, "/api/tenants/ghost-lab", cookieA, ""); code != http.StatusForbidden {
		t.Fatalf("ghost guess must stay 403 under grants, got %d", code)
	}
	// Bearer operator still reads across labs (delegation-grade credential).
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", "", "test-token"); code != http.StatusOK {
		t.Fatalf("bearer cross-lab read must stay 200, got %d", code)
	}
}

func grantDrillRevoke(t *testing.T, srv *api.Server, priv ed25519.PrivateKey, pubHex, source, grantID, reason string) {
	t.Helper()
	sig := ed25519.Sign(priv, tenants.RevokeCanonicalPayload(grantID, reason))
	if err := srv.TenantManager().RevokeGrant(source, grantID, reason, pubHex, hex.EncodeToString(sig)); err != nil {
		t.Fatalf("revoke %s: %v", grantID, err)
	}
}

// Phase-2 grant HTTP lifecycle: the tenant-grant issue/revoke endpoints are
// the only way to mint the grants the read paths enforce. Every pin is
// HTTP-level: anonymous refuses (bearer transport gate), self-signed foreign
// grantors and wildcard scopes refuse (signature/class gates), and the full
// issue -> allow -> revoke -> refuse cycle runs over the wire.
func TestTenantGrantHTTPLifecycle(t *testing.T) {
	srv, cleanup := setupTenantDrill(t)
	defer cleanup()
	_, _, cookieA := grantDrillLab(t, srv, "lab-a")
	privB, pubB, _ := grantDrillLab(t, srv, "lab-b")

	post := func(path string, body any, token string) (int, string) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	expiry := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	sign := func(source, target string, scopes []string, exp string, priv ed25519.PrivateKey) string {
		t.Helper()
		e, err := time.Parse(time.RFC3339, exp)
		if err != nil {
			t.Fatalf("bad expiry: %v", err)
		}
		return hex.EncodeToString(ed25519.Sign(priv, tenants.GrantCanonicalPayload(source, target, scopes, e)))
	}

	// Anonymous: transport gate refuses before crypto is even read.
	if code, _ := post("/api/tenant-grants/issue", map[string]any{}, ""); code != http.StatusUnauthorized {
		t.Fatalf("anon grant issue must 401, got %d", code)
	}

	// Self-signed foreign grantor: valid signature, wrong key for lab-b.
	evilPub, evilPriv, _ := ed25519.GenerateKey(rand.Reader)
	if code, _ := post("/api/tenant-grants/issue", map[string]any{
		"source_tenant": "lab-b", "target_tenant": "lab-a",
		"scopes": []string{"read"}, "expiry": expiry,
		"grantor_pubkey_hex": hex.EncodeToString(evilPub),
		"signature_hex":      sign("lab-b", "lab-a", []string{"read"}, expiry, evilPriv),
	}, "test-token"); code != http.StatusForbidden {
		t.Fatalf("self-signed grantor must 403, got %d", code)
	}

	// Wildcard scope: refused at issue, never mints.
	if code, _ := post("/api/tenant-grants/issue", map[string]any{
		"source_tenant": "lab-b", "target_tenant": "lab-a",
		"scopes": []string{"*"}, "expiry": expiry,
		"grantor_pubkey_hex": pubB,
		"signature_hex":      sign("lab-b", "lab-a", []string{"*"}, expiry, privB),
	}, "test-token"); code != http.StatusForbidden {
		t.Fatalf("wildcard scope must 403, got %d", code)
	}

	// Expired: refuses (never mints a dead-on-arrival grant).
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if code, _ := post("/api/tenant-grants/issue", map[string]any{
		"source_tenant": "lab-b", "target_tenant": "lab-a",
		"scopes": []string{"read"}, "expiry": past,
		"grantor_pubkey_hex": pubB,
		"signature_hex":      sign("lab-b", "lab-a", []string{"read"}, past, privB),
	}, "test-token"); code != http.StatusForbidden {
		t.Fatalf("expired grant must 403, got %d", code)
	}

	// Happy path over HTTP: issue -> cross-lab read allows.
	code, body := post("/api/tenant-grants/issue", map[string]any{
		"source_tenant": "lab-b", "target_tenant": "lab-a",
		"scopes": []string{"read"}, "expiry": expiry,
		"grantor_pubkey_hex": pubB,
		"signature_hex":      sign("lab-b", "lab-a", []string{"read"}, expiry, privB),
	}, "test-token")
	if code != http.StatusOK {
		t.Fatalf("grant issue must 200, got %d: %s", code, body)
	}
	var issued struct {
		GrantID string `json:"grant_id"`
	}
	if err := json.Unmarshal([]byte(body), &issued); err != nil || issued.GrantID == "" {
		t.Fatalf("issue must return a grant_id: %s", body)
	}
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusOK {
		t.Fatalf("granted cross-lab read must 200, got %d", code)
	}

	// Revoke over HTTP: next read refuses immediately.
	revSig := hex.EncodeToString(ed25519.Sign(privB, tenants.RevokeCanonicalPayload(issued.GrantID, "http drill done")))
	if code, body := post("/api/tenant-grants/revoke", map[string]any{
		"source_tenant": "lab-b", "grant_id": issued.GrantID,
		"reason":             "http drill done",
		"grantor_pubkey_hex": pubB, "signature_hex": revSig,
	}, "test-token"); code != http.StatusOK {
		t.Fatalf("grant revoke must 200, got %d: %s", code, body)
	}
	if code, _ := drillGet(t, srv, "/api/tenants/lab-b", cookieA, ""); code != http.StatusForbidden {
		t.Fatalf("revoked grant must 403 the read again, got %d", code)
	}
}
