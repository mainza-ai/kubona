package api_test

// S4 — lease binding, caps, revocation, and value hygiene. Threat:
// unattributed leases (G7 class), immortal leases, unbounded scopes, and
// secret values leaking into machine-emitted records.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testLeaseKey = "0123456789abcdef0123456789abcdef"

func leasePost(t *testing.T, h http.Handler, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	rawBody := w.Body.String()
	out := map[string]any{}
	_ = json.Unmarshal([]byte(rawBody), &out)
	out["_raw"] = rawBody
	return w.Code, out
}

func TestLeaseSessionBinding(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	t.Setenv("KUBONA_LEASE_HMAC_KEY", testLeaseKey)
	h := srv.Handler()
	sid := idemSession(t, h, "lease bound")

	code, out := leasePost(t, h, "/api/vault/leases", map[string]any{"scope": "provider:qpu", "ttl_seconds": 300, "session_id": sid})
	if code != http.StatusOK {
		t.Fatalf("bound mint: %d %v", code, out)
	}
	if out["tenant_id"] != "default" || out["session_id"] != sid {
		t.Fatalf("mint must echo binding, got %v", out)
	}
	events, err := l.GetRecentEvents(sid, 5)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType != "lease_issued" {
			continue
		}
		var p map[string]any
		_ = json.Unmarshal(e.Payload, &p)
		if p["session_id"] == sid && p["tenant_id"] == "default" {
			found = true
		}
		if _, ok := p["caller_binding"]; !ok {
			t.Errorf("lease event must carry caller_binding (reserved), got %v", p)
		}
	}
	if !found {
		t.Fatalf("lease_issued event must land on the bound session")
	}

	// Ghost session binds nothing.
	code, out = leasePost(t, h, "/api/vault/leases", map[string]any{"session_id": "sess-ghost"})
	if code != http.StatusNotFound {
		t.Fatalf("ghost-bound mint must 404, got %d %v", code, out)
	}
}

func TestLeaseScopeTTLCaps(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	t.Setenv("KUBONA_LEASE_HMAC_KEY", testLeaseKey)
	h := srv.Handler()

	for scope, want := range map[string]int{
		"provider:qpu":                 http.StatusOK,
		"a" + strings.Repeat("b", 200): http.StatusBadRequest,
		"evil scope!":                  http.StatusBadRequest,
		"../../etc/passwd":             http.StatusBadRequest,
		"":                             http.StatusOK, // default scope
	} {
		code, _ := leasePost(t, h, "/api/vault/leases", map[string]any{"scope": scope})
		if code != want {
			t.Errorf("scope %q: want %d got %d", scope, want, code)
		}
	}
	if code, _ := leasePost(t, h, "/api/vault/leases", map[string]any{"ttl_seconds": 999999999}); code != http.StatusBadRequest {
		t.Errorf("immortal TTL must 400, got %d", code)
	}
	if code, out := leasePost(t, h, "/api/vault/leases", map[string]any{"ttl_seconds": 0}); code != http.StatusOK {
		t.Errorf("zero TTL must default (60), got %d %v", code, out)
	} else if out["ttl_seconds"] != float64(60) {
		t.Errorf("zero TTL must default to 60, got %v", out["ttl_seconds"])
	}
	if code, out := leasePost(t, h, "/api/vault/leases", map[string]any{"ttl_seconds": 86400}); code != http.StatusOK {
		t.Errorf("max TTL must admit, got %d %v", code, out)
	}
}

func TestLeaseRevokeLifecycle(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	t.Setenv("KUBONA_LEASE_HMAC_KEY", testLeaseKey)
	h := srv.Handler()

	code, out := leasePost(t, h, "/api/vault/leases", map[string]any{"scope": "provider:test"})
	if code != http.StatusOK {
		t.Fatalf("mint: %d %v", code, out)
	}
	lid, _ := out["lease_id"].(string)
	if lid == "" {
		t.Fatalf("mint must return lease_id: %v", out)
	}
	code, out = leasePost(t, h, "/api/vault/leases/revoke", map[string]any{"lease_id": lid})
	if code != http.StatusOK {
		t.Fatalf("revoke: %d %v", code, out)
	}
	code, body := leasePost(t, h, "/api/vault/leases/revoke", map[string]any{"lease_id": lid})
	if code != http.StatusNotFound {
		t.Fatalf("double revoke must 404 (already dead), got %d %v", code, body)
	}
	code, body = leasePost(t, h, "/api/vault/leases/revoke", map[string]any{"lease_id": "lease-nope"})
	if code != http.StatusNotFound {
		t.Fatalf("unknown revoke must 404, got %d %v", code, body)
	}
	code, body = leasePost(t, h, "/api/vault/leases/revoke", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("empty revoke must 400, got %d %v", code, body)
	}
}

func TestLeaseValuesNeverInRecords(t *testing.T) {
	// The HMAC key and any lease secret must never appear in
	// machine-emitted records or responses (S4 hygiene). Seeded canary
	// scan over every ledger payload after exercising issuance + spend.
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	canary := "canary-hmac-key-0123456789abcdef-ONLY-IN-ENV"
	t.Setenv("KUBONA_LEASE_HMAC_KEY", canary)
	h := srv.Handler()
	sid := idemSession(t, h, "lease hygiene")

	if code, _ := leasePost(t, h, "/api/vault/leases", map[string]any{"scope": "provider:test", "session_id": sid}); code != http.StatusOK {
		t.Fatalf("mint: %d", code)
	}
	if code, _, _ := idemPost(t, h, "/api/jobs", map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator", "shots": 8}); code != http.StatusCreated {
		t.Fatalf("dispatch: %d", code)
	}
	events, err := l.GetRecentEvents(sid, 100)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, e := range events {
		if strings.Contains(string(e.Payload), canary) {
			t.Fatalf("HMAC key leaked into %s event payload", e.EventType)
		}
	}
	gEvents, err := l.GetRecentEvents("vault-plane", 100)
	if err != nil {
		t.Fatalf("vault events: %v", err)
	}
	for _, e := range gEvents {
		if strings.Contains(string(e.Payload), canary) {
			t.Fatalf("HMAC key leaked into vault-plane %s payload", e.EventType)
		}
	}
}
