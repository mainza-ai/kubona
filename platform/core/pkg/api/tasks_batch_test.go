package api_test

// C8 pins: batch task creation is plan-ordered and idempotent under one key.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTaskBatchOrderAndReplay(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	sid := idemSession(t, h, "plan tasks")

	body := map[string]any{
		"session_id": sid, "request_id": "plan-batch-1",
		"tasks": []map[string]any{
			{"title": "Run VQE at 0.9 A", "source_domain": "plan", "source_id": "wf-1", "order": 2},
			{"title": "Compute classical anchor", "source_domain": "plan", "source_id": "wf-1", "order": 1},
		},
	}
	code, first, header := idemPost(t, h, "/api/tasks/batch", body)
	if code != http.StatusOK {
		t.Fatalf("batch create: %d %s", code, first)
	}
	var out struct {
		Tasks []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"tasks"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(first), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || len(out.Tasks) != 2 {
		t.Fatalf("two tasks expected: %s", first)
	}
	// `order` wins over array order.
	if out.Tasks[0].Title != "Compute classical anchor" || out.Tasks[1].Title != "Run VQE at 0.9 A" {
		t.Fatalf("plan order not preserved: %+v", out.Tasks)
	}

	code, replay, replayHeader := idemPost(t, h, "/api/tasks/batch", body)
	if code != http.StatusOK || replay != first || replayHeader.Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("batch replay must be identical: %d %s", code, replay)
	}
	_ = header

	// Validation: blank title and oversize batch.
	if code, _, _ := idemPost(t, h, "/api/tasks/batch", map[string]any{
		"session_id": sid, "tasks": []map[string]any{{"title": "  "}},
	}); code != http.StatusBadRequest {
		t.Fatalf("blank title must 400, got %d", code)
	}
	tooMany := make([]map[string]any, 51)
	for i := range tooMany {
		tooMany[i] = map[string]any{"title": "t"}
	}
	if code, _, _ := idemPost(t, h, "/api/tasks/batch", map[string]any{"session_id": sid, "tasks": tooMany}); code != http.StatusBadRequest {
		t.Fatalf("oversize batch must 400, got %d", code)
	}

	// The single-task shape still works.
	if code, body, _ := idemPost(t, h, "/api/tasks", map[string]any{"session_id": sid, "title": "solo"}); code != http.StatusOK || !strings.Contains(body, `"title":"solo"`) {
		t.Fatalf("single task must still work: %d %s", code, body)
	}
}

func TestTaskCreationRefusesGhostSessionsBeforeRows(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()
	// Bearer must not bypass session attribution: ghosts 404 before any row,
	// never a 500 from the foreign-key write.
	if code, body, _ := idemPost(t, h, "/api/tasks/batch", map[string]any{
		"session_id": "sess-ghost", "request_id": "ghost-batch",
		"tasks": []map[string]any{{"title": "t"}},
	}); code != http.StatusNotFound {
		t.Fatalf("ghost batch must 404, got %d %s", code, body)
	}
	if code, body, _ := idemPost(t, h, "/api/tasks", map[string]any{
		"session_id": "sess-ghost", "title": "t",
	}); code != http.StatusNotFound {
		t.Fatalf("ghost single create must 404, got %d %s", code, body)
	}
}
