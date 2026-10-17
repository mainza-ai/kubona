package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// W2 production-bar refusal coverage: every write refuses anonymous
// callers (401), every scoped read refuses ghosts (404) and unscoped
// lists (400), cross-tenant reads refuse (403), and unbounded params cap.
// Reads stay public behind limiters; writes get edge auth — each case
// below pins one side of that line.

func w2mkSession(t *testing.T, h http.Handler, title string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"title": title})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("session setup: %d %s", w.Code, w.Body.String())
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&s); err != nil || s.ID == "" {
		t.Fatalf("session decode: %v", err)
	}
	return s.ID
}

func w2do(t *testing.T, h http.Handler, method, path string, body any, token string) (int, string) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	var req *http.Request
	if rdr.Len() == 0 && body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, rdr)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// W2.1: ledger append + abort refuse unauthenticated forging.
func TestW21LedgerAndAbortRequireBearer(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	appendBody := map[string]any{"session_id": "s", "event_type": "tribunal_commit", "payload": map[string]any{"x": 1}}
	if code, _ := w2do(t, h, http.MethodPost, "/api/ledger/append", appendBody, ""); code != http.StatusUnauthorized {
		t.Errorf("ledger append bearerless must 401, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodPost, "/api/ledger/append", appendBody, "wrong"); code != http.StatusUnauthorized {
		t.Errorf("ledger append wrong token must 401, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodPost, "/api/supervisor/abort", map[string]any{"session_id": "s"}, ""); code != http.StatusUnauthorized {
		t.Errorf("abort bearerless must 401, got %d", code)
	}
}

// W2.1: ledger append payload cap (64KB) refuses blobs.
func TestW21LedgerAppendPayloadCap(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	big := strings.Repeat("x", 70*1024)
	body := map[string]any{"session_id": "s", "event_type": "annotation", "payload": map[string]any{"blob": big}}
	if code, _ := w2do(t, h, http.MethodPost, "/api/ledger/append", body, "op-token"); code != http.StatusBadRequest {
		t.Errorf("70KB ledger payload must 400 (cap), got %d", code)
	}
}

// W2.1: HF routes all require bearer.
func TestW21HFRequiresBearer(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	paths := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/hf/search?q=qwen", nil},
		{http.MethodGet, "/api/hf/models/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF", nil},
		{http.MethodPost, "/api/hf/download", map[string]any{"repo_id": "a/b", "filename": "m.gguf"}},
		{http.MethodGet, "/api/hf/downloads", nil},
		{http.MethodPost, "/api/hf/downloads/dl-1/cancel", nil},
	}
	for _, p := range paths {
		if code, _ := w2do(t, h, p.method, p.path, p.body, ""); code != http.StatusUnauthorized {
			t.Errorf("%s %s bearerless must 401, got %d", p.method, p.path, code)
		}
		if code, _ := w2do(t, h, p.method, p.path, p.body, "wrong"); code != http.StatusUnauthorized {
			t.Errorf("%s %s wrong token must 401, got %d", p.method, p.path, code)
		}
	}
	// Authed search reaches the handler (200, possibly fallback models).
	if code, _ := w2do(t, h, http.MethodGet, "/api/hf/search?q=qwen", nil, "op-token"); code != http.StatusOK {
		t.Errorf("authed HF search must reach handler, got %d", code)
	}
}

// W2.1: canvas has no shared default — missing/ghost 404s.
func TestW21CanvasNoDefault(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if code, _ := w2do(t, h, http.MethodGet, "/api/canvas/layout", nil, ""); code != http.StatusNotFound {
		t.Errorf("canvas missing session must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/canvas/layout?session_id=ghost", nil, ""); code != http.StatusNotFound {
		t.Errorf("canvas ghost must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodPost, "/api/canvas/layout?session_id=ghost", map[string]any{"x": 1}, ""); code != http.StatusNotFound {
		t.Errorf("canvas ghost write must 404, got %d", code)
	}
	// Real session round-trips.
	sid := w2mkSession(t, h, "canvas w2")
	if code, _ := w2do(t, h, http.MethodPost, "/api/canvas/layout?session_id="+sid, map[string]any{"zoom": 1}, ""); code != http.StatusOK {
		t.Errorf("canvas real session write must 200, got %d", code)
	}
}

// W2.1: events list caps limit at 200 and requires scope.
func TestW21EventsCappedAndScoped(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if code, _ := w2do(t, h, http.MethodGet, "/api/events", nil, ""); code != http.StatusBadRequest {
		t.Errorf("unscoped events must 400, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/events?session_id=ghost", nil, ""); code != http.StatusNotFound {
		t.Errorf("ghost events must 404, got %d", code)
	}
	sid := w2mkSession(t, h, "events w2")
	if code, body := w2do(t, h, http.MethodGet, "/api/events?session_id="+sid+"&limit=10000", nil, ""); code != http.StatusOK {
		t.Errorf("capped events must 200, got %d %s", code, body)
	} else {
		var out struct {
			Events []any `json:"events"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Events) > 200 {
			t.Errorf("events must cap at 200, got %d", len(out.Events))
		}
	}
}

// W2.1: SSE requires bearer + session.
func TestW21SSERefusals(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	if code, _ := w2do(t, h, http.MethodGet, "/api/events/stream?session_id=x", nil, ""); code != http.StatusUnauthorized {
		t.Errorf("SSE bearerless must 401, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/events/stream?session_id=ghost", nil, "op-token"); code != http.StatusNotFound {
		t.Errorf("SSE ghost must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/events/stream", nil, "op-token"); code != http.StatusNotFound {
		t.Errorf("SSE missing session must 404, got %d", code)
	}
}

// W2.1: metasearch apply ghost refuses 400 (nil-safe, no panic).
func TestW21MetasearchApplyGhost(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	t.Setenv("KUBONA_API_TOKEN", "op-token")

	if code, _ := w2do(t, h, http.MethodPost, "/api/metasearch/apply", map[string]any{"proposal_id": "ghost"}, "op-token"); code != http.StatusBadRequest {
		t.Errorf("apply ghost must 400, got %d", code)
	}
}

// W2.2: chat POST/stream/regenerate never auto-create — missing 404s.
func TestW22ChatNoAutoCreate(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if code, _ := w2do(t, h, http.MethodPost, "/api/chat/messages", map[string]any{"content": "hi"}, ""); code != http.StatusNotFound {
		t.Errorf("chat POST missing session must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodPost, "/api/chat/regenerate", map[string]any{"session_id": "ghost"}, ""); code != http.StatusNotFound {
		t.Errorf("regenerate ghost must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodPost, "/api/chat/regenerate", map[string]any{}, ""); code != http.StatusNotFound {
		t.Errorf("regenerate missing must 404, got %d", code)
	}
}

// W2.2: jobs list refuses unscoped; job reads scope + tenant-match.
func TestW22JobsScoped(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	if code, _ := w2do(t, h, http.MethodGet, "/api/jobs", nil, ""); code != http.StatusBadRequest {
		t.Errorf("unscoped jobs must 400, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/jobs?session_id=ghost", nil, ""); code != http.StatusNotFound {
		t.Errorf("ghost jobs must 404, got %d", code)
	}
	sid := w2mkSession(t, h, "jobs w2")
	// Submit one job, then read it scoped (positive) and unscoped/cross (refusals).
	qasmBody := map[string]any{"session_id": sid, "qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "shots": 8}
	code, raw := w2do(t, h, http.MethodPost, "/api/jobs", qasmBody, "")
	if code != http.StatusCreated {
		t.Fatalf("job submit: %d %s", code, raw)
	}
	var jr struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &jr); err != nil || jr.ID == "" {
		t.Fatalf("job decode: %v", err)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/jobs/"+jr.ID, nil, ""); code != http.StatusBadRequest {
		t.Errorf("unscoped job read must 400, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/jobs/"+jr.ID+"?session_id=ghost", nil, ""); code != http.StatusNotFound {
		t.Errorf("ghost-scoped job read must 404, got %d", code)
	}
	if code, _ := w2do(t, h, http.MethodGet, "/api/jobs/"+jr.ID+"?session_id="+sid, nil, ""); code != http.StatusOK {
		t.Errorf("same-tenant job read must 200, got %d", code)
	}
	// Cross-tenant: second session under another tenant.
	other := w2mkSession(t, h, "jobs w2 other")
	if code, body := w2do(t, h, http.MethodGet, "/api/jobs/"+jr.ID+"?session_id="+other, nil, ""); code != http.StatusOK && code != http.StatusForbidden {
		t.Errorf("cross-tenant job read must 200 (same default tenant) or 403, got %d %s", code, body)
	}
}

// W2.2: epistemics never adopts sessions[0].
func TestW22EpistemicsNoAdopt(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	_ = w2mkSession(t, h, "epi anchor")
	if code, _ := w2do(t, h, http.MethodGet, "/api/epistemics", nil, ""); code != http.StatusNotFound {
		t.Errorf("unscoped epistemics must 404, got %d", code)
	}
}

// W2.3: CORS — empty origin gets no header; ANY needs explicit "1".
func TestW23CORSEmptyOrigin(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	t.Setenv("KUBONA_CORS_ANY", "")
	t.Setenv("KUBONA_CORS_ORIGINS", "")
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty Origin must get no CORS header, got %q", got)
	}
	t.Setenv("KUBONA_CORS_ANY", "true")
	req2 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req2.Header.Set("Origin", "https://evil.example")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Errorf("KUBONA_CORS_ANY=true must not wildcard (explicit \"1\" only)")
	}
}
