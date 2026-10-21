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

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
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
