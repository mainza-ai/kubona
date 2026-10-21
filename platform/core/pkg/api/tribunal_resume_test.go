package api

// T-16 pin: the resume route round-trips a real bundle. A template run
// (nil LLM = deterministic fallback, no network) is bundled, posted to
// /api/tribunal/resume, and must produce a fresh run with a FRESH
// obligation (closed history is never rewritten). Negatives: unparseable
// and empty bundles 400, ghosts 404.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func setupResumeDrill(t *testing.T) (*Server, *ledger.Ledger, string) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/resume.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)
	if _, err := srv.tenantMgr.CreateTenant("lab-res", "Resume Lab"); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessionMgr.CreateSession("resume drill", 100, "lab-res")
	if err != nil {
		t.Fatal(err)
	}
	return srv, l, sess.ID
}

func postResume(t *testing.T, srv *Server, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tribunal/resume", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestTribunalResumeRoundTrip(t *testing.T) {
	srv, _, sessID := setupResumeDrill(t)

	res, err := srv.collective.ExecuteTribunalWithSession(context.Background(), nil, nil, nil, sessID, "resume seed prompt")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := agents.MarshalTribunalBundle(res)
	if err != nil {
		t.Fatal(err)
	}
	code, out := postResume(t, srv, map[string]any{
		"session_id": sessID, "prompt": "continue the debate", "bundle": json.RawMessage(bundle),
	})
	if code != http.StatusOK {
		t.Fatalf("resume must 200, got %d: %+v", code, out)
	}
	trib, _ := out["tribunal"].(map[string]any)
	turns, _ := trib["turns"].([]any)
	if len(turns) != 4 {
		t.Errorf("resumed run must produce 4 new turns, got %d", len(turns))
	}
	newOb, _ := out["obligation_id"].(string)
	if newOb == "" {
		t.Fatal("resumed run must open an obligation")
	}
	if newOb == res.ObligationID {
		t.Errorf("resumed run must open a FRESH obligation, reused %s", newOb)
	}
}

func TestTribunalResumeRefusals(t *testing.T) {
	srv, _, sessID := setupResumeDrill(t)
	valid := map[string]any{"version": 1, "obligation_id": "ob-x", "target": "t", "molecule": "m",
		"turns": []any{map[string]any{"turn_index": 1}}, "synthesis": "s", "consensus_passed": false}

	if code, _ := postResume(t, srv, map[string]any{"session_id": sessID, "bundle": valid}); code != http.StatusOK {
		t.Errorf("bundle without prompt must 200 (default prompt), got %d", code)
	}
	if code, body := postResume(t, srv, map[string]any{"session_id": sessID, "prompt": "x", "bundle": json.RawMessage(`{nope`)}); code != http.StatusBadRequest {
		t.Errorf("unparseable bundle must 400, got %d: %v", code, body)
	}
	if code, _ := postResume(t, srv, map[string]any{"session_id": sessID, "prompt": "x"}); code != http.StatusBadRequest {
		t.Errorf("missing bundle must 400, got %d", code)
	}
	empty := map[string]any{"version": 1, "obligation_id": "ob-x", "target": "t", "turns": []any{}}
	if code, _ := postResume(t, srv, map[string]any{"session_id": sessID, "prompt": "x", "bundle": empty}); code != http.StatusBadRequest {
		t.Errorf("turnless bundle must 400, got %d", code)
	}
	if code, _ := postResume(t, srv, map[string]any{"session_id": "ghost", "prompt": "x", "bundle": valid}); code != http.StatusNotFound {
		t.Errorf("ghost session resume must 404, got %d", code)
	}
}
