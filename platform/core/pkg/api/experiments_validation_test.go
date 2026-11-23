package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExperimentSpecValidationAndAliases(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	// Create session
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"spec-val","target_system":"H2","spend_limit_usd":5}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session = %d: %s", rec.Code, rec.Body.String())
	}
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)

	// 1. thermodynamic_limit with size=2 must fail synchronously with 400 (not 201!)
	body := `{"session_id":"` + sess.ID + `","kind":"thermodynamic_limit","spec":{"kind":"thermodynamic_limit","sizes":[2,4,6]}}`
	req = httptest.NewRequest(http.MethodPost, "/api/experiments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("thermodynamic_limit size=2 status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	// 2. QAOA with alias "p": 1 must normalize to "layers": 1 in stored params_json
	body = `{"session_id":"` + sess.ID + `","kind":"qaoa","spec":{"kind":"qaoa","edges":[[0,1],[1,2]],"p":1,"shots":128}}`
	req = httptest.NewRequest(http.MethodPost, "/api/experiments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("qaoa alias submit = %d: %s", rec.Code, rec.Body.String())
	}
	var qout struct {
		Job struct {
			ParamsJSON map[string]any `json:"params_json"`
		} `json:"job"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &qout)
	if qout.Job.ParamsJSON["layers"] != float64(1) {
		t.Fatalf("expected qaoa alias p=1 normalized to layers=1, got %v", qout.Job.ParamsJSON)
	}
	if _, hasP := qout.Job.ParamsJSON["p"]; hasP {
		t.Fatalf("expected alias key 'p' removed after normalization, got %v", qout.Job.ParamsJSON)
	}

	// 3. VQE with alias "bond_distance_angstrom": 0.74 must normalize to "bond_distance": 0.74
	body = `{"session_id":"` + sess.ID + `","kind":"vqe","spec":{"kind":"vqe","molecule":"H2","bond_distance_angstrom":0.74,"shots":128}}`
	req = httptest.NewRequest(http.MethodPost, "/api/experiments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("vqe alias submit = %d: %s", rec.Code, rec.Body.String())
	}
	var vout struct {
		Job struct {
			ParamsJSON map[string]any `json:"params_json"`
		} `json:"job"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &vout)
	if vout.Job.ParamsJSON["bond_distance"] != 0.74 {
		t.Fatalf("expected bond_distance_angstrom normalized to bond_distance=0.74, got %v", vout.Job.ParamsJSON)
	}
}

func TestManualHypothesisAndAirGappedMemoryRecall(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"hyp-mem","target_system":"H2","spend_limit_usd":5}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)

	// 1. Manual hypothesis creation via POST /api/sessions/{id}/hypotheses
	hbody := `{"statement":"UCCSD achieves chemical accuracy on H2 at 0.74 A","rationale":"Weak static correlation","falsifier":"Error > 1.6 mHa"}`
	req = httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/hypotheses", bytes.NewBufferString(hbody))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manual hypothesis status = %d: %s", rec.Code, rec.Body.String())
	}
	var hout struct {
		Hypotheses []struct {
			Source    string `json:"source"`
			Statement string `json:"statement"`
		} `json:"hypotheses"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &hout)
	if len(hout.Hypotheses) != 1 || hout.Hypotheses[0].Source != "manual" {
		t.Fatalf("expected 1 manual hypothesis, got %+v", hout.Hypotheses)
	}

	// 2. Retain + Recall with short exact keyword (no min_similarity override)
	mbody := `{"session_id":"` + sess.ID + `","content":"H2 STO-3G FCI anchor at 0.74 A is -1.137284 Ha.","bank":"world-facts","confidence":0.95}`
	req = httptest.NewRequest(http.MethodPost, "/api/memory/retain", bytes.NewBufferString(mbody))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("memory retain status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/memory/recall?session_id="+sess.ID+"&q=H2", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("memory recall status = %d: %s", rec.Code, rec.Body.String())
	}
	var mout struct {
		Snippets []map[string]any `json:"snippets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mout)
	if len(mout.Snippets) == 0 {
		t.Fatalf("expected air-gapped lexical fallback to return H2 snippet, got empty list")
	}
}
