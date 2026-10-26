package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func quantumGet(t *testing.T, srv interface {
	Handler() http.Handler
}, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func quantumPut(t *testing.T, srv interface {
	Handler() http.Handler
}, payload map[string]any, bearer string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/quantum/settings", bytes.NewReader(raw))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestQuantumSettingsDefaultsSafe(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Public read; safe default is the local simulator.
	code, body := quantumGet(t, srv, "/api/quantum/settings")
	if code != http.StatusOK {
		t.Fatalf("settings GET must 200, got %d: %v", code, body)
	}
	if body["default_backend"] != "local-simulator" {
		t.Fatalf("default backend must be local-simulator, got %v", body["default_backend"])
	}
}

func TestQuantumSettingsWriteGatedAndValidated(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Anonymous write refuses (settings redirect human runs at hardware).
	if code, _ := quantumPut(t, srv, map[string]any{"default_backend": "ibm-qiskit"}, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT must 401, got %d", code)
	}

	// Unknown backend refuses — a typo must never become a silent default.
	if code, body := quantumPut(t, srv, map[string]any{"default_backend": "ibm-heron-2"}, "test-token"); code != http.StatusBadRequest {
		t.Fatalf("unknown backend must 400, got %d: %v", code, body)
	}

	// Empty harness name refuses.
	if code, _ := quantumPut(t, srv, map[string]any{
		"default_backend":  "local-simulator",
		"harness_backends": map[string]any{"": "ibm-qiskit"},
	}, "test-token"); code != http.StatusBadRequest {
		t.Fatalf("empty harness name must 400, got %d", code)
	}

	// Valid save persists and reads back, including harness overrides.
	code, body := quantumPut(t, srv, map[string]any{
		"default_backend":  "ibm-qiskit",
		"harness_backends": map[string]any{"vqe-ansatz": "rigetti"},
	}, "test-token")
	if code != http.StatusOK {
		t.Fatalf("valid PUT must 200, got %d: %v", code, body)
	}
	code, body = quantumGet(t, srv, "/api/quantum/settings")
	if code != http.StatusOK || body["default_backend"] != "ibm-qiskit" {
		t.Fatalf("saved default must read back, got %d: %v", code, body)
	}
	hb, _ := body["harness_backends"].(map[string]any)
	if hb["vqe-ansatz"] != "rigetti" {
		t.Fatalf("harness override must persist, got %v", body["harness_backends"])
	}
}

func TestQuantumBackendsListsAllProviders(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	code, body := quantumGet(t, srv, "/api/quantum/backends")
	if code != http.StatusOK {
		t.Fatalf("backends GET must 200, got %d: %v", code, body)
	}
	list, _ := body["backends"].([]any)
	if len(list) != 10 {
		t.Fatalf("must list all 10 known providers, got %d", len(list))
	}
	byID := map[string]map[string]any{}
	for _, b := range list {
		m, _ := b.(map[string]any)
		byID[m["provider"].(string)] = m
	}
	// Simulators executable without credentials; wired QPUs without
	// tokens are credentialless-but-listed; unwired/unintegrated are
	// unavailable — visible, never executable.
	if byID["local-simulator"]["executable_now"] != true {
		t.Fatal("local-simulator must be executable")
	}
	if byID["ibm-qiskit"]["credential_state"] != "credentialless" {
		t.Fatalf("ibm-qiskit must be credentialless keyless, got %v", byID["ibm-qiskit"]["credential_state"])
	}
	if byID["ibm-qiskit"]["executable_now"] != false {
		t.Fatal("credentialless QPU must not be executable")
	}
	if byID["neutral-atom-quera-pasqal"]["executable_now"] != false {
		t.Fatal("unwired provider must not be executable")
	}
	if byID["ionq"]["credential_state"] != "unavailable" {
		t.Fatalf("unintegrated vendor must read unavailable, got %v", byID["ionq"]["credential_state"])
	}
	// dwave-annealing solves locally key-free via QUBO matrix.
	if byID["dwave-annealing"]["executable_now"] != true {
		t.Fatal("dwave-annealing local solving must be executable")
	}
	if byID["dwave-annealing"]["dispatch_ready"] != true || byID["dwave-annealing"]["input_format"] != "qubo" {
		t.Fatalf("dwave-annealing must report dispatch_ready=true and input_format=qubo, got %v", byID["dwave-annealing"])
	}
	// Rigetti QPU submit (_submit_via_qcs) is not wired, so even with a token
	// set in the environment, executable_now and dispatch_ready stay false.
	t.Setenv("KUBONA_RIGETTI_QCS_TOKEN", "test-rigetti-token")
	_, bodyWithToken := quantumGet(t, srv, "/api/quantum/backends")
	for _, b := range bodyWithToken["backends"].([]any) {
		m := b.(map[string]any)
		if m["provider"] == "rigetti" {
			if m["credential_state"] != "ready" {
				t.Fatalf("rigetti credential_state must reflect token presence, got %v", m["credential_state"])
			}
			if m["executable_now"] != false || m["dispatch_ready"] != false {
				t.Fatalf("rigetti must stay non-executable while QCS submit is unwired, got %v", m)
			}
		}
	}
}
