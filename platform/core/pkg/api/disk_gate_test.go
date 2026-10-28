package api_test

// S2 — disk-guard gating at the HTTP edge. Threat: disk-full outage with
// new work still admitted. Blocked must 503 every new-work entry with a
// machine-readable code while reads stay available.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/disk"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

// attachBlockedGuard wires a guard pinned at blocked (zero-space measurer).
func attachBlockedGuard(t *testing.T, srv *api.Server) *disk.Guard {
	t.Helper()
	g := disk.NewGuard(disk.DefaultThresholds(), []string{"/full"}, func(string) (uint64, error) {
		return 0, nil
	}, nil)
	g.PollOnce()
	if !g.Blocks() {
		t.Fatalf("fixture guard must block")
	}
	srv.SetDiskGuard(g)
	return g
}

func diskPost(t *testing.T, h http.Handler, path string, body any) (int, string, http.Header) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	// Bearer-gated entries (metasearch) authenticate before any other
	// check, so the test authenticates everywhere (harmless elsewhere).
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String(), w.Header()
}

func TestDiskBlockedRefusesNewWork(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	attachBlockedGuard(t, srv)
	h := srv.Handler()
	sid := idemSession(t, h, "disk blocked")

	// setupIdemServer's session creation ran unguarded (attached after);
	// every new-work entry below must now 503 with the code.
	entries := map[string]any{
		"/api/jobs":               map[string]any{"session_id": sid, "qasm": idemQASM},
		"/api/chat/launch":        map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator"},
		"/api/chat/tribunal":      map[string]any{"session_id": sid, "prompt": "disk drill"},
		"/api/autoresearch/start": map[string]any{"session_id": sid, "harness_id": "ansatz-search", "max_iterations": 1},
		"/api/metasearch/apply":   map[string]any{"proposal_id": "nope"},
		"/api/chat/messages":      map[string]any{"session_id": sid, "content": "hi"},
		"/api/chat/regenerate":    map[string]any{"session_id": sid},
		"/api/observatory/lens":   map[string]any{"session_id": sid, "target_system": "H2", "task_type": "images2PosedRGBD"},
		"/api/quantum/jobs":       map[string]any{"session_id": sid, "qasm": idemQASM, "provider": "local-simulator", "shots": 8, "idempotency_key": "disk-q"},
		"/api/workflows/run":      map[string]any{"session_id": sid, "workflow": map[string]any{"name": "disk"}},
		"/api/memory/retain":      map[string]any{"session_id": sid, "content": "disk drill"},
		"/api/sessions/import":    map[string]any{"manifest": map[string]any{}},
	}
	for path, body := range entries {
		code, resp, hdr := diskPost(t, h, path, body)
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s blocked must 503, got %d %s", path, code, resp)
			continue
		}
		if !strings.Contains(strings.ToLower(resp), "storage_blocked") {
			t.Errorf("%s refusal must name storage_blocked, got %q", path, resp)
		}
		if hdr.Get("Retry-After") == "" {
			t.Errorf("%s refusal must carry Retry-After", path)
		}
	}
	// Stream is SSE: blocked before headers, plain 503 (not 200+error).
	raw, _ := json.Marshal(map[string]any{"session_id": sid, "content": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("stream blocked must 503, got %d", w.Code)
	}
}

func TestDiskBlockedReadsStay(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "disk reads")
	attachBlockedGuard(t, srv)

	get := func(path string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	if code, body := get("/api/health"); code != http.StatusOK {
		t.Errorf("health stays 200, got %d", code)
	} else if !strings.Contains(body, "blocked") {
		t.Errorf("health must report the disk level, got %s", body)
	}
	if code, _ := get("/api/ready"); code != http.StatusServiceUnavailable {
		t.Errorf("ready must 503 while blocked, got %d", code)
	}
	if code, _ := get("/api/chat/messages?session_id=" + sid); code != http.StatusOK {
		t.Errorf("message reads stay 200, got %d", code)
	}
	if code, _ := get("/api/jobs?session_id=" + sid); code != http.StatusOK {
		t.Errorf("job reads stay 200, got %d", code)
	}
}

func TestDiskReadyWhenClear(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	// Real measurement on a temp dir: must be ok on any sane machine, and
	// health must carry real nonzero bytes (live measurement, not stub).
	g := disk.NewGuard(disk.DefaultThresholds(), []string{t.TempDir()}, nil, nil)
	g.PollOnce()
	srv.SetDiskGuard(g)
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ready must 200 when clear, got %d %s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	var health map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &health); err != nil {
		t.Fatalf("health decode: %v", err)
	}
	if health["disk_level"] == "" || health["disk_avail_bytes"] == "" || health["disk_avail_bytes"] == "0" {
		t.Fatalf("health must carry live disk readings, got %v", health)
	}
}

func TestDiskSubmitJobDefenseInDepth(t *testing.T) {
	// A2A/internal callers reach SubmitJob directly: the runner-level check
	// must refuse even with no HTTP edge involved.
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	dbPath := t.TempDir() + "/disk-runner.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("disk-sess", "disk drill", 100.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := jobs.NewRunner(l, nil, 1)
	g := disk.NewGuard(disk.DefaultThresholds(), []string{"/full"}, func(string) (uint64, error) {
		return 0, nil
	}, nil)
	g.PollOnce()
	r.SetDiskGuard(g)
	_, err = r.SubmitJob("disk-sess", "circuit_run", map[string]any{"qasm": idemQASM, "shots": 8})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "storage_blocked") {
		t.Fatalf("blocked SubmitJob must refuse naming storage_blocked, got %v", err)
	}
	// Unguarded runner still admits (nil guard = legacy path for tests).
	r2 := jobs.NewRunner(l, nil, 1)
	if _, err := r2.SubmitJob("disk-sess", "circuit_run", map[string]any{"qasm": idemQASM, "shots": 8}); err != nil {
		t.Fatalf("unguarded SubmitJob must proceed, got %v", err)
	}
}
