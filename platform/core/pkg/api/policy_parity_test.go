package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/grants"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

// Cross-rail parity: the shared table in platform/tests/policy_parity.json
// binds abstract cases to verdicts; this test drives the Go rail (tool
// registry grants, tenant autonomy, admission) and asserts agreement.
// Python drives the same table against PolicyEngine (test_policy_parity.py).
// Abstract principal/action mapping (mirrors production grants):
//
//	operator            -> full grant incl. dispatch (copilot-equivalent)
//	researcher          -> read + dispatch, no approval powers
//	researcher-approved -> researcher + approval artifact (grant chain)
//	observer            -> read-only (tribunal-equivalent)
//	nobody              -> no grants (unknown role)
//	l0-lab              -> autonomy-revoked tenant (Go-only: Python has no
//	                       tenant states)
func TestPolicyParityTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "policy_parity.json"))
	if err != nil {
		t.Fatalf("shared parity table missing (must be committed): %v", err)
	}
	var table struct {
		Version int `json:"version"`
		Cases   []struct {
			ID        string   `json:"id"`
			Rails     []string `json:"rails"`
			Principal string   `json:"principal"`
			Action    string   `json:"action"`
			Expect    string   `json:"expect"`
			Why       string   `json:"why"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	if table.Version != 1 {
		t.Fatalf("parity table version %d: update this driver, never silently accept", table.Version)
	}
	// R11: rail labels are a closed vocabulary and every go-railed case
	// must actually run — a typo'd rail ("goo") or an accidental skip
	// must fail loudly, never silently shrink coverage.
	for _, tc := range table.Cases {
		if len(tc.Rails) == 0 {
			t.Fatalf("case %q names no rails: every case must run somewhere", tc.ID)
		}
		for _, r := range tc.Rails {
			if r != "go" && r != "py" {
				t.Fatalf("case %q names unknown rail %q: rails are go/py only", tc.ID, r)
			}
		}
	}

	stubOK := func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}
	newParityRegistry := func() *tools.Registry {
		r := tools.NewRegistry()
		for _, name := range []string{"get_anchor", "submit_local_sim"} {
			if err := r.Register(tools.Definition{
				Name: name, Description: "parity stub",
				Parameters: map[string]any{"type": "object"},
				Handler:    stubOK,
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.Allow("operator", "get_anchor", "submit_local_sim"); err != nil {
			t.Fatal(err)
		}
		if err := r.Allow("researcher", "get_anchor", "submit_local_sim"); err != nil {
			t.Fatal(err)
		}
		if err := r.Allow("observer", "get_anchor"); err != nil {
			t.Fatal(err)
		}
		return r
	}
	toolFor := func(action string) string {
		switch action {
		case "tool:submit":
			return "submit_local_sim"
		case "tool:teleport":
			return "teleport_thing"
		default:
			return "get_anchor"
		}
	}
	verdictOf := func(err error) string {
		if err == nil {
			return "allow"
		}
		return "deny"
	}

	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	// R4: tenants/grants mutations sit behind the bearer rail.
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	post := func(path string, body any) (int, map[string]any) {
		t.Helper()
		rawBody, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(rawBody))
		req.Header.Set("Authorization", "Bearer op-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}

	// Grant-flow fixture for metered-with-approval (mirrors the production
	// admission path: tenant admins, minted UCAN chain, registered, grantee
	// session, QPU submit).
	var grantSession string
	var grantChain []string
	setupGrant := func(t *testing.T) {
		t.Helper()
		_, privA, _ := ed25519.GenerateKey(nil)
		pubAHex := hex.EncodeToString(privA.Public().(ed25519.PublicKey))
		_, privB, _ := ed25519.GenerateKey(nil)
		pubBHex := hex.EncodeToString(privB.Public().(ed25519.PublicKey))
		for _, lab := range []string{"parity-grantor", "parity-grantee"} {
			if code, _ := post("/api/tenants", map[string]any{"id": lab}); code != http.StatusCreated {
				t.Fatalf("register %s: %d", lab, code)
			}
		}
		for _, tc := range []struct{ lab, pub string }{
			{"parity-grantor", pubAHex}, {"parity-grantee", pubBHex},
		} {
			code, _ := post("/api/tenants/admin-key", map[string]any{"tenant_id": tc.lab, "new_admin_pubkey_hex": tc.pub})
			if code != http.StatusOK {
				t.Fatalf("bootstrap %s: %d", tc.lab, code)
			}
		}
		didB, err := grants.DIDForAdminPubkey(pubBHex)
		if err != nil {
			t.Fatal(err)
		}
		rootSealed, _, err := grants.MintGrant(
			hex.EncodeToString(privA), didB, "parity-grantor", "parity-grantee", "qpu/submit",
			[]string{grants.ScopeQPUSubmit, grants.ScopeRead}, 60.0, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		grantChain = grants.EncodeChainB64([][]byte{rootSealed})
		if code, _ := post("/api/grants/register", map[string]any{"chain": grantChain}); code != http.StatusOK {
			t.Fatalf("grant register: %d", code)
		}
		sessBody, _ := json.Marshal(map[string]any{"title": "parity metered", "tenant_id": "parity-grantee"})
		sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
		sessW := httptest.NewRecorder()
		h.ServeHTTP(sessW, sessReq)
		var sessResp struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
			t.Fatal(err)
		}
		grantSession = sessResp.ID
	}
	qpuPayload := func() map[string]any {
		return map[string]any{
			"session_id": grantSession,
			"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
			"backend":    "ibm_brisbane", "shots": 100, "max_cost_usd": 25.0,
			"caller_jurisdictions": []string{"US"},
		}
	}

	// R11: ran must equal the go-railed count — every go case runs.
	ranGo := 0
	for _, tc := range table.Cases {
		if !hasRail(tc.Rails, "go") {
			continue
		}
		ranGo++
		var got string
		switch tc.ID {
		case "unknown-tool", "unscoped-principal", "observer-submit",
			"operator-submit", "researcher-read":
			r := newParityRegistry()
			role := tc.Principal
			if role == "nobody" {
				role = "ghost-role"
			}
			_, err := r.Dispatch(context.Background(), role,
				tools.Call{ID: "t1", Name: toolFor(tc.Action), Arguments: `{}`})
			got = verdictOf(err)
		case "metered-without-approval":
			if grantSession == "" {
				setupGrant(t)
			}
			p := qpuPayload()
			code, _ := post("/api/jobs", p)
			got = verdictOf(errForCode(code))
		case "metered-with-approval":
			if grantSession == "" {
				setupGrant(t)
			}
			p := qpuPayload()
			p["grant_chain"] = grantChain
			code, _ := post("/api/jobs", p)
			// Approval admits: only 201 counts (any refusal code denies).
			if code == http.StatusCreated {
				got = "allow"
			} else {
				got = "deny"
			}
		case "l0-tenant":
			got = l0ParityVerdict(t, post, h)
		default:
			t.Fatalf("parity table added case %q: this driver must bind it, never skip", tc.ID)
		}
		if got != tc.Expect {
			t.Errorf("case %q (%s): Go rail says %s, table demands %s — %s",
				tc.ID, tc.Principal+"/"+tc.Action, got, tc.Expect, tc.Why)
		}
	}
	wantGo := 0
	for _, tc := range table.Cases {
		if hasRail(tc.Rails, "go") {
			wantGo++
		}
	}
	if ranGo != wantGo {
		t.Errorf("parity Go coverage shrank: ran %d of %d go-railed cases", ranGo, wantGo)
	}
}

// l0ParityVerdict revokes a tenant to L0 via its admin envelope, then
// dispatches in it: autonomy must refuse before admission runs.
func l0ParityVerdict(t *testing.T, post func(string, any) (int, map[string]any), h http.Handler) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if code, _ := post("/api/tenants", map[string]any{"id": "parity-l0"}); code != http.StatusCreated {
		t.Fatalf("register parity-l0: %d", code)
	}
	code, _ := post("/api/tenants/admin-key", map[string]any{"tenant_id": "parity-l0", "new_admin_pubkey_hex": pubHex})
	if code != http.StatusOK {
		t.Fatalf("bootstrap parity-l0: %d", code)
	}
	revAct := signTenantAction(t, priv, pubHex, "parity-l0", "revoke_autonomy",
		map[string]string{"reason": "parity drill"})
	if code, _ := post("/api/tenants/revoke", map[string]any{"tenant_id": "parity-l0", "reason": "parity drill", "action": revAct}); code != http.StatusOK {
		t.Fatalf("revoke parity-l0: %d", code)
	}
	sessBody, _ := json.Marshal(map[string]any{"title": "parity l0", "tenant_id": "parity-l0"})
	sessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(sessBody))
	sessW := httptest.NewRecorder()
	h.ServeHTTP(sessW, sessReq)
	var sessResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sessW.Body).Decode(&sessResp); err != nil {
		t.Fatal(err)
	}
	code, body := post("/api/jobs", map[string]any{
		"session_id": sessResp.ID,
		"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
	})
	if code == http.StatusCreated {
		t.Errorf("L0 dispatch admitted: %v", body)
		return "allow"
	}
	// The refusal reason is plain text (http.Error), not JSON: re-issue
	// to read it raw and confirm autonomy is named.
	rawBody, _ := json.Marshal(map[string]any{
		"session_id": sessResp.ID,
		"qasm":       "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n",
	})
	rawReq := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(rawBody))
	rawW := httptest.NewRecorder()
	h.ServeHTTP(rawW, rawReq)
	msg := strings.ToLower(rawW.Body.String())
	if !strings.Contains(msg, "l0") && !strings.Contains(msg, "autonomy") && !strings.Contains(msg, "refus") {
		t.Errorf("L0 refusal must name autonomy, got %d %q", code, rawW.Body.String())
	}
	return "deny"
}

func hasRail(rails []string, want string) bool {
	for _, r := range rails {
		if r == want {
			return true
		}
	}
	return false
}

func errForCode(code int) error {
	if code == http.StatusCreated {
		return nil
	}
	return errParityRefused("refused")
}

type errParityRefused string

func (e errParityRefused) Error() string { return string(e) }
