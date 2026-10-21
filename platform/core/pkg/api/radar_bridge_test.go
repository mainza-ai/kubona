package api_test

// Audit 2026-10-21: /api/radar dialed 127.0.0.1:50051, which is loopback
// inside the kubonad container. In the shipped compose topology the bridge
// lives at bridge:50051 (unpublished), so a HEALTHY bridge rendered
// OFFLINE/DISCONNECTED in Fleet Radar. The radar must probe the configured
// --bridge address, same standard as the fleet liveness probe.

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRadarProbesConfiguredBridgeAddr(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	h := srv.Handler()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("dummy bridge listener: %v", err)
	}
	defer ln.Close()
	srv.SetBridgeAddr(ln.Addr().String())

	req := httptest.NewRequest(http.MethodGet, "/api/radar", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/radar, got %d", w.Code)
	}
	var out struct {
		Endpoints []struct {
			Port      int    `json:"port"`
			Listening bool   `json:"listening"`
			Service   string `json:"service"`
		} `json:"radar_endpoints"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode radar: %v", err)
	}
	for _, e := range out.Endpoints {
		if e.Port == 50051 {
			if !e.Listening {
				t.Errorf("50051 must report listening against the configured bridge addr %s: %+v", ln.Addr(), e)
			}
			return
		}
	}
	t.Errorf("radar response missing the 50051 endpoint: %+v", out.Endpoints)
}
