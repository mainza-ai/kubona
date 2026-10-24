package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Spend authority without identity is refused: anonymous session creation
// cannot mint an above-default budget (server.go handleSessions clamps to
// session.DefaultBudgetUSD). Smaller requests pass through untouched, and
// authenticated callers keep whatever they request. Rationale: no
// ReserveSpend>0 path is anonymously reachable (lens, loops, mandates are
// all authenticated), so the clamp closes budget choice, not spending —
// the spending rail was already identity-gated. Companion: the lens suite
// mints spend sessions as operator (createLensTestSession sends the bearer
// like production).

func createSessionAs(t *testing.T, srv interface {
	Handler() http.Handler
}, budget float64, bearer string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"title": "spend drill", "spend_limit_usd": budget})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(raw))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestAnonymousSessionNoBudget(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Anonymous asks for $1M: clamped to the $100 default. Status stays
	// 201 — creation works, the budget choice is what is refused, so the
	// zero-login local flow survives untouched.
	code, body := createSessionAs(t, srv, 1000000.0, "")
	if code != http.StatusCreated {
		t.Fatalf("anonymous creation must still 201, got %d: %v", code, body)
	}
	if body["spend_limit_usd"] != 100.0 {
		t.Fatalf("anonymous session must be clamped to $100, got %v", body["spend_limit_usd"])
	}

	// Anonymous asks for less: passes through untouched.
	code, body = createSessionAs(t, srv, 0.20, "")
	if code != http.StatusCreated {
		t.Fatalf("anonymous creation must still 201, got %d: %v", code, body)
	}
	if body["spend_limit_usd"] != 0.20 {
		t.Fatalf("small anonymous budget must pass through, got %v", body["spend_limit_usd"])
	}

	// Authenticated keeps whatever it requests, including above-default.
	code, body = createSessionAs(t, srv, 1000000.0, "test-token")
	if code != http.StatusCreated {
		t.Fatalf("operator creation must 201, got %d: %v", code, body)
	}
	if body["spend_limit_usd"] != 1000000.0 {
		t.Fatalf("operator session must keep $1M, got %v", body["spend_limit_usd"])
	}
}

// Cookie Secure follows the transport unless explicitly overridden:
// plain-HTTP (tests, loopback dev) stays working; TLS gets Secure;
// KUBONA_COOKIE_SECURE forces either way for TLS-terminating proxies.
// Black-box through the real login (no access to unexported helpers).
func TestSessionCookieSecureFlag(t *testing.T) {
	loginWithTLS := func(t *testing.T, useTLS bool) *http.Cookie {
		t.Helper()
		srv, cleanup := setupTestServer(t)
		defer cleanup()
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		authTestSetup(t, srv, hex.EncodeToString(pub))
		nonce, _ := authChallenge(t, srv)
		sig := ed25519.Sign(priv, []byte("kubona-auth/v1 login "+nonce))
		raw, _ := json.Marshal(map[string]any{
			"tenant_id": "lab-auth", "pubkey_hex": hex.EncodeToString(pub),
			"nonce": nonce, "signature_hex": hex.EncodeToString(sig),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(raw))
		if useTLS {
			req.TLS = &tls.ConnectionState{ServerName: "kubona.example"}
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("login: got %d: %s", rec.Code, rec.Body.String())
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name == "kubona_session" {
				return c
			}
		}
		t.Fatal("login must set the session cookie")
		return nil
	}

	// Plain HTTP default: no Secure, HttpOnly intact.
	c := loginWithTLS(t, false)
	if c.Secure {
		t.Fatal("plain-HTTP cookie must not set Secure by default")
	}
	if !c.HttpOnly {
		t.Fatal("session cookie must stay httpOnly")
	}
	// TLS: Secure set.
	if c := loginWithTLS(t, true); !c.Secure {
		t.Fatal("TLS-served cookie must set Secure")
	}
	// Explicit override on (proxy termination where r.TLS is nil).
	t.Setenv("KUBONA_COOKIE_SECURE", "1")
	if c := loginWithTLS(t, false); !c.Secure {
		t.Fatal("KUBONA_COOKIE_SECURE=1 must force Secure")
	}
	// Explicit override off wins even over TLS.
	t.Setenv("KUBONA_COOKIE_SECURE", "0")
	if c := loginWithTLS(t, true); c.Secure {
		t.Fatal("KUBONA_COOKIE_SECURE=0 must force Secure off")
	}
}
