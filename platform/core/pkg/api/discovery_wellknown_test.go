package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWellKnownAgentAndLLMsTxt(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	// /.well-known/agent.json
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/.well-known/agent.json status = %d, want 200", rec.Code)
	}
	var card map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("unmarshal agent card: %v", err)
	}
	if card["name"] != "Kubona" {
		t.Fatalf("agent card name = %v, want Kubona", card["name"])
	}
	protocols, _ := card["protocols"].(map[string]any)
	if protocols["mcp"] == nil || protocols["a2a"] == nil || protocols["http"] == nil {
		t.Fatalf("agent card missing protocol sections: %v", protocols)
	}

	// /llms.txt
	req = httptest.NewRequest(http.MethodGet, "/llms.txt", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/llms.txt status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/api/openapi.json") || !strings.Contains(rec.Body.String(), "kubona://skills") {
		t.Fatalf("/llms.txt missing discovery pointers: %s", rec.Body.String())
	}

	// /api/openapi.json
	req = httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/openapi.json status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi: %v", err)
	}
	if spec["openapi"] == nil || spec["paths"] == nil {
		t.Fatalf("openapi payload incomplete")
	}
}

func TestChemistryAnchorFailClosed(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	// Unauthenticated -> 401
	req := httptest.NewRequest(http.MethodPost, "/api/chemistry/anchor", bytes.NewBufferString(`{"molecule":"H2"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth anchor status = %d, want 401", rec.Code)
	}

	// Missing molecule -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/chemistry/anchor", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty molecule status = %d, want 400", rec.Code)
	}

	// Negative distance -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/chemistry/anchor", bytes.NewBufferString(`{"molecule":"H2","bond_distance_angstrom":-1}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative distance status = %d, want 400", rec.Code)
	}

	// Valid molecule with no bridge attached -> 502 bridge_unavailable (fail closed)
	req = httptest.NewRequest(http.MethodPost, "/api/chemistry/anchor", bytes.NewBufferString(`{"molecule":"H2","bond_distance_angstrom":0.74}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("bridgeless anchor status = %d, want 502 (body=%s)", rec.Code, rec.Body.String())
	}
}
