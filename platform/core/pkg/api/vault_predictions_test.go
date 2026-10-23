package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
)

func postPrediction(t *testing.T, srv interface {
	Handler() http.Handler
}, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func sealH2Prediction(t *testing.T, srv interface {
	Handler() http.Handler
}, sessID string, mean, std float64) string {
	t.Helper()
	rec := postPrediction(t, srv, "/api/vault/predictions/seal", map[string]any{
		"session_id":     sessID,
		"target_system":  "H2",
		"method":         "VQE UCCSD + Qiskit Aer",
		"quantity":       "ground_state_energy_ha",
		"expected_mean":  mean,
		"expected_std":   std,
		"budget_cap_usd": 0.0,
		"author":         "agent_ansatz_scout",
		"p_correlated":   0.88,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("seal failed: %d (%s)", rec.Code, rec.Body.String())
	}
	var resp api.SealPredictionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || resp.PredictionHash == "" {
		t.Fatalf("seal returned no hash: %s", rec.Body.String())
	}
	return resp.PredictionHash
}

func TestPredictionSealHappyPath(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	hash := sealH2Prediction(t, srv, sessID, -1.137, 0.005)
	if len(hash) != 64 {
		t.Fatalf("expected 64-hex sha256 hash, got %q", hash)
	}

	// Deterministic: same payload seals to the same hash.
	hash2 := sealH2Prediction(t, srv, sessID, -1.137, 0.005)
	if hash != hash2 {
		t.Fatalf("seal hash not deterministic: %q vs %q", hash, hash2)
	}
}

func TestPredictionSealStdFloor(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	rec := postPrediction(t, srv, "/api/vault/predictions/seal", map[string]any{
		"session_id":     sessID,
		"target_system":  "H2",
		"method":         "VQE",
		"quantity":       "ground_state_energy_ha",
		"expected_mean":  -1.137,
		"expected_std":   1e-9,
		"budget_cap_usd": 0.0,
		"author":         "agent_ansatz_scout",
		"p_correlated":   0.5,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with floored std, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp api.SealPredictionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.ExpectedStdUsed != 1e-4 || !resp.StdFloorApplied {
		t.Fatalf("expected 1e-4 floor applied, got used=%v applied=%v", resp.ExpectedStdUsed, resp.StdFloorApplied)
	}

	// Non-positive std is a client bug, not a floor case.
	rec = postPrediction(t, srv, "/api/vault/predictions/seal", map[string]any{
		"session_id":     sessID,
		"target_system":  "H2",
		"method":         "VQE",
		"quantity":       "ground_state_energy_ha",
		"expected_mean":  -1.137,
		"expected_std":   0.0,
		"budget_cap_usd": 0.0,
		"author":         "agent_ansatz_scout",
		"p_correlated":   0.5,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for zero std, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPredictionSealValidation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Ghost session.
	rec := postPrediction(t, srv, "/api/vault/predictions/seal", map[string]any{
		"session_id": "no-such-session",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 ghost_session, got %d (%s)", rec.Code, rec.Body.String())
	}

	sessID := createLensTestSession(t, srv, 100.0)
	base := map[string]any{
		"session_id":     sessID,
		"target_system":  "H2",
		"method":         "VQE",
		"quantity":       "ground_state_energy_ha",
		"expected_mean":  -1.137,
		"expected_std":   0.005,
		"budget_cap_usd": 0.0,
		"author":         "agent_ansatz_scout",
		"p_correlated":   0.88,
	}
	for name, mutate := range map[string]func(map[string]any){
		"p_correlated above 1": func(m map[string]any) { m["p_correlated"] = 1.5 },
		"empty method":         func(m map[string]any) { m["method"] = "" },
		"negative budget":      func(m map[string]any) { m["budget_cap_usd"] = -1.0 },
	} {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		mutate(m)
		if rec := postPrediction(t, srv, "/api/vault/predictions/seal", m); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

func TestPredictionEvaluateMath(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	// mu=-1.137, sigma=0.005, measured=-1.1362 -> z=+0.16 (Normal).
	hash := sealH2Prediction(t, srv, sessID, -1.137, 0.005)

	rec := postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id":      sessID,
		"prediction_hash": hash,
		"measured_value":  -1.1362,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate failed: %d (%s)", rec.Code, rec.Body.String())
	}
	var resp api.EvaluatePredictionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if math.Abs(resp.SurpriseZ-0.16) > 1e-9 {
		t.Fatalf("expected z=0.16, got %v", resp.SurpriseZ)
	}
	if math.Abs(resp.SurpriseRatio-0.16) > 1e-9 {
		t.Fatalf("expected |z|=0.16, got %v", resp.SurpriseRatio)
	}
	wantBrier := (math.Erf(3/math.Sqrt2) - 1) * (math.Erf(3/math.Sqrt2) - 1)
	if math.Abs(resp.BrierScore-wantBrier) > 1e-12 {
		t.Fatalf("expected Brier %v, got %v", wantBrier, resp.BrierScore)
	}
	if !resp.Within3Sigma || resp.Level != "normal" {
		t.Fatalf("expected within/level=normal, got within=%v level=%q", resp.Within3Sigma, resp.Level)
	}

	// Anomalous: measured = mu + 4σ -> z=4, outside, Brier = f².
	rec = postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id":      sessID,
		"prediction_hash": hash,
		"measured_value":  -1.137 + 4*0.005,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("anomalous evaluate failed: %d (%s)", rec.Code, rec.Body.String())
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if math.Abs(resp.SurpriseZ-4.0) > 1e-9 || resp.Within3Sigma || resp.Level != "anomalous" {
		t.Fatalf("expected z=4/outside/anomalous, got %+v", resp)
	}
	wantBrier = math.Erf(3/math.Sqrt2) * math.Erf(3/math.Sqrt2)
	if math.Abs(resp.BrierScore-wantBrier) > 1e-12 {
		t.Fatalf("expected anomalous Brier %v, got %v", wantBrier, resp.BrierScore)
	}
}

func TestPredictionEvaluateUnknownHash(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	rec := postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id":      sessID,
		"prediction_hash": "deadbeef",
		"measured_value":  -1.137,
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown hash, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPredictionLatest(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)

	get := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/vault/predictions/latest?session_id=%s", sessID), nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("latest failed: %d (%s)", rec.Code, rec.Body.String())
		}
		var out map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&out)
		return out
	}

	if out := get(); out["sealed"] != nil || out["evaluation"] != nil {
		t.Fatalf("expected nulls on fresh session, got %v", out)
	}

	hash := sealH2Prediction(t, srv, sessID, -1.137, 0.005)
	if out := get(); out["sealed"] == nil || out["evaluation"] != nil {
		t.Fatalf("expected sealed-only after seal, got %v", out)
	}

	postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id":      sessID,
		"prediction_hash": hash,
		"measured_value":  -1.1362,
	})
	out := get()
	sealed, _ := out["sealed"].(map[string]any)
	eval, _ := out["evaluation"].(map[string]any)
	if sealed == nil || sealed["prediction_hash"] != hash {
		t.Fatalf("expected sealed hash %q, got %v", hash, out["sealed"])
	}
	if eval == nil || eval["prediction_hash"] != hash {
		t.Fatalf("expected evaluation for %q, got %v", hash, out["evaluation"])
	}
}

func TestPredictionRoutesRequireBearer(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	for _, path := range []string{
		"/api/vault/predictions/seal",
		"/api/vault/predictions/evaluate",
		"/api/vault/predictions/latest?session_id=x",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 401 (or 405 after auth), got %d", path, rec.Code)
		}
	}
}

func TestPredictionAssertValidation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	hash := sealH2Prediction(t, srv, sessID, -1.137, 0.005)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"no evidence", map[string]any{"session_id": sessID, "prediction_hash": hash, "rho": 0.9, "evidence_refs": []string{}}, http.StatusBadRequest},
		{"rho zero", map[string]any{"session_id": sessID, "prediction_hash": hash, "rho": 0.0, "evidence_refs": []string{"e1"}}, http.StatusBadRequest},
		{"rho above one", map[string]any{"session_id": sessID, "prediction_hash": hash, "rho": 1.5, "evidence_refs": []string{"e1"}}, http.StatusBadRequest},
		{"unknown hash", map[string]any{"session_id": sessID, "prediction_hash": "nope", "rho": 0.9, "evidence_refs": []string{"e1"}}, http.StatusNotFound},
		{"no evaluation yet", map[string]any{"session_id": sessID, "prediction_hash": hash, "rho": 0.9, "evidence_refs": []string{"e1"}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := postPrediction(t, srv, "/api/vault/predictions/assert", tc.body)
		if rec.Code != tc.want {
			t.Fatalf("%s: expected %d, got %d (%s)", tc.name, tc.want, rec.Code, rec.Body.String())
		}
	}

	// Happy path after evaluation.
	eval := postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id": sessID, "prediction_hash": hash, "measured_value": -1.1362,
	})
	if eval.Code != http.StatusOK {
		t.Fatalf("evaluate failed: %d", eval.Code)
	}
	ok := postPrediction(t, srv, "/api/vault/predictions/assert", map[string]any{
		"session_id": sessID, "prediction_hash": hash, "rho": 0.9, "evidence_refs": []string{"job_run_1"},
	})
	if ok.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", ok.Code, ok.Body.String())
	}
	var out struct {
		Rho           float64 `json:"rho"`
		ClaimRecordID string  `json:"claim_record_id"`
	}
	_ = json.NewDecoder(ok.Body).Decode(&out)
	if out.Rho != 0.9 {
		t.Fatalf("expected echoed rho 0.9, got %v", out.Rho)
	}
	if out.ClaimRecordID == "" {
		t.Fatalf("expected minted claim_record_id in assert response, got %+v", out)
	}
}

func TestPredictionCalibration(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	h1 := sealH2Prediction(t, srv, sessID, -1.137, 0.005)
	h2 := sealH2Prediction(t, srv, sessID, -1.14, 0.01)
	for _, tc := range []struct {
		hash string
		val  float64
	}{{h1, -1.1362}, {h2, -1.14}} {
		rec := postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
			"session_id": sessID, "prediction_hash": tc.hash, "measured_value": tc.val,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("evaluate failed: %d", rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/vault/predictions/calibration?session_id="+sessID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Authors map[string]struct {
			Sealed    int     `json:"sealed"`
			Evaluated int     `json:"evaluated"`
			MeanBrier float64 `json:"mean_brier_score"`
		} `json:"authors"`
		Evaluations int `json:"evaluations"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out.Evaluations != 2 {
		t.Fatalf("expected 2 evaluations, got %d", out.Evaluations)
	}
	a, ok := out.Authors["agent_ansatz_scout"]
	if !ok || a.Sealed != 2 || a.Evaluated != 2 {
		t.Fatalf("expected scout 2/2, got %+v", out.Authors)
	}
}

func TestPredictionHistory(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	sessID := createLensTestSession(t, srv, 100.0)
	get := func() []map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/vault/predictions/history?session_id="+sessID+"&limit=10", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("history failed: %d (%s)", rec.Code, rec.Body.String())
		}
		var out struct {
			Count int              `json:"count"`
			Items []map[string]any `json:"predictions"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&out)
		return out.Items
	}

	if items := get(); len(items) != 0 {
		t.Fatalf("expected empty history, got %v", items)
	}
	hash := sealH2Prediction(t, srv, sessID, -1.137, 0.005)
	postPrediction(t, srv, "/api/vault/predictions/evaluate", map[string]any{
		"session_id": sessID, "prediction_hash": hash, "measured_value": -1.1362,
	})
	postPrediction(t, srv, "/api/vault/predictions/assert", map[string]any{
		"session_id": sessID, "prediction_hash": hash, "rho": 0.9, "evidence_refs": []string{"job_1"},
	})
	items := get()
	if len(items) != 3 {
		t.Fatalf("expected seal+evaluate+assert, got %d: %v", len(items), items)
	}
	kinds := map[string]bool{}
	for _, it := range items {
		k, _ := it["event_type"].(string)
		kinds[k] = true
	}
	for _, k := range []string{"prediction_sealed", "prediction_evaluated", "prediction_asserted"} {
		if !kinds[k] {
			t.Fatalf("missing %s in history: %v", k, kinds)
		}
	}
}
