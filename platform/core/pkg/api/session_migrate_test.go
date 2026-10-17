package api_test

// S5 — session export/import + DeleteSession cascade. Threat: raw-DB
// copying diverges silently (G9); deletion orphans 5 of 8 tables (G6).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// waitJobsSettled polls the session's jobs until none are queued/running.
// Unused by the migration tests (their servers run no workers, so queued
// rows are stable) — kept for workerful servers, where comparing exports
// across execution would race status/result/completed_at moves.
func waitJobsSettled(t *testing.T, h http.Handler, sid string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs?session_id="+sid, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out struct {
			Jobs []jobs.Job `json:"jobs"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("jobs list: %v", err)
		}
		pending := false
		for _, j := range out.Jobs {
			if st := j.Status.String(); st == "JOB_STATUS_QUEUED" || st == "JOB_STATUS_RUNNING" {
				pending = true
			}
		}
		if !pending {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("jobs never settled for %s", sid)
}

func migPost(t *testing.T, h http.Handler, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	out["_raw"] = w.Body.String()
	return w.Code, out
}

func openFreshServer(t *testing.T, dbPath string) (*api.Server, *ledger.Ledger, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	return api.NewServer(l, sm, jr, sup, wd), l, func() { l.Close() }
}

func TestSessionExportImportRoundTrip(t *testing.T) {
	srv1, _, close1 := setupIdemServer(t)
	defer close1()
	h1 := srv1.Handler()
	sid := idemSession(t, h1, "migrate me")
	// Content: chat + job + canvas.
	if code, _, _ := idemPost(t, h1, "/api/chat/messages", map[string]any{"session_id": sid, "content": "hello migrate"}); code != http.StatusCreated {
		t.Fatalf("chat: %d", code)
	}
	if code, _, _ := idemPost(t, h1, "/api/jobs", map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator", "shots": 8}); code != http.StatusCreated {
		t.Fatalf("job: %d", code)
	}
	if code, _, _ := idemPost(t, h1, "/api/chat/launch", map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator"}); code != http.StatusCreated {
		t.Fatalf("launch: %d", code)
	}
	// No settle-wait: these servers run no workers, so queued rows are
	// stable (double-export determinism proven separately). A workerful
	// server would need terminal-state polling before comparing.

	code, exp := migPost(t, h1, "/api/sessions/export", map[string]any{"session_id": sid})
	if code != http.StatusOK {
		t.Fatalf("export: %d %v", code, exp)
	}
	if exp["version"] != "1" {
		t.Fatalf("manifest version must be 1, got %v", exp["version"])
	}
	digests, _ := exp["digests"].(map[string]any)
	for _, sec := range []string{"session", "messages", "jobs", "loops", "events", "campaign_state", "versions", "claims", "canvas_layout"} {
		if _, ok := digests[sec]; !ok {
			t.Fatalf("manifest must digest %s", sec)
		}
	}

	// Import into a FRESH database: all rows land, one activation record.
	srv2, l2, close2 := openFreshServer(t, t.TempDir()+"/import.db")
	defer close2()
	code, imp := migPost(t, srv2.Handler(), "/api/sessions/import", map[string]any{"manifest": exp})
	if code != http.StatusOK {
		t.Fatalf("import: %d %v", code, imp)
	}
	if imp["session_id"] != sid {
		t.Fatalf("import must preserve the session id, got %v", imp)
	}
	// Re-export from the new DB: every section except events must digest
	// identically (events gain exactly the session_imported activation
	// record — expected drift, asserted separately below).
	code, exp2 := migPost(t, srv2.Handler(), "/api/sessions/export", map[string]any{"session_id": sid})
	if code != http.StatusOK {
		t.Fatalf("re-export: %d", code)
	}
	d1, _ := exp["digests"].(map[string]any)
	d2, _ := exp2["digests"].(map[string]any)
	for sec, h1 := range d1 {
		if sec == "events" {
			continue
		}
		if h2, ok := d2[sec]; !ok || h1 != h2 {
			t.Fatalf("section %s digest drifted across migration: %v vs %v", sec, h1, h2)
		}
	}
	var n int
	if err := l2.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE session_id = ? AND event_type = 'session_imported'`, sid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("exactly one activation record: %v %d", err, n)
	}

	// Double import replays (no duplicate rows).
	code, imp2 := migPost(t, srv2.Handler(), "/api/sessions/import", map[string]any{"manifest": exp})
	if code != http.StatusOK || imp2["replayed"] != true {
		t.Fatalf("double import must replay, got %d %v", code, imp2)
	}
	if err := l2.DB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("replay must not duplicate the session: %v %d", err, n)
	}

	// Ghost export 404s.
	if code, _ := migPost(t, h1, "/api/sessions/export", map[string]any{"session_id": "sess-ghost"}); code != http.StatusNotFound {
		t.Fatalf("ghost export must 404, got %d", code)
	}
}

func TestSessionImportRefusals(t *testing.T) {
	srv1, _, close1 := setupIdemServer(t)
	defer close1()
	h1 := srv1.Handler()
	sid := idemSession(t, h1, "migrate refusals")
	if code, _, _ := idemPost(t, h1, "/api/chat/messages", map[string]any{"session_id": sid, "content": "x"}); code != http.StatusCreated {
		t.Fatalf("chat: %d", code)
	}
	_, exp := migPost(t, h1, "/api/sessions/export", map[string]any{"session_id": sid})

	srv2, _, close2 := openFreshServer(t, t.TempDir()+"/refuse.db")
	defer close2()
	h2 := srv2.Handler()
	clone := func() map[string]any {
		raw, _ := json.Marshal(exp)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		return m
	}
	// Tampered content (digest mismatch).
	bad := clone()
	msgs := bad["messages"].([]any)
	msgs[0].(map[string]any)["content"] = "tampered"
	if code, body := migPost(t, h2, "/api/sessions/import", map[string]any{"manifest": bad}); code != http.StatusBadRequest {
		t.Fatalf("tampered manifest must 400, got %d %v", code, body)
	} else if !strings.Contains(strings.ToLower(body["_raw"].(string)), "digest") {
		t.Fatalf("tamper refusal must name digests, got %v", body)
	}
	// Unknown major version.
	bad = clone()
	bad["version"] = "2"
	if code, body := migPost(t, h2, "/api/sessions/import", map[string]any{"manifest": bad}); code != http.StatusBadRequest {
		t.Fatalf("v2 manifest must 400, got %d %v", code, body)
	} else if !strings.Contains(strings.ToLower(body["_raw"].(string)), "unsupported_manifest_version") {
		t.Fatalf("version refusal must name it, got %v", body)
	}
	// Oversize canvas.
	bad = clone()
	bad["canvas_layout"] = strings.Repeat("x", 300*1024)
	if code, _ := migPost(t, h2, "/api/sessions/import", map[string]any{"manifest": bad}); code != http.StatusBadRequest {
		t.Fatalf("oversize canvas must 400, got %d", code)
	}
	// Unparseable manifest.
	if code, _ := migPost(t, h2, "/api/sessions/import", map[string]any{"manifest": "nope"}); code != http.StatusBadRequest {
		t.Fatalf("garbage manifest must 400, got %d", code)
	}
	// Missing manifest.
	if code, _ := migPost(t, h2, "/api/sessions/import", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("missing manifest must 400, got %d", code)
	}
}

func TestSessionImportValidatesBeforeReplay(t *testing.T) {
	// A tampered manifest must 400 even when its session already lives
	// here: replay-by-existence must never launder corrupt bytes.
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "validate-first")
	if code, _, _ := idemPost(t, h, "/api/chat/messages", map[string]any{"session_id": sid, "content": "orig"}); code != http.StatusCreated {
		t.Fatalf("chat: %d", code)
	}
	_, exp := migPost(t, h, "/api/sessions/export", map[string]any{"session_id": sid})
	raw, _ := json.Marshal(exp)
	var tampered map[string]any
	_ = json.Unmarshal(raw, &tampered)
	jobs := tampered["jobs"].([]any)
	_ = jobs
	msgs := tampered["messages"].([]any)
	msgs[0].(map[string]any)["content"] = "evil-edit"
	if code, body := migPost(t, h, "/api/sessions/import", map[string]any{"manifest": tampered}); code != http.StatusBadRequest {
		t.Fatalf("tampered-existing must 400, got %d %v", code, body)
	}
	// Untouched re-import still replays.
	if code, imp := migPost(t, h, "/api/sessions/import", map[string]any{"manifest": exp}); code != http.StatusOK || imp["replayed"] != true {
		t.Fatalf("clean re-import must replay, got %d %v", code, imp)
	}
}

func TestSessionContextReadout(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "ctx readout")

	get := func(sid string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/context?session_id="+sid, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	if code, _ := get("sess-ghost"); code != http.StatusNotFound {
		t.Fatalf("ghost context must 404, got %d", code)
	}
	if code, _ := get(""); code != http.StatusBadRequest {
		t.Fatalf("empty context must 400, got %d", code)
	}
	code, empty := get(sid)
	if code != http.StatusOK {
		t.Fatalf("context: %d", code)
	}
	if empty["turns"] != float64(0) || empty["pressure_pct"] != float64(0) {
		t.Fatalf("fresh session must read zero, got %v", empty)
	}
	if empty["turn_cap"] != float64(40) || empty["history_chars_cap"] != float64(48000) {
		t.Fatalf("caps must be the BudgetHistory consts, got %v", empty)
	}
	if code, _, _ := idemPost(t, h, "/api/chat/messages", map[string]any{"session_id": sid, "content": "hello context"}); code != http.StatusCreated {
		t.Fatalf("chat: %d", code)
	}
	code, full := get(sid)
	if code != http.StatusOK {
		t.Fatalf("context: %d", code)
	}
	if full["turns"] != float64(2) {
		t.Fatalf("user+assistant = 2 turns, got %v", full["turns"])
	}
	if full["history_chars"].(float64) <= 0 || full["pressure_pct"].(float64) <= 0 {
		t.Fatalf("nonempty transcript must read positive, got %v", full)
	}
	if full["would_drop"] != float64(0) {
		t.Fatalf("small transcript drops nothing, got %v", full)
	}
}
