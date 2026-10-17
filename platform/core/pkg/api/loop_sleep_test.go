package api_test

// S3 — loop sleep/wake/drain at the HTTP edge + sweeper + boot reconcile.
// Threat: idle loops hold resources forever; restarts lose or refill loops.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sleepStartLoop(t *testing.T, h http.Handler, sid string) string {
	t.Helper()
	code, body, _ := idemPost(t, h, "/api/autoresearch/start", map[string]any{
		"session_id": sid, "harness_id": "ansatz-search", "max_iterations": 2,
	})
	if code != http.StatusCreated {
		t.Fatalf("loop start: %d %s", code, body)
	}
	var st struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || st.ID == "" {
		t.Fatalf("loop id: %v %s", err, body)
	}
	return st.ID
}

func loopAction(t *testing.T, h http.Handler, loopID, action string, body any) (int, string) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+loopID+"/"+action, bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestLoopSleepWakeDrain(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "loop sleep")
	lid := sleepStartLoop(t, h, sid)

	// Persisted on start (G5 fixed: loops table previously had no writer).
	rows, err := l.ListLoopStates()
	if err != nil || len(rows) != 1 || rows[0].ID != lid {
		t.Fatalf("loop row must persist on start: %v %+v", err, rows)
	}

	if code, body := loopAction(t, h, lid, "sleep", nil); code != http.StatusOK {
		t.Fatalf("sleep: %d %s", code, body)
	} else if !strings.Contains(body, "sleeping") {
		t.Fatalf("sleep must latch sleeping, got %s", body)
	}
	// Sleeping loops refuse steps.
	if code, _ := loopAction(t, h, lid, "step", nil); code == http.StatusOK {
		t.Fatalf("step on sleeping loop must refuse")
	}
	// Wake is S1-keyed: same key replays, never forks.
	wake := map[string]any{"request_id": "wake-1"}
	code, body1, _ := func() (int, string, http.Header) {
		raw, _ := json.Marshal(wake)
		req := httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+lid+"/wake", bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String(), w.Header()
	}()
	if code != http.StatusOK {
		t.Fatalf("wake: %d %s", code, body1)
	}
	code2, body2 := loopAction(t, h, lid, "wake", wake)
	_ = code2
	_ = body2
	// Second wake with same key: replay path returns stored state; a fresh
	// key on a running loop 409s (only sleeping/paused/stagnant wake).
	raw, _ := json.Marshal(wake)
	req := httptest.NewRequest(http.MethodPost, "/api/autoresearch/loops/"+lid+"/wake", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("wake replay must 200, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("wake replay must carry the header")
	}
	// Drain latch refuses steps with the drain reason.
	if code, body := loopAction(t, h, lid, "drain", map[string]any{"drain": true}); code != http.StatusOK {
		t.Fatalf("drain: %d %s", code, body)
	}
	if code, body := loopAction(t, h, lid, "step", nil); code == http.StatusOK {
		t.Fatalf("drained step must refuse, got %s", body)
	}
	if code, body := loopAction(t, h, lid, "drain", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("drain without flag must 400, got %d %s", code, body)
	}
	if code, _ := loopAction(t, h, lid, "drain", map[string]any{"drain": false}); code != http.StatusOK {
		t.Fatalf("undrain must 200, got %d", code)
	}
	// Sleep a finished loop is a conflict (no history rewriting).
	if code, _ := loopAction(t, h, lid, "stop", nil); code != http.StatusOK {
		t.Fatalf("stop: %d", code)
	}
	if code, body := loopAction(t, h, lid, "sleep", nil); code != http.StatusConflict {
		t.Fatalf("sleep on aborted loop must 409, got %d %s", code, body)
	}
}

func TestLoopSweeperSleepsIdle(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "sweeper")
	lid := sleepStartLoop(t, h, sid)

	// Fresh loop: sweeper with a generous threshold sleeps nothing.
	if slept := srv.SweepIdleLoops(time.Now().UTC(), time.Hour); len(slept) != 0 {
		t.Fatalf("fresh loop must not sleep, slept %v", slept)
	}
	// Zero threshold: everything idle sleeps exactly once.
	slept := srv.SweepIdleLoops(time.Now().UTC(), 0)
	if len(slept) != 1 || slept[0] != lid {
		t.Fatalf("idle loop must sleep, got %v", slept)
	}
	if slept := srv.SweepIdleLoops(time.Now().UTC(), 0); len(slept) != 0 {
		t.Fatalf("sleeping loop must not re-sleep, got %v", slept)
	}
	rows, err := l.ListLoopStates()
	if err != nil || len(rows) != 1 || rows[0].Status != "sleeping" {
		t.Fatalf("swept row must persist sleeping: %v %+v", err, rows)
	}
	// Drained loops are exempt even at zero threshold.
	lid2 := sleepStartLoop(t, h, sid)
	if code, _ := loopAction(t, h, lid2, "drain", map[string]any{"drain": true}); code != http.StatusOK {
		t.Fatalf("drain: %d", code)
	}
	// Undrain lid (running) so only the drained one is exempt... lid is
	// sleeping; wake it first via fresh key, then sweep: only running
	// non-drained loops sleep.
	if code, _ := loopAction(t, h, lid, "wake", map[string]any{"request_id": "wake-sweep"}); code != http.StatusOK {
		t.Fatalf("wake: %d", code)
	}
	slept = srv.SweepIdleLoops(time.Now().UTC(), 0)
	if len(slept) != 1 || slept[0] != lid {
		t.Fatalf("only the non-drained loop must sleep, got %v", slept)
	}
}
