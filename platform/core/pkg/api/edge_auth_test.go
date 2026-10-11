package api_test

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
	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// signTestAdmin holds a generated tenant-admin key for envelope tests.
type signTestAdmin struct {
	priv ed25519.PrivateKey
	pub  string
}

func newSignTestAdmin(t *testing.T) *signTestAdmin {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &signTestAdmin{priv: priv, pub: hex.EncodeToString(pub)}
}

// mandateSignBody builds a /api/mandates/sign request body. The envelope
// binds the EFFECTIVE parameters (post ttl-default), exactly as the
// handler hashes them: map keys job_hash/max_budget_usd/ttl_seconds with
// JSON number semantics (float64 budget, int64 ttl).
func mandateSignBody(t *testing.T, tenantID, jobHash string, budget float64, ttl int64, admin *signTestAdmin, actionName string) []byte {
	t.Helper()
	if ttl <= 0 {
		ttl = 3600
	}
	params := map[string]any{"job_hash": jobHash, "max_budget_usd": budget, "ttl_seconds": ttl}
	hash, err := tenants.ParamsHashOf(params)
	if err != nil {
		t.Fatal(err)
	}
	var envelope *tenants.AdminAction
	if admin != nil {
		expiry := time.Now().UTC().Add(time.Hour)
		act := &tenants.AdminAction{
			TenantID: tenantID, Action: actionName, ParamsHash: hash,
			Expiry: expiry, PubkeyHex: admin.pub,
		}
		sig := ed25519.Sign(admin.priv, act.CanonicalPayload())
		act.SignatureHex = hex.EncodeToString(sig)
		envelope = act
	}
	body, _ := json.Marshal(map[string]any{
		"tenant_id": tenantID, "job_hash": jobHash,
		"max_budget_usd": budget, "ttl_seconds": ttl,
		"admin_action": envelope,
	})
	return body
}

func authedSignRequest(t *testing.T, srv *api.Server, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// Unique client per call: the limiter is per-IP, and test cases must
	// not spend each other's budget. Loopback honors X-Forwarded-For.
	req.RemoteAddr = "127.0.0.1:0"
	req.Header.Set("X-Forwarded-For", nextTestIP())
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// authedSignRequestFrom pins the client IP: the rate test needs all calls
// on one budget.
func authedSignRequestFrom(t *testing.T, srv *api.Server, body []byte, token, ip string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mandates/sign", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.RemoteAddr = "127.0.0.1:0"
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

var testIPCounter = 100

func nextTestIP() string {
	testIPCounter++
	return "10.99.0." + itoa(testIPCounter)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Bearer skeleton: unset token fails closed, wrong token 401s, and the
// console's client-visible token buys nothing unless the operator aligns it.
func TestMandateSignBearerGate(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")

	body := mandateSignBody(t, "default", "h", 1.0, 60, nil, "mandate_sign")

	t.Setenv("KUBONA_API_TOKEN", "")
	if w := authedSignRequest(t, srv, body, "anything"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unset token must fail closed with 503, got %d", w.Code)
	}

	t.Setenv("KUBONA_API_TOKEN", "op-token")
	if w := authedSignRequest(t, srv, body, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("missing bearer must 401, got %d", w.Code)
	}
	if w := authedSignRequest(t, srv, body, "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong bearer must 401, got %d", w.Code)
	}
}

// Spend authority requires the tenant admin's envelope: no envelope,
// wrong action, tampered params, unknown tenant, missing or foreign admin
// key all refuse — even with a valid bearer token.
func TestMandateSignRequiresAdminEnvelope(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	admin := newSignTestAdmin(t)
	if _, err := srv.TenantManager().SetAdminKey("default", admin.pub, nil); err != nil {
		t.Fatal(err)
	}
	other := newSignTestAdmin(t)

	cases := []struct {
		name string
		body []byte
	}{
		{"no envelope", mandateSignBody(t, "default", "h", 1.0, 60, nil, "mandate_sign")},
		{"wrong action", mandateSignBody(t, "default", "h", 1.0, 60, admin, "spend")},
		{"unknown tenant", mandateSignBody(t, "ghost-lab", "h", 1.0, 60, admin, "mandate_sign")},
		{"foreign admin key", mandateSignBody(t, "default", "h", 1.0, 60, other, "mandate_sign")},
	}
	for _, tc := range cases {
		if w := authedSignRequest(t, srv, tc.body, "op-token"); w.Code != http.StatusForbidden {
			t.Errorf("%s: must 403, got %d (%s)", tc.name, w.Code, w.Body.String())
		}
	}

	// Tampered budget after signing: hash mismatch.
	tampered := mandateSignBody(t, "default", "h", 1.0, 60, admin, "mandate_sign")
	var decoded map[string]any
	if err := json.Unmarshal(tampered, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["max_budget_usd"] = 9999.0
	retampered, _ := json.Marshal(decoded)
	if w := authedSignRequest(t, srv, retampered, "op-token"); w.Code != http.StatusForbidden {
		t.Errorf("tampered budget: must 403, got %d", w.Code)
	}

	// Happy path: bearer + valid envelope mints a verifiable mandate.
	happy := mandateSignBody(t, "default", "job-sha256-ok", 25.0, 3600, admin, "mandate_sign")
	w := authedSignRequest(t, srv, happy, "op-token")
	if w.Code != http.StatusOK {
		t.Fatalf("valid envelope must mint, got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Mandate struct {
			TenantID     string  `json:"tenant_id"`
			JobHash      string  `json:"job_hash"`
			MaxBudgetUSD float64 `json:"max_budget_usd"`
		} `json:"mandate"`
		Verified bool `json:"verified"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Verified || out.Mandate.JobHash != "job-sha256-ok" {
		t.Errorf("minted mandate must verify: %+v", out)
	}
}

// Rate limits bite at machine pace: burst then 429. Same envelope replays
// within its expiry (bounded by its own params) — documented, not silent.
func TestMandateSignRateLimited(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	admin := newSignTestAdmin(t)
	if _, err := srv.TenantManager().SetAdminKey("default", admin.pub, nil); err != nil {
		t.Fatal(err)
	}
	body := mandateSignBody(t, "default", "burst-job", 1.0, 60, admin, "mandate_sign")

	var codes []int
	for i := 0; i < 8; i++ {
		codes = append(codes, authedSignRequestFrom(t, srv, body, "op-token", "10.99.9.9").Code)
	}
	seen429 := false
	for _, c := range codes {
		if c == http.StatusTooManyRequests {
			seen429 = true
		} else if c != http.StatusOK {
			t.Fatalf("unexpected code in burst: %v", codes)
		}
	}
	if !seen429 {
		t.Errorf("burst of 8 must trip the limiter, got %v", codes)
	}
}

// CORS allowlist: local console origins echo, strangers get nothing, the
// "*" escape hatch needs explicit opt-in.
func TestCORSAllowlist(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	get := func(origin, corsAny, corsOrigins string) string {
		t.Setenv("KUBONA_CORS_ANY", corsAny)
		t.Setenv("KUBONA_CORS_ORIGINS", corsOrigins)
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Header().Get("Access-Control-Allow-Origin")
	}

	if got := get("http://localhost:3000", "", ""); got != "http://localhost:3000" {
		t.Errorf("console origin must echo, got %q", got)
	}
	if got := get("https://evil.example", "", ""); got != "" {
		t.Errorf("stranger must get no header, got %q", got)
	}
	if got := get("https://evil.example", "1", ""); got != "*" {
		t.Errorf("explicit opt-in must restore *, got %q", got)
	}
	if got := get("https://ops.example", "", "https://ops.example"); got != "https://ops.example" {
		t.Errorf("configured extra must echo, got %q", got)
	}
}

// B0-6: lease authenticity rests on an operator key, never source. Short
// or missing keys refuse instead of minting forgeable leases.
func TestVaultLeaseRequiresKey(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mint := func() (int, map[string]any) {
		body, _ := json.Marshal(map[string]any{"scope": "provider:test", "ttl_seconds": 60})
		req := httptest.NewRequest(http.MethodPost, "/api/vault/leases", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}

	t.Setenv("KUBONA_LEASE_HMAC_KEY", "")
	if code, _ := mint(); code != http.StatusServiceUnavailable {
		t.Errorf("missing key must 503, got %d", code)
	}
	t.Setenv("KUBONA_LEASE_HMAC_KEY", "too-short")
	if code, _ := mint(); code != http.StatusServiceUnavailable {
		t.Errorf("short key must 503, got %d", code)
	}

	t.Setenv("KUBONA_LEASE_HMAC_KEY", "0123456789abcdef0123456789abcdef")
	code, out := mint()
	if code != http.StatusOK {
		t.Fatalf("keyed mint must succeed, got %d", code)
	}
	sig, _ := out["lease_signature"].(string)
	if len(sig) != 64 {
		t.Errorf("lease must carry a 64-hex HMAC, got %q", out["lease_signature"])
	}
}
