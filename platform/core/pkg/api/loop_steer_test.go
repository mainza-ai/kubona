package api_test

// C10 API pins: steering is session-bound, refuses non-running runs, and the
// applied steer shows up in the run parts after the next step.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLoopSteerEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "steer drill")

	code, body := partsPost(t, h, "/api/loops", map[string]any{"session_id": sid, "max_steps": 4})
	if code != http.StatusCreated {
		t.Fatalf("register loop: %d %s", code, body)
	}
	var loop struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &loop); err != nil || loop.ID == "" {
		t.Fatalf("loop decode: %v %s", err, body)
	}

	// Blank text refuses.
	if code, _ := partsPost(t, h, "/api/loops/"+loop.ID+"/steer", map[string]any{"text": "   "}); code != http.StatusBadRequest {
		t.Fatalf("blank steer must 400, got %d", code)
	}
	code, body = partsPost(t, h, "/api/loops/"+loop.ID+"/steer", map[string]any{"text": "prefer shorter ansatz"})
	if code != http.StatusCreated {
		t.Fatalf("steer: %d %s", code, body)
	}
	if !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("steer must be pending until a step: %s", body)
	}
	// Ghost loop refuses.
	if code, _ := partsPost(t, h, "/api/loops/loop-missing/steer", map[string]any{"text": "x"}); code != http.StatusNotFound {
		t.Fatalf("ghost loop must 404, got %d", code)
	}

	// Step applies the steer; parts carry it.
	if code, body := partsPost(t, h, "/api/loops/"+loop.ID+"/step?grad=0.1", map[string]any{}); code != http.StatusOK {
		t.Fatalf("step: %d %s", code, body)
	}
	code, body = partsGet(t, h, "/api/loops/"+loop.ID+"/parts")
	if code != http.StatusOK {
		t.Fatalf("parts: %d %s", code, body)
	}
	if !strings.Contains(body, "Steering applied") || !strings.Contains(body, "prefer shorter ansatz") {
		t.Fatalf("applied steer must appear in the run lane: %s", body)
	}

	// A paused run cannot consume a steer: refuse 409, not a silent accept.
	if code, body := partsPost(t, h, "/api/loops/"+loop.ID+"/pause", map[string]any{}); code != http.StatusOK {
		t.Fatalf("pause: %d %s", code, body)
	}
	if code, body := partsPost(t, h, "/api/loops/"+loop.ID+"/steer", map[string]any{"text": "too late"}); code != http.StatusConflict || !strings.Contains(body, "loop_not_running") {
		t.Fatalf("paused steer must 409 loop_not_running, got %d %s", code, body)
	}
}
