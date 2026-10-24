package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// U3-fleet: /api/fleet tells credentialless apart from down, and measures
// bridge liveness instead of inferring it from a never-nil client.
func fleetBackends(t *testing.T, handler http.Handler) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/fleet", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/fleet, got %d", w.Code)
	}
	var out struct {
		Backends []map[string]any `json:"backends"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("fleet body must parse: %v", err)
	}
	return out.Backends
}

func backendByName(t *testing.T, backends []map[string]any, name string) map[string]any {
	t.Helper()
	for _, b := range backends {
		if b["name"] == name {
			return b
		}
	}
	t.Fatalf("fleet missing backend %q", name)
	return nil
}

func TestFleetCredentialStates(t *testing.T) {
	t.Setenv("KUBONA_IBM_QISKIT_TOKEN", "")
	t.Setenv("KUBONA_QUANTINUUM_API_KEY", "")
	t.Setenv("KUBONA_RIGETTI_QCS_TOKEN", "")
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	backends := fleetBackends(t, srv.Handler())
	// No bridge addr configured in tests → UNKNOWN, never claimed ONLINE.
	if got := backendByName(t, backends, "local_simulator")["status"]; got != "UNKNOWN" {
		t.Fatalf("unconfigured bridge must read UNKNOWN, got %v", got)
	}
	// Simulators are credential-free by construction.
	if got := backendByName(t, backends, "local_simulator")["credential_state"]; got != "ready" {
		t.Fatalf("local_simulator must be credential-ready, got %v", got)
	}
	// QPU backends without tokens are credentialless — distinct from down.
	for _, name := range []string{"ibm_heron", "quantinuum_h1", "rigetti_aspen"} {
		if got := backendByName(t, backends, name)["credential_state"]; got != "credentialless" {
			t.Fatalf("%s must be credentialless without its token, got %v", name, got)
		}
	}
}

func TestFleetTelemetryProvenance(t *testing.T) {
	// Queue depths and calibration medians on hardware cards are vendor
	// catalog, not live measurement — the response must say so per card
	// so no client can present them as telemetry.
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	backends := fleetBackends(t, srv.Handler())
	if got := backendByName(t, backends, "local_simulator")["telemetry"]; got != "measured" {
		t.Fatalf("local_simulator telemetry must be measured, got %v", got)
	}
	for _, name := range []string{"ibm_heron", "quantinuum_h1", "rigetti_aspen"} {
		b := backendByName(t, backends, name)
		if b["telemetry"] != "declared" {
			t.Fatalf("%s telemetry must be declared, got %v", name, b["telemetry"])
		}
		if _, ok := b["catalog_verified"]; !ok {
			t.Fatalf("%s must carry catalog_verified (null until verified)", name)
		}
	}
}

func TestFleetBridgeLivenessMeasured(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// A live listener reads ONLINE.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("test listener failed: %v", err)
	}
	defer ln.Close()
	srv.SetBridgeAddr(ln.Addr().String())
	if got := backendByName(t, fleetBackends(t, srv.Handler()), "local_simulator")["status"]; got != "ONLINE" {
		t.Fatalf("live bridge must read ONLINE, got %v", got)
	}

	// A closed port reads DEGRADED (measured, not inferred).
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("test listener failed: %v", err)
	}
	dead := ln2.Addr().String()
	ln2.Close()
	srv.SetBridgeAddr(dead)
	if got := backendByName(t, fleetBackends(t, srv.Handler()), "local_simulator")["status"]; got != "DEGRADED" {
		t.Fatalf("dead bridge must read DEGRADED, got %v", got)
	}
}

func TestFleetCredentialPresentReadsReady(t *testing.T) {
	t.Setenv("KUBONA_IBM_QISKIT_TOKEN", "test-token-value")
	t.Setenv("KUBONA_QUANTINUUM_API_KEY", "")
	t.Setenv("KUBONA_RIGETTI_QCS_TOKEN", "")
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	backends := fleetBackends(t, srv.Handler())
	if got := backendByName(t, backends, "ibm_heron")["credential_state"]; got != "ready" {
		t.Fatalf("ibm_heron with token must read ready, got %v", got)
	}
	if got := backendByName(t, backends, "quantinuum_h1")["credential_state"]; got != "credentialless" {
		t.Fatalf("quantinuum_h1 without token must read credentialless, got %v", got)
	}
}
