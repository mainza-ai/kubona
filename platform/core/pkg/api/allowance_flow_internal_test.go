package api

// C11 end-to-end pin: a signed allowance mints, a metered job draws it
// atomically, tainted launches are excluded, and revocation stops spends.

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

	"github.com/mainza-ai/kubona/core/pkg/money"
)

const allowanceQASM = "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n"

func allowancePost(t *testing.T, h http.Handler, path string, body any) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestRunAllowanceMintDrawTaintRevoke(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/allowance-flow.db")
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	h := srv.Handler()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.TenantManager().SetAdminKey("default", hex.EncodeToString(pub), nil); err != nil {
		t.Fatalf("admin key: %v", err)
	}
	sess, err := srv.sessionMgr.CreateSession("allowance drill", 100, "default")
	if err != nil {
		t.Fatal(err)
	}

	perJob, _ := money.FromUSD(1.0)
	total, _ := money.FromUSD(3.0)
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	nonce := "allowance-nonce-0001"
	scope := allowanceScope{
		TenantID: "default", SessionID: sess.ID, Backend: "ibm_fez", JobTypes: []string{"circuit_run"},
		MaxPerJobMicroUSD: int64(perJob), MaxTotalMicroUSD: int64(total),
		MaxJobs: 2, ShotsCap: 1024, ExpiresAt: expires, Nonce: nonce,
	}
	canonical, err := scope.canonical()
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, canonical)
	mintBody := map[string]any{
		"session_id": sess.ID, "backend": "ibm_fez", "job_types": []string{"circuit_run"},
		"max_per_job_usd": 1.0, "max_total_usd": 3.0, "max_jobs": 2,
		"shots_cap": 1024, "expires_at": expires, "nonce": nonce,
		"signer_pubkey_hex": hex.EncodeToString(pub), "signature_hex": hex.EncodeToString(sig),
	}
	code, body := allowancePost(t, h, "/api/run-allowances", mintBody)
	if code != http.StatusCreated {
		t.Fatalf("mint: %d %s", code, body)
	}
	var minted struct {
		Allowance struct {
			ID           string  `json:"id"`
			Status       string  `json:"status"`
			RemainingUSD float64 `json:"remaining_usd"`
		} `json:"allowance"`
	}
	if err := json.Unmarshal([]byte(body), &minted); err != nil || minted.Allowance.ID == "" {
		t.Fatalf("mint decode: %v %s", err, body)
	}
	if minted.Allowance.Status != "active" || minted.Allowance.RemainingUSD != 3 {
		t.Fatalf("mint view: %+v", minted.Allowance)
	}
	id := minted.Allowance.ID

	// Replayed nonce refuses (UNIQUE) and a tampered signature refuses.
	replay := map[string]any{}
	for k, v := range mintBody {
		replay[k] = v
	}
	if code, body := allowancePost(t, h, "/api/run-allowances", replay); code != http.StatusBadRequest {
		t.Fatalf("nonce replay must 400, got %d %s", code, body)
	}
	tampered := map[string]any{}
	for k, v := range mintBody {
		tampered[k] = v
	}
	tampered["nonce"] = "allowance-nonce-0002"
	tampered["max_total_usd"] = 4.0
	tampered["max_per_job_usd"] = 1.0
	if code, body := allowancePost(t, h, "/api/run-allowances", tampered); code != http.StatusForbidden {
		t.Fatalf("tampered scope must 403, got %d %s", code, body)
	}

	// A metered job draws the allowance atomically.
	jobParams := func(extra map[string]any) map[string]any {
		params := map[string]any{
			"qasm": allowanceQASM, "requested_backend": "ibm_fez", "shots": 1024,
			"max_cost_usd": 1.0, "caller_jurisdictions": []string{"US"},
			"run_allowance_id": id,
		}
		for k, v := range extra {
			params[k] = v
		}
		return params
	}
	if _, err := srv.jobRunner.SubmitJob(sess.ID, "circuit_run", jobParams(nil)); err != nil {
		t.Fatalf("allowance job must admit: %v", err)
	}
	a, err := srv.ledger.GetRunAllowance(id)
	if err != nil || a == nil {
		t.Fatal(err)
	}
	if a.JobsUsed != 1 || a.RemainingMicroUSD != int64(total)-int64(perJob) {
		t.Fatalf("draw not reflected: jobs=%d remaining=%d", a.JobsUsed, a.RemainingMicroUSD)
	}

	// Tainted launches are excluded from the allowance and fall back to
	// per-job approval (which this request does not carry).
	if _, err := srv.jobRunner.SubmitJob(sess.ID, "circuit_run", jobParams(map[string]any{"basis_tainted": true})); err == nil ||
		!strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("tainted launch must not use an allowance, got %v", err)
	}

	// Revocation stops further draws.
	code, body = allowancePost(t, h, "/api/run-allowances/"+id+"/revoke", map[string]any{
		"session_id": sess.ID, "reason": "operator stop",
	})
	if code != http.StatusOK {
		t.Fatalf("revoke: %d %s", code, body)
	}
	if _, err := srv.jobRunner.SubmitJob(sess.ID, "circuit_run", jobParams(nil)); err == nil ||
		!strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked allowance must refuse, got %v", err)
	}
}

func mintAllowanceForTest(t *testing.T, srv *Server, h http.Handler, sessionID, nonce string) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.TenantManager().SetAdminKey("default", hex.EncodeToString(pub), nil); err != nil {
		t.Fatalf("admin key: %v", err)
	}
	perJob, _ := money.FromUSD(1.0)
	total, _ := money.FromUSD(3.0)
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	scope := allowanceScope{
		TenantID: "default", SessionID: sessionID, Backend: "ibm_fez", JobTypes: []string{"circuit_run"},
		MaxPerJobMicroUSD: int64(perJob), MaxTotalMicroUSD: int64(total),
		MaxJobs: 2, ShotsCap: 1024, ExpiresAt: expires, Nonce: nonce,
	}
	canonical, err := scope.canonical()
	if err != nil {
		t.Fatal(err)
	}
	code, body := allowancePost(t, h, "/api/run-allowances", map[string]any{
		"session_id": sessionID, "backend": "ibm_fez", "job_types": []string{"circuit_run"},
		"max_per_job_usd": 1.0, "max_total_usd": 3.0, "max_jobs": 2,
		"shots_cap": 1024, "expires_at": expires, "nonce": nonce,
		"signer_pubkey_hex": hex.EncodeToString(pub),
		"signature_hex":     hex.EncodeToString(ed25519.Sign(priv, canonical)),
	})
	if code != http.StatusCreated {
		t.Fatalf("mint: %d %s", code, body)
	}
	var out struct {
		Allowance struct {
			ID string `json:"id"`
		} `json:"allowance"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.Allowance.ID == "" {
		t.Fatalf("mint decode: %v %s", err, body)
	}
	return out.Allowance.ID
}

func TestRunAllowanceCalibrationMoveSuspends(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/allowance-basis.db")
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	h := srv.Handler()
	sess, err := srv.sessionMgr.CreateSession("basis drill", 100, "default")
	if err != nil {
		t.Fatal(err)
	}
	id := mintAllowanceForTest(t, srv, h, sess.ID, "allowance-nonce-basis-1")

	// A calibration refresh after approval moves the basis: the allowance is
	// suspended and the job refuses rather than spending against a stale plan.
	device, err := srv.deviceForBackend("ibm_fez")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := srv.fidelityGate.Profile(device)
	if !ok {
		t.Fatal("profile missing")
	}
	profile.CalibratedAt = time.Now().UTC().Format(time.RFC3339)
	profile.Version = "drill-refresh"
	if err := srv.fidelityGate.UpdateProfile(profile); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{
		"qasm": allowanceQASM, "requested_backend": "ibm_fez", "shots": 1024,
		"max_cost_usd": 1.0, "caller_jurisdictions": []string{"US"}, "run_allowance_id": id,
	}
	if _, err := srv.jobRunner.SubmitJob(sess.ID, "circuit_run", params); err == nil ||
		!strings.Contains(err.Error(), "calibration changed") {
		t.Fatalf("moved calibration must refuse the allowance, got %v", err)
	}
	stored, _ := srv.ledger.GetRunAllowance(id)
	if stored == nil || stored.Status != "suspended" {
		t.Fatalf("allowance must be suspended by the basis move: %+v", stored)
	}
}

func TestRunAllowanceAirGapSuspends(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, t.TempDir()+"/allowance-airgap.db")
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "test-token")
	h := srv.Handler()
	sess, err := srv.sessionMgr.CreateSession("airgap drill", 100, "default")
	if err != nil {
		t.Fatal(err)
	}
	id := mintAllowanceForTest(t, srv, h, sess.ID, "allowance-nonce-airgap-1")
	srv.suspendActiveAllowances("air_gap_engaged")
	params := map[string]any{
		"qasm": allowanceQASM, "requested_backend": "ibm_fez", "shots": 1024,
		"max_cost_usd": 1.0, "caller_jurisdictions": []string{"US"}, "run_allowance_id": id,
	}
	if _, err := srv.jobRunner.SubmitJob(sess.ID, "circuit_run", params); err == nil ||
		!strings.Contains(err.Error(), "suspended") {
		t.Fatalf("air-gap suspension must refuse the allowance, got %v", err)
	}
}
