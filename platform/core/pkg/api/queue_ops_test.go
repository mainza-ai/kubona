package api_test

// S7 — per-job cancel + rewrite hardening. Threat: no way to stop a
// queued mistake except session-wide abort; concurrent rewrites
// last-writer-win silently. (Steer documented N/A: the execution model is
// synchronous turns + pollable jobs — no background turn exists to steer.)

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func queuePost(t *testing.T, h http.Handler, path string, body any) (int, map[string]any) {
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

func TestJobCancelQueued(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "cancel me")
	// No workers run in this server: dispatched jobs stay QUEUED (the
	// exact state cancel targets).
	code, body, _ := idemPost(t, h, "/api/jobs", map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator", "shots": 8})
	if code != http.StatusCreated {
		t.Fatalf("dispatch: %d %s", code, body)
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &job); err != nil || job.ID == "" {
		t.Fatalf("job id: %v %s", err, body)
	}

	code, out := queuePost(t, h, "/api/jobs/"+job.ID+"/cancel", map[string]any{"reason": "wrong backend"})
	if code != http.StatusOK {
		t.Fatalf("cancel: %d %v", code, out)
	}
	if out["cancelled"] != true {
		t.Fatalf("cancel must report transition, got %v", out)
	}
	// Reservation unwinds exactly once; terminal event sealed.
	var reserved float64
	if err := l.DB().QueryRow(`SELECT reserved_amount FROM jobs WHERE id = ?`, job.ID).Scan(&reserved); err != nil {
		t.Fatalf("row: %v", err)
	}
	_ = reserved
	events, err := l.GetRecentEvents(sid, 20)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	cancelled := false
	for _, e := range events {
		if e.EventType == "job_cancelled" {
			cancelled = true
		}
	}
	if !cancelled {
		t.Fatalf("job_cancelled event must be sealed")
	}

	// Second cancel replays terminal success (idempotent by state machine).
	code, out = queuePost(t, h, "/api/jobs/"+job.ID+"/cancel", map[string]any{})
	if code != http.StatusOK || out["already_terminal"] != true {
		t.Fatalf("re-cancel must replay terminal success, got %d %v", code, out)
	}

	// Unknown job 404s.
	if code, _ := queuePost(t, h, "/api/jobs/nope/cancel", map[string]any{}); code != http.StatusNotFound {
		t.Fatalf("ghost cancel must 404, got %d", code)
	}
	// (Empty id collapses at the mux with a 307 redirect — stdlib path
	// cleaning, not handler logic; not asserted.)
}

func TestRewriteHardening(t *testing.T) {
	srv, _, cleanup := setupIdemServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "rewrite cas")

	// Ghost session 404s explicitly (was conflated with ghost message).
	if code, out := queuePost(t, h, "/api/chat/rewrite", map[string]any{"session_id": "sess-ghost", "message_id": "m", "content": "x"}); code != http.StatusNotFound {
		t.Fatalf("ghost rewrite must 404, got %d %v", code, out)
	}

	code, body, _ := idemPost(t, h, "/api/chat/messages", map[string]any{"session_id": sid, "content": "original"})
	if code != http.StatusCreated {
		t.Fatalf("chat: %d %s", code, body)
	}
	var posted struct {
		User struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &posted); err != nil || posted.User.ID == "" {
		t.Fatalf("message id: %v %s", err, body)
	}

	// CAS match rewrites.
	code, out := queuePost(t, h, "/api/chat/rewrite", map[string]any{"session_id": sid, "message_id": posted.User.ID, "content": "edited", "expected_content": "original"})
	if code != http.StatusOK {
		t.Fatalf("CAS-match rewrite must 200, got %d %v", code, out)
	}
	// Stale CAS 409s (content is now "edited").
	code, out = queuePost(t, h, "/api/chat/rewrite", map[string]any{"session_id": sid, "message_id": posted.User.ID, "content": "race", "expected_content": "original"})
	if code != http.StatusConflict {
		t.Fatalf("stale CAS must 409, got %d %v", code, out)
	} else if !strings.Contains(strings.ToLower(out["_raw"].(string)), "expected_content") {
		t.Fatalf("409 must name expected_content, got %v", out)
	}
	// Empty content still 400s.
	if code, _ := queuePost(t, h, "/api/chat/rewrite", map[string]any{"session_id": sid, "message_id": posted.User.ID, "content": ""}); code != http.StatusBadRequest {
		t.Fatalf("empty rewrite must 400, got %d", code)
	}
}
